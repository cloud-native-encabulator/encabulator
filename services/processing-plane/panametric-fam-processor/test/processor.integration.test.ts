/**
 * The processor end to end against real NATS: the HTTP route, the saga, real JetStream
 * publishes, and the `inspection.failed` consumer.
 *
 * The three neighbours are Fastify stubs on real sockets. Everything else is the real thing.
 */

import {
  connectNats,
  consume,
  createApp,
  createLogger,
  Durables,
  EventPublisher,
  ensureStream,
  HttpClient,
  type JetStreamManager,
  type NatsConnection,
  natsReadiness,
  STREAM_NAME,
  Subjects,
  type Subscription,
} from '@encabulator/kit';
import type { FastifyInstance } from 'fastify';
import { afterAll, afterEach, beforeAll, expect, it } from 'vitest';
import { CircuitBreaker } from '../src/circuit-breaker.js';
import type { InspectionFailed, LedgerOrder } from '../src/domain.js';
import { registerRoutes } from '../src/routes.js';
import { type Dependency, OrderSaga } from '../src/saga.js';
import { describeWithContainers, type StartedNats, startNats } from './containers.js';
import { must, stubFlags, Upstream } from './stubs.js';

const logger = createLogger({ service: 'processor-integration', level: 'silent' });

const QUOTE = {
  id: 'qte_int',
  model: 'turbo',
  quantity: 1,
  unitPrice: 1944,
  subtotal: 1944,
  surcharges: [],
  total: 1944,
  currency: 'USD',
  computedAt: '2026-09-30T12:00:00.000Z',
  cached: false,
};

function ledgerOrder(id: string): LedgerOrder {
  return {
    id,
    customer: 'alice',
    model: 'turbo',
    quantity: 1,
    options: { dingleArm: false },
    status: 'accepted',
    attempt: 1,
    quote: { id: QUOTE.id, total: QUOTE.total, currency: QUOTE.currency },
    reservationId: 'rsv_int',
    createdAt: '2026-09-30T12:00:00.000Z',
    updatedAt: '2026-09-30T12:00:00.000Z',
  };
}

