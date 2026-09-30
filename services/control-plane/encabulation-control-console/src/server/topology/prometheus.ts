/**
 * The Prometheus reader.
 *
 * Every number on the Factory Floor that is not a Kubernetes fact comes from
 * here. Two rules:
 *
 *  1. A query that fails, returns no series, or returns NaN yields `null`. It
 *     never yields zero, because "nothing measured" and "measured zero" are
 *     different things and the console says which.
 *  2. Label values are escaped before interpolation. The namespace comes from
 *     our own environment, but escaping it costs nothing and keeps the habit.
 */

import type { Gateway } from '../gateway.js'
import type { Measure } from '../../shared/topology.js'

export interface PromSample {
  labels: Record<string, string>
  value: number | null
}

export interface PromQueryResult {
  ok: boolean
  detail: string | null
  samples: PromSample[]
}

interface PromVectorResponse {
  status?: string
  error?: string
  data?: {
    resultType?: string
    result?: { metric?: Record<string, string>; value?: [number, string] }[]
  }
}

export const EMPTY_RESULT: PromQueryResult = { ok: false, detail: 'not configured', samples: [] }

/** Escape a PromQL string literal. */
export function escapeLabelValue(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n')
}

export function parseValue(raw: unknown): Measure {
  if (typeof raw !== 'string') return null
  const n = Number.parseFloat(raw)
  if (!Number.isFinite(n)) return null
  return n
}

export function parseVector(body: unknown): PromQueryResult {
  const res = body as PromVectorResponse | null
  if (res === null || typeof res !== 'object') {
    return { ok: false, detail: 'prometheus returned a non-object response', samples: [] }
  }
  if (res.status !== 'success') {
    return {
      ok: false,
      detail: res.error ?? `prometheus returned status ${String(res.status)}`,
      samples: [],
    }
  }
  const rows = res.data?.result ?? []
  const samples: PromSample[] = rows.map((row) => ({
    labels: row.metric ?? {},
    value: parseValue(row.value?.[1]),
  }))
  return { ok: true, detail: null, samples }
}

export interface PrometheusReader {
  readonly configured: boolean
  /** Instant query. Never throws; failures come back as `ok: false`. */
  query(promql: string): Promise<PromQueryResult>
}

export function createPrometheusReader(opts: { gateway: Gateway }): PrometheusReader {
  const info = opts.gateway.info('prometheus')
  return {
    configured: info.configured,
    async query(promql: string): Promise<PromQueryResult> {
      if (!info.configured) {
        return { ok: false, detail: `${info.env} is not set`, samples: [] }
      }
      try {
        const res = await opts.gateway.call({
          target: 'prometheus',
          method: 'GET',
          path: `/api/v1/query?query=${encodeURIComponent(promql)}`,
          // The topology tick runs every three seconds; a retried query would
          // still be in flight when the next tick starts.
          retries: 0,
        })
        if (res.status >= 400) {
          return { ok: false, detail: `prometheus returned HTTP ${res.status}`, samples: [] }
        }
        return parseVector(res.body)
      } catch (err) {
        return {
          ok: false,
          detail: err instanceof Error ? err.message : String(err),
          samples: [],
        }
      }
    },
  }
}

/** Index a result by one label, keeping the last value for duplicate labels. */
export function byLabel(result: PromQueryResult, label: string): Map<string, Measure> {
  const out = new Map<string, Measure>()
  for (const sample of result.samples) {
    const key = sample.labels[label]
    if (key === undefined || key === '') continue
    out.set(key, sample.value)
  }
  return out
}

/** Index a result by two labels joined with a null byte. */
export function byPair(
  result: PromQueryResult,
  a: string,
  b: string,
): Map<string, { a: string; b: string; value: Measure }> {
  const out = new Map<string, { a: string; b: string; value: Measure }>()
  for (const sample of result.samples) {
    const av = sample.labels[a]
    const bv = sample.labels[b]
    if (av === undefined || bv === undefined || av === '' || bv === '') continue
    out.set(`${av}\u0000${bv}`, { a: av, b: bv, value: sample.value })
  }
  return out
}

export function pairKey(a: string, b: string): string {
  return `${a}\u0000${b}`
}

// ── The queries ───────────────────────────────────────────────────────────────
// Per-pod server stats come from `http_server_request_duration_seconds`, which
// every service in the system exposes with the same labels (design section 7).
// Edges come only from the OTel collector's service graph connector.

export interface QuerySet {
  podRps: string
  podErrors: string
  podP50: string
  podP99: string
  edgeRps: string
  edgeFailed: string
  edgeP50: string
  edgeP99: string
}

export function buildQueries(opts: { namespace: string | null; window?: string }): QuerySet {
  const w = opts.window ?? '1m'
  const ns =
    opts.namespace === null || opts.namespace === ''
      ? ''
      : `namespace="${escapeLabelValue(opts.namespace)}"`
  const sel = (extra?: string): string => {
    const parts = [ns, extra].filter((p): p is string => p !== undefined && p !== '')
    return parts.length === 0 ? '' : `{${parts.join(',')}}`
  }

  return {
    podRps: `sum by (pod) (rate(http_server_request_duration_seconds_count${sel()}[${w}]))`,
    podErrors: `sum by (pod) (rate(http_server_request_duration_seconds_count${sel('http_response_status_code=~"5.."')}[${w}]))`,
    podP50: `histogram_quantile(0.5, sum by (pod, le) (rate(http_server_request_duration_seconds_bucket${sel()}[${w}])))`,
    podP99: `histogram_quantile(0.99, sum by (pod, le) (rate(http_server_request_duration_seconds_bucket${sel()}[${w}])))`,
    edgeRps: `sum by (client, server) (rate(traces_service_graph_request_total[${w}]))`,
    edgeFailed: `sum by (client, server) (rate(traces_service_graph_request_failed_total[${w}]))`,
    edgeP50: `histogram_quantile(0.5, sum by (client, server, le) (rate(traces_service_graph_request_server_seconds_bucket[${w}])))`,
    edgeP99: `histogram_quantile(0.99, sum by (client, server, le) (rate(traces_service_graph_request_server_seconds_bucket[${w}])))`,
  }
}
