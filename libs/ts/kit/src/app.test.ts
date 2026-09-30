import type { FastifyInstance } from 'fastify';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { createApp, type ReadinessCheck } from './app.js';
import { conflict } from './errors.js';
import { createLogger } from './logger.js';
import { MalaiseController } from './malaise.js';
import { registry } from './metrics.js';

const logger = createLogger({ service: 'kit-test', level: 'silent' });

async function buildApp(
  readiness: ReadinessCheck[] = [],
  malaise?: MalaiseController,
): Promise<FastifyInstance> {
  const app = await createApp({
    serviceName: 'kit-test',
    logger,
    readiness,
    ...(malaise ? { malaise } : {}),
    register: async (instance) => {
      instance.get('/v1/things/:id', async (request) => ({
        id: (request.params as { id: string }).id,
      }));
      instance.get('/v1/boom', async () => {
        throw conflict('side_fumbling', 'The bearings are side fumbling.');
      });
      instance.get('/v1/unhandled', async () => {
        throw new Error('a secret internal detail');
      });
      instance.get('/v1/whoami', async (request) => request.identity);
    },
  });
  await app.ready();
  return app;
}

describe('createApp', () => {
  let app: FastifyInstance;

  beforeEach(() => {
    registry.resetMetrics();
  });

  afterEach(async () => {
    await app?.close();
  });

  it('serves /healthz', async () => {
    app = await buildApp();
    const res = await app.inject({ method: 'GET', url: '/healthz' });
    expect(res.statusCode).toBe(200);
    expect(res.json()).toEqual({ status: 'ok' });
  });

  it('serves /readyz green when every check passes', async () => {
    app = await buildApp([{ name: 'valkey', check: () => {} }]);
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ status: 'ok', checks: ['valkey'] });
  });

  it('serves /readyz 503 when a hard dependency is down', async () => {
    app = await buildApp([
      {
        name: 'valkey',
        check: () => {
          throw new Error('ECONNREFUSED');
        },
      },
    ]);
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(503);
    expect(res.json()).toMatchObject({
      error: { code: 'not_ready' },
      failures: [{ name: 'valkey', error: 'ECONNREFUSED' }],
    });
  });

  it('fails readiness once draining', async () => {
    app = await buildApp();
    app.failReadiness();
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(503);
    expect(res.json().error.code).toBe('shutting_down');
  });

  it('exposes Prometheus metrics', async () => {
    app = await buildApp();
    await app.inject({ method: 'GET', url: '/v1/things/42' });
    const res = await app.inject({ method: 'GET', url: '/metrics' });
    expect(res.statusCode).toBe(200);
    expect(res.headers['content-type']).toContain('text/plain');
    expect(res.body).toContain('http_server_request_duration_seconds');
  });

  it('labels the duration histogram with the route template, never the raw path', async () => {
    app = await buildApp();
    await app.inject({ method: 'GET', url: '/v1/things/abc-123' });
    const metrics = await registry.metrics();
    expect(metrics).toContain('http_route="/v1/things/:id"');
    expect(metrics).not.toContain('abc-123');
  });

  it('does not record infra paths in the request histogram', async () => {
    app = await buildApp();
    await app.inject({ method: 'GET', url: '/healthz' });
    await app.inject({ method: 'GET', url: '/readyz' });
    const metrics = await registry.metrics();
    expect(metrics).not.toContain('http_route="/healthz"');
    expect(metrics).not.toContain('http_route="/readyz"');
  });

  it('echoes an inbound X-Request-Id', async () => {
    app = await buildApp();
    const res = await app.inject({
      method: 'GET',
      url: '/v1/things/1',
      headers: { 'x-request-id': 'req-from-the-gateway' },
    });
    expect(res.headers['x-request-id']).toBe('req-from-the-gateway');
  });

  it('generates a request id when none is supplied', async () => {
    app = await buildApp();
    const res = await app.inject({ method: 'GET', url: '/v1/things/1' });
    expect(res.headers['x-request-id']).toMatch(/^[0-9a-f-]{36}$/);
  });

  it('renders HttpError in the documented error shape', async () => {
    app = await buildApp();
    const res = await app.inject({ method: 'GET', url: '/v1/boom' });
    expect(res.statusCode).toBe(409);
    expect(res.json()).toEqual({
      error: { code: 'side_fumbling', message: 'The bearings are side fumbling.' },
    });
  });

  it('does not leak internals on an unexpected error', async () => {
    app = await buildApp();
    const res = await app.inject({ method: 'GET', url: '/v1/unhandled' });
    expect(res.statusCode).toBe(500);
    expect(res.json().error.code).toBe('internal_error');
    expect(res.body).not.toContain('a secret internal detail');
  });

  it('uses the error shape for 404s', async () => {
    app = await buildApp();
    const res = await app.inject({ method: 'GET', url: '/nope' });
    expect(res.statusCode).toBe(404);
    expect(res.json().error.code).toBe('not_found');
  });

  it('parses identity headers onto the request', async () => {
    app = await buildApp();
    const res = await app.inject({
      method: 'GET',
      url: '/v1/whoami',
      headers: { 'x-encabulator-subject': 'alice', 'x-encabulator-roles': 'customer,operator' },
    });
    expect(res.json()).toEqual({ subject: 'alice', roles: ['customer', 'operator'] });
  });

  it('accepts readiness checks added after construction', async () => {
    app = await buildApp();
    app.addReadinessCheck({
      name: 'nats',
      check: () => {
        throw new Error('closed');
      },
    });
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(503);
  });
});

describe('malaise integration with the app', () => {
  let app: FastifyInstance;

  afterEach(async () => {
    await app?.close();
    registry.resetMetrics();
  });

  it('returns the injected error for the API but keeps health green', async () => {
    const malaise = new MalaiseController({
      serviceName: 'kit-test',
      engineUrl: 'http://malaise.invalid',
      logger,
    });
    malaise.setForTesting([
      {
        id: 'mal_1',
        kind: 'error',
        params: { status: 503, percent: 100 },
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
      },
    ]);
    app = await buildApp([], malaise);

    const api = await app.inject({ method: 'GET', url: '/v1/things/1' });
    expect(api.statusCode).toBe(503);
    expect(api.json().error.code).toBe('malaise_injected');

    const health = await app.inject({ method: 'GET', url: '/healthz' });
    expect(health.statusCode).toBe(200);
  });
});
