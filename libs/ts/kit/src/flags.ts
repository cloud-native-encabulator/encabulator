/**
 * Feature flag client for `differential-girdlespring` (design section 6).
 *
 * Polls `GET /v1/flags` every 5 s with `If-None-Match`, keeps the last good set, and
 * falls back to the built-in defaults when the service is unreachable at startup or
 * `DIFFERENTIAL_GIRDLESPRING_URL` is unset. The poll is untraced.
 */

import type { Logger } from 'pino';
import { envRaw } from './config.js';
import { inRollout } from './hash.js';
import { withSuppressedTracing } from './telemetry.js';

export type FlagType = 'boolean' | 'number' | 'string';
export type FlagValue = boolean | number | string;

export interface Flag {
  key: string;
  type: FlagType;
  value: FlagValue;
  rolloutPercent?: number | null;
  description?: string;
  updatedAt?: string;
  updatedBy?: string;
}

export interface FlagsResponse {
  flags: Flag[];
}

/** Design section 6. Every client ships these so a girdlespring outage is survivable. */
export const FLAG_DEFAULTS: Readonly<Record<string, FlagValue>> = Object.freeze({
  'turbo-encabulation': false,
  'quote-cache': true,
  'retro-encabulator-sales': true,
  'strict-side-fumbling': false,
  'amulite-foundry-rate': 20,
  'gateway-rate-limit-rps': 50,
  'dingle-arm-damping': 50,
});

export const FLAG_POLL_INTERVAL_MS = 5_000;

export interface FlagClientOptions {
  /** Defaults to `DIFFERENTIAL_GIRDLESPRING_URL`. Unset means "always use defaults". */
  baseUrl?: string | undefined;
  pollIntervalMs?: number;
  timeoutMs?: number;
  logger?: Logger;
  /** Overrides for the built-in defaults, merged on top of `FLAG_DEFAULTS`. */
  defaults?: Record<string, FlagValue>;
}

export class FlagClient {
  private readonly baseUrl: string | undefined;
  private readonly pollIntervalMs: number;
  private readonly timeoutMs: number;
  private readonly logger: Logger | undefined;
  private readonly defaults: Record<string, FlagValue>;

  private flags = new Map<string, Flag>();
  private etag: string | undefined;
  private timer: NodeJS.Timeout | undefined;
  private everLoaded = false;

  constructor(opts: FlagClientOptions = {}) {
    this.baseUrl = (opts.baseUrl ?? envRaw('DIFFERENTIAL_GIRDLESPRING_URL'))?.replace(/\/+$/, '');
    this.pollIntervalMs = opts.pollIntervalMs ?? FLAG_POLL_INTERVAL_MS;
    this.timeoutMs = opts.timeoutMs ?? 2_000;
    this.logger = opts.logger;
    this.defaults = { ...FLAG_DEFAULTS, ...(opts.defaults ?? {}) };
  }

  get enabled(): boolean {
    return this.baseUrl !== undefined;
  }

  /** True once at least one poll has returned a flag set. */
  get loaded(): boolean {
    return this.everLoaded;
  }

  /** Fetch once, then poll. Never throws: a missing girdlespring means defaults. */
  async start(): Promise<void> {
    if (!this.baseUrl) {
      this.logger?.info('girdlespring URL unset; using built-in flag defaults');
      return;
    }
    await this.refresh();
    this.timer = setInterval(() => {
      void this.refresh();
    }, this.pollIntervalMs);
    this.timer.unref?.();
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = undefined;
  }

  async refresh(): Promise<void> {
    if (!this.baseUrl) return;
    try {
      const headers: Record<string, string> = { accept: 'application/json' };
      if (this.etag) headers['if-none-match'] = this.etag;
      const response = await withSuppressedTracing(() =>
        fetch(`${this.baseUrl}/v1/flags`, {
          headers,
          signal: AbortSignal.timeout(this.timeoutMs),
        }),
      );
      if (response.status === 304) return;
      if (!response.ok) {
        this.logger?.warn({ status: response.status }, 'girdlespring poll failed');
        return;
      }
      const payload = (await response.json()) as FlagsResponse;
      const next = new Map<string, Flag>();
      for (const flag of payload.flags ?? []) next.set(flag.key, flag);
      this.flags = next;
      this.etag = response.headers.get('etag') ?? undefined;
      this.everLoaded = true;
    } catch (err) {
      // Keep the last good set. This is the documented behaviour, not an oversight.
      this.logger?.warn({ err: errMessage(err) }, 'girdlespring unreachable; keeping last flags');
    }
  }

  all(): Flag[] {
    return [...this.flags.values()];
  }

  raw(key: string): Flag | undefined {
    return this.flags.get(key);
  }

  /**
   * Boolean flag evaluation. When the flag carries a `rolloutPercent` and a subject is
   * supplied, `fnv1a32(key + ":" + subject) % 100 < rolloutPercent` decides.
   */
  boolean(key: string, subject?: string): boolean {
    const flag = this.flags.get(key);
    if (!flag) return Boolean(this.defaults[key] ?? false);
    const value = toBoolean(flag.value, Boolean(this.defaults[key] ?? false));
    const rollout = flag.rolloutPercent;
    if (rollout === undefined || rollout === null) return value;
    if (!subject) return value;
    // The rollout gates the configured value; outside the rollout you get the opposite.
    return inRollout(key, subject, rollout) ? value : !value;
  }

  number(key: string): number {
    const flag = this.flags.get(key);
    const fallback = Number(this.defaults[key] ?? 0);
    if (!flag) return fallback;
    const value = typeof flag.value === 'number' ? flag.value : Number(flag.value);
    return Number.isFinite(value) ? value : fallback;
  }

  string(key: string): string {
    const flag = this.flags.get(key);
    if (!flag) return String(this.defaults[key] ?? '');
    return String(flag.value);
  }

  /** Test seam: install a flag set without talking to girdlespring. */
  setForTesting(flags: Flag[]): void {
    this.flags = new Map(flags.map((f) => [f.key, f]));
    this.everLoaded = true;
  }
}

function toBoolean(value: FlagValue, fallback: boolean): boolean {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (value === 'true') return true;
  if (value === 'false') return false;
  return fallback;
}

function errMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
