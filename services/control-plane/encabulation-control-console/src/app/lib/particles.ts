/**
 * The particle engine behind the Factory Floor.
 *
 * What the particles mean: one particle leaving a plate is a batch of requests.
 * Spawn rate is proportional to the edge's measured requests per second, colour
 * is the edge's health, and an edge with no data emits nothing at all. The
 * animation is therefore a reading, not decoration, which is why it lives in its
 * own tested module rather than inside a component.
 *
 * Performance rules, from the brief:
 *   - one `requestAnimationFrame` loop drawing to a canvas, never a DOM update
 *     per particle;
 *   - a hard cap on live particles, and a per-edge cap so one hot edge cannot
 *     starve the rest;
 *   - nothing runs while the tab is hidden;
 *   - if frames get long, the spawn multiplier backs off rather than the frame
 *     rate collapsing.
 */

import type { Health } from '@shared/topology'
import { createPrng, type Prng } from './prng.js'
import { bezierAt, normalAt, type PlacedEdge } from './layout.js'

/** Total particles alive across the whole panel. */
export const MAX_PARTICLES = 900
/** Particles per second a single edge may emit, however busy it is. */
export const MAX_SPAWN_PER_EDGE = 24
/** Particles emitted per request per second. */
export const PARTICLES_PER_RPS = 1.4
/** Seconds a particle takes to traverse an edge, before speed jitter. */
export const BASE_TRANSIT_SECONDS = 1.6

export interface Particle {
  edgeKey: string
  /** 0..1 along the edge. */
  t: number
  /** Progress per second. */
  speed: number
  /** Lateral offset in pixels. */
  offset: number
  health: Health
  /** 0..1, used for size and alpha so a stream has depth. */
  weight: number
}

export interface EdgeEmitter {
  key: string
  placed: PlacedEdge
  /** Particles per second this edge should emit right now. */
  spawnPerSecond: number
  health: Health
  prng: Prng
  /** Fractional carry-over, so a 0.3/s edge still emits. */
  accumulator: number
}

export function edgeKey(source: string, target: string): string {
  return `${source}->${target}`
}

/** Particles per second for a measured rate. Null or zero emits nothing. */
export function spawnRateFor(rps: number | null): number {
  if (rps === null || !Number.isFinite(rps) || rps <= 0) return 0
  return Math.min(MAX_SPAWN_PER_EDGE, rps * PARTICLES_PER_RPS)
}

export function buildEmitters(edges: PlacedEdge[]): Map<string, EdgeEmitter> {
  const out = new Map<string, EdgeEmitter>()
  for (const placed of edges) {
    const key = edgeKey(placed.edge.source, placed.edge.target)
    out.set(key, {
      key,
      placed,
      spawnPerSecond: spawnRateFor(placed.edge.rps),
      health: placed.edge.health,
      prng: createPrng(key),
      accumulator: 0,
    })
  }
  return out
}

/**
 * Carry the emitters forward by `dt` seconds and return the particles spawned.
 * Pure apart from mutating each emitter's accumulator, which is what makes a
 * fractional spawn rate work across frames.
 */
export function spawn(
  emitters: Iterable<EdgeEmitter>,
  dt: number,
  live: number,
  multiplier = 1,
): Particle[] {
  const born: Particle[] = []
  let budget = MAX_PARTICLES - live
  if (budget <= 0) return born

  for (const emitter of emitters) {
    if (emitter.spawnPerSecond <= 0) {
      emitter.accumulator = 0
      continue
    }
    emitter.accumulator += emitter.spawnPerSecond * multiplier * dt
    while (emitter.accumulator >= 1 && budget > 0) {
      emitter.accumulator -= 1
      budget -= 1
      const jitter = emitter.prng
      born.push({
        edgeKey: emitter.key,
        t: 0,
        speed: (1 / BASE_TRANSIT_SECONDS) * jitter.between(0.8, 1.25),
        offset: jitter.spread(3.2),
        health: emitter.health,
        weight: jitter.between(0.55, 1),
      })
    }
    // Never let an idle frame bank more than one particle's worth of credit.
    if (emitter.accumulator > 1) emitter.accumulator = 1
  }
  return born
}

/** Advance particles, dropping those that have arrived. Mutates in place. */
export function advance(particles: Particle[], dt: number): Particle[] {
  let write = 0
  for (let read = 0; read < particles.length; read += 1) {
    const p = particles[read] as Particle
    p.t += p.speed * dt
    if (p.t < 1) {
      particles[write] = p
      write += 1
    }
  }
  particles.length = write
  return particles
}

export interface ParticleColours {
  healthy: string
  degraded: string
  faulted: string
  nodata: string
}

