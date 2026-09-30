import { describe, expect, it } from 'vitest';
import { fnv1a32, inRollout, rolloutBucket } from './hash.js';

describe('fnv1a32', () => {
  it('matches the published FNV-1a 32-bit test vectors', () => {
    // These are the canonical vectors; the Go/Python/Rust kits must agree.
    expect(fnv1a32('')).toBe(0x811c9dc5);
    expect(fnv1a32('a')).toBe(0xe40c292c);
    expect(fnv1a32('foobar')).toBe(0xbf9cf968);
  });

  it('hashes UTF-8 bytes, not UTF-16 code units', () => {
    // "é" is 0xC3 0xA9 in UTF-8 but a single 0xE9 code unit; only the byte-wise answer
    // agrees with the Go and Rust kits.
    expect(fnv1a32('é')).toBe(513665217);
    expect(fnv1a32('é')).not.toBe(1812687940);
  });

  it('is unsigned', () => {
    for (const input of ['a', 'bb', 'turbo-encabulation:alice', 'x'.repeat(100)]) {
      expect(fnv1a32(input)).toBeGreaterThanOrEqual(0);
      expect(fnv1a32(input)).toBeLessThanOrEqual(0xffffffff);
    }
  });
});

describe('rollout evaluation', () => {
  it('buckets into 0..99', () => {
    for (let i = 0; i < 200; i++) {
      const bucket = rolloutBucket('turbo-encabulation', `customer-${i}`);
      expect(bucket).toBeGreaterThanOrEqual(0);
      expect(bucket).toBeLessThan(100);
    }
  });

  it('is stable for the same key and subject', () => {
    expect(rolloutBucket('quote-cache', 'alice')).toBe(rolloutBucket('quote-cache', 'alice'));
  });

  it('differs per key for the same subject', () => {
    const a = rolloutBucket('quote-cache', 'alice');
    const b = rolloutBucket('turbo-encabulation', 'alice');
    expect(a).not.toBe(b);
  });

  it('treats 0 and 100 as all-or-nothing', () => {
    expect(inRollout('k', 'alice', 0)).toBe(false);
    expect(inRollout('k', 'alice', 100)).toBe(true);
  });

  it('distributes roughly evenly', () => {
    let included = 0;
    const n = 10_000;
    for (let i = 0; i < n; i++) {
      if (inRollout('turbo-encabulation', `subject-${i}`, 25)) included++;
    }
    const ratio = included / n;
    expect(ratio).toBeGreaterThan(0.22);
    expect(ratio).toBeLessThan(0.28);
  });
});
