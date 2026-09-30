/**
 * The components.
 *
 * These assert what a reader actually sees, which is the only level at which
 * "no invented numbers" can be checked end to end: a null measurement must reach
 * the screen as the words "no data", and a plate with a dead upstream must name
 * the upstream.
 */

import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import GaugeDial from '@/components/panel/GaugeDial.vue'
import Lexeme from '@/components/panel/Lexeme.vue'
import LiveFeed from '@/components/panel/LiveFeed.vue'
import NarrationTicker from '@/components/panel/NarrationTicker.vue'
import NoData from '@/components/panel/NoData.vue'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'
import Readout from '@/components/panel/Readout.vue'
import RockerSwitch from '@/components/panel/RockerSwitch.vue'
import StatusLamp from '@/components/panel/StatusLamp.vue'
import TopologyPanel from '@/components/topology/TopologyPanel.vue'
import { NO_DATA, millis, rate } from '@/lib/format'
import { LEXICON } from '@/lib/lexicon'
import type { EdgeView, NodeView, TopologySnapshot } from '@shared/topology'
import { REGISTRY } from '@shared/registry'

function nodeOf(name: string, over: Partial<NodeView> = {}): NodeView {
  const registration = REGISTRY.find((r) => r.name === name)!
  return {
    ...registration,
    pods: [],
    desiredReplicas: null,
    readyReplicas: null,
    rps: null,
    p50Ms: null,
    p99Ms: null,
    errorRatio: null,
    health: 'nodata',
    ...over,
  }
}

describe('GaugeDial', () => {
  it('draws no needle and reads NO DATA when there is nothing to point at', () => {
    const wrapper = mount(GaugeDial, {
      props: { label: 'Orders / min', value: null, display: NO_DATA, max: 100 },
    })
    expect(wrapper.text()).toContain('NO DATA')
    // The needle is the only round-capped line; there must not be one. The
    // scale ticks are still drawn, because the instrument is not broken.
    const needles = wrapper.findAll('line').filter((l) => l.attributes('stroke-linecap') === 'round')
    expect(needles).toHaveLength(0)
    expect(wrapper.findAll('line').length).toBeGreaterThan(0)
  })

  it('draws a needle and the reading when there is a measurement', () => {
    const wrapper = mount(GaugeDial, {
      props: { label: 'Orders / min', value: 42, display: '42.0', max: 100 },
    })
    const needles = wrapper.findAll('line').filter((l) => l.attributes('stroke-linecap') === 'round')
    expect(needles).toHaveLength(1)
    expect(wrapper.text()).toContain('42.0')
    expect(wrapper.text()).not.toContain('NO DATA')
  })

  it('colours the needle amber then red as the value crosses the bands', () => {
    const needleOf = (value: number) => {
      const wrapper = mount(GaugeDial, {
        props: { label: 'x', value, display: String(value), max: 100, warnAt: 50, faultAt: 80 },
      })
      return wrapper
        .findAll('line')
        .find((l) => l.attributes('stroke-linecap') === 'round')
        ?.attributes('stroke')
    }
    expect(needleOf(10)).toBe('var(--c-phosphor)')
    expect(needleOf(60)).toBe('var(--c-amber)')
    expect(needleOf(90)).toBe('var(--c-lacquer)')
  })

  it('labels itself for a screen reader with the reading, not the raw number', () => {
    const wrapper = mount(GaugeDial, {
      props: { label: 'Refabrication rate', value: 5, display: '5.0%', max: 40 },
    })
    expect(wrapper.find('svg').attributes('aria-label')).toBe('Refabrication rate: 5.0%')
  })
})

describe('Readout', () => {
  it('shows the words "no data" rather than an empty window', () => {
    const wrapper = mount(Readout, { props: { label: 'Total stock', value: NO_DATA } })
    expect(wrapper.text()).toContain(NO_DATA)
    expect(wrapper.find('.no-data').exists()).toBe(true)
  })

  it('treats an empty string as absent too', () => {
    const wrapper = mount(Readout, { props: { label: 'Shortest SKU', value: '' } })
    expect(wrapper.find('.no-data').exists()).toBe(true)
  })

  it('shows a measured zero as a number, not as absent', () => {
    const wrapper = mount(Readout, { props: { label: 'In flight', value: '0' } })
    expect(wrapper.find('.no-data').exists()).toBe(false)
    expect(wrapper.text()).toContain('0')
  })
})

describe('NoData', () => {
  it('renders the two words and carries the reason in its tooltip', () => {
    const wrapper = mount(NoData, { props: { why: 'PROMETHEUS_URL is not set' } })
    expect(wrapper.text()).toBe(NO_DATA)
    expect(wrapper.attributes('title')).toBe('PROMETHEUS_URL is not set')
  })
})

