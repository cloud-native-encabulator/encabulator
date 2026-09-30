/**
 * Role enforcement. This is the test that keeps the demo honest about who may
 * touch the machine: it walks the entire proxy route table and asserts, for
 * every role, that the server refuses what the design says it must refuse.
 *
 * A new route added to `PROXY_ROUTES` is covered automatically. A route added
 * with the wrong `access` fails here.
 */

import { describe, expect, it } from 'vitest'
import { MUTATING_METHODS, PROXY_ROUTES, type ProxyRoute } from '../routes/proxy.js'
import { harness, type Harness } from './helpers.js'
import type { Role } from '../session.js'

/** A concrete URL for a route template, with plausible parameter values. */
function urlFor(route: ProxyRoute): string {
  return `/bff${route.path
    .replace(':id', 'mal_01J000000000000000000000')
    .replace(':key', 'turbo-encabulation')
    .replace(':sku', 'AMU-BASE')
    .replace(':username', 'someone')
    .replace(':orderId', 'ord_01J000000000000000000000')}`
}

function bodyFor(route: ProxyRoute): unknown {
  return route.method === 'GET' || route.method === 'DELETE' ? undefined : { value: true }
}

async function call(h: Harness, route: ProxyRoute, cookie: string | null) {
  return h.app.inject({
    method: route.method,
    url: urlFor(route),
    ...(cookie === null ? {} : { headers: { cookie } }),
    ...(bodyFor(route) === undefined ? {} : { payload: bodyFor(route) as object }),
  })
}

describe('the proxy route table', () => {
  it('marks every mutating route operator or admin', () => {
    const offenders = PROXY_ROUTES.filter(
      (r) => MUTATING_METHODS.has(r.method) && r.access === 'viewer',
    )
    expect(offenders.map((r) => `${r.method} ${r.path}`)).toEqual([])
  })

  it('puts user management and the audit log behind admin', () => {
    const adminSurface = PROXY_ROUTES.filter(
      (r) => r.path.startsWith('/users') || r.path === '/audit',
    )
    expect(adminSurface.length).toBeGreaterThan(0)
    for (const route of adminSurface) {
      expect(route.access, `${route.method} ${route.path}`).toBe('admin')
    }
  })

  it('covers every upstream the design lists as proxied', () => {
    const upstreams = new Set(PROXY_ROUTES.map((r) => r.upstream))
    for (const expected of [
      'grammeter',
      'malaise',
      'orchestrator',
      'girdlespring',
      'depot',
      'datastore',
      'detector',
      'dispatch',
      'recorder',
      'detractor',
    ]) {
      expect(upstreams, expected).toContain(expected)
    }
  })
})

