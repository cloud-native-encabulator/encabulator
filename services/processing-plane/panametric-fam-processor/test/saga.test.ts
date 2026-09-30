/**
 * The order saga, including every compensation path (design 4.1 and 4.2 step 4).
 *
 * No containers: the neighbours are real Fastify servers with programmable behaviour, and
 * the publisher records instead of talking to JetStream.
 */

import { createLogger, HttpClient, Subjects } from '@encabulator/kit';
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { CircuitBreaker } from '../src/circuit-breaker.js';
import type { LedgerOrder } from '../src/domain.js';
import { type Dependency, OrderSaga } from '../src/saga.js';
import { must, RecordingPublisher, stubFlags, Upstream } from './stubs.js';

const logger = createLogger({ service: 'processor-test', level: 'silent' });

const QUOTE = {
  id: 'qte_1',
  model: 'turbo',
  quantity: 2,
  unitPrice: 1944,
  subtotal: 3888,
  surcharges: [],
  total: 3888,
  currency: 'USD',
  computedAt: '2026-09-30T12:00:00.000Z',
  cached: false,
};

const RESERVATION = {
  id: 'rsv_1',
  orderId: 'ord_placeholder',
  items: [{ sku: 'AMU-BASE', quantity: 2 }],
  expiresAt: '2026-09-30T12:15:00.000Z',
};

function ledgerOrder(id: string): LedgerOrder {
  return {
    id,
    customer: 'alice',
    model: 'turbo',
    quantity: 2,
    options: { dingleArm: false },
    status: 'accepted',
    attempt: 1,
    quote: { id: QUOTE.id, total: QUOTE.total, currency: QUOTE.currency },
    reservationId: RESERVATION.id,
    createdAt: '2026-09-30T12:00:00.000Z',
    updatedAt: '2026-09-30T12:00:00.000Z',
  };
}

