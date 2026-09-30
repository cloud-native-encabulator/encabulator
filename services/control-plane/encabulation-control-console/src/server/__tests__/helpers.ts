/**
 * Test harness for the BFF. Everything the console talks to is faked here, so
 * the whole suite runs without Kubernetes, Prometheus, or a container runtime.
 */

import { buildApp, type BuiltApp } from '../app.js'
import { loadConfig, type ConsoleConfig } from '../config.js'
import {
  UpstreamDown,
  UpstreamNotConfigured,
  type CallTarget,
  type Gateway,
  type UpstreamCall,
  type UpstreamResult,
} from '../gateway.js'
import { consoleMetrics, createLogger } from '../platform/index.js'
import type { ClusterReader, ClusterSnapshot } from '../topology/kubernetes.js'
import type { PrometheusReader, PromQueryResult } from '../topology/prometheus.js'
import type { Role } from '../session.js'

export const SILENT_LOGGER = createLogger({
  service: 'encabulation-control-console',
  level: 'silent',
})

export const TEST_ENV: Record<string, string> = {
  SESSION_SECRET: 'a-test-secret-at-least-sixteen-characters',
  POD_NAMESPACE: 'encabulator',
  PROMETHEUS_URL: 'http://prometheus.test:9090',
  GRAFANA_EXTERNAL_URL: 'https://grafana.example',
  JAEGER_EXTERNAL_URL: 'https://jaeger.example',
  UNILATERAL_PHASE_DETRACTOR_URL: 'http://detractor.test:8080',
  DINGLE_ARM_GATEWAY_URL: 'http://gateway.test:8080',
  CARDINAL_GRAMMETER_URL: 'http://grammeter.test:8080',
  MALAISE_ENGINE_URL: 'http://malaise.test:8080',
  DEMO_SCENARIO_ORCHESTRATOR_URL: 'http://orchestrator.test:8080',
  DIFFERENTIAL_GIRDLESPRING_URL: 'http://girdlespring.test:8080',
  PREFABULATED_AMULITE_DEPOT_URL: 'http://depot.test:8080',
  HYDROCOPTIC_DATASTORE_URL: 'http://datastore.test:8080',
  SIDE_FUMBLING_DETECTOR_URL: 'http://detector.test:8080',
  NOFER_TRUNNION_DISPATCH_URL: 'http://dispatch.test:8080',
  SINUSOIDAL_RECORDER_URL: 'http://recorder.test:8080',
}

export function testConfig(overrides: Record<string, string | undefined> = {}): ConsoleConfig {
  const env: Record<string, string> = { ...TEST_ENV }
  for (const [key, value] of Object.entries(overrides)) {
    if (value === undefined) delete env[key]
    else env[key] = value
  }
  return loadConfig(env as NodeJS.ProcessEnv)
}

export interface RecordedCall {
  target: CallTarget
  method: string
  path: string
  headers: Record<string, string>
  body: unknown
  retries: number | undefined
}

export interface FakeGateway extends Gateway {
  calls: RecordedCall[]
  /** Answer any call whose path contains `fragment`. */
  on(fragment: string, response: Partial<UpstreamResult> | (() => Partial<UpstreamResult>)): void
  /** Make any call whose path or target contains `fragment` fail in transport. */
  fail(fragment: string, message: string): void
}

