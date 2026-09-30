/**
 * The proxy routes.
 *
 * The table below is the whole of the console's upstream surface. It is data,
 * not code, for one reason: the role required by each route is then reviewable
 * at a glance and testable exhaustively. `src/server/__tests__/roles.test.ts`
 * walks this table and asserts that every `operator` route refuses a viewer and
 * every `admin` route refuses an operator.
 *
 * Rules the table encodes (design section 8.15):
 *   - every mutating route requires `operator` or `admin`
 *   - user management and the audit log require `admin`
 *   - everything else is readable by a viewer, which includes guests
 */

import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify'
import type { UpstreamKey } from '../config.js'
import { UpstreamDown, UpstreamNotConfigured, type Gateway } from '../gateway.js'
import { errorBody, type ConsoleMetrics, type Logger } from '../platform/index.js'
import { ADMIN_ROLES, OPERATOR_ROLES, requireRole, sessionOf } from '../roles.js'
import type { Role } from '../session.js'

export type Access = 'viewer' | 'operator' | 'admin'

export interface ProxyRoute {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  /** Path under `/bff`, Fastify syntax. */
  path: string
  access: Access
  upstream: UpstreamKey
  /**
   * Upstream path, with `:param` placeholders filled from the request params.
   * The query string is forwarded verbatim.
   */
  to: string
  /** Human label used in error messages and the README. */
  what: string
}

export const PROXY_ROUTES: ProxyRoute[] = [
  // ── Grammeters: synthetic customer traffic ────────────────────────────────
  { method: 'GET', path: '/traffic', access: 'viewer', upstream: 'grammeter', to: '/v1/traffic', what: 'traffic settings and live journey stats' },
  { method: 'PUT', path: '/traffic', access: 'operator', upstream: 'grammeter', to: '/v1/traffic', what: 'change the traffic rate, pattern or mix' },

  // ── Malaise: fault injection ──────────────────────────────────────────────
  { method: 'GET', path: '/malaises', access: 'viewer', upstream: 'malaise', to: '/v1/malaises', what: 'active and past malaises' },
  { method: 'POST', path: '/malaises', access: 'operator', upstream: 'malaise', to: '/v1/malaises', what: 'induce a malaise' },
  { method: 'DELETE', path: '/malaises', access: 'operator', upstream: 'malaise', to: '/v1/malaises', what: 'clear every malaise' },
  { method: 'DELETE', path: '/malaises/:id', access: 'operator', upstream: 'malaise', to: '/v1/malaises/:id', what: 'clear one malaise' },

  // ── Scenarios ─────────────────────────────────────────────────────────────
  { method: 'GET', path: '/scenarios', access: 'viewer', upstream: 'orchestrator', to: '/v1/scenarios', what: 'the scenario library' },
  { method: 'POST', path: '/scenarios/:id/runs', access: 'operator', upstream: 'orchestrator', to: '/v1/scenarios/:id/runs', what: 'start a scenario' },
  { method: 'GET', path: '/runs', access: 'viewer', upstream: 'orchestrator', to: '/v1/runs', what: 'recent scenario runs' },
  { method: 'GET', path: '/runs/:id', access: 'viewer', upstream: 'orchestrator', to: '/v1/runs/:id', what: 'one run, with its narration log' },
  { method: 'POST', path: '/runs/:id/cancel', access: 'operator', upstream: 'orchestrator', to: '/v1/runs/:id/cancel', what: 'cancel a run' },

  // ── Girdlespring: feature flags ───────────────────────────────────────────
  { method: 'GET', path: '/flags', access: 'viewer', upstream: 'girdlespring', to: '/v1/flags', what: 'every feature flag' },
  { method: 'GET', path: '/flags/:key', access: 'viewer', upstream: 'girdlespring', to: '/v1/flags/:key', what: 'one flag' },
  { method: 'PUT', path: '/flags/:key', access: 'operator', upstream: 'girdlespring', to: '/v1/flags/:key', what: 'set a flag' },

  // ── Parts cage ────────────────────────────────────────────────────────────
  { method: 'GET', path: '/parts', access: 'viewer', upstream: 'depot', to: '/v1/parts', what: 'the parts cage' },
  { method: 'POST', path: '/parts/:sku/restock', access: 'operator', upstream: 'depot', to: '/v1/parts/:sku/restock', what: 'restock a part' },

  // ── Order ledger ──────────────────────────────────────────────────────────
  { method: 'GET', path: '/orders', access: 'viewer', upstream: 'datastore', to: '/v1/orders', what: 'the order ledger' },
  { method: 'GET', path: '/orders/:id', access: 'viewer', upstream: 'datastore', to: '/v1/orders/:id', what: 'one order and its timeline' },
  { method: 'GET', path: '/stats', access: 'viewer', upstream: 'datastore', to: '/v1/stats', what: 'ledger counts and throughput' },

  // ── Inspection ────────────────────────────────────────────────────────────
  { method: 'GET', path: '/incidents', access: 'viewer', upstream: 'detector', to: '/v1/incidents', what: 'side-fumbling incidents' },
  { method: 'GET', path: '/detector/stats', access: 'viewer', upstream: 'detector', to: '/v1/stats', what: 'inspection pass and fail rates' },

  // ── Shipping ──────────────────────────────────────────────────────────────
  { method: 'GET', path: '/shipments', access: 'viewer', upstream: 'dispatch', to: '/v1/shipments', what: 'recent shipments' },
  { method: 'GET', path: '/shipments/:orderId', access: 'viewer', upstream: 'dispatch', to: '/v1/shipments/:orderId', what: 'one order’s shipment' },

  // ── Telemetry ─────────────────────────────────────────────────────────────
  { method: 'GET', path: '/telemetry/summary', access: 'viewer', upstream: 'recorder', to: '/v1/summary', what: 'sinusoidal repleneration aggregates' },

  // ── Administration ────────────────────────────────────────────────────────
  { method: 'GET', path: '/users', access: 'admin', upstream: 'detractor', to: '/v1/users', what: 'the user list' },
  { method: 'POST', path: '/users', access: 'admin', upstream: 'detractor', to: '/v1/users', what: 'create a user' },
  { method: 'PATCH', path: '/users/:username', access: 'admin', upstream: 'detractor', to: '/v1/users/:username', what: 'change a user' },
  { method: 'DELETE', path: '/users/:username', access: 'admin', upstream: 'detractor', to: '/v1/users/:username', what: 'delete a user' },
  { method: 'GET', path: '/audit', access: 'admin', upstream: 'detractor', to: '/v1/audit', what: 'the audit log' },
]

