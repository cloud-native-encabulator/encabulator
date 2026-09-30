/**
 * The particle engine and the layout geometry.
 *
 * The particle rules are the animated half of the honesty contract: an edge with
 * no measured rate emits nothing, spawn is proportional to the measured rate, and
 * the caps and the visibility pause keep a conference laptop's fans down.
 */

import { describe, expect, it, vi } from 'vitest'
import {
  BASE_TRANSIT_SECONDS,
  MAX_PARTICLES,
  MAX_SPAWN_PER_EDGE,
  advance,
  buildEmitters,
  createParticleLoop,
  edgeKey,
  spawn,
  spawnRateFor,
  type Particle,
} from '@/lib/particles'
import {
  GRID_COLUMNS,
  bezierAt,
  edgeWidthScale,
  healthLabel,
  layoutEdges,
  layoutNodes,
  normalAt,
  podDots,
} from '@/lib/layout'
import { REGISTRY } from '@shared/registry'
import type { EdgeView, NodeView } from '@shared/topology'

const VIEW = { width: 1200, height: 660 }

function node(name: string, over: Partial<NodeView> = {}): NodeView {
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

function edge(source: string, target: string, over: Partial<EdgeView> = {}): EdgeView {
  return {
    source,
    target,
    via: 'http',
    rps: 10,
    errorRatio: 0,
    p50Ms: 20,
    p99Ms: 60,
    health: 'healthy',
    ...over,
  }
}

const NODES = [node('dingle-arm-gateway'), node('panametric-fam-processor'), node('valkey')]

describe('spawn rate', () => {
  it('emits nothing for an edge with no measured rate', () => {
    expect(spawnRateFor(null)).toBe(0)
  })

  it('emits nothing for a measured zero, so an idle edge is visibly idle', () => {
    expect(spawnRateFor(0)).toBe(0)
  })

  it('is proportional to the measured rate', () => {
    expect(spawnRateFor(2)).toBeCloseTo(2.8)
    expect(spawnRateFor(4)).toBeCloseTo(5.6)
  })

  it('caps a single edge, so one hot line cannot starve the panel', () => {
    expect(spawnRateFor(1_000_000)).toBe(MAX_SPAWN_PER_EDGE)
  })

  it('treats a NaN rate as no data', () => {
    expect(spawnRateFor(Number.NaN)).toBe(0)
  })
})

describe('spawning', () => {
  const placed = layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor')], layoutNodes(NODES, VIEW), VIEW)

  it('accumulates fractional rates rather than dropping them', () => {
    const emitters = buildEmitters(
      layoutEdges(
        [edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 0.2 })],
        layoutNodes(NODES, VIEW),
        VIEW,
      ),
    )
    const list = [...emitters.values()]
    // 0.2 rps × 1.4 = 0.28/s, so nothing in a 16 ms frame…
    expect(spawn(list, 0.016, 0)).toHaveLength(0)
    // …but one after four seconds' worth of frames.
    let born = 0
    for (let i = 0; i < 250; i += 1) born += spawn(list, 0.016, 0).length
    expect(born).toBeGreaterThan(0)
  })

  it('never exceeds the global particle cap', () => {
    const emitters = buildEmitters(
      layoutEdges(
        [edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 500 })],
        layoutNodes(NODES, VIEW),
        VIEW,
      ),
    )
    const born = spawn([...emitters.values()], 10, MAX_PARTICLES - 3)
    expect(born.length).toBeLessThanOrEqual(3)
    expect(spawn([...emitters.values()], 10, MAX_PARTICLES)).toHaveLength(0)
  })

  it('gives each particle the edge’s health, so colour is a reading', () => {
    const emitters = buildEmitters(
      layoutEdges(
        [edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 50, health: 'faulted' })],
        layoutNodes(NODES, VIEW),
        VIEW,
      ),
    )
    const born = spawn([...emitters.values()], 1, 0)
    expect(born.length).toBeGreaterThan(0)
    expect(born.every((p) => p.health === 'faulted')).toBe(true)
  })

  it('is reproducible for the same edge, because the jitter is seeded', () => {
    const build = () =>
      buildEmitters(
        layoutEdges(
          [edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 20 })],
          layoutNodes(NODES, VIEW),
          VIEW,
        ),
      )
    const a = spawn([...build().values()], 1, 0)
    const b = spawn([...build().values()], 1, 0)
    expect(a.map((p) => [p.speed, p.offset])).toEqual(b.map((p) => [p.speed, p.offset]))
  })

  it('applies the adaptive multiplier', () => {
    const emitters = buildEmitters(
      layoutEdges(
        [edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 10 })],
        layoutNodes(NODES, VIEW),
        VIEW,
      ),
    )
    const full = spawn([...emitters.values()], 1, 0, 1).length
    const throttled = spawn([...buildEmitters(placed).values()], 1, 0, 0.25).length
    expect(throttled).toBeLessThan(full)
  })
})

