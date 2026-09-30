/** Domain metrics for the depot (design 8.7). */

import { counter, gauge } from '@encabulator/kit';
import type { PartStock } from './depot.js';

export const partsStock = gauge({
  name: 'encabulator_parts_stock',
  help: 'Units of each SKU on the shelf.',
  labelNames: ['sku'],
});

export const partsReserved = gauge({
  name: 'encabulator_parts_reserved',
  help: 'Units of each SKU reserved against an order but not yet consumed.',
  labelNames: ['sku'],
});

export const reservationsTotal = counter({
  name: 'encabulator_reservations_total',
  help: 'Reservation attempts by outcome.',
  labelNames: ['result'],
});

export type ReservationResult =
  | 'created'
  | 'duplicate'
  | 'insufficient_parts'
  | 'invalid'
  | 'committed'
  | 'released'
  | 'expired';

export function recordReservation(result: ReservationResult): void {
  reservationsTotal.inc({ result });
}

export function recordStock(levels: PartStock[]): void {
  for (const level of levels) {
    partsStock.set({ sku: level.sku }, level.stock);
    partsReserved.set({ sku: level.sku }, level.reserved);
  }
}
