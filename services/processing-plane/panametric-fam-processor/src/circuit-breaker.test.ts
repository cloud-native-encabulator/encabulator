import { createLogger } from '@encabulator/kit';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CircuitBreaker, DEFAULT_FAILURE_THRESHOLD } from './circuit-breaker.js';

const logger = createLogger({ service: 'processor-test', level: 'silent' });

function breaker(nowRef: { value: number }) {
  return new CircuitBreaker({ name: 'test-dependency', logger, now: () => nowRef.value });
}

describe('CircuitBreaker', () => {
  let now: { value: number };

  beforeEach(() => {
    now = { value: 1_000 };
  });

  it('starts closed and passes calls through', async () => {
    const cb = breaker(now);
    expect(cb.currentState()).toBe('closed');
    await expect(cb.run(async () => 'ok')).resolves.toBe('ok');
  });

  it('opens after five consecutive failures, not four', async () => {
    const cb = breaker(now);
    const boom = async () => {
      throw new Error('dependency down');
    };
    for (let i = 0; i < DEFAULT_FAILURE_THRESHOLD - 1; i++) {
      await expect(cb.run(boom)).rejects.toThrow('dependency down');
      expect(cb.currentState()).toBe('closed');
    }
    await expect(cb.run(boom)).rejects.toThrow('dependency down');
    expect(cb.currentState()).toBe('open');
  });

  it('fails fast with dependency_unavailable while open', async () => {
    const cb = breaker(now);
    const inner = vi.fn(async () => {
      throw new Error('down');
    });
    for (let i = 0; i < DEFAULT_FAILURE_THRESHOLD; i++) {
      await cb.run(inner).catch(() => undefined);
    }
    inner.mockClear();

    await expect(cb.run(inner)).rejects.toMatchObject({
      statusCode: 503,
      code: 'dependency_unavailable',
    });
    expect(inner).not.toHaveBeenCalled();
  });

  it('resets the failure count on a success', async () => {
    const cb = breaker(now);
    for (let i = 0; i < 4; i++) {
      await cb
        .run(async () => {
          throw new Error('down');
        })
        .catch(() => undefined);
    }
    await cb.run(async () => 'recovered');
    await cb
      .run(async () => {
        throw new Error('down');
      })
      .catch(() => undefined);
    expect(cb.currentState()).toBe('closed');
  });

  it('half-opens after ten seconds and closes on a successful probe', async () => {
    const cb = breaker(now);
    for (let i = 0; i < DEFAULT_FAILURE_THRESHOLD; i++) {
      await cb
        .run(async () => {
          throw new Error('down');
        })
        .catch(() => undefined);
    }
    expect(cb.currentState()).toBe('open');

    now.value += 9_999;
    expect(cb.currentState()).toBe('open');

    now.value += 1;
    expect(cb.currentState()).toBe('half-open');
    await expect(cb.run(async () => 'back')).resolves.toBe('back');
    expect(cb.currentState()).toBe('closed');
  });

  it('re-opens immediately when the probe fails', async () => {
    const cb = breaker(now);
    for (let i = 0; i < DEFAULT_FAILURE_THRESHOLD; i++) {
      await cb
        .run(async () => {
          throw new Error('down');
        })
        .catch(() => undefined);
    }
    now.value += 10_000;
    expect(cb.currentState()).toBe('half-open');

    await cb
      .run(async () => {
        throw new Error('still down');
      })
      .catch(() => undefined);
    expect(cb.currentState()).toBe('open');

    // And the half-open timer restarts from the new failure.
    now.value += 9_000;
    expect(cb.currentState()).toBe('open');
  });

  it('lets only one probe through while half-open', async () => {
    const cb = breaker(now);
    for (let i = 0; i < DEFAULT_FAILURE_THRESHOLD; i++) {
      await cb
        .run(async () => {
          throw new Error('down');
        })
        .catch(() => undefined);
    }
    now.value += 10_000;

    let release: () => void = () => {};
    const slow = cb.run(
      () =>
        new Promise<string>((resolve) => {
          release = () => resolve('ok');
        }),
    );
    await expect(cb.run(async () => 'second')).rejects.toMatchObject({
      code: 'dependency_unavailable',
    });
    release();
    await expect(slow).resolves.toBe('ok');
  });

  it('does not count a business-level 4xx as a dependency failure', async () => {
    const cb = breaker(now);
    // This is the shape HttpClientError exposes.
    const businessError = Object.assign(new Error('insufficient parts'), {
      status: 409,
      isDependencyFailure: false,
    });
    for (let i = 0; i < 20; i++) {
      await expect(
        cb.run(async () => {
          throw businessError;
        }),
      ).rejects.toThrow('insufficient parts');
    }
    expect(cb.currentState()).toBe('closed');
  });

  it('counts a 5xx as a dependency failure', async () => {
    const cb = breaker(now);
    const serverError = Object.assign(new Error('boom'), {
      status: 503,
      isDependencyFailure: true,
    });
    for (let i = 0; i < DEFAULT_FAILURE_THRESHOLD; i++) {
      await cb
        .run(async () => {
          throw serverError;
        })
        .catch(() => undefined);
    }
    expect(cb.currentState()).toBe('open');
  });

  it('treats a business answer during a probe as proof of life', async () => {
    const cb = breaker(now);
    for (let i = 0; i < DEFAULT_FAILURE_THRESHOLD; i++) {
      await cb
        .run(async () => {
          throw new Error('down');
        })
        .catch(() => undefined);
    }
    now.value += 10_000;
    expect(cb.currentState()).toBe('half-open');

    const businessError = Object.assign(new Error('no parts'), { isDependencyFailure: false });
    await cb
      .run(async () => {
        throw businessError;
      })
      .catch(() => undefined);
    expect(cb.currentState()).toBe('closed');
  });
});
