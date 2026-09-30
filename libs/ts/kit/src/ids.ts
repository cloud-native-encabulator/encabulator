/**
 * Prefixed identifiers (design section 8): a prefix, an underscore, then a UUIDv7 so
 * ids sort by creation time.
 */

import { v7 as uuidv7 } from 'uuid';

export const ID_PREFIXES = {
  order: 'ord',
  quote: 'qte',
  reservation: 'rsv',
  malaise: 'mal',
  run: 'run',
  incident: 'inc',
  shipment: 'shp',
  event: 'evt',
} as const;

export type IdKind = keyof typeof ID_PREFIXES;

export function newId(kind: IdKind): string {
  return `${ID_PREFIXES[kind]}_${uuidv7()}`;
}

export function isId(kind: IdKind, value: unknown): value is string {
  return typeof value === 'string' && value.startsWith(`${ID_PREFIXES[kind]}_`);
}

/** RFC 3339 UTC with milliseconds, the timestamp format used everywhere. */
export function nowRfc3339(date: Date = new Date()): string {
  return date.toISOString();
}
