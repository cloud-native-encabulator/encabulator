/** Domain metrics for the dispatch office (design 8.5). */

import { counter, gauge } from '@encabulator/kit';
import type { Shipment } from './dispatch.js';

export const shipmentsTotal = counter({
  name: 'encabulator_shipments_total',
  help: 'Shipments by carrier and status.',
  labelNames: ['carrier', 'status'],
});

export const shipmentsInTransit = gauge({
  name: 'encabulator_shipments_in_transit',
  help: 'Shipments dispatched but not yet delivered.',
});

export function recordDispatched(shipment: Shipment): void {
  shipmentsTotal.inc({ carrier: shipment.carrier, status: 'dispatched' });
}

export function recordDelivered(shipment: Shipment): void {
  shipmentsTotal.inc({ carrier: shipment.carrier, status: 'delivered' });
}

export function recordInTransit(count: number): void {
  shipmentsInTransit.set(count);
}
