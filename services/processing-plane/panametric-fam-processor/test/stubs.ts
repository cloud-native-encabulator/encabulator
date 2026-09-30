/**
 * Programmable stand-ins for the processor's three neighbours.
 *
 * They are real Fastify servers on real sockets, so the HTTP client, its retries, its
 * timeouts and the circuit breakers all behave exactly as they will in the cluster. Only
 * the *behaviour* of the upstream is faked, never the transport.
 */

import {
  type EventEnvelope,
  type EventPublisher,
  type FlagClient,
  newEnvelope,
} from '@encabulator/kit';
import Fastify, { type FastifyInstance } from 'fastify';

/** Assert-and-narrow, so tests can index into recorded calls without optional chaining. */
export function must<T>(value: T | undefined, what = 'value'): T {
  if (value === undefined) throw new Error(`expected a ${what}, got undefined`);
  return value;
}

export interface StubCall {
  method: string;
  url: string;
  body: unknown;
  headers: Record<string, string | undefined>;
}

export type StubHandler = (call: StubCall) => {
  status: number;
  body?: unknown;
  delayMs?: number;
};

export class Upstream {
  private app: FastifyInstance | undefined;
  readonly calls: StubCall[] = [];
  private routes = new Map<string, StubHandler>();
  url = '';

  constructor(readonly name: string) {}

  /** `on('POST /v1/quotes', ...)`. `*` matches any path with the given method. */
  on(route: string, handler: StubHandler): this {
    this.routes.set(route, handler);
    return this;
  }

  async start(): Promise<this> {
    const app = Fastify({ logger: false });
    app.all('/*', async (request, reply) => {
      const call: StubCall = {
        method: request.method,
        url: request.url,
        body: request.body,
        headers: request.headers as Record<string, string | undefined>,
      };
      this.calls.push(call);
      const path = request.url.split('?', 1)[0] ?? request.url;
      const handler =
        this.routes.get(`${request.method} ${path}`) ?? this.routes.get(`${request.method} *`);
      if (!handler) {
        return reply.code(404).send({ error: { code: 'not_stubbed', message: call.url } });
      }
      const result = handler(call);
      if (result.delayMs) {
        await new Promise((resolve) => setTimeout(resolve, result.delayMs));
      }
      return reply.code(result.status).send(result.body ?? {});
    });
    await app.listen({ port: 0, host: '127.0.0.1' });
    const address = app.server.address();
    const port = typeof address === 'object' && address ? address.port : 0;
    this.url = `http://127.0.0.1:${port}`;
    this.app = app;
    return this;
  }

  async stop(): Promise<void> {
    await this.app?.close();
  }

  callsTo(method: string, path: string): StubCall[] {
    return this.calls.filter(
      (c) => c.method === method && (c.url.split('?', 1)[0] ?? c.url) === path,
    );
  }

  /** Forget both recorded calls and registered routes, so tests cannot leak into each other. */
  reset(): void {
    this.calls.length = 0;
    this.routes.clear();
  }
}

/** Records what the saga publishes instead of talking to JetStream. */
export class RecordingPublisher {
  readonly published: Array<{ subject: string; envelope: EventEnvelope<unknown> }> = [];
  failNext = false;

  async publish<T>(subject: string, data: T): Promise<EventEnvelope<T>> {
    if (this.failNext) {
      this.failNext = false;
      throw new Error('jetstream unreachable');
    }
    const envelope = newEnvelope(subject, 'panametric-fam-processor', data);
    this.published.push({ subject, envelope });
    return envelope;
  }

  async publishEnvelope(): Promise<void> {
    throw new Error('not used by the saga');
  }

  subjects(): string[] {
    return this.published.map((p) => p.subject);
  }

  dataFor(subject: string): unknown {
    return this.published.find((p) => p.subject === subject)?.envelope.data;
  }

  asPublisher(): EventPublisher {
    return this as unknown as EventPublisher;
  }

  reset(): void {
    this.published.length = 0;
    this.failNext = false;
  }
}

/** A flag client that answers from a plain object, no girdlespring required. */
export function stubFlags(overrides: Record<string, boolean | number> = {}): FlagClient {
  const values: Record<string, boolean | number> = {
    'retro-encabulator-sales': true,
    ...overrides,
  };
  return {
    boolean: (key: string) => Boolean(values[key]),
    number: (key: string) => Number(values[key] ?? 0),
    string: (key: string) => String(values[key] ?? ''),
  } as unknown as FlagClient;
}
