/**
 * Malaise (fault injection) middleware (design section 7, "Malaise").
 *
 * Polls `GET ${MALAISE_ENGINE_URL}/v1/directives?service=${OTEL_SERVICE_NAME}` every 2 s,
 * untraced, and applies whatever it finds to inbound HTTP requests and to message
 * handlers. If the engine goes away, the last directives keep being applied until each
 * one's `expiresAt`, which is what makes an engine restart mid-demo harmless.
 */

import { Worker } from 'node:worker_threads';
import type { Logger } from 'pino';
import { envRaw } from './config.js';
import { HttpError } from './errors.js';
import { malaiseApplied } from './metrics.js';
import { withSuppressedTracing } from './telemetry.js';

export const MALAISE_KINDS = ['latency', 'error', 'congestion', 'cpu-burn'] as const;
export type MalaiseKind = (typeof MALAISE_KINDS)[number];

export interface LatencyParams {
  ms: number;
  jitterMs?: number;
  percent?: number;
}
export interface ErrorParams {
  status?: number;
  percent?: number;
}
export interface CongestionParams {
  maxConcurrent: number;
  queueTimeoutMs?: number;
}
export interface CpuBurnParams {
  millicores: number;
}

export interface Directive {
  id: string;
  kind: MalaiseKind | string;
  params: Record<string, unknown>;
  expiresAt: string;
}

export interface DirectivesResponse {
  directives: Directive[];
}

export const MALAISE_POLL_INTERVAL_MS = 2_000;
const DEFAULT_QUEUE_TIMEOUT_MS = 2_000;

/** Thrown by the message-handler wrapper when a directive says "nak this message". */
export class MalaiseNak extends Error {
  readonly directiveId: string;
  constructor(directiveId: string, message = 'malaise_injected') {
    super(message);
    this.name = 'MalaiseNak';
    this.directiveId = directiveId;
  }
}

export interface MalaiseControllerOptions {
  serviceName: string;
  /** Defaults to `MALAISE_ENGINE_URL`. Unset disables the middleware entirely. */
  engineUrl?: string | undefined;
  pollIntervalMs?: number;
  timeoutMs?: number;
  logger?: Logger;
  /** Deterministic randomness for tests. */
  random?: () => number;
  sleep?: (ms: number) => Promise<void>;
}

const realSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

function num(params: Record<string, unknown>, key: string, fallback: number): number {
  const raw = params[key];
  if (typeof raw === 'number' && Number.isFinite(raw)) return raw;
  if (typeof raw === 'string') {
    const parsed = Number(raw);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

/**
 * A permit-based semaphore with a bounded wait. `maxConcurrent` can change under us when
 * the engine updates the directive, so the limit is read on each acquire.
 */
class CongestionGate {
  private inFlight = 0;
  private readonly waiters: Array<{ resolve: () => void; timer: NodeJS.Timeout }> = [];

  async acquire(maxConcurrent: number, queueTimeoutMs: number): Promise<() => void> {
    if (this.inFlight < maxConcurrent) {
      this.inFlight++;
      return () => this.release(maxConcurrent);
    }
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => {
        const index = this.waiters.findIndex((w) => w.timer === timer);
        if (index >= 0) this.waiters.splice(index, 1);
        reject(new HttpError(503, 'congested', 'The encabulator is congested; try again shortly.'));
      }, queueTimeoutMs);
      timer.unref?.();
      this.waiters.push({
        resolve: () => {
          clearTimeout(timer);
          resolve();
        },
        timer,
      });
    });
    this.inFlight++;
    return () => this.release(maxConcurrent);
  }

  private release(maxConcurrent: number): void {
    this.inFlight = Math.max(0, this.inFlight - 1);
    while (this.inFlight < maxConcurrent) {
      const next = this.waiters.shift();
      if (!next) break;
      next.resolve();
      return;
    }
  }

  get depth(): number {
    return this.waiters.length;
  }
}

/**
 * Burns CPU in worker threads so the event loop stays responsive enough for probes to
 * answer while the pod's CPU usage climbs. One worker per whole core requested, each
 * running a duty cycle for the fractional remainder.
 */