describe('StatusLamp', () => {
  it('is dark for no data, never green by default', () => {
    const wrapper = mount(StatusLamp, { props: { health: 'nodata' } })
    expect(wrapper.attributes('title')).toBe('no data')
    expect(wrapper.html()).toContain('var(--c-slate)')
  })

  it('lights each band its own colour', () => {
    expect(mount(StatusLamp, { props: { health: 'healthy' } }).html()).toContain('var(--c-phosphor)')
    expect(mount(StatusLamp, { props: { health: 'degraded' } }).html()).toContain('var(--c-amber)')
    expect(mount(StatusLamp, { props: { health: 'faulted' } }).html()).toContain('var(--c-lacquer)')
  })
})

describe('Lexeme', () => {
  it('prints the 1944 term and explains it in the tooltip', () => {
    const wrapper = mount(Lexeme, { props: { of: 'skorMotion' } })
    expect(wrapper.text()).toBe('Fluorescent skor motion')
    const title = wrapper.attributes('title') ?? ''
    expect(title).toContain('Requests per second')
    expect(title).toContain('http_server_request_duration_seconds')
  })

  it('has a tooltip for every term in the lexicon', () => {
    for (const key of Object.keys(LEXICON)) {
      const wrapper = mount(Lexeme, { props: { of: key as keyof typeof LEXICON } })
      expect((wrapper.attributes('title') ?? '').length, key).toBeGreaterThan(8)
    }
  })
})

describe('PanelPlate', () => {
  it('names the failed upstream instead of showing an empty plate', () => {
    const wrapper = mount(PanelPlate, {
      props: {
        legend: 'Parts cage',
        source: { ok: false, detail: 'PREFABULATED_AMULITE_DEPOT_URL is not set' },
      },
      slots: { default: '<p>should not be shown</p>' },
    })
    expect(wrapper.text()).toContain('No data.')
    expect(wrapper.text()).toContain('PREFABULATED_AMULITE_DEPOT_URL')
    expect(wrapper.text()).not.toContain('should not be shown')
  })

  it('shows its content when the source is healthy', () => {
    const wrapper = mount(PanelPlate, {
      props: { legend: 'Parts cage', source: { ok: true, detail: null } },
      slots: { default: '<p>eight hundred marzlevanes</p>' },
    })
    expect(wrapper.text()).toContain('eight hundred marzlevanes')
  })

  it('shows the empty note when there is simply nothing to show', () => {
    const wrapper = mount(PanelPlate, {
      props: { legend: 'Malaise', empty: true, emptyNote: 'The works are in good health.' },
      slots: { default: '<p>rows</p>' },
    })
    expect(wrapper.text()).toContain('good health')
  })
})

describe('Notice', () => {
  it('renders nothing at all when there is no message', () => {
    expect(mount(Notice, { props: { message: null } }).html()).toBe('<!--v-if-->')
  })

  it('renders the message when there is one', () => {
    const wrapper = mount(Notice, { props: { message: 'Refused: operator role required.', tone: 'error' } })
    expect(wrapper.text()).toContain('operator role required')
  })
})

