/**
 * Circuit breakers around the quoter, the depot and the datastore (design 8.2):
 * open after five consecutive failures, half-open after ten seconds. An open breaker fails
 * fast with `503 dependency_unavailable` rather than piling more load onto a sick service.
 *
 * Only *dependency* failures trip the breaker. A `409 insufficient_parts` is the depot
 * working correctly and says nothing about its health.
 */

import { gauge, type Logger, unavailable } from '@encabulator/kit';

export type BreakerState = 'closed' | 'open' | 'half-open';

export const DEFAULT_FAILURE_THRESHOLD = 5;
export const DEFAULT_HALF_OPEN_AFTER_MS = 10_000;

const breakerState = gauge({
  name: 'encabulator_circuit_breaker_state',
  help: 'Circuit breaker state per dependency: 0 closed, 1 half-open, 2 open.',
  labelNames: ['dependency'],
});

const STATE_VALUE: Record<BreakerState, number> = { closed: 0, 'half-open': 1, open: 2 };

export interface CircuitBreakerOptions {
  /** The dependency's service name; appears in the metric and the error message. */
  name: string;
  logger?: Logger;
  failureThreshold?: number;
  halfOpenAfterMs?: number;
  now?: () => number;
  /**
   * Decides whether a thrown error reflects on the dependency's health. Defaults to
   * "anything that is not an `HttpClientError` with a 4xx status".
   */
  isFailure?: (err: unknown) => boolean;
}

function defaultIsFailure(err: unknown): boolean {
  const candidate = err as { isDependencyFailure?: unknown; status?: unknown };
  if (typeof candidate?.isDependencyFailure === 'boolean') return candidate.isDependencyFailure;
  return true;
}

export class CircuitBreaker {
  readonly name: string;
  private readonly logger: Logger | undefined;
  private readonly failureThreshold: number;
  private readonly halfOpenAfterMs: number;
  private readonly now: () => number;
  private readonly isFailure: (err: unknown) => boolean;

  private consecutiveFailures = 0;
  private openedAt = 0;
  private state: BreakerState = 'closed';
  /** True while a half-open trial call is in flight, so only one probe gets through. */
  private trialInFlight = false;

  constructor(opts: CircuitBreakerOptions) {
    this.name = opts.name;
    this.logger = opts.logger;
    this.failureThreshold = opts.failureThreshold ?? DEFAULT_FAILURE_THRESHOLD;
    this.halfOpenAfterMs = opts.halfOpenAfterMs ?? DEFAULT_HALF_OPEN_AFTER_MS;
    this.now = opts.now ?? (() => Date.now());
    this.isFailure = opts.isFailure ?? defaultIsFailure;
    this.publish();
  }

  currentState(): BreakerState {
    this.maybeHalfOpen();
    return this.state;
  }

  private maybeHalfOpen(): void {
    if (this.state === 'open' && this.now() - this.openedAt >= this.halfOpenAfterMs) {
      this.transition('half-open');
    }
  }

  private transition(next: BreakerState): void {
    if (this.state === next) return;
    this.state = next;
    if (next === 'open') this.openedAt = this.now();
    if (next !== 'half-open') this.trialInFlight = false;
    this.publish();
    this.logger?.warn({ dependency: this.name, state: next }, 'circuit breaker changed state');
  }

  private publish(): void {
    breakerState.set({ dependency: this.name }, STATE_VALUE[this.state]);
  }

  /** Run `fn` through the breaker. */
  async run<T>(fn: () => Promise<T>): Promise<T> {
    this.maybeHalfOpen();

    if (this.state === 'open') {
      throw unavailable(
        'dependency_unavailable',
        `${this.name} is unavailable; the circuit is open.`,
        { dependency: this.name },
      );
    }
    if (this.state === 'half-open') {
      if (this.trialInFlight) {
        throw unavailable(
          'dependency_unavailable',
          `${this.name} is being probed; try again shortly.`,
          { dependency: this.name },
        );
      }
      this.trialInFlight = true;
    }

    try {
      const result = await fn();
      this.onSuccess();
      return result;
    } catch (err) {
      if (this.isFailure(err)) {
        this.onFailure();
      } else if (this.state === 'half-open') {
        // A business-level answer still proves the dependency is alive.
        this.onSuccess();
      }
      throw err;
    }
  }

  private onSuccess(): void {
    this.consecutiveFailures = 0;
    this.trialInFlight = false;
    this.transition('closed');
  }

  private onFailure(): void {
    this.consecutiveFailures++;
    this.trialInFlight = false;
    if (this.state === 'half-open' || this.consecutiveFailures >= this.failureThreshold) {
      // Re-open even if already open, so the half-open timer restarts.
      this.state = 'open';
      this.openedAt = this.now();
      this.publish();
      this.logger?.warn(
        { dependency: this.name, consecutiveFailures: this.consecutiveFailures },
        'circuit breaker open',
      );
    }
  }

  /** Test seam. */
  reset(): void {
    this.consecutiveFailures = 0;
    this.trialInFlight = false;
    this.transition('closed');
  }
}
