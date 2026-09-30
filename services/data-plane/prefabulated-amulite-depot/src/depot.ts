/**
 * The parts cage itself: everything that talks to Valkey.
 *
 * Reservations are all-or-nothing, idempotent on `orderId`, and expire after fifteen
 * minutes if nobody commits them (design 8.7).
 */

import { conflict, type Logger, newId, notFound, nowRfc3339 } from '@encabulator/kit';
import type { Redis } from 'ioredis';
import {
  type BomItem,
  billOfMaterials,
  CATALOG,
  findPart,
  foundryIncrement,
  initialStock,
  type OrderOptions,
  SKUS,
  STOCK_CAP,
} from './catalog.js';
import {
  orderIndexKey,
  PENDING_KEY,
  reservationKey,
  reservedKey,
  stockKey,
  stockKeyPairs,
} from './keys.js';
import { COMMIT_SCRIPT, FOUNDRY_SCRIPT, RELEASE_SCRIPT, RESERVE_SCRIPT } from './scripts.js';

export const RESERVATION_TTL_MS = 15 * 60 * 1000;
/** Reservation records outlive their pending window so a late commit gets a real answer. */
export const RESERVATION_KEY_TTL_SECONDS = 24 * 60 * 60;
export const FOUNDRY_INTERVAL_MS = 10_000;
export const SWEEP_INTERVAL_MS = 30_000;
export const SWEEP_BATCH = 100;

export type ReservationStatus = 'pending' | 'committed' | 'released' | 'expired';

export interface Reservation {
  id: string;
  orderId: string;
  model: string;
  quantity: number;
  options: OrderOptions;
  items: BomItem[];
  status: ReservationStatus;
  createdAt: string;
  expiresAt: string;
  committedAt?: string;
  releasedAt?: string;
}

export interface PartStock {
  sku: string;
  name: string;
  stock: number;
  reserved: number;
  perUnit: number;
}

export interface ShortSku {
  sku: string;
  available: number;
  needed: number;
}

export class InsufficientPartsError extends Error {
  readonly short: ShortSku[];
  constructor(short: ShortSku[]) {
    super(
      `insufficient parts: ${short.map((s) => `${s.sku} ${s.available}/${s.needed}`).join(', ')}`,
    );
    this.name = 'InsufficientPartsError';
    this.short = short;
  }
}

/** ioredis `defineCommand` adds methods dynamically; this is the shape we rely on. */
type ScriptedRedis = Redis & {
  depotReserve(numKeys: number, ...args: (string | number)[]): Promise<string[]>;
  depotCommit(numKeys: number, ...args: (string | number)[]): Promise<string[]>;
  depotRelease(numKeys: number, ...args: (string | number)[]): Promise<string[]>;
  depotFoundry(numKeys: number, ...args: (string | number)[]): Promise<string[]>;
};

export interface DepotOptions {
  redis: Redis;
  logger: Logger;
  /** Injected so tests can freeze time. */
  now?: () => number;
  reservationTtlMs?: number;
  /** Called whenever stock levels change, so the gauge stays honest. */
  onStockChange?: (levels: PartStock[]) => void;
}

export class Depot {
  private readonly redis: ScriptedRedis;
  private readonly logger: Logger;
  private readonly now: () => number;
  private readonly reservationTtlMs: number;
  private readonly onStockChange: ((levels: PartStock[]) => void) | undefined;

  constructor(opts: DepotOptions) {
    this.redis = opts.redis as ScriptedRedis;
    this.logger = opts.logger;
    this.now = opts.now ?? (() => Date.now());
    this.reservationTtlMs = opts.reservationTtlMs ?? RESERVATION_TTL_MS;
    this.onStockChange = opts.onStockChange;

    // No `numberOfKeys`: the BOM decides how many keys each call touches, so the count is
    // passed at call time.
    this.redis.defineCommand('depotReserve', { lua: RESERVE_SCRIPT });
    this.redis.defineCommand('depotCommit', { lua: COMMIT_SCRIPT });
    this.redis.defineCommand('depotRelease', { lua: RELEASE_SCRIPT });
    this.redis.defineCommand('depotFoundry', { lua: FOUNDRY_SCRIPT });
  }

