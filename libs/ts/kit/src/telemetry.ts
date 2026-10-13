/**
 * OpenTelemetry setup (design section 7, "Tracing").
 *
 * OTLP over HTTP/protobuf to `OTEL_EXPORTER_OTLP_ENDPOINT`. When that variable is unset
 * the whole thing is a no-op, which keeps unit tests and local runs quiet.
 *
 * Import this module (`@encabulator/kit/telemetry`) rather than the kit barrel from a
 * service entrypoint: it deliberately imports no Fastify, ioredis or undici code, so
 * the instrumentations get a chance to register before those modules load. (Once the
 * SDK has started it does `require` ioredis, when the service has it, so the patch
 * reaches an ESM service too; see `REQUIRE_PATCHED_MODULES`.) The usual
 * shape of a service entrypoint is:
 *
 *     import { startTelemetry } from '@encabulator/kit/telemetry';
 *     const telemetry = startTelemetry({ serviceName: 'my-service' });
 *     const { main } = await import('./main.js');
 *     await main(telemetry);
 */

import { createRequire } from 'node:module';
import path from 'node:path';
import { FastifyOtelInstrumentation } from '@fastify/otel';
import { type Context, context, type Span, type Tracer, trace } from '@opentelemetry/api';
import { suppressTracing } from '@opentelemetry/core';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-proto';
import type { Instrumentation } from '@opentelemetry/instrumentation';
import { HttpInstrumentation } from '@opentelemetry/instrumentation-http';
import { IORedisInstrumentation } from '@opentelemetry/instrumentation-ioredis';
import { UndiciInstrumentation } from '@opentelemetry/instrumentation-undici';
import { defaultResource, resourceFromAttributes } from '@opentelemetry/resources';
import { NodeSDK } from '@opentelemetry/sdk-node';
import { ATTR_SERVICE_NAME, ATTR_SERVICE_VERSION } from '@opentelemetry/semantic-conventions';
import { envRaw, serviceVersion } from './config.js';
import { isInfraPath } from './paths.js';

/** Poller endpoints that must never produce spans (design section 7). */
const UNTRACED_PATH_PREFIXES = ['/v1/directives', '/v1/flags'];

function isUntracedOutgoingPath(path: string): boolean {
  const clean = path.split('?', 1)[0] ?? path;
  return UNTRACED_PATH_PREFIXES.some((p) => clean.startsWith(p)) || isInfraPath(clean);
}

export interface TelemetryOptions {
  serviceName: string;
  serviceVersion?: string;
  /** Defaults to `OTEL_EXPORTER_OTLP_ENDPOINT`. Empty or unset means "no tracing". */
  endpoint?: string | undefined;
  /** Extra instrumentations, e.g. a service-specific database client. */
  instrumentations?: Instrumentation[];
  /**
   * A file the service's own dependencies resolve from, used to load the
   * require-patched modules (see `REQUIRE_PATCHED_MODULES`). Defaults to the
   * process entry point, which is the service's `dist/index.js`.
   */
  modulesFrom?: string;
}

/**
 * Modules whose instrumentation patches their CommonJS exports when they are
 * first `require`d. A service written as ESM loads them with `import`, which
 * Node resolves without `require`, so require-in-the-middle never sees them and
 * the instrumentation silently patches nothing: the depot and dispatch produced
 * no Valkey client spans at all. Loading each one once through `require`, right
 * after the SDK starts, patches the module object the service's later `import`
 * receives, because Node keeps one CommonJS instance for both.
 */
export const REQUIRE_PATCHED_MODULES: readonly string[] = ['ioredis'];

/**
 * `require` each of `REQUIRE_PATCHED_MODULES` that `fromFile` can resolve, and
 * return the ones loaded. A module the service does not depend on is skipped.
 */
export function loadRequirePatchedModules(fromFile: string): string[] {
  const load = createRequire(fromFile);
  const loaded: string[] = [];
  for (const id of REQUIRE_PATCHED_MODULES) {
    try {
      load(id);
      loaded.push(id);
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== 'MODULE_NOT_FOUND') throw err;
    }
  }
  return loaded;
}

/**
 * The backing store is Valkey, which speaks the Redis protocol. ioredis 0.70's
 * instrumentation reports only the stable `db.system.name=redis`; keep
 * `db.system=redis` (the protocol, and what the Python services report) and set
 * `db.system.name=valkey` to name the store: the collector's service_graph
 * connector names the server of a database edge after `db.system.name`, which is
 * the console's `valkey` node.
 */
