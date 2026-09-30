import { describe, expect, it } from 'vitest';
import { ID_PREFIXES, isId, newId, nowRfc3339 } from './ids.js';

describe('newId', () => {
  it('uses the documented prefixes', () => {
    expect(newId('order')).toMatch(/^ord_/);
    expect(newId('quote')).toMatch(/^qte_/);
    expect(newId('reservation')).toMatch(/^rsv_/);
    expect(newId('shipment')).toMatch(/^shp_/);
    expect(newId('incident')).toMatch(/^inc_/);
    expect(newId('malaise')).toMatch(/^mal_/);
    expect(newId('run')).toMatch(/^run_/);
    expect(newId('event')).toMatch(/^evt_/);
  });

  it('covers every prefix in the design', () => {
    expect(Object.values(ID_PREFIXES).sort()).toEqual([
      'evt',
      'inc',
      'mal',
      'ord',
      'qte',
      'rsv',
      'run',
      'shp',
    ]);
  });

  it('produces a UUIDv7 suffix', () => {
    const suffix = newId('order').slice('ord_'.length);
    expect(suffix).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  it('sorts lexicographically by creation time', async () => {
    const first = newId('order');
    await new Promise((resolve) => setTimeout(resolve, 5));
    const second = newId('order');
    expect(first < second).toBe(true);
  });

  it('recognises its own ids', () => {
    const id = newId('reservation');
    expect(isId('reservation', id)).toBe(true);
    expect(isId('order', id)).toBe(false);
    expect(isId('order', 42)).toBe(false);
  });
});

describe('nowRfc3339', () => {
  it('formats UTC with milliseconds', () => {
    expect(nowRfc3339(new Date(Date.UTC(2026, 8, 30, 12, 0, 0, 123)))).toBe(
      '2026-09-30T12:00:00.123Z',
    );
  });
});
