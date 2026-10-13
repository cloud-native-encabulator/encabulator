/**
 * The tolerant upstream readers, and the outbound HTTP policy.
 *
 * The readers exist because the design describes a few summary endpoints in
 * prose. The rule they enforce is the important part: an unrecognised shape
 * yields `null`, which becomes "no data" on screen. It never yields zero.
 */

import { describe, expect, it, vi } from 'vitest'
import {
  buildHeadline,
  pickNumber,
  readDatastoreStats,
  readDetectorStats,
  readIncidentFeed,
  readOrderFeed,
  readParts,
  readReplenerationMean,
  readRunNarration,
  readShipmentFeed,
  OK,
  failed,
} from '../upstreams.js'
import { UpstreamDown, UpstreamNotConfigured, createGateway } from '../gateway.js'
import { SILENT_LOGGER, testConfig } from './helpers.js'

describe('pickNumber', () => {
  it('finds the first finite number among the candidate paths', () => {
    expect(pickNumber({ a: { b: 3 } }, ['x.y', 'a.b'])).toBe(3)
    expect(pickNumber({ a: '4.5' }, ['a'])).toBe(4.5)
  })

  it('returns null rather than zero for a missing or unusable value', () => {
    expect(pickNumber({}, ['a.b'])).toBeNull()
    expect(pickNumber({ a: null }, ['a'])).toBeNull()
    expect(pickNumber({ a: 'many' }, ['a'])).toBeNull()
    expect(pickNumber({ a: Number.NaN }, ['a'])).toBeNull()
  })
})

describe('the ledger summary', () => {
  it('turns a five-minute count into a per-minute rate', () => {
    const stats = readDatastoreStats({ throughput: { ordersAccepted: 30, unitsDelivered: 12 } })
    expect(stats.ordersPerMin).toBe(6)
    expect(stats.unitsDelivered).toBe(12)
  })

  it('prefers an explicit per-minute figure when the ledger offers one', () => {
    expect(readDatastoreStats({ throughput: { ordersPerMinute: 9, ordersAccepted: 30 } }).ordersPerMin).toBe(9)
  })

  it('counts fabricating and refabricating orders together as work in progress', () => {
    expect(readDatastoreStats({ byStatus: { fabricating: 3, refabricating: 2 } }).unitsInFabrication).toBe(5)
  })

  it('reports null for a shape it does not recognise', () => {
    const stats = readDatastoreStats({ something: 'else' })
    expect(stats.ordersPerMin).toBeNull()
    expect(stats.unitsInFabrication).toBeNull()
    expect(stats.unitsDelivered).toBeNull()
  })
})

describe('the inspection summary', () => {
  it('reads failRate5m as a ratio, and normalises a percentage', () => {
    expect(readDetectorStats({ failRate5m: 0.07 }).refabricationRate).toBeCloseTo(0.07)
    expect(readDetectorStats({ failRate5m: 7 }).refabricationRate).toBeCloseTo(0.07)
  })

  it('derives the ratio from counts when no rate is given', () => {
    expect(readDetectorStats({ inspected: 200, failed: 8 }).refabricationRate).toBeCloseTo(0.04)
  })

  it('does not divide by zero', () => {
    expect(readDetectorStats({ inspected: 0, failed: 0 }).refabricationRate).toBeNull()
  })
})

describe('the parts cage', () => {
  it('reads a bare array or a wrapped one', () => {
    const rows = [{ sku: 'SPV-BRG', name: 'Spurving bearing', stock: 40, reserved: 2, perUnit: 2 }]
    expect(readParts(rows)).toHaveLength(1)
    expect(readParts({ parts: rows })).toHaveLength(1)
  })

  it('keeps a missing stock figure null', () => {
    expect(readParts([{ sku: 'PNT-FAN' }])[0]?.stock).toBeNull()
  })
})

describe('the telemetry summary', () => {
  it('finds the repleneration mean in either shape', () => {
    expect(readReplenerationMean({ metrics: { repleneration: { mean: 0.99 } } })).toBeCloseTo(0.99)
    expect(readReplenerationMean({ metrics: [{ metric: 'repleneration', mean: 1.04 }] })).toBeCloseTo(1.04)
    expect(readReplenerationMean({ metrics: [{ metric: 'grammeter_drift', mean: 3 }] })).toBeNull()
  })
})

