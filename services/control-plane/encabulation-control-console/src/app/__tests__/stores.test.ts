/**
 * The Pinia stores.
 *
 * The live store is tested by feeding it the exact frames the BFF broadcasts,
 * which keeps the two halves of the stream contract honest in one place. The
 * control stores are tested for the behaviour that matters operationally: a
 * refusal from the server becomes a message an operator can read, and never a
 * silent no-op.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { FEED_LIMIT, useLiveStore } from '@/stores/live'
import { GUEST_SESSION, useSessionStore } from '@/stores/session'
import { useFlagStore, useMalaiseStore, useTrafficStore, useWorksStore } from '@/stores/controls'
import type { FeedBatch, HeadlineStats, TopologySnapshot } from '@shared/topology'

function messageEvent(data: unknown): MessageEvent<string> {
  return { data: JSON.stringify(data) } as MessageEvent<string>
}

const SNAPSHOT: TopologySnapshot = {
  at: '2026-09-30T12:00:00.000Z',
  nodes: [
    {
      name: 'dingle-arm-gateway',
      display: 'Dingle-Arm Gateway',
      plane: 'processing',
      kind: 'service',
      language: 'Go',
      epigraph: 'The drawn reciprocation dingle arm.',
      plainly: 'Edge API gateway.',
      lay: { col: 5.5, row: 1 },
      pods: [
        {
          name: 'dingle-arm-gateway-abc',
          phase: 'Running',
          ready: true,
          restarts: 0,
          rps: 12,
          p50Ms: 8,
          p99Ms: 40,
          errorRatio: 0,
        },
      ],
      desiredReplicas: 2,
      readyReplicas: 1,
      rps: 12,
      p50Ms: 8,
      p99Ms: 40,
      errorRatio: 0,
      health: 'healthy',
    },
  ],
  edges: [
    {
      source: 'dingle-arm-gateway',
      target: 'panametric-fam-processor',
      via: 'http',
      rps: 8,
      errorRatio: 0,
      p50Ms: 12,
      p99Ms: 55,
      health: 'healthy',
    },
    {
      source: 'panametric-fam-processor',
      target: 'nats',
      via: 'messaging',
      rps: null,
      errorRatio: null,
      p50Ms: null,
      p99Ms: null,
      health: 'nodata',
    },
  ],
  unmapped: [{ client: 'something-else', server: 'dingle-arm-gateway', rps: 1 }],
  sources: {
    kubernetes: { ok: true, detail: null },
    prometheus: { ok: true, detail: null },
  },
}

const HEADLINE: HeadlineStats = {
  at: '2026-09-30T12:00:00.000Z',
  ordersPerMin: 6,
  unitsInFabrication: 4,
  refabricationRate: 0.05,
  partsTotal: 900,
  partsLowestSku: 'HYD-MARZ',
  partsLowestStock: 40,
  unitsDelivered: 11,
  replenerationMean: 1.01,
  sources: {
    datastore: { ok: true, detail: null },
    detector: { ok: true, detail: null },
    depot: { ok: true, detail: null },
    recorder: { ok: false, detail: 'SINUSOIDAL_RECORDER_URL is not set' },
  },
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('the live store', () => {
  it('applies a topology frame and exposes its parts', () => {
    const live = useLiveStore()
    live.handle(messageEvent(SNAPSHOT), 'topology')
    expect(live.nodes).toHaveLength(1)
    expect(live.edges).toHaveLength(2)
    expect(live.podCount).toBe(1)
    // Only the edge with data counts as active; the nodata one does not.
    expect(live.activeEdges).toBe(1)
    expect(live.unmapped).toHaveLength(1)
    expect(live.sources.prometheus.ok).toBe(true)
  })

  it('applies a headline frame, keeping a failed source visible', () => {
    const live = useLiveStore()
    live.handle(messageEvent(HEADLINE), 'headline')
    expect(live.headline?.ordersPerMin).toBe(6)
    expect(live.headline?.sources['recorder']?.ok).toBe(false)
  })

  it('reports empty sources before the first frame, rather than claiming health', () => {
    const live = useLiveStore()
    expect(live.sources.prometheus.ok).toBe(false)
    expect(live.sources.kubernetes.ok).toBe(false)
    expect(live.nodes).toEqual([])
  })

  it('sorts feed items into their own buckets, newest first', () => {
    const live = useLiveStore()
    const batch: FeedBatch = {
      at: '2026-09-30T12:00:02.000Z',
      items: [
        {
          id: 'order:ord_1:accepted:',
          at: '2026-09-30T12:00:00.000Z',
          kind: 'order',
          headline: 'turbo ×1 · accepted',
          detail: null,
          fields: { order: 'ord_1' },
        },
        {
          id: 'incident:inc_1',
          at: '2026-09-30T12:00:01.000Z',
          kind: 'incident',
          headline: 'Side fumbling detected',
          detail: null,
          fields: { serial: 'ENC-1' },
        },
        {
          id: 'order:ord_2:accepted:',
          at: '2026-09-30T12:00:02.000Z',
          kind: 'order',
          headline: 'hyper ×2 · accepted',
          detail: null,
          fields: { order: 'ord_2' },
        },
      ],
      sources: { datastore: { ok: true, detail: null } },
    }
    live.handle(messageEvent(batch), 'feed')
    expect(live.orders.map((o) => o.fields['order'])).toEqual(['ord_2', 'ord_1'])
    expect(live.incidents).toHaveLength(1)
    expect(live.shipments).toEqual([])
  })

  it('does not duplicate a row the stream replays', () => {
    const live = useLiveStore()
    const item = {
      id: 'shipment:shp_1:dispatched',
      at: '2026-09-30T12:00:00.000Z',
      kind: 'shipment' as const,
      headline: 'Waneshaft Express · dispatched',
      detail: null,
      fields: {},
    }
    const batch: FeedBatch = { at: 'x', items: [item], sources: {} }
    live.handle(messageEvent(batch), 'feed')
    live.handle(messageEvent(batch), 'feed')
    expect(live.shipments).toHaveLength(1)
  })

  it('caps each feed so a long demo cannot grow without bound', () => {
    const live = useLiveStore()
    live.applyFeed({
      at: 'x',
      items: Array.from({ length: FEED_LIMIT + 50 }, (_, i) => ({
        id: `order:ord_${i}`,
        at: '2026-09-30T12:00:00.000Z',
        kind: 'order' as const,
        headline: `order ${i}`,
        detail: null,
        fields: {},
      })),
      sources: {},
    })
    expect(live.orders).toHaveLength(FEED_LIMIT)
  })

  it('records a frame it cannot parse instead of throwing into the stream', () => {
    const live = useLiveStore()
    live.handle({ data: 'not json' } as MessageEvent<string>, 'topology')
    expect(live.lastError).toContain('could not read')
    expect(live.topology).toBeNull()
  })

  it('applies a scenario frame, and a null one when a run ends', () => {
    const live = useLiveStore()
    live.handle(
      messageEvent({
        runId: 'run_1',
        scenarioId: 'grand-tour',
        status: 'running',
        currentStep: 'ramp up',
        stepIndex: 2,
        stepCount: 9,
        items: [],
      }),
      'scenario',
    )
    expect(live.scenario?.scenarioId).toBe('grand-tour')
    live.handle(messageEvent(null), 'scenario')
    expect(live.scenario).toBeNull()
  })
})

describe('the session store', () => {
  it('starts as a guest viewer who may not operate', () => {
    const session = useSessionStore()
    expect(session.session).toEqual(GUEST_SESSION)
    expect(session.isGuest).toBe(true)
    expect(session.canOperate).toBe(false)
    expect(session.canAdminister).toBe(false)
  })

  it('takes canOperate from the server rather than deciding for itself', () => {
    const session = useSessionStore()
    session.apply({
      session: {
        authenticated: true,
        username: 'shift-lead',
        displayName: 'Shift Lead',
        roles: ['operator'],
        expiresAt: null,
        canOperate: true,
        canAdminister: false,
      },
      links: { grafana: null, jaeger: null },
      upstreams: {
        recorder: { service: 'sinusoidal-recorder', configured: false, env: 'SINUSOIDAL_RECORDER_URL' },
        depot: { service: 'prefabulated-amulite-depot', configured: true, env: 'PREFABULATED_AMULITE_DEPOT_URL' },
      },
      prometheusConfigured: true,
    })
    expect(session.canOperate).toBe(true)
    expect(session.canAdminister).toBe(false)
    expect(session.roleLabel).toBe('operator')
    expect(session.unconfigured.map((u) => u.env)).toEqual(['SINUSOIDAL_RECORDER_URL'])
    expect(session.upstreamFor('depot')?.configured).toBe(true)
    expect(session.upstreamFor('nonexistent')).toBeNull()
  })
})

describe('the control stores', () => {
  function stubFetch(reply: { status: number; body: unknown }) {
    const fn = vi.fn(async () =>
      new Response(JSON.stringify(reply.body), {
        status: reply.status,
        headers: { 'content-type': 'application/json' },
      }),
    )
    globalThis.fetch = fn as unknown as typeof fetch
    return fn
  }

  it('turns a 403 from the BFF into a message, not a silent failure', async () => {
    stubFetch({
      status: 403,
      body: { error: { code: 'forbidden', message: 'This control requires the operator role.' } },
    })
    const traffic = useTrafficStore()
    const result = await traffic.change({ rps: 50 })
    expect(result.ok).toBe(false)
    expect(traffic.notice).toContain('operator role')
  })

  it('turns a 401 into a message telling the reader to sign in', async () => {
    stubFetch({
      status: 401,
      body: { error: { code: 'unauthenticated', message: 'Sign in as operator or admin.' } },
    })
    const malaise = useMalaiseStore()
    const result = await malaise.induce({
      target: 'spurving-bearing-quoter',
      kind: 'latency',
      params: { ms: 400 },
      durationSeconds: 60,
    })
    expect(result.ok).toBe(false)
    expect(malaise.notice).toContain('Sign in')
  })

  it('says which variable is missing when the BFF has no upstream address', async () => {
    stubFetch({
      status: 503,
      body: {
        error: {
          code: 'upstream_not_configured',
          message: 'cardinal-grammeter has no address: set CARDINAL_GRAMMETER_URL.',
        },
      },
    })
    const traffic = useTrafficStore()
    await traffic.reload()
    expect(traffic.error).toContain('CARDINAL_GRAMMETER_URL')
    expect(traffic.data).toBeNull()
  })

  it('reads a flag list in either wire shape', async () => {
    stubFetch({
      status: 200,
      body: { flags: [{ key: 'turbo-encabulation', type: 'boolean', value: false }] },
    })
    const flags = useFlagStore()
    await flags.reload()
    expect(flags.data).toHaveLength(1)

    stubFetch({ status: 200, body: [{ key: 'quote-cache', type: 'boolean', value: true }] })
    await flags.reload()
    expect(flags.data[0]?.key).toBe('quote-cache')
  })

  it('surfaces an insufficient_parts 409 from the storefront verbatim', async () => {
    stubFetch({
      status: 409,
      body: {
        error: { code: 'insufficient_parts', message: 'HYD-MARZ short by 4' },
      },
    })
    const works = useWorksStore()
    const result = await works.placeOrder({ model: 'turbo', quantity: 3, options: { dingleArm: false } })
    expect(result.ok).toBe(false)
    expect(works.notice).toContain('HYD-MARZ')
    expect(works.placed).toBeNull()
  })

  it('names the shortest part without inventing one when stock is unknown', async () => {
    stubFetch({
      status: 200,
      body: [
        { sku: 'AMU-BASE', name: 'Base-plate', stock: 900, reserved: 0, perUnit: 1 },
        { sku: 'HYD-MARZ', name: 'Marzlevane', stock: 40, reserved: 0, perUnit: 6 },
        { sku: 'PNT-FAN', name: 'Fan', stock: null, reserved: null, perUnit: 1 },
      ],
    })
    const works = useWorksStore()
    await works.loadParts()
    expect(works.lowestStock?.sku).toBe('HYD-MARZ')
  })

  it('keeps the last good value when a reload fails', async () => {
    stubFetch({ status: 200, body: [{ sku: 'AMU-BASE', name: 'Base', stock: 10, reserved: 0, perUnit: 1 }] })
    const works = useWorksStore()
    await works.loadParts()
    expect(works.parts).toHaveLength(1)

    stubFetch({ status: 502, body: { error: { code: 'upstream_unreachable', message: 'depot is unreachable' } } })
    await works.loadParts()
    expect(works.parts).toHaveLength(1)
    expect(works.partsError).toContain('unreachable')
  })
})