export function markValkey(span: Pick<Span, 'setAttribute'>): void {
  span.setAttribute('db.system', 'redis');
  span.setAttribute('db.system.name', 'valkey');
}

/** Structural type for the Fastify plugin produced by `@fastify/otel`. */
// biome-ignore lint/suspicious/noExplicitAny: Fastify's plugin signature is intentionally loose here.
export type FastifyTracingPlugin = (instance: any, opts: any, done: any) => void;

export interface Telemetry {
  /** True when an exporter endpoint was configured and the SDK actually started. */
  readonly enabled: boolean;
  readonly serviceName: string;
  tracer(name?: string): Tracer;
  /**
   * The Fastify instrumentation plugin, or undefined when tracing is disabled. The app
   * factory registers it; ESM hoisting makes module patching unreliable, so Fastify is
   * instrumented explicitly instead.
   */
  fastifyPlugin(): FastifyTracingPlugin | undefined;
  shutdown(): Promise<void>;
}

/** Suffix an OTLP base endpoint with the traces path if the caller did not. */
export function tracesUrl(endpoint: string): string {
  const base = endpoint.replace(/\/+$/, '');
  return base.endsWith('/v1/traces') ? base : `${base}/v1/traces`;
}

function buildInstrumentations(
  fastifyInstrumentation: FastifyOtelInstrumentation,
  extra: Instrumentation[],
): Instrumentation[] {
  const http = new HttpInstrumentation({
    // Health, readiness and metrics scrapes would otherwise dominate trace volume.
    ignoreIncomingRequestHook: (req) => isInfraPath(req.url),
    ignoreOutgoingRequestHook: (req) =>
      isUntracedOutgoingPath(typeof req.path === 'string' ? req.path : ''),
  });

  const undici = new UndiciInstrumentation({
    ignoreRequestHook: (req) => isUntracedOutgoingPath(req.path),
  });

  const ioredis = new IORedisInstrumentation({
    requireParentSpan: false,
    requestHook: (span) => markValkey(span),
  });

  return [http, undici, ioredis, fastifyInstrumentation as Instrumentation, ...extra];
}

class NoopTelemetry implements Telemetry {
  readonly enabled = false;
  constructor(readonly serviceName: string) {}
  tracer(name = this.serviceName): Tracer {
    return trace.getTracer(name);
  }
  fastifyPlugin(): undefined {
    return undefined;
  }
  async shutdown(): Promise<void> {
    /* nothing to flush */
  }
}

/**
 * Start the OTel SDK. Returns a no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset.
 */
export function startTelemetry(opts: TelemetryOptions): Telemetry {
  const endpoint = opts.endpoint ?? envRaw('OTEL_EXPORTER_OTLP_ENDPOINT');
  if (!endpoint) {
    return new NoopTelemetry(opts.serviceName);
  }

  const version = opts.serviceVersion ?? serviceVersion();
  const fastifyInstrumentation = new FastifyOtelInstrumentation({
    registerOnInitialization: false,
    ignorePaths: (routeOpts) => isInfraPath(routeOpts.url),
  });

  const sdk = new NodeSDK({
    resource: defaultResource().merge(
      resourceFromAttributes({
        [ATTR_SERVICE_NAME]: opts.serviceName,
        [ATTR_SERVICE_VERSION]: version,
        'service.namespace': 'encabulator',
      }),
    ),
    traceExporter: new OTLPTraceExporter({ url: tracesUrl(endpoint) }),
    instrumentations: buildInstrumentations(fastifyInstrumentation, opts.instrumentations ?? []),
  });
  sdk.start();
  loadRequirePatchedModules(
    opts.modulesFrom ?? process.argv[1] ?? path.join(process.cwd(), 'index.js'),
  );

  let stopped = false;
  return {
    enabled: true,
    serviceName: opts.serviceName,
    tracer: (name = opts.serviceName) => trace.getTracer(name, version),
    fastifyPlugin: () => fastifyInstrumentation.plugin() as unknown as FastifyTracingPlugin,
    shutdown: async () => {
      if (stopped) return;
      stopped = true;
      await sdk.shutdown();
    },
  };
}

/** Run `fn` with tracing suppressed. Used by the malaise and flag pollers. */
export async function withSuppressedTracing<T>(fn: () => Promise<T>): Promise<T> {
  return context.with(suppressTracing(context.active()), fn);
}

export function suppressedContext(): Context {
  return suppressTracing(context.active());
}
