/**
 * The static node registry: the fifteen services and four backing stores from
 * the design's section 2. Nothing here is telemetry; it is the nameplate under
 * each instrument. Live pods and metrics are merged on top by the BFF.
 *
 * `lay` is a deterministic grid slot (12 columns wide) so the panel never
 * re-shuffles between snapshots. It mirrors the topology diagram: control
 * plane on top, the gateway below it, the processing and support planes in the
 * middle, the data plane under those, and the backing stores at the bottom.
 */

import type { NodeRegistration, Plane } from './topology.js'

export const PLANE_ORDER: Plane[] = ['control', 'processing', 'support', 'data']

export const PLANE_LABEL: Record<Plane, string> = {
  control: 'Control plane',
  processing: 'Processing plane',
  support: 'Support plane',
  data: 'Data plane',
}

export const REGISTRY: NodeRegistration[] = [
  // ── Row 0: control plane ───────────────────────────────────────────────────
  {
    name: 'encabulation-control-console',
    display: 'Encabulation Control Console',
    plane: 'control',
    kind: 'service',
    language: 'TypeScript',
    epigraph: 'The instrument panel itself, wired to its own bus bars.',
    plainly: 'This web console: a Vue single-page app served by a Fastify BFF.',
    lay: { col: 1, row: 0 },
  },
  {
    name: 'cardinal-grammeter',
    display: 'Cardinal Grammeter',
    plane: 'control',
    kind: 'service',
    language: 'Go',
    epigraph: 'Automatically synchronises cardinal grammeters.',
    plainly: 'Synthetic traffic generator: pools of fake customers driving the gateway.',
    lay: { col: 4, row: 0 },
  },
  {
    name: 'malaise-injection-engine',
    display: 'Malaise Injection Engine',
    plane: 'control',
    kind: 'service',
    language: 'Go',
    epigraph: 'Induces malaise, to prove the encabulator recovers from it.',
    plainly: 'Fault injection: latency, errors, congestion, CPU burn, pod kills, partitions, scaling.',
    lay: { col: 7, row: 0 },
  },
  {
    name: 'demo-scenario-orchestrator',
    display: 'Demo Scenario Orchestrator',
    plane: 'control',
    kind: 'service',
    language: 'Python',
    epigraph: 'Rehearses the demonstration so the presenter need not.',
    plainly: 'Runs scripted scenarios: traffic changes, faults, flag flips, with narration.',
    lay: { col: 10, row: 0 },
  },

  // ── Row 1: the edge ────────────────────────────────────────────────────────
  {
    name: 'dingle-arm-gateway',
    display: 'Dingle-Arm Gateway',
    plane: 'processing',
    kind: 'service',
    language: 'Go',
    epigraph: 'The drawn reciprocation dingle arm, for barescent skor motion.',
    plainly: 'Edge API gateway: JWT verification, per-subject rate limiting, routing.',
    lay: { col: 5.5, row: 1 },
  },

  // ── Row 2: processing ──────────────────────────────────────────────────────
  {
    name: 'spurving-bearing-quoter',
    display: 'Spurving-Bearing Quoter',
    plane: 'processing',
    kind: 'service',
    language: 'Rust',
    epigraph: 'Aligns the spurving bearings with the pentametric fan, economically.',
    plainly: 'Pricing engine. Burns real CPU computing a "pentametric harmonic" per quote.',
    lay: { col: 1, row: 2 },
  },
  {
    name: 'panametric-fam-processor',
    display: 'Panametric-Fam Processor',
    plane: 'processing',
    kind: 'service',
    language: 'TypeScript',
    epigraph: 'Named for the panametric fam, in direct line with the spurving bearings.',
    plainly: 'Order saga: quote, reserve parts, write the ledger, request fabrication.',
    lay: { col: 4, row: 2 },
  },
  {
    name: 'logarithmic-casing-line',
    display: 'Logarithmic Casing Line',
    plane: 'processing',
    kind: 'service',
    language: 'Python',
    epigraph: 'The malleable logarithmic casing surmounts the base-plate.',
    plainly: 'Fabrication workers: seven stages per unit, emitting real telemetry.',
    lay: { col: 7, row: 2 },
  },
  {
    name: 'nofer-trunnion-dispatch',
    display: 'Nofer-Trunnion Dispatch',
    plane: 'processing',
    kind: 'service',
    language: 'TypeScript',
    epigraph: 'For the operation of nofer trunnions (Time, 1946).',
    plainly: 'Shipping: carrier selection, tracking numbers, durable delivery timers.',
    lay: { col: 10, row: 2 },
  },

  // ── Row 3: support ─────────────────────────────────────────────────────────
  {
    name: 'unilateral-phase-detractor',
    display: 'Unilateral Phase Detractor',
    plane: 'support',
    kind: 'service',
    language: 'Python',
    epigraph: 'Supplies inverse reactive credentials for unilateral phase detractors.',
    plainly: 'Identity: users, Argon2id passwords, RS256 JWT issuance, JWKS.',
    lay: { col: 1, row: 3 },
  },
  {
    name: 'differential-girdlespring',
    display: 'Differential Girdlespring',
    plane: 'support',
    kind: 'service',
    language: 'Go',
    epigraph: 'The differential girdlespring on the "up" end of the grammeters.',
    plainly: 'Feature flags, polled every five seconds with ETags.',
    lay: { col: 4, row: 3 },
  },
  {
    name: 'side-fumbling-detector',
    display: 'Side-Fumbling Detector',
    plane: 'support',
    kind: 'service',
    language: 'Python',
    epigraph: 'So fitted that side fumbling is effectively prevented.',
    plainly: 'Inspection: rolling z-scores over unit telemetry, and incident records.',
    lay: { col: 7, row: 3 },
  },

  // ── Row 4: data ────────────────────────────────────────────────────────────
  {
    name: 'hydrocoptic-datastore',
    display: 'Hydrocoptic Datastore',
    plane: 'data',
    kind: 'service',
    language: 'Go',
    epigraph: 'Six hydrocoptic marzlevanes of pure persistence.',
    plainly: 'The order ledger and its timeline, fed by every event on the bus.',
    lay: { col: 1, row: 4 },
  },
  {
    name: 'prefabulated-amulite-depot',
    display: 'Prefabulated-Amulite Depot',
    plane: 'data',
    kind: 'service',
    language: 'TypeScript',
    epigraph: 'A base-plate of prefabulated amulite, and the rest of the parts cage.',
    plainly: 'Parts inventory: atomic reservations, commits, releases, and the foundry.',
    lay: { col: 4, row: 4 },
  },
  {
    name: 'sinusoidal-recorder',
    display: 'Sinusoidal Recorder',
    plane: 'data',
    kind: 'service',
    language: 'Go',
    epigraph: 'Records sinusoidal repleneration so that it may be reduced.',
    plainly: 'Unit telemetry: per-stage readings written to and queried from InfluxDB.',
    lay: { col: 7, row: 4 },
  },

  // ── Row 5: backing stores ──────────────────────────────────────────────────
  {
    name: 'postgres',
    display: 'PostgreSQL 18',
    plane: 'data',
    kind: 'store',
    language: 'postgres:18',
    epigraph: 'Ledgers, in ink that does not run.',
    plainly: 'Databases for the datastore, the detractor, and the girdlespring.',
    lay: { col: 1, row: 5 },
  },
  {
    name: 'valkey',
    display: 'Valkey 9',
    plane: 'data',
    kind: 'store',
    language: 'valkey/valkey:9',
    epigraph: 'Short-term memory for the parts cage and the quote book.',
    plainly: 'Cache and durable timers for the depot, quoter, dispatch, detector, malaise engine.',
    lay: { col: 4, row: 5 },
  },
  {
    name: 'nats',
    display: 'NATS 2.12 + JetStream',
    plane: 'data',
    kind: 'store',
    language: 'nats:2.12',
    epigraph: 'Every seventh conductor, connected by non-reversible tremie pipe.',
    plainly: 'The event bus: one stream, ENCABULATOR, subjects encabulator.>.',
    lay: { col: 7, row: 5 },
  },
  {
    name: 'influxdb',
    display: 'InfluxDB 3 Core',
    plane: 'data',
    kind: 'store',
    language: 'influxdb:3-core',
    epigraph: 'A strip chart that never runs out of paper.',
    plainly: 'Time series store for sinusoidal repleneration readings.',
    lay: { col: 10, row: 5 },
  },
]

