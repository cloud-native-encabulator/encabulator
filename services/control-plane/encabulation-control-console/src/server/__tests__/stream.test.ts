/**
 * The live stream.
 *
 * Two design promises are tested here that matter operationally:
 *   - upstream polling runs only while a client is connected, so an idle
 *     console adds no load to the factory;
 *   - one poll is fanned out to every client, and a client attaching late gets
 *     the cached snapshot at once instead of a blank panel.
 */

import { describe, expect, it, vi } from 'vitest'
import { Collector, EVENT_FEED, EVENT_HEADLINE, EVENT_TOPOLOGY } from '../collector.js'
import { SseHub, formatSse, type SseSink } from '../hub.js'
import { consoleMetrics } from '../platform/index.js'
import {
  EMPTY_CLUSTER,
  SILENT_LOGGER,
  fakeCluster,
  fakeGateway,
  fakePrometheus,
  harness,
  testConfig,
} from './helpers.js'

function recordingSink(): SseSink & { frames: string[]; closedFlag: boolean } {
  const frames: string[] = []
  return {
    frames,
    closedFlag: false,
    write(chunk: string) {
      frames.push(chunk)
      return true
    },
    end() {
      this.closedFlag = true
    },
    get closed() {
      return this.closedFlag
    },
  }
}

function parseFrames(frames: string[]): { event: string; data: unknown }[] {
  return frames
    .filter((f) => f.startsWith('id:') || f.startsWith('event:'))
    .map((frame) => {
      const eventLine = frame.split('\n').find((l) => l.startsWith('event: '))
      const dataLine = frame.split('\n').find((l) => l.startsWith('data: '))
      return {
        event: eventLine?.slice('event: '.length) ?? '',
        data: dataLine === undefined ? null : JSON.parse(dataLine.slice('data: '.length)),
      }
    })
}

describe('SSE framing', () => {
  it('emits a well-formed event with an id and a single data line', () => {
    const frame = formatSse({ event: 'topology', data: { at: 'now' }, id: '7' })
    expect(frame).toBe('id: 7\nevent: topology\ndata: {"at":"now"}\n\n')
  })
})

describe('the hub', () => {
  it('starts polling on the first client and stops on the last', () => {
    const start = vi.fn()
    const stop = vi.fn()
    const hub = new SseHub({ onFirstClient: start, onLastClient: stop })

    const a = recordingSink()
    const b = recordingSink()
    hub.add(a)
    hub.add(b)
    expect(start).toHaveBeenCalledTimes(1)
    expect(stop).not.toHaveBeenCalled()

    hub.remove(a)
    expect(stop).not.toHaveBeenCalled()
    hub.remove(b)
    expect(stop).toHaveBeenCalledTimes(1)

    // Re-attaching starts it again.
    hub.add(recordingSink())
    expect(start).toHaveBeenCalledTimes(2)
  })

  it('fans one payload out to every client', () => {
    const hub = new SseHub()
    const a = recordingSink()
    const b = recordingSink()
    hub.add(a)
    hub.add(b)
    const written = hub.broadcast('topology', { nodes: [] })
    expect(written).toBe(2)
    expect(parseFrames(a.frames)).toEqual(parseFrames(b.frames))
  })

  it('replays the cached snapshot to a client that attaches later', () => {
    const hub = new SseHub({ replayEvents: [EVENT_TOPOLOGY, EVENT_HEADLINE] })
    hub.broadcast(EVENT_TOPOLOGY, { at: 'first' })
    const late = recordingSink()
    hub.add(late)
    const events = parseFrames(late.frames)
    expect(events).toHaveLength(1)
    expect(events[0]).toEqual({ event: EVENT_TOPOLOGY, data: { at: 'first' } })
  })

  it('tells the browser how long to wait before reconnecting', () => {
    const hub = new SseHub()
    const sink = recordingSink()
    hub.add(sink)
    expect(sink.frames[0]).toBe('retry: 3000\n\n')
  })

  it('drops a client whose socket has gone and stops polling for it', () => {
    const stop = vi.fn()
    const hub = new SseHub({ onLastClient: stop })
    const sink = recordingSink()
    hub.add(sink)
    sink.closedFlag = true
    hub.broadcast('topology', {})
    expect(hub.size).toBe(0)
    expect(stop).toHaveBeenCalledTimes(1)
  })

  it('sends a heartbeat comment so idle proxies do not close the stream', () => {
    const hub = new SseHub()
    const sink = recordingSink()
    hub.add(sink)
    hub.heartbeat()
    expect(sink.frames.at(-1)).toMatch(/^: ping /)
  })
})

