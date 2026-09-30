/**
 * NATS JetStream helpers (design section 5).
 *
 * One stream, `ENCABULATOR`, subjects `encabulator.>`. Every publisher and consumer calls
 * `ensureStream` at startup with exactly the config below; nobody updates an existing
 * stream. Publishes set `Nats-Msg-Id` to the event id for deduplication and carry W3C
 * trace context in NATS headers, with PRODUCER and CONSUMER spans so the service graph
 * links producer to consumer.
 *
 * Uses the modular v3 NATS clients (`@nats-io/*`).
 */

import {
  AckPolicy,
  DeliverPolicy,
  type JetStreamClient,
  type JetStreamManager,
  type JsMsg,
  jetstream,
  jetstreamManager,
  ReplayPolicy,
  RetentionPolicy,
  StorageType,
} from '@nats-io/jetstream';
import { headers, type MsgHdrs, type NatsConnection, nanos } from '@nats-io/nats-core';
import { connect } from '@nats-io/transport-node';
import {
  context,
  propagation,
  SpanKind,
  SpanStatusCode,
  type Tracer,
  trace,
} from '@opentelemetry/api';
import type { Logger } from 'pino';
import { envString } from './config.js';
import { newId, nowRfc3339 } from './ids.js';
import { MalaiseNak } from './malaise.js';
import { messagesConsumed, messagesPublished } from './metrics.js';

/**
 * Re-exported so services can type NATS objects without taking a direct dependency on the
 * client packages.
 */
export type { JetStreamClient, JetStreamManager, JsMsg, StreamInfo } from '@nats-io/jetstream';
export type { MsgHdrs, NatsConnection } from '@nats-io/nats-core';

export const STREAM_NAME = 'ENCABULATOR';
export const STREAM_SUBJECTS = ['encabulator.>'] as const;
export const STREAM_MAX_AGE_MS = 24 * 60 * 60 * 1000;
export const STREAM_DUPLICATE_WINDOW_MS = 2 * 60 * 1000;

export const DEFAULT_ACK_WAIT_MS = 30_000;
export const DEFAULT_MAX_DELIVER = 5;

/** Every subject in the system (design section 5). */
export const Subjects = {
  OrderAccepted: 'encabulator.order.accepted',
  OrderScrapped: 'encabulator.order.scrapped',
  FabricationRequested: 'encabulator.fabrication.requested',
  FabricationStage: 'encabulator.fabrication.stage',
  FabricationCompleted: 'encabulator.fabrication.completed',
  InspectionPassed: 'encabulator.inspection.passed',
  InspectionFailed: 'encabulator.inspection.failed',
  ShipmentDispatched: 'encabulator.shipment.dispatched',
  ShipmentDelivered: 'encabulator.shipment.delivered',
} as const;

export type Subject = (typeof Subjects)[keyof typeof Subjects];

/** Durable consumer names (design section 5). */
export const Durables = {
  CasingLine: 'logarithmic-casing-line',
  Detector: 'side-fumbling-detector',
  Processor: 'panametric-fam-processor',
  Dispatch: 'nofer-trunnion-dispatch',
  Datastore: 'hydrocoptic-datastore',
} as const;

export interface EventEnvelope<T = unknown> {
  id: string;
  type: string;
  time: string;
  source: string;
  data: T;
}

export function newEnvelope<T>(type: string, source: string, data: T): EventEnvelope<T> {
  return { id: newId('event'), type, time: nowRfc3339(), source, data };
}

export interface ConnectNatsOptions {
  /** Defaults to `NATS_URL`, itself defaulting to `nats://nats:4222`. */
  url?: string;
  name: string;
  logger?: Logger;
  maxReconnectAttempts?: number;
}

export async function connectNats(opts: ConnectNatsOptions): Promise<NatsConnection> {
  const servers = opts.url ?? envString('NATS_URL', 'nats://nats:4222');
  const nc = await connect({
    servers,
    name: opts.name,
    maxReconnectAttempts: opts.maxReconnectAttempts ?? -1,
    reconnectTimeWait: 1_000,
  });
  if (opts.logger) {
    const logger = opts.logger;
    void (async () => {
      for await (const status of nc.status()) {
        logger.info({ nats_event: status.type }, 'nats status');
      }
    })();
  }
  return nc;
}

/**
 * Create the stream if it is missing, with exactly the configuration in the design. An
 * existing stream is left alone, including when a concurrently starting replica wins the
 * race.
 */
