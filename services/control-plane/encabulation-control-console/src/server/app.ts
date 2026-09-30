/**
 * The Fastify application.
 *
 * The shell comes from `@encabulator/kit`'s `createApp`: one port for the API,
 * `/healthz`, `/readyz` and `/metrics`, request ids, the house error shape, and
 * the `http_server_request_duration_seconds` histogram with `http_route` taken
 * from the route template. The console adds three things of its own: the session
 * cookie, the `/bff` surface, and the built SPA with a history fallback.
 *
 * Two deliberate omissions, both from the design:
 *
 *  - **No malaise controller.** "The console, grammeter, malaise engine, and
 *    orchestrator do not apply malaise to themselves." The console asks the
 *    engine to break other services; breaking the panel would hide the demo.
 *  - **No readiness checks.** "Never check downstream services." The console has
 *    no database and no NATS connection, so it is ready as soon as it is
 *    listening, even with every upstream on fire. That is the truth, and it is
 *    also what stops one sick upstream from taking the console out of service and
 *    blinding the operator at the worst moment.
 */

import { existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify'
import fastifyCookie from '@fastify/cookie'
import fastifyStatic from '@fastify/static'
import type { ConsoleConfig } from './config.js'
import { Collector } from './collector.js'
import { createGateway, type Gateway } from './gateway.js'
import {
  consoleMetrics,
  createApp,
  errorBody,
  type ConsoleMetrics,
  type Logger,
  type Telemetry,
} from './platform/index.js'
import { GUEST, sessionFromToken, type Session } from './session.js'
import { registerProxyRoutes } from './routes/proxy.js'
import { registerSessionRoutes } from './routes/session.js'
import { registerStorefrontRoutes } from './routes/storefront.js'
import { registerStreamRoutes } from './routes/stream.js'
import { createClusterReader, type ClusterReader } from './topology/kubernetes.js'
import { createPrometheusReader, type PrometheusReader } from './topology/prometheus.js'

export interface BuildOptions {
  config: ConsoleConfig
  logger: Logger
  telemetry?: Telemetry
  metrics?: ConsoleMetrics
  gateway?: Gateway
  prometheus?: PrometheusReader
  cluster?: ClusterReader
  /** Serve the built SPA. Off in unit tests, on in the image. */
  serveStatic?: boolean
  now?: () => Date
}

export interface BuiltApp {
  app: FastifyInstance
  collector: Collector
  metrics: ConsoleMetrics
  gateway: Gateway
}

/** Where the built SPA lives relative to the compiled BFF entry point. */
export function defaultWebRoot(): string {
  const here = dirname(fileURLToPath(import.meta.url))
  // dist/bff/server/app.js -> dist/web
  return resolve(here, '..', '..', 'web')
}

export async function buildApp(opts: BuildOptions): Promise<BuiltApp> {
  const { config, logger } = opts
  const metrics = opts.metrics ?? consoleMetrics
  const gateway = opts.gateway ?? createGateway({ config, logger })
  const prometheus = opts.prometheus ?? createPrometheusReader({ gateway })
  const cluster =
    opts.cluster ??
    createClusterReader({
      namespace: config.podNamespace,
      logger,
      enabled: config.kubernetesEnabled,
    })

  const collector = new Collector({
    config,
    logger,
    metrics,
    gateway,
    prometheus,
    cluster,
    ...(opts.now === undefined ? {} : { now: opts.now }),
  })

  const app = await createApp({
    serviceName: config.serviceName,
    version: config.serviceVersion,
    logger,
    ...(opts.telemetry === undefined ? {} : { telemetry: opts.telemetry }),
    register: async (instance) => {
      await instance.register(fastifyCookie, { secret: config.sessionSecret })

      // The console's session sits alongside the kit's `request.identity`: the
      // kit reads identity from trusted internal headers, which a browser must
      // never be allowed to set, so the signed cookie is the console's only
      // source of truth about who is asking.
      instance.addHook('onRequest', async (request: FastifyRequest) => {
        request.session = readSession(request, config, opts.now?.() ?? new Date())
      })

      await instance.register(
        async (bff) => {
          registerSessionRoutes(bff, {
            config,
            gateway,
            logger,
            ...(opts.now === undefined ? {} : { now: opts.now }),
          })
          registerStreamRoutes(bff, { collector, logger })
          registerStorefrontRoutes(bff, { gateway, logger, metrics })
          registerProxyRoutes(bff, { gateway, logger, metrics })
        },
        { prefix: '/bff' },
      )

      await registerSpa(instance, {
        config,
        logger,
        serveStatic: opts.serveStatic !== false,
      })
    },
  })

  return { app, collector, metrics, gateway }
}

async function registerSpa(
  app: FastifyInstance,
  opts: { config: ConsoleConfig; logger: Logger; serveStatic: boolean },
): Promise<void> {
  if (!opts.serveStatic) return

  const root = opts.config.webRoot !== '' ? resolve(opts.config.webRoot) : defaultWebRoot()
  if (!existsSync(root)) {
    opts.logger.warn(
      { root },
      'no built SPA found: run `npm run build:web`. The BFF still serves /bff.',
    )
    return
  }

  await app.register(fastifyStatic, {
    root,
    prefix: '/',
    index: ['index.html'],
    // `wildcard: false` registers one route per built file instead of claiming
    // `GET /*`, which leaves the wildcard free for the history fallback below.
    // The kit's own not-found handler already answers everything else with the
    // house error shape, and Fastify allows only one of those per prefix.
    wildcard: false,
    setHeaders(reply, path) {
      // @fastify/static passes the FastifyReply, not the raw ServerResponse, so
      // this must go through `reply.header`. Using `res.setHeader` here throws
      // inside the send stream, where Fastify's error handler cannot catch it,
      // and takes the process down on the first page load.
      if (path.endsWith('.html')) {
        // Vite fingerprints the assets, so index.html is the one file that must
        // never be cached: it is what points at the new hashes.
        reply.header('cache-control', 'no-cache')
      } else if (path.includes('/assets/')) {
        reply.header('cache-control', 'public, max-age=31536000, immutable')
      }
    },
  })

  // History fallback, so /malaise and /works survive a page refresh and a pasted
  // link. A concrete file route wins over this wildcard, so only unmatched paths
  // arrive here. An API path or something that looks like a file still gets a
  // JSON 404, because serving HTML to a fetch() would be a lie.
  app.route({
    method: ['GET', 'HEAD'],
    url: '/*',
    handler: async (request: FastifyRequest, reply: FastifyReply) => {
      const path = request.url.split('?')[0] ?? '/'
      if (isApiPath(path) || hasExtension(path)) {
        await reply.code(404).send(errorBody('not_found', `No route for ${request.url}`))
        return
      }
      await reply.sendFile('index.html')
    },
  })

  opts.logger.info({ root }, 'serving the single-page app')
}

export function isApiPath(path: string): boolean {
  return (
    path.startsWith('/bff') || path === '/healthz' || path === '/readyz' || path === '/metrics'
  )
}

export function hasExtension(path: string): boolean {
  const last = path.split('/').pop() ?? ''
  return /\.[A-Za-z0-9]{1,8}$/.test(last)
}

function readSession(request: FastifyRequest, config: ConsoleConfig, now: Date): Session {
  const raw = request.cookies?.[config.cookieName]
  if (raw === undefined || raw === '') return GUEST
  const unsigned = request.unsignCookie(raw)
  if (!unsigned.valid || unsigned.value === null) return GUEST
  return sessionFromToken(unsigned.value, now) ?? GUEST
}