describe('OrderSaga', () => {
  let quoter: Upstream;
  let depot: Upstream;
  let datastore: Upstream;
  let publisher: RecordingPublisher;
  let breakers: { quoter: CircuitBreaker; depot: CircuitBreaker; datastore: CircuitBreaker };

  beforeAll(async () => {
    quoter = await new Upstream('spurving-bearing-quoter').start();
    depot = await new Upstream('prefabulated-amulite-depot').start();
    datastore = await new Upstream('hydrocoptic-datastore').start();
  });

  afterAll(async () => {
    await Promise.all([quoter.stop(), depot.stop(), datastore.stop()]);
  });

  afterEach(() => {
    for (const stub of [quoter, depot, datastore]) stub.reset();
  });

  function happyPath(): void {
    quoter.on('POST /v1/quotes', () => ({ status: 200, body: QUOTE }));
    depot.on('POST *', (call) => {
      if (call.url.endsWith('/release')) return { status: 200, body: { status: 'released' } };
      const body = call.body as { orderId: string };
      return { status: 201, body: { ...RESERVATION, orderId: body.orderId } };
    });
    datastore.on('POST /v1/orders', (call) => {
      const body = call.body as { id: string };
      return { status: 201, body: ledgerOrder(body.id) };
    });
  }

  function buildSaga(flags = stubFlags()): OrderSaga {
    publisher = new RecordingPublisher();
    breakers = {
      quoter: new CircuitBreaker({ name: 'spurving-bearing-quoter', logger }),
      depot: new CircuitBreaker({ name: 'prefabulated-amulite-depot', logger }),
      datastore: new CircuitBreaker({ name: 'hydrocoptic-datastore', logger }),
    };
    const dep = (name: string, url: string, breaker: CircuitBreaker): Dependency => ({
      client: new HttpClient({ name, baseUrl: url, timeoutMs: 1_000, retries: 0, logger }),
      breaker,
    });
    return new OrderSaga({
      quoter: dep('spurving-bearing-quoter', quoter.url, breakers.quoter),
      depot: dep('prefabulated-amulite-depot', depot.url, breakers.depot),
      datastore: dep('hydrocoptic-datastore', datastore.url, breakers.datastore),
      publisher: publisher.asPublisher(),
      flags,
      logger,
    });
  }

  // ---------------------------------------------------------------- happy path

  it('quotes, reserves, writes the ledger and publishes, in that order', async () => {
    happyPath();
    const saga = buildSaga();

    const order = await saga.placeOrder({
      customer: 'alice',
      body: { model: 'turbo', quantity: 2, options: { dingleArm: false } },
    });

    expect(order.status).toBe('accepted');
    expect(String(order.id)).toMatch(/^ord_/);
    expect(quoter.callsTo('POST', '/v1/quotes')).toHaveLength(1);
    expect(depot.callsTo('POST', '/v1/reservations')).toHaveLength(1);
    expect(datastore.callsTo('POST', '/v1/orders')).toHaveLength(1);
    expect(publisher.subjects()).toEqual([Subjects.OrderAccepted, Subjects.FabricationRequested]);
  });

  it('passes the order id as the reservation key, so a retry cannot double-reserve', async () => {
    happyPath();
    const saga = buildSaga();
    const order = await saga.placeOrder({
      customer: 'alice',
      body: { model: 'turbo', quantity: 2 },
    });
    const reservationCall = must(depot.callsTo('POST', '/v1/reservations')[0], 'reservation call');
    expect((reservationCall.body as { orderId: string }).orderId).toBe(order.id);
    expect(reservationCall.headers['idempotency-key']).toBe(order.id);
  });

  it('publishes order.accepted with the quoted total', async () => {
    happyPath();
    const saga = buildSaga();
    const order = await saga.placeOrder({
      customer: 'alice',
      body: { model: 'turbo', quantity: 2, options: { dingleArm: true } },
    });
    expect(publisher.dataFor(Subjects.OrderAccepted)).toEqual({
      orderId: order.id,
      customer: 'alice',
      model: 'turbo',
      quantity: 2,
      options: { dingleArm: true },
      total: 3888,
      currency: 'USD',
    });
  });

  it('publishes fabrication.requested for attempt 1 with the reservation id', async () => {
    happyPath();
    const saga = buildSaga();
    const order = await saga.placeOrder({
      customer: 'alice',
      body: { model: 'hyper', quantity: 3 },
    });
    expect(publisher.dataFor(Subjects.FabricationRequested)).toEqual({
      orderId: order.id,
      attempt: 1,
      model: 'hyper',
      quantity: 3,
      options: { dingleArm: false },
      reservationId: 'rsv_1',
    });
  });

  // ------------------------------------------------------------- rejections

  it('rejects an invalid order with 400 invalid_order and calls nobody', async () => {
    happyPath();
    const saga = buildSaga();
    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 99 } }),
    ).rejects.toMatchObject({ statusCode: 400, code: 'invalid_order' });
    expect(quoter.calls).toHaveLength(0);
    expect(depot.calls).toHaveLength(0);
  });

  it('rejects an order with no identity', async () => {
    happyPath();
    const saga = buildSaga();
    await expect(
      saga.placeOrder({ customer: '', body: { model: 'turbo', quantity: 1 } }),
    ).rejects.toMatchObject({ statusCode: 400, code: 'invalid_order' });
    expect(quoter.calls).toHaveLength(0);
  });

  it('rejects retro orders with model_unavailable when the flag is off', async () => {
    happyPath();
    const saga = buildSaga(stubFlags({ 'retro-encabulator-sales': false }));
    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'retro', quantity: 1 } }),
    ).rejects.toMatchObject({ statusCode: 409, code: 'model_unavailable' });
    expect(quoter.calls).toHaveLength(0);

    // Turbo still sells.
    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } }),
    ).resolves.toMatchObject({ status: 'accepted' });
  });

  it('turns the depot 409 into 409 insufficient_parts and forwards the short SKUs', async () => {
    happyPath();
    depot.on('POST /v1/reservations', () => ({
      status: 409,
      body: {
        error: {
          code: 'insufficient_parts',
          message: 'short',
          short: [{ sku: 'HYD-MARZ', available: 1, needed: 6 }],
        },
      },
    }));
    const saga = buildSaga();

    const err = await saga
      .placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } })
      .then(() => undefined)
      .catch((e: unknown) => e as { statusCode: number; code: string; details?: unknown });

    expect(err).toMatchObject({ statusCode: 409, code: 'insufficient_parts' });
    expect(err?.details).toEqual({ short: [{ sku: 'HYD-MARZ', available: 1, needed: 6 }] });
    // Nothing reached the ledger and nothing was published.
    expect(datastore.calls).toHaveLength(0);
    expect(publisher.published).toHaveLength(0);
  });

  it('turns a quoter 400 into 400 invalid_order without reserving anything', async () => {
    happyPath();
    quoter.on('POST /v1/quotes', () => ({
      status: 400,
      body: { error: { code: 'unknown_model', message: 'no such model' } },
    }));
    const saga = buildSaga();

    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } }),
    ).rejects.toMatchObject({ statusCode: 400, code: 'invalid_order' });
    expect(depot.calls).toHaveLength(0);
  });

  // ------------------------------------------------------------ compensation

  it('releases the reservation when the ledger write fails, then returns 503', async () => {
    happyPath();
    datastore.on('POST /v1/orders', () => ({
      status: 500,
      body: { error: { code: 'internal_error', message: 'postgres is having a moment' } },
    }));
    const saga = buildSaga();

    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 2 } }),
    ).rejects.toMatchObject({ statusCode: 503, code: 'dependency_unavailable' });

    // The compensation actually happened, against the reservation we created.
    const releases = depot.callsTo('POST', '/v1/reservations/rsv_1/release');
    expect(releases).toHaveLength(1);
    // And no order event escaped.
    expect(publisher.published).toHaveLength(0);
  });

  it('releases the reservation when the ledger is unreachable', async () => {
    happyPath();
    const saga = buildSaga();
    // Point the datastore client at a closed port by stopping the stub's route.
    datastore.on('POST /v1/orders', () => ({ status: 503, body: {} }));

    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } }),
    ).rejects.toMatchObject({ statusCode: 503 });
    expect(depot.callsTo('POST', '/v1/reservations/rsv_1/release')).toHaveLength(1);
  });

  it('still reports 503 when the compensating release also fails', async () => {
    happyPath();
    datastore.on('POST /v1/orders', () => ({ status: 500, body: {} }));
    depot.on('POST /v1/reservations/rsv_1/release', () => ({ status: 500, body: {} }));
    const saga = buildSaga();

    // The depot's sweeper is the backstop; the customer still gets a clean 503.
    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } }),
    ).rejects.toMatchObject({ statusCode: 503, code: 'dependency_unavailable' });
    expect(depot.callsTo('POST', '/v1/reservations/rsv_1/release')).toHaveLength(1);
  });

  // ------------------------------------------------------- circuit breakers

  it('opens the quoter breaker after five failures and then fails fast', async () => {
    happyPath();
    quoter.on('POST /v1/quotes', () => ({ status: 500, body: {} }));
    const saga = buildSaga();

    for (let i = 0; i < 5; i++) {
      await saga
        .placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } })
        .catch(() => undefined);
    }
    expect(breakers.quoter.currentState()).toBe('open');

    quoter.reset();
    await expect(
      saga.placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } }),
    ).rejects.toMatchObject({ statusCode: 503, code: 'dependency_unavailable' });
    // The open breaker means the quoter was not even called.
    expect(quoter.calls).toHaveLength(0);
  });

  it('does not open the depot breaker on repeated insufficient_parts', async () => {
    happyPath();
    depot.on('POST /v1/reservations', () => ({
      status: 409,
      body: { error: { code: 'insufficient_parts', message: 'short', short: [] } },
    }));
    const saga = buildSaga();

    for (let i = 0; i < 10; i++) {
      await saga
        .placeOrder({ customer: 'alice', body: { model: 'turbo', quantity: 1 } })
        .catch(() => undefined);
    }
    expect(breakers.depot.currentState()).toBe('closed');
  });

  // -------------------------------------------------- inspection.failed path

  it('requests a second attempt after the first failed inspection', async () => {
    happyPath();
    datastore.on('GET /v1/orders/ord_1', () => ({ status: 200, body: ledgerOrder('ord_1') }));
    const saga = buildSaga();

    await saga.handleInspectionFailed({
      orderId: 'ord_1',
      attempt: 1,
      serials: ['ENC-1'],
      findings: [
        {
          serial: 'ENC-1',
          stage: 'fit-marzlevanes',
          metric: 'repleneration',
          score: 4.5,
          reason: 'spike',
        },
      ],
    });

    expect(publisher.subjects()).toEqual([Subjects.FabricationRequested]);
    expect(publisher.dataFor(Subjects.FabricationRequested)).toEqual({
      orderId: 'ord_1',
      attempt: 2,
      model: 'turbo',
      quantity: 2,
      options: { dingleArm: false },
      reservationId: 'rsv_1',
    });
  });

  it('reads the order as an internal operator so the ledger does not filter it out', async () => {
    happyPath();
    datastore.on('GET /v1/orders/ord_1', () => ({ status: 200, body: ledgerOrder('ord_1') }));
    const saga = buildSaga();
    await saga.handleInspectionFailed({ orderId: 'ord_1', attempt: 2 });
    const call = datastore.callsTo('GET', '/v1/orders/ord_1')[0];
    expect(call?.headers['x-encabulator-subject']).toBe('panametric-fam-processor');
    expect(call?.headers['x-encabulator-roles']).toBe('operator');
  });

  it('requests a third attempt after the second failure', async () => {
    happyPath();
    datastore.on('GET /v1/orders/ord_2', () => ({ status: 200, body: ledgerOrder('ord_2') }));
    const saga = buildSaga();
    await saga.handleInspectionFailed({ orderId: 'ord_2', attempt: 2 });
    expect((publisher.dataFor(Subjects.FabricationRequested) as { attempt: number }).attempt).toBe(
      3,
    );
  });

  it('scraps the order on the third failure and never asks for a fourth attempt', async () => {
    happyPath();
    const saga = buildSaga();

    await saga.handleInspectionFailed({
      orderId: 'ord_3',
      attempt: 3,
      serials: ['ENC-1', 'ENC-2'],
      findings: [
        {
          serial: 'ENC-1',
          stage: 'align-spurving-bearings',
          metric: 'marzlevane_alignment',
          score: 6.1,
          reason: 'phase slip',
        },
      ],
    });

    expect(publisher.subjects()).toEqual([Subjects.OrderScrapped]);
    const data = publisher.dataFor(Subjects.OrderScrapped) as {
      orderId: string;
      attempt: number;
      reason: string;
    };
    expect(data.orderId).toBe('ord_3');
    expect(data.attempt).toBe(3);
    expect(data.reason).toContain('marzlevane_alignment');
    // Scrapping needs no ledger read.
    expect(datastore.calls).toHaveLength(0);
  });

  it('drops a failed inspection for an order the ledger has never heard of', async () => {
    happyPath();
    datastore.on('GET /v1/orders/ord_ghost', () => ({
      status: 404,
      body: { error: { code: 'order_not_found', message: 'nope' } },
    }));
    const saga = buildSaga();
    await saga.handleInspectionFailed({ orderId: 'ord_ghost', attempt: 1 });
    expect(publisher.published).toHaveLength(0);
  });

  it('drops an inspection.failed with no orderId', async () => {
    happyPath();
    const saga = buildSaga();
    await saga.handleInspectionFailed({ orderId: '', attempt: 1 });
    expect(publisher.published).toHaveLength(0);
    expect(datastore.calls).toHaveLength(0);
  });

  it('propagates a ledger outage so the message is naked and retried', async () => {
    happyPath();
    datastore.on('GET /v1/orders/ord_4', () => ({ status: 500, body: {} }));
    const saga = buildSaga();
    await expect(saga.handleInspectionFailed({ orderId: 'ord_4', attempt: 1 })).rejects.toThrow();
    expect(publisher.published).toHaveLength(0);
  });
});
