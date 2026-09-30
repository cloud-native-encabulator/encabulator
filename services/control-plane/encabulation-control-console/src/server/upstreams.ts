/**
 * Tolerant readers for the upstream APIs the console summarises.
 *
 * The design fixes most response shapes but leaves a few summary endpoints
 * described in prose ("counts by status and throughput over the last five
 * minutes"). Rather than guess a single spelling and silently render zero when
 * it is wrong, each reader below tries the plausible spellings and returns
 * `null` when it finds none. `null` reaches the screen as "no data", which is
 * the honest answer to "the upstream did not tell us".
 */

import type { FeedItem, HeadlineStats, Measure, SourceStatus } from '../shared/topology.js'

type Json = Record<string, unknown>

export function asRecord(value: unknown): Json | null {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Json
}

export function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

/** First finite number found at any of `paths`, else null. Paths are dotted. */
export function pickNumber(root: unknown, paths: string[]): Measure {
  for (const path of paths) {
    let cursor: unknown = root
    for (const part of path.split('.')) {
      const record = asRecord(cursor)
      if (record === null) {
        cursor = undefined
        break
      }
      cursor = record[part]
    }
    if (typeof cursor === 'number' && Number.isFinite(cursor)) return cursor
    if (typeof cursor === 'string') {
      const parsed = Number.parseFloat(cursor)
      if (Number.isFinite(parsed)) return parsed
    }
  }
  return null
}

export function pickString(root: unknown, paths: string[]): string | null {
  for (const path of paths) {
    let cursor: unknown = root
    for (const part of path.split('.')) {
      const record = asRecord(cursor)
      if (record === null) {
        cursor = undefined
        break
      }
      cursor = record[part]
    }
    if (typeof cursor === 'string' && cursor !== '') return cursor
  }
  return null
}

export const OK: SourceStatus = { ok: true, detail: null }
export function failed(detail: string): SourceStatus {
  return { ok: false, detail }
}

// ── Headline gauges ───────────────────────────────────────────────────────────

/**
 * `GET hydrocoptic-datastore /v1/stats`: "counts by status and throughput
 * (orders accepted, units delivered) over the last 5 minutes".
 */
export function readDatastoreStats(body: unknown): {
  ordersPerMin: Measure
  unitsInFabrication: Measure
  unitsDelivered: Measure
} {
  const acceptedIn5m = pickNumber(body, [
    'throughput.ordersAccepted',
    'throughput.ordersAccepted5m',
    'throughput.orders_accepted',
    'ordersAccepted',
    'orders_accepted',
  ])
  // Prefer an explicit per-minute figure if the datastore offers one.
  const perMinute = pickNumber(body, [
    'throughput.ordersPerMinute',
    'throughput.ordersAcceptedPerMinute',
    'ordersPerMinute',
  ])
  const ordersPerMin = perMinute ?? (acceptedIn5m === null ? null : acceptedIn5m / 5)

  const fabricating = pickNumber(body, [
    'byStatus.fabricating',
    'statuses.fabricating',
    'counts.fabricating',
    'fabricating',
  ])
  const refabricating = pickNumber(body, [
    'byStatus.refabricating',
    'statuses.refabricating',
    'counts.refabricating',
    'refabricating',
  ])
  const unitsInFabrication =
    fabricating === null && refabricating === null ? null : (fabricating ?? 0) + (refabricating ?? 0)

  const unitsDelivered = pickNumber(body, [
    'throughput.unitsDelivered',
    'throughput.unitsDelivered5m',
    'throughput.units_delivered',
    'unitsDelivered',
    'units_delivered',
  ])

  return { ordersPerMin, unitsInFabrication, unitsDelivered }
}

/** `GET side-fumbling-detector /v1/stats`: `{inspected, passed, failed, failRate5m}`. */
export function readDetectorStats(body: unknown): { refabricationRate: Measure } {
  const rate = pickNumber(body, ['failRate5m', 'fail_rate_5m', 'failRate'])
  if (rate !== null) {
    // Some services report a percentage; anything above 1 must be one.
    return { refabricationRate: rate > 1 ? rate / 100 : rate }
  }
  const failedCount = pickNumber(body, ['failed'])
  const inspected = pickNumber(body, ['inspected'])
  if (failedCount !== null && inspected !== null && inspected > 0) {
    return { refabricationRate: failedCount / inspected }
  }
  return { refabricationRate: null }
}

