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
import { byLabel, byPair, pairKey, type PromQueryResult } from './prometheus.js'

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
    const nodeRps = sumMeasures(pods.map((p) => p.rps))
    const nodeErrors = sumMeasures(
      pods.map((p) => (p.errorRatio === null || p.rps === null ? null : p.errorRatio * p.rps)),
    )
    const nodeP99 = maxMeasure(pods.map((p) => p.p99Ms))
    const nodeP50 = weightedMean(pods.map((p) => ({ value: p.p50Ms, weight: p.rps })))
    const errorRatio = ratio(nodeErrors, nodeRps)

    return {
      ...registration,
      pods,
      desiredReplicas: workload?.desiredReplicas ?? null,
      readyReplicas: workload?.readyReplicas ?? null,
      rps: nodeRps,
      p50Ms: nodeP50,
      p99Ms: nodeP99,
      errorRatio,
      health: classifyHealth(nodeP99, errorRatio, nodeRps),
    }
  })

  const { edges, unmapped } = assembleEdges(input.edges)

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

export function assembleEdges(metrics: EdgeMetrics | null): {
  edges: EdgeView[]
  unmapped: UnmappedEdge[]
} {
  if (metrics === null) return { edges: [], unmapped: [] }

  const rps = byPair(metrics.rps, 'client', 'server')
  const failed = byPair(metrics.failed, 'client', 'server')
  const p50 = byPair(metrics.p50, 'client', 'server')
  const p99 = byPair(metrics.p99, 'client', 'server')

  // Every pair any of the four queries mentioned: a pair with latency but no
  // rate is still a real observation.
  const keys = new Set<string>([...rps.keys(), ...failed.keys(), ...p50.keys(), ...p99.keys()])

  const edges = new Map<string, EdgeView>()
  const unmapped: UnmappedEdge[] = []

  for (const key of [...keys].sort()) {
    const sample = rps.get(key) ?? failed.get(key) ?? p50.get(key) ?? p99.get(key)
    if (sample === undefined) continue
    const client = sample.a
    const server = sample.b
    if (IGNORED_PEERS.has(client.toLowerCase()) || IGNORED_PEERS.has(server.toLowerCase())) {
      continue
    }

    const source = resolvePeer(client)
    const target = resolvePeer(server)
    if (source === null || target === null) {
      unmapped.push({ client, server, rps: rps.get(key)?.value ?? null })
      continue
    }
    if (source === target) continue

    const edgeRps = rps.get(key)?.value ?? null
    const edgeFailed = failed.get(key)?.value ?? null
    const edgeP50 = toMs(p50.get(key)?.value ?? null)
    const edgeP99 = toMs(p99.get(key)?.value ?? null)
    const errorRatio = ratio(edgeFailed, edgeRps)

    const via: EdgeView['via'] = isStore(target)
      ? target === 'nats'
        ? 'messaging'
        : 'database'
      : 'http'

    const pk = pairKey(source, target)
    const existing = edges.get(pk)
    if (existing === undefined) {
      edges.set(pk, {
        source,
        target,
        via,
        rps: edgeRps,
        errorRatio,
        p50Ms: edgeP50,
        p99Ms: edgeP99,
        health: classifyHealth(edgeP99, errorRatio, edgeRps),
      })
    } else {
      // Two service-graph aliases collapsed onto the same registry pair: add
      // the rates, keep the worse latency.
      const mergedRps = sumMeasures([existing.rps, edgeRps])
      const mergedFailed = sumMeasures([
        existing.errorRatio === null || existing.rps === null
          ? null
          : existing.errorRatio * existing.rps,
        edgeFailed,
      ])
      const mergedP99 = maxMeasure([existing.p99Ms, edgeP99])
      const mergedP50 = maxMeasure([existing.p50Ms, edgeP50])
      const mergedRatio = ratio(mergedFailed, mergedRps)
      edges.set(pk, {
        ...existing,
        rps: mergedRps,
        errorRatio: mergedRatio,
        p50Ms: mergedP50,
        p99Ms: mergedP99,
        health: classifyHealth(mergedP99, mergedRatio, mergedRps),
      })
    }
  }

  return {
    edges: [...edges.values()].sort(
      (a, b) => a.source.localeCompare(b.source) || a.target.localeCompare(b.target),
    ),
    unmapped: unmapped.sort((a, b) => a.client.localeCompare(b.client) || a.server.localeCompare(b.server)),
  }
}
