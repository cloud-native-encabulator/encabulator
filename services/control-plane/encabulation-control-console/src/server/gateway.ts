/**
 * The upstream gateway: one `@encabulator/kit` `HttpClient` per configured
 * upstream, behind a narrow interface.
 *
 * Two things the console needs that a plain kit client does not give it:
 *
 *  1. **Statuses pass through.** The kit client throws on a non-2xx, which is
 *     right for a service calling a dependency and wrong for a BFF proxying one:
 *     a 409 `insufficient_parts` from the depot must arrive in the browser as a
 *     409 `insufficient_parts`. `call` therefore returns the upstream's status
 *     and body whatever they are, and only throws when the request never got an
 *     answer at all.
 *  2. **Unconfigured is a first-class state.** An upstream whose environment
 *     variable is unset is not "down"; it is unconfigured, and the console says
 *     which variable to set rather than inventing a localhost default.
 */

import { HttpClient, HttpClientError, type Logger } from './platform/index.js'
import type { ConsoleConfig, UpstreamKey } from './config.js'

/** Prometheus is addressed like an upstream but is not in the service catalog. */
export type CallTarget = UpstreamKey | 'prometheus'

export interface UpstreamCall {
  target: CallTarget
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  /** Path and query relative to the upstream base URL. */
  path: string
  body?: unknown
  headers?: Record<string, string>
  requestId?: string
  timeoutMs?: number
  /**
   * Retries for this call. The polling loops pass 0: a slow upstream must not
   * turn one tick into three in-flight requests.
   */
  retries?: number
  idempotencyKey?: string
}

export interface UpstreamResult {
  status: number
  body: unknown
  contentType: string | null
}

export class UpstreamDown extends Error {
  readonly target: CallTarget
  readonly service: string
  readonly code: string

  constructor(opts: { target: CallTarget; service: string; code: string; message: string }) {
    super(opts.message)
    this.name = 'UpstreamDown'
    this.target = opts.target
    this.service = opts.service
    this.code = opts.code
  }
}

export class UpstreamNotConfigured extends Error {
  readonly target: CallTarget
  readonly service: string
  readonly env: string

  constructor(opts: { target: CallTarget; service: string; env: string }) {
    super(`${opts.service} has no address: set ${opts.env}.`)
    this.name = 'UpstreamNotConfigured'
    this.target = opts.target
    this.service = opts.service
    this.env = opts.env
  }
}

export interface UpstreamInfo {
  service: string
  env: string
  configured: boolean
}

export interface Gateway {
  info(target: CallTarget): UpstreamInfo
  /**
   * Make the call. Any HTTP status, including 4xx and 5xx, comes back as a
   * result. Only a transport failure, a timeout, or an unconfigured upstream
   * throws.
   */
  call(request: UpstreamCall): Promise<UpstreamResult>
}

/** Statuses the kit client should hand back rather than throw on: all of them. */
const PASS_THROUGH: number[] = Array.from({ length: 200 }, (_, i) => 400 + i)

export function createGateway(opts: { config: ConsoleConfig; logger: Logger }): Gateway {
  const { config, logger } = opts

  const infos = new Map<CallTarget, UpstreamInfo>()
  const clients = new Map<CallTarget, HttpClient>()

  for (const [key, upstream] of Object.entries(config.upstreams) as [
    UpstreamKey,
    ConsoleConfig['upstreams'][UpstreamKey],
  ][]) {
    infos.set(key, {
      service: upstream.service,
      env: upstream.env,
      configured: upstream.url !== null,
    })
    if (upstream.url !== null) {
      clients.set(
        key,
        new HttpClient({
          name: upstream.service,
          baseUrl: upstream.url,
          timeoutMs: config.upstreamTimeoutMs,
          logger,
        }),
      )
    }
  }

  infos.set('prometheus', {
    service: 'prometheus',
    env: 'PROMETHEUS_URL',
    configured: config.prometheusUrl !== null,
  })
  if (config.prometheusUrl !== null) {
    clients.set(
      'prometheus',
      new HttpClient({
        name: 'prometheus',
        baseUrl: config.prometheusUrl,
        timeoutMs: config.prometheusTimeoutMs,
        logger,
      }),
    )
  }

  function info(target: CallTarget): UpstreamInfo {
    return infos.get(target) ?? { service: String(target), env: 'unknown', configured: false }
  }

  return {
    info,
    async call(request: UpstreamCall): Promise<UpstreamResult> {
      const detail = info(request.target)
      const client = clients.get(request.target)
      if (client === undefined) {
        throw new UpstreamNotConfigured({
          target: request.target,
          service: detail.service,
          env: detail.env,
        })
      }

      try {
        const res = await client.request(request.method, request.path, {
          ...(request.body === undefined ? {} : { body: request.body }),
          ...(request.headers === undefined ? {} : { headers: request.headers }),
          ...(request.requestId === undefined ? {} : { requestId: request.requestId }),
          ...(request.timeoutMs === undefined ? {} : { timeoutMs: request.timeoutMs }),
          ...(request.retries === undefined ? {} : { retries: request.retries }),
          ...(request.idempotencyKey === undefined
            ? {}
            : { idempotencyKey: request.idempotencyKey }),
          expect: PASS_THROUGH,
        })
        return {
          status: res.status,
          body: res.body,
          contentType: res.headers.get('content-type'),
        }
      } catch (err) {
        if (err instanceof HttpClientError) {
          // With `expect` covering every 4xx and 5xx, reaching here means the
          // request never completed: a timeout, DNS failure, or reset.
          throw new UpstreamDown({
            target: request.target,
            service: detail.service,
            code: err.code === '' ? 'upstream_unreachable' : err.code,
            message: err.message,
          })
        }
        throw new UpstreamDown({
          target: request.target,
          service: detail.service,
          code: 'upstream_unreachable',
          message: `${detail.service} is unreachable: ${
            err instanceof Error ? err.message : String(err)
          }`,
        })
      }
    },
  }
}