  /**
   * Seed the catalog on first start. `SETNX` never overwrites an existing level, so this is
   * safe to run on every replica and on every restart. Throws if Valkey refused any command,
   * which is what lets `seedUntilSuccessful` keep trying.
   */
  async seed(): Promise<void> {
    const pipeline = this.redis.pipeline();
    for (const part of CATALOG) {
      pipeline.setnx(stockKey(part.sku), initialStock(part.sku));
      pipeline.setnx(reservedKey(part.sku), 0);
    }
    // A pipeline resolves even when individual commands failed, so check each reply.
    const results = await pipeline.exec();
    const failure = results?.find(([err]) => err)?.[0];
    if (failure) throw failure;
    this.logger.info({ skus: SKUS.length }, 'parts cage seeded');
    await this.publishStock();
  }

  /**
   * Keep trying to seed until it works. A depot that starts before Valkey is reachable must
   * not end up serving an empty parts cage, which is exactly what a one-shot seed would do.
   * Resolves once the cage is stocked.
   */
  async seedUntilSuccessful(retryDelayMs = 2_000, signal?: { stopped: boolean }): Promise<void> {
    for (let attempt = 1; !signal?.stopped; attempt++) {
      try {
        await this.seed();
        return;
      } catch (err) {
        this.logger.warn(
          { attempt, err: err instanceof Error ? err.message : String(err) },
          'could not seed the parts cage yet; retrying',
        );
        await new Promise((resolve) => setTimeout(resolve, retryDelayMs));
      }
    }
  }

  async parts(): Promise<PartStock[]> {
    const keys = CATALOG.flatMap((p) => [stockKey(p.sku), reservedKey(p.sku)]);
    const values = await this.redis.mget(keys);
    return CATALOG.map((part, index) => ({
      sku: part.sku,
      name: part.name,
      perUnit: part.perUnit,
      stock: Number(values[index * 2] ?? 0),
      reserved: Number(values[index * 2 + 1] ?? 0),
    }));
  }

  private async publishStock(): Promise<PartStock[]> {
    const levels = await this.parts();
    this.onStockChange?.(levels);
    return levels;
  }

  async getReservation(id: string): Promise<Reservation | undefined> {
    const raw = await this.redis.get(reservationKey(id));
    return raw ? (JSON.parse(raw) as Reservation) : undefined;
  }

  async reservationForOrder(orderId: string): Promise<Reservation | undefined> {
    const id = await this.redis.get(orderIndexKey(orderId));
    return id ? this.getReservation(id) : undefined;
  }

  /**
   * Reserve the whole bill of materials atomically. Idempotent on `orderId`: a replay
   * returns the reservation that already exists, whatever state it is in.
   */
  async reserve(input: {
    orderId: string;
    model: string;
    quantity: number;
    options?: OrderOptions;
  }): Promise<{ reservation: Reservation; created: boolean }> {
    const options = input.options ?? {};
    const items = billOfMaterials(input.model, input.quantity, options);
    const id = newId('reservation');
    const createdAtMs = this.now();
    const reservation: Reservation = {
      id,
      orderId: input.orderId,
      model: input.model,
      quantity: input.quantity,
      options,
      items,
      status: 'pending',
      createdAt: new Date(createdAtMs).toISOString(),
      expiresAt: new Date(createdAtMs + this.reservationTtlMs).toISOString(),
    };

    const keys = [
      orderIndexKey(input.orderId),
      PENDING_KEY,
      reservationKey(id),
      ...stockKeyPairs(items.map((i) => i.sku)),
    ];
    const args: (string | number)[] = [
      id,
      JSON.stringify(reservation),
      createdAtMs + this.reservationTtlMs,
      RESERVATION_KEY_TTL_SECONDS,
      items.length,
      ...items.flatMap((item) => [item.sku, item.quantity]),
    ];

    const result = await this.redis.depotReserve(keys.length, ...keys, ...args);
    const outcome = result[0];

    if (outcome === 'existing') {
      const existingId = result[1] ?? '';
      const existing = await this.getReservation(existingId);
      if (!existing) {
        // The index outlived the record; treat it as gone and let the caller retry.
        await this.redis.del(orderIndexKey(input.orderId));
        throw conflict('reservation_vanished', 'A stale reservation index was cleared; retry.');
      }
      return { reservation: existing, created: false };
    }

    if (outcome === 'short') {
      const short: ShortSku[] = [];
      for (let i = 1; i + 2 < result.length; i += 3) {
        short.push({
          sku: String(result[i]),
          available: Number(result[i + 1]),
          needed: Number(result[i + 2]),
        });
      }
      throw new InsufficientPartsError(short);
    }

    await this.publishStock();
    this.logger.info(
      { reservationId: id, orderId: input.orderId, model: input.model, quantity: input.quantity },
      'parts reserved',
    );
    return { reservation, created: true };
  }

