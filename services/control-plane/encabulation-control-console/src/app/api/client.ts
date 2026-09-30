/**
 * The SPA's only door to the network.
 *
 * Every path is relative. The SPA is served by the BFF from the same origin, so
 * `/bff/...` always resolves correctly whether the console is reached by
 * port-forward, through an ingress, or from a sub-path. No host, port, or scheme
 * is baked into the bundle; `honesty.test.ts` asserts that.
 */

export const BFF = '/bff'

export interface ApiError {
  code: string
  message: string
  status: number
}

export class ApiFailure extends Error {
  readonly code: string
  readonly status: number

  constructor(error: ApiError) {
    super(error.message)
    this.name = 'ApiFailure'
    this.code = error.code
    this.status = error.status
  }
}

async function parse(res: Response): Promise<unknown> {
  const text = await res.text()
  if (text === '') return null
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function errorFrom(status: number, body: unknown): ApiError {
  const shaped = body as { error?: { code?: unknown; message?: unknown } } | null
  const code = typeof shaped?.error?.code === 'string' ? shaped.error.code : `http_${status}`
  const message =
    typeof shaped?.error?.message === 'string'
      ? shaped.error.message
      : `The console's backend answered ${status}.`
  return { code, message, status }
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
  query?: Record<string, string | number | boolean | undefined>
}

export function withQuery(path: string, query: RequestOptions['query']): string {
  if (query === undefined) return path
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined) continue
    params.set(key, String(value))
  }
  const qs = params.toString()
  return qs === '' ? path : `${path}?${qs}`
}

export async function call<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const url = withQuery(`${BFF}${path}`, options.query)
  const init: RequestInit = {
    method: options.method ?? 'GET',
    headers: { accept: 'application/json' },
    // Same-origin: the session cookie rides along, and nothing else does.
    credentials: 'same-origin',
    ...(options.signal === undefined ? {} : { signal: options.signal }),
  }
  if (options.body !== undefined) {
    init.headers = { ...init.headers, 'content-type': 'application/json' }
    init.body = JSON.stringify(options.body)
  }

  let res: Response
  try {
    res = await fetch(url, init)
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiFailure({
      code: 'console_unreachable',
      message: 'The console’s own backend did not answer. It may be restarting.',
      status: 0,
    })
  }

  const body = await parse(res)
  if (!res.ok) throw new ApiFailure(errorFrom(res.status, body))
  return body as T
}

export const api = {
  get: <T>(path: string, options: Omit<RequestOptions, 'method' | 'body'> = {}) =>
    call<T>(path, { ...options, method: 'GET' }),
  post: <T>(path: string, body?: unknown, options: Omit<RequestOptions, 'method'> = {}) =>
    call<T>(path, { ...options, method: 'POST', ...(body === undefined ? {} : { body }) }),
  put: <T>(path: string, body?: unknown, options: Omit<RequestOptions, 'method'> = {}) =>
    call<T>(path, { ...options, method: 'PUT', ...(body === undefined ? {} : { body }) }),
  patch: <T>(path: string, body?: unknown, options: Omit<RequestOptions, 'method'> = {}) =>
    call<T>(path, { ...options, method: 'PATCH', ...(body === undefined ? {} : { body }) }),
  delete: <T>(path: string, options: Omit<RequestOptions, 'method' | 'body'> = {}) =>
    call<T>(path, { ...options, method: 'DELETE' }),
}

/** The stream URL, relative for the same reason everything else is. */
export const STREAM_URL = `${BFF}/stream`
