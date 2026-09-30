import { describe, expect, it } from 'vitest';
import {
  billOfMaterials,
  CATALOG,
  findPart,
  foundryIncrement,
  InvalidReservationError,
  initialStock,
  MAX_QUANTITY,
  STOCK_CAP,
} from './catalog.js';

describe('the parts catalog', () => {
  it('matches the SKUs and per-unit quantities in the design', () => {
    expect(CATALOG.map((p) => [p.sku, p.perUnit])).toEqual([
      ['AMU-BASE', 1],
      ['LOG-CASE', 1],
      ['SPV-BRG', 2],
      ['PNT-FAN', 1],
      ['HYD-MARZ', 6],
      ['LUN-WANE', 1],
      ['LOT-WIND', 1],
      ['TRM-PIPE', 3],
      ['GRD-SPRG', 1],
      ['DNG-ARM', 1],
    ]);
  });

  it('marks the dingle arm as optional and nothing else', () => {
    const optional = CATALOG.filter((p) => p.requiresOption);
    expect(optional.map((p) => p.sku)).toEqual(['DNG-ARM']);
  });

  it('looks parts up by SKU', () => {
    expect(findPart('HYD-MARZ')?.name).toBe('Hydrocoptic marzlevane');
    expect(findPart('NOT-REAL')).toBeUndefined();
  });
});

describe('billOfMaterials', () => {
  it('omits the dingle arm unless it was ordered', () => {
    const plain = billOfMaterials('turbo', 1);
    expect(plain).toHaveLength(9);
    expect(plain.map((i) => i.sku)).not.toContain('DNG-ARM');
  });

  it('adds one dingle arm per unit when the option is set', () => {
    const withArm = billOfMaterials('turbo', 3, { dingleArm: true });
    expect(withArm).toHaveLength(10);
    expect(withArm.find((i) => i.sku === 'DNG-ARM')).toEqual({ sku: 'DNG-ARM', quantity: 3 });
  });

  it('multiplies every line by the quantity', () => {
    const items = billOfMaterials('hyper', 4);
    expect(items.find((i) => i.sku === 'HYD-MARZ')).toEqual({ sku: 'HYD-MARZ', quantity: 24 });
    expect(items.find((i) => i.sku === 'SPV-BRG')).toEqual({ sku: 'SPV-BRG', quantity: 8 });
    expect(items.find((i) => i.sku === 'TRM-PIPE')).toEqual({ sku: 'TRM-PIPE', quantity: 12 });
    expect(items.find((i) => i.sku === 'AMU-BASE')).toEqual({ sku: 'AMU-BASE', quantity: 4 });
  });

  it('keeps the lines in catalog order, so the Lua key layout is stable', () => {
    expect(billOfMaterials('retro', 1, { dingleArm: true }).map((i) => i.sku)).toEqual(
      CATALOG.map((p) => p.sku),
    );
  });

  it('accepts every model in the design', () => {
    for (const model of ['turbo', 'retro', 'hyper']) {
      expect(() => billOfMaterials(model, 1)).not.toThrow();
    }
  });

  it('rejects an unknown model', () => {
    expect(() => billOfMaterials('encabulator-9000', 1)).toThrow(InvalidReservationError);
  });

  it('rejects quantities outside 1 to 10', () => {
    expect(() => billOfMaterials('turbo', 0)).toThrow(InvalidReservationError);
    expect(() => billOfMaterials('turbo', MAX_QUANTITY + 1)).toThrow(InvalidReservationError);
    expect(() => billOfMaterials('turbo', 2.5)).toThrow(InvalidReservationError);
    expect(() => billOfMaterials('turbo', Number.NaN)).toThrow(InvalidReservationError);
  });

  it('accepts the boundaries', () => {
    expect(billOfMaterials('turbo', 1)).toHaveLength(9);
    expect(billOfMaterials('turbo', MAX_QUANTITY)).toHaveLength(9);
  });
});

describe('stock levels', () => {
  it('seeds 1,000 of each SKU and 6,000 marzlevanes', () => {
    expect(initialStock('AMU-BASE')).toBe(1_000);
    expect(initialStock('SPV-BRG')).toBe(1_000);
    expect(initialStock('HYD-MARZ')).toBe(6_000);
  });

  it('gives the foundry a 6x marzlevane pour', () => {
    expect(foundryIncrement('AMU-BASE', 20)).toBe(20);
    expect(foundryIncrement('HYD-MARZ', 20)).toBe(120);
  });

  it('stops the foundry at rate 0', () => {
    for (const part of CATALOG) {
      expect(foundryIncrement(part.sku, 0)).toBe(0);
    }
  });

  it('ignores a negative rate', () => {
    expect(foundryIncrement('AMU-BASE', -5)).toBe(0);
  });

  it('caps the foundry below the initial marzlevane stock, deliberately', () => {
    // The design seeds 6,000 marzlevanes but caps the foundry at 5,000, so the bin only
    // refills once it has been drawn down.
    expect(STOCK_CAP).toBe(5_000);
    expect(initialStock('HYD-MARZ')).toBeGreaterThan(STOCK_CAP);
  });
});
