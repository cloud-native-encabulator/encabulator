/**
 * DEV ONLY. This file is never copied into the production image.
 *
 * A single process that answers every upstream the console talks to, so the SPA
 * can be developed without a Kubernetes cluster. It is a fixture server, not a
 * simulator: the numbers below are fixed constants in a file you can read, and
 * they move only on a slow, obvious sawtooth so that an animation can be seen to
 * respond. Nothing here is random, and nothing here ships.
 *
 *   node --experimental-strip-types dev/fixture-server.ts     # or: npm run dev:fixtures
 *
 * Point the BFF at it with `dev/dev.env`, which `npm run dev:bff` loads.
 */

import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'

const PORT = Number.parseInt(process.env['FIXTURE_PORT'] ?? '9099', 10)

/** A slow sawtooth so gauges visibly move. Deterministic: a clock, not a die. */
function wave(periodSeconds: number, low: number, high: number): number {
  const phase = (Date.now() / 1000 / periodSeconds) % 1
  const triangle = phase < 0.5 ? phase * 2 : 2 - phase * 2
  return low + (high - low) * triangle
}

const SERVICES = [
  'dingle-arm-gateway',
  'panametric-fam-processor',
  'spurving-bearing-quoter',
  'logarithmic-casing-line',
  'nofer-trunnion-dispatch',
  'hydrocoptic-datastore',
  'prefabulated-amulite-depot',
  'sinusoidal-recorder',
  'unilateral-phase-detractor',
  'differential-girdlespring',
  'side-fumbling-detector',
  'cardinal-grammeter',
  'malaise-injection-engine',
  'demo-scenario-orchestrator',
  'encabulation-control-console',
]

const EDGES: [string, string, number][] = [
  ['cardinal-grammeter', 'dingle-arm-gateway', 24],
  ['dingle-arm-gateway', 'unilateral-phase-detractor', 4],
  ['dingle-arm-gateway', 'prefabulated-amulite-depot', 9],
  ['dingle-arm-gateway', 'spurving-bearing-quoter', 6],
  ['dingle-arm-gateway', 'panametric-fam-processor', 5],
  ['dingle-arm-gateway', 'hydrocoptic-datastore', 4],
  ['panametric-fam-processor', 'spurving-bearing-quoter', 5],
  ['panametric-fam-processor', 'prefabulated-amulite-depot', 5],
  ['panametric-fam-processor', 'hydrocoptic-datastore', 5],
  ['panametric-fam-processor', 'nats', 10],
  ['logarithmic-casing-line', 'sinusoidal-recorder', 14],
  ['logarithmic-casing-line', 'nats', 8],
  ['side-fumbling-detector', 'sinusoidal-recorder', 3],
  ['side-fumbling-detector', 'valkey', 2],
  ['nofer-trunnion-dispatch', 'valkey', 3],
  ['hydrocoptic-datastore', 'postgres', 7],
  ['unilateral-phase-detractor', 'postgres', 4],
  ['differential-girdlespring', 'postgres', 2],
  ['prefabulated-amulite-depot', 'valkey', 9],
  ['spurving-bearing-quoter', 'valkey', 6],
  ['sinusoidal-recorder', 'influxdb', 14],
  ['encabulation-control-console', 'hydrocoptic-datastore', 1],
]

const PARTS = [
  { sku: 'AMU-BASE', name: 'Prefabulated amulite base-plate', perUnit: 1, stock: 912 },
  { sku: 'LOG-CASE', name: 'Malleable logarithmic casing', perUnit: 1, stock: 880 },
  { sku: 'SPV-BRG', name: 'Spurving bearing', perUnit: 2, stock: 1640 },
  { sku: 'PNT-FAN', name: 'Pentametric fan', perUnit: 1, stock: 903 },
  { sku: 'HYD-MARZ', name: 'Hydrocoptic marzlevane', perUnit: 6, stock: 2411 },
  { sku: 'LUN-WANE', name: 'Ambifacient lunar waneshaft', perUnit: 1, stock: 47 },
  { sku: 'LOT-WIND', name: 'Lotus-o-delta main winding', perUnit: 1, stock: 876 },
  { sku: 'TRM-PIPE', name: 'Non-reversible tremie pipe', perUnit: 3, stock: 2502 },
  { sku: 'GRD-SPRG', name: 'Differential girdlespring', perUnit: 1, stock: 899 },
  { sku: 'DNG-ARM', name: 'Drawn reciprocation dingle arm', perUnit: 1, stock: 264 },
]

