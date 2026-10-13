/**
 * Topology assembly.
 *
 * This is where the design's honesty rules are actually implemented:
 *
 *  - Nodes are the static registry, merged with live pods from the Kubernetes
 *    API. A registry node with no pods is still drawn, with no pods on it.
 *  - Per-pod rate, p50, p99 and error ratio come from
 *    `http_server_request_duration_seconds` keyed by `pod`. A pod Prometheus has
 *    not scraped has `null` everywhere, not zero.
 *  - Edges come only from the OTel service graph metrics. There is no fallback,
 *    no inference from the registry, and no drawing of edges the design document
 *    says exist but telemetry has not confirmed.
 *  - A `messaging_system` pair (a producer span paired with its consumer span)
 *    is a message that went through NATS, so it is drawn as two edges,
 *    producer -> nats and nats -> consumer, not as a line between the services.
 *  - Backing stores are not instrumented and have no HTTP server metrics, so a
 *    store node's rate, latency and error ratio are those of the edges into it.
 *  - A service-graph peer that is not a registry node is reported in `unmapped`
 *    so the UI can say how much traffic it is not drawing, rather than inventing
 *    a node or silently dropping the edge.
 */

import {
  classifyHealth,
  type EdgeView,
  type Measure,
  type NodeView,
  type PodView,
  type SourceStatus,
  type TopologySnapshot,
  type UnmappedEdge,
} from '../../shared/topology.js'
import { IGNORED_PEERS, REGISTRY, isStore, resolvePeer } from '../../shared/registry.js'
import type { ClusterSnapshot } from './kubernetes.js'
import { byEdge, byLabel, pairKey, type EdgeSample, type PromQueryResult } from './prometheus.js'

/** The registry node every `messaging_system` pair is routed through. */
export const BUS_NODE = 'nats'
/** The service graph's connection type for a producer/consumer pair. */
export const MESSAGING_CONNECTION = 'messaging_system'

export interface PodMetrics {
  rps: PromQueryResult
  errors: PromQueryResult
  p50: PromQueryResult
  p99: PromQueryResult
}

export interface EdgeMetrics {
  rps: PromQueryResult
  failed: PromQueryResult
  p50: PromQueryResult
  p99: PromQueryResult
  /**
   * Producer-side latency of `messaging_system` pairs (the client span, i.e.
   * the publish). Optional: without it the publish leg into NATS has no
   * latency, rather than borrowing the consumer's.
   */
  clientP50?: PromQueryResult
  clientP99?: PromQueryResult
}

export interface AssembleInput {
  at?: Date
  cluster: ClusterSnapshot
  /** Null when Prometheus is not configured or every query failed. */
  pods: PodMetrics | null
  edges: EdgeMetrics | null
  /** Detail to report for Prometheus when `pods`/`edges` are null. */
  prometheusDetail?: string | null
}

/** Seconds to milliseconds, preserving null. */
function toMs(seconds: Measure): Measure {
  if (seconds === null) return null
  // histogram_quantile on an empty histogram yields NaN, which parseValue
  // already turned into null; a negative value is nonsense, so drop it too.
  if (seconds < 0) return null
  return seconds * 1000
}

/** Sum of non-null values, or null when nothing was measured at all. */
export function sumMeasures(values: Measure[]): Measure {
  const present = values.filter((v): v is number => v !== null)
  if (present.length === 0) return null
  return present.reduce((a, b) => a + b, 0)
}

/** Largest non-null value, or null. */
export function maxMeasure(values: Measure[]): Measure {
  const present = values.filter((v): v is number => v !== null)
  if (present.length === 0) return null
  return Math.max(...present)
}

/** Request-weighted mean, falling back to a plain mean when rates are absent. */
export function weightedMean(pairs: { value: Measure; weight: Measure }[]): Measure {
  const usable = pairs.filter((p): p is { value: number; weight: Measure } => p.value !== null)
  if (usable.length === 0) return null
  const weights = usable.map((p) => (p.weight === null || p.weight <= 0 ? 0 : p.weight))
  const totalWeight = weights.reduce((a, b) => a + b, 0)
  if (totalWeight <= 0) {
    return usable.reduce((a, p) => a + p.value, 0) / usable.length
  }
  let acc = 0
  usable.forEach((p, i) => {
    acc += p.value * (weights[i] as number)
  })
  return acc / totalWeight
}

function ratio(numerator: Measure, denominator: Measure): Measure {
  if (numerator === null || denominator === null) return null
  if (denominator <= 0) return numerator > 0 ? 1 : null
  return numerator / denominator
}