export async function ensureStream(nc: NatsConnection, logger?: Logger): Promise<JetStreamManager> {
  const jsm = await jetstreamManager(nc);
  try {
    await jsm.streams.info(STREAM_NAME);
    return jsm;
  } catch {
    // Fall through to create.
  }
  try {
    await jsm.streams.add({
      name: STREAM_NAME,
      subjects: [...STREAM_SUBJECTS],
      retention: RetentionPolicy.Limits,
      storage: StorageType.File,
      max_age: nanos(STREAM_MAX_AGE_MS),
      duplicate_window: nanos(STREAM_DUPLICATE_WINDOW_MS),
      num_replicas: 1,
    });
    logger?.info({ stream: STREAM_NAME }, 'created JetStream stream');
  } catch (err) {
    // Another replica created it between our info() and add(). That is fine.
    await jsm.streams.info(STREAM_NAME);
    logger?.debug({ err: errText(err) }, 'stream already existed');
  }
  return jsm;
}

const HEADER_SETTER = {
  set(carrier: MsgHdrs, key: string, value: string): void {
    carrier.set(key, value);
  },
};

const HEADER_GETTER = {
  keys(carrier: MsgHdrs | undefined): string[] {
    return carrier ? carrier.keys() : [];
  },
  get(carrier: MsgHdrs | undefined, key: string): string | undefined {
    if (!carrier) return undefined;
    const value = carrier.get(key);
    return value === '' ? undefined : value;
  },
};

export interface EventPublisherOptions {
  source: string;
  tracer?: Tracer;
  logger?: Logger;
}

/** Publishes envelopes to JetStream with dedup id, trace headers, and a PRODUCER span. */
export class EventPublisher {
  private readonly js: JetStreamClient;
  private readonly source: string;
  private readonly tracer: Tracer;
  private readonly logger: Logger | undefined;

  constructor(nc: NatsConnection, opts: EventPublisherOptions) {
    this.js = jetstream(nc);
    this.source = opts.source;
    this.tracer = opts.tracer ?? trace.getTracer(opts.source);
    this.logger = opts.logger;
  }

  /** Publish `data` on `subject`; the envelope `type` is the subject. */
  async publish<T>(subject: Subject | string, data: T): Promise<EventEnvelope<T>> {
    const envelope = newEnvelope(subject, this.source, data);
    await this.publishEnvelope(subject, envelope);
    return envelope;
  }

  async publishEnvelope<T>(subject: Subject | string, envelope: EventEnvelope<T>): Promise<void> {
    await this.tracer.startActiveSpan(
      `${subject} publish`,
      {
        kind: SpanKind.PRODUCER,
        attributes: {
          'messaging.system': 'nats',
          'messaging.operation.name': 'publish',
          'messaging.operation.type': 'send',
          'messaging.destination.name': subject,
          'messaging.message.id': envelope.id,
        },
      },
      async (span) => {
        const hdrs = headers();
        propagation.inject(context.active(), hdrs, HEADER_SETTER);
        try {
          await this.js.publish(subject, JSON.stringify(envelope), {
            msgID: envelope.id,
            headers: hdrs,
            timeout: 5_000,
          });
          messagesPublished.inc({ subject, result: 'ok' });
        } catch (err) {
          messagesPublished.inc({ subject, result: 'error' });
          span.setStatus({ code: SpanStatusCode.ERROR, message: errText(err) });
          this.logger?.error({ subject, err: errText(err) }, 'publish failed');
          throw err;
        } finally {
          span.end();
        }
      },
    );
  }
}

export interface ConsumerSpec {
  durable: string;
  filterSubject: string;
  ackWaitMs?: number;
  maxDeliver?: number;
  maxAckPending?: number;
}

/** Create the durable pull consumer if missing; leave an existing one alone. */
export async function ensureConsumer(jsm: JetStreamManager, spec: ConsumerSpec): Promise<void> {
  try {
    await jsm.consumers.info(STREAM_NAME, spec.durable);
    return;
  } catch {
    // Fall through to create.
  }
  try {
    await jsm.consumers.add(STREAM_NAME, {
      durable_name: spec.durable,
      name: spec.durable,
      ack_policy: AckPolicy.Explicit,
      deliver_policy: DeliverPolicy.All,
      replay_policy: ReplayPolicy.Instant,
      filter_subject: spec.filterSubject,
      ack_wait: nanos(spec.ackWaitMs ?? DEFAULT_ACK_WAIT_MS),
      max_deliver: spec.maxDeliver ?? DEFAULT_MAX_DELIVER,
      ...(spec.maxAckPending !== undefined ? { max_ack_pending: spec.maxAckPending } : {}),
    });
  } catch (err) {
    await jsm.consumers.info(STREAM_NAME, spec.durable);
    void err;
  }
}

