/**
 * The order saga (design 4.1) and the `inspection.failed` handler (design 4.2 step 4).
 *
 * Quote, reserve, write the ledger, publish. If the ledger write fails, the reservation is
 * released before the caller is told the works could not take the order; that compensation
 * is the whole reason this service exists.
 */

import {
  badRequest,
  conflict,
  type EventPublisher,
  type FlagClient,
  type HttpClient,
  HttpClientError,
  type Identity,
  type Logger,
  newId,
  Subjects,
  unavailable,
} from '@encabulator/kit';
import type { CircuitBreaker } from './circuit-breaker.js';
import {
  type Finding,
  type InspectionFailed,
  InvalidOrderError,
  type LedgerOrder,
  nextAttempt,
  type OrderRequest,
  parseOrderRequest,
  type Quote,
  type Reservation,
  scrapReason,
} from './domain.js';
import { recordOrder, recordRefabrication, recordScrapped } from './metrics.js';

/** The processor speaks to the ledger as an internal operator so it can read any order. */
export const INTERNAL_IDENTITY: Identity = {
  subject: 'panametric-fam-processor',
  roles: ['operator'],
};

export interface Dependency {
  client: HttpClient;
  breaker: CircuitBreaker;
}

export interface SagaOptions {
  quoter: Dependency;
  depot: Dependency;
  datastore: Dependency;
  publisher: EventPublisher;
  flags: FlagClient;
  logger: Logger;
}

export interface PlaceOrderInput {
  customer: string;
  body: unknown;
  requestId?: string | undefined;
}

export class OrderSaga {
  constructor(private readonly opts: SagaOptions) {}

  /** Design 4.1, end to end. Returns the ledger's representation of the order. */
  async placeOrder(input: PlaceOrderInput): Promise<LedgerOrder> {
    const { logger } = this.opts;
    let request: OrderRequest;
    try {
      request = parseOrderRequest(input.body);
    } catch (err) {
      if (err instanceof InvalidOrderError) {
        recordOrder('unknown', 'rejected');
        throw badRequest('invalid_order', err.message);
      }
      throw err;
    }

    if (!input.customer) {
      recordOrder(request.model, 'rejected');
      throw badRequest('invalid_order', 'No X-Encabulator-Subject; who is ordering?');
    }

    // Step 3: flags, from the cached set. A girdlespring outage must not stop the line.
    if (request.model === 'retro' && !this.opts.flags.boolean('retro-encabulator-sales')) {
      recordOrder(request.model, 'rejected');
      throw conflict(
        'model_unavailable',
        'The Retro Encabulator is not currently available to order.',
      );
    }

    const orderId = newId('order');
    const log = logger.child({ orderId, customer: input.customer, model: request.model });

    const quote = await this.quote(request, orderId, input.requestId, log);
    const reservation = await this.reserve(request, orderId, input.requestId, log);

    let order: LedgerOrder;
    try {
      order = await this.writeLedger(
        { orderId, customer: input.customer, request, quote, reservation },
        input.requestId,
      );
    } catch (err) {
      // Compensation: put the parts back before telling the customer no.
      await this.releaseReservation(reservation.id, input.requestId, log);
      recordOrder(request.model, 'failed');
      log.error(
        { err: err instanceof Error ? err.message : String(err), reservationId: reservation.id },
        'ledger write failed; reservation released',
      );
      throw unavailable(
        'dependency_unavailable',
        'The order ledger is unavailable; nothing was charged and no parts were consumed.',
      );
    }

    // Step 6: announce the order and ask for the first fabrication attempt.
    await this.opts.publisher.publish(Subjects.OrderAccepted, {
      orderId,
      customer: input.customer,
      model: request.model,
      quantity: request.quantity,
      options: request.options,
      total: quote.total,
      currency: quote.currency,
    });
    await this.opts.publisher.publish(Subjects.FabricationRequested, {
      orderId,
      attempt: 1,
      model: request.model,
      quantity: request.quantity,
      options: request.options,
      reservationId: reservation.id,
    });

    recordOrder(request.model, 'accepted');
    log.info({ total: quote.total, reservationId: reservation.id }, 'order accepted');
    return order;
  }

  private async quote(
    request: OrderRequest,
    orderId: string,
    requestId: string | undefined,
    log: Logger,
  ): Promise<Quote> {
    const { quoter } = this.opts;
    try {
      const response = await quoter.breaker.run(() =>
        quoter.client.post<Quote>(
          '/v1/quotes',
          { model: request.model, quantity: request.quantity, options: request.options },
          { idempotencyKey: orderId, requestId },
        ),
      );
      return response.body;
    } catch (err) {
      if (err instanceof HttpClientError && err.status >= 400 && err.status < 500) {
        recordOrder(request.model, 'rejected');
        log.warn({ code: err.code }, 'quoter refused the order');
        throw badRequest('invalid_order', `The quoter refused this order: ${err.message}`);
      }
      recordOrder(request.model, 'failed');
      throw this.dependencyError(err, 'spurving-bearing-quoter');
    }
  }