const FLAGS = [
  { key: 'turbo-encabulation', type: 'boolean', value: false, description: 'Halves stage durations; quoter adds a 15% surcharge.', updatedBy: 'seed', updatedAt: '2026-09-30T08:00:00.000Z' },
  { key: 'quote-cache', type: 'boolean', value: true, description: 'Cache quotes in Valkey for 60s.', updatedBy: 'seed', updatedAt: '2026-09-30T08:00:00.000Z' },
  { key: 'retro-encabulator-sales', type: 'boolean', value: true, description: 'When false, retro orders are rejected.', updatedBy: 'seed', updatedAt: '2026-09-30T08:00:00.000Z' },
  { key: 'strict-side-fumbling', type: 'boolean', value: false, description: 'Tighter anomaly thresholds.', updatedBy: 'seed', updatedAt: '2026-09-30T08:00:00.000Z' },
  { key: 'amulite-foundry-rate', type: 'number', value: 20, description: 'Units of each SKU restocked per 10s.', updatedBy: 'seed', updatedAt: '2026-09-30T08:00:00.000Z' },
  { key: 'gateway-rate-limit-rps', type: 'number', value: 50, description: 'Per-subject token bucket rate.', updatedBy: 'seed', updatedAt: '2026-09-30T08:00:00.000Z' },
  { key: 'dingle-arm-damping', type: 'number', value: 50, description: 'Percent defect-rate reduction with a dingle arm.', updatedBy: 'seed', updatedAt: '2026-09-30T08:00:00.000Z' },
]

/** A JWT with a real structure and a fake signature, exactly like the tests use. */
function devToken(sub: string, roles: string[]): string {
  const b64 = (o: unknown) => Buffer.from(JSON.stringify(o)).toString('base64url')
  return [
    b64({ alg: 'RS256', typ: 'JWT', kid: 'dev' }),
    b64({
      sub,
      roles,
      iss: 'unilateral-phase-detractor',
      aud: 'encabulator',
      exp: Math.floor(Date.now() / 1000) + 3600,
      displayName: sub,
    }),
    'dev-fixture-not-a-real-signature',
  ].join('.')
}

/** `dev` / `dev` signs you in; the role comes from the username. */
const DEV_USERS: Record<string, string[]> = {
  dev: ['admin'],
  admin: ['admin'],
  operator: ['operator'],
  viewer: ['viewer'],
  customer: ['customer'],
}

function promVector(rows: { labels: Record<string, string>; value: number }[]): unknown {
  return {
    status: 'success',
    data: {
      resultType: 'vector',
      result: rows.map((row) => ({
        metric: row.labels,
        value: [Math.floor(Date.now() / 1000), String(row.value)],
      })),
    },
  }
}

function podsFor(service: string): string[] {
  const replicas = service === 'logarithmic-casing-line' ? 3 : service === 'spurving-bearing-quoter' ? 2 : 1
  return Array.from({ length: replicas }, (_, i) => `${service}-7d4f9-${'abcde'.slice(i, i + 1)}${i}x`)
}

