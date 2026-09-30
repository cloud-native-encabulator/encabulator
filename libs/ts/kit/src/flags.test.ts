import { describe, expect, it, vi } from 'vitest';
import { FLAG_DEFAULTS, FlagClient } from './flags.js';
import { createLogger } from './logger.js';

const logger = createLogger({ service: 'kit-test', level: 'silent' });

function flagsResponse(flags: unknown[], etag?: string): Response {
  return new Response(JSON.stringify({ flags }), {
    headers: {
      'content-type': 'application/json',
      ...(etag ? { etag } : {}),
    },
  });
}

describe('FlagClient without girdlespring', () => {
  it('reports itself disabled and serves the built-in defaults', () => {
    const client = new FlagClient({ baseUrl: undefined, logger });
    expect(client.enabled).toBe(false);
    expect(client.boolean('turbo-encabulation')).toBe(false);
    expect(client.boolean('quote-cache')).toBe(true);
    expect(client.boolean('retro-encabulator-sales')).toBe(true);
    expect(client.boolean('strict-side-fumbling')).toBe(false);
    expect(client.number('amulite-foundry-rate')).toBe(20);
    expect(client.number('gateway-rate-limit-rps')).toBe(50);
    expect(client.number('dingle-arm-damping')).toBe(50);
  });

  it('ships defaults for exactly the flags in the design', () => {
    expect(Object.keys(FLAG_DEFAULTS).sort()).toEqual([
      'amulite-foundry-rate',
      'dingle-arm-damping',
      'gateway-rate-limit-rps',
      'quote-cache',
      'retro-encabulator-sales',
      'strict-side-fumbling',
      'turbo-encabulation',
    ]);
  });
});

describe('FlagClient polling', () => {
  it('loads flags and sends If-None-Match on the next poll', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        flagsResponse([{ key: 'turbo-encabulation', type: 'boolean', value: true }], 'W/"rev-1"'),
      )
      .mockResolvedValueOnce(new Response(null, { status: 304 }));
    vi.stubGlobal('fetch', fetchMock);

    const client = new FlagClient({ baseUrl: 'http://girdlespring:8080', logger });
    await client.refresh();
    expect(client.boolean('turbo-encabulation')).toBe(true);
    expect(client.loaded).toBe(true);

    await client.refresh();
    const secondCall = fetchMock.mock.calls[1] as [string, { headers: Record<string, string> }];
    expect(secondCall[1].headers['if-none-match']).toBe('W/"rev-1"');
    // A 304 must not clear the set.
    expect(client.boolean('turbo-encabulation')).toBe(true);

    vi.unstubAllGlobals();
  });

  it('keeps the last good set when the service becomes unreachable', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(flagsResponse([{ key: 'quote-cache', type: 'boolean', value: false }]))
      .mockRejectedValueOnce(new Error('ECONNREFUSED'));
    vi.stubGlobal('fetch', fetchMock);

    const client = new FlagClient({ baseUrl: 'http://girdlespring:8080', logger });
    await client.refresh();
    expect(client.boolean('quote-cache')).toBe(false);
    await client.refresh();
    expect(client.boolean('quote-cache')).toBe(false);

    vi.unstubAllGlobals();
  });

  it('falls back to defaults when the very first poll fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('ECONNREFUSED')));
    const client = new FlagClient({ baseUrl: 'http://girdlespring:8080', logger });
    await client.start();
    expect(client.loaded).toBe(false);
    expect(client.boolean('quote-cache')).toBe(true);
    client.stop();
    vi.unstubAllGlobals();
  });

  it('ignores a non-200, non-304 response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('nope', { status: 500 })));
    const client = new FlagClient({ baseUrl: 'http://girdlespring:8080', logger });
    await client.refresh();
    expect(client.loaded).toBe(false);
    vi.unstubAllGlobals();
  });
});

describe('flag evaluation', () => {
  it('applies rolloutPercent per subject', () => {
    const client = new FlagClient({ baseUrl: undefined, logger });
    client.setForTesting([
      { key: 'turbo-encabulation', type: 'boolean', value: true, rolloutPercent: 50 },
    ]);
    const included = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'].filter((s) =>
      client.boolean('turbo-encabulation', s),
    );
    // A 50% rollout over eight subjects should split, not land all on one side.
    expect(included.length).toBeGreaterThan(0);
    expect(included.length).toBeLessThan(8);
  });

  it('applies the value to everyone when rolloutPercent is absent', () => {
    const client = new FlagClient({ baseUrl: undefined, logger });
    client.setForTesting([{ key: 'turbo-encabulation', type: 'boolean', value: true }]);
    for (const subject of ['a', 'b', 'c']) {
      expect(client.boolean('turbo-encabulation', subject)).toBe(true);
    }
  });

  it('ignores rolloutPercent when no subject is supplied', () => {
    const client = new FlagClient({ baseUrl: undefined, logger });
    client.setForTesting([
      { key: 'turbo-encabulation', type: 'boolean', value: true, rolloutPercent: 1 },
    ]);
    expect(client.boolean('turbo-encabulation')).toBe(true);
  });

  it('coerces number flags and falls back on garbage', () => {
    const client = new FlagClient({ baseUrl: undefined, logger });
    client.setForTesting([
      { key: 'amulite-foundry-rate', type: 'number', value: 0 },
      { key: 'gateway-rate-limit-rps', type: 'number', value: 'not-a-number' },
    ]);
    expect(client.number('amulite-foundry-rate')).toBe(0);
    expect(client.number('gateway-rate-limit-rps')).toBe(50);
  });
});