describe('RockerSwitch', () => {
  it('is a switch with an accessible checked state', () => {
    const wrapper = mount(RockerSwitch, { props: { modelValue: true } })
    expect(wrapper.attributes('role')).toBe('switch')
    expect(wrapper.attributes('aria-checked')).toBe('true')
  })

  it('carries the reason it is disabled, so a viewer knows why', () => {
    const wrapper = mount(RockerSwitch, {
      props: { modelValue: false, disabled: true, reason: 'Requires the operator role.' },
    })
    expect(wrapper.attributes('title')).toBe('Requires the operator role.')
  })

  it('emits the flipped value', async () => {
    const wrapper = mount(RockerSwitch, { props: { modelValue: false } })
    await wrapper.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
  })

  it('emits nothing while disabled', async () => {
    const wrapper = mount(RockerSwitch, { props: { modelValue: false, disabled: true } })
    await wrapper.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})

describe('LiveFeed', () => {
  it('marks a field the upstream did not supply as no data', () => {
    const wrapper = mount(LiveFeed, {
      props: {
        legend: 'Nofer trunnions',
        items: [
          {
            id: 'shipment:1',
            at: new Date().toISOString(),
            kind: 'shipment',
            headline: 'x',
            detail: null,
            fields: { order: 'ord_1', carrier: 'Trunnion Freight', tracking: 'no data' },
          },
        ],
        columns: [
          { key: 'order', label: 'Order' },
          { key: 'carrier', label: 'Carrier' },
          { key: 'tracking', label: 'Tracking' },
        ],
        emptyNote: 'No nofer trunnions in transit.',
      },
    })
    const cells = wrapper.findAll('td')
    const tracking = cells.find((c) => c.text() === 'no data')
    expect(tracking?.classes()).toContain('no-data')
  })

  it('explains an empty feed in the register of the panel', () => {
    const wrapper = mount(LiveFeed, {
      props: {
        legend: 'Side fumbling',
        items: [],
        columns: [{ key: 'serial', label: 'Serial' }],
        emptyNote: 'No side fumbling detected.',
      },
    })
    expect(wrapper.text()).toContain('No side fumbling detected.')
  })
})

describe('NarrationTicker', () => {
  it('says nothing is running rather than scrolling filler', () => {
    const wrapper = mount(NarrationTicker, { props: { narration: [], scenario: null } })
    expect(wrapper.text()).toContain('No scenario running')
  })

  it('prints narration and the run’s step progress', () => {
    const wrapper = mount(NarrationTicker, {
      props: {
        narration: [
          {
            id: 'n1',
            at: '2026-09-30T12:00:00.000Z',
            kind: 'narration',
            headline: 'The amulite foundry falls silent.',
            detail: null,
            fields: {},
          },
        ],
        scenario: {
          runId: 'run_1',
          scenarioId: 'marzlevane-shortage',
          status: 'running',
          currentStep: 'stop the foundry',
          stepIndex: 2,
          stepCount: 7,
          items: [],
        },
      },
    })
    expect(wrapper.text()).toContain('foundry falls silent')
    expect(wrapper.text()).toContain('step 2 of 7')
    expect(wrapper.text()).toContain('running')
  })
})

describe('TopologyPanel', () => {
  const SNAPSHOT: TopologySnapshot = {
    at: '2026-09-30T12:00:00.000Z',
    nodes: [
      nodeOf('dingle-arm-gateway', {
        rps: 12,
        p50Ms: 8,
        p99Ms: 40,
        errorRatio: 0,
        health: 'healthy',
        desiredReplicas: 2,
        readyReplicas: 2,
        pods: [
          { name: 'gw-1', phase: 'Running', ready: true, restarts: 0, rps: 6, p50Ms: 8, p99Ms: 40, errorRatio: 0 },
          { name: 'gw-2', phase: 'Running', ready: false, restarts: 3, rps: 6, p50Ms: 9, p99Ms: 44, errorRatio: 0 },
        ],
      }),
      nodeOf('panametric-fam-processor'),
      nodeOf('nats'),
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
      } satisfies EdgeView,
      {
        source: 'panametric-fam-processor',
        target: 'nats',
        via: 'messaging',
        rps: null,
        errorRatio: null,
        p50Ms: null,
        p99Ms: null,
        health: 'nodata',
      } satisfies EdgeView,
    ],
    unmapped: [],
    sources: { kubernetes: { ok: true, detail: null }, prometheus: { ok: true, detail: null } },
  }

  it('draws a plate per node and a line per edge, and nothing else', () => {
    setActivePinia(createPinia())
    const wrapper = mount(TopologyPanel, { props: { snapshot: SNAPSHOT, animate: false } })
    // One visible path plus one transparent hit path per edge.
    const paths = wrapper.findAll('svg path')
    expect(paths.length).toBe(SNAPSHOT.edges.length * 2)
    // Three node plates.
    expect(wrapper.findAll('svg rect[rx="3"]')).toHaveLength(3)
  })

  it('dashes an edge with no service-graph data instead of drawing it solid', () => {
    setActivePinia(createPinia())
    const wrapper = mount(TopologyPanel, { props: { snapshot: SNAPSHOT, animate: false } })
    const dashed = wrapper
      .findAll('svg path')
      .filter((p) => p.attributes('stroke-dasharray') !== undefined)
    expect(dashed).toHaveLength(1)
    expect(dashed[0]?.attributes('stroke')).toBe('var(--c-slate)')
  })

  it('prints a node’s measured rate, and "no data" for one Prometheus has not seen', () => {
    setActivePinia(createPinia())
    const wrapper = mount(TopologyPanel, { props: { snapshot: SNAPSHOT, animate: false } })
    expect(wrapper.text()).toContain(`${rate(12)}/s`)
    expect(wrapper.text()).toContain(`p99 ${millis(40)}`)
    // The processor has no metrics at all.
    expect(wrapper.text()).toContain('no data')
  })

  it('draws a dot per pod, amber for one that is not ready', () => {
    setActivePinia(createPinia())
    const wrapper = mount(TopologyPanel, { props: { snapshot: SNAPSHOT, animate: false } })
    const dots = wrapper.findAll('circle[r="3.2"]')
    expect(dots).toHaveLength(2)
    expect(dots.map((d) => d.attributes('fill'))).toEqual(['var(--c-phosphor)', 'var(--c-amber)'])
  })

  it('shows a node tooltip on hover, with the epigraph and the plain meaning', async () => {
    setActivePinia(createPinia())
    const wrapper = mount(TopologyPanel, { props: { snapshot: SNAPSHOT, animate: false } })
    const plate = wrapper.findAll('svg > g').at(-1)!.findAll('g')[0]!
    await plate.trigger('mouseenter')
    expect(wrapper.text()).toContain('drawn reciprocation dingle arm')
    expect(wrapper.text()).toContain('Edge API gateway')
  })

  it('renders an empty panel without a snapshot, rather than failing', () => {
    setActivePinia(createPinia())
    const wrapper = mount(TopologyPanel, { props: { snapshot: null, animate: false } })
    expect(wrapper.findAll('svg rect[rx="3"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('Factory floor')
  })
})