function prometheusAnswer(query: string): unknown {
  const busy = wave(45, 0.6, 1.4)

  if (query.includes('traces_service_graph_request_total')) {
    return promVector(
      EDGES.map(([client, server, base]) => ({
        labels: { client, server },
        value: Number((base * busy).toFixed(3)),
      })),
    )
  }
  if (query.includes('traces_service_graph_request_failed_total')) {
    return promVector([
      { labels: { client: 'dingle-arm-gateway', server: 'spurving-bearing-quoter' }, value: 0.09 },
    ])
  }
  if (query.includes('traces_service_graph_request_server_seconds_bucket')) {
    const p99 = query.includes('0.99')
    return promVector(
      EDGES.map(([client, server], i) => ({
        labels: { client, server },
        // A spread of latencies so all four health colours appear on the panel.
        value: p99 ? [0.04, 0.08, 0.3, 0.7][i % 4]! : [0.01, 0.02, 0.06, 0.2][i % 4]!,
      })),
    )
  }
  if (query.includes('http_server_request_duration_seconds_count')) {
    const errors = query.includes('5..')
    return promVector(
      SERVICES.flatMap((service) =>
        podsFor(service).map((pod) => ({
          labels: { pod, namespace: 'encabulator' },
          value: errors ? (service === 'spurving-bearing-quoter' ? 0.4 : 0) : Number((6 * busy).toFixed(3)),
        })),
      ),
    )
  }
  if (query.includes('http_server_request_duration_seconds_bucket')) {
    const p99 = query.includes('0.99')
    return promVector(
      SERVICES.flatMap((service, i) =>
        podsFor(service).map((pod) => ({
          labels: { pod, namespace: 'encabulator' },
          value: p99 ? [0.05, 0.12, 0.42, 0.9][i % 4]! : [0.01, 0.03, 0.08, 0.2][i % 4]!,
        })),
      ),
    )
  }
  return promVector([])
}

const runLog = [
  { time: '2026-09-30T12:00:00.000Z', narration: 'The morning shift arrives; the grammeters begin to synchronise.' },
  { time: '2026-09-30T12:00:20.000Z', narration: 'The amulite foundry falls silent at the presenter’s request.' },
  { time: '2026-09-30T12:00:44.000Z', narration: 'Marzlevanes run short; the depot begins refusing reservations with 409.' },
]

