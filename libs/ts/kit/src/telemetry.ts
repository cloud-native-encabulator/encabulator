/**
 * OpenTelemetry setup (design section 7, "Tracing").
 *
 * OTLP over HTTP/protobuf to `OTEL_EXPORTER_OTLP_ENDPOINT`. When that variable is unset
 * the whole thing is a no-op, which keeps unit tests and local runs quiet.
 *
 * Import this module (`@encabulator/kit/telemetry`) rather than the kit barrel from a
 * service entrypoint: it deliberately pulls in no Fastify, ioredis or undici code, so
 * the instrumentations get a chance to register before those modules load. The usual
 * shape of a service entrypoint is:
 *
 *     import { startTelemetry } from '@encabulator/kit/telemetry';
 *     const telemetry = startTelemetry({ serviceName: 'my-service' });
 *     const { main } = await import('./main.js');
 *     await main(telemetry);
 */

import { FastifyOtelInstrumentation } from '@fastify/otel';
import { type Context, context, type Tracer, trace } from '@opentelemetry/api';
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

  const ioredis = new IORedisInstrumentation({ requireParentSpan: false });

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
