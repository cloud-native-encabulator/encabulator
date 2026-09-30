/** HTTP routes for the processor (design 8.2). */

import type { FastifyInstance } from 'fastify';
import type { OrderSaga } from './saga.js';

export function registerRoutes(app: FastifyInstance, saga: OrderSaga): void {
  app.post('/v1/orders', async (request, reply) => {
    const order = await saga.placeOrder({
      customer: request.identity.subject,
      body: request.body,
      requestId: String(request.id),
    });
    // 202: the order is on the books, but no encabulator exists yet.
    return reply.code(202).send(order);
  });
}
