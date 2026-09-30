/**
 * The dispatch office: choose a carrier, book a shipment, and deliver it when the simulated
 * transit time is up (design 8.5).
 *
 * Delivery schedules live in Valkey, so a restart mid-transit loses nothing.
 */

import { type EventPublisher, type Logger, newId, nowRfc3339, Subjects } from '@encabulator/kit';
import type { Redis } from 'ioredis';
import {
  type Carrier,
  chooseCarrier,
  trackingNumber as makeTrackingNumber,
  etaSeconds as pickEtaSeconds,
  type Random,
} from './carriers.js';
import { DUE_KEY, deliveredKey, RECENT_KEY, RECENT_LIMIT, shipmentKey } from './keys.js';
import { CLAIM_DELIVERY_SCRIPT, DISPATCH_SCRIPT, REARM_SCRIPT } from './scripts.js';

export const DELIVERY_TICK_MS = 1_000;
export const DELIVERY_BATCH = 50;
/** How long to wait before retrying a delivery whose event failed to publish. */
export const REARM_DELAY_MS = 5_000;

export type ShipmentStatus = 'in_transit' | 'delivered';

/** The immutable part of a shipment, as stored. */
export interface ShipmentRecord {
  id: string;
  orderId: string;
  carrier: Carrier;
  trackingNumber: string;
  etaSeconds: number;
  attempt: number;
  serials: string[];
  dispatchedAt: string;
  dueAt: string;
}

/** The wire shape: the record plus its current status. */
export interface Shipment extends ShipmentRecord {
  status: ShipmentStatus;
  deliveredAt?: string;
}

export interface InspectionPassed {
  orderId: string;
  attempt?: number;
  serials?: string[];
}

type ScriptedRedis = Redis & {
  dispatchBook(numKeys: number, ...args: (string | number)[]): Promise<string[]>;
  dispatchClaim(numKeys: number, ...args: (string | number)[]): Promise<string[]>;
  dispatchRearm(numKeys: number, ...args: (string | number)[]): Promise<string[]>;
};

export interface DispatchOptions {
  redis: Redis;
  publisher: EventPublisher;
  logger: Logger;
  random?: Random;
  now?: () => number;
  onShipmentDispatched?: (shipment: Shipment) => void;
  onShipmentDelivered?: (shipment: Shipment) => void;
  onInTransit?: (count: number) => void;
}

export class DispatchOffice {
  private readonly redis: ScriptedRedis;
  private readonly publisher: EventPublisher;
  private readonly logger: Logger;
  private readonly random: Random;
  private readonly now: () => number;
  private readonly onShipmentDispatched: ((shipment: Shipment) => void) | undefined;
  private readonly onShipmentDelivered: ((shipment: Shipment) => void) | undefined;
  private readonly onInTransit: ((count: number) => void) | undefined;

  constructor(opts: DispatchOptions) {
    this.redis = opts.redis as ScriptedRedis;
    this.publisher = opts.publisher;
    this.logger = opts.logger;
    this.random = opts.random ?? Math.random;
    this.now = opts.now ?? (() => Date.now());
    this.onShipmentDispatched = opts.onShipmentDispatched;
    this.onShipmentDelivered = opts.onShipmentDelivered;
    this.onInTransit = opts.onInTransit;

    // No `numberOfKeys`: the key count is passed at call time, as the first argument.
    this.redis.defineCommand('dispatchBook', { lua: DISPATCH_SCRIPT });
    this.redis.defineCommand('dispatchClaim', { lua: CLAIM_DELIVERY_SCRIPT });
    this.redis.defineCommand('dispatchRearm', { lua: REARM_SCRIPT });
  }