describe('advancing', () => {
  function particle(over: Partial<Particle> = {}): Particle {
    return {
      edgeKey: 'a->b',
      t: 0,
      speed: 1 / BASE_TRANSIT_SECONDS,
      offset: 0,
      health: 'healthy',
      weight: 1,
      ...over,
    }
  }

  it('moves particles along and retires those that arrive', () => {
    const list = [particle({ t: 0 }), particle({ t: 0.95 })]
    advance(list, 0.5)
    expect(list).toHaveLength(1)
    expect(list[0]!.t).toBeGreaterThan(0)
  })

  it('leaves the array empty rather than holding dead particles', () => {
    const list = [particle({ t: 0.99 }), particle({ t: 0.995 })]
    advance(list, 1)
    expect(list).toHaveLength(0)
  })
})

describe('the animation loop', () => {
  function fakeCanvas() {
    const calls = { clear: 0, arcs: 0 }
    const ctx = {
      clearRect: () => {
        calls.clear += 1
      },
      beginPath: () => {},
      arc: () => {
        calls.arcs += 1
      },
      fill: () => {},
      globalAlpha: 1,
      fillStyle: '',
    }
    return {
      calls,
      canvas: { getContext: () => ctx, width: 1200, height: 660 } as unknown as HTMLCanvasElement,
    }
  }

  const colours = () => ({
    healthy: '#0f0',
    degraded: '#fa0',
    faulted: '#f00',
    nodata: '#888',
  })

  it('does not paint at all while the tab is hidden', () => {
    const { canvas, calls } = fakeCanvas()
    // Held on an object: a plain `let` assigned only inside the raf callback
    // narrows to `never` under strict TypeScript.
    const held: { frame: ((now: number) => void) | null } = { frame: null }
    const loop = createParticleLoop({
      canvas,
      colours,
      view: () => VIEW,
      hidden: () => true,
      raf: (cb) => {
        held.frame = cb
        return 1
      },
      cancel: () => {},
      now: () => 0,
    })
    loop.setEdges(layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor')], layoutNodes(NODES, VIEW), VIEW))
    loop.start()
    held.frame?.(16)
    expect(calls.clear).toBe(0)
    loop.destroy()
  })

  it('paints while visible and keeps requesting frames', () => {
    const { canvas, calls } = fakeCanvas()
    // Held on an object: a plain `let` assigned only inside the raf callback
    // narrows to `never` under strict TypeScript.
    const held: { frame: ((now: number) => void) | null } = { frame: null }
    const raf = vi.fn((cb: (now: number) => void) => {
      held.frame = cb
      return 1
    })
    const loop = createParticleLoop({
      canvas,
      colours,
      view: () => VIEW,
      hidden: () => false,
      raf,
      cancel: () => {},
      now: () => 0,
    })
    loop.setEdges(
      layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 30 })], layoutNodes(NODES, VIEW), VIEW),
    )
    loop.start()
    // Ten 50 ms frames: the per-edge cap is 24 particles a second, so a couple
    // of 16 ms frames would legitimately produce none.
    for (let i = 1; i <= 10; i += 1) held.frame?.(i * 50)
    expect(calls.clear).toBeGreaterThan(0)
    expect(calls.arcs).toBeGreaterThan(0)
    expect(raf.mock.calls.length).toBeGreaterThan(1)
    loop.destroy()
  })

  it('spawns nothing when the reader has asked for reduced motion', () => {
    const { canvas, calls } = fakeCanvas()
    // Held on an object: a plain `let` assigned only inside the raf callback
    // narrows to `never` under strict TypeScript.
    const held: { frame: ((now: number) => void) | null } = { frame: null }
    const loop = createParticleLoop({
      canvas,
      colours,
      view: () => VIEW,
      hidden: () => false,
      reducedMotion: () => true,
      raf: (cb) => {
        held.frame = cb
        return 1
      },
      cancel: () => {},
      now: () => 0,
    })
    loop.setEdges(
      layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 500 })], layoutNodes(NODES, VIEW), VIEW),
    )
    loop.start()
    for (let i = 1; i <= 10; i += 1) held.frame?.(i * 50)
    expect(loop.count).toBe(0)
    // It still clears and draws, so the canvas is not left with a stale frame.
    expect(calls.clear).toBeGreaterThan(0)
    loop.destroy()
  })

  it('retires particles whose edge disappears from the next snapshot', () => {
    const { canvas } = fakeCanvas()
    // Held on an object: a plain `let` assigned only inside the raf callback
    // narrows to `never` under strict TypeScript.
    const held: { frame: ((now: number) => void) | null } = { frame: null }
    const loop = createParticleLoop({
      canvas,
      colours,
      view: () => VIEW,
      hidden: () => false,
      raf: (cb) => {
        held.frame = cb
        return 1
      },
      cancel: () => {},
      now: () => 0,
    })
    const placed = layoutNodes(NODES, VIEW)
    loop.setEdges(layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor', { rps: 100 })], placed, VIEW))
    loop.start()
    for (let i = 1; i <= 10; i += 1) held.frame?.(i * 50)
    expect(loop.count).toBeGreaterThan(0)
    loop.setEdges([])
    expect(loop.count).toBe(0)
    loop.destroy()
  })

  it('reports itself stopped after stop(), so the view can pause it', () => {
    const { canvas } = fakeCanvas()
    const loop = createParticleLoop({
      canvas,
      colours,
      view: () => VIEW,
      hidden: () => false,
      raf: () => 7,
      cancel: () => {},
      now: () => 0,
    })
    loop.start()
    expect(loop.running).toBe(true)
    loop.stop()
    expect(loop.running).toBe(false)
    loop.destroy()
  })
})