const BURN_WORKER_SOURCE = `
const { workerData, parentPort } = require('node:worker_threads');
const duty = Math.max(0, Math.min(1, workerData.duty));
const sliceMs = 20;
let stop = false;
parentPort?.on('message', (m) => { if (m === 'stop') stop = true; });
function burn() {
  if (stop) return;
  const busyUntil = Date.now() + sliceMs * duty;
  // Deliberate spin: this is the point of the fault.
  while (Date.now() < busyUntil) { Math.sqrt(Math.random() * 1e9); }
  setTimeout(burn, Math.max(1, sliceMs * (1 - duty)));
}
burn();
`;

class CpuBurner {
  private workers: Worker[] = [];
  private millicores = 0;

  set(millicores: number, logger?: Logger): void {
    const target = Math.max(0, Math.round(millicores));
    if (target === this.millicores) return;
    this.stop();
    this.millicores = target;
    if (target === 0) return;

    const whole = Math.floor(target / 1000);
    const remainder = (target % 1000) / 1000;
    const duties = [...Array.from({ length: whole }, () => 1)];
    if (remainder > 0) duties.push(remainder);

    for (const duty of duties) {
      try {
        const worker = new Worker(BURN_WORKER_SOURCE, { eval: true, workerData: { duty } });
        worker.unref();
        worker.on('error', (err) => logger?.warn({ err: err.message }, 'cpu-burn worker failed'));
        this.workers.push(worker);
      } catch (err) {
        logger?.warn({ err: String(err) }, 'could not start cpu-burn worker');
      }
    }
  }

  stop(): void {
    for (const worker of this.workers) {
      worker.postMessage('stop');
      void worker.terminate();
    }
    this.workers = [];
    this.millicores = 0;
  }

  get active(): number {
    return this.millicores;
  }
}

export class MalaiseController {
  readonly serviceName: string;
  private readonly engineUrl: string | undefined;
  private readonly pollIntervalMs: number;
  private readonly timeoutMs: number;
  private readonly logger: Logger | undefined;
  private readonly random: () => number;
  private readonly sleep: (ms: number) => Promise<void>;

  private active: Directive[] = [];
  private timer: NodeJS.Timeout | undefined;
  private readonly gate = new CongestionGate();
  private readonly burner = new CpuBurner();

  constructor(opts: MalaiseControllerOptions) {
    this.serviceName = opts.serviceName;
    this.engineUrl = (opts.engineUrl ?? envRaw('MALAISE_ENGINE_URL'))?.replace(/\/+$/, '');
    this.pollIntervalMs = opts.pollIntervalMs ?? MALAISE_POLL_INTERVAL_MS;
    this.timeoutMs = opts.timeoutMs ?? 1_500;
    this.logger = opts.logger;
    this.random = opts.random ?? Math.random;
    this.sleep = opts.sleep ?? realSleep;
  }

  get enabled(): boolean {
    return this.engineUrl !== undefined;
  }

  async start(): Promise<void> {
    if (!this.engineUrl) {
      this.logger?.info('malaise engine URL unset; fault injection middleware disabled');
      return;
    }
    await this.poll();
    this.timer = setInterval(() => {
      void this.poll();
    }, this.pollIntervalMs);
    this.timer.unref?.();
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = undefined;
    this.burner.stop();
  }

  async poll(): Promise<void> {
    if (!this.engineUrl) return;
    const url = `${this.engineUrl}/v1/directives?service=${encodeURIComponent(this.serviceName)}`;
    try {
      const response = await withSuppressedTracing(() =>
        fetch(url, {
          headers: { accept: 'application/json' },
          signal: AbortSignal.timeout(this.timeoutMs),
        }),
      );
      if (!response.ok) {
        this.logger?.warn({ status: response.status }, 'malaise directive poll failed');
        this.reconcile();
        return;
      }
      const payload = (await response.json()) as DirectivesResponse;
      this.active = Array.isArray(payload.directives) ? payload.directives : [];
    } catch (err) {
      // Keep applying what we have until each directive expires.
      this.logger?.debug(
        { err: err instanceof Error ? err.message : String(err) },
        'malaise engine unreachable; keeping last directives',
      );
    }
    this.reconcile();
  }

