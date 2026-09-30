/**
 * Configuration is environment variables only (design, section 7). Nothing in
 * this file has a `localhost` default: an unset upstream is reported as
 * unconfigured and the console says so, rather than quietly calling the wrong
 * host. Appendix A of the design is the authority for the names.
 */

export interface UpstreamConfig {
  /** Registry name of the service, for logs and error messages. */
  service: string
  /** Base URL, or null when the variable is unset. */
  url: string | null
  /** The environment variable that sets it. */
  env: string
}

export interface ConsoleConfig {
  port: number
  logLevel: string
  serviceName: string
  serviceVersion: string
  otlpEndpoint: string | null
  podNamespace: string
  /**
   * Read pods and deployments from the Kubernetes API. Off means the topology
   * still draws the registry and Prometheus data, and says plainly that it has
   * no pod information; useful for a laptop demo with no cluster.
   */
  kubernetesEnabled: boolean
  sessionSecret: string
  cookieSecure: boolean
  cookieName: string
  /** Where the built SPA lives; overridable so dev and the image agree. */
  webRoot: string
  prometheusUrl: string | null
  grafanaExternalUrl: string | null
  jaegerExternalUrl: string | null
  /** Milliseconds between topology assemblies while SSE clients are connected. */
  topologyIntervalMs: number
  /** Milliseconds between feed polls while SSE clients are connected. */
  feedIntervalMs: number
  /** Outbound upstream request timeout. */
  upstreamTimeoutMs: number
  /** Prometheus gets a little longer: a range query over a busy factory is slow. */
  prometheusTimeoutMs: number
  upstreams: Record<UpstreamKey, UpstreamConfig>
}

export type UpstreamKey =
  | 'detractor'
  | 'gateway'
  | 'grammeter'
  | 'malaise'
  | 'orchestrator'
  | 'girdlespring'
  | 'depot'
  | 'datastore'
  | 'detector'
  | 'dispatch'
  | 'recorder'

interface UpstreamSpec {
  service: string
  /** Primary variable first; later names are accepted fallbacks. */
  envs: string[]
}

/**
 * Appendix A: service URLs are `<SERVICE_NAME_IN_UPPER_SNAKE>_URL`. The malaise
 * engine is the one exception the design names explicitly: `MALAISE_ENGINE_URL`.
 */
const UPSTREAM_SPECS: Record<UpstreamKey, UpstreamSpec> = {
  detractor: { service: 'unilateral-phase-detractor', envs: ['UNILATERAL_PHASE_DETRACTOR_URL'] },
  gateway: { service: 'dingle-arm-gateway', envs: ['DINGLE_ARM_GATEWAY_URL'] },
  grammeter: { service: 'cardinal-grammeter', envs: ['CARDINAL_GRAMMETER_URL'] },
  malaise: {
    service: 'malaise-injection-engine',
    envs: ['MALAISE_ENGINE_URL', 'MALAISE_INJECTION_ENGINE_URL'],
  },
  orchestrator: { service: 'demo-scenario-orchestrator', envs: ['DEMO_SCENARIO_ORCHESTRATOR_URL'] },
  girdlespring: { service: 'differential-girdlespring', envs: ['DIFFERENTIAL_GIRDLESPRING_URL'] },
  depot: { service: 'prefabulated-amulite-depot', envs: ['PREFABULATED_AMULITE_DEPOT_URL'] },
  datastore: { service: 'hydrocoptic-datastore', envs: ['HYDROCOPTIC_DATASTORE_URL'] },
  detector: { service: 'side-fumbling-detector', envs: ['SIDE_FUMBLING_DETECTOR_URL'] },
  dispatch: { service: 'nofer-trunnion-dispatch', envs: ['NOFER_TRUNNION_DISPATCH_URL'] },
  recorder: { service: 'sinusoidal-recorder', envs: ['SINUSOIDAL_RECORDER_URL'] },
}

export const UPSTREAM_KEYS = Object.keys(UPSTREAM_SPECS) as UpstreamKey[]

function str(env: NodeJS.ProcessEnv, name: string): string | null {
  const raw = env[name]
  if (raw === undefined) return null
  const trimmed = raw.trim()
  return trimmed === '' ? null : trimmed
}

function url(env: NodeJS.ProcessEnv, names: string[]): { url: string | null; env: string } {
  for (const name of names) {
    const value = str(env, name)
    if (value !== null) return { url: value.replace(/\/+$/, ''), env: name }
  }
  return { url: null, env: names[0] as string }
}

function int(env: NodeJS.ProcessEnv, name: string, fallback: number): number {
  const raw = str(env, name)
  if (raw === null) return fallback
  const parsed = Number.parseInt(raw, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

function bool(env: NodeJS.ProcessEnv, name: string, fallback: boolean): boolean {
  const raw = str(env, name)
  if (raw === null) return fallback
  return /^(1|true|yes|on)$/i.test(raw)
}

export class MissingSessionSecretError extends Error {
  constructor() {
    super(
      'SESSION_SECRET is required: it signs the session cookie. ' +
        'Set it from a Kubernetes Secret (the chart generates one on first install).',
    )
    this.name = 'MissingSessionSecretError'
  }
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): ConsoleConfig {
  const sessionSecret = str(env, 'SESSION_SECRET')
  if (sessionSecret === null || sessionSecret.length < 16) {
    throw new MissingSessionSecretError()
  }

  const upstreams = {} as Record<UpstreamKey, UpstreamConfig>
  for (const key of UPSTREAM_KEYS) {
    const spec = UPSTREAM_SPECS[key]
    const resolved = url(env, spec.envs)
    upstreams[key] = { service: spec.service, url: resolved.url, env: resolved.env }
  }

  return {
    port: int(env, 'PORT', 8080),
    logLevel: str(env, 'LOG_LEVEL') ?? 'info',
    serviceName: str(env, 'OTEL_SERVICE_NAME') ?? 'encabulation-control-console',
    serviceVersion: str(env, 'SERVICE_VERSION') ?? '1.0.0',
    otlpEndpoint: str(env, 'OTEL_EXPORTER_OTLP_ENDPOINT'),
    podNamespace: str(env, 'POD_NAMESPACE') ?? 'encabulator',
    kubernetesEnabled: bool(env, 'KUBERNETES_ENABLED', true),
    sessionSecret,
    cookieSecure: bool(env, 'COOKIE_SECURE', false),
    cookieName: str(env, 'SESSION_COOKIE_NAME') ?? 'encabulator_session',
    webRoot: str(env, 'WEB_ROOT') ?? '',
    prometheusUrl: url(env, ['PROMETHEUS_URL']).url,
    grafanaExternalUrl: url(env, ['GRAFANA_EXTERNAL_URL']).url,
    jaegerExternalUrl: url(env, ['JAEGER_EXTERNAL_URL']).url,
    topologyIntervalMs: int(env, 'TOPOLOGY_INTERVAL_MS', 3000),
    feedIntervalMs: int(env, 'FEED_INTERVAL_MS', 3000),
    upstreamTimeoutMs: int(env, 'UPSTREAM_TIMEOUT_MS', 3000),
    prometheusTimeoutMs: int(env, 'PROMETHEUS_TIMEOUT_MS', 5000),
    upstreams,
  }
}
