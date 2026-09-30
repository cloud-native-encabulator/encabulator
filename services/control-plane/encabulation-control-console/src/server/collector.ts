/**
 * The live collector: the only thing in the console that polls upstreams.
 *
 * It starts when the first SSE client attaches and stops when the last one
 * leaves, so a console nobody is watching costs the factory nothing. Each tick
 * produces one payload that is fanned out to every client, rather than one poll
 * per client.
 *
 * It never throws into the timer. Every upstream failure becomes a `SourceStatus`
 * in the payload, which the SPA renders as "no data" beside the affected gauge.
 */

import type { FeedBatch, FeedItem, HeadlineStats, TopologySnapshot } from '../shared/topology.js'
import type { ConsoleConfig, UpstreamKey } from './config.js'
import { UpstreamNotConfigured, type Gateway } from './gateway.js'
import type { ConsoleMetrics, Logger } from './platform/index.js'
import { SseHub } from './hub.js'
import type { ClusterReader } from './topology/kubernetes.js'
import { assembleTopology, type EdgeMetrics, type PodMetrics } from './topology/assemble.js'
import { buildQueries, type PrometheusReader } from './topology/prometheus.js'
import {
  OK,
  buildHeadline,
  failed,
  readIncidentFeed,
  readOrderFeed,
  readRunNarration,
  readShipmentFeed,
  type RunNarration,
} from './upstreams.js'

export const EVENT_TOPOLOGY = 'topology'
export const EVENT_FEED = 'feed'
export const EVENT_HEADLINE = 'headline'
export const EVENT_SCENARIO = 'scenario'

/** How many ids to remember per feed kind so the same row is not resent. */
const SEEN_LIMIT = 400

/** Heartbeat interval. Comfortably under the 60s idle timeout of most proxies. */
export const HEARTBEAT_MS = 15_000

export interface CollectorDeps {
  config: ConsoleConfig
  logger: Logger
  metrics: ConsoleMetrics
  gateway: Gateway
  prometheus: PrometheusReader
  cluster: ClusterReader
  /** Injected in tests. */
  now?: () => Date
  setIntervalImpl?: typeof setInterval
  clearIntervalImpl?: typeof clearInterval
}

interface Read {
  body: unknown
  status: ReturnType<typeof failed> | typeof OK
}

export class Collector {
  readonly hub: SseHub
  private timers: ReturnType<typeof setInterval>[] = []
  private running = false
  private seen = new Map<string, Set<string>>()
  private lastRun: RunNarration | null = null

  constructor(private readonly deps: CollectorDeps) {
    this.hub = new SseHub({
      onFirstClient: () => this.start(),
      onLastClient: () => this.stop(),
      onCount: (n) => deps.metrics.setStreamClients(n),
      replayEvents: [EVENT_TOPOLOGY, EVENT_HEADLINE, EVENT_SCENARIO, EVENT_FEED],
    })
  }

  get isRunning(): boolean {
    return this.running
  }

  /** Start polling. Idempotent; called by the hub on the first client. */
  start(): void {
    if (this.running) return
    this.running = true
    const setIntervalFn = this.deps.setIntervalImpl ?? setInterval
    this.deps.logger.info(
      { topologyIntervalMs: this.deps.config.topologyIntervalMs },
      'a client attached: starting upstream polling',
    )

    // Kick both ticks immediately so the first client is not left staring at an
    // empty panel for three seconds.
    void this.tickTopology()
    void this.tickFeed()

    this.timers.push(
      setIntervalFn(() => void this.tickTopology(), this.deps.config.topologyIntervalMs),
      setIntervalFn(() => void this.tickFeed(), this.deps.config.feedIntervalMs),
      setIntervalFn(() => this.hub.heartbeat(), HEARTBEAT_MS),
    )
    for (const timer of this.timers) {
      // Never hold the process open on account of the poll loop.
      const maybe = timer as unknown as { unref?: () => void }
      if (typeof maybe.unref === 'function') maybe.unref()
    }
  }