  /**
   * Handle `inspection.passed`: assign a carrier and a tracking number, schedule delivery,
   * and publish `shipment.dispatched`. Idempotent per order.
   */
  async dispatch(event: InspectionPassed): Promise<{ shipment: Shipment; created: boolean }> {
    const orderId = event.orderId;
    const dispatchedAtMs = this.now();
    const eta = pickEtaSeconds(this.random);
    const record: ShipmentRecord = {
      id: newId('shipment'),
      orderId,
      carrier: chooseCarrier(this.random),
      trackingNumber: makeTrackingNumber(this.random),
      etaSeconds: eta,
      attempt: event.attempt ?? 1,
      serials: event.serials ?? [],
      dispatchedAt: nowRfc3339(new Date(dispatchedAtMs)),
      dueAt: nowRfc3339(new Date(dispatchedAtMs + eta * 1_000)),
    };

    const result = await this.redis.dispatchBook(
      3,
      shipmentKey(orderId),
      DUE_KEY,
      RECENT_KEY,
      JSON.stringify(record),
      orderId,
      dispatchedAtMs + eta * 1_000,
      RECENT_LIMIT,
    );

    if (result[0] === 'existing') {
      const existing = JSON.parse(String(result[1])) as ShipmentRecord;
      const shipment = await this.withStatus(existing);
      this.logger.debug({ orderId }, 'shipment already booked; nothing to do');
      return { shipment, created: false };
    }

    const shipment: Shipment = { ...record, status: 'in_transit' };
    await this.publisher.publish(Subjects.ShipmentDispatched, {
      orderId,
      carrier: record.carrier,
      trackingNumber: record.trackingNumber,
      etaSeconds: record.etaSeconds,
    });
    this.onShipmentDispatched?.(shipment);
    await this.reportInTransit();
    this.logger.info(
      { orderId, carrier: record.carrier, trackingNumber: record.trackingNumber, etaSeconds: eta },
      'shipment away by nofer trunnion',
    );
    return { shipment, created: true };
  }

  /**
   * One turn of the delivery loop. Each due shipment is claimed with an atomic `ZREM`, so
   * two replicas can run this at the same time without double-delivering.
   */
  async deliverDue(limit = DELIVERY_BATCH): Promise<number> {
    const due = await this.redis.zrangebyscore(DUE_KEY, '-inf', this.now(), 'LIMIT', 0, limit);
    let delivered = 0;
    for (const orderId of due) {
      const deliveredAt = nowRfc3339(new Date(this.now()));
      const claim = await this.redis.dispatchClaim(
        3,
        DUE_KEY,
        shipmentKey(orderId),
        deliveredKey(orderId),
        orderId,
        deliveredAt,
      );
      if (claim[0] !== 'ok') {
        if (claim[0] === 'missing') {
          this.logger.warn({ orderId }, 'due shipment had no record; dropped from the queue');
        }
        continue;
      }
      const record = JSON.parse(String(claim[1])) as ShipmentRecord;
      try {
        await this.publisher.publish(Subjects.ShipmentDelivered, {
          orderId,
          carrier: record.carrier,
          trackingNumber: record.trackingNumber,
        });
      } catch (err) {
        // Put it back rather than losing the delivery event.
        await this.redis.dispatchRearm(
          2,
          DUE_KEY,
          deliveredKey(orderId),
          orderId,
          this.now() + REARM_DELAY_MS,
        );
        this.logger.warn(
          { orderId, err: err instanceof Error ? err.message : String(err) },
          'could not publish the delivery; re-armed',
        );
        continue;
      }
      delivered++;
      this.onShipmentDelivered?.({ ...record, status: 'delivered', deliveredAt });
      this.logger.info(
        { orderId, carrier: record.carrier, trackingNumber: record.trackingNumber },
        'shipment delivered',
      );
    }
    await this.reportInTransit();
    return delivered;
  }

  async get(orderId: string): Promise<Shipment | undefined> {
    const raw = await this.redis.get(shipmentKey(orderId));
    if (!raw) return undefined;
    return this.withStatus(JSON.parse(raw) as ShipmentRecord);
  }

  /** Recent shipments, newest first. */
  async recent(limit = 50): Promise<Shipment[]> {
    const orderIds = await this.redis.lrange(RECENT_KEY, 0, Math.max(0, limit - 1));
    if (orderIds.length === 0) return [];
    const records = await this.redis.mget(orderIds.map(shipmentKey));
    const deliveries = await this.redis.mget(orderIds.map(deliveredKey));
    const shipments: Shipment[] = [];
    for (const [index, raw] of records.entries()) {
      if (!raw) continue;
      const record = JSON.parse(raw) as ShipmentRecord;
      const deliveredAt = deliveries[index];
      shipments.push({
        ...record,
        status: deliveredAt ? 'delivered' : 'in_transit',
        ...(deliveredAt ? { deliveredAt } : {}),
      });
    }
    return shipments;
  }

  async inTransitCount(): Promise<number> {
    return this.redis.zcard(DUE_KEY);
  }

  private async withStatus(record: ShipmentRecord): Promise<Shipment> {
    const deliveredAt = await this.redis.get(deliveredKey(record.orderId));
    return {
      ...record,
      status: deliveredAt ? 'delivered' : 'in_transit',
      ...(deliveredAt ? { deliveredAt } : {}),
    };
  }

  private async reportInTransit(): Promise<void> {
    if (!this.onInTransit) return;
    this.onInTransit(await this.inTransitCount());
  }
}
