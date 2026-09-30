import { describe, expect, it } from 'vitest';
import {
  InvalidOrderError,
  MAX_ATTEMPTS,
  MAX_QUANTITY,
  nextAttempt,
  parseOrderRequest,
  scrapReason,
} from './domain.js';

describe('parseOrderRequest', () => {
  it('normalises a valid order', () => {
    expect(
      parseOrderRequest({ model: 'turbo', quantity: 2, options: { dingleArm: true } }),
    ).toEqual({ model: 'turbo', quantity: 2, options: { dingleArm: true } });
  });

  it('defaults the dingle arm to off', () => {
    expect(parseOrderRequest({ model: 'hyper', quantity: 1 }).options).toEqual({
      dingleArm: false,
    });
    expect(parseOrderRequest({ model: 'hyper', quantity: 1, options: {} }).options).toEqual({
      dingleArm: false,
    });
  });

  it('accepts all three models', () => {
    for (const model of ['turbo', 'retro', 'hyper']) {
      expect(parseOrderRequest({ model, quantity: 1 }).model).toBe(model);
    }
  });

  it('rejects an unknown model', () => {
    expect(() => parseOrderRequest({ model: 'flux-capacitor', quantity: 1 })).toThrow(
      InvalidOrderError,
    );
  });

  it('rejects quantities outside 1 to 10', () => {
    for (const quantity of [0, -1, MAX_QUANTITY + 1, 2.5, Number.NaN, '3']) {
      expect(() => parseOrderRequest({ model: 'turbo', quantity })).toThrow(InvalidOrderError);
    }
  });

  it('accepts the boundaries', () => {
    expect(parseOrderRequest({ model: 'turbo', quantity: 1 }).quantity).toBe(1);
    expect(parseOrderRequest({ model: 'turbo', quantity: MAX_QUANTITY }).quantity).toBe(10);
  });

  it('rejects a non-boolean dingleArm', () => {
    expect(() =>
      parseOrderRequest({ model: 'turbo', quantity: 1, options: { dingleArm: 'yes' } }),
    ).toThrow(InvalidOrderError);
  });

  it('rejects a non-object body', () => {
    for (const body of [null, undefined, 'turbo', 42]) {
      expect(() => parseOrderRequest(body)).toThrow(InvalidOrderError);
    }
  });

  it('ignores a customer smuggled in through the body', () => {
    const parsed = parseOrderRequest({ model: 'turbo', quantity: 1, customer: 'mallory' });
    expect(parsed).not.toHaveProperty('customer');
  });
});

describe('nextAttempt', () => {
  it('buys a second and a third attempt', () => {
    expect(nextAttempt(1)).toBe(2);
    expect(nextAttempt(2)).toBe(3);
  });

  it('scraps after the third', () => {
    expect(nextAttempt(MAX_ATTEMPTS)).toBeUndefined();
    expect(nextAttempt(4)).toBeUndefined();
  });

  it('treats a missing or nonsense attempt as the first', () => {
    expect(nextAttempt(0)).toBe(1);
    expect(nextAttempt(Number.NaN)).toBe(1);
  });
});

describe('scrapReason', () => {
  it('names the worst finding', () => {
    const reason = scrapReason(3, [
      {
        serial: 'ENC-1',
        stage: 'fit-marzlevanes',
        metric: 'repleneration',
        score: 4.2,
        reason: 'spike',
      },
      {
        serial: 'ENC-1',
        stage: 'wind-lotus-o-delta',
        metric: 'grammeter_drift',
        score: 7.9,
        reason: 'drift',
      },
    ]);
    expect(reason).toContain('3 attempts');
    expect(reason).toContain('1 unit');
    expect(reason).toContain('grammeter_drift');
    expect(reason).toContain('wind-lotus-o-delta');
    expect(reason).toContain('7.90');
  });

  it('counts distinct units', () => {
    const reason = scrapReason(3, [
      { serial: 'ENC-1', stage: 's', metric: 'm', score: 1, reason: 'r' },
      { serial: 'ENC-2', stage: 's', metric: 'm', score: 2, reason: 'r' },
    ]);
    expect(reason).toContain('2 units');
  });

  it('copes with no findings at all', () => {
    expect(scrapReason(3)).toContain('no findings recorded');
  });
});
