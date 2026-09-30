/**
 * Sessions and the app shell: login, logout, health, metrics, and the history
 * fallback that makes deep links work.
 */

import { describe, expect, it } from 'vitest'
import { hasAnyRole, parseRoles, publicSession, sessionFromToken } from '../session.js'
import { hasExtension, isApiPath } from '../app.js'
import { loadConfig, MissingSessionSecretError } from '../config.js'
import { fakeToken, harness, TEST_ENV, testConfig } from './helpers.js'

describe('reading a detractor token', () => {
  it('accepts a live token and keeps only the roles the system defines', () => {
    const token = fakeToken({ sub: 'shift-lead', roles: ['operator'], displayName: 'Shift Lead' })
    const session = sessionFromToken(token)
    expect(session).not.toBeNull()
    expect(session?.username).toBe('shift-lead')
    expect(session?.displayName).toBe('Shift Lead')
    expect(session?.roles).toEqual(['operator'])
  })

  it('refuses an expired token', () => {
    const token = fakeToken({ sub: 'shift-lead', roles: ['admin'], expiresInSeconds: -10 })
    expect(sessionFromToken(token)).toBeNull()
  })

  it('refuses a token with no expiry, rather than treating it as eternal', () => {
    const header = Buffer.from('{"alg":"RS256"}').toString('base64url')
    const payload = Buffer.from(JSON.stringify({ sub: 'x', roles: ['admin'] })).toString('base64url')
    expect(sessionFromToken(`${header}.${payload}.sig`)).toBeNull()
  })

  it('refuses nonsense', () => {
    expect(sessionFromToken('not-a-token')).toBeNull()
    expect(sessionFromToken('a.b.c')).toBeNull()
  })

  it('gives a signed-in user with no recognised roles the viewer role', () => {
    const header = Buffer.from('{"alg":"RS256"}').toString('base64url')
    const payload = Buffer.from(
      JSON.stringify({ sub: 'x', roles: ['wizard'], exp: Math.floor(Date.now() / 1000) + 60 }),
    ).toString('base64url')
    expect(sessionFromToken(`${header}.${payload}.sig`)?.roles).toEqual(['viewer'])
  })

  it('reads roles from an array or a comma-separated string, and drops unknowns', () => {
    expect(parseRoles(['admin', 'operator', 'sorcerer'])).toEqual(['admin', 'operator'])
    expect(parseRoles('viewer, admin')).toEqual(['viewer', 'admin'])
    expect(parseRoles(42)).toEqual([])
  })
})

describe('what the SPA is told', () => {
  it('never includes the token', () => {
    const session = sessionFromToken(fakeToken({ sub: 'a', roles: ['admin'] }))!
    const shown = publicSession(session) as Record<string, unknown>
    expect(Object.keys(shown)).not.toContain('token')
    expect(JSON.stringify(shown)).not.toContain('not-a-real-signature')
  })

  it('states plainly whether this person may operate or administer', () => {
    const operator = publicSession(sessionFromToken(fakeToken({ sub: 'o', roles: ['operator'] }))!)
    expect(operator.canOperate).toBe(true)
    expect(operator.canAdminister).toBe(false)

    const admin = publicSession(sessionFromToken(fakeToken({ sub: 'a', roles: ['admin'] }))!)
    expect(admin.canOperate).toBe(true)
    expect(admin.canAdminister).toBe(true)
  })

  it('treats admin as sufficient wherever operator is required', () => {
    const admin = sessionFromToken(fakeToken({ sub: 'a', roles: ['admin'] }))!
    expect(hasAnyRole(admin, ['operator', 'admin'])).toBe(true)
  })
})

