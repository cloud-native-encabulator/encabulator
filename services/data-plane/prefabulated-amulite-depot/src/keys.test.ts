import { describe, expect, it } from 'vitest';
import {
  orderIndexKey,
  PENDING_KEY,
  PREFIX,
  reservationKey,
  reservedKey,
  stockKey,
  stockKeyPairs,
} from './keys.js';

/** What Valkey Cluster hashes: the first `{...}` with something inside, else the whole key. */
function hashTag(key: string): string {
  const open = key.indexOf('{');
  const close = key.indexOf('}', open + 1);
  return open >= 0 && close > open + 1 ? key.slice(open + 1, close) : key;
}

describe('keyspace', () => {
  it('keeps every key in one hash slot, so cluster-mode servers accept the multi-key scripts', () => {
    const keys = [
      ...stockKeyPairs(['AMU-BASE', 'DNG-ARM']),
      stockKey('MZL-6'),
      reservedKey('MZL-6'),
      reservationKey('rsv_1'),
      orderIndexKey('ord_1'),
      PENDING_KEY,
    ];
    expect(new Set(keys.map(hashTag))).toEqual(new Set(['depot']));
    for (const key of keys) expect(key.startsWith(PREFIX)).toBe(true);
  });
});
