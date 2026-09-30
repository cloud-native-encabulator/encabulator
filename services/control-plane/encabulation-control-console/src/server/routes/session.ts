/**
 * Session routes: `POST /bff/login`, `POST /bff/logout`, `GET /bff/session`.
 *
 * The browser never sees the JWT. It goes straight into a signed, `httpOnly`,
 * `SameSite=Strict` cookie, which means the SPA cannot leak it and a cross-site
 * request cannot use it.
 */

import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify'
import type { ConsoleConfig } from '../config.js'
import { UpstreamDown, UpstreamNotConfigured, type Gateway } from '../gateway.js'
import { errorBody, type Logger } from '../platform/index.js'
import { sessionOf } from '../roles.js'
import { GUEST, publicSession, secondsUntilExpiry, sessionFromToken } from '../session.js'

interface LoginBody {
  username?: unknown
  password?: unknown
}

export interface SessionDeps {
  config: ConsoleConfig
  gateway: Gateway
  logger: Logger
  now?: () => Date
}

export function cookieOptions(config: ConsoleConfig, maxAgeSeconds: number) {
  return {
    path: '/',
    httpOnly: true,
    sameSite: 'strict' as const,
    secure: config.cookieSecure,
    signed: true,
    maxAge: maxAgeSeconds,
  }
}

export function registerSessionRoutes(app: FastifyInstance, deps: SessionDeps): void {
  const { config } = deps
  const now = (): Date => deps.now?.() ?? new Date()

  app.get('/session', async (request: FastifyRequest, reply: FastifyReply) => {
    await reply.send({
      session: publicSession(sessionOf(request)),
      links: {
        grafana: config.grafanaExternalUrl,
        jaeger: config.jaegerExternalUrl,
      },
      // The SPA shows which upstreams are configured so an operator can see at
      // a glance why a panel is empty.
      upstreams: Object.fromEntries(
        Object.keys(config.upstreams).map((key) => {
          const info = deps.gateway.info(key as keyof ConsoleConfig['upstreams'])
          return [key, { service: info.service, configured: info.configured, env: info.env }]
        }),
      ),
      prometheusConfigured: deps.gateway.info('prometheus').configured,
    })
  })

  app.post('/login', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = (request.body ?? {}) as LoginBody
    const username = typeof body.username === 'string' ? body.username.trim() : ''
    const password = typeof body.password === 'string' ? body.password : ''
    if (username === '' || password === '') {
      await reply
        .code(400)
        .send(errorBody('invalid_credentials', 'A username and a password are both required.'))
      return
    }

    try {
      const res = await deps.gateway.call({
        target: 'detractor',
        method: 'POST',
        path: '/v1/auth/login',
        body: { username, password },
        ...(typeof request.id === 'string' ? { requestId: request.id } : {}),
      })

      if (res.status === 401 || res.status === 403) {
        deps.logger.info({ username }, 'login refused by the detractor')
        await reply
          .code(401)
          .send(errorBody('invalid_credentials', 'That username and password do not match.'))
        return
      }
      if (res.status >= 400) {
        await reply.code(res.status).send(res.body)
        return
      }

      const shaped = res.body as
        | { accessToken?: unknown; user?: { displayName?: unknown } }
        | null
      const token = typeof shaped?.accessToken === 'string' ? shaped.accessToken : null
      if (token === null) {
        await reply
          .code(502)
          .send(errorBody('upstream_error', 'The detractor did not return an access token.'))
        return
      }

      const displayName =
        typeof shaped?.user?.displayName === 'string' ? shaped.user.displayName : undefined
      const session = sessionFromToken(token, now(), displayName)
      if (session === null) {
        await reply
          .code(502)
          .send(
            errorBody('upstream_error', 'The detractor returned a token this console cannot read.'),
          )
        return
      }

      const maxAge = secondsUntilExpiry(session, now())
      void reply.setCookie(config.cookieName, token, cookieOptions(config, maxAge))
      deps.logger.info({ username: session.username, roles: session.roles }, 'session opened')
      await reply.send({ session: publicSession(session) })
    } catch (err) {
      if (err instanceof UpstreamNotConfigured) {
        await reply
          .code(503)
          .send(errorBody('upstream_not_configured', `${err.service} has no address: set ${err.env}.`))
        return
      }
      if (err instanceof UpstreamDown) {
        await reply.code(502).send(errorBody(err.code, err.message))
        return
      }
      throw err
    }
  })

  app.post('/logout', async (request: FastifyRequest, reply: FastifyReply) => {
    void reply.clearCookie(config.cookieName, { path: '/' })
    const session = sessionOf(request)
    if (session.authenticated) {
      deps.logger.info({ username: session.username }, 'session closed')
    }
    await reply.send({ session: publicSession(GUEST) })
  })
}
