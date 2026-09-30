/**
 * Integration tests for the depot against a real Valkey.
 *
 * The Lua reservation script is the interesting part: it must be all-or-nothing under
 * concurrency, idempotent on `orderId`, and honest about which SKUs are short.
 */

import { createLogger } from '@encabulator/kit';
import type { FastifyInstance } from 'fastify';
import { Redis } from 'ioredis';
import { afterAll, afterEach, beforeAll, expect, it } from 'vitest';
import { billOfMaterials, initialStock, STOCK_CAP } from '../src/catalog.js';
import { Depot, InsufficientPartsError } from '../src/depot.js';
import { PENDING_KEY, reservedKey, stockKey } from '../src/keys.js';
import { describeWithContainers, type StartedValkey, startValkey } from './containers.js';
import { buildTestApp } from './helpers.js';

const logger = createLogger({ service: 'depot-integration', level: 'silent' });

describeWithContainers('Depot against Valkey', () => {
  let valkey: StartedValkey;
  let redis: Redis;
  let depot: Depot;
  let now = Date.UTC(2026, 8, 30, 12, 0, 0);

  beforeAll(async () => {
    valkey = await startValkey();
    redis = new Redis(valkey.url, { maxRetriesPerRequest: 3 });
    depot = new Depot({ redis, logger, now: () => now });
  }, 180_000);

  afterAll(async () => {
    await redis?.quit().catch(() => redis?.disconnect());
    await valkey?.container.stop();
  });

  afterEach(async () => {
    now = Date.UTC(2026, 8, 30, 12, 0, 0);
    await redis.flushdb();
  });

  async function seeded(): Promise<void> {
    await depot.seed();
  }

  it('seeds 1,000 of each SKU and 6,000 marzlevanes, and never overwrites', async () => {
    await seeded();
    const parts = await depot.parts();
    expect(parts.find((p) => p.sku === 'AMU-BASE')?.stock).toBe(1_000);
    expect(parts.find((p) => p.sku === 'HYD-MARZ')?.stock).toBe(6_000);
    expect(parts.every((p) => p.reserved === 0)).toBe(true);

    await redis.set(stockKey('AMU-BASE'), 7);
    await depot.seed();
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(7);
  });

  it('reserves the whole bill of materials atomically', async () => {
    await seeded();
    const { reservation, created } = await depot.reserve({
      orderId: 'ord_1',
      model: 'turbo',
      quantity: 2,
      options: { dingleArm: true },
    });
    expect(created).toBe(true);
    expect(reservation.status).toBe('pending');
    expect(reservation.items).toEqual(billOfMaterials('turbo', 2, { dingleArm: true }));

    const parts = await depot.parts();
    expect(parts.find((p) => p.sku === 'AMU-BASE')).toMatchObject({ stock: 998, reserved: 2 });
    expect(parts.find((p) => p.sku === 'HYD-MARZ')).toMatchObject({ stock: 5_988, reserved: 12 });
    expect(parts.find((p) => p.sku === 'SPV-BRG')).toMatchObject({ stock: 996, reserved: 4 });
    expect(parts.find((p) => p.sku === 'DNG-ARM')).toMatchObject({ stock: 998, reserved: 2 });
  });

  it('is idempotent on orderId', async () => {
    await seeded();
    const first = await depot.reserve({ orderId: 'ord_same', model: 'turbo', quantity: 1 });
    const second = await depot.reserve({ orderId: 'ord_same', model: 'turbo', quantity: 1 });
    expect(second.created).toBe(false);
    expect(second.reservation.id).toBe(first.reservation.id);
    // Stock was only drawn down once.
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(999);
  });

  it('decrements nothing when any single SKU is short', async () => {
    await seeded();
    // Just short of the six marzlevanes a single unit needs.
    await redis.set(stockKey('HYD-MARZ'), 5);
    const before = await depot.parts();

    await expect(
      depot.reserve({ orderId: 'ord_short', model: 'turbo', quantity: 1 }),
    ).rejects.toBeInstanceOf(InsufficientPartsError);

    const after = await depot.parts();
    expect(after).toEqual(before);
    // No reservation, and no index left behind.
    expect(await redis.exists(PENDING_KEY)).toBe(0);
    expect(await depot.reservationForOrder('ord_short')).toBeUndefined();
  });

  it('reports every short SKU, not just the first', async () => {
    await seeded();
    await redis.set(stockKey('HYD-MARZ'), 1);
    await redis.set(stockKey('TRM-PIPE'), 0);

    const err: unknown = await depot
      .reserve({ orderId: 'ord_short2', model: 'turbo', quantity: 1 })
      .then(() => undefined)
      .catch((e: unknown) => e);

    expect(err).toBeInstanceOf(InsufficientPartsError);
    expect((err as InsufficientPartsError).short).toEqual([
      { sku: 'HYD-MARZ', available: 1, needed: 6 },
      { sku: 'TRM-PIPE', available: 0, needed: 3 },
    ]);
  });

  it('never oversells under concurrency', async () => {
    await seeded();
    // Exactly enough base-plates for five units, one per order.
    await redis.set(stockKey('AMU-BASE'), 5);

    const attempts = await Promise.allSettled(
      Array.from({ length: 12 }, (_unused, i) =>
        depot.reserve({ orderId: `ord_race_${i}`, model: 'turbo', quantity: 1 }),
      ),
    );
    const granted = attempts.filter((a) => a.status === 'fulfilled').length;
    const refused = attempts.filter(
      (a) => a.status === 'rejected' && a.reason instanceof InsufficientPartsError,
    ).length;

    expect(granted).toBe(5);
    expect(refused).toBe(7);
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(0);
    expect(Number(await redis.get(reservedKey('AMU-BASE')))).toBe(5);
  });

  it('commits a reservation by consuming the reserved parts', async () => {
    await seeded();
    const { reservation } = await depot.reserve({
      orderId: 'ord_commit',
      model: 'turbo',
      quantity: 2,
    });
    const committed = await depot.commit(reservation.id);
    expect(committed.status).toBe('committed');
    expect(committed.committedAt).toBeTypeOf('string');

    const parts = await depot.parts();
    // Stock stays down and the reservation is released from the "reserved" bucket.
    expect(parts.find((p) => p.sku === 'AMU-BASE')).toMatchObject({ stock: 998, reserved: 0 });
    expect(await redis.zscore(PENDING_KEY, reservation.id)).toBeNull();
  });

  it('treats a second commit as a no-op', async () => {
    await seeded();
    const { reservation } = await depot.reserve({ orderId: 'ord_c2', model: 'turbo', quantity: 1 });
    await depot.commit(reservation.id);
    const again = await depot.commit(reservation.id);
    expect(again.status).toBe('committed');
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(999);
    expect(Number(await redis.get(reservedKey('AMU-BASE')))).toBe(0);
  });

  it('releases a reservation back onto the shelf', async () => {
    await seeded();
    const { reservation } = await depot.reserve({
      orderId: 'ord_release',
      model: 'hyper',
      quantity: 3,
      options: { dingleArm: true },
    });
    const released = await depot.release(reservation.id);
    expect(released.status).toBe('released');

    const parts = await depot.parts();
    for (const part of parts) {
      expect(part.stock).toBe(initialStock(part.sku));
      expect(part.reserved).toBe(0);
    }
    // The order index is dropped, so the same order can be reserved again.
    expect(await depot.reservationForOrder('ord_release')).toBeUndefined();
    const retry = await depot.reserve({ orderId: 'ord_release', model: 'turbo', quantity: 1 });
    expect(retry.created).toBe(true);
  });

  it('refuses to release a committed reservation', async () => {
    await seeded();
    const { reservation } = await depot.reserve({ orderId: 'ord_rc', model: 'turbo', quantity: 1 });
    await depot.commit(reservation.id);
    await expect(depot.release(reservation.id)).rejects.toMatchObject({
      statusCode: 409,
      code: 'reservation_not_pending',
    });
  });

  it('404s for an unknown reservation', async () => {
    await seeded();
    await expect(depot.commit('rsv_nope')).rejects.toMatchObject({
      statusCode: 404,
      code: 'reservation_not_found',
    });
  });

  it('sweeps expired reservations back onto the shelf', async () => {
    await seeded();
    const { reservation } = await depot.reserve({
      orderId: 'ord_expire',
      model: 'turbo',
      quantity: 2,
    });
    expect(await depot.sweepExpired()).toBe(0);

    // Fifteen minutes and one second later.
    now += 15 * 60 * 1000 + 1_000;
    expect(await depot.sweepExpired()).toBe(1);

    const swept = await depot.getReservation(reservation.id);
    expect(swept?.status).toBe('expired');
    const parts = await depot.parts();
    expect(parts.find((p) => p.sku === 'AMU-BASE')).toMatchObject({ stock: 1_000, reserved: 0 });
    // Nothing left to sweep.
    expect(await depot.sweepExpired()).toBe(0);
  });

  it('leaves committed reservations alone when sweeping', async () => {
    await seeded();
    const { reservation } = await depot.reserve({ orderId: 'ord_cs', model: 'turbo', quantity: 1 });
    await depot.commit(reservation.id);
    now += 20 * 60 * 1000;
    expect(await depot.sweepExpired()).toBe(0);
    expect((await depot.getReservation(reservation.id))?.status).toBe('committed');
  });

  it('runs the foundry up to the cap and no further', async () => {
    await seeded();
    await redis.set(stockKey('AMU-BASE'), STOCK_CAP - 5);
    await depot.runFoundry(20);
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(STOCK_CAP);

    await depot.runFoundry(20);
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(STOCK_CAP);
  });

  it('pours six marzlevanes for every one of everything else', async () => {
    await seeded();
    await redis.mset({ [stockKey('AMU-BASE')]: 0, [stockKey('HYD-MARZ')]: 0 });
    await depot.runFoundry(20);
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(20);
    expect(Number(await redis.get(stockKey('HYD-MARZ')))).toBe(120);
  });

  it('stops the foundry at rate 0', async () => {
    await seeded();
    await redis.set(stockKey('AMU-BASE'), 1);
    await depot.runFoundry(0);
    expect(Number(await redis.get(stockKey('AMU-BASE')))).toBe(1);
  });

  it('never trims a SKU that is already over the cap', async () => {
    await seeded();
    // The marzlevane bin starts above the cap by design.
    await depot.runFoundry(20);
    expect(Number(await redis.get(stockKey('HYD-MARZ')))).toBe(6_000);
  });

  it('restocks a SKU on request', async () => {
    await seeded();
    const level = await depot.restock('AMU-BASE', 250);
    expect(level.stock).toBe(1_250);
    await expect(depot.restock('NOPE', 1)).rejects.toMatchObject({ code: 'unknown_sku' });
  });
});