export interface PartRow {
  sku: string
  name: string
  stock: Measure
  reserved: Measure
  perUnit: Measure
}

/** `GET prefabulated-amulite-depot /v1/parts`: `[{sku, name, stock, reserved, perUnit}]`. */
export function readParts(body: unknown): PartRow[] {
  const rows = Array.isArray(body) ? body : asArray(asRecord(body)?.['parts'])
  return rows
    .map((row) => {
      const sku = pickString(row, ['sku'])
      if (sku === null) return null
      return {
        sku,
        name: pickString(row, ['name']) ?? sku,
        stock: pickNumber(row, ['stock']),
        reserved: pickNumber(row, ['reserved']),
        perUnit: pickNumber(row, ['perUnit', 'per_unit']),
      }
    })
    .filter((row): row is PartRow => row !== null)
}

/** `GET sinusoidal-recorder /v1/summary?window=5m`: per-metric count, mean, p95. */
export function readReplenerationMean(body: unknown): Measure {
  const direct = pickNumber(body, [
    'metrics.repleneration.mean',
    'repleneration.mean',
    'summary.repleneration.mean',
  ])
  if (direct !== null) return direct
  // Or an array of `{metric, mean}` rows.
  const rows = Array.isArray(body) ? body : asArray(asRecord(body)?.['metrics'])
  for (const row of rows) {
    if (pickString(row, ['metric']) === 'repleneration') return pickNumber(row, ['mean'])
  }
  return null
}

export function buildHeadline(parts: {
  at: Date
  datastore: { body: unknown; status: SourceStatus }
  detector: { body: unknown; status: SourceStatus }
  depot: { body: unknown; status: SourceStatus }
  recorder: { body: unknown; status: SourceStatus }
}): HeadlineStats {
  const ds = parts.datastore.status.ok
    ? readDatastoreStats(parts.datastore.body)
    : { ordersPerMin: null, unitsInFabrication: null, unitsDelivered: null }
  const det = parts.detector.status.ok
    ? readDetectorStats(parts.detector.body)
    : { refabricationRate: null }
  const partRows = parts.depot.status.ok ? readParts(parts.depot.body) : []
  const stocks = partRows.filter((p): p is PartRow & { stock: number } => p.stock !== null)
  const lowest = stocks.reduce<(PartRow & { stock: number }) | null>(
    (min, row) => (min === null || row.stock < min.stock ? row : min),
    null,
  )

  return {
    at: parts.at.toISOString(),
    ordersPerMin: ds.ordersPerMin,
    unitsInFabrication: ds.unitsInFabrication,
    unitsDelivered: ds.unitsDelivered,
    refabricationRate: det.refabricationRate,
    partsTotal: stocks.length === 0 ? null : stocks.reduce((a, p) => a + p.stock, 0),
    partsLowestSku: lowest?.sku ?? null,
    partsLowestStock: lowest?.stock ?? null,
    replenerationMean: parts.recorder.status.ok
      ? readReplenerationMean(parts.recorder.body)
      : null,
    sources: {
      datastore: parts.datastore.status,
      detector: parts.detector.status,
      depot: parts.depot.status,
      recorder: parts.recorder.status,
    },
  }
}

// ── Live feeds ────────────────────────────────────────────────────────────────

function rowsOf(body: unknown, key: string): unknown[] {
  if (Array.isArray(body)) return body
  const record = asRecord(body)
  if (record === null) return []
  const named = record[key]
  if (Array.isArray(named)) return named
  const items = record['items']
  return Array.isArray(items) ? items : []
}

