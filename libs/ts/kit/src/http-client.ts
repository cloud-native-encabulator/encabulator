/**
 * Outbound HTTP (design section 7, "HTTP").
 *
 * - default timeout 3s
 * - idempotent requests (GET, PUT, DELETE) retry twice with exponential backoff,
 *   100 ms base, full jitter
 * - POST retries only when the caller supplies an idempotency key
 * - every attempt records `http_client_request_duration_seconds` with `server_address`
 *   set to the target *service* name, not the host:port
 *
 * Built on global `fetch` (undici under the hood) so
 * `@opentelemetry/instrumentation-undici` produces the client spans for free.
 */

import type { Logger } from 'pino';
import { REQUEST_ID_HEADER } from './identity.js';
import { httpClientDuration } from './metrics.js';

export const DEFAULT_TIMEOUT_MS = 3_000;
export const DEFAULT_RETRIES = 2;
export const RETRY_BASE_MS = 100;

const IDEMPOTENT_METHODS = new Set(['GET', 'HEAD', 'PUT', 'DELETE', 'OPTIONS']);
const RETRYABLE_STATUS = new Set([429, 502, 503, 504]);

export interface HttpClientOptions {
  /** Target service name, used as the `server_address` metric label. */
  name: string;
  baseUrl: string;
  timeoutMs?: number;
  retries?: number;
  logger?: Logger;
  /** Headers added to every request (identity headers, for instance). */
  defaultHeaders?: Record<string, string>;
}

export interface RequestOptions {
  headers?: Record<string, string | undefined>;
  query?: Record<string, string | number | boolean | undefined>;
  body?: unknown;
  timeoutMs?: number;
  /** Overrides the default retry count for this call. */
  retries?: number;
  /**
   * Makes a POST retryable, and is sent as `Idempotency-Key`.
   */
  idempotencyKey?: string;
  requestId?: string | undefined;
  /** Status codes that should be returned rather than thrown. */
  expect?: number[];
  signal?: AbortSignal;
}

export interface HttpResponse<T> {
  status: number;
  body: T;
  headers: Headers;
}

export class HttpClientError extends Error {
  readonly service: string;
  readonly status: number;
  readonly code: string;
  readonly body: unknown;

  constructor(service: string, status: number, code: string, message: string, body: unknown) {
    super(message);
    this.name = 'HttpClientError';
    this.service = service;
    this.status = status;
    this.code = code;
    this.body = body;
  }

  /** True for the failures a circuit breaker should count. */
  get isDependencyFailure(): boolean {
    return this.status === 0 || this.status >= 500;
  }
}

function fullJitterBackoff(attempt: number): number {
  return Math.random() * RETRY_BASE_MS * 2 ** attempt;
}

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

function extractErrorCode(body: unknown, status: number): string {
  if (body && typeof body === 'object' && 'error' in body) {
    const err = (body as { error?: { code?: unknown } }).error;
    if (err && typeof err === 'object' && typeof err.code === 'string') return err.code;
  }
  return `http_${status}`;
}

function extractErrorMessage(body: unknown, fallback: string): string {
  if (body && typeof body === 'object' && 'error' in body) {
    const err = (body as { error?: { message?: unknown } }).error;
    if (err && typeof err === 'object' && typeof err.message === 'string') return err.message;
  }
  return fallback;
}

export class HttpClient {
  readonly name: string;
  readonly baseUrl: string;
  private readonly timeoutMs: number;
  private readonly retries: number;
  private readonly logger: Logger | undefined;
  private readonly defaultHeaders: Record<string, string>;

  constructor(opts: HttpClientOptions) {
    this.name = opts.name;
    this.baseUrl = opts.baseUrl.replace(/\/+$/, '');
    this.timeoutMs = opts.timeoutMs ?? DEFAULT_TIMEOUT_MS;
    this.retries = opts.retries ?? DEFAULT_RETRIES;
    this.logger = opts.logger;
    this.defaultHeaders = opts.defaultHeaders ?? {};
  }

  /** A clone of this client that forwards the given headers (identity, request id). */
  withHeaders(headers: Record<string, string>): HttpClient {
    return new HttpClient({
      name: this.name,
      baseUrl: this.baseUrl,
      timeoutMs: this.timeoutMs,
      retries: this.retries,
      ...(this.logger ? { logger: this.logger } : {}),
      defaultHeaders: { ...this.defaultHeaders, ...headers },
    });
  }

  get<T>(path: string, opts: RequestOptions = {}): Promise<HttpResponse<T>> {
    return this.request<T>('GET', path, opts);
  }

  post<T>(path: string, body: unknown, opts: RequestOptions = {}): Promise<HttpResponse<T>> {
    return this.request<T>('POST', path, { ...opts, body });
  }

  put<T>(path: string, body: unknown, opts: RequestOptions = {}): Promise<HttpResponse<T>> {
    return this.request<T>('PUT', path, { ...opts, body });
  }

