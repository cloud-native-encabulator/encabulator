import { describe, expect, it, vi } from 'vitest';
import { HttpError } from './errors.js';
import { createLogger } from './logger.js';
import { type Directive, MalaiseController, MalaiseNak } from './malaise.js';

const logger = createLogger({ service: 'kit-test', level: 'silent' });

function future(ms = 60_000): string {
  return new Date(Date.now() + ms).toISOString();
}

function controller(directives: Directive[], random = () => 0.5) {
  const slept: number[] = [];
  const c = new MalaiseController({
    serviceName: 'kit-test',
    engineUrl: 'http://malaise.invalid',
    logger,
    random,
    sleep: async (ms) => {
      slept.push(ms);
    },
  });
  c.setForTesting(directives);
  return { c, slept };
}

describe('MalaiseController', () => {
  it('is disabled when no engine URL is configured', async () => {
    const c = new MalaiseController({ serviceName: 'kit-test', engineUrl: undefined, logger });
    expect(c.enabled).toBe(false);
    const release = await c.applyInbound();
    expect(release).toBeTypeOf('function');
  });

  it('sleeps ms ± jitterMs for latency', async () => {
    const { c, slept } = controller(
      [{ id: 'mal_1', kind: 'latency', params: { ms: 400, jitterMs: 100 }, expiresAt: future() }],
      () => 1, // jitter at its maximum
    );
    await c.applyInbound();
    expect(slept).toEqual([500]);
  });

  it('honours the latency percentage', async () => {
    const { c, slept } = controller(
      [
        {
          id: 'mal_1',
          kind: 'latency',
          params: { ms: 100, percent: 25 },
          expiresAt: future(),
        },
      ],
      () => 0.9, // 90 >= 25, so this request is not slowed
    );
    await c.applyInbound();
    expect(slept).toEqual([]);
  });

  it('throws the injected error with the documented code', async () => {
    const { c } = controller([
      { id: 'mal_2', kind: 'error', params: { status: 502, percent: 100 }, expiresAt: future() },
    ]);
    await expect(c.applyInbound()).rejects.toMatchObject({
      statusCode: 502,
      code: 'malaise_injected',
    });
  });

  it('defaults the injected error to 503', async () => {
    const { c } = controller([{ id: 'mal_2', kind: 'error', params: {}, expiresAt: future() }]);
    await expect(c.applyInbound()).rejects.toMatchObject({ statusCode: 503 });
  });

  it('stops applying a directive once it has expired', async () => {
    const { c, slept } = controller([
      {
        id: 'mal_3',
        kind: 'latency',
        params: { ms: 999 },
        expiresAt: new Date(Date.now() - 1_000).toISOString(),
      },
    ]);
    await c.applyInbound();
    expect(slept).toEqual([]);
    expect(c.directives()).toEqual([]);
  });

  it('admits only maxConcurrent requests, then times the rest out', async () => {
    const c = new MalaiseController({
      serviceName: 'kit-test',
      engineUrl: 'http://malaise.invalid',
      logger,
    });
    c.setForTesting([
      {
        id: 'mal_4',
        kind: 'congestion',
        params: { maxConcurrent: 1, queueTimeoutMs: 40 },
        expiresAt: future(),
      },
    ]);

    const firstRelease = await c.applyInbound();
    // The second request has to wait, and nobody releases within 40 ms.
    await expect(c.applyInbound()).rejects.toMatchObject({ statusCode: 503, code: 'congested' });
    firstRelease();
    // With the permit back, a third request gets straight through.
    const thirdRelease = await c.applyInbound();
    thirdRelease();
  });

  it('lets a queued request in as soon as a permit frees up', async () => {
    const c = new MalaiseController({
      serviceName: 'kit-test',
      engineUrl: 'http://malaise.invalid',
      logger,
    });
    c.setForTesting([
      {
        id: 'mal_5',
        kind: 'congestion',
        params: { maxConcurrent: 1, queueTimeoutMs: 2_000 },
        expiresAt: future(),
      },
    ]);
    const release = await c.applyInbound();
    const queued = c.applyInbound();
    expect(c.snapshot().queueDepth).toBe(1);
    release();
    const secondRelease = await queued;
    secondRelease();
  });
});

describe('wrapMessageHandler', () => {
  it('passes the message through when nothing is injected', async () => {
    const { c } = controller([]);
    const handler = c.wrapMessageHandler(async (n: number) => n * 2);
    await expect(handler(21)).resolves.toBe(42);
  });

  it('slows the handler down for latency', async () => {
    const { c, slept } = controller([
      { id: 'mal_6', kind: 'latency', params: { ms: 250 }, expiresAt: future() },
    ]);
    const handler = c.wrapMessageHandler(async () => 'done');
    await expect(handler()).resolves.toBe('done');
    expect(slept).toEqual([250]);
  });

  it('naks instead of responding for the error kind', async () => {
    const { c } = controller([
      { id: 'mal_7', kind: 'error', params: { percent: 100 }, expiresAt: future() },
    ]);
    const inner = vi.fn(async () => 'never');
    const handler = c.wrapMessageHandler(inner);
    await expect(handler()).rejects.toBeInstanceOf(MalaiseNak);
    expect(inner).not.toHaveBeenCalled();
  });

  it('releases the congestion permit even when the handler throws', async () => {
    const c = new MalaiseController({
      serviceName: 'kit-test',
      engineUrl: 'http://malaise.invalid',
      logger,
    });
    c.setForTesting([
      {
        id: 'mal_8',
        kind: 'congestion',
        params: { maxConcurrent: 1, queueTimeoutMs: 50 },
        expiresAt: future(),
      },
    ]);
    const handler = c.wrapMessageHandler(async () => {
      throw new Error('fabrication failed');
    });
    await expect(handler()).rejects.toThrow('fabrication failed');
    // If the permit leaked, this would time out instead of resolving.
    const release = await c.applyInbound();
    release();
  });
});

describe('directive polling', () => {
  it('keeps the last directives when the engine goes away', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            directives: [{ id: 'mal_9', kind: 'latency', params: { ms: 10 }, expiresAt: future() }],
          }),
          { headers: { 'content-type': 'application/json' } },
        ),
      )
      .mockRejectedValueOnce(new Error('ECONNREFUSED'));
    vi.stubGlobal('fetch', fetchMock);

    const c = new MalaiseController({
      serviceName: 'kit-test',
      engineUrl: 'http://malaise.invalid',
      logger,
    });
    await c.poll();
    expect(c.directives()).toHaveLength(1);
    await c.poll();
    expect(c.directives()).toHaveLength(1);

    vi.unstubAllGlobals();
  });

  it('asks the engine for its own service only', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ directives: [] }), {
        headers: { 'content-type': 'application/json' },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);
    const c = new MalaiseController({
      serviceName: 'panametric-fam-processor',
      engineUrl: 'http://engine:8080/',
      logger,
    });
    await c.poll();
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      'http://engine:8080/v1/directives?service=panametric-fam-processor',
    );
    vi.unstubAllGlobals();
  });
});

describe('HttpError from congestion', () => {
  it('carries the congested code', () => {
    const err = new HttpError(503, 'congested', 'nope');
    expect(err.toBody()).toEqual({ error: { code: 'congested', message: 'nope' } });
  });
});
