import Fastify, { type FastifyInstance } from 'fastify';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { HttpClient, HttpClientError } from './http-client.js';
import { registry } from './metrics.js';

interface Upstream {
  app: FastifyInstance;
  url: string;
  calls: string[];
}

async function upstream(
  configure: (app: FastifyInstance, calls: string[]) => void,
): Promise<Upstream> {
  const calls: string[] = [];
  const app = Fastify({ logger: false });
  app.addHook('onRequest', async (request) => {
    calls.push(`${request.method} ${request.url}`);
  });
  configure(app, calls);
  await app.listen({ port: 0, host: '127.0.0.1' });
  const address = app.server.address();
  const port = typeof address === 'object' && address ? address.port : 0;
  return { app, url: `http://127.0.0.1:${port}`, calls };
}

describe('HttpClient', () => {
  let target: Upstream;

  beforeEach(() => {
    registry.resetMetrics();
  });

  afterEach(async () => {
    await target?.app.close();
  });

  it('returns a parsed JSON body', async () => {
    target = await upstream((app) => {
      app.get('/v1/parts', async () => [{ sku: 'AMU-BASE', stock: 1000 }]);
    });
    const client = new HttpClient({ name: 'prefabulated-amulite-depot', baseUrl: target.url });
    const res = await client.get<Array<{ sku: string }>>('/v1/parts');
    expect(res.status).toBe(200);
    expect(res.body[0]?.sku).toBe('AMU-BASE');
  });

  it('records the client histogram with the service name as server_address', async () => {
    target = await upstream((app) => {
      app.get('/v1/parts', async () => ({ ok: true }));
    });
    const client = new HttpClient({ name: 'prefabulated-amulite-depot', baseUrl: target.url });
    await client.get('/v1/parts');
    const metrics = await registry.metrics();
    expect(metrics).toContain('server_address="prefabulated-amulite-depot"');
    expect(metrics).toContain('http_client_request_duration_seconds');
    // The host:port must not leak into the label.
    expect(metrics).not.toContain('server_address="127.0.0.1');
  });

  it('throws HttpClientError carrying the upstream error code', async () => {
    target = await upstream((app) => {
      app.post('/v1/reservations', async (_req, reply) =>
        reply.code(409).send({ error: { code: 'insufficient_parts', message: 'no marzlevanes' } }),
      );
    });
    const client = new HttpClient({ name: 'depot', baseUrl: target.url });
    await expect(client.post('/v1/reservations', {})).rejects.toMatchObject({
      status: 409,
      code: 'insufficient_parts',
    });
  });

  it('lets the caller opt into an expected status instead of throwing', async () => {
    target = await upstream((app) => {
      app.post('/v1/reservations', async (_req, reply) =>
        reply.code(409).send({ error: { code: 'insufficient_parts', message: 'no' } }),
      );
    });
    const client = new HttpClient({ name: 'depot', baseUrl: target.url });
    const res = await client.post('/v1/reservations', {}, { expect: [409] });
    expect(res.status).toBe(409);
  });

  it('retries a GET twice on 503, then succeeds', async () => {
    let attempts = 0;
    target = await upstream((app) => {
      app.get('/v1/flaky', async (_req, reply) => {
        attempts++;
        if (attempts < 3) return reply.code(503).send({ error: { code: 'busy', message: 'busy' } });
        return { ok: true };
      });
    });
    const client = new HttpClient({ name: 'flaky', baseUrl: target.url });
    const res = await client.get<{ ok: boolean }>('/v1/flaky');
    expect(res.body.ok).toBe(true);
    expect(attempts).toBe(3);
  });

  it('gives up after two retries', async () => {
    target = await upstream((app) => {
      app.get('/v1/down', async (_req, reply) =>
        reply.code(503).send({ error: { code: 'busy', message: 'busy' } }),
      );
    });
    const client = new HttpClient({ name: 'down', baseUrl: target.url });
    await expect(client.get('/v1/down')).rejects.toBeInstanceOf(HttpClientError);
    expect(target.calls).toHaveLength(3);
  });

  it('does not retry a POST without an idempotency key', async () => {
    target = await upstream((app) => {
      app.post('/v1/orders', async (_req, reply) =>
        reply.code(503).send({ error: { code: 'busy', message: 'busy' } }),
      );
    });
    const client = new HttpClient({ name: 'processor', baseUrl: target.url });
    await expect(client.post('/v1/orders', {})).rejects.toMatchObject({ status: 503 });
    expect(target.calls).toHaveLength(1);
  });

  it('retries a POST when an idempotency key is supplied and forwards it', async () => {
    const keys: Array<string | undefined> = [];
    target = await upstream((app) => {
      app.post('/v1/orders', async (request, reply) => {
        keys.push(request.headers['idempotency-key'] as string | undefined);
        return reply.code(503).send({ error: { code: 'busy', message: 'busy' } });
      });
    });
    const client = new HttpClient({ name: 'processor', baseUrl: target.url });
    await expect(client.post('/v1/orders', {}, { idempotencyKey: 'ord_1' })).rejects.toMatchObject({
      status: 503,
    });
    expect(keys).toEqual(['ord_1', 'ord_1', 'ord_1']);
  });

  it('does not retry a 4xx', async () => {
    target = await upstream((app) => {
      app.get('/v1/nope', async (_req, reply) =>
        reply.code(400).send({ error: { code: 'invalid_order', message: 'bad' } }),
      );
    });
    const client = new HttpClient({ name: 'processor', baseUrl: target.url });
    await expect(client.get('/v1/nope')).rejects.toMatchObject({ status: 400 });
    expect(target.calls).toHaveLength(1);
  });

  it('times out and reports a dependency_timeout', async () => {
    target = await upstream((app) => {
      app.get('/v1/slow', async () => {
        await new Promise((resolve) => setTimeout(resolve, 500));
        return { ok: true };
      });
    });
    const client = new HttpClient({ name: 'slow', baseUrl: target.url, timeoutMs: 50, retries: 0 });
    await expect(client.get('/v1/slow')).rejects.toMatchObject({
      status: 0,
      code: 'dependency_timeout',
    });
  });

  it('flags network failures as dependency failures for a circuit breaker', async () => {
    const client = new HttpClient({
      name: 'gone',
      baseUrl: 'http://127.0.0.1:1',
      timeoutMs: 200,
      retries: 0,
    });
    const err = await client.get('/v1/anything').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(HttpClientError);
    expect((err as HttpClientError).isDependencyFailure).toBe(true);
  });

  it('forwards default and per-request headers, plus the request id', async () => {
    const seen: Record<string, string | undefined>[] = [];
    target = await upstream((app) => {
      app.get('/v1/echo', async (request) => {
        seen.push({
          subject: request.headers['x-encabulator-subject'] as string | undefined,
          roles: request.headers['x-encabulator-roles'] as string | undefined,
          requestId: request.headers['x-request-id'] as string | undefined,
        });
        return { ok: true };
      });
    });
    const client = new HttpClient({
      name: 'datastore',
      baseUrl: target.url,
      defaultHeaders: { 'x-encabulator-subject': 'alice' },
    });
    await client
      .withHeaders({ 'x-encabulator-roles': 'customer' })
      .get('/v1/echo', { requestId: 'req-1' });
    expect(seen[0]).toEqual({ subject: 'alice', roles: 'customer', requestId: 'req-1' });
  });

  it('builds query strings and skips undefined values', async () => {
    target = await upstream((app) => {
      app.get('/v1/orders', async () => ({ ok: true }));
    });
    const client = new HttpClient({ name: 'datastore', baseUrl: target.url });
    await client.get('/v1/orders', { query: { limit: 50, status: undefined, customer: 'alice' } });
    expect(target.calls[0]).toBe('GET /v1/orders?limit=50&customer=alice');
  });
});