  /** Stop polling. Idempotent; called by the hub when the last client leaves. */
  stop(): void {
    if (!this.running) return
    this.running = false
    const clearIntervalFn = this.deps.clearIntervalImpl ?? clearInterval
    for (const timer of this.timers) clearIntervalFn(timer)
    this.timers = []
    this.deps.logger.info('no clients remain: upstream polling stopped')
  }

  close(): void {
    this.stop()
    this.hub.closeAll()
  }

  private at(): Date {
    return this.deps.now?.() ?? new Date()
  }

  // ── topology ───────────────────────────────────────────────────────────────

  async buildTopology(): Promise<TopologySnapshot> {
    const { config, prometheus, cluster } = this.deps
    const queries = buildQueries({ namespace: config.podNamespace })

    const [
      clusterSnapshot,
      podRps,
      podErrors,
      podP50,
      podP99,
      edgeRps,
      edgeFailed,
      edgeP50,
      edgeP99,
    ] = await Promise.all([
      cluster.read(),
      prometheus.query(queries.podRps),
      prometheus.query(queries.podErrors),
      prometheus.query(queries.podP50),
      prometheus.query(queries.podP99),
      prometheus.query(queries.edgeRps),
      prometheus.query(queries.edgeFailed),
      prometheus.query(queries.edgeP50),
      prometheus.query(queries.edgeP99),
    ])

    const pods: PodMetrics = { rps: podRps, errors: podErrors, p50: podP50, p99: podP99 }
    const edges: EdgeMetrics = { rps: edgeRps, failed: edgeFailed, p50: edgeP50, p99: edgeP99 }

    this.deps.metrics.countUpstreamPoll('prometheus', podRps.ok ? 'ok' : 'error')
    this.deps.metrics.countUpstreamPoll('kubernetes', clusterSnapshot.ok ? 'ok' : 'error')
    this.deps.metrics.countSnapshot(
      podRps.ok ? 'ok' : 'error',
      clusterSnapshot.ok ? 'ok' : 'error',
    )

    return assembleTopology({
      at: this.at(),
      cluster: clusterSnapshot,
      pods,
      edges,
      prometheusDetail: prometheus.configured ? null : 'PROMETHEUS_URL is not set',
    })
  }

  async tickTopology(): Promise<void> {
    try {
      const snapshot = await this.buildTopology()
      this.hub.broadcast(EVENT_TOPOLOGY, snapshot)
    } catch (err) {
      this.deps.logger.error(
        { detail: err instanceof Error ? err.message : String(err) },
        'topology assembly failed',
      )
    }
  }

  // ── feeds and headline gauges ──────────────────────────────────────────────

  private async get(key: UpstreamKey, path: string): Promise<Read> {
    const info = this.deps.gateway.info(key)
    try {
      const res = await this.deps.gateway.call({
        target: key,
        method: 'GET',
        path,
        // The polling loops must not retry: a slow upstream would pile up.
        retries: 0,
        headers: {
          // Reading as the console's own service identity, viewer only. The
          // datastore's visibility rule treats a viewer as staff, which is what
          // makes the works-wide feeds possible.
          'x-encabulator-subject': this.deps.config.serviceName,
          'x-encabulator-roles': 'viewer',
        },
      })
      this.deps.metrics.countUpstreamPoll(key, res.status < 400 ? 'ok' : 'error')
      if (res.status >= 400) {
        return { body: null, status: failed(`${info.service} returned HTTP ${res.status}`) }
      }
      return { body: res.body, status: OK }
    } catch (err) {
      this.deps.metrics.countUpstreamPoll(key, 'error')
      if (err instanceof UpstreamNotConfigured) {
        return { body: null, status: failed(`${info.env} is not set`) }
      }
      return { body: null, status: failed(err instanceof Error ? err.message : String(err)) }
    }
  }

