/**
 * The storefront.
 *
 * "so a person in a browser drives the same path as the grammeter" · the design
 * means that literally. These routes go through the dingle-arm gateway with the
 * signed-in user's own bearer token, which means JWT verification, rate limiting,
 * the order saga, parts reservation and the event bus all behave exactly as they
 * do for synthetic traffic. Nothing is short-circuited.
 *
 * They therefore require a real session, not an operator role: a customer
 * ordering an encabulator is the normal case.
 */

import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify'
import { UpstreamDown, UpstreamNotConfigured, type Gateway } from '../gateway.js'
import { errorBody, type ConsoleMetrics, type Logger } from '../platform/index.js'
import { requireSession, sessionOf } from '../roles.js'

export interface StorefrontDeps {
  gateway: Gateway
  logger: Logger
  metrics: ConsoleMetrics
}

/** The gateway's own paths, per design section 8.1. */
export const GATEWAY_PATHS = {
  quotes: '/api/v1/quotes',
  orders: '/api/v1/orders',
  parts: '/api/v1/parts',
} as const

export function registerStorefrontRoutes(app: FastifyInstance, deps: StorefrontDeps): void {
  const gate = requireSession((route, role) => deps.metrics.countForbidden(route, role))

  async function throughGateway(
    request: FastifyRequest,
    reply: FastifyReply,
    method: 'GET' | 'POST',
    path: string,
  ): Promise<void> {
    const session = sessionOf(request)
    if (session.token === null) {
      await reply
        .code(401)
        .send(errorBody('unauthenticated', 'The storefront needs you signed in.'))
      return
    }

    try {
      const res = await deps.gateway.call({
        target: 'gateway',
        method,
        path,
        ...(request.body === undefined || request.body === null ? {} : { body: request.body }),
        headers: {
          // The gateway strips inbound X-Encabulator-* headers and derives
          // identity from this token itself, which is the point.
          authorization: `Bearer ${session.token}`,
        },
        ...(typeof request.id === 'string' ? { requestId: request.id } : {}),
      })
      if (res.contentType !== null) void reply.header('content-type', res.contentType)
      await reply.code(res.status).send(res.body)
    } catch (err) {
      if (err instanceof UpstreamNotConfigured) {
        await reply.code(503).send(
          errorBody(
            'upstream_not_configured',
            `The dingle-arm gateway has no address: set ${err.env} to use the storefront.`,
          ),
        )
        return
      }
      if (err instanceof UpstreamDown) {
        await reply.code(502).send(errorBody(err.code, err.message))
        return
      }
      throw err
    }
  }

  app.post('/storefront/quotes', { preHandler: gate }, async (request, reply) =>
    throughGateway(request, reply, 'POST', GATEWAY_PATHS.quotes),
  )

  app.post('/storefront/orders', { preHandler: gate }, async (request, reply) =>
    throughGateway(request, reply, 'POST', GATEWAY_PATHS.orders),
  )

  // Tracking your own orders through the same front door, so the Works page can
  // show a customer exactly what a customer would see.
  app.get('/storefront/orders', { preHandler: gate }, async (request, reply) => {
    const q = request.url.indexOf('?')
    const suffix = q === -1 ? '?limit=10' : request.url.slice(q)
    return throughGateway(request, reply, 'GET', `${GATEWAY_PATHS.orders}${suffix}`)
  })
}