  private async reserve(
    request: OrderRequest,
    orderId: string,
    requestId: string | undefined,
    log: Logger,
  ): Promise<Reservation> {
    const { depot } = this.opts;
    try {
      const response = await depot.breaker.run(() =>
        depot.client.post<Reservation>(
          '/v1/reservations',
          {
            orderId,
            model: request.model,
            quantity: request.quantity,
            options: request.options,
          },
          { idempotencyKey: orderId, requestId },
        ),
      );
      return response.body;
    } catch (err) {
      if (err instanceof HttpClientError && err.status === 409) {
        recordOrder(request.model, 'rejected');
        log.info({ code: err.code }, 'the parts cage is short');
        const details = extractShort(err.body);
        throw conflict(
          'insufficient_parts',
          'The parts cage is short; the foundry is working on it.',
          details,
        );
      }
      if (err instanceof HttpClientError && err.status >= 400 && err.status < 500) {
        recordOrder(request.model, 'rejected');
        throw badRequest('invalid_order', `The depot refused this order: ${err.message}`);
      }
      recordOrder(request.model, 'failed');
      throw this.dependencyError(err, 'prefabulated-amulite-depot');
    }
  }

  private async writeLedger(
    input: {
      orderId: string;
      customer: string;
      request: OrderRequest;
      quote: Quote;
      reservation: Reservation;
    },
    requestId: string | undefined,
  ): Promise<LedgerOrder> {
    const { datastore } = this.opts;
    const response = await datastore.breaker.run(() =>
      datastore.client.post<LedgerOrder>(
        '/v1/orders',
        {
          id: input.orderId,
          customer: input.customer,
          model: input.request.model,
          quantity: input.request.quantity,
          options: input.request.options,
          quote: {
            id: input.quote.id,
            total: input.quote.total,
            currency: input.quote.currency,
          },
          reservationId: input.reservation.id,
        },
        // The datastore is idempotent on `id`, so this POST is safe to retry.
        { idempotencyKey: input.orderId, requestId },
      ),
    );
    return response.body;
  }

  private async releaseReservation(
    reservationId: string,
    requestId: string | undefined,
    log: Logger,
  ): Promise<void> {
    const { depot } = this.opts;
    try {
      await depot.client.post(`/v1/reservations/${reservationId}/release`, undefined, {
        idempotencyKey: `${reservationId}:release`,
        requestId,
      });
      log.info({ reservationId }, 'reservation released');
    } catch (err) {
      // Nothing more we can do here: the depot's sweeper returns the parts after fifteen
      // minutes regardless.
      log.error(
        { reservationId, err: err instanceof Error ? err.message : String(err) },
        'could not release the reservation; the depot sweeper will reclaim it',
      );
    }
  }

  private dependencyError(err: unknown, dependency: string): Error {
    if (err instanceof Error && 'statusCode' in err) return err;
    return unavailable('dependency_unavailable', `${dependency} is unavailable.`, { dependency });
  }

  /**
   * Design 4.2 step 4. A failed inspection buys another attempt, up to three; the third
   * failure scraps the order.
   */
  async handleInspectionFailed(event: InspectionFailed): Promise<void> {
    const { logger, publisher } = this.opts;
    const orderId = event.orderId;
    if (!orderId) {
      logger.warn('inspection.failed with no orderId; dropping');
      return;
    }
    const attempt = Number.isInteger(event.attempt) ? event.attempt : 1;
    const log = logger.child({ orderId, attempt });
    const next = nextAttempt(attempt);

    if (next === undefined) {
      const reason = scrapReason(attempt, event.findings ?? ([] as Finding[]));
      await publisher.publish(Subjects.OrderScrapped, { orderId, attempt, reason });
      recordScrapped();
      log.warn({ reason }, 'order scrapped after three failed inspections');
      return;
    }

    // Rebuild the fabrication request from the ledger; inspection.failed carries only the
    // serials and the findings.
    const order = await this.readOrder(orderId, log);
    if (!order) {
      logger.warn({ orderId }, 'no ledger entry for a failed inspection; dropping');
      return;
    }

    await publisher.publish(Subjects.FabricationRequested, {
      orderId,
      attempt: next,
      model: order.model,
      quantity: order.quantity,
      options: order.options,
      reservationId: order.reservationId,
    });
    recordRefabrication();
    log.info({ nextAttempt: next }, 'refabrication requested');
  }

  private async readOrder(orderId: string, log: Logger): Promise<LedgerOrder | undefined> {
    const { datastore } = this.opts;
    const response = await datastore.breaker.run(() =>
      datastore.client
        .withHeaders({
          'x-encabulator-subject': INTERNAL_IDENTITY.subject,
          'x-encabulator-roles': INTERNAL_IDENTITY.roles.join(','),
        })
        .get<LedgerOrder>(`/v1/orders/${orderId}`, { expect: [404] }),
    );
    if (response.status === 404) {
      log.warn('order not found in the ledger');
      return undefined;
    }
    return response.body;
  }
}

/** Forward the depot's short-SKU list to the customer; it is genuinely useful. */
function extractShort(body: unknown): Record<string, unknown> | undefined {
  if (body && typeof body === 'object' && 'error' in body) {
    const err = (body as { error?: { short?: unknown } }).error;
    if (err && typeof err === 'object' && Array.isArray(err.short)) {
      return { short: err.short };
    }
  }
  return undefined;
}