  async buildHeadlineStats(): Promise<HeadlineStats> {
    const [datastore, detector, depot, recorder] = await Promise.all([
      this.get('datastore', '/v1/stats'),
      this.get('detector', '/v1/stats'),
      this.get('depot', '/v1/parts'),
      this.get('recorder', '/v1/summary?window=5m'),
    ])
    return buildHeadline({ at: this.at(), datastore, detector, depot, recorder })
  }

  async buildFeed(): Promise<{ batch: FeedBatch; run: RunNarration | null }> {
    const [orders, incidents, shipments, runs] = await Promise.all([
      this.get('datastore', '/v1/orders?limit=25'),
      this.get('detector', '/v1/incidents?limit=25'),
      this.get('dispatch', '/v1/shipments?limit=25'),
      this.get('orchestrator', '/v1/runs?limit=1'),
    ])

    const items: FeedItem[] = [
      ...this.fresh('order', orders.status.ok ? readOrderFeed(orders.body) : []),
      ...this.fresh('incident', incidents.status.ok ? readIncidentFeed(incidents.body) : []),
      ...this.fresh('shipment', shipments.status.ok ? readShipmentFeed(shipments.body) : []),
    ]

    let run: RunNarration | null = null
    let runStatus = runs.status
    if (runs.status.ok) {
      const runId = firstRunId(runs.body)
      if (runId !== null) {
        const detail = await this.get('orchestrator', `/v1/runs/${encodeURIComponent(runId)}`)
        runStatus = detail.status
        if (detail.status.ok) {
          run = readRunNarration(detail.body)
          items.push(...this.fresh('narration', run.items))
        }
      }
    }

    items.sort((a, b) => a.at.localeCompare(b.at))

    return {
      batch: {
        at: this.at().toISOString(),
        items,
        sources: {
          datastore: orders.status,
          detector: incidents.status,
          dispatch: shipments.status,
          orchestrator: runStatus,
        },
      },
      run,
    }
  }

  async tickFeed(): Promise<void> {
    try {
      const [headline, feed] = await Promise.all([this.buildHeadlineStats(), this.buildFeed()])
      this.hub.broadcast(EVENT_HEADLINE, headline)
      if (feed.batch.items.length > 0 || this.hub.cached(EVENT_FEED) === null) {
        this.hub.broadcast(EVENT_FEED, feed.batch)
      }
      if (feed.run !== null || this.lastRun !== null) {
        this.lastRun = feed.run
        this.hub.broadcast(EVENT_SCENARIO, feed.run)
      }
    } catch (err) {
      this.deps.logger.error(
        { detail: err instanceof Error ? err.message : String(err) },
        'feed poll failed',
      )
    }
  }

  /** Drop items already sent, so the SPA's ticker only shows what is new. */
  private fresh(kind: string, items: FeedItem[]): FeedItem[] {
    let seen = this.seen.get(kind)
    if (seen === undefined) {
      seen = new Set<string>()
      this.seen.set(kind, seen)
    }
    const out: FeedItem[] = []
    for (const item of items) {
      if (seen.has(item.id)) continue
      seen.add(item.id)
      out.push(item)
    }
    if (seen.size > SEEN_LIMIT) {
      // Trim the oldest half; insertion order is preserved by Set.
      const keep = [...seen].slice(-Math.floor(SEEN_LIMIT / 2))
      this.seen.set(kind, new Set(keep))
    }
    return out
  }
}

/** The id of the most recent run, from either response shape. */
export function firstRunId(body: unknown): string | null {
  const list = Array.isArray(body)
    ? body
    : Array.isArray((body as { runs?: unknown[] } | null)?.runs)
      ? ((body as { runs: unknown[] }).runs)
      : []
  const first = list[0]
  if (first === null || typeof first !== 'object') return null
  const id = (first as { id?: unknown }).id
  return typeof id === 'string' && id !== '' ? id : null
}
