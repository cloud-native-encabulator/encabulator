import { describe, expect, it } from 'vitest';
import { DUE_KEY, deliveredKey, PREFIX, RECENT_KEY, shipmentKey } from './keys.js';

/** What Valkey Cluster hashes: the first `{...}` with something inside, else the whole key. */
function hashTag(key: string): string {
  const open = key.indexOf('{');
  const close = key.indexOf('}', open + 1);
  return open >= 0 && close > open + 1 ? key.slice(open + 1, close) : key;
}

describe('keyspace', () => {
  it('keeps every key in one hash slot, so cluster-mode servers accept the scripts and MGET', () => {
    const keys = [
      shipmentKey('ord_1'),
      shipmentKey('ord_2'),
      deliveredKey('ord_1'),
      DUE_KEY,
      RECENT_KEY,
    ];
    expect(new Set(keys.map(hashTag))).toEqual(new Set(['shipment']));
    for (const key of keys) expect(key.startsWith(PREFIX)).toBe(true);
  });
});