export const MUTATING_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

const ROLES_FOR: Record<Access, Role[] | null> = {
  viewer: null, // everyone, including guests
  operator: OPERATOR_ROLES,
  admin: ADMIN_ROLES,
}

function fillPath(template: string, params: Record<string, unknown>): string {
  return template.replace(/:([A-Za-z_][A-Za-z0-9_]*)/g, (_all, name: string) => {
    const value = params[name]
    return encodeURIComponent(typeof value === 'string' ? value : String(value ?? ''))
  })
}

export function queryStringOf(url: string): string {
  const q = url.indexOf('?')
  return q === -1 ? '' : url.slice(q)
}

export interface ProxyDeps {
  gateway: Gateway
  logger: Logger
  metrics: ConsoleMetrics
}

export function registerProxyRoutes(app: FastifyInstance, deps: ProxyDeps): void {
  for (const route of PROXY_ROUTES) {
    const required = ROLES_FOR[route.access]
    const handler = makeHandler(route, deps)

    app.route({
      method: route.method,
      url: route.path,
      ...(required === null
        ? {}
        : {
            preHandler: requireRole(required, (r, role) => deps.metrics.countForbidden(r, role)),
          }),
      handler,
    })
  }
}

function makeHandler(route: ProxyRoute, deps: ProxyDeps) {
  return async function proxy(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    const session = sessionOf(request)
    const params = (request.params ?? {}) as Record<string, unknown>
    const path = `${fillPath(route.to, params)}${queryStringOf(request.url)}`

    try {
      const res = await deps.gateway.call({
        target: route.upstream,
        method: route.method,
        path,
        ...(request.body === undefined || request.body === null ? {} : { body: request.body }),
        headers: {
          // Design section 7: internal services trust these headers, and the
          // console BFF sets them from its session. Whatever the browser sent is
          // discarded, which is the point.
          'x-encabulator-subject': session.username,
          'x-encabulator-roles': session.roles.join(','),
        },
        ...(typeof request.id === 'string' ? { requestId: request.id } : {}),
      })
      // Pass the upstream's answer through unchanged, including its failures: a
      // 409 from the depot should read as a 409 in the console.
      if (res.contentType !== null) void reply.header('content-type', res.contentType)
      await reply.code(res.status).send(res.body)
    } catch (err) {
      if (err instanceof UpstreamNotConfigured) {
        await reply
          .code(503)
          .send(
            errorBody(
              'upstream_not_configured',
              `${err.service} has no address: set ${err.env} to reach ${route.what}.`,
            ),
          )
        return
      }
      if (err instanceof UpstreamDown) {
        deps.logger.warn(
          { upstream: err.service, code: err.code, detail: err.message },
          'upstream call failed',
        )
        await reply.code(502).send(errorBody(err.code, err.message))
        return
      }
      throw err
    }
  }
}