  /** Directives that have not expired, newest configuration wins per kind. */
  directives(now: number = Date.now()): Directive[] {
    return this.active.filter((d) => {
      const expires = Date.parse(d.expiresAt);
      return Number.isNaN(expires) ? true : expires > now;
    });
  }

  private find(kind: MalaiseKind, now = Date.now()): Directive | undefined {
    return this.directives(now).find((d) => d.kind === kind);
  }

  /** Apply expiry to side-effectful directives (right now, just cpu-burn). */
  private reconcile(): void {
    const burn = this.find('cpu-burn');
    const millicores = burn ? num(burn.params, 'millicores', 0) : 0;
    if (millicores !== this.burner.active) {
      if (millicores > 0) {
        malaiseApplied.inc({ kind: 'cpu-burn' });
        this.logger?.warn({ millicores }, 'burning CPU on request; the grammeters strain');
      }
      this.burner.set(millicores, this.logger);
    }
  }

  private hits(percent: number): boolean {
    if (percent >= 100) return true;
    if (percent <= 0) return false;
    return this.random() * 100 < percent;
  }

  /**
   * Latency, then error, then congestion. Returns a release function for the congestion
   * permit (a no-op when no congestion directive is active). Throws `HttpError` for the
   * `error` and `congestion` kinds.
   */
  async applyInbound(): Promise<() => void> {
    if (!this.enabled) return () => {};
    const now = Date.now();

    const latency = this.find('latency', now);
    if (latency) {
      const percent = num(latency.params, 'percent', 100);
      if (this.hits(percent)) {
        const ms = num(latency.params, 'ms', 0);
        const jitter = num(latency.params, 'jitterMs', 0);
        const delay = Math.max(0, ms + (this.random() * 2 - 1) * jitter);
        malaiseApplied.inc({ kind: 'latency' });
        await this.sleep(delay);
      }
    }

    const error = this.find('error', now);
    if (error) {
      const percent = num(error.params, 'percent', 100);
      if (this.hits(percent)) {
        malaiseApplied.inc({ kind: 'error' });
        const status = num(error.params, 'status', 503);
        throw new HttpError(status, 'malaise_injected', 'Injected malaise, as requested.');
      }
    }

    const congestion = this.find('congestion', now);
    if (congestion) {
      const maxConcurrent = Math.max(1, num(congestion.params, 'maxConcurrent', 1));
      const queueTimeoutMs = Math.max(
        1,
        num(congestion.params, 'queueTimeoutMs', DEFAULT_QUEUE_TIMEOUT_MS),
      );
      malaiseApplied.inc({ kind: 'congestion' });
      return this.gate.acquire(maxConcurrent, queueTimeoutMs);
    }

    return () => {};
  }

  /**
   * Wrap a message handler. Latency slows it down; `error` and a congestion timeout make
   * it throw `MalaiseNak`, which the JetStream consume helper turns into `msg.nak()`.
   */
  wrapMessageHandler<A extends unknown[], R>(
    handler: (...args: A) => Promise<R>,
  ): (...args: A) => Promise<R> {
    return async (...args: A): Promise<R> => {
      let release: () => void = () => {};
      try {
        release = await this.applyInbound();
      } catch (err) {
        if (err instanceof HttpError) {
          throw new MalaiseNak(err.code, err.message);
        }
        throw err;
      }
      try {
        return await handler(...args);
      } finally {
        release();
      }
    };
  }

  /** For `/v1/line`-style introspection endpoints and tests. */
  snapshot(): {
    enabled: boolean;
    directives: Directive[];
    queueDepth: number;
    cpuBurnMillicores: number;
  } {
    return {
      enabled: this.enabled,
      directives: this.directives(),
      queueDepth: this.gate.depth,
      cpuBurnMillicores: this.burner.active,
    };
  }

  /** Test seam: install directives without an engine. */
  setForTesting(directives: Directive[]): void {
    this.active = directives;
    this.reconcile();
  }
}
