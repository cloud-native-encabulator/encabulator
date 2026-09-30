/**
 * A deterministic pseudo-random generator, for visual jitter only.
 *
 * The console shows no invented numbers, so `Math.random` is banned outright
 * (see the ESLint rule and `honesty.test.ts`). But a particle stream that rides
 * exactly down the centre line of an edge looks like a dashed border rather than
 * traffic, so particles get a small lateral offset and speed variation.
 *
 * Those come from here: seeded per edge, so the same edge always produces the
 * same texture, the same frame is reproducible in a test, and nothing on screen
 * that represents a measurement has a random component.
 *
 * mulberry32, by Tommy Ettinger. Small, fast, good enough for jitter.
 */

export interface Prng {
  /** [0, 1) */
  next(): number
  /** [min, max) */
  between(min: number, max: number): number
  /** A symmetric offset in [-spread, spread). */
  spread(spread: number): number
}

export function hashString(text: string): number {
  // FNV-1a 32-bit, the same hash the girdlespring uses for rollouts.
  let hash = 0x811c9dc5
  for (let i = 0; i < text.length; i += 1) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return hash >>> 0
}

export function createPrng(seed: number | string): Prng {
  let a = (typeof seed === 'string' ? hashString(seed) : seed) >>> 0
  const next = (): number => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
  return {
    next,
    between(min, max) {
      return min + next() * (max - min)
    },
    spread(s) {
      return (next() * 2 - 1) * s
    },
  }
}
