/** HTTP routes for the dispatch office (design 8.5). */

import { notFound } from '@encabulator/kit';
import type { FastifyInstance } from 'fastify';
import type { DispatchOffice } from './dispatch.js';

const listSchema = {
  querystring: {
    type: 'object',
    properties: { limit: { type: 'integer', minimum: 1, maximum: 500, default: 50 } },
  },
} as const;

export function registerRoutes(app: FastifyInstance, dispatch: DispatchOffice): void {
  app.get<{ Querystring: { limit?: number } }>(
    '/v1/shipments',
    { schema: listSchema },
    async (request) => ({ shipments: await dispatch.recent(request.query.limit ?? 50) }),
  );

  app.get<{ Params: { orderId: string } }>('/v1/shipments/:orderId', async (request) => {
    const shipment = await dispatch.get(request.params.orderId);
    if (!shipment) {
      throw notFound('shipment_not_found', `No shipment for order ${request.params.orderId}`);
    }
    return shipment;
  });
}