function route(url: URL, method: string, body: unknown): { status: number; payload: unknown } {
  const p = url.pathname

  // ── identity ──────────────────────────────────────────────────────────────
  if (p === '/v1/auth/login' && method === 'POST') {
    const creds = body as { username?: string; password?: string } | null
    const username = creds?.username ?? ''
    const roles = DEV_USERS[username]
    if (roles === undefined || creds?.password === '') {
      return { status: 401, payload: { error: { code: 'invalid_credentials', message: 'Try dev / dev.' } } }
    }
    return {
      status: 200,
      payload: {
        accessToken: devToken(username, roles),
        tokenType: 'Bearer',
        expiresIn: 3600,
        user: { username, displayName: username, roles },
      },
    }
  }
  if (p === '/v1/users') {
    if (method === 'GET') {
      return {
        status: 200,
        payload: {
          users: Object.entries(DEV_USERS).map(([username, roles]) => ({
            username,
            displayName: username,
            roles,
            createdAt: '2026-09-30T08:00:00.000Z',
          })),
        },
      }
    }
    return { status: 201, payload: { username: 'created', roles: ['customer'] } }
  }
  if (p.startsWith('/v1/users/')) return { status: 200, payload: { ok: true } }
  if (p === '/v1/audit') {
    return {
      status: 200,
      payload: {
        entries: [
          { time: '2026-09-30T12:00:00.000Z', action: 'login', subject: 'dev', actor: 'dev', result: 'success' },
          { time: '2026-09-30T11:59:00.000Z', action: 'login', subject: 'nobody', actor: 'nobody', result: 'failure', detail: 'unknown user' },
        ],
      },
    }
  }

  // ── grammeter ─────────────────────────────────────────────────────────────
  if (p === '/v1/traffic') {
    return {
      status: 200,
      payload: {
        running: true,
        rps: 24,
        pattern: 'sine',
        periodSeconds: 60,
        customers: 50,
        mix: { browse: 40, quote: 25, order: 20, track: 15 },
        targetRps: 24,
        actualRps: Number(wave(45, 14, 28).toFixed(2)),
        inFlight: Math.round(wave(20, 2, 18)),
        dropped: 0,
        journeys: {
          browse: { count: 1204, errors: 0, p50Ms: 9, p99Ms: 42 },
          quote: { count: 748, errors: 6, p50Ms: 24, p99Ms: 180 },
          order: { count: 602, errors: 2, p50Ms: 61, p99Ms: 420 },
          track: { count: 451, errors: 0, p50Ms: 11, p99Ms: 58 },
        },
        statusCodes: { '200': 2604, '202': 602, '409': 6, '429': 2, '503': 2 },
      },
    }
  }

  // ── malaise engine ────────────────────────────────────────────────────────
  if (p === '/v1/malaises') {
    if (method === 'GET') {
      return {
        status: 200,
        payload: {
          malaises: [
            {
              id: 'mal_01JDEVFIXTURE0000000001',
              target: 'spurving-bearing-quoter',
              kind: 'latency',
              params: { ms: 400, jitterMs: 100, percent: 100 },
              state: 'active',
              expiresAt: new Date(Date.now() + 90_000).toISOString(),
            },
          ],
        },
      }
    }
    return { status: 201, payload: { id: 'mal_01JDEVFIXTURE0000000002', state: 'active' } }
  }
  if (p.startsWith('/v1/malaises/')) return { status: 204, payload: null }
  if (p === '/v1/directives') return { status: 200, payload: { directives: [] } }

  // ── orchestrator ──────────────────────────────────────────────────────────
  if (p === '/v1/scenarios') {
    return {
      status: 200,
      payload: {
        scenarios: [
          { id: 'morning-shift', title: 'Morning shift', description: 'Ramp the grammeters up to a working rate.' },
          { id: 'marzlevane-shortage', title: 'Marzlevane shortage', description: 'Stop the foundry, watch the 409s, restart it.' },
          { id: 'hydrocoptic-outage', title: 'Hydrocoptic outage', description: 'Kill Postgres and watch the ledger recover.' },
          { id: 'side-fumbling-outbreak', title: 'Side fumbling outbreak', description: 'Slow the casing line and tighten inspection.' },
          { id: 'dingle-arm-congestion', title: 'Dingle-arm congestion', description: 'Congest the gateway under burst traffic.' },
          { id: 'grand-tour', title: 'The grand tour', description: 'All of the above, narrated in full encabulator prose.' },
        ],
      },
    }
  }
  if (p === '/v1/runs') {
    return {
      status: 200,
      payload: {
        runs: [
          { id: 'run_01JDEVFIXTURE000001', scenarioId: 'marzlevane-shortage', status: 'running', startedAt: '2026-09-30T12:00:00.000Z' },
        ],
      },
    }
  }
  if (p.startsWith('/v1/runs/') && p.endsWith('/cancel')) return { status: 202, payload: { ok: true } }
  if (p.startsWith('/v1/runs/')) {
    return {
      status: 200,
      payload: {
        id: 'run_01JDEVFIXTURE000001',
        scenarioId: 'marzlevane-shortage',
        status: 'running',
        currentStep: 'stop the foundry',
        stepIndex: 2,
        stepCount: 7,
        log: runLog,
      },
    }
  }
  if (p.startsWith('/v1/scenarios/')) return { status: 202, payload: { id: 'run_01JDEVFIXTURE000002' } }

  // ── girdlespring ──────────────────────────────────────────────────────────
  if (p === '/v1/flags') return { status: 200, payload: { flags: FLAGS } }
  if (p.startsWith('/v1/flags/')) return { status: 200, payload: FLAGS[0] }

  // ── depot ─────────────────────────────────────────────────────────────────
  if (p === '/v1/parts' || p === '/api/v1/parts') {
    return { status: 200, payload: PARTS.map((part) => ({ ...part, reserved: part.perUnit * 3 })) }
  }
  if (p.endsWith('/restock')) return { status: 200, payload: { ok: true } }

  // ── ledger ────────────────────────────────────────────────────────────────
  if (p === '/v1/stats') {
    // Both the ledger and the detector answer /v1/stats; the console asks each
    // on its own base URL, so one fixture body carries both shapes.
    return {
      status: 200,
      payload: {
        byStatus: { accepted: 3, fabricating: 5, inspecting: 2, refabricating: 1, dispatched: 9, delivered: 142, scrapped: 2 },
        throughput: { ordersAccepted: Math.round(wave(60, 18, 42)), unitsDelivered: Math.round(wave(60, 9, 26)) },
        inspected: 320,
        passed: 298,
        failed: 22,
        failRate5m: 0.068,
      },
    }
  }
  if (p === '/v1/orders' || p === '/api/v1/orders') {
    if (method === 'POST') {
      return {
        status: 202,
        payload: {
          id: 'ord_01JDEVFIXTURE000010',
          customer: 'dev',
          model: 'turbo',
          quantity: 1,
          status: 'accepted',
          stage: null,
          quote: { id: 'qte_01JDEVFIXTURE00001', total: 1944, currency: 'USD' },
          createdAt: new Date().toISOString(),
        },
      }
    }
    return {
      status: 200,
      payload: {
        orders: [
          { id: 'ord_01JDEVFIXTURE000001', customer: 'flora', model: 'turbo', quantity: 2, status: 'fabricating', stage: 'fit-marzlevanes', quote: { total: 3888 }, updatedAt: new Date(Date.now() - 4000).toISOString() },
          { id: 'ord_01JDEVFIXTURE000002', customer: 'gus', model: 'hyper', quantity: 1, status: 'inspecting', stage: null, quote: { total: 4096 }, updatedAt: new Date(Date.now() - 12_000).toISOString() },
          { id: 'ord_01JDEVFIXTURE000003', customer: 'ida', model: 'retro', quantity: 5, status: 'dispatched', stage: null, quote: { total: 13_057.75 }, updatedAt: new Date(Date.now() - 40_000).toISOString() },
        ],
      },
    }
  }
  if (p.startsWith('/v1/orders/') || p.startsWith('/api/v1/orders/')) {
    return {
      status: 200,
      payload: {
        id: p.split('/').pop(),
        customer: 'flora',
        model: 'turbo',
        quantity: 2,
        status: 'fabricating',
        stage: 'fit-marzlevanes',
        attempt: 1,
        serials: ['ENC-turbo-m4k2z1-9f3a', 'ENC-turbo-m4k2z1-c07b'],
        quote: { id: 'qte_01JDEVFIXTURE00002', total: 3888, currency: 'USD' },
        createdAt: '2026-09-30T11:58:00.000Z',
        updatedAt: new Date().toISOString(),
        timeline: [
          { time: '2026-09-30T11:58:00.000Z', type: 'order.accepted', detail: 'quote 3888.00 USD' },
          { time: '2026-09-30T11:58:01.000Z', type: 'fabrication.requested', detail: 'attempt 1' },
          { time: '2026-09-30T11:58:04.000Z', type: 'fabrication.stage', detail: 'align-spurving-bearings' },
        ],
      },
    }
  }

  // ── detector ──────────────────────────────────────────────────────────────
  if (p === '/v1/incidents') {
    return {
      status: 200,
      payload: {
        incidents: [
          { id: 'inc_01JDEVFIXTURE00001', orderId: 'ord_01JDEVFIXTURE000002', serial: 'ENC-hyper-m4k2yy-1a2b', stage: 'wind-lotus-o-delta', metric: 'repleneration', score: 4.82, reason: 'rolling z-score above 4.0', strict: false, time: new Date(Date.now() - 9000).toISOString() },
          { id: 'inc_01JDEVFIXTURE00002', orderId: 'ord_01JDEVFIXTURE000002', serial: 'ENC-hyper-m4k2yy-1a2b', stage: 'fit-marzlevanes', metric: 'marzlevane_alignment', score: 5.41, reason: 'phase slip', strict: false, time: new Date(Date.now() - 26_000).toISOString() },
        ],
      },
    }
  }

  // ── dispatch ──────────────────────────────────────────────────────────────
  if (p === '/v1/shipments' || p.startsWith('/v1/shipments/')) {
    return {
      status: 200,
      payload: {
        shipments: [
          { id: 'shp_01JDEVFIXTURE00001', orderId: 'ord_01JDEVFIXTURE000003', carrier: 'Waneshaft Express', trackingNumber: 'NT4820193756', status: 'dispatched', updatedAt: new Date(Date.now() - 18_000).toISOString() },
          { id: 'shp_01JDEVFIXTURE00002', orderId: 'ord_01JDEVFIXTURE000004', carrier: 'Tremie Pipe & Sons', trackingNumber: 'NT1039485712', status: 'delivered', updatedAt: new Date(Date.now() - 62_000).toISOString() },
        ],
      },
    }
  }

  // ── recorder ──────────────────────────────────────────────────────────────
  if (p === '/v1/summary') {
    return {
      status: 200,
      payload: {
        window: '5m',
        metrics: {
          repleneration: { count: 12_400, mean: Number(wave(90, 0.96, 1.04).toFixed(4)), p95: 1.09 },
          marzlevane_alignment: { count: 12_400, mean: 0.01, p95: 0.38 },
          grammeter_drift: { count: 12_400, mean: 0.4, p95: 3.9 },
        },
      },
    }
  }

  // ── quoter, through the gateway ───────────────────────────────────────────
  if (p === '/api/v1/quotes' || p === '/v1/quotes') {
    const input = (body ?? {}) as { model?: string; quantity?: number; options?: { dingleArm?: boolean } }
    const base = { turbo: 1944, retro: 2749, hyper: 4096 }[input.model ?? 'turbo'] ?? 1944
    const quantity = input.quantity ?? 1
    const unit = base + (input.options?.dingleArm === true ? 312 : 0)
    const subtotal = unit * quantity
    const discount = quantity >= 5 ? subtotal * 0.05 : 0
    return {
      status: 200,
      payload: {
        id: 'qte_01JDEVFIXTURE00003',
        model: input.model ?? 'turbo',
        quantity,
        options: input.options ?? { dingleArm: false },
        unitPrice: unit,
        subtotal,
        surcharges: discount > 0 ? [{ code: 'volume_discount', amount: -Number(discount.toFixed(2)) }] : [],
        total: Number((subtotal - discount).toFixed(2)),
        currency: 'USD',
        cached: false,
        computedAt: new Date().toISOString(),
      },
    }
  }

  // ── Prometheus ────────────────────────────────────────────────────────────
  if (p === '/api/v1/query') {
    return { status: 200, payload: prometheusAnswer(url.searchParams.get('query') ?? '') }
  }

  return { status: 404, payload: { error: { code: 'not_found', message: `fixture has no ${method} ${p}` } } }
}

function readBody(req: IncomingMessage): Promise<unknown> {
  return new Promise((resolve) => {
    const chunks: Buffer[] = []
    req.on('data', (chunk: Buffer) => chunks.push(chunk))
    req.on('end', () => {
      const text = Buffer.concat(chunks).toString('utf8')
      if (text === '') {
        resolve(null)
        return
      }
      try {
        resolve(JSON.parse(text))
      } catch {
        resolve(text)
      }
    })
  })
}

createServer(async (req: IncomingMessage, res: ServerResponse) => {
  const url = new URL(req.url ?? '/', 'http://fixtures.invalid')
  const body = await readBody(req)
  const { status, payload } = route(url, req.method ?? 'GET', body)
  process.stdout.write(`${req.method} ${url.pathname} -> ${status}\n`)
  res.writeHead(status, { 'content-type': 'application/json' })
  res.end(payload === null ? '' : JSON.stringify(payload))
}).listen(PORT, () => {
  process.stdout.write(
    `dev fixtures on http://127.0.0.1:${PORT}\n` +
      `sign in with dev / dev (admin), or operator / viewer / customer with any password\n`,
  )
})
