/**
 * Prometheus metrics (design section 7, "Metrics").
 *
 * Everything lands in prom-client's default registry so that service-local metrics
 * created with plain `new Counter({...})` show up on `/metrics` without extra wiring.
 */

import {
  Counter,
  collectDefaultMetrics,
  Gauge,
  Histogram,
  type Registry,
  register,
} from 'prom-client';

export type { Registry } from 'prom-client';
export { Counter, Gauge, Histogram, register, Summary } from 'prom-client';

export const registry: Registry = register;

/** The bucket set mandated by the design for both HTTP histograms. */
export const HTTP_DURATION_BUCKETS = [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10];

/**
 * Metrics are module-level singletons, but test files import this module repeatedly in
 * the same process. Re-use an already-registered collector instead of throwing.
 */
function singleton<T>(name: string, create: () => T): T {
  const existing = registry.getSingleMetric(name);
  if (existing) return existing as unknown as T;
  return create();
}

export const httpServerDuration = singleton(
  'http_server_request_duration_seconds',
  () =>
    new Histogram({
      name: 'http_server_request_duration_seconds',
      help: 'Duration of inbound HTTP requests in seconds.',
      labelNames: ['http_request_method', 'http_route', 'http_response_status_code'],
      buckets: HTTP_DURATION_BUCKETS,
    }),
);

export const httpClientDuration = singleton(
  'http_client_request_duration_seconds',
  () =>
    new Histogram({
      name: 'http_client_request_duration_seconds',
      help: 'Duration of outbound HTTP requests in seconds.',
      labelNames: ['http_request_method', 'server_address', 'http_response_status_code'],
      buckets: HTTP_DURATION_BUCKETS,
    }),
);

export const malaiseApplied = singleton(
  'encabulator_malaise_applied_total',
  () =>
    new Counter({
      name: 'encabulator_malaise_applied_total',
      help: 'Number of times an injected malaise directive was applied.',
      labelNames: ['kind'],
    }),
);

export const messagesConsumed = singleton(
  'encabulator_messages_consumed_total',
  () =>
    new Counter({
      name: 'encabulator_messages_consumed_total',
      help: 'JetStream messages handled, by subject and outcome.',
      labelNames: ['subject', 'result'],
    }),
);

export const messagesPublished = singleton(
  'encabulator_messages_published_total',
  () =>
    new Counter({
      name: 'encabulator_messages_published_total',
      help: 'JetStream messages published, by subject and outcome.',
      labelNames: ['subject', 'result'],
    }),
);

let defaultMetricsStarted = false;

/** Node process and runtime metrics, once per process. */
export function startDefaultMetrics(): void {
  if (defaultMetricsStarted) return;
  defaultMetricsStarted = true;
  collectDefaultMetrics({ register: registry });
}

export async function metricsText(): Promise<string> {
  return registry.metrics();
}

export function metricsContentType(): string {
  return registry.contentType;
}

/** Convenience helpers so service code does not have to repeat the singleton dance. */
export function counter(opts: {
  name: string;
  help: string;
  labelNames?: readonly string[];
}): Counter<string> {
  return singleton(
    opts.name,
    () =>
      new Counter({
        name: opts.name,
        help: opts.help,
        labelNames: [...(opts.labelNames ?? [])],
      }),
  );
}

export function gauge(opts: {
  name: string;
  help: string;
  labelNames?: readonly string[];
}): Gauge<string> {
  return singleton(
    opts.name,
    () =>
      new Gauge({
        name: opts.name,
        help: opts.help,
        labelNames: [...(opts.labelNames ?? [])],
      }),
  );
}

export function histogram(opts: {
  name: string;
  help: string;
  labelNames?: readonly string[];
  buckets?: number[];
}): Histogram<string> {
  return singleton(
    opts.name,
    () =>
      new Histogram({
        name: opts.name,
        help: opts.help,
        labelNames: [...(opts.labelNames ?? [])],
        ...(opts.buckets ? { buckets: opts.buckets } : {}),
      }),
  );
}