describeWithContainers('panametric-fam-processor over real JetStream', () => {
  let nats: StartedNats;
  let nc: NatsConnection;
  let jsm: JetStreamManager;
  let app: FastifyInstance;
  let subscription: Subscription;
  let quoter: Upstream;
  let depot: Upstream;
  let datastore: Upstream;

  beforeAll(async () => {
    nats = await startNats();
    nc = await connectNats({ url: nats.url, name: 'processor-integration' });
    jsm = await ensureStream(nc, logger);

    [quoter, depot, datastore] = await Promise.all([
      new Upstream('spurving-bearing-quoter').start(),
      new Upstream('prefabulated-amulite-depot').start(),
      new Upstream('hydrocoptic-datastore').start(),
    ]);

    const dep = (name: string, url: string): Dependency => ({
      client: new HttpClient({ name, baseUrl: url, timeoutMs: 2_000, retries: 0, logger }),
      breaker: new CircuitBreaker({ name, logger }),
    });

    const saga = new OrderSaga({
      quoter: dep('spurving-bearing-quoter', quoter.url),
      depot: dep('prefabulated-amulite-depot', depot.url),
      datastore: dep('hydrocoptic-datastore', datastore.url),
      publisher: new EventPublisher(nc, { source: 'panametric-fam-processor', logger }),
      flags: stubFlags(),
      logger,
    });

    subscription = await consume(nc, jsm, {
      spec: { durable: Durables.Processor, filterSubject: Subjects.InspectionFailed },
      logger,
      handler: async ({ envelope }) => {
        await saga.handleInspectionFailed(envelope.data as InspectionFailed);
      },
    });

    app = await createApp({
      serviceName: 'panametric-fam-processor',
      logger,
      readiness: [natsReadiness(nc)],
      register: async (instance) => registerRoutes(instance, saga),
    });
    await app.ready();
  }, 240_000);

  afterAll(async () => {
    await subscription?.stop();
    await app?.close();
    await nc?.close();
    await Promise.all([quoter?.stop(), depot?.stop(), datastore?.stop()]);
    await nats?.container.stop();
  });

  afterEach(async () => {
    for (const stub of [quoter, depot, datastore]) stub.reset();
  });

  function happyPath(): void {
    quoter.on('POST /v1/quotes', () => ({ status: 200, body: QUOTE }));
    depot.on('POST *', (call) => {
      if (call.url.endsWith('/release')) return { status: 200, body: { status: 'released' } };
      const body = call.body as { orderId: string };
      return {
        status: 201,
        body: { id: 'rsv_int', orderId: body.orderId, items: [], expiresAt: '' },
      };
    });
    datastore.on('POST /v1/orders', (call) => ({
      status: 201,
      body: ledgerOrder((call.body as { id: string }).id),
    }));
  }

  async function eventsFor(subject: string): Promise<Array<Record<string, unknown>>> {
    const info = await jsm.streams.info(STREAM_NAME);
    const out: Array<Record<string, unknown>> = [];
    for (let seq = info.state.first_seq; seq <= info.state.last_seq; seq++) {
      const msg = await jsm.streams.getMessage(STREAM_NAME, { seq }).catch(() => null);
      if (!msg || msg.subject !== subject) continue;
      out.push(JSON.parse(new TextDecoder().decode(msg.data)) as Record<string, unknown>);
    }
    return out;
  }

  it('POST /v1/orders returns 202 and lands both events in the stream', async () => {
    happyPath();
    const res = await app.inject({
      method: 'POST',
      url: '/v1/orders',
      payload: { model: 'turbo', quantity: 1, options: { dingleArm: false } },
      headers: { 'x-encabulator-subject': 'alice', 'x-encabulator-roles': 'customer' },
    });

    expect(res.statusCode).toBe(202);
    const order = res.json() as LedgerOrder;
    expect(order.status).toBe('accepted');

    const accepted = await eventsFor(Subjects.OrderAccepted);
    const requested = await eventsFor(Subjects.FabricationRequested);
    expect(accepted.some((e) => (e.data as { orderId: string }).orderId === order.id)).toBe(true);
    expect(
      requested.some(
        (e) =>
          (e.data as { orderId: string; attempt: number }).orderId === order.id &&
          (e.data as { attempt: number }).attempt === 1,
      ),
    ).toBe(true);
  });

  it('sets the envelope type, source and a dedupe id on every event', async () => {
    happyPath();
    await app.inject({
      method: 'POST',
      url: '/v1/orders',
      payload: { model: 'turbo', quantity: 1 },
      headers: { 'x-encabulator-subject': 'bob' },
    });
    const accepted = await eventsFor(Subjects.OrderAccepted);
    const latest = accepted.at(-1);
    expect(latest?.type).toBe(Subjects.OrderAccepted);
    expect(latest?.source).toBe('panametric-fam-processor');
    expect(String(latest?.id)).toMatch(/^evt_/);
    expect(String(latest?.time)).toMatch(/^\d{4}-\d{2}-\d{2}T/);
  });

  it('takes the customer from the identity header, not the body', async () => {
    happyPath();
    const res = await app.inject({
      method: 'POST',
      url: '/v1/orders',
      payload: { model: 'turbo', quantity: 1, customer: 'mallory' },
      headers: { 'x-encabulator-subject': 'alice' },
    });
    expect(res.statusCode).toBe(202);
    const ledgerCalls = datastore.callsTo('POST', '/v1/orders');
    const ledgerCall = must(ledgerCalls[ledgerCalls.length - 1], 'ledger call');
    expect((ledgerCall.body as { customer: string }).customer).toBe('alice');
  });

  it('returns 409 insufficient_parts without writing the ledger', async () => {
    happyPath();
    depot.on('POST /v1/reservations', () => ({
      status: 409,
      body: { error: { code: 'insufficient_parts', message: 'short', short: [] } },
    }));
    const res = await app.inject({
      method: 'POST',
      url: '/v1/orders',
      payload: { model: 'turbo', quantity: 1 },
      headers: { 'x-encabulator-subject': 'alice' },
    });
    expect(res.statusCode).toBe(409);
    expect(res.json().error.code).toBe('insufficient_parts');
    expect(datastore.calls).toHaveLength(0);
  });

  it('returns 400 invalid_order in the standard error shape', async () => {
    happyPath();
    const res = await app.inject({
      method: 'POST',
      url: '/v1/orders',
      payload: { model: 'gyroscopic-marzlevane', quantity: 1 },
      headers: { 'x-encabulator-subject': 'alice' },
    });
    expect(res.statusCode).toBe(400);
    expect(res.json()).toEqual({
      error: { code: 'invalid_order', message: expect.stringContaining('model must be one of') },
    });
  });

  it('consumes inspection.failed and publishes the next fabrication attempt', async () => {
    happyPath();
    datastore.on('GET /v1/orders/ord_refab', () => ({
      status: 200,
      body: ledgerOrder('ord_refab'),
    }));

    const publisher = new EventPublisher(nc, { source: 'side-fumbling-detector', logger });
    await publisher.publish(Subjects.InspectionFailed, {
      orderId: 'ord_refab',
      attempt: 1,
      serials: ['ENC-turbo-int-0001'],
      findings: [
        {
          serial: 'ENC-turbo-int-0001',
          stage: 'fit-marzlevanes',
          metric: 'repleneration',
          score: 4.4,
          reason: 'spike',
        },
      ],
    });

    await waitFor(async () => {
      const requested = await eventsFor(Subjects.FabricationRequested);
      return requested.some((e) => {
        const data = e.data as { orderId: string; attempt: number };
        return data.orderId === 'ord_refab' && data.attempt === 2;
      });
    });
  });

  it('consumes a third inspection.failed and scraps the order', async () => {
    happyPath();
    const publisher = new EventPublisher(nc, { source: 'side-fumbling-detector', logger });
    await publisher.publish(Subjects.InspectionFailed, {
      orderId: 'ord_scrap',
      attempt: 3,
      serials: ['ENC-turbo-int-0002'],
      findings: [],
    });

    await waitFor(async () => {
      const scrapped = await eventsFor(Subjects.OrderScrapped);
      return scrapped.some((e) => (e.data as { orderId: string }).orderId === 'ord_scrap');
    });
    // No fourth attempt was requested.
    const requested = await eventsFor(Subjects.FabricationRequested);
    expect(requested.some((e) => (e.data as { orderId: string }).orderId === 'ord_scrap')).toBe(
      false,
    );
  });

  it('is ready while NATS is up', async () => {
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(200);
    expect(res.json().checks).toEqual(['nats']);
  });

  it('reports the order metrics on /metrics', async () => {
    happyPath();
    await app.inject({
      method: 'POST',
      url: '/v1/orders',
      payload: { model: 'hyper', quantity: 1 },
      headers: { 'x-encabulator-subject': 'alice' },
    });
    const res = await app.inject({ method: 'GET', url: '/metrics' });
    expect(res.body).toContain('encabulator_orders_total{model="hyper",result="accepted"}');
    expect(res.body).toContain('encabulator_refabrications_total');
    expect(res.body).toContain('encabulator_orders_scrapped_total');
    expect(res.body).toContain('http_route="/v1/orders"');
  });
});

async function waitFor(predicate: () => Promise<boolean>, timeoutMs = 15_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error('condition not met before the deadline');
}