export function assembleTopology(input: AssembleInput): TopologySnapshot {
  const at = (input.at ?? new Date()).toISOString()

  const podsByApp = new Map<string, ObservedPodLike[]>()
  for (const pod of input.cluster.pods) {
    const list = podsByApp.get(pod.app)
    if (list === undefined) podsByApp.set(pod.app, [pod])
    else list.push(pod)
  }
  const workloadsByApp = new Map(input.cluster.workloads.map((w) => [w.app, w]))

  const podRps = input.pods === null ? new Map<string, Measure>() : byLabel(input.pods.rps, 'pod')
  const podErrors =
    input.pods === null ? new Map<string, Measure>() : byLabel(input.pods.errors, 'pod')
  const podP50 = input.pods === null ? new Map<string, Measure>() : byLabel(input.pods.p50, 'pod')
  const podP99 = input.pods === null ? new Map<string, Measure>() : byLabel(input.pods.p99, 'pod')

  const { edges, unmapped } = assembleEdges(input.edges)

  const nodes: NodeView[] = REGISTRY.map((registration) => {
    const observed = podsByApp.get(registration.name) ?? []
    const pods: PodView[] = observed
      .slice()
      .sort((a, b) => a.name.localeCompare(b.name))
      .map((pod) => {
        const rps = podRps.get(pod.name) ?? null
        const errors = podErrors.get(pod.name) ?? null
        return {
          name: pod.name,
          phase: pod.phase,
          ready: pod.ready,
          restarts: pod.restarts,
          rps,
          p50Ms: toMs(podP50.get(pod.name) ?? null),
          p99Ms: toMs(podP99.get(pod.name) ?? null),
          errorRatio: ratio(errors, rps),
        }
      })

    const workload = workloadsByApp.get(registration.name)
    // A backing store is measured by the traffic into it; a service by its pods.
    const stats =
      registration.kind === 'store'
        ? storeStats(edges.filter((e) => e.target === registration.name))
        : podStats(pods)

    return {
      ...registration,
      pods,
      desiredReplicas: workload?.desiredReplicas ?? null,
      readyReplicas: workload?.readyReplicas ?? null,
      ...stats,
      health: classifyHealth(stats.p99Ms, stats.errorRatio, stats.rps),
    }
  })

  const prometheusOk =
    input.pods !== null &&
    input.edges !== null &&
    (input.pods.rps.ok || input.edges.rps.ok)

  const prometheusDetail = prometheusOk
    ? null
    : (input.prometheusDetail ??
      input.pods?.rps.detail ??
      input.edges?.rps.detail ??
      'Prometheus reported nothing')

  const sources: TopologySnapshot['sources'] = {
    kubernetes: statusOf(input.cluster.ok, input.cluster.detail),
    prometheus: statusOf(prometheusOk, prometheusDetail),
  }

  return { at, nodes, edges, unmapped, sources }
}

function statusOf(ok: boolean, detail: string | null): SourceStatus {
  return { ok, detail: ok ? null : detail }
}

interface ObservedPodLike {
  name: string
  app: string
  phase: string
  ready: boolean
  restarts: number
}

/** A node's rate, latency and error ratio. */
interface NodeStats {
  rps: Measure
  p50Ms: Measure
  p99Ms: Measure
  errorRatio: Measure
}

/** A service: the sum of its pods' rates and the worst pod p99, from its own HTTP server metrics. */
function podStats(pods: PodView[]): NodeStats {
  const rps = sumMeasures(pods.map((p) => p.rps))
  const errors = sumMeasures(
    pods.map((p) => (p.errorRatio === null || p.rps === null ? null : p.errorRatio * p.rps)),
  )
  return {
    rps,
    p50Ms: weightedMean(pods.map((p) => ({ value: p.p50Ms, weight: p.rps }))),
    p99Ms: maxMeasure(pods.map((p) => p.p99Ms)),
    errorRatio: ratio(errors, rps),
  }
}

/**
 * A backing store: nothing instruments it and it serves no HTTP metrics, so it
 * is measured by the edges into it. Rate is their summed rate, p99 the worst
 * edge p99, p50 the rate-weighted mean of the edge p50s, and the error ratio
 * their summed failure rate over their summed rate. No inbound edge, or none
 * with a value, leaves the store at null ("no data"), never zero.
 */
export function storeStats(inbound: EdgeView[]): NodeStats {
  const rps = sumMeasures(inbound.map((e) => e.rps))
  const failed = sumMeasures(
    inbound.map((e) => (e.errorRatio === null || e.rps === null ? null : e.errorRatio * e.rps)),
  )
  return {
    rps,
    p50Ms: weightedMean(inbound.map((e) => ({ value: e.p50Ms, weight: e.rps }))),
    p99Ms: maxMeasure(inbound.map((e) => e.p99Ms)),
    errorRatio: ratio(failed, rps),
  }
}

/** An edge being built up from one or more service-graph series. */
interface EdgeAccumulator {
  source: string
  target: string
  via: EdgeView['via']
  rps: Measure
  /** Failed requests per second, kept apart so merged edges add failures, not ratios. */
  failed: Measure
  p50Ms: Measure
  p99Ms: Measure
}