  /** Mark a reservation consumed. Idempotent: committing twice is a no-op. */
  async commit(id: string): Promise<Reservation> {
    return this.settle(id, 'committed');
  }

  /** Put the parts back. Idempotent: releasing twice is a no-op. */
  async release(id: string, reason: ReservationStatus = 'released'): Promise<Reservation> {
    return this.settle(id, reason === 'expired' ? 'expired' : 'released');
  }

  private async settle(
    id: string,
    target: Extract<ReservationStatus, 'committed' | 'released' | 'expired'>,
  ): Promise<Reservation> {
    const current = await this.getReservation(id);
    if (!current) throw notFound('reservation_not_found', `No reservation ${id}`);
    if (current.status === target) return current;
    if (current.status !== 'pending') {
      throw conflict('reservation_not_pending', `Reservation ${id} is already ${current.status}`, {
        status: current.status,
      });
    }

    const next: Reservation = {
      ...current,
      status: target,
      ...(target === 'committed'
        ? { committedAt: nowRfc3339(new Date(this.now())) }
        : { releasedAt: nowRfc3339(new Date(this.now())) }),
    };

    const keys = [
      reservationKey(id),
      PENDING_KEY,
      orderIndexKey(current.orderId),
      ...stockKeyPairs(current.items.map((i) => i.sku)),
    ];
    const args: (string | number)[] = [
      JSON.stringify(next),
      id,
      current.items.length,
      ...current.items.flatMap((item) => [item.sku, item.quantity]),
    ];

    const result =
      target === 'committed'
        ? await this.redis.depotCommit(keys.length, ...keys, ...args)
        : await this.redis.depotRelease(keys.length, ...keys, ...args);

    if (result[0] === 'missing') {
      throw notFound('reservation_not_found', `No reservation ${id}`);
    }
    if (result[0] === 'conflict') {
      const status = String(result[1]);
      if (status === target) {
        // Lost a race with an identical call; that is a success, not a failure.
        return (await this.getReservation(id)) ?? next;
      }
      throw conflict('reservation_not_pending', `Reservation ${id} is already ${status}`, {
        status,
      });
    }

    await this.publishStock();
    this.logger.info(
      { reservationId: id, orderId: current.orderId, status: target },
      'reservation settled',
    );
    return next;
  }

  async restock(sku: string, quantity: number): Promise<PartStock> {
    const part = findPart(sku);
    if (!part) throw notFound('unknown_sku', `No such part ${sku}`);
    await this.redis.incrby(stockKey(sku), quantity);
    const levels = await this.publishStock();
    const level = levels.find((l) => l.sku === sku);
    if (!level) throw notFound('unknown_sku', `No such part ${sku}`);
    return level;
  }

  /**
   * One turn of the amulite foundry. `rate` comes from the `amulite-foundry-rate` flag;
   * zero stops the foundry.
   */
  async runFoundry(rate: number): Promise<PartStock[]> {
    const increments = CATALOG.map((p) => foundryIncrement(p.sku, rate));
    if (increments.every((n) => n === 0)) return this.parts();
    const keys = CATALOG.map((p) => stockKey(p.sku));
    await this.redis.depotFoundry(keys.length, ...keys, STOCK_CAP, ...increments);
    return this.publishStock();
  }

  /**
   * Return the stock of every reservation nobody committed in time. Each candidate is
   * claimed with an atomic `ZREM` so replicas do not sweep the same reservation twice.
   */
  async sweepExpired(limit = SWEEP_BATCH): Promise<number> {
    const due = await this.redis.zrangebyscore(PENDING_KEY, '-inf', this.now(), 'LIMIT', 0, limit);
    let swept = 0;
    for (const id of due) {
      // Whoever wins the ZREM owns the sweep; the rest move on.
      const claimed = await this.redis.zrem(PENDING_KEY, id);
      if (claimed !== 1) continue;
      const reservation = await this.getReservation(id);
      if (reservation?.status !== 'pending') continue;
      try {
        await this.release(id, 'expired');
        swept++;
      } catch (err) {
        this.logger.warn(
          { reservationId: id, err: err instanceof Error ? err.message : String(err) },
          'could not sweep reservation',
        );
      }
    }
    if (swept > 0) {
      this.logger.info({ swept }, 'swept expired reservations back onto the shelf');
    }
    return swept;
  }
}