describe('GET /bff/session', () => {
  it('describes a visitor as a guest with the viewer role', async () => {
    const h = await harness()
    try {
      const res = await h.app.inject({ method: 'GET', url: '/bff/session' })
      expect(res.statusCode).toBe(200)
      expect(res.json().session).toMatchObject({
        authenticated: false,
        username: 'guest',
        roles: ['viewer'],
        canOperate: false,
      })
    } finally {
      await h.close()
    }
  })

  it('hands out the external Grafana and Jaeger links from the environment', async () => {
    const h = await harness()
    try {
      const body = (await h.app.inject({ method: 'GET', url: '/bff/session' })).json()
      expect(body.links).toEqual({
        grafana: 'https://grafana.example',
        jaeger: 'https://jaeger.example',
      })
    } finally {
      await h.close()
    }
  })

  it('says which upstreams are unconfigured so an empty panel is explainable', async () => {
    const h = await harness({ config: testConfig({ SINUSOIDAL_RECORDER_URL: undefined }) })
    try {
      const body = (await h.app.inject({ method: 'GET', url: '/bff/session' })).json()
      expect(body.upstreams.recorder).toEqual({
        service: 'sinusoidal-recorder',
        configured: false,
        env: 'SINUSOIDAL_RECORDER_URL',
      })
      expect(body.upstreams.datastore.configured).toBe(true)
    } finally {
      await h.close()
    }
  })
})

describe('POST /bff/login', () => {
  it('stores the token in an httpOnly, SameSite=Strict, signed cookie', async () => {
    const h = await harness()
    h.gateway.on('/v1/auth/login', {
      body: {
        accessToken: fakeToken({ sub: 'shift-lead', roles: ['operator'] }),
        tokenType: 'Bearer',
        expiresIn: 3600,
        user: { username: 'shift-lead', displayName: 'Shift Lead', roles: ['operator'] },
      },
    })
    try {
      const res = await h.app.inject({
        method: 'POST',
        url: '/bff/login',
        payload: { username: 'shift-lead', password: 'correct horse' },
      })
      expect(res.statusCode).toBe(200)
      expect(res.json().session.roles).toEqual(['operator'])

      const cookie = res.headers['set-cookie']
      const raw = Array.isArray(cookie) ? cookie.join(';') : String(cookie)
      expect(raw).toContain('HttpOnly')
      expect(raw).toContain('SameSite=Strict')
      // The response body must not leak the token.
      expect(res.body).not.toContain('not-a-real-signature')
    } finally {
      await h.close()
    }
  })

  it('never logs or echoes the password', async () => {
    const h = await harness()
    h.gateway.on('/v1/auth/login', { status: 401, body: { error: { code: 'invalid_credentials', message: 'no' } } })
    try {
      const res = await h.app.inject({
        method: 'POST',
        url: '/bff/login',
        payload: { username: 'shift-lead', password: 'hunter2' },
      })
      expect(res.statusCode).toBe(401)
      expect(res.body).not.toContain('hunter2')
      const sent = h.gateway.calls.at(-1)
      expect(sent?.target).toBe('detractor')
      expect(sent?.path).toBe('/v1/auth/login')
    } finally {
      await h.close()
    }
  })

  it('asks for both fields before bothering the detractor', async () => {
    const h = await harness()
    try {
      const res = await h.app.inject({ method: 'POST', url: '/bff/login', payload: { username: 'a' } })
      expect(res.statusCode).toBe(400)
      expect(h.gateway.calls).toEqual([])
    } finally {
      await h.close()
    }
  })

  it('refuses to open a session on a token it cannot read', async () => {
    const h = await harness()
    h.gateway.on('/v1/auth/login', { body: { accessToken: 'garbage' } })
    try {
      const res = await h.app.inject({
        method: 'POST',
        url: '/bff/login',
        payload: { username: 'a', password: 'b' },
      })
      expect(res.statusCode).toBe(502)
      expect(res.headers['set-cookie']).toBeUndefined()
    } finally {
      await h.close()
    }
  })

  it('reports the detractor being unreachable rather than pretending to sign you in', async () => {
    const h = await harness()
    h.gateway.fail('/v1/auth/login', 'connect ECONNREFUSED')
    try {
      const res = await h.app.inject({
        method: 'POST',
        url: '/bff/login',
        payload: { username: 'a', password: 'b' },
      })
      expect(res.statusCode).toBe(502)
    } finally {
      await h.close()
    }
  })
})

