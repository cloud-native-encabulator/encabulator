/**
 * Integration tests for the dispatch office against real Valkey and real NATS.
 *
 * The scheduling and the atomic claim are the parts worth testing for real: a shipment must
 * be booked once per order, delivered once when its transit time is up, and survive a
 * restart because the schedule lives in Valkey rather than in a timer.
 */

import {
  connectNats,
  createApp,
  createLogger,
  EventPublisher,
  ensureStream,
  type JetStreamManager,
  type NatsConnection,
  natsReadiness,
  STREAM_NAME,
  Subjects,
} from '@encabulator/kit';
import type { FastifyInstance } from 'fastify';
import { Redis } from 'ioredis';
import { afterAll, afterEach, beforeAll, expect, it } from 'vitest';
import { DispatchOffice, type Shipment } from '../src/dispatch.js';
import { DUE_KEY, deliveredKey, RECENT_KEY, shipmentKey } from '../src/keys.js';
import { registerRoutes } from '../src/routes.js';
import { valkeyReadiness } from '../src/valkey.js';
import {
  describeWithContainers,
  type StartedNats,
  type StartedValkey,
  startNats,
  startValkey,
} from './containers.js';

const logger = createLogger({ service: 'dispatch-integration', level: 'silent' });

describeWithContainers('DispatchOffice', () => {
  let valkey: StartedValkey;
  let nats: StartedNats;
  let redis: Redis;
  let nc: NatsConnection;
  let jsm: JetStreamManager;
  let office: DispatchOffice;
  let now = Date.UTC(2026, 8, 30, 12, 0, 0);

  beforeAll(async () => {
    [valkey, nats] = await Promise.all([startValkey(), startNats()]);
    redis = new Redis(valkey.url, { maxRetriesPerRequest: 3 });
    nc = await connectNats({ url: nats.url, name: 'dispatch-integration' });
    jsm = await ensureStream(nc, logger);
    const publisher = new EventPublisher(nc, { source: 'nofer-trunnion-dispatch', logger });
    office = new DispatchOffice({
      redis,
      publisher,
      logger,
      now: () => now,
      // Deterministic carrier and tracking number.
      random: () => 0.5,
    });
  }, 240_000);

  afterAll(async () => {
    await nc?.close();
    await redis?.quit().catch(() => redis?.disconnect());
    await Promise.all([valkey?.container.stop(), nats?.container.stop()]);
  });

  afterEach(async () => {
    now = Date.UTC(2026, 8, 30, 12, 0, 0);
    await redis.flushdb();
    await jsm.streams.purge(STREAM_NAME).catch(() => undefined);
  });

  async function storedEvents(subject: string): Promise<Array<Record<string, unknown>>> {
    const info = await jsm.streams.info(STREAM_NAME);
    const events: Array<Record<string, unknown>> = [];
    for (let seq = 1; seq <= info.state.last_seq; seq++) {
      const msg = await jsm.streams.getMessage(STREAM_NAME, { seq }).catch(() => null);
      if (!msg || msg.subject !== subject) continue;
      events.push(JSON.parse(new TextDecoder().decode(msg.data)) as Record<string, unknown>);
    }
    return events;
  }

  it('books a shipment, schedules delivery, and publishes shipment.dispatched', async () => {
    const { shipment, created } = await office.dispatch({
      orderId: 'ord_ship_1',
      attempt: 1,
      serials: ['ENC-turbo-abc-0001'],
    });

    expect(created).toBe(true);
    expect(shipment.status).toBe('in_transit');
    expect(shipment.carrier).toBe('Grammeter Parcel'); // random() === 0.5
    expect(shipment.trackingNumber).toMatch(/^NT\d{10}$/);
    expect(shipment.etaSeconds).toBeGreaterThanOrEqual(10);
    expect(shipment.etaSeconds).toBeLessThanOrEqual(60);
    expect(String(shipment.id)).toMatch(/^shp_/);

    // The schedule is durable, in the sorted set, not in a timer.
    const score = await redis.zscore(DUE_KEY, 'ord_ship_1');
    expect(Number(score)).toBe(now + shipment.etaSeconds * 1_000);

    const events = await storedEvents(Subjects.ShipmentDispatched);
    expect(events).toHaveLength(1);
    expect(events[0]?.data).toEqual({
      orderId: 'ord_ship_1',
      carrier: shipment.carrier,
      trackingNumber: shipment.trackingNumber,
      etaSeconds: shipment.etaSeconds,
    });
  });

  it('is idempotent: a redelivered inspection.passed does not re-dispatch', async () => {
    const first = await office.dispatch({ orderId: 'ord_dupe', serials: ['ENC-1'] });
    const second = await office.dispatch({ orderId: 'ord_dupe', serials: ['ENC-1'] });

    expect(second.created).toBe(false);
    expect(second.shipment.id).toBe(first.shipment.id);
    expect(second.shipment.trackingNumber).toBe(first.shipment.trackingNumber);
    expect(await redis.llen(RECENT_KEY)).toBe(1);
    expect(await storedEvents(Subjects.ShipmentDispatched)).toHaveLength(1);
  });

  it('delivers nothing before the transit time is up', async () => {
    const { shipment } = await office.dispatch({ orderId: 'ord_early', serials: ['ENC-1'] });
    now += (shipment.etaSeconds - 1) * 1_000;
    expect(await office.deliverDue()).toBe(0);
    expect((await office.get('ord_early'))?.status).toBe('in_transit');
  });

  it('delivers once the transit time has elapsed and publishes shipment.delivered', async () => {
    const { shipment } = await office.dispatch({ orderId: 'ord_late', serials: ['ENC-1'] });
    now += shipment.etaSeconds * 1_000;

    expect(await office.deliverDue()).toBe(1);
    const delivered = await office.get('ord_late');
    expect(delivered?.status).toBe('delivered');
    expect(delivered?.deliveredAt).toBeTypeOf('string');

    const events = await storedEvents(Subjects.ShipmentDelivered);
    expect(events).toHaveLength(1);
    expect(events[0]?.data).toEqual({
      orderId: 'ord_late',
      carrier: shipment.carrier,
      trackingNumber: shipment.trackingNumber,
    });
  });

  it('delivers each shipment exactly once, even from two offices at once', async () => {
    const publisher = new EventPublisher(nc, { source: 'nofer-trunnion-dispatch', logger });
    const replica = new DispatchOffice({ redis, publisher, logger, now: () => now });

    for (let i = 0; i < 10; i++) {
      await office.dispatch({ orderId: `ord_race_${i}`, serials: [`ENC-${i}`] });
    }
    now += 61_000;

    const [a, b] = await Promise.all([office.deliverDue(), replica.deliverDue()]);
    expect(a + b).toBe(10);
    expect(await redis.zcard(DUE_KEY)).toBe(0);
    expect(await storedEvents(Subjects.ShipmentDelivered)).toHaveLength(10);
  });

  it('does not deliver the same shipment twice on a later tick', async () => {
    await office.dispatch({ orderId: 'ord_twice', serials: ['ENC-1'] });
    now += 61_000;
    expect(await office.deliverDue()).toBe(1);
    expect(await office.deliverDue()).toBe(0);
    expect(await storedEvents(Subjects.ShipmentDelivered)).toHaveLength(1);
  });

  it('survives a restart mid-transit, because the schedule is in Valkey', async () => {
    await office.dispatch({ orderId: 'ord_restart', serials: ['ENC-1'] });

    // A brand new office object, as if the pod had just come back.
    const publisher = new EventPublisher(nc, { source: 'nofer-trunnion-dispatch', logger });
    const restarted = new DispatchOffice({ redis, publisher, logger, now: () => now });
    now += 61_000;

    expect(await restarted.deliverDue()).toBe(1);
    expect((await restarted.get('ord_restart'))?.status).toBe('delivered');
  });

  it('re-arms a delivery whose event could not be published', async () => {
    const failing = new DispatchOffice({
      redis,
      // A publisher that always throws, standing in for a NATS outage.
      publisher: {
        publish: async () => {
          throw new Error('nats unreachable');
        },
      } as unknown as EventPublisher,
      logger,
      now: () => now,
    });
    // Book the shipment with the working publisher, then try to deliver with the broken one.
    await office.dispatch({ orderId: 'ord_rearm', serials: ['ENC-1'] });
    now += 61_000;

    expect(await failing.deliverDue()).toBe(0);
    // Still in transit, and back in the queue for a later attempt.
    expect((await office.get('ord_rearm'))?.status).toBe('in_transit');
    expect(await redis.zcard(DUE_KEY)).toBe(1);

    now += 6_000;
    expect(await office.deliverDue()).toBe(1);
  });

  it('drops a due entry whose record has vanished', async () => {
    await office.dispatch({ orderId: 'ord_orphan', serials: ['ENC-1'] });
    await redis.del(shipmentKey('ord_orphan'));
    now += 61_000;
    expect(await office.deliverDue()).toBe(0);
    expect(await redis.zcard(DUE_KEY)).toBe(0);
  });

  it('tracks how many shipments are in transit', async () => {
    expect(await office.inTransitCount()).toBe(0);
    await office.dispatch({ orderId: 'ord_a', serials: ['ENC-1'] });
    await office.dispatch({ orderId: 'ord_b', serials: ['ENC-2'] });
    expect(await office.inTransitCount()).toBe(2);
    now += 61_000;
    await office.deliverDue();
    expect(await office.inTransitCount()).toBe(0);
  });

  it('lists recent shipments newest first and caps the list', async () => {
    for (let i = 0; i < 5; i++) {
      await office.dispatch({ orderId: `ord_recent_${i}`, serials: [`ENC-${i}`] });
    }
    const recent = await office.recent(3);
    expect(recent.map((s) => s.orderId)).toEqual(['ord_recent_4', 'ord_recent_3', 'ord_recent_2']);
  });

  it('reports the delivered status in the recent list', async () => {
    await office.dispatch({ orderId: 'ord_mix_1', serials: ['ENC-1'] });
    now += 61_000;
    await office.deliverDue();
    await office.dispatch({ orderId: 'ord_mix_2', serials: ['ENC-2'] });

    const statuses = Object.fromEntries(
      (await office.recent(10)).map((s: Shipment) => [s.orderId, s.status]),
    );
    expect(statuses).toEqual({ ord_mix_1: 'delivered', ord_mix_2: 'in_transit' });
  });

  it('cleans up the delivered marker if the record disappeared', async () => {
    await office.dispatch({ orderId: 'ord_marker', serials: ['ENC-1'] });
    await redis.del(shipmentKey('ord_marker'));
    now += 61_000;
    await office.deliverDue();
    expect(await redis.exists(deliveredKey('ord_marker'))).toBe(0);
  });
});