describeWithContainers('Depot HTTP API', () => {
  let valkey: StartedValkey;
  let redis: Redis;
  let app: FastifyInstance;

  beforeAll(async () => {
    valkey = await startValkey();
    redis = new Redis(valkey.url, { maxRetriesPerRequest: 3 });
    const depot = new Depot({ redis, logger });
    await depot.seed();
    app = await buildTestApp(depot);
  }, 180_000);

  afterAll(async () => {
    await app?.close();
    await redis?.quit().catch(() => redis?.disconnect());
    await valkey?.container.stop();
  });

  it('lists the parts cage', async () => {
    const res = await app.inject({ method: 'GET', url: '/v1/parts' });
    expect(res.statusCode).toBe(200);
    const parts = res.json() as Array<{ sku: string; stock: number; perUnit: number }>;
    expect(parts).toHaveLength(10);
    expect(parts[0]).toMatchObject({ sku: 'AMU-BASE', perUnit: 1 });
  });

  it('creates a reservation with 201 and replays it with 200', async () => {
    const body = { orderId: 'ord_http_1', model: 'turbo', quantity: 1, options: {} };
    const first = await app.inject({ method: 'POST', url: '/v1/reservations', payload: body });
    expect(first.statusCode).toBe(201);
    expect(first.json()).toMatchObject({ orderId: 'ord_http_1', status: 'pending' });
    expect(String(first.json().id)).toMatch(/^rsv_/);

    const replay = await app.inject({ method: 'POST', url: '/v1/reservations', payload: body });
    expect(replay.statusCode).toBe(200);
    expect(replay.json().id).toBe(first.json().id);
  });

  it('returns 409 insufficient_parts with the short SKUs', async () => {
    await redis.set(stockKey('LOT-WIND'), 0);
    const res = await app.inject({
      method: 'POST',
      url: '/v1/reservations',
      payload: { orderId: 'ord_http_short', model: 'turbo', quantity: 1 },
    });
    expect(res.statusCode).toBe(409);
    expect(res.json().error.code).toBe('insufficient_parts');
    expect(res.json().error.short).toEqual([{ sku: 'LOT-WIND', available: 0, needed: 1 }]);
    await redis.set(stockKey('LOT-WIND'), 1_000);
  });

  it('rejects a bad model or quantity with 400', async () => {
    const badModel = await app.inject({
      method: 'POST',
      url: '/v1/reservations',
      payload: { orderId: 'ord_bad', model: 'flux-capacitor', quantity: 1 },
    });
    expect(badModel.statusCode).toBe(400);

    const badQuantity = await app.inject({
      method: 'POST',
      url: '/v1/reservations',
      payload: { orderId: 'ord_bad', model: 'turbo', quantity: 99 },
    });
    expect(badQuantity.statusCode).toBe(400);
  });

  it('commits and releases through the API', async () => {
    const created = await app.inject({
      method: 'POST',
      url: '/v1/reservations',
      payload: { orderId: 'ord_http_commit', model: 'retro', quantity: 1 },
    });
    const id = created.json().id as string;

    const committed = await app.inject({
      method: 'POST',
      url: `/v1/reservations/${id}/commit`,
    });
    expect(committed.statusCode).toBe(200);
    expect(committed.json().status).toBe('committed');

    const released = await app.inject({ method: 'POST', url: `/v1/reservations/${id}/release` });
    expect(released.statusCode).toBe(409);
  });

  it('requires the operator role to restock', async () => {
    const asCustomer = await app.inject({
      method: 'POST',
      url: '/v1/parts/AMU-BASE/restock',
      payload: { quantity: 10 },
      headers: { 'x-encabulator-subject': 'alice', 'x-encabulator-roles': 'customer' },
    });
    expect(asCustomer.statusCode).toBe(403);

    const asOperator = await app.inject({
      method: 'POST',
      url: '/v1/parts/AMU-BASE/restock',
      payload: { quantity: 10 },
      headers: { 'x-encabulator-subject': 'ops', 'x-encabulator-roles': 'operator' },
    });
    expect(asOperator.statusCode).toBe(200);
    expect(asOperator.json().sku).toBe('AMU-BASE');
  });

  it('exposes the stock gauge on /metrics', async () => {
    await app.inject({ method: 'GET', url: '/v1/parts' });
    const res = await app.inject({ method: 'GET', url: '/metrics' });
    expect(res.body).toContain('encabulator_parts_stock{sku="AMU-BASE"}');
    expect(res.body).toContain('encabulator_reservations_total');
  });

  it('reports readiness from Valkey', async () => {
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(200);
    expect(res.json().checks).toContain('valkey');
  });
});
