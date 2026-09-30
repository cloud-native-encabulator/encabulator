/**
 * Structured logging (design section 7, "Logging").
 *
 * JSON lines on stdout with `time` (RFC 3339 with milliseconds), `level`, `msg`, and
 * `service`. `trace_id` / `span_id` are attached automatically whenever a span is
 * recording, and `request_id` is attached per HTTP request by the app factory.
 */

import { trace } from '@opentelemetry/api';
import { type DestinationStream, type Logger, type LoggerOptions, pino } from 'pino';

export type { Logger } from 'pino';

export interface CreateLoggerOptions {
  service: string;
  level?: string;
  /** Extra fields added to every line. */
  base?: Record<string, unknown>;
  /** Destination override; used by tests to capture output. */
  destination?: DestinationStream;
}

/** Pulled into every log line so traces and logs can be correlated in one click. */
function traceMixin(): Record<string, string> {
  const span = trace.getActiveSpan();
  if (!span) return {};
  const ctx = span.spanContext();
  if (!ctx.traceId || ctx.traceId === '00000000000000000000000000000000') return {};
  return { trace_id: ctx.traceId, span_id: ctx.spanId };
}

export function createLogger(opts: CreateLoggerOptions): Logger {
  const options: LoggerOptions = {
    level: opts.level ?? 'info',
    base: { service: opts.service, ...opts.base },
    messageKey: 'msg',
    // RFC 3339 with milliseconds, which is what the design asks for (pino's default
    // is epoch millis).
    timestamp: () => `,"time":"${new Date().toISOString()}"`,
    formatters: {
      level: (label) => ({ level: label }),
    },
    mixin: traceMixin,
    // Never let a stray password or token reach stdout.
    redact: {
      paths: [
        'password',
        'token',
        'accessToken',
        'authorization',
        'req.headers.authorization',
        'req.headers.cookie',
        '*.password',
        '*.accessToken',
      ],
      censor: '[redacted]',
    },
  };
  return opts.destination ? pino(options, opts.destination) : pino(options);
}
