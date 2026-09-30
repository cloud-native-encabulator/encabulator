/**
 * Every page, mounted.
 *
 * A template error in one view would otherwise only surface on stage, so each
 * page is mounted twice: once with no data at all (the first three seconds of a
 * cold start, and the permanent state of a misconfigured deployment) and once
 * with a full snapshot. Neither may throw, and the empty case must say why it is
 * empty rather than showing a blank plate.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import Administration from '@/views/Administration.vue'
import FactoryFloor from '@/views/FactoryFloor.vue'
import Girdlespring from '@/views/Girdlespring.vue'
import Grammeters from '@/views/Grammeters.vue'
import Malaise from '@/views/Malaise.vue'
import NotFound from '@/views/NotFound.vue'
import Scenarios from '@/views/Scenarios.vue'
import SignIn from '@/views/SignIn.vue'
import Works from '@/views/Works.vue'
import { routes } from '@/router'
import { useLiveStore } from '@/stores/live'
import { useSessionStore } from '@/stores/session'
import type { FeedBatch, HeadlineStats, TopologySnapshot } from '@shared/topology'
import { REGISTRY } from '@shared/registry'

const PAGES = [
  ['Factory Floor', FactoryFloor],
  ['Grammeters', Grammeters],
  ['Malaise', Malaise],
  ['Scenarios', Scenarios],
  ['Girdlespring', Girdlespring],
  ['Works', Works],
  ['Administration', Administration],
  ['Sign in', SignIn],
  ['Not found', NotFound],
] as const

let router: Router

/** Answers whatever any store asks for, in the shapes the BFF returns. */
function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input)
    const body = (() => {
      if (url.includes('/traffic')) {
        return {
          running: true,
          rps: 12,
          pattern: 'sine',
          periodSeconds: 60,
          customers: 50,
          mix: { browse: 40, quote: 25, order: 20, track: 15 },
          targetRps: 12,
          actualRps: 11.4,
          inFlight: 3,
          dropped: 0,
          journeys: { browse: { count: 10, errors: 0, p50Ms: 8, p99Ms: 30 } },
          statusCodes: { '200': 10 },
        }
      }
      if (url.includes('/malaises')) {
        return {
          malaises: [
            {
              id: 'mal_1',
              target: 'spurving-bearing-quoter',
              kind: 'latency',
              params: { ms: 400 },
              state: 'active',
              expiresAt: new Date(Date.now() + 60_000).toISOString(),
            },
          ],
        }
      }
      if (url.includes('/scenarios')) {
        return { scenarios: [{ id: 'grand-tour', title: 'The grand tour', description: 'All of it.' }] }
      }
      if (url.includes('/runs')) {
        return { runs: [{ id: 'run_1', scenarioId: 'grand-tour', status: 'running', startedAt: new Date().toISOString() }] }
      }
      if (url.includes('/flags')) {
        return {
          flags: [
            { key: 'turbo-encabulation', type: 'boolean', value: false, updatedBy: 'seed', updatedAt: new Date().toISOString() },
            { key: 'amulite-foundry-rate', type: 'number', value: 20, updatedBy: 'seed', updatedAt: new Date().toISOString() },
          ],
        }
      }
      if (url.includes('/parts')) {
        return [{ sku: 'HYD-MARZ', name: 'Hydrocoptic marzlevane', stock: 40, reserved: 6, perUnit: 6 }]
      }
      if (url.includes('/orders')) {
        return { orders: [{ id: 'ord_1', customer: 'flora', model: 'turbo', quantity: 2, status: 'fabricating' }] }
      }
      if (url.includes('/users')) {
        return { users: [{ username: 'admin', displayName: 'Admin', roles: ['admin'] }] }
      }
      if (url.includes('/audit')) {
        return { entries: [{ time: new Date().toISOString(), action: 'login', subject: 'admin', result: 'success' }] }
      }
      if (url.includes('/session')) {
        return {
          session: {
            authenticated: true,
            username: 'works-manager',
            displayName: 'Works Manager',
            roles: ['admin'],
            expiresAt: null,
            canOperate: true,
            canAdminister: true,
          },
          links: { grafana: null, jaeger: null },
          upstreams: {},
          prometheusConfigured: true,
        }
      }
      return {}
    })()
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  }) as unknown as typeof fetch
}