describe('buildHeadline', () => {
  it('propagates every upstream failure as a source status and a null gauge', () => {
    const headline = buildHeadline({
      at: new Date('2026-09-30T12:00:00.000Z'),
      datastore: { body: null, status: failed('HYDROCOPTIC_DATASTORE_URL is not set') },
      detector: { body: null, status: failed('timeout') },
      depot: { body: null, status: failed('timeout') },
      recorder: { body: null, status: failed('timeout') },
    })
    expect(headline.ordersPerMin).toBeNull()
    expect(headline.refabricationRate).toBeNull()
    expect(headline.partsTotal).toBeNull()
    expect(headline.partsLowestSku).toBeNull()
    expect(headline.replenerationMean).toBeNull()
    expect(headline.sources.datastore?.detail).toContain('HYDROCOPTIC_DATASTORE_URL')
  })

  it('names the shortest SKU in the cage', () => {
    const headline = buildHeadline({
      at: new Date(),
      datastore: { body: {}, status: OK },
      detector: { body: {}, status: OK },
      depot: {
        body: [
          { sku: 'AMU-BASE', stock: 900 },
          { sku: 'HYD-MARZ', stock: 40 },
          { sku: 'SPV-BRG', stock: 500 },
        ],
        status: OK,
      },
      recorder: { body: {}, status: OK },
    })
    expect(headline.partsLowestSku).toBe('HYD-MARZ')
    expect(headline.partsLowestStock).toBe(40)
    expect(headline.partsTotal).toBe(1440)
  })
})

describe('the live feeds', () => {
  it('gives each order row an id that changes when its status changes', () => {
    const a = readOrderFeed({ orders: [{ id: 'ord_1', status: 'fabricating', model: 'turbo', quantity: 1 }] })
    const b = readOrderFeed({ orders: [{ id: 'ord_1', status: 'dispatched', model: 'turbo', quantity: 1 }] })
    expect(a[0]?.id).not.toBe(b[0]?.id)
  })

  it('renders a missing quantity as "no data" rather than as zero', () => {
    const rows = readOrderFeed({ orders: [{ id: 'ord_2', status: 'accepted', model: 'hyper' }] })
    expect(rows[0]?.fields['quantity']).toBe('no data')
  })

  it('reads an incident with its stage, metric and score', () => {
    const rows = readIncidentFeed({
      incidents: [
        {
          id: 'inc_1',
          orderId: 'ord_3',
          serial: 'ENC-turbo-abc-1234',
          stage: 'fit-marzlevanes',
          metric: 'repleneration',
          score: 4.82,
          reason: 'z-score above threshold',
          strict: true,
          time: '2026-09-30T12:00:00.000Z',
        },
      ],
    })
    expect(rows[0]).toMatchObject({ kind: 'incident', detail: 'z-score above threshold' })
    expect(rows[0]?.fields['score']).toBe('4.82')
    expect(rows[0]?.fields['strict']).toBe('true')
  })

  it('reads a shipment and reports a missing tracking number honestly', () => {
    const rows = readShipmentFeed({ shipments: [{ orderId: 'ord_4', carrier: 'Waneshaft Express' }] })
    expect(rows[0]?.fields['tracking']).toBe('no data')
  })

  it('reads scenario narration out of a run log', () => {
    const run = readRunNarration({
      id: 'run_1',
      scenarioId: 'marzlevane-shortage',
      status: 'running',
      currentStep: 'stop the foundry',
      stepIndex: 2,
      stepCount: 7,
      log: [
        { time: '2026-09-30T12:00:00.000Z', narration: 'The amulite foundry falls silent.' },
        { time: '2026-09-30T12:00:05.000Z', say: 'Marzlevanes run short; the depot begins refusing orders.' },
      ],
    })
    expect(run.scenarioId).toBe('marzlevane-shortage')
    expect(run.items).toHaveLength(2)
    expect(run.items[0]?.headline).toContain('foundry falls silent')
  })

  it('reports an empty run log without inventing narration', () => {
    const run = readRunNarration({ id: 'run_2', status: 'succeeded' })
    expect(run.items).toEqual([])
    expect(run.currentStep).toBeNull()
  })

  it('reads the orchestrator run as it really is: numeric steps, a message log, an error', () => {
    // The shape demo-scenario-orchestrator's Run.to_json() returns.
    const run = readRunNarration({
      id: 'run_01M4V8Q61WM0PGHRR4NY2ZF9MK',
      scenarioId: 'grand-tour',
      status: 'failed',
      currentStep: 24,
      totalSteps: 50,
      error: 'UpstreamError: malaise-injection-engine responded 400 unknown_target',
      log: [
        { time: '2026-10-13T18:34:40.573Z', step: 0, kind: 'run', level: 'info', message: 'The floor is yours.' },
      ],
    })
    expect(run.stepIndex).toBe(24)
    expect(run.stepCount).toBe(50)
    expect(run.currentStep).toBe('24 of 50')
    expect(run.error).toContain('unknown_target')
    expect(run.items[0]?.headline).toBe('The floor is yours.')
  })
})

