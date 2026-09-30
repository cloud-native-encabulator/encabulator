import { describe, expect, it } from 'vitest';
import {
  CARRIERS,
  chooseCarrier,
  etaSeconds,
  isCarrier,
  MAX_ETA_SECONDS,
  MIN_ETA_SECONDS,
  trackingNumber,
} from './carriers.js';

describe('carriers', () => {
  it('offers exactly the four carriers in the design', () => {
    expect([...CARRIERS]).toEqual([
      'Trunnion Freight',
      'Waneshaft Express',
      'Grammeter Parcel',
      'Tremie Pipe & Sons',
    ]);
  });

  it('maps the random range across every carrier', () => {
    expect(chooseCarrier(() => 0)).toBe('Trunnion Freight');
    expect(chooseCarrier(() => 0.3)).toBe('Waneshaft Express');
    expect(chooseCarrier(() => 0.6)).toBe('Grammeter Parcel');
    expect(chooseCarrier(() => 0.99)).toBe('Tremie Pipe & Sons');
  });

  it('never runs off the end of the list', () => {
    // Math.random() is documented as < 1, but a stub might not be.
    expect(isCarrier(chooseCarrier(() => 1))).toBe(true);
  });

  it('eventually picks all four', () => {
    const seen = new Set<string>();
    for (let i = 0; i < 400; i++) seen.add(chooseCarrier());
    expect(seen.size).toBe(4);
  });
});

describe('trackingNumber', () => {
  it('is NT followed by ten digits', () => {
    for (let i = 0; i < 50; i++) {
      expect(trackingNumber()).toMatch(/^NT\d{10}$/);
    }
  });

  it('is deterministic given the random source', () => {
    expect(trackingNumber(() => 0)).toBe('NT0000000000');
    expect(trackingNumber(() => 0.99)).toBe('NT9999999999');
  });
});

describe('etaSeconds', () => {
  it('stays within the simulated transit window', () => {
    for (let i = 0; i < 500; i++) {
      const eta = etaSeconds();
      expect(eta).toBeGreaterThanOrEqual(MIN_ETA_SECONDS);
      expect(eta).toBeLessThanOrEqual(MAX_ETA_SECONDS);
      expect(Number.isInteger(eta)).toBe(true);
    }
  });

  it('reaches both ends of the window', () => {
    expect(etaSeconds(() => 0)).toBe(10);
    expect(etaSeconds(() => 0.999999)).toBe(60);
  });
});