describe('the collector', () => {
  function build() {
    const http = fakeGateway()
    http.on('/v1/stats', {
      body: { byStatus: { fabricating: 4, refabricating: 1 }, throughput: { ordersAccepted: 25, unitsDelivered: 9 } },
    })
    http.on('/v1/parts', { body: [{ sku: 'AMU-BASE', name: 'Base-plate', stock: 700, reserved: 3, perUnit: 1 }] })
    http.on('/v1/orders', { body: { orders: [{ id: 'ord_1', status: 'fabricating', model: 'turbo', quantity: 2, customer: 'buyer', updatedAt: '2026-09-30T10:00:00.000Z' }] } })
    http.on('/v1/incidents', { body: { incidents: [] } })
    http.on('/v1/shipments', { body: { shipments: [] } })
    http.on('/v1/runs', { body: { runs: [] } })
    http.on('/v1/summary', { body: { metrics: { repleneration: { mean: 1.02 } } } })

    const collector = new Collector({
      config: testConfig(),
      logger: SILENT_LOGGER,
      metrics: consoleMetrics,
      gateway: http,
      prometheus: fakePrometheus(),
      cluster: fakeCluster(EMPTY_CLUSTER),
    })
    return { collector, http }
  }

  it('does not touch an upstream until a client attaches', async () => {
    const { collector, http } = build()
    expect(collector.isRunning).toBe(false)
    expect(http.calls).toEqual([])
    collector.close()
  })

  it('polls while a client is attached and stops when it leaves', async () => {
    const { collector, http } = build()
    const sink = recordingSink()
    collector.hub.add(sink)
    expect(collector.isRunning).toBe(true)

    // The first tick is immediate, not one interval later.
    await vi.waitFor(() => expect(http.calls.length).toBeGreaterThan(0))

    collector.hub.remove(sink)
    expect(collector.isRunning).toBe(false)
    const after = http.calls.length
    await new Promise((r) => setTimeout(r, 30))
    expect(http.calls.length).toBe(after)
    collector.close()
  })

  it('summarises the headline gauges from the ledger, detector, depot and recorder', async () => {
    const { collector } = build()
    const headline = await collector.buildHeadlineStats()
    expect(headline.ordersPerMin).toBe(5) // 25 accepted over five minutes
    expect(headline.unitsInFabrication).toBe(5)
    expect(headline.unitsDelivered).toBe(9)
    expect(headline.partsTotal).toBe(700)
    expect(headline.partsLowestSku).toBe('AMU-BASE')
    expect(headline.replenerationMean).toBeCloseTo(1.02)
    collector.close()
  })

  it('reports null and a reason when an upstream is down, never a zero', async () => {
    const { collector, http } = build()
    http.fail('datastore', 'connect ECONNREFUSED')
    const headline = await collector.buildHeadlineStats()
    expect(headline.ordersPerMin).toBeNull()
    expect(headline.unitsInFabrication).toBeNull()
    expect(headline.sources.datastore?.ok).toBe(false)
    expect(headline.sources.datastore?.detail).toContain('ECONNREFUSED')
    collector.close()
  })

  it('does not resend a feed row it has already sent', async () => {
    const { collector } = build()
    const first = await collector.buildFeed()
    expect(first.batch.items.map((i) => i.kind)).toContain('order')
    const second = await collector.buildFeed()
    expect(second.batch.items).toEqual([])
    collector.close()
  })

  it('never retries a polling call, so a slow upstream cannot pile up', async () => {
    const { collector, http } = build()
    await collector.buildFeed()
    expect(http.calls.every((c) => c.method === 'GET')).toBe(true)
    expect(http.calls.every((c) => c.retries === 0)).toBe(true)
    // One call per endpoint, not three.
    const orderCalls = http.calls.filter((c) => c.path.includes('/v1/orders'))
    expect(orderCalls).toHaveLength(1)
    collector.close()
  })
})

describe('GET /bff/stream', () => {
  it('opens an event stream with buffering disabled for proxies', async () => {
    const h = await harness()
    try {
      const res = await h.app.inject({ method: 'GET', url: '/bff/stream', payloadAsStream: true })
      expect(res.statusCode).toBe(200)
      expect(res.headers['content-type']).toContain('text/event-stream')
      expect(res.headers['cache-control']).toContain('no-transform')
      expect(res.headers['x-accel-buffering']).toBe('no')
      res.stream().destroy()
    } finally {
      await h.close()
    }
  })

  it('is readable by a guest, because watching the factory is public', async () => {
    const h = await harness()
    try {
      const res = await h.app.inject({ method: 'GET', url: '/bff/topology' })
      expect(res.statusCode).toBe(200)
      expect(res.json().nodes).toHaveLength(19)
    } finally {
      await h.close()
    }
  })

  it('serves the cached snapshot from /bff/topology once one exists', async () => {
    const h = await harness()
    try {
      h.collector.hub.broadcast(EVENT_TOPOLOGY, { at: 'cached', nodes: [], edges: [] })
      const res = await h.app.inject({ method: 'GET', url: '/bff/topology' })
      expect(res.json().at).toBe('cached')
    } finally {
      await h.close()
    }
  })

  it('reports the stream client count as a metric', async () => {
    const h = await harness()
    try {
      const sink = recordingSink()
      h.collector.hub.add(sink)
      const gauge = await h.metrics.read('encabulator_console_stream_clients')
      expect(gauge?.values[0]?.value).toBe(1)
      h.collector.hub.remove(sink)
      const after = await h.metrics.read('encabulator_console_stream_clients')
      expect(after?.values[0]?.value).toBe(0)
    } finally {
      await h.close()
    }
  })
})

describe('event names the SPA depends on', () => {
  it('are the ones the design names, plus the headline gauges', () => {
    expect([EVENT_TOPOLOGY, EVENT_FEED, EVENT_HEADLINE]).toEqual(['topology', 'feed', 'headline'])
  })
})
