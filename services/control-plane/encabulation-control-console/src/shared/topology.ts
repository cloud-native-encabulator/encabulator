/**
 * The topology model, shared by the BFF (which assembles it) and the SPA
 * (which draws it).
 *
 * Honesty contract: every numeric field here is `number | null`. `null` means
 * "the upstream did not report this", and the UI must render it as "no data".
 * Nothing in this file is ever filled in with a guess.
 */

export type Plane = 'control' | 'processing' | 'data' | 'support'
export type NodeKind = 'service' | 'store'
export type Health = 'healthy' | 'degraded' | 'faulted' | 'nodata'

/** A measurement that may simply not exist. */
export type Measure = number | null

/** Static facts about a node. Comes from the registry, never from telemetry. */
export interface NodeRegistration {
  /** Kubernetes `app.kubernetes.io/name` and the service-graph identity. */
  name: string
  display: string
  plane: Plane
  kind: NodeKind
  /** Implementation language, or the image for a backing store. */
  language: string
  /** One line of genuine encabulator prose. */
  epigraph: string
  /** What it actually does, in plain English, for the tooltip. */
  plainly: string
  /** Deterministic layout slot: column and row in the panel grid. */
  lay: { col: number; row: number }
}

export interface PodView {
  name: string
  /** Pending | Running | Succeeded | Failed | Unknown, verbatim from the API. */
  phase: string
  ready: boolean
  restarts: number
  /** Per-pod request rate, requests per second over the last minute. */
  rps: Measure
  p50Ms: Measure
  p99Ms: Measure
  /** 0..1 */
  errorRatio: Measure
}

export interface NodeView extends NodeRegistration {
  /** Live pods from the Kubernetes API, empty when the API is unavailable. */
  pods: PodView[]
  /** Deployment spec/status replicas. Backing stores are StatefulSets: null. */
  desiredReplicas: Measure
  readyReplicas: Measure
  /** Sum of pod rates. Null when Prometheus reported nothing for this node. */
  rps: Measure
  /** Worst pod p99. */
  p99Ms: Measure
  p50Ms: Measure
  errorRatio: Measure
  health: Health
}

export interface EdgeView {
  /** Registry node names. Edges to unknown peers are not invented; see `unmapped`. */
  source: string
  target: string
  /** How the edge was observed. */
  via: 'http' | 'messaging' | 'database'
  rps: Measure
  errorRatio: Measure
  p50Ms: Measure
  p99Ms: Measure
  health: Health
}

/** An edge the service graph reported whose peers are not in the registry. */
export interface UnmappedEdge {
  client: string
  server: string
  rps: Measure
}

export interface SourceStatus {
  ok: boolean
  /** Human readable reason the source is unavailable, when it is. */
  detail: string | null
}

export interface TopologySnapshot {
  /** RFC 3339, when the BFF assembled this snapshot. */
  at: string
  nodes: NodeView[]
  edges: EdgeView[]
  unmapped: UnmappedEdge[]
  sources: {
    kubernetes: SourceStatus
    prometheus: SourceStatus
  }
}

/** Headline gauges on the Factory Floor. All measured, all nullable. */
export interface HeadlineStats {
  at: string
  /** Orders accepted per minute, from the datastore's 5 minute throughput. */
  ordersPerMin: Measure
  /** Units currently on the casing line, from the datastore's status counts. */
  unitsInFabrication: Measure
  /** Refabrication rate 0..1, from the detector's 5 minute fail rate. */
  refabricationRate: Measure
  /** Total parts in the cage and the shortest SKU, from the depot. */
  partsTotal: Measure
  partsLowestSku: string | null
  partsLowestStock: Measure
  /** Units delivered in the last 5 minutes, from the datastore. */
  unitsDelivered: Measure
  /** Sinusoidal repleneration mean, from the recorder's summary. */
  replenerationMean: Measure
  sources: Record<string, SourceStatus>
}

export interface FeedItem {
  /** Stable id so the SPA can dedupe; the upstream's own id. */
  id: string
  at: string
  kind: 'order' | 'incident' | 'shipment' | 'narration'
  headline: string
  detail: string | null
  /** Freeform, already-safe fields for the row (no HTML). */
  fields: Record<string, string>
}

export interface FeedBatch {
  at: string
  items: FeedItem[]
  sources: Record<string, SourceStatus>
}

/** Above this error ratio an edge is red no matter how fast it is. */
export const FAULTED_ERROR_RATIO = 0.05
/** Green requires errors below this. */
export const HEALTHY_ERROR_RATIO = 0.01

/**
 * Health colouring, per the design: green under 100ms p99 *and* under 1%
 * errors, yellow under 500ms, red otherwise, gray when there is no data.
 *
 * The design does not say what to do with a fast endpoint that is failing
 * half its requests, so we choose: at or above 5% errors the edge is red
 * regardless of latency. Everything else follows the latency bands.
 */
export function classifyHealth(p99Ms: Measure, errorRatio: Measure, rps: Measure): Health {
  const noSignal = p99Ms === null && errorRatio === null && (rps === null || rps === 0)
  if (noSignal) return 'nodata'
  if (errorRatio !== null && errorRatio >= FAULTED_ERROR_RATIO) return 'faulted'
  if (p99Ms === null) {
    // Errors reported without any latency histogram: say so rather than guess.
    return errorRatio !== null && errorRatio >= HEALTHY_ERROR_RATIO ? 'degraded' : 'nodata'
  }
  const errorsOk = errorRatio === null || errorRatio < HEALTHY_ERROR_RATIO
  if (p99Ms < 100 && errorsOk) return 'healthy'
  if (p99Ms < 500) return 'degraded'
  return 'faulted'
}
