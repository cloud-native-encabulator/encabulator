/**
 * The honesty guards.
 *
 * These are the tests that make the claims on the front of this console
 * checkable rather than rhetorical. Each one states a rule from the brief and
 * then proves the source tree obeys it.
 */

import { readFileSync, readdirSync, statSync } from 'node:fs'
import { extname, join, relative, resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { NO_DATA, ago, clock, count, decimal, millis, money, percent, rate, until } from '@/lib/format'
import { LEXICON } from '@/lib/lexicon'
import { createPrng, hashString } from '@/lib/prng'

// Vitest runs from the service root; happy-dom's `import.meta.url` is not a
// file URL, so the tree is located from the working directory instead.
const SRC = resolve(process.cwd(), 'src')

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      walk(full, out)
      continue
    }
    out.push(full)
  }
  return out
}

const SOURCE_FILES = walk(SRC).filter((file) => {
  const ext = extname(file)
  if (!['.ts', '.vue'].includes(ext)) return false
  // Tests and the dev-only fixtures are allowed to say whatever they like.
  return !file.includes('__tests__') && !file.endsWith('.test.ts')
})

describe('no invented numbers', () => {
  it('never calls Math.random in the shipped source', () => {
    // A call, not the phrase: the comments in format.ts and prng.ts explain why
    // the rule exists, and should keep doing so.
    const CALL = /Math\s*\.\s*random\s*\(/
    const offenders = SOURCE_FILES.filter((file) => CALL.test(readFileSync(file, 'utf8'))).map(
      (file) => relative(SRC, file),
    )
    expect(offenders).toEqual([])
  })

  it('has a deterministic generator for visual jitter, seeded by name', () => {
    const a = createPrng('dingle-arm-gateway->panametric-fam-processor')
    const b = createPrng('dingle-arm-gateway->panametric-fam-processor')
    const c = createPrng('dingle-arm-gateway->spurving-bearing-quoter')
    const first = [a.next(), a.next(), a.next()]
    expect([b.next(), b.next(), b.next()]).toEqual(first)
    expect([c.next(), c.next(), c.next()]).not.toEqual(first)
  })

  it('agrees with the girdlespring on FNV-1a, so rollouts are reproducible', () => {
    // Known FNV-1a 32-bit values.
    expect(hashString('')).toBe(0x811c9dc5)
    expect(hashString('a')).toBe(0xe40c292c)
    expect(hashString('foobar')).toBe(0xbf9cf968)
  })
})

describe('no hard-coded hosts in the shipped SPA', () => {
  const APP = walk(join(SRC, 'app')).filter(
    (f) => ['.ts', '.vue'].includes(extname(f)) && !f.includes('__tests__'),
  )

  it('never mentions localhost or a 127.0.0.1 URL', () => {
    const offenders = APP.filter((file) => {
      const text = readFileSync(file, 'utf8')
      return text.includes('localhost') || text.includes('127.0.0.1')
    }).map((file) => relative(SRC, file))
    expect(offenders).toEqual([])
  })

  it('never builds an absolute http(s) URL to an upstream', () => {
    // External links (Grafana, Jaeger) come from the BFF at runtime, so no
    // http:// literal should appear in the app source at all.
    const offenders = APP.filter((file) => /https?:\/\/[a-z0-9]/i.test(readFileSync(file, 'utf8')))
      .map((file) => relative(SRC, file))
    expect(offenders).toEqual([])
  })

  it('reaches the BFF by a relative path', async () => {
    const { BFF, STREAM_URL } = await import('@/api/client')
    expect(BFF).toBe('/bff')
    expect(STREAM_URL).toBe('/bff/stream')
    expect(STREAM_URL.startsWith('/')).toBe(true)
  })
})

describe('formatting refuses to fill in a blank', () => {
  it('renders every absent measurement as "no data"', () => {
    expect(rate(null)).toBe(NO_DATA)
    expect(millis(null)).toBe(NO_DATA)
    expect(percent(null)).toBe(NO_DATA)
    expect(count(null)).toBe(NO_DATA)
    expect(decimal(null)).toBe(NO_DATA)
    expect(money(null)).toBe(NO_DATA)
    expect(clock(null)).toBe(NO_DATA)
    expect(ago(null)).toBe(NO_DATA)
    expect(until(null)).toBe(NO_DATA)
  })

  it('treats NaN and Infinity as absent, not as numbers', () => {
    expect(rate(Number.NaN)).toBe(NO_DATA)
    expect(millis(Number.POSITIVE_INFINITY)).toBe(NO_DATA)
    expect(count(Number.NaN)).toBe(NO_DATA)
  })

  it('distinguishes a measured zero from an absent reading', () => {
    expect(rate(0)).toBe('0')
    expect(count(0)).toBe('0')
    expect(percent(0)).toBe('0.0%')
  })

  it('does not round a small non-zero down to zero', () => {
    expect(rate(0.04)).toBe('<0.1')
    expect(percent(0.0004)).toBe('<0.1%')
  })

  it('picks sensible units without lying about precision', () => {
    expect(millis(0.4)).toBe('0.40 ms')
    expect(millis(42.35)).toBe('42.4 ms')
    expect(millis(420)).toBe('420 ms')
    expect(millis(4200)).toBe('4.20 s')
  })

  it('renders a relative time without a clock skew surprise', () => {
    const now = Date.parse('2026-09-30T12:00:00.000Z')
    expect(ago('2026-09-30T11:59:58.000Z', now)).toBe('2s ago')
    expect(ago('2026-09-30T11:55:00.000Z', now)).toBe('5m ago')
    // A timestamp in the future is not negative; it is "just now".
    expect(ago('2026-09-30T12:00:30.000Z', now)).toBe('just now')
  })

  it('counts a malaise down to expiry and then says expired', () => {
    const now = Date.parse('2026-09-30T12:00:00.000Z')
    expect(until('2026-09-30T12:01:30.000Z', now)).toBe('1m 30s')
    expect(until('2026-09-30T12:00:05.000Z', now)).toBe('5s')
    expect(until('2026-09-30T11:59:00.000Z', now)).toBe('expired')
  })
})

describe('the lexicon', () => {
  it('pairs every joke term with a real, non-empty explanation', () => {
    const entries = Object.entries(LEXICON)
    expect(entries.length).toBeGreaterThan(15)
    for (const [key, lexeme] of entries) {
      expect(lexeme.term.length, key).toBeGreaterThan(3)
      expect(lexeme.real.length, key).toBeGreaterThan(8)
      // The explanation must not just restate the joke.
      expect(lexeme.real.toLowerCase(), key).not.toBe(lexeme.term.toLowerCase())
    }
  })

  it('names a real metric wherever it claims a source', () => {
    for (const [key, lexeme] of Object.entries(LEXICON)) {
      if (lexeme.source === undefined) continue
      expect(lexeme.source.length, key).toBeGreaterThan(8)
    }
  })

  it('explains the term the brief calls out by name', () => {
    expect(LEXICON['skorMotion']?.term).toBe('Fluorescent skor motion')
    expect(LEXICON['skorMotion']?.real).toBe('Requests per second')
  })
})
