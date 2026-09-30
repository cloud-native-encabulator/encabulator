/**
 * The Fastify application factory that every TypeScript service in the Encabulator Works
 * is built from (design section 7).
 *
 * One port serves the API, `/healthz`, `/readyz` and `/metrics`. Request ids, the error
 * shape, the request-duration histogram, malaise injection, and graceful shutdown all
 * live here so a service file contains only its own domain routes.
 */

import { randomUUID } from 'node:crypto';
import Fastify, {
  type FastifyBaseLogger,
  type FastifyError,
  type FastifyInstance,
  type FastifyReply,
  type FastifyRequest,
  LogController,
} from 'fastify';
import type { Logger } from 'pino';
import { errorBody, HttpError } from './errors.js';
import { ANONYMOUS, type Identity, identityFromHeaders, REQUEST_ID_HEADER } from './identity.js';
import type { MalaiseController } from './malaise.js';
import {
  httpServerDuration,
  metricsContentType,
  metricsText,
  startDefaultMetrics,
} from './metrics.js';
import { HEALTH_PATH, isInfraPath, METRICS_PATH, READY_PATH } from './paths.js';
import type { Telemetry } from './telemetry.js';

export interface ReadinessCheck {
  /** Shows up in the `/readyz` body, so name it after the dependency. */
  name: string;
  check: () => Promise<void> | void;
}

export interface EncabulatorAppState {
  serviceName: string;
  version: string;
  shuttingDown: boolean;
  readiness: ReadinessCheck[];
}

declare module 'fastify' {
  interface FastifyInstance {
    encabulator: EncabulatorAppState;
    /** Adds a readiness check after construction (handy once a client has connected). */
    addReadinessCheck(check: ReadinessCheck): void;
    /** Makes `/readyz` fail without stopping the server. Used on SIGTERM. */
    failReadiness(): void;
  }
  interface FastifyRequest {
    identity: Identity;
  }
}

export interface CreateAppOptions {
  serviceName: string;
  logger: Logger;
  version?: string;
  telemetry?: Telemetry | undefined;
  malaise?: MalaiseController | undefined;
  readiness?: ReadinessCheck[];
  bodyLimit?: number;
  /** Extra routes and plugins. Called after the kit's own wiring. */
  register?: (app: FastifyInstance) => Promise<void> | void;
}

/** Server timeouts from design section 7: read-header 5s, idle 60s. */
export const HEADERS_TIMEOUT_MS = 5_000;
export const KEEP_ALIVE_TIMEOUT_MS = 60_000;
export const DRAIN_TIMEOUT_MS = 20_000;

const CONGESTION_RELEASE = Symbol('malaiseRelease');
const IDENTITY = Symbol('identity');

type WithRelease = FastifyRequest & { [CONGESTION_RELEASE]?: () => void };
type WithIdentity = FastifyRequest & { [IDENTITY]?: Identity };

/**
 * Fastify's own per-request log lines are replaced by one structured "request completed"
 * line from the kit's `onResponse` hook, and the request id is logged as `request_id`.
 */
class EncabulatorLogController extends LogController {
  constructor() {
    super({ disableRequestLogging: true, requestIdLogLabel: 'request_id' });
  }
}

function routeLabel(request: FastifyRequest): string {
  // Never the raw path: the design is explicit about the label being the route template.
  return request.routeOptions?.url ?? 'unmatched';
}