export function fakeGateway(config: ConsoleConfig = testConfig()): FakeGateway {
  const calls: RecordedCall[] = []
  const handlers: {
    fragment: string
    respond?: Partial<UpstreamResult> | (() => Partial<UpstreamResult>)
    failure?: string
  }[] = []

  const infos = new Map<CallTarget, { service: string; env: string; configured: boolean }>()
  for (const [key, upstream] of Object.entries(config.upstreams)) {
    infos.set(key as CallTarget, {
      service: upstream.service,
      env: upstream.env,
      configured: upstream.url !== null,
    })
  }
  infos.set('prometheus', {
    service: 'prometheus',
    env: 'PROMETHEUS_URL',
    configured: config.prometheusUrl !== null,
  })

  return {
    calls,
    info(target) {
      return infos.get(target) ?? { service: String(target), env: 'unknown', configured: false }
    },
    async call(request: UpstreamCall): Promise<UpstreamResult> {
      calls.push({
        target: request.target,
        method: request.method,
        path: request.path,
        headers: request.headers ?? {},
        body: request.body ?? null,
        retries: request.retries,
      })
      const info = infos.get(request.target)
      if (info === undefined || !info.configured) {
        throw new UpstreamNotConfigured({
          target: request.target,
          service: info?.service ?? String(request.target),
          env: info?.env ?? 'unknown',
        })
      }
      const key = `${String(request.target)} ${request.path}`
      const match = handlers.find((h) => key.includes(h.fragment))
      if (match?.failure !== undefined) {
        throw new UpstreamDown({
          target: request.target,
          service: info.service,
          code: 'upstream_unreachable',
          message: `${info.service} is unreachable: ${match.failure}`,
        })
      }
      const raw = typeof match?.respond === 'function' ? match.respond() : match?.respond
      return {
        status: raw?.status ?? 200,
        body: raw?.body ?? {},
        contentType: raw?.contentType ?? 'application/json',
      }
    },
    on(fragment, response) {
      handlers.unshift({ fragment, respond: response })
    },
    fail(fragment, message) {
      handlers.unshift({ fragment, failure: message })
    },
  }
}

export function emptyPromResult(detail: string | null = null): PromQueryResult {
  return detail === null
    ? { ok: true, detail: null, samples: [] }
    : { ok: false, detail, samples: [] }
}

export function fakePrometheus(
  answers: (promql: string) => PromQueryResult = () => emptyPromResult(),
): PrometheusReader {
  return { configured: true, query: async (q) => answers(q) }
}

export function fakeCluster(snapshot: ClusterSnapshot): ClusterReader {
  return { configured: snapshot.ok, read: async () => snapshot }
}

export const EMPTY_CLUSTER: ClusterSnapshot = { ok: true, detail: null, pods: [], workloads: [] }

/**
 * A detractor-shaped RS256 JWT with the given subject and roles. The signature
 * is not real, and does not need to be: the BFF never verifies it (see the
 * comment at the top of `session.ts`). The gateway does, on storefront calls.
 */
export function fakeToken(opts: {
  sub: string
  roles: Role[]
  expiresInSeconds?: number
  displayName?: string
}): string {
  const b64 = (obj: unknown): string => Buffer.from(JSON.stringify(obj)).toString('base64url')
  const header = b64({ alg: 'RS256', typ: 'JWT', kid: 'test-kid' })
  const payload = b64({
    sub: opts.sub,
    roles: opts.roles,
    iss: 'unilateral-phase-detractor',
    aud: 'encabulator',
    exp: Math.floor(Date.now() / 1000) + (opts.expiresInSeconds ?? 3600),
    ...(opts.displayName === undefined ? {} : { displayName: opts.displayName }),
  })
  return `${header}.${payload}.not-a-real-signature`
}

export interface Harness extends BuiltApp {
  gateway: FakeGateway
  /** Cookie header for a signed-in user of the given roles. */
  cookieFor(sub: string, roles: Role[]): string
  close(): Promise<void>
}

export async function harness(
  opts: {
    config?: ConsoleConfig
    gateway?: FakeGateway
    prometheus?: PrometheusReader
    cluster?: ClusterReader
    now?: () => Date
  } = {},
): Promise<Harness> {
  const config = opts.config ?? testConfig()
  const gateway = opts.gateway ?? fakeGateway(config)
  const built = await buildApp({
    config,
    logger: SILENT_LOGGER,
    metrics: consoleMetrics,
    gateway,
    prometheus: opts.prometheus ?? fakePrometheus(),
    cluster: opts.cluster ?? fakeCluster(EMPTY_CLUSTER),
    serveStatic: false,
    ...(opts.now === undefined ? {} : { now: opts.now }),
  })
  await built.app.ready()

  return {
    ...built,
    gateway,
    cookieFor(sub, roles) {
      const token = fakeToken({ sub, roles })
      const signed = built.app.signCookie(token)
      return `${config.cookieName}=${signed}`
    },
    async close() {
      built.collector.close()
      await built.app.close()
    },
  }
}