describe('the upstream gateway', () => {
  /**
   * The kit's HttpClient is tested in the kit. What the console adds, and what
   * these cover, is: statuses pass through instead of throwing, an unset
   * variable is distinguishable from a dead host, and the polling loops can turn
   * retries off.
   */
  function gatewayOver(fetchImpl: typeof fetch) {
    const original = globalThis.fetch
    globalThis.fetch = fetchImpl
    const gateway = createGateway({ config: testConfig(), logger: SILENT_LOGGER })
    return {
      gateway,
      restore() {
        globalThis.fetch = original
      },
    }
  }

  function jsonResponse(status: number, body: unknown): Response {
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json' },
    })
  }

  it('passes a 409 through with its body rather than throwing', async () => {
    const { gateway, restore } = gatewayOver(
      (async () =>
        jsonResponse(409, {
          error: { code: 'insufficient_parts', message: 'HYD-MARZ short by 4' },
        })) as unknown as typeof fetch,
    )
    try {
      const res = await gateway.call({
        target: 'depot',
        method: 'POST',
        path: '/v1/reservations',
        body: {},
      })
      expect(res.status).toBe(409)
      expect((res.body as { error: { code: string } }).error.code).toBe('insufficient_parts')
    } finally {
      restore()
    }
  })

  it('passes a 503 through too, so the console shows the real failure', async () => {
    const { gateway, restore } = gatewayOver(
      (async () => jsonResponse(503, { error: { code: 'congested', message: 'queued out' } })) as
        unknown as typeof fetch,
    )
    try {
      const res = await gateway.call({
        target: 'grammeter',
        method: 'GET',
        path: '/v1/traffic',
        retries: 0,
      })
      expect(res.status).toBe(503)
    } finally {
      restore()
    }
  })

  it('throws UpstreamNotConfigured, naming the variable, when the URL is unset', async () => {
    const gateway = createGateway({
      config: testConfig({ CARDINAL_GRAMMETER_URL: undefined }),
      logger: SILENT_LOGGER,
    })
    await expect(
      gateway.call({ target: 'grammeter', method: 'GET', path: '/v1/traffic' }),
    ).rejects.toThrow(UpstreamNotConfigured)
    expect(gateway.info('grammeter')).toEqual({
      service: 'cardinal-grammeter',
      env: 'CARDINAL_GRAMMETER_URL',
      configured: false,
    })
  })

  it('throws UpstreamDown when the host never answers', async () => {
    const { gateway, restore } = gatewayOver(
      (async () => {
        throw new Error('connect ECONNREFUSED 10.0.0.5:8080')
      }) as unknown as typeof fetch,
    )
    try {
      await expect(
        gateway.call({
          target: 'datastore',
          method: 'GET',
          path: '/v1/stats',
          retries: 0,
        }),
      ).rejects.toThrow(UpstreamDown)
    } finally {
      restore()
    }
  })

  it('makes exactly one attempt when retries are off', async () => {
    const calls = vi.fn(async () => jsonResponse(503, {}))
    const { gateway, restore } = gatewayOver(calls as unknown as typeof fetch)
    try {
      await gateway.call({ target: 'detector', method: 'GET', path: '/v1/stats', retries: 0 })
      expect(calls).toHaveBeenCalledTimes(1)
    } finally {
      restore()
    }
  })

  it('reports Prometheus as an addressable target of its own', () => {
    const gateway = createGateway({ config: testConfig(), logger: SILENT_LOGGER })
    expect(gateway.info('prometheus')).toEqual({
      service: 'prometheus',
      env: 'PROMETHEUS_URL',
      configured: true,
    })
  })
})
