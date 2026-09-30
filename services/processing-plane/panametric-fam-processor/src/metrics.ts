/** Domain metrics for the processor (design 8.2). */

import { counter } from '@encabulator/kit';

export const ordersTotal = counter({
  name: 'encabulator_orders_total',
  help: 'Orders by model and outcome.',
  labelNames: ['model', 'result'],
});

export const refabricationsTotal = counter({
  name: 'encabulator_refabrications_total',
  help: 'Fabrication attempts requested because an inspection failed.',
});

export const ordersScrappedTotal = counter({
  name: 'encabulator_orders_scrapped_total',
  help: 'Orders scrapped after the third failed inspection.',
});

/**
 * `accepted` means the saga completed; `rejected` means the works said no on purpose
 * (bad request, model withdrawn, no parts); `failed` means something broke.
 */
export type OrderResult = 'accepted' | 'rejected' | 'failed';

export function recordOrder(model: string, result: OrderResult): void {
  ordersTotal.inc({ model, result });
}

export function recordRefabrication(): void {
  refabricationsTotal.inc();
}

export function recordScrapped(): void {
  ordersScrappedTotal.inc();
}