  delete<T>(path: string, opts: RequestOptions = {}): Promise<HttpResponse<T>> {
    return this.request<T>('DELETE', path, opts);
  }

  private buildUrl(path: string, query: RequestOptions['query']): string {
    const url = new URL(`${this.baseUrl}${path.startsWith('/') ? path : `/${path}`}`);
    for (const [key, value] of Object.entries(query ?? {})) {
      if (value !== undefined) url.searchParams.set(key, String(value));
    }
    return url.toString();
  }

  private maxAttempts(method: string, opts: RequestOptions): number {
    const retries = opts.retries ?? this.retries;
    if (IDEMPOTENT_METHODS.has(method)) return retries + 1;
    if (opts.idempotencyKey) return retries + 1;
    return 1;
  }

  async request<T>(
    method: string,
    path: string,
    opts: RequestOptions = {},
  ): Promise<HttpResponse<T>> {
    const url = this.buildUrl(path, opts.query);
    const attempts = this.maxAttempts(method, opts);
    const timeoutMs = opts.timeoutMs ?? this.timeoutMs;
    const expected = new Set(opts.expect ?? []);

    let lastError: HttpClientError | undefined;

    for (let attempt = 0; attempt < attempts; attempt++) {
      const started = process.hrtime.bigint();
      let statusLabel = '0';
      try {
        const response = await this.fetchOnce(method, url, opts, timeoutMs);
        statusLabel = String(response.status);
        const body = await parseBody<T>(response);

        if (response.status >= 400 && !expected.has(response.status)) {
          const err = new HttpClientError(
            this.name,
            response.status,
            extractErrorCode(body, response.status),
            extractErrorMessage(body, `${this.name} responded ${response.status}`),
            body,
          );
          if (RETRYABLE_STATUS.has(response.status) && attempt < attempts - 1) {
            lastError = err;
            await sleep(fullJitterBackoff(attempt));
            continue;
          }
          throw err;
        }

        return { status: response.status, body, headers: response.headers };
      } catch (cause) {
        if (cause instanceof HttpClientError) throw cause;
        const err = new HttpClientError(
          this.name,
          0,
          isTimeout(cause) ? 'dependency_timeout' : 'dependency_unreachable',
          `${method} ${this.name}${path} failed: ${errText(cause)}`,
          undefined,
        );
        lastError = err;
        if (attempt < attempts - 1) {
          await sleep(fullJitterBackoff(attempt));
          continue;
        }
        throw err;
      } finally {
        httpClientDuration.observe(
          {
            http_request_method: method,
            server_address: this.name,
            http_response_status_code: statusLabel,
          },
          Number(process.hrtime.bigint() - started) / 1e9,
        );
      }
    }

    // Unreachable in practice: the loop either returns or throws.
    throw (
      lastError ??
      new HttpClientError(this.name, 0, 'dependency_unreachable', 'no attempts', undefined)
    );
  }

  private async fetchOnce(
    method: string,
    url: string,
    opts: RequestOptions,
    timeoutMs: number,
  ): Promise<Response> {
    const headers = new Headers();
    for (const [key, value] of Object.entries(this.defaultHeaders)) headers.set(key, value);
    for (const [key, value] of Object.entries(opts.headers ?? {})) {
      if (value !== undefined) headers.set(key, value);
    }
    if (opts.requestId) headers.set(REQUEST_ID_HEADER, opts.requestId);
    if (opts.idempotencyKey) headers.set('idempotency-key', opts.idempotencyKey);
    headers.set('accept', 'application/json');

    const init: RequestInit = { method, headers, signal: timeoutSignal(timeoutMs, opts.signal) };
    if (opts.body !== undefined) {
      headers.set('content-type', 'application/json');
      init.body = JSON.stringify(opts.body);
    }

    this.logger?.debug({ method, url, target: this.name }, 'outbound request');
    return fetch(url, init);
  }
}

function timeoutSignal(timeoutMs: number, extra: AbortSignal | undefined): AbortSignal {
  const timeout = AbortSignal.timeout(timeoutMs);
  return extra ? AbortSignal.any([timeout, extra]) : timeout;
}

async function parseBody<T>(response: Response): Promise<T> {
  const text = await response.text();
  if (text.length === 0) return undefined as T;
  const contentType = response.headers.get('content-type') ?? '';
  if (!contentType.includes('json')) return text as unknown as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    return text as unknown as T;
  }
}

function isTimeout(cause: unknown): boolean {
  return cause instanceof Error && (cause.name === 'TimeoutError' || cause.name === 'AbortError');
}

function errText(cause: unknown): string {
  if (cause instanceof Error) {
    const inner = (cause as { cause?: unknown }).cause;
    if (inner instanceof Error) return `${cause.message} (${inner.message})`;
    return cause.message;
  }
  return String(cause);
}