export async function createApp(opts: CreateAppOptions): Promise<FastifyInstance> {
  startDefaultMetrics();

  const app = Fastify({
    loggerInstance: opts.logger as unknown as FastifyBaseLogger,
    logController: new EncabulatorLogController(),
    requestIdHeader: REQUEST_ID_HEADER,
    genReqId: () => randomUUID(),
    keepAliveTimeout: KEEP_ALIVE_TIMEOUT_MS,
    bodyLimit: opts.bodyLimit ?? 1_048_576,
    trustProxy: true,
  });

  app.server.headersTimeout = HEADERS_TIMEOUT_MS;

  const state: EncabulatorAppState = {
    serviceName: opts.serviceName,
    version: opts.version ?? '1.0.0',
    shuttingDown: false,
    readiness: [...(opts.readiness ?? [])],
  };
  app.decorate('encabulator', state);
  app.decorate('addReadinessCheck', (check: ReadinessCheck) => {
    state.readiness.push(check);
  });
  app.decorate('failReadiness', () => {
    state.shuttingDown = true;
  });
  // Fastify 5 only allows getter/setter decorators for reference types.
  app.decorateRequest('identity', {
    getter(this: FastifyRequest) {
      return (this as WithIdentity)[IDENTITY] ?? ANONYMOUS;
    },
  });

  // Fastify spans. Registered explicitly rather than by module patching, because ESM
  // hoisting means `fastify` is usually loaded before the SDK starts.
  const tracingPlugin = opts.telemetry?.fastifyPlugin();
  if (tracingPlugin) {
    // biome-ignore lint/suspicious/noExplicitAny: the plugin's own types are loose.
    await app.register(tracingPlugin as any);
  }

  app.addHook('onRequest', async (request: FastifyRequest, reply: FastifyReply) => {
    reply.header(REQUEST_ID_HEADER, request.id);
    (request as WithIdentity)[IDENTITY] = identityFromHeaders(
      request.headers as Record<string, string | undefined>,
    );

    if (isInfraPath(request.url)) return;

    if (opts.malaise?.enabled) {
      (request as WithRelease)[CONGESTION_RELEASE] = await opts.malaise.applyInbound();
    }
  });

  const releaseCongestion = (request: FastifyRequest) => {
    const holder = request as WithRelease;
    const release = holder[CONGESTION_RELEASE];
    if (release) {
      delete holder[CONGESTION_RELEASE];
      release();
    }
  };

  app.addHook('onResponse', async (request: FastifyRequest, reply: FastifyReply) => {
    releaseCongestion(request);
    if (isInfraPath(request.url)) return;

    const route = routeLabel(request);
    httpServerDuration.observe(
      {
        http_request_method: request.method,
        http_route: route,
        http_response_status_code: String(reply.statusCode),
      },
      reply.elapsedTime / 1000,
    );
    request.log.info(
      {
        method: request.method,
        route,
        status: reply.statusCode,
        duration_ms: Math.round(reply.elapsedTime * 100) / 100,
      },
      'request completed',
    );
  });

  app.addHook('onError', async (request: FastifyRequest) => {
    releaseCongestion(request);
  });

  app.setErrorHandler((raw, request, reply) => {
    if (raw instanceof HttpError) {
      reply.code(raw.statusCode).send(raw.toBody());
      return;
    }
    const error = raw as FastifyError;
    // Fastify schema validation.
    if (error.validation) {
      reply.code(400).send(errorBody('invalid_request', error.message));
      return;
    }
    const status = typeof error.statusCode === 'number' ? error.statusCode : 500;
    if (status >= 500) {
      request.log.error({ err: error.message, stack: error.stack }, 'request failed');
      reply.code(status).send(errorBody('internal_error', 'The encabulator is unwell.'));
      return;
    }
    reply.code(status).send(errorBody(error.code ?? 'request_failed', error.message));
  });

  app.setNotFoundHandler((request, reply) => {
    reply.code(404).send(errorBody('not_found', `No route for ${request.method} ${request.url}`));
  });

  app.get(HEALTH_PATH, async () => ({ status: 'ok' }));

  app.get(READY_PATH, async (_request, reply) => {
    if (state.shuttingDown) {
      return reply.code(503).send(errorBody('shutting_down', 'Draining.'));
    }
    const failures: Array<{ name: string; error: string }> = [];
    await Promise.all(
      state.readiness.map(async (check) => {
        try {
          await check.check();
        } catch (err) {
          failures.push({
            name: check.name,
            error: err instanceof Error ? err.message : String(err),
          });
        }
      }),
    );
    if (failures.length > 0) {
      return reply
        .code(503)
        .send({ error: { code: 'not_ready', message: 'Dependencies unavailable' }, failures });
    }
    return reply.code(200).send({ status: 'ok', checks: state.readiness.map((c) => c.name) });
  });

  app.get(METRICS_PATH, async (_request, reply) => {
    reply.header('content-type', metricsContentType());
    return metricsText();
  });

  await opts.register?.(app);

  return app;
}

export interface ServeOptions {
  port: number;
  host?: string;
  logger: Logger;
  /** Run in order after the HTTP server drains: NATS drain, Valkey quit, OTel flush. */
  shutdownHooks?: Array<() => Promise<void> | void>;
  drainTimeoutMs?: number;
  /** Signals to trap. Pass `[]` in tests. */
  signals?: NodeJS.Signals[];
}

export interface RunningService {
  readonly address: string;
  /** Idempotent. Fails readiness, drains, then runs the shutdown hooks. */
  shutdown(reason?: string): Promise<void>;
}

/**
 * Listen, and wire graceful shutdown: on SIGTERM fail readiness, stop accepting, drain
 * for up to 20 s, then exit (design section 7).
 */
export async function serveApp(app: FastifyInstance, opts: ServeOptions): Promise<RunningService> {
  const drainTimeoutMs = opts.drainTimeoutMs ?? DRAIN_TIMEOUT_MS;
  const signals = opts.signals ?? ['SIGTERM', 'SIGINT'];

  await app.listen({ port: opts.port, host: opts.host ?? '0.0.0.0' });
  const address = app.server.address();
  const printable =
    typeof address === 'string' ? address : `${address?.address}:${address?.port ?? opts.port}`;
  opts.logger.info({ address: printable }, 'the encabulator is listening');

  let shuttingDown: Promise<void> | undefined;

  const shutdown = async (reason = 'shutdown'): Promise<void> => {
    if (shuttingDown) return shuttingDown;
    shuttingDown = (async () => {
      opts.logger.info({ reason }, 'draining');
      app.failReadiness();
      const closed = app.close();
      await Promise.race([
        closed,
        new Promise<void>((resolve) => {
          const timer = setTimeout(resolve, drainTimeoutMs);
          timer.unref?.();
        }),
      ]);
      for (const hook of opts.shutdownHooks ?? []) {
        try {
          await hook();
        } catch (err) {
          opts.logger.warn(
            { err: err instanceof Error ? err.message : String(err) },
            'shutdown hook failed',
          );
        }
      }
      opts.logger.info('stopped');
    })();
    return shuttingDown;
  };

  for (const signal of signals) {
    process.once(signal, () => {
      void shutdown(signal).then(
        () => process.exit(0),
        () => process.exit(1),
      );
    });
  }

  return { address: printable, shutdown };
}