/**
 * Add one leg to the edge set. Several series can land on the same registry
 * pair (two aliases of one store, or several producer/consumer pairs sharing a
 * leg through NATS): add the rates and failures, keep the worse latency.
 */
function addLeg(edges: Map<string, EdgeAccumulator>, leg: EdgeAccumulator): void {
  const key = pairKey(leg.source, leg.target)
  const existing = edges.get(key)
  if (existing === undefined) {
    edges.set(key, { ...leg })
    return
  }
  existing.rps = sumMeasures([existing.rps, leg.rps])
  existing.failed = sumMeasures([existing.failed, leg.failed])
  existing.p50Ms = maxMeasure([existing.p50Ms, leg.p50Ms])
  existing.p99Ms = maxMeasure([existing.p99Ms, leg.p99Ms])
}

export function assembleEdges(metrics: EdgeMetrics | null): {
  edges: EdgeView[]
  unmapped: UnmappedEdge[]
} {
  if (metrics === null) return { edges: [], unmapped: [] }

  const none = new Map<string, EdgeSample>()
  const rps = byEdge(metrics.rps)
  const failed = byEdge(metrics.failed)
  const p50 = byEdge(metrics.p50)
  const p99 = byEdge(metrics.p99)
  const clientP50 = metrics.clientP50 === undefined ? none : byEdge(metrics.clientP50)
  const clientP99 = metrics.clientP99 === undefined ? none : byEdge(metrics.clientP99)

  // Every series any of the queries mentioned: a pair with latency but no rate
  // is still a real observation.
  const keys = new Set<string>([
    ...rps.keys(),
    ...failed.keys(),
    ...p50.keys(),
    ...p99.keys(),
    ...clientP50.keys(),
    ...clientP99.keys(),
  ])

  const edges = new Map<string, EdgeAccumulator>()
  const unmapped = new Map<string, UnmappedEdge>()

  for (const key of [...keys].sort()) {
    const sample =
      rps.get(key) ?? failed.get(key) ?? p50.get(key) ?? p99.get(key) ?? clientP50.get(key) ?? clientP99.get(key)
    if (sample === undefined) continue
    const { client, server, connectionType } = sample
    if (IGNORED_PEERS.has(client.toLowerCase()) || IGNORED_PEERS.has(server.toLowerCase())) {
      continue
    }

    const edgeRps = rps.get(key)?.value ?? null
    const source = resolvePeer(client)
    const target = resolvePeer(server)
    if (source === null || target === null) {
      // One unmapped pair, however many connection types it was seen with.
      const uk = pairKey(client, server)
      const existing = unmapped.get(uk)
      unmapped.set(uk, {
        client,
        server,
        rps: existing === undefined ? edgeRps : sumMeasures([existing.rps, edgeRps]),
      })
      continue
    }

    const edgeFailed = failed.get(key)?.value ?? null
    const serverP50 = toMs(p50.get(key)?.value ?? null)
    const serverP99 = toMs(p99.get(key)?.value ?? null)

    if (connectionType === MESSAGING_CONNECTION && source !== BUS_NODE && target !== BUS_NODE) {
      // A producer span paired with its consumer span: the message went through
      // the bus. The publish leg carries the producer's latency, the delivery leg
      // the consumer's. The connector does not say which side failed, so a failed
      // pair counts against both legs.
      addLeg(edges, {
        source,
        target: BUS_NODE,
        via: 'messaging',
        rps: edgeRps,
        failed: edgeFailed,
        p50Ms: toMs(clientP50.get(key)?.value ?? null),
        p99Ms: toMs(clientP99.get(key)?.value ?? null),
      })
      addLeg(edges, {
        source: BUS_NODE,
        target,
        via: 'messaging',
        rps: edgeRps,
        failed: edgeFailed,
        p50Ms: serverP50,
        p99Ms: serverP99,
      })
      continue
    }

    if (source === target) continue

    const via: EdgeView['via'] = isStore(target)
      ? target === BUS_NODE
        ? 'messaging'
        : 'database'
      : 'http'

    addLeg(edges, {
      source,
      target,
      via,
      rps: edgeRps,
      failed: edgeFailed,
      p50Ms: serverP50,
      p99Ms: serverP99,
    })
  }

  return {
    edges: [...edges.values()]
      .map((e): EdgeView => {
        const errorRatio = ratio(e.failed, e.rps)
        return {
          source: e.source,
          target: e.target,
          via: e.via,
          rps: e.rps,
          errorRatio,
          p50Ms: e.p50Ms,
          p99Ms: e.p99Ms,
          health: classifyHealth(e.p99Ms, errorRatio, e.rps),
        }
      })
      .sort((a, b) => a.source.localeCompare(b.source) || a.target.localeCompare(b.target)),
    unmapped: [...unmapped.values()].sort(
      (a, b) => a.client.localeCompare(b.client) || a.server.localeCompare(b.server),
    ),
  }
}