describeWithContainers('Dispatch HTTP API', () => {
  let valkey: StartedValkey;
  let nats: StartedNats;
  let redis: Redis;
  let nc: NatsConnection;
  let app: FastifyInstance;
  let office: DispatchOffice;
  const now = Date.UTC(2026, 8, 30, 12, 0, 0);

  beforeAll(async () => {
    [valkey, nats] = await Promise.all([startValkey(), startNats()]);
    redis = new Redis(valkey.url, { maxRetriesPerRequest: 3 });
    nc = await connectNats({ url: nats.url, name: 'dispatch-http' });
    await ensureStream(nc, logger);
    office = new DispatchOffice({
      redis,
      publisher: new EventPublisher(nc, { source: 'nofer-trunnion-dispatch', logger }),
      logger,
      now: () => now,
    });
    app = await createApp({
      serviceName: 'nofer-trunnion-dispatch',
      logger,
      readiness: [valkeyReadiness(redis), natsReadiness(nc)],
      register: async (instance) => registerRoutes(instance, office),
    });
    await app.ready();
  }, 240_000);

  afterAll(async () => {
    await app?.close();
    await nc?.close();
    await redis?.quit().catch(() => redis?.disconnect());
    await Promise.all([valkey?.container.stop(), nats?.container.stop()]);
  });

  it('404s for an order with no shipment', async () => {
    const res = await app.inject({ method: 'GET', url: '/v1/shipments/ord_nope' });
    expect(res.statusCode).toBe(404);
    expect(res.json().error.code).toBe('shipment_not_found');
  });

  it('returns a shipment by order id', async () => {
    await office.dispatch({ orderId: 'ord_http_1', serials: ['ENC-1'] });
    const res = await app.inject({ method: 'GET', url: '/v1/shipments/ord_http_1' });
    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ orderId: 'ord_http_1', status: 'in_transit' });
  });

  it('lists recent shipments with a limit', async () => {
    for (let i = 0; i < 4; i++) {
      await office.dispatch({ orderId: `ord_http_list_${i}`, serials: ['ENC-1'] });
    }
    const res = await app.inject({ method: 'GET', url: '/v1/shipments?limit=2' });
    expect(res.statusCode).toBe(200);
    expect(res.json().shipments).toHaveLength(2);
  });

  it('rejects a nonsense limit', async () => {
    const res = await app.inject({ method: 'GET', url: '/v1/shipments?limit=9999' });
    expect(res.statusCode).toBe(400);
  });

  it('is ready when Valkey and NATS are both up', async () => {
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(200);
    expect(res.json().checks).toEqual(['valkey', 'nats']);
  });
});