export function readOrderFeed(body: unknown): FeedItem[] {
  return rowsOf(body, 'orders')
    .map((row): FeedItem | null => {
      const id = pickString(row, ['id'])
      if (id === null) return null
      const status = pickString(row, ['status']) ?? 'unknown'
      const model = pickString(row, ['model']) ?? 'unknown'
      const quantity = pickNumber(row, ['quantity'])
      const stage = pickString(row, ['stage'])
      const fields: Record<string, string> = {
        order: id,
        customer: pickString(row, ['customer']) ?? 'unknown',
        model,
        quantity: quantity === null ? 'no data' : String(quantity),
        status,
      }
      if (stage !== null) fields['stage'] = stage
      const total = pickNumber(row, ['quote.total', 'total'])
      if (total !== null) fields['total'] = total.toFixed(2)
      return {
        id: `order:${id}:${status}:${stage ?? ''}`,
        at: pickString(row, ['updatedAt', 'createdAt']) ?? new Date().toISOString(),
        kind: 'order' as const,
        headline: `${model} ×${quantity ?? '?'} — ${status}`,
        detail: stage === null ? null : `stage: ${stage}`,
        fields,
      }
    })
    .filter((item): item is FeedItem => item !== null)
}

export function readIncidentFeed(body: unknown): FeedItem[] {
  return rowsOf(body, 'incidents')
    .map((row): FeedItem | null => {
      const id = pickString(row, ['id'])
      if (id === null) return null
      const serial = pickString(row, ['serial']) ?? 'unknown'
      const metric = pickString(row, ['metric']) ?? 'unknown'
      const stage = pickString(row, ['stage']) ?? 'unknown'
      const score = pickNumber(row, ['score'])
      return {
        id: `incident:${id}`,
        at: pickString(row, ['time', 'at']) ?? new Date().toISOString(),
        kind: 'incident' as const,
        headline: `Side fumbling detected on ${serial}`,
        detail: pickString(row, ['reason']),
        fields: {
          serial,
          stage,
          metric,
          score: score === null ? 'no data' : score.toFixed(2),
          order: pickString(row, ['orderId']) ?? 'unknown',
          strict: String(asRecord(row)?.['strict'] === true),
        },
      }
    })
    .filter((item): item is FeedItem => item !== null)
}

export function readShipmentFeed(body: unknown): FeedItem[] {
  return rowsOf(body, 'shipments')
    .map((row): FeedItem | null => {
      const tracking = pickString(row, ['trackingNumber', 'tracking_number'])
      const orderId = pickString(row, ['orderId', 'order_id'])
      const id = pickString(row, ['id']) ?? tracking ?? orderId
      if (id === null) return null
      const status = pickString(row, ['status']) ?? 'dispatched'
      const carrier = pickString(row, ['carrier']) ?? 'unknown'
      return {
        id: `shipment:${id}:${status}`,
        at: pickString(row, ['updatedAt', 'dispatchedAt', 'time']) ?? new Date().toISOString(),
        kind: 'shipment' as const,
        headline: `${carrier} — ${status}`,
        detail: tracking === null ? null : `tracking ${tracking}`,
        fields: {
          order: orderId ?? 'unknown',
          carrier,
          tracking: tracking ?? 'no data',
          status,
        },
      }
    })
    .filter((item): item is FeedItem => item !== null)
}

export interface RunNarration {
  runId: string | null
  scenarioId: string | null
  status: string | null
  currentStep: string | null
  stepIndex: Measure
  stepCount: Measure
  items: FeedItem[]
}

/** `GET demo-scenario-orchestrator /v1/runs/{id}`: status, current step, log. */
export function readRunNarration(body: unknown): RunNarration {
  const runId = pickString(body, ['id', 'runId'])
  const log = rowsOf(asRecord(body)?.['log'] ?? [], 'log')
  const items = log
    .map((row, index): FeedItem | null => {
      const text = pickString(row, ['narration', 'say', 'message', 'text'])
      if (text === null) return null
      const at = pickString(row, ['time', 'at']) ?? new Date().toISOString()
      return {
        id: `narration:${runId ?? 'run'}:${index}:${at}`,
        at,
        kind: 'narration' as const,
        headline: text,
        detail: pickString(row, ['step', 'stepId']),
        fields: {
          run: runId ?? 'unknown',
          step: pickString(row, ['step', 'stepId']) ?? String(index + 1),
        },
      }
    })
    .filter((item): item is FeedItem => item !== null)

  return {
    runId,
    scenarioId: pickString(body, ['scenarioId', 'scenario', 'scenario_id']),
    status: pickString(body, ['status', 'state']),
    currentStep: pickString(body, ['currentStep', 'current_step']),
    stepIndex: pickNumber(body, ['stepIndex', 'currentStepIndex']),
    stepCount: pickNumber(body, ['stepCount', 'steps']),
    items,
  }
}