/** Draw one frame. Kept separate from the loop so a test can call it. */
export function draw(
  ctx: CanvasRenderingContext2D,
  particles: Particle[],
  emitters: Map<string, EdgeEmitter>,
  colours: ParticleColours,
  view: { width: number; height: number },
): void {
  ctx.clearRect(0, 0, view.width, view.height)
  for (const p of particles) {
    const emitter = emitters.get(p.edgeKey)
    if (emitter === undefined) continue
    const point = bezierAt(emitter.placed, p.t)
    const normal = normalAt(emitter.placed, p.t)
    const x = point.x + normal.x * p.offset
    const y = point.y + normal.y * p.offset
    const radius = 1 + p.weight * 1.6
    ctx.globalAlpha = 0.35 + p.weight * 0.5
    ctx.fillStyle = colours[p.health]
    ctx.beginPath()
    ctx.arc(x, y, radius, 0, Math.PI * 2)
    ctx.fill()
  }
  ctx.globalAlpha = 1
}

/**
 * The animation loop. A thin shell over the functions above: it owns the frame
 * clock, the visibility check and the adaptive multiplier, and nothing else.
 */
export interface LoopHandles {
  /** Replace the emitter set when a new topology snapshot arrives. */
  setEdges(edges: PlacedEdge[]): void
  /** Current particle count, for the diagnostics readout. */
  readonly count: number
  /** Smoothed frame time in milliseconds. */
  readonly frameMs: number
  /** Current spawn multiplier; below 1 means the loop is protecting the frame rate. */
  readonly multiplier: number
  start(): void
  stop(): void
  readonly running: boolean
  destroy(): void
}

export interface LoopOptions {
  canvas: HTMLCanvasElement
  colours: () => ParticleColours
  view: () => { width: number; height: number }
  /** Defaults to `document.hidden`. */
  hidden?: () => boolean
  /** Honour prefers-reduced-motion by drawing statically. */
  reducedMotion?: () => boolean
  raf?: (cb: (now: number) => void) => number
  cancel?: (handle: number) => void
  now?: () => number
}

export function createParticleLoop(options: LoopOptions): LoopHandles {
  const raf = options.raf ?? ((cb) => requestAnimationFrame(cb))
  const cancel = options.cancel ?? ((h) => cancelAnimationFrame(h))
  const clock = options.now ?? (() => performance.now())
  const isHidden = options.hidden ?? (() => document.hidden)
  const reduced = options.reducedMotion ?? (() => false)

  let emitters = new Map<string, EdgeEmitter>()
  const particles: Particle[] = []
  let handle: number | null = null
  let last = 0
  let frameMs = 16
  let multiplier = 1

  function frame(now: number): void {
    handle = null
    if (isHidden()) {
      // Stop entirely; `start` is called again on visibilitychange.
      return
    }
    const rawDt = last === 0 ? 0.016 : (now - last) / 1000
    last = now
    // Clamp: a backgrounded tab that wakes up must not teleport every particle.
    const dt = Math.min(0.05, Math.max(0.001, rawDt))
    frameMs = frameMs * 0.9 + rawDt * 1000 * 0.1

    // Adaptive: back the spawn rate off if we are missing frames, recover slowly.
    if (frameMs > 26 && multiplier > 0.25) multiplier = Math.max(0.25, multiplier - 0.05)
    else if (frameMs < 19 && multiplier < 1) multiplier = Math.min(1, multiplier + 0.02)

    if (!reduced()) {
      particles.push(...spawn(emitters.values(), dt, particles.length, multiplier))
      advance(particles, dt)
    }

    const ctx = options.canvas.getContext('2d')
    if (ctx !== null) draw(ctx, particles, emitters, options.colours(), options.view())

    handle = raf(frame)
  }

  return {
    setEdges(edges) {
      const next = buildEmitters(edges)
      // Preserve accumulators for edges that survive, so streams do not stutter
      // every three seconds when a new snapshot lands.
      for (const [key, emitter] of next) {
        const previous = emitters.get(key)
        if (previous !== undefined) emitter.accumulator = previous.accumulator
      }
      emitters = next
      // Retire particles whose edge has gone.
      for (let i = particles.length - 1; i >= 0; i -= 1) {
        if (!emitters.has((particles[i] as Particle).edgeKey)) particles.splice(i, 1)
      }
    },
    get count() {
      return particles.length
    },
    get frameMs() {
      return frameMs
    },
    get multiplier() {
      return multiplier
    },
    start() {
      if (handle !== null) return
      last = clock()
      handle = raf(frame)
    },
    stop() {
      if (handle === null) return
      cancel(handle)
      handle = null
    },
    get running() {
      return handle !== null
    },
    destroy() {
      if (handle !== null) cancel(handle)
      handle = null
      particles.length = 0
      emitters.clear()
    },
  }
}