function fullSnapshot(): TopologySnapshot {
  return {
    at: new Date().toISOString(),
    nodes: REGISTRY.map((registration, index) => ({
      ...registration,
      pods: [
        {
          name: `${registration.name}-abc${index}`,
          phase: 'Running',
          ready: index % 7 !== 0,
          restarts: index % 3,
          rps: registration.kind === 'store' ? null : 4 + index,
          p50Ms: registration.kind === 'store' ? null : 10,
          p99Ms: registration.kind === 'store' ? null : 40 + index * 30,
          errorRatio: registration.kind === 'store' ? null : 0,
        },
      ],
      desiredReplicas: registration.kind === 'store' ? null : 1,
      readyReplicas: registration.kind === 'store' ? null : 1,
      rps: registration.kind === 'store' ? null : 4 + index,
      p50Ms: registration.kind === 'store' ? null : 10,
      p99Ms: registration.kind === 'store' ? null : 40 + index * 30,
      errorRatio: registration.kind === 'store' ? null : 0,
      health: registration.kind === 'store' ? ('nodata' as const) : ('healthy' as const),
    })),
    edges: [
      {
        source: 'cardinal-grammeter',
        target: 'dingle-arm-gateway',
        via: 'http' as const,
        rps: 22,
        errorRatio: 0.002,
        p50Ms: 9,
        p99Ms: 42,
        health: 'healthy' as const,
      },
      {
        source: 'hydrocoptic-datastore',
        target: 'postgres',
        via: 'database' as const,
        rps: null,
        errorRatio: null,
        p50Ms: null,
        p99Ms: null,
        health: 'nodata' as const,
      },
    ],
    unmapped: [{ client: 'kube-probe', server: 'dingle-arm-gateway', rps: 0.2 }],
    sources: { kubernetes: { ok: true, detail: null }, prometheus: { ok: true, detail: null } },
  }
}

const HEADLINE: HeadlineStats = {
  at: new Date().toISOString(),
  ordersPerMin: 8.2,
  unitsInFabrication: 6,
  refabricationRate: 0.068,
  partsTotal: 11_334,
  partsLowestSku: 'LUN-WANE',
  partsLowestStock: 47,
  unitsDelivered: 25,
  replenerationMean: 1.011,
  sources: {
    datastore: { ok: true, detail: null },
    detector: { ok: true, detail: null },
    depot: { ok: true, detail: null },
    recorder: { ok: true, detail: null },
  },
}

const FEED: FeedBatch = {
  at: new Date().toISOString(),
  items: [
    { id: 'order:ord_1', at: new Date().toISOString(), kind: 'order', headline: 'turbo ×2 — fabricating', detail: null, fields: { order: 'ord_1', model: 'turbo', quantity: '2', status: 'fabricating', stage: 'fit-marzlevanes' } },
    { id: 'incident:inc_1', at: new Date().toISOString(), kind: 'incident', headline: 'Side fumbling', detail: 'z above 4', fields: { serial: 'ENC-1', stage: 'fit-marzlevanes', metric: 'repleneration', score: '4.82', strict: 'false' } },
    { id: 'shipment:shp_1', at: new Date().toISOString(), kind: 'shipment', headline: 'Waneshaft Express', detail: null, fields: { order: 'ord_3', carrier: 'Waneshaft Express', tracking: 'NT1', status: 'dispatched' } },
    { id: 'narration:1', at: new Date().toISOString(), kind: 'narration', headline: 'The morning shift arrives.', detail: null, fields: {} },
  ],
  sources: { datastore: { ok: true, detail: null } },
}

beforeEach(async () => {
  setActivePinia(createPinia())
  stubFetch()
  router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/')
  await router.isReady()
})

afterEach(() => {
  vi.restoreAllMocks()
})

