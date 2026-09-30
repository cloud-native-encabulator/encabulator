/**
 * The console shell.
 *
 * A smoke test for the thing every page sits inside: it must mount, render the
 * seven pages, open exactly one stream, respect the guest banner, and hold the
 * enamel / control-room finish. `EventSource` does not exist in the test DOM, so
 * it is stubbed — which also lets the test assert that the shell opens one
 * connection and releases it when the tab goes away.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import App from '@/App.vue'
import { routes } from '@/router'

class StubEventSource {
  static instances: StubEventSource[] = []
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2

  readyState = 0
  closed = false
  listeners = new Map<string, ((event: MessageEvent<string>) => void)[]>()
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(readonly url: string) {
    StubEventSource.instances.push(this)
  }

  addEventListener(name: string, handler: (event: MessageEvent<string>) => void): void {
    const list = this.listeners.get(name) ?? []
    list.push(handler)
    this.listeners.set(name, list)
  }

  emit(name: string, data: unknown): void {
    for (const handler of this.listeners.get(name) ?? []) {
      handler({ data: JSON.stringify(data) } as MessageEvent<string>)
    }
  }

  close(): void {
    this.closed = true
    this.readyState = 2
  }
}

function stubFetch(body: unknown, status = 200): void {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json' },
    })) as unknown as typeof fetch
}

const SESSION_BODY = {
  session: {
    authenticated: false,
    username: 'guest',
    displayName: 'Guest',
    roles: ['viewer'],
    expiresAt: null,
    canOperate: false,
    canAdminister: false,
  },
  links: { grafana: 'https://grafana.test', jaeger: 'https://jaeger.test' },
  upstreams: {},
  prometheusConfigured: true,
}

async function mountShell(initialPath = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(initialPath)
  await router.isReady()
  const wrapper = mount(App, { global: { plugins: [createPinia(), router] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  StubEventSource.instances = []
  ;(globalThis as { EventSource?: unknown }).EventSource = StubEventSource
  stubFetch(SESSION_BODY)
  document.documentElement.className = ''
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the shell', () => {
  it('mounts and prints the nameplate', async () => {
    const { wrapper } = await mountShell()
    expect(wrapper.text()).toContain('Encabulation Control Console')
    expect(wrapper.text()).toContain('The Encabulator Works')
    // The claim on the front of the panel.
    expect(wrapper.text()).toContain('Every reading on this panel is measured')
  })

  it('offers the seven pages the design names', async () => {
    const { wrapper } = await mountShell()
    const labels = wrapper.findAll('nav a').map((a) => a.text())
    expect(labels).toEqual([
      'Factory Floor',
      'Grammeters',
      'Malaise',
      'Scenarios',
      'Girdlespring',
      'Works',
      'Administration',
    ])
  })

  it('links every page to a route that resolves', async () => {
    const { wrapper, router } = await mountShell()
    for (const anchor of wrapper.findAll('nav a')) {
      const href = anchor.attributes('href') ?? ''
      const resolved = router.resolve(href)
      expect(resolved.name, href).not.toBe('not-found')
    }
  })

  it('opens exactly one stream, however many pages are shown', async () => {
    await mountShell()
    expect(StubEventSource.instances).toHaveLength(1)
    expect(StubEventSource.instances[0]?.url).toBe('/bff/stream')
  })

  it('closes the stream when the component goes away, so the BFF stops polling', async () => {
    const { wrapper } = await mountShell()
    const stream = StubEventSource.instances[0]!
    expect(stream.closed).toBe(false)
    wrapper.unmount()
    expect(stream.closed).toBe(true)
  })

  it('tells a guest plainly what they can and cannot do', async () => {
    const { wrapper } = await mountShell()
    expect(wrapper.text()).toContain('You are a viewer')
    expect(wrapper.text()).toContain('every control is locked')
    expect(wrapper.find('a[href="/sign-in"]').exists()).toBe(true)
  })

  it('shows the Grafana and Jaeger links the BFF supplied, and no others', async () => {
    const { wrapper } = await mountShell()
    const external = wrapper.findAll('a[target="_blank"]')
    expect(external.map((a) => a.attributes('href'))).toEqual([
      'https://grafana.test',
      'https://jaeger.test',
    ])
    for (const link of external) {
      expect(link.attributes('rel')).toContain('noopener')
    }
  })

  it('hides an external link the environment did not configure', async () => {
    stubFetch({ ...SESSION_BODY, links: { grafana: null, jaeger: null } })
    const { wrapper } = await mountShell()
    expect(wrapper.findAll('a[target="_blank"]')).toHaveLength(0)
  })

  it('switches between the enamel and control-room finishes', async () => {
    const { wrapper } = await mountShell()
    const before = document.documentElement.classList.contains('control-room')
    const toggle = wrapper
      .findAll('button')
      .find((b) => b.text() === 'Control room' || b.text() === 'Enamel')!
    await toggle.trigger('click')
    expect(document.documentElement.classList.contains('control-room')).toBe(!before)
  })

  it('reports the live-feed state, dark before the stream opens', async () => {
    const { wrapper } = await mountShell()
    expect(wrapper.text()).toContain('live feed')
    // Nothing has arrived, so the last-frame readout must say so.
    expect(wrapper.text()).toContain('no data')
  })

  it('prints the 1944 epigraph, and says the system is not nonsense', async () => {
    const { wrapper } = await mountShell()
    expect(wrapper.text()).toContain('prefabulated amulite')
    expect(wrapper.text()).toContain('The names are nonsense; the system is not')
  })
})