describe('layout', () => {
  it('places every node deterministically, twice the same', () => {
    const a = layoutNodes(NODES, VIEW)
    const b = layoutNodes(NODES, VIEW)
    expect(a.map((p) => p.box)).toEqual(b.map((p) => p.box))
  })

  it('keeps every plate inside the drawing surface', () => {
    for (const placed of layoutNodes(REGISTRY.map((r) => node(r.name)), VIEW)) {
      expect(placed.box.x, placed.node.name).toBeGreaterThanOrEqual(0)
      expect(placed.box.x + placed.box.width, placed.node.name).toBeLessThanOrEqual(VIEW.width)
      expect(placed.box.y, placed.node.name).toBeGreaterThanOrEqual(0)
      expect(placed.box.y + placed.box.height, placed.node.name).toBeLessThanOrEqual(VIEW.height)
    }
  })

  it('uses a twelve-column grid, as the registry assumes', () => {
    expect(GRID_COLUMNS).toBe(12)
  })

  it('drops an edge whose endpoint is not on the panel', () => {
    const placed = layoutNodes([node('dingle-arm-gateway')], VIEW)
    expect(layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor')], placed, VIEW)).toEqual([])
  })

  it('scales stroke width by rate, and gives a no-data edge the thinnest line', () => {
    const scale = edgeWidthScale([edge('a', 'b', { rps: 100 }), edge('c', 'd', { rps: 1 })])
    expect(scale(0)).toBeCloseTo(1)
    expect(scale(100)).toBeGreaterThan(scale(1))
  })

  it('evaluates the curve at both ends exactly', () => {
    const placed = layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor')], layoutNodes(NODES, VIEW), VIEW)[0]!
    expect(bezierAt(placed, 0)).toEqual(placed.from)
    expect(bezierAt(placed, 1)).toEqual(placed.to)
  })

  it('produces a unit normal along the curve', () => {
    const placed = layoutEdges([edge('dingle-arm-gateway', 'panametric-fam-processor')], layoutNodes(NODES, VIEW), VIEW)[0]!
    for (const t of [0.1, 0.5, 0.9]) {
      const n = normalAt(placed, t)
      expect(Math.hypot(n.x, n.y)).toBeCloseTo(1, 5)
    }
  })

  it('lays pod dots inside the plate and caps how many it draws', () => {
    const box = layoutNodes(NODES, VIEW)[0]!.box
    expect(podDots(box, 0)).toEqual([])
    const many = podDots(box, 400)
    expect(many.length).toBeLessThanOrEqual(20)
    for (const dot of many) {
      expect(dot.x).toBeGreaterThanOrEqual(box.x)
      expect(dot.x).toBeLessThanOrEqual(box.x + box.width)
      expect(dot.y).toBeLessThanOrEqual(box.y + box.height)
    }
  })

  it('names the health bands in words an operator can read aloud', () => {
    expect(healthLabel('healthy')).toBe('in tolerance')
    expect(healthLabel('nodata')).toBe('no data')
  })

  it('keys an edge stably', () => {
    expect(edgeKey('a', 'b')).toBe('a->b')
  })
})