async function render(component: unknown) {
  const wrapper = mount(component as never, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('every page, with nothing to show', () => {
  for (const [name, component] of PAGES) {
    it(`${name} renders without data and explains itself`, async () => {
      const wrapper = await render(component)
      expect(wrapper.html().length).toBeGreaterThan(80)
      // No page may render the literal string "undefined" or "NaN" at a reader.
      expect(wrapper.text()).not.toContain('undefined')
      expect(wrapper.text()).not.toContain('NaN')
      expect(wrapper.text()).not.toContain('[object Object]')
      wrapper.unmount()
    })
  }
})

describe('every page, with a full snapshot', () => {
  beforeEach(async () => {
    const live = useLiveStore()
    live.topology = fullSnapshot()
    live.headline = HEADLINE
    live.applyFeed(FEED)
    live.scenario = {
      runId: 'run_1',
      scenarioId: 'grand-tour',
      status: 'running',
      currentStep: 'ramp the grammeters',
      stepIndex: 3,
      stepCount: 11,
      items: [],
    }
    const session = useSessionStore()
    await session.load()
  })

  for (const [name, component] of PAGES) {
    it(`${name} renders with data and prints no placeholder rubbish`, async () => {
      const wrapper = await render(component)
      expect(wrapper.text()).not.toContain('undefined')
      expect(wrapper.text()).not.toContain('NaN')
      expect(wrapper.text()).not.toContain('[object Object]')
      wrapper.unmount()
    })
  }

  it('the Factory Floor shows its headline readings and the undrawn peer', async () => {
    const wrapper = await render(FactoryFloor)
    expect(wrapper.text()).toContain('8.2')
    expect(wrapper.text()).toContain('LUN-WANE')
    expect(wrapper.text()).toContain('kube-probe')
    wrapper.unmount()
  })

  it('the Works page lists the parts cage and the live feeds', async () => {
    const wrapper = await render(Works)
    expect(wrapper.text()).toContain('Hydrocoptic marzlevane')
    expect(wrapper.text()).toContain('Waneshaft Express')
    expect(wrapper.text()).toContain('ENC-1')
    wrapper.unmount()
  })

  it('Girdlespring explains what each flag actually changes', async () => {
    const wrapper = await render(Girdlespring)
    expect(wrapper.text()).toContain('Halves every fabrication stage duration')
    expect(wrapper.text()).toContain('Units of each SKU restocked every ten seconds')
    wrapper.unmount()
  })

  it('Malaise offers only targets other than the console itself', async () => {
    const wrapper = await render(Malaise)
    const options = wrapper.findAll('option').map((o) => o.attributes('value'))
    expect(options).not.toContain('encabulation-control-console')
    expect(options).toContain('spurving-bearing-quoter')
    wrapper.unmount()
  })

  it('Administration shows users to an admin', async () => {
    const wrapper = await render(Administration)
    expect(wrapper.text()).toContain('admin')
    expect(wrapper.text()).not.toContain('requires the admin role')
    wrapper.unmount()
  })

  it('Scenarios follows the active run', async () => {
    const wrapper = await render(Scenarios)
    expect(wrapper.text()).toContain('grand-tour')
    expect(wrapper.text()).toContain('ramp the grammeters')
    expect(wrapper.text()).toContain('27%')
    wrapper.unmount()
  })
})

describe('a viewer sees locked controls with a reason', () => {
  beforeEach(() => {
    globalThis.fetch = (async () =>
      new Response(
        JSON.stringify({
          session: {
            authenticated: false,
            username: 'guest',
            displayName: 'Guest',
            roles: ['viewer'],
            expiresAt: null,
            canOperate: false,
            canAdminister: false,
          },
          links: { grafana: null, jaeger: null },
          upstreams: {},
          prometheusConfigured: true,
        }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      )) as unknown as typeof fetch
  })

  it('disables the traffic controls on Grammeters', async () => {
    const session = useSessionStore()
    await session.load()
    const wrapper = await render(Grammeters)
    expect(wrapper.text()).toContain('You are a viewer')
    const apply = wrapper.findAll('button').find((b) => b.text().includes('Apply'))
    expect(apply?.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('refuses Administration and names the role required', async () => {
    const session = useSessionStore()
    await session.load()
    const wrapper = await render(Administration)
    expect(wrapper.text()).toContain('requires the admin role')
    wrapper.unmount()
  })
})