export const REGISTRY_BY_NAME: Map<string, NodeRegistration> = new Map(
  REGISTRY.map((n) => [n.name, n]),
)

/**
 * Names the OTel service graph and `db.system` client spans use for the same
 * things the registry calls `postgres`, `valkey`, `nats` and `influxdb`.
 * Anything not in the registry and not here is reported as unmapped, never
 * silently dropped and never invented as a node.
 */
export const PEER_ALIASES: Record<string, string> = {
  postgresql: 'postgres',
  psql: 'postgres',
  'postgres:5432': 'postgres',
  redis: 'valkey',
  'valkey:6379': 'valkey',
  'nats:4222': 'nats',
  'nats.js': 'nats',
  jetstream: 'nats',
  influxdb3: 'influxdb',
  influx: 'influxdb',
  'influxdb:8181': 'influxdb',
}

/** Peers that are real but deliberately outside the drawn topology. */
export const IGNORED_PEERS = new Set(['prometheus', 'otel-collector', 'jaeger', 'grafana'])

/** Resolve a service-graph peer name to a registry node, or null. */
export function resolvePeer(raw: string): string | null {
  const name = raw.trim().toLowerCase()
  if (REGISTRY_BY_NAME.has(name)) return name
  const alias = PEER_ALIASES[name]
  if (alias !== undefined && REGISTRY_BY_NAME.has(alias)) return alias
  // Strip a namespace suffix the collector sometimes appends, e.g. "valkey.encabulator".
  const bare = name.split('.')[0]
  if (bare !== undefined && bare !== name) {
    if (REGISTRY_BY_NAME.has(bare)) return bare
    const bareAlias = PEER_ALIASES[bare]
    if (bareAlias !== undefined) return bareAlias
  }
  return null
}

/** Store nodes are databases and caches; edges into them are drawn as such. */
export function isStore(name: string): boolean {
  return REGISTRY_BY_NAME.get(name)?.kind === 'store'
}
