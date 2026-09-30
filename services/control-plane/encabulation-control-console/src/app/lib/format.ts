/**
 * Formatting, with the console's one non-negotiable rule built in: a `null`
 * measurement formats as "no data", never as "0", "-", or a blank.
 *
 * Every number the panel prints goes through one of these. That is what makes
 * "no Math.random on screen" verifiable rather than aspirational: there is no
 * code path from an absent upstream to a plausible-looking figure.
 */

import type { Measure } from '@shared/topology'

export const NO_DATA = 'no data'

export function isMissing(value: Measure): boolean {
  return value === null || !Number.isFinite(value)
}

function fixed(value: number, digits: number): string {
  return value.toLocaleString('en-US', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })
}

/** Requests per second. */
export function rate(value: Measure, digits = 1): string {
  if (isMissing(value)) return NO_DATA
  const n = value as number
  if (n === 0) return '0'
  if (n < 0.1) return '<0.1'
  return fixed(n, digits)
}

/** Milliseconds, with the unit. */
export function millis(value: Measure): string {
  if (isMissing(value)) return NO_DATA
  const n = value as number
  if (n < 1) return `${fixed(n, 2)} ms`
  if (n < 100) return `${fixed(n, 1)} ms`
  if (n < 1000) return `${Math.round(n)} ms`
  return `${fixed(n / 1000, 2)} s`
}

/** A 0..1 ratio, as a percentage. */
export function percent(value: Measure, digits = 1): string {
  if (isMissing(value)) return NO_DATA
  const n = (value as number) * 100
  if (n > 0 && n < 0.1) return '<0.1%'
  return `${fixed(n, n >= 10 ? 0 : digits)}%`
}

/** A plain count. */
export function count(value: Measure): string {
  if (isMissing(value)) return NO_DATA
  return Math.round(value as number).toLocaleString('en-US')
}

/** A number with a fixed number of decimals. */
export function decimal(value: Measure, digits = 2): string {
  if (isMissing(value)) return NO_DATA
  return fixed(value as number, digits)
}

/** US dollars, as the quoter reports them. */
export function money(value: Measure, currency = 'USD'): string {
  if (isMissing(value)) return NO_DATA
  return (value as number).toLocaleString('en-US', { style: 'currency', currency })
}

/** Clock time, local, seconds included: operators read these off a wall clock. */
export function clock(iso: string | null): string {
  if (iso === null || iso === '') return NO_DATA
  const when = Date.parse(iso)
  if (!Number.isFinite(when)) return NO_DATA
  return new Date(when).toLocaleTimeString('en-US', { hour12: false })
}

/** "4s ago", "2m ago". Absent timestamps stay absent. */
export function ago(iso: string | null, now = Date.now()): string {
  if (iso === null || iso === '') return NO_DATA
  const when = Date.parse(iso)
  if (!Number.isFinite(when)) return NO_DATA
  const seconds = Math.max(0, Math.round((now - when) / 1000))
  if (seconds < 2) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

/** A countdown to an expiry, for malaise cards. */
export function until(iso: string | null, now = Date.now()): string {
  if (iso === null || iso === '') return NO_DATA
  const when = Date.parse(iso)
  if (!Number.isFinite(when)) return NO_DATA
  const seconds = Math.round((when - now) / 1000)
  if (seconds <= 0) return 'expired'
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${String(seconds % 60).padStart(2, '0')}s`
}

/** Title-case a kebab-case key for a label. */
export function humanise(key: string): string {
  return key
    .split(/[-_]/)
    .filter((part) => part !== '')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