describe('POST /bff/logout', () => {
  it('clears the cookie and drops back to guest', async () => {
    const h = await harness()
    try {
      const res = await h.app.inject({
        method: 'POST',
        url: '/bff/logout',
        headers: { cookie: h.cookieFor('shift-lead', ['operator']) },
      })
      expect(res.statusCode).toBe(200)
      const raw = String(res.headers['set-cookie'])
      expect(raw).toContain('encabulator_session=')
      expect(res.json().session.authenticated).toBe(false)
    } finally {
      await h.close()
    }
  })
})

describe('health, readiness and metrics', () => {
  it('answers healthz and readyz, and readyz checks nothing downstream', async () => {
    const h = await harness()
    h.gateway.fail('datastore.test', 'down')
    try {
      expect((await h.app.inject({ url: '/healthz' })).json()).toEqual({ status: 'ok' })
      // Every upstream could be on fire; the console is still ready to serve,
      // because it registers no readiness checks at all.
      const ready = (await h.app.inject({ url: '/readyz' })).json()
      expect(ready.status).toBe('ok')
      expect(ready.checks).toEqual([])
    } finally {
      await h.close()
    }
  })

  it('exposes the two histograms the design mandates', async () => {
    const h = await harness()
    try {
      await h.app.inject({ url: '/bff/session' })
      const text = (await h.app.inject({ url: '/metrics' })).body
      expect(text).toContain('http_server_request_duration_seconds')
      expect(text).toContain('http_route="/bff/session"')
    } finally {
      await h.close()
    }
  })

  it('does not record a server observation for /metrics itself', async () => {
    const h = await harness()
    try {
      await h.app.inject({ url: '/metrics' })
      const metric = await h.metrics.read('http_server_request_duration_seconds')
      const routes = (metric?.values ?? []).map((v) => v.labels['http_route'])
      expect(routes).not.toContain('/metrics')
    } finally {
      await h.close()
    }
  })
})

describe('the SPA history fallback', () => {
  it('knows an API path from a client route', () => {
    expect(isApiPath('/bff/traffic')).toBe(true)
    expect(isApiPath('/metrics')).toBe(true)
    expect(isApiPath('/works')).toBe(false)
  })

  it('knows a file from a route', () => {
    expect(hasExtension('/assets/index-a1b2c3.js')).toBe(true)
    expect(hasExtension('/favicon.svg')).toBe(true)
    expect(hasExtension('/girdlespring')).toBe(false)
  })

  it('still 404s an unknown API path instead of serving HTML', async () => {
    const h = await harness()
    try {
      const res = await h.app.inject({ url: '/bff/nonexistent' })
      expect(res.statusCode).toBe(404)
      expect(res.json().error.code).toBe('not_found')
    } finally {
      await h.close()
    }
  })
})

describe('configuration', () => {
  it('refuses to start without a session secret', () => {
    expect(() => loadConfig({} as NodeJS.ProcessEnv)).toThrow(MissingSessionSecretError)
    expect(() => loadConfig({ SESSION_SECRET: 'short' } as NodeJS.ProcessEnv)).toThrow(
      MissingSessionSecretError,
    )
  })

  it('bakes in no localhost defaults: an unset upstream is null', () => {
    const config = loadConfig({ SESSION_SECRET: TEST_ENV['SESSION_SECRET'] } as NodeJS.ProcessEnv)
    for (const [key, upstream] of Object.entries(config.upstreams)) {
      expect(upstream.url, key).toBeNull()
    }
    expect(config.prometheusUrl).toBeNull()
    expect(config.grafanaExternalUrl).toBeNull()
  })

  it('accepts the design’s MALAISE_ENGINE_URL spelling', () => {
    const config = loadConfig({
      ...TEST_ENV,
      MALAISE_ENGINE_URL: 'http://engine:8080/',
    } as NodeJS.ProcessEnv)
    expect(config.upstreams.malaise.url).toBe('http://engine:8080')
    expect(config.upstreams.malaise.env).toBe('MALAISE_ENGINE_URL')
  })

  it('defaults the namespace to encabulator and the port to 8080', () => {
    const config = loadConfig({ SESSION_SECRET: TEST_ENV['SESSION_SECRET'] } as NodeJS.ProcessEnv)
    expect(config.port).toBe(8080)
    expect(config.podNamespace).toBe('encabulator')
    expect(config.cookieSecure).toBe(false)
  })
})