describe('server-side role checks', () => {
  const OPERATOR_ONLY = PROXY_ROUTES.filter((r) => r.access === 'operator')
  const ADMIN_ONLY = PROXY_ROUTES.filter((r) => r.access === 'admin')
  const VIEWER_OK = PROXY_ROUTES.filter((r) => r.access === 'viewer')

  it('turns a guest away from every operator route with 401', async () => {
    const h = await harness()
    try {
      for (const route of OPERATOR_ONLY) {
        const res = await call(h, route, null)
        expect(res.statusCode, `${route.method} ${route.path}`).toBe(401)
        expect(res.json().error.code).toBe('unauthenticated')
      }
    } finally {
      await h.close()
    }
  })

  it('turns a signed-in viewer away from every operator route with 403', async () => {
    const h = await harness()
    try {
      const cookie = h.cookieFor('watcher', ['viewer'])
      for (const route of OPERATOR_ONLY) {
        const res = await call(h, route, cookie)
        expect(res.statusCode, `${route.method} ${route.path}`).toBe(403)
        expect(res.json().error.code).toBe('forbidden')
      }
    } finally {
      await h.close()
    }
  })

  it('turns a customer away from every operator route', async () => {
    const h = await harness()
    try {
      const cookie = h.cookieFor('buyer', ['customer'])
      for (const route of OPERATOR_ONLY) {
        const res = await call(h, route, cookie)
        expect(res.statusCode, `${route.method} ${route.path}`).toBe(403)
      }
    } finally {
      await h.close()
    }
  })

  it('lets an operator through every operator route', async () => {
    const h = await harness()
    try {
      const cookie = h.cookieFor('shift-lead', ['operator'])
      for (const route of OPERATOR_ONLY) {
        const res = await call(h, route, cookie)
        expect(res.statusCode, `${route.method} ${route.path}`).toBe(200)
      }
    } finally {
      await h.close()
    }
  })

  it('refuses an operator on admin routes and admits an admin', async () => {
    const h = await harness()
    try {
      const operator = h.cookieFor('shift-lead', ['operator'])
      const admin = h.cookieFor('works-manager', ['admin'])
      expect(ADMIN_ONLY.length).toBeGreaterThan(0)
      for (const route of ADMIN_ONLY) {
        const refused = await call(h, route, operator)
        expect(refused.statusCode, `operator on ${route.path}`).toBe(403)
        const allowed = await call(h, route, admin)
        expect(allowed.statusCode, `admin on ${route.path}`).toBe(200)
      }
    } finally {
      await h.close()
    }
  })

  it('lets a guest read every viewer route, because guests are viewers', async () => {
    const h = await harness()
    try {
      for (const route of VIEWER_OK) {
        const res = await call(h, route, null)
        expect(res.statusCode, `${route.method} ${route.path}`).toBe(200)
      }
    } finally {
      await h.close()
    }
  })

  it('forwards the session identity, not the browser’s claim, to upstreams', async () => {
    const h = await harness()
    try {
      const res = await h.app.inject({
        method: 'PUT',
        url: '/bff/traffic',
        headers: {
          cookie: h.cookieFor('shift-lead', ['operator']),
          // A malicious client trying to promote itself in transit.
          'x-encabulator-roles': 'admin',
          'x-encabulator-subject': 'root',
        },
        payload: { rps: 12 },
      })
      expect(res.statusCode).toBe(200)
      const call = h.gateway.calls.at(-1)
      expect(call?.headers['x-encabulator-subject']).toBe('shift-lead')
      expect(call?.headers['x-encabulator-roles']).toBe('operator')
    } finally {
      await h.close()
    }
  })

  it('counts refusals so an operator can see them in Prometheus', async () => {
    const h = await harness()
    try {
      await h.app.inject({ method: 'PUT', url: '/bff/traffic', payload: { rps: 1 } })
      const metric = await h.metrics.read('encabulator_console_forbidden_total')
      expect(metric?.values.length).toBeGreaterThan(0)
    } finally {
      await h.close()
    }
  })

  it('treats an expired token as a guest', async () => {
    const h = await harness()
    try {
      // A token that expired an hour ago, correctly signed into the cookie.
      const expired = Buffer.from(
        JSON.stringify({
          sub: 'shift-lead',
          roles: ['operator'],
          exp: Math.floor(Date.now() / 1000) - 3600,
        }),
      ).toString('base64url')
      const token = `${Buffer.from('{"alg":"RS256"}').toString('base64url')}.${expired}.sig`
      const cookie = `encabulator_session=${h.app.signCookie(token)}`
      const res = await h.app.inject({
        method: 'PUT',
        url: '/bff/traffic',
        headers: { cookie },
        payload: { rps: 1 },
      })
      expect(res.statusCode).toBe(401)
    } finally {
      await h.close()
    }
  })

  it('ignores a session cookie whose signature does not check out', async () => {
    const h = await harness()
    try {
      const forged = `encabulator_session=${h.cookieFor('shift-lead', ['admin']).split('=')[1]}tampered`
      const res = await h.app.inject({
        method: 'DELETE',
        url: '/bff/malaises',
        headers: { cookie: forged },
      })
      expect(res.statusCode).toBe(401)
    } finally {
      await h.close()
    }
  })
})

describe('the storefront', () => {
  it('requires a signed-in user of any role, not an operator', async () => {
    const h = await harness()
    try {
      const guest = await h.app.inject({
        method: 'POST',
        url: '/bff/storefront/orders',
        payload: { model: 'turbo', quantity: 1 },
      })
      expect(guest.statusCode).toBe(401)

      const customer = await h.app.inject({
        method: 'POST',
        url: '/bff/storefront/orders',
        headers: { cookie: h.cookieFor('buyer', ['customer']) },
        payload: { model: 'turbo', quantity: 1, options: { dingleArm: true } },
      })
      expect(customer.statusCode).toBe(200)
    } finally {
      await h.close()
    }
  })

  it('goes through the gateway with the user’s own bearer token', async () => {
    const h = await harness()
    try {
      await h.app.inject({
        method: 'POST',
        url: '/bff/storefront/quotes',
        headers: { cookie: h.cookieFor('buyer', ['customer']) },
        payload: { model: 'hyper', quantity: 2 },
      })
      const call = h.gateway.calls.at(-1)
      expect(call?.target).toBe('gateway')
      expect(call?.path).toBe('/api/v1/quotes')
      expect(call?.headers['authorization']).toMatch(/^Bearer /)
      // Identity headers must not be forged here: the gateway derives identity
      // from the token itself.
      expect(call?.headers['x-encabulator-roles']).toBeUndefined()
    } finally {
      await h.close()
    }
  })
})

describe('unconfigured upstreams', () => {
  it('says which environment variable is missing instead of guessing a host', async () => {
    const { testConfig } = await import('./helpers.js')
    const h = await harness({
      config: testConfig({ CARDINAL_GRAMMETER_URL: undefined }),
    })
    try {
      const res = await h.app.inject({ method: 'GET', url: '/bff/traffic' })
      expect(res.statusCode).toBe(503)
      expect(res.json().error.code).toBe('upstream_not_configured')
      expect(res.json().error.message).toContain('CARDINAL_GRAMMETER_URL')
    } finally {
      await h.close()
    }
  })
})

/** Roles used above, kept explicit so a future role addition is noticed. */
const _ROLES: Role[] = ['customer', 'viewer', 'operator', 'admin']
void _ROLES