export interface MessageContext {
  msg: JsMsg;
  subject: string;
  envelope: EventEnvelope<unknown>;
}

export type MessageHandler = (ctx: MessageContext) => Promise<void>;

export interface ConsumeOptions {
  spec: ConsumerSpec;
  handler: MessageHandler;
  logger: Logger;
  tracer?: Tracer;
  /** Messages consumed concurrently. Defaults to 1 (strictly sequential). */
  concurrency?: number;
  /** Delay before redelivery when a handler fails. */
  nakDelayMs?: number;
}

export interface Subscription {
  stop(): Promise<void>;
}

/**
 * Consume a durable pull consumer. Each message gets a CONSUMER span parented to the
 * publisher's span via the NATS trace headers. A handler that throws naks the message,
 * up to the consumer's `MaxDeliver`, after which JetStream drops it.
 */
export async function consume(
  nc: NatsConnection,
  jsm: JetStreamManager,
  opts: ConsumeOptions,
): Promise<Subscription> {
  await ensureConsumer(jsm, opts.spec);
  const js = jetstream(nc);
  const consumer = await js.consumers.get(STREAM_NAME, opts.spec.durable);
  const tracer = opts.tracer ?? trace.getTracer(opts.spec.durable);
  const concurrency = Math.max(1, opts.concurrency ?? 1);

  const messages = await consumer.consume({
    max_messages: opts.spec.maxAckPending ?? Math.max(concurrency * 2, 8),
  });

  let stopped = false;
  const inFlight = new Set<Promise<void>>();

  const pump = (async () => {
    for await (const msg of messages) {
      if (stopped) break;
      const task = handleMessage(msg, tracer, opts).finally(() => inFlight.delete(task));
      inFlight.add(task);
      if (inFlight.size >= concurrency) {
        await Promise.race(inFlight);
      }
    }
    await Promise.allSettled([...inFlight]);
  })().catch((err) => {
    if (!stopped) opts.logger.error({ err: errText(err) }, 'consumer loop stopped');
  });

  return {
    stop: async () => {
      stopped = true;
      await messages.close();
      await pump;
    },
  };
}

async function handleMessage(msg: JsMsg, tracer: Tracer, opts: ConsumeOptions): Promise<void> {
  const parent = propagation.extract(context.active(), msg.headers, HEADER_GETTER);
  await tracer.startActiveSpan(
    `${msg.subject} process`,
    {
      kind: SpanKind.CONSUMER,
      attributes: {
        'messaging.system': 'nats',
        'messaging.operation.name': 'process',
        'messaging.operation.type': 'process',
        'messaging.destination.name': msg.subject,
        'messaging.consumer.group.name': opts.spec.durable,
        'messaging.message.delivery_count': msg.info.deliveryCount,
      },
    },
    parent,
    async (span) => {
      try {
        const envelope = msg.json<EventEnvelope<unknown>>();
        span.setAttribute('messaging.message.id', envelope.id);
        await opts.handler({ msg, subject: msg.subject, envelope });
        msg.ack();
        messagesConsumed.inc({ subject: msg.subject, result: 'ack' });
      } catch (err) {
        const injected = err instanceof MalaiseNak;
        msg.nak(opts.nakDelayMs ?? (injected ? 1_000 : 2_000));
        messagesConsumed.inc({ subject: msg.subject, result: injected ? 'malaise_nak' : 'nak' });
        span.setStatus({ code: SpanStatusCode.ERROR, message: errText(err) });
        opts.logger[injected ? 'warn' : 'error'](
          { subject: msg.subject, err: errText(err), deliveries: msg.info.deliveryCount },
          injected ? 'message naked by injected malaise' : 'message handler failed',
        );
      } finally {
        span.end();
      }
    },
  );
}

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/** A readiness check for the NATS connection (design section 7: own hard dependencies). */
export function natsReadiness(nc: NatsConnection): { name: string; check: () => void } {
  return {
    name: 'nats',
    check: () => {
      if (nc.isClosed()) throw new Error('NATS connection closed');
    },
  };
}
