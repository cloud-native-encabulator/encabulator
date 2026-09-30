/** HTTP routes for the depot (design 8.7). */

import {
  badRequest,
  conflict,
  forbidden,
  hasAnyRole,
  notFound,
  ROLE_ADMIN,
  ROLE_OPERATOR,
} from '@encabulator/kit';
import type { FastifyInstance } from 'fastify';
import { InvalidReservationError, MAX_QUANTITY, MIN_QUANTITY, MODELS } from './catalog.js';
import { type Depot, InsufficientPartsError, type Reservation } from './depot.js';
import { recordReservation } from './metrics.js';

interface ReservationBody {
  orderId?: unknown;
  model?: unknown;
  quantity?: unknown;
  options?: { dingleArm?: unknown };
}

const reservationSchema = {
  body: {
    type: 'object',
    required: ['orderId', 'model', 'quantity'],
    properties: {
      orderId: { type: 'string', minLength: 1, maxLength: 128 },
      model: { type: 'string', enum: [...MODELS] },
      quantity: { type: 'integer', minimum: MIN_QUANTITY, maximum: MAX_QUANTITY },
      options: {
        type: 'object',
        additionalProperties: false,
        properties: { dingleArm: { type: 'boolean' } },
      },
    },
  },
} as const;

const restockSchema = {
  body: {
    type: 'object',
    required: ['quantity'],
    properties: { quantity: { type: 'integer', minimum: 1, maximum: 100_000 } },
  },
} as const;

export function registerRoutes(app: FastifyInstance, depot: Depot): void {
  app.get('/v1/parts', async () => depot.parts());

  app.post<{ Body: ReservationBody }>(
    '/v1/reservations',
    { schema: reservationSchema },
    async (request, reply) => {
      const body = request.body;
      try {
        const { reservation, created } = await depot.reserve({
          orderId: String(body.orderId),
          model: String(body.model),
          quantity: Number(body.quantity),
          options: { dingleArm: Boolean(body.options?.dingleArm) },
        });
        recordReservation(created ? 'created' : 'duplicate');
        return reply.code(created ? 201 : 200).send(view(reservation));
      } catch (err) {
        if (err instanceof InsufficientPartsError) {
          recordReservation('insufficient_parts');
          throw conflict(
            'insufficient_parts',
            'The parts cage is short; the foundry has been notified.',
            { short: err.short },
          );
        }
        if (err instanceof InvalidReservationError) {
          recordReservation('invalid');
          throw badRequest('invalid_reservation', err.message);
        }
        throw err;
      }
    },
  );

  app.get<{ Params: { id: string } }>('/v1/reservations/:id', async (request) => {
    const reservation = await depot.getReservation(request.params.id);
    if (!reservation) {
      throw notFound('reservation_not_found', `No reservation ${request.params.id}`);
    }
    return view(reservation);
  });

  app.post<{ Params: { id: string } }>('/v1/reservations/:id/commit', async (request) => {
    const reservation = await depot.commit(request.params.id);
    recordReservation('committed');
    return view(reservation);
  });

  app.post<{ Params: { id: string } }>('/v1/reservations/:id/release', async (request) => {
    const reservation = await depot.release(request.params.id);
    recordReservation('released');
    return view(reservation);
  });

  app.post<{ Params: { sku: string }; Body: { quantity: number } }>(
    '/v1/parts/:sku/restock',
    { schema: restockSchema },
    async (request) => {
      if (!hasAnyRole(request.identity, ROLE_OPERATOR, ROLE_ADMIN)) {
        throw forbidden('Restocking the parts cage requires the operator or admin role.');
      }
      return depot.restock(request.params.sku, request.body.quantity);
    },
  );
}

/** The wire shape of a reservation; identical to the stored record today. */
function view(reservation: Reservation): Reservation {
  return reservation;
}
