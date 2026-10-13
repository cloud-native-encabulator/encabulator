# Cloud Native Encabulator: System Design

> "The original machine had a base-plate of prefamulated amulite, surmounted by a malleable
> logarithmic casing in such a way that the two main spurving bearings were in a direct line
> with the pentametric fan." (J. H. Quick, *The Turbo-Encabulator in Industry*, 1944)

This document is the contract for the v1 system. Every service, API, event, and convention the
implementation relies on is defined here. When code and this document disagree, one of them is a
bug.

## 1. The premise

The Cloud Native Encabulator runs the **Encabulator Works**, a factory that builds turbo
encabulators to order. Customers browse the parts catalog, request quotes, and place orders. The
factory reserves parts, fabricates each unit through the canonical assembly stages, inspects it for
side fumbling, and ships it by nofer trunnion.

The names are nonsense. The system is not. Every arrow in the topology is a real network call or a
real message, every number on the console comes from real telemetry, and every fault the console
injects produces real, observable symptoms.

### Design principles

1. **Every edge is real.** If the topology shows A talking to B, A calls B in code.
2. **Every number is measured.** The console shows only data from Prometheus, the Kubernetes API,
   or a service API. No `Math.random()` on screen.
3. **Faults are first-class.** Every service honors injected malaise, so the demo can break any
   part of the system on demand.
4. **Polyglot on purpose.** Go, TypeScript, Python, and Rust, each where it fits.
5. **Swap-ready backends.** Each backing store has a managed AWS counterpart, so later phases can
   replace them with configuration rather than code changes.
6. **Small diffs between phases.** v1 is built so that adding GitOps, ACK, and kro are legible,
   demoable diffs.

## 2. Topology

Fifteen services in four planes, plus four backing stores and an observability platform. All
application workloads and backing stores run in the `encabulator` namespace; the plane is a label
(`encabulator.io/plane`), not a namespace.

```
                        ┌──────────────────────────── control plane ─────────────────────────────┐
  browser ──▶ encabulation-control-console (BFF) ──▶ cardinal-grammeter · malaise-injection-engine │
                        │                           demo-scenario-orchestrator                      │
                        └──────────────┬──────────────────────────────┬────────────────────────────┘
                                       ▼                              │ (grammeter drives traffic)
                               dingle-arm-gateway ◀───────────────────┘
               ┌──────────────┬────────┼─────────────┬──────────────────┬──────────────┐
               ▼              ▼        ▼             ▼                  ▼              ▼
   unilateral-phase-  prefabulated-  spurving-   panametric-fam-   hydrocoptic-   nofer-trunnion-
   detractor          amulite-depot  bearing-    processor         datastore      dispatch
   (identity)         (inventory)    quoter      (orchestrator)    (order ledger) (shipping)
                                     (pricing)        │
                                                      │ NATS JetStream: encabulator.>
                     ┌────────────────────────────────┼───────────────────────────┐
                     ▼                                ▼                           ▼
          logarithmic-casing-line  ──readings──▶ sinusoidal-recorder ◀── side-fumbling-detector
          (fabrication workers)                  (telemetry)               (inspection)

   differential-girdlespring (feature flags) is consulted by processor, quoter, gateway,
   casing-line, and detector.
```

### Service catalog

| Service | Plane | Lang | Role | Owns |
|---|---|---|---|---|
| `encabulation-control-console` | control | TypeScript (Vue + Fastify BFF) | The web portal: topology, traffic, malaise, scenarios, flags, storefront | none |
| `cardinal-grammeter` | control | Go | Synthetic customer traffic generator | none |
| `malaise-injection-engine` | control | Go | Fault injection, app-level and Kubernetes-level | Valkey keys `{malaise}:*` |
| `demo-scenario-orchestrator` | control | Python | Runs scripted demo scenarios with narration | in-memory run history |
| `dingle-arm-gateway` | processing | Go | Edge API gateway: JWT auth, rate limiting, routing | none |
| `panametric-fam-processor` | processing | TypeScript | Order orchestration saga | none |
| `spurving-bearing-quoter` | processing | Rust | Pricing engine | Valkey keys `quote:*` |
| `logarithmic-casing-line` | processing | Python | Fabrication workers | none |
| `nofer-trunnion-dispatch` | processing | TypeScript | Shipping and delivery | Valkey keys `{shipment}:*` |
| `hydrocoptic-datastore` | data | Go | Order ledger and timeline | Postgres db `hydrocoptic` |
| `prefabulated-amulite-depot` | data | TypeScript | Parts inventory and reservations | Valkey keys `{depot}:*` |
| `sinusoidal-recorder` | data | Go | Unit telemetry (sinusoidal repleneration readings) | InfluxDB database `sinusoidal` |
| `unilateral-phase-detractor` | support | Python | Identity: users, login, JWT issuance, JWKS | Postgres db `detractor` |
| `differential-girdlespring` | support | Go | Feature flags | Postgres db `girdlespring` |
| `side-fumbling-detector` | support | Python | Anomaly detection over unit telemetry; incidents | Valkey keys `incident:*` |

### Backing stores

| Store | Image | In-cluster DNS | Used by | Managed counterpart (phase 3) |
|---|---|---|---|---|
| PostgreSQL 18 | `postgres:18` | `postgres:5432` | datastore, detractor, girdlespring | Amazon RDS / Aurora PostgreSQL |
| Valkey 9 | `valkey/valkey:9` | `valkey:6379` | depot, quoter, dispatch, detector, malaise engine | Amazon ElastiCache for Valkey |
| NATS 2.12 + JetStream | `nats:2.12` | `nats:4222` | processor, casing-line, detector, dispatch, datastore | Amazon SQS/SNS (via adapter) or keep NATS |
| InfluxDB 3 Core | `influxdb:3-core` | `influxdb:8181` | sinusoidal-recorder | Amazon Timestream for InfluxDB |

Each Postgres database has its own user and password (Kubernetes Secret). Valkey uses ACL auth
with a single password in v1. InfluxDB and NATS are reachable only inside the namespace.

## 3. Domain

### Parts catalog

Seeded by the depot on first start. SKUs and the quantity each encabulator consumes:

| SKU | Part | Per unit |
|---|---|---|
| `AMU-BASE` | Prefabulated amulite base-plate | 1 |
| `LOG-CASE` | Malleable logarithmic casing | 1 |
| `SPV-BRG` | Spurving bearing | 2 |
| `PNT-FAN` | Pentametric fan | 1 |
| `HYD-MARZ` | Hydrocoptic marzlevane | 6 |
| `LUN-WANE` | Ambifacient lunar waneshaft | 1 |
| `LOT-WIND` | Lotus-o-delta main winding | 1 |
| `TRM-PIPE` | Non-reversible tremie pipe | 3 |
| `GRD-SPRG` | Differential girdlespring | 1 |
| `DNG-ARM` | Drawn reciprocation dingle arm | 1 if `options.dingleArm` |

### Models

| Model | Name | Base price | Stage time factor | Base defect rate |
|---|---|---|---|---|
| `turbo` | Turbo Encabulator (1944) | 1944.00 | 1.0 | 4% |
| `retro` | Retro Encabulator (Rockwell) | 2749.00 | 1.3 | 3% |
| `hyper` | HyperEncabulator (SANS ICS) | 4096.00 | 1.8 | 6% |

Options: `dingleArm: boolean` (adds `DNG-ARM`, costs 312.00, and cuts the defect rate in half by
"reducing sinusoidal repleneration"). Quantity is 1 to 10 per order.

### Fabrication stages

Each unit passes through seven stages, in order. Base durations are milliseconds, multiplied by
the model's stage time factor, halved when the `turbo-encabulation` flag is on, and jittered ±25%.

| # | Stage key | Description | Base ms |
|---|---|---|---|
| 1 | `prelaminate-base-plate` | Prelaminate the amulite base-plate | 300 |
| 2 | `surmount-logarithmic-casing` | Surmount the malleable logarithmic casing | 400 |
| 3 | `align-spurving-bearings` | Align both spurving bearings with the pentametric fan | 500 |
| 4 | `fit-marzlevanes` | Fit six hydrocoptic marzlevanes to the ambifacient lunar waneshaft | 700 |
| 5 | `wind-lotus-o-delta` | Wind the lotus-o-delta main winding into the panendermic semi-boloid slots | 600 |
| 6 | `connect-tremie-pipes` | Connect every seventh conductor by non-reversible tremie pipe | 400 |
| 7 | `synchronize-cardinal-grammeters` | Synchronize the cardinal grammeters | 300 |

### Order lifecycle

```
accepted ─▶ fabricating ─▶ inspecting ─┬─▶ dispatched ─▶ delivered
                ▲                      │
                └──── refabricating ◀──┤ (inspection failed, attempt < 3)
                                       └─▶ scrapped   (inspection failed, attempt = 3)
```

Orders that cannot be priced or reserved are rejected synchronously (HTTP 409/503) and are never
written to the ledger.

## 4. Workflows

### 4.1 Place an order (synchronous, through the gateway)

1. Client `POST /api/v1/orders` to `dingle-arm-gateway` with a bearer token.
2. Gateway verifies the JWT against the detractor's JWKS, applies the rate limit, and forwards to
   `panametric-fam-processor` with identity headers.
3. Processor reads flags (cached), then `POST spurving-bearing-quoter /v1/quotes`.
4. Processor `POST prefabulated-amulite-depot /v1/reservations`. On `409` the order is rejected
   with code `insufficient_parts`.
5. Processor `POST hydrocoptic-datastore /v1/orders` with status `accepted`. On failure, the
   processor releases the reservation (compensation) and returns 503.
6. Processor publishes `encabulator.order.accepted` and `encabulator.fabrication.requested`
   (attempt 1), and returns `202` with the order.

### 4.2 Fabricate, inspect, ship (asynchronous, over NATS)

1. `logarithmic-casing-line` pulls `fabrication.requested`. For each unit it assigns a serial
   (`ENC-<model>-<base36 time>-<4 hex>`), runs the seven stages, publishes a
   `fabrication.stage` event per stage, and writes readings to `sinusoidal-recorder`
   (`POST /v1/readings`) after each stage.
2. When all units are done it calls `POST depot /v1/reservations/{id}/commit` (attempt 1 only) and
   publishes `fabrication.completed`.
3. `side-fumbling-detector` pulls `fabrication.completed`, fetches readings for each serial
   (`GET sinusoidal-recorder /v1/readings?serial=`), and runs anomaly detection. It publishes
   `inspection.passed` or `inspection.failed`, and records an incident for each failed unit.
4. On `inspection.failed`, the processor publishes `fabrication.requested` with `attempt + 1`, or
   `order.scrapped` if the attempt was the third.
5. On `inspection.passed`, `nofer-trunnion-dispatch` assigns a carrier and tracking number,
   publishes `shipment.dispatched`, and later `shipment.delivered` after the simulated transit
   time (10 to 60 seconds, stored durably in Valkey so it survives restarts).
6. `hydrocoptic-datastore` consumes every event and updates the order's status and timeline.

### 4.3 How defects arise and are caught

The casing line does not label defective units. It generates telemetry, and a defective unit's
telemetry contains an anomaly: a spike, a drift, or a phase slip in one metric during one stage.
The detector must find it in the data. With the `strict-side-fumbling` flag on, the detector's
threshold tightens, which catches subtler anomalies and also produces false positives, so the
refabrication rate visibly rises.

Readings per stage: 20 samples each of three metrics, 10 ms apart in synthetic time:

- `repleneration`: sinusoidal amplitude around 1.0 (sin wave plus Gaussian noise σ = 0.05)
- `marzlevane_alignment`: degrees of misalignment around 0 (noise σ = 0.2)
- `grammeter_drift`: cumulative drift in ppm around 0 (random walk, step σ = 0.5)

Detector rule (default): fail a unit if any metric in any stage has a rolling z-score above 4.0
(window 10) or `repleneration` leaves [0.7, 1.3]. Strict mode: z > 3.0 and band [0.8, 1.2].

## 5. Events

All events go to one JetStream stream.

- Stream `ENCABULATOR`, subjects `encabulator.>`, file storage, limits retention, max age 24h,
  1 replica (v1), duplicate window 2m.
- Every service that publishes or consumes calls "create stream if missing" at startup with
  exactly this config. None of them updates an existing stream.
- Publishers set `Nats-Msg-Id` to the event `id` for deduplication.
- Trace context propagates in NATS headers (`traceparent`, `tracestate`). Publishers create a
  `PRODUCER` span and consumers a `CONSUMER` span, following OTel messaging conventions
  (`messaging.system=nats`, `messaging.destination.name=<subject>`), so the service graph links
  them.

Envelope (JSON body):

```json
{
  "id": "evt_01J...",            // ULID or UUIDv7
  "type": "encabulator.fabrication.requested",
  "time": "2026-09-30T12:00:00.000Z",
  "source": "panametric-fam-processor",
  "data": { }
}
```

| Subject | Publisher | Data |
|---|---|---|
| `encabulator.order.accepted` | processor | `orderId, customer, model, quantity, options, total, currency` |
| `encabulator.fabrication.requested` | processor | `orderId, attempt, model, quantity, options, reservationId` |
| `encabulator.fabrication.stage` | casing-line | `orderId, attempt, serial, stage, stageIndex (1-7), durationMs` |
| `encabulator.fabrication.completed` | casing-line | `orderId, attempt, model, serials[], durationMs` |
| `encabulator.inspection.passed` | detector | `orderId, attempt, serials[]` |
| `encabulator.inspection.failed` | detector | `orderId, attempt, serials[], findings[{serial, stage, metric, score, reason}]` |
| `encabulator.order.scrapped` | processor | `orderId, attempt, reason` |
| `encabulator.shipment.dispatched` | dispatch | `orderId, carrier, trackingNumber, etaSeconds` |
| `encabulator.shipment.delivered` | dispatch | `orderId, carrier, trackingNumber` |

Durable pull consumers (shared across a service's replicas, explicit ack, `AckWait` 30s,
`MaxDeliver` 5):

| Durable | Service | Filter | Notes |
|---|---|---|---|
| `logarithmic-casing-line` | casing-line | `encabulator.fabrication.requested` | `MaxAckPending` = 64; each pod processes `LINE_CAPACITY` orders at once (default 2) and sends in-progress acks while working |
| `side-fumbling-detector` | detector | `encabulator.fabrication.completed` | |
| `panametric-fam-processor` | processor | `encabulator.inspection.failed` | |
| `nofer-trunnion-dispatch` | dispatch | `encabulator.inspection.passed` | |
| `hydrocoptic-datastore` | datastore | `encabulator.>` | Must be idempotent and tolerate out-of-order delivery |

Consumer lag on `logarithmic-casing-line` is the congestion signal. KEDA scales the casing line on
it.

## 6. Feature flags

`differential-girdlespring` stores flags in Postgres. Clients poll `GET /v1/flags` every 5 seconds
with `If-None-Match`, keep the last good set, and fall back to built-in defaults if the service is
unreachable at startup.

| Key | Type | Default | Read by | Effect |
|---|---|---|---|---|
| `turbo-encabulation` | boolean | false | casing-line, quoter | Halves stage durations; quoter adds a 15% turbo surcharge |
| `quote-cache` | boolean | true | quoter | Cache quotes in Valkey for 60s |
| `retro-encabulator-sales` | boolean | true | processor | When false, `retro` orders are rejected with `model_unavailable` |
| `strict-side-fumbling` | boolean | false | detector | Tighter anomaly thresholds |
| `amulite-foundry-rate` | number | 20 | depot | Units of each SKU restocked per 10s (0 stops the foundry) |
| `gateway-rate-limit-rps` | number | 50 | gateway | Per-subject token bucket rate (burst = 2×) |
| `dingle-arm-damping` | number | 50 | casing-line | Percent reduction of defect rate when `dingleArm` is ordered |

Boolean flags may also carry `rolloutPercent` (0 to 100). Evaluation for a subject is
`fnv1a32(key + ":" + subject) % 100 < rolloutPercent`. When `rolloutPercent` is absent, the value
applies to everyone.

## 7. Cross-cutting conventions

Every service follows these. The shared kits (`libs/go/kit`, `libs/ts/kit`,
`libs/python/encabulator-kit`) implement them, and the Rust quoter implements them directly.

### HTTP

- Listen on `PORT` (default `8080`). One port serves the API, health, and metrics.
- `GET /healthz`: liveness. 200 `{"status":"ok"}` if the process is running.
- `GET /readyz`: readiness. 200 only if the service's *own* hard dependencies (its database, its
  NATS connection) are reachable. Never check downstream services.
- `GET /metrics`: Prometheus text format.
- Errors: `{"error": {"code": "snake_case_code", "message": "Human readable"}}`.
- Timeouts: server read-header 5s, idle 60s. Outbound client default timeout 3s.
- Outbound retries: idempotent requests (GET, PUT, DELETE) retry twice with exponential backoff
  (100ms base, full jitter). POST retries only when the caller supplies an idempotency key.
- Graceful shutdown on SIGTERM: fail readiness, stop accepting, drain for up to 20s, exit.
- `X-Request-Id`: accept or generate; echo in the response; include in logs.

### Identity propagation

The gateway strips any inbound `X-Encabulator-*` headers and sets:

- `X-Encabulator-Subject`: JWT `sub` (username)
- `X-Encabulator-Roles`: comma-separated roles

Internal services trust these headers. Roles: `customer`, `viewer`, `operator`, `admin`. The
console BFF calls control-plane services directly and sets the same headers from its session.

### Logging

JSON lines on stdout. Required fields: `time` (RFC 3339 with ms), `level`, `msg`, `service`.
Include `trace_id` and `span_id` when a span is active, and `request_id` for HTTP requests. Do
not log request bodies, tokens, or passwords. `LOG_LEVEL` sets the level (default `info`).

### Metrics

- `http_server_request_duration_seconds` histogram, labels `http_request_method`, `http_route`
  (the route template, never the raw path), `http_response_status_code`. Buckets: 0.005, 0.01,
  0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10.
- `http_client_request_duration_seconds` histogram, labels `http_request_method`,
  `server_address` (target service name), `http_response_status_code`.
- Domain metrics are prefixed `encabulator_` (for example `encabulator_orders_total{model,result}`,
  `encabulator_units_fabricated_total{model}`, `encabulator_parts_stock{sku}`).
- The runtime's default process and runtime metrics.

### Tracing

- OpenTelemetry SDK, OTLP over HTTP/protobuf to `OTEL_EXPORTER_OTLP_ENDPOINT`. When the variable
  is unset, tracing is a no-op (this keeps tests and local runs quiet).
- `OTEL_SERVICE_NAME` is the service name. Resource attributes include `service.namespace=encabulator`
  and `service.version`.
- W3C trace context over HTTP and NATS headers.
- Instrument inbound HTTP (server spans), outbound HTTP (client spans with `peer.service` or
  `server.address` set to the target service name), database and cache clients (`db.system`,
  `db.name` / `db.namespace`; the recorder's InfluxDB calls carry `db.system.name=influxdb`), and
  NATS (producer and consumer spans).
- Do not trace `/healthz`, `/readyz`, `/metrics`, or the malaise and flag polling loops.

### Malaise (fault injection) middleware

Every service in the processing, data, and support planes applies injected faults to its inbound
HTTP requests (excluding health and metrics) and, for message consumers, to message handling.

- Poll `GET ${MALAISE_ENGINE_URL}/v1/directives?service=${OTEL_SERVICE_NAME}` every 2s, untraced.
  If `MALAISE_ENGINE_URL` is unset, the middleware is disabled. If the engine is unreachable,
  keep applying the last directives until each one's `expiresAt`.
- Response shape:

```json
{
  "directives": [
    {"id": "mal_...", "kind": "latency",    "params": {"ms": 400, "jitterMs": 100, "percent": 100}, "expiresAt": "..."},
    {"id": "mal_...", "kind": "error",      "params": {"status": 503, "percent": 25},               "expiresAt": "..."},
    {"id": "mal_...", "kind": "congestion", "params": {"maxConcurrent": 2, "queueTimeoutMs": 2000}, "expiresAt": "..."},
    {"id": "mal_...", "kind": "cpu-burn",   "params": {"millicores": 500},                          "expiresAt": "..."}
  ]
}
```

- `latency`: for `percent`% of requests, sleep `ms ± jitterMs` before the handler.
- `error`: for `percent`% of requests, respond with `status` and
  `{"error":{"code":"malaise_injected","message":"..."}}` without calling the handler. For message
  handlers, `nak` the message instead.
- `congestion`: at most `maxConcurrent` requests in the handler at once; others wait up to
  `queueTimeoutMs`, then get 503 `congested`.
- `cpu-burn`: a background worker burns roughly `millicores` of CPU until the directive expires.
- Count applications in `encabulator_malaise_applied_total{kind}`.

The console, grammeter, malaise engine, and orchestrator do not apply malaise to themselves.

### Configuration

Environment variables only. Service URLs follow `<NAME>_URL`, for example
`HYDROCOPTIC_DATASTORE_URL=http://hydrocoptic-datastore:8080`. Secrets arrive as environment
variables from Kubernetes Secrets. Every service's README lists its variables.

### Containers

- Build context is the repository root (so kits can be copied in). The Dockerfile lives in the
  service directory.
- Multi-stage builds. Runtime images are minimal (distroless where the language allows) and run as
  UID/GID `65532`.
- Read-only root filesystem compatible. Write scratch data only to `/tmp` (an `emptyDir`).
- No `HEALTHCHECK` in Dockerfiles; Kubernetes probes own health.
- Multi-arch: `linux/amd64` and `linux/arm64`.
- Image: `${REGISTRY}/<service>:<version>`. Default registry
  `ghcr.io/jlbutler/cloud-native-encabulator`.

### Testing

- Unit tests for domain logic and handlers in every service; they run without containers.
- Integration tests use Testcontainers against real Postgres, Valkey, NATS, or InfluxDB, and skip
  cleanly when no container runtime is available.
- `make test` at the repository root runs every service's unit tests.

## 8. Service specifications

Paths below are relative to the service root. All request and response bodies are JSON.
Timestamps are RFC 3339 UTC. IDs are prefixed strings (`ord_`, `qte_`, `rsv_`, `mal_`, `run_`,
`inc_`, `shp_`) followed by a UUIDv7 or ULID.

### 8.1 dingle-arm-gateway (Go)

The drawn reciprocation dingle arm, employed whenever a barescent skor motion is required. The
only public entry point for customer traffic.

Routes (prefix `/api`):

| Method | Path | Auth | Upstream |
|---|---|---|---|
| POST | `/api/v1/auth/login` | public | detractor `POST /v1/auth/login` |
| POST | `/api/v1/auth/register` | public | detractor `POST /v1/auth/register` |
| GET | `/api/v1/users/me` | any role | detractor `GET /v1/users/me` |
| GET | `/api/v1/parts` | public | depot `GET /v1/parts` |
| POST | `/api/v1/quotes` | customer+ | quoter `POST /v1/quotes` |
| POST | `/api/v1/orders` | customer+ | processor `POST /v1/orders` |
| GET | `/api/v1/orders` | customer+ | datastore `GET /v1/orders` |
| GET | `/api/v1/orders/{id}` | customer+ | datastore `GET /v1/orders/{id}` |
| GET | `/api/v1/shipments/{orderId}` | customer+ | dispatch `GET /v1/shipments/{orderId}` |

- JWT: RS256, verified against `${UNILATERAL_PHASE_DETRACTOR_URL}/.well-known/jwks.json`, cached,
  refreshed every 5 minutes and on unknown `kid`. Check `exp`, `iss=unilateral-phase-detractor`,
  `aud=encabulator`.
- Rate limit: token bucket per subject (or per client IP from the first `X-Forwarded-For` hop for
  public routes), rate from flag `gateway-rate-limit-rps`, burst 2×. Buckets live in a bounded LRU
  (10,000 entries). Over the limit: 429 `rate_limited` with `Retry-After`.
- Proxying preserves method, path suffix, query, and body; adds identity headers; propagates trace
  context; streams responses.
- Metrics: `encabulator_gateway_requests_total{route,status}`, `encabulator_gateway_rate_limited_total`.
- Uses Go 1.22+ `net/http` routing patterns. No third-party router.

### 8.2 panametric-fam-processor (TypeScript)

Named for the panametric fam (Quick wrote "pentametric fan"; the Rockwell film says "panametric fam"). Orchestrates the order saga in section 4.1
and handles `inspection.failed`.

- `POST /v1/orders` body `{model, quantity, options: {dingleArm}}`; customer is
  `X-Encabulator-Subject`. Validation errors return 400 `invalid_order`. Returns 202 with the
  created order (the datastore's representation).
- `GET /v1/orders/{id}` is not implemented here; reads go to the datastore.
- Circuit breakers around quoter, depot, and datastore (open after 5 consecutive failures, half-open
  after 10s). An open breaker fails fast with 503 `dependency_unavailable`.
- Metrics: `encabulator_orders_total{model,result}` where result is `accepted`, `rejected`,
  `failed`; `encabulator_refabrications_total`; `encabulator_orders_scrapped_total`.
- Stack: Node 24, Fastify 5, `nats` (nats.js v2 or the current JetStream client), undici or
  native fetch, `prom-client`, `@opentelemetry/sdk-node`, `pino`.

### 8.3 spurving-bearing-quoter (Rust)

Aligns the spurving bearings with the pentametric fan, economically.

- `POST /v1/quotes` body `{model, quantity, options: {dingleArm}}` returns
  `{id, model, quantity, options, unitPrice, subtotal, surcharges: [{code, amount}], total,
  currency: "USD", computedAt, cached}`. Unknown model: 400 `unknown_model`.
- `GET /v1/quotes/{id}` returns a stored quote (Valkey, 1 hour TTL) or 404.
- Pricing: base price per model, plus dingle arm option, times quantity; volume discount 5% for
  quantity ≥ 5; turbo surcharge 15% when `turbo-encabulation` is on. Prices are rounded to cents.
- Pricing deliberately burns CPU: it computes a "pentametric harmonic" by iterating a numeric
  series (about 5 to 15 ms of CPU per quote depending on model and quantity). This makes the
  quoter the natural HPA target.
- With `quote-cache` on, identical `(model, quantity, options, turbo)` inputs are served from
  Valkey for 60s (`cached: true`). If Valkey is down, compute without the cache; readiness stays
  green because Valkey is a soft dependency.
- Metrics: `encabulator_quotes_total{model,cached}`, `encabulator_quote_compute_seconds` histogram.
- Stack: Rust stable, axum, tokio, `redis` crate (Valkey-compatible), `opentelemetry` +
  `opentelemetry-otlp`, `tracing` with JSON output, `prometheus` or `metrics-exporter-prometheus`.

### 8.4 logarithmic-casing-line (Python)

The malleable logarithmic casing surmounts everything. Fabrication workers.

- Consumes `fabrication.requested` per section 4.2. Runs up to `LINE_CAPACITY` orders
  concurrently per pod (asyncio). Units within an order are built sequentially.
- For each stage: sleep the stage duration, generate readings (section 4.3; defect probability
  from the model's base rate, reduced by `dingle-arm-damping` when `dingleArm` is set), write them
  in one `POST sinusoidal-recorder /v1/readings`, and publish `fabrication.stage`.
- Malaise applies per message (latency slows the line; error naks the message).
- HTTP API: `GET /v1/line` returns `{capacity, activeOrders, activeUnits: [{orderId, serial, stage}],
  unitsCompleted}` for this pod.
- Metrics: `encabulator_units_fabricated_total{model}`, `encabulator_stage_duration_seconds{stage}`
  histogram, `encabulator_line_active_orders` gauge.
- Stack: Python 3.14, FastAPI, uvicorn, `nats-py`, httpx, `opentelemetry-*`, `prometheus-client`,
  `numpy` for signal generation.

### 8.5 nofer-trunnion-dispatch (TypeScript)

For the operation of nofer trunnions (Time, 1946).

- Consumes `inspection.passed`. Chooses a carrier (`Trunnion Freight`, `Waneshaft Express`,
  `Grammeter Parcel`, `Tremie Pipe & Sons`), generates a tracking number (`NT` + 10 digits),
  stores the shipment in Valkey, schedules delivery 10 to 60s later in a Valkey sorted set, and
  publishes `shipment.dispatched`. A 1s loop publishes `shipment.delivered` for due shipments
  (claim each with an atomic `ZREM` so replicas do not double-deliver).
- `GET /v1/shipments/{orderId}` returns the shipment or 404; `GET /v1/shipments?limit=50` returns
  recent shipments.
- Metrics: `encabulator_shipments_total{carrier,status}`, `encabulator_shipments_in_transit` gauge.
- Stack: same as the processor, plus `ioredis`.

### 8.6 hydrocoptic-datastore (Go)

Six hydrocoptic marzlevanes of pure persistence. The order ledger.

- `POST /v1/orders` body `{id, customer, model, quantity, options, quote: {id, total, currency},
  reservationId}` creates an order with status `accepted`. Idempotent on `id` (a replay returns
  the existing order with 200; a new order returns 201).
- `GET /v1/orders?customer=&status=&limit=50&before=` lists orders, newest first. When
  `X-Encabulator-Roles` lacks `operator`, `viewer`, or `admin`, results are filtered to
  `customer = X-Encabulator-Subject` regardless of the query.
- `GET /v1/orders/{id}` returns the order with `timeline: [{time, type, detail}]` (same visibility
  rule; 404 if not visible).
- `GET /v1/stats` returns counts by status and throughput (orders accepted, units delivered) over
  the last 5 minutes.
- Order JSON: `{id, customer, model, quantity, options, status, stage, attempt, quote, reservationId,
  serials, carrier, trackingNumber, createdAt, updatedAt}`.
- Consumes `encabulator.>` and applies events idempotently (dedupe on event id in a
  `processed_events` table) to update status per section 3. Status transitions are monotonic by
  rank, except `inspecting → refabricating → fabricating`.
- Schema via embedded, versioned SQL migrations applied at startup (`golang-migrate` or a minimal
  in-house runner).
- Readiness: Postgres and NATS.
- Stack: Go, `pgx/v5` with `otelpgx`, `nats.go` JetStream API.

### 8.7 prefabulated-amulite-depot (TypeScript)

The base-plate of prefamulated amulite, and everything else in the parts cage.

- `GET /v1/parts` returns `[{sku, name, stock, reserved, perUnit}]`.
- `POST /v1/reservations` body `{orderId, model, quantity, options}` computes the bill of materials
  and atomically reserves every SKU (one Lua script: check all, then decrement all). Returns 201
  `{id, orderId, items, expiresAt}` or 409 `insufficient_parts` with the short SKUs. Idempotent on
  `orderId`.
- `POST /v1/reservations/{id}/commit` marks it consumed. `POST /v1/reservations/{id}/release`
  returns stock. Uncommitted reservations expire after 15 minutes and are returned by a sweeper.
- `POST /v1/parts/{sku}/restock` body `{quantity}` (operator) adds stock.
- The amulite foundry: every 10s, add `amulite-foundry-rate` units of each SKU (`HYD-MARZ` gets 6×),
  up to a cap of 5,000 per SKU. Initial stock is 1,000 per SKU (6,000 marzlevanes).
- Metrics: `encabulator_parts_stock{sku}` gauge, `encabulator_reservations_total{result}`.
- Readiness: Valkey.

### 8.8 sinusoidal-recorder (Go)

Records sinusoidal repleneration so that it can be reduced.

- `POST /v1/readings` body `{readings: [{serial, orderId, model, stage, metric, value, time}]}`
  writes a batch (up to 1,000 points) to InfluxDB table `readings` (tags: `serial`, `order_id`,
  `model`, `stage`, `metric`; field `value`).
- `GET /v1/readings?serial=...` returns `{serial, readings: [...]}` ordered by time. Uses
  parameterized SQL queries; never interpolates input into query text.
- `GET /v1/summary?window=5m` returns per-metric aggregates across the factory (count, mean, p95)
  for the console's gauges.
- Readiness: InfluxDB.
- Stack: Go, `github.com/InfluxCommunity/influxdb3-go`.

### 8.9 unilateral-phase-detractor (Python)

Supplies inverse reactive credentials for use in unilateral phase detractors.

- `POST /v1/auth/login` body `{username, password}` returns
  `{accessToken, tokenType: "Bearer", expiresIn, user}`. Tokens are RS256 JWTs with `sub`, `roles`,
  `iss=unilateral-phase-detractor`, `aud=encabulator`, `exp` (1 hour), `kid`.
- `POST /v1/auth/register` body `{username, password, displayName}` creates a `customer`. Rate
  limited per IP (in process). Usernames: 3 to 64 chars `[a-z0-9._-]`.
- `GET /v1/users/me` (from `X-Encabulator-Subject`, or from a bearer token when called directly).
- `GET /v1/users`, `POST /v1/users`, `PATCH /v1/users/{username}`, `DELETE /v1/users/{username}`:
  admin only. `roles` is always an array.
- `GET /v1/audit?limit=100`: admin only. Login successes and failures, user changes.
- `GET /.well-known/jwks.json`.
- Passwords hashed with Argon2id (`argon2-cffi`). The signing key comes from `JWT_PRIVATE_KEY`
  (PEM). Bootstrap users come from `BOOTSTRAP_ADMIN_PASSWORD` and `BOOTSTRAP_OPERATOR_PASSWORD`
  (users `admin` and `operator`); no default passwords exist in code.
- Readiness: Postgres.
- Stack: Python 3.14, FastAPI, SQLAlchemy 2 async with `asyncpg`, PyJWT with `cryptography`.

### 8.10 differential-girdlespring (Go)

The differential girdlespring on the "up" end of the grammeters. Feature flags.

- `GET /v1/flags` returns `{flags: [...]}` with an `ETag`; honors `If-None-Match` (304).
- `GET /v1/flags/{key}`; `GET /v1/flags/{key}/evaluate?subject=` returns `{key, value, reason}`.
- `PUT /v1/flags/{key}` body `{value, rolloutPercent?, description?}` (operator or admin). Validates
  type. Records `updatedBy` and `updatedAt`.
- Flag JSON: `{key, type, value, rolloutPercent, description, updatedAt, updatedBy}`.
- Seeds the flags in section 6 on first start (insert if missing; never overwrite).
- Readiness: Postgres.

### 8.11 side-fumbling-detector (Python)

So fitted that side fumbling is effectively prevented, or at least detected.

- Consumes `fabrication.completed`, inspects every serial per section 4.3, publishes the result.
- `GET /v1/incidents?limit=50` returns recent incidents `[{id, orderId, serial, stage, metric,
  score, reason, strict, time}]` (Valkey list capped at 500).
- `GET /v1/stats` returns `{inspected, passed, failed, failRate5m}`.
- Metrics: `encabulator_units_inspected_total{result}`, `encabulator_anomaly_score` histogram.
- Stack: Python 3.14, FastAPI, `nats-py`, httpx, `numpy`, `redis` (async), OpenTelemetry.

### 8.12 cardinal-grammeter (Go)

Automatically synchronizes cardinal grammeters, which is to say it generates customer traffic.

- Runs a pool of synthetic customers (default 50) that register (idempotently) and log in through
  the gateway, then perform weighted journeys:

  | Journey | Default weight | Calls |
  |---|---|---|
  | `browse` | 40 | `GET /api/v1/parts` |
  | `quote` | 25 | `POST /api/v1/quotes` |
  | `order` | 20 | `POST /api/v1/orders` |
  | `track` | 15 | `GET /api/v1/orders?limit=5`, then `GET /api/v1/orders/{id}` and shipments for one |

  Models are chosen 60% turbo, 25% retro, 15% hyper; quantity 1 to 3 (occasionally up to 8);
  dingle arm 30%.
- `GET /v1/traffic` returns config plus live stats: target and actual RPS, in-flight, and per
  journey `{count, errors, p50Ms, p99Ms}` over the last 60s, and status code counts.
- `PUT /v1/traffic` body (all optional):
  `{running, rps (0.1-500), pattern: "steady"|"burst"|"ramp"|"sine", periodSeconds, mix: {...},
  customers}`.
  - `steady`: constant rate.
  - `sine`: rate × (1 + 0.8·sin(2πt/period)) (the modial interaction of magneto-reluctance).
  - `burst`: rate for 80% of each period, 5× rate for 20% (capacitive diractance).
  - `ramp`: 0 to rate linearly over the period, then hold.
- Scheduling uses a fractional accumulator on a 50ms tick; in-flight capped at `MAX_IN_FLIGHT`
  (default 256); excess is counted as `dropped`, not queued.
- The target is fixed by `DINGLE_ARM_GATEWAY_URL`. There is no API to change it.
- Starts stopped unless `AUTOSTART=true` (with `AUTOSTART_RPS`, default 5). Runs as a single
  replica.
- Metrics: `encabulator_grammeter_requests_total{journey,status}`, `encabulator_grammeter_target_rps`.

### 8.13 malaise-injection-engine (Go)

Induces malaise for the purpose of demonstrating that the encabulator recovers from it.

- `POST /v1/malaises` body `{target, kind, params, durationSeconds}` (1 to 900s, default 120).
  Returns 201 with the malaise. `target` must be a service in the catalog or, for Kubernetes-level
  kinds, any workload labeled `encabulator.io/malaise-target=true` (backing stores included).
- `GET /v1/malaises?state=active|all` lists malaises; `DELETE /v1/malaises/{id}` ends one;
  `DELETE /v1/malaises` ends all.
- `GET /v1/directives?service=` returns active app-level directives for the middleware.
- App-level kinds: `latency`, `error`, `congestion`, `cpu-burn` (params per section 7).
- Kubernetes-level kinds:
  - `pod-kill` `{count}`: delete `count` random ready pods of the target (instant; the malaise
    records what it killed).
  - `partition` `{from}`: create a NetworkPolicy selecting the target's pods that allows ingress
    from everything except pods labeled `app.kubernetes.io/name` in `from` (or from nothing but
    the observability namespace when `from` is `"*"`). Deleted on expiry.
  - `scale` `{replicas}`: set the target Deployment's replicas, remembering the original in an
    annotation; restored on expiry.
- State lives in Valkey (malaise records) and in the Kubernetes objects themselves (labels
  `encabulator.io/malaise-id`, annotation `encabulator.io/expires-at`). A 2s reconcile loop expires
  malaises and reverts Kubernetes changes, so a restart never strands a fault.
- RBAC: a namespaced Role in `encabulator` only: pods get/list/delete, deployments get/list/patch,
  deployments/scale get/patch, networkpolicies get/list/create/delete.
- Metrics: `encabulator_malaises_active{kind}`, `encabulator_malaises_total{kind,target}`.

### 8.14 demo-scenario-orchestrator (Python)

Rehearses the demo so the presenter does not have to.

- Scenarios are YAML files (built-ins in the image, extras mounted from a ConfigMap at
  `/etc/scenarios`). A scenario is `{id, title, description, steps: [...]}`. Step types:
  - `say: "text"`: narration, shown on the console.
  - `traffic: {...}`: `PUT` to the grammeter.
  - `malaise: {...}`: `POST` to the engine; `clearMalaise: all | <step-ref>`.
  - `flag: {key, value}`: `PUT` to girdlespring (restored at the end of the run unless `keep: true`).
  - `wait: 30s`.
  - `waitFor: {promql, op, value, timeout}`: poll Prometheus until a condition holds.
- `GET /v1/scenarios`, `POST /v1/scenarios/{id}/runs` (202 with a run; 409 if a run is active),
  `GET /v1/runs?limit=20`, `GET /v1/runs/{id}` (status, current step, log with narration),
  `POST /v1/runs/{id}/cancel` (reverts flags and clears malaises it created).
- Built-in scenarios: `morning-shift` (ramp up traffic), `marzlevane-shortage` (stop the foundry,
  watch 409s, restart), `hydrocoptic-outage` (kill Postgres, watch recovery), `side-fumbling-outbreak`
  (slow the casing line and tighten inspection), `dingle-arm-congestion` (congest the gateway under
  burst traffic), and `grand-tour` (all of the above, narrated in full encabulator prose).
- Stack: Python 3.14, FastAPI, httpx, PyYAML. Runs execute as asyncio tasks, never inside a request.

### 8.15 encabulation-control-console (TypeScript)

The instrument panel. A Vue 3 single-page app served by a Fastify backend-for-frontend (BFF) in one
container.

**BFF** (`/bff/*`):

- Session: `POST /bff/login` (calls the detractor, stores the JWT in an `httpOnly`, `SameSite=Strict`
  cookie), `POST /bff/logout`, `GET /bff/session`. Unauthenticated visitors are `guest` with the
  `viewer` role.
- Every mutating route requires `operator` or `admin`, enforced in the BFF. User management requires
  `admin`.
- Live stream: `GET /bff/stream` (Server-Sent Events) pushes `topology` every 3s and `feed` events
  (recent orders, incidents, shipments, scenario narration) as they are polled. Polling of upstreams
  runs only while at least one client is connected.
- Proxies with role checks: traffic (grammeter), malaises (engine), scenarios and runs
  (orchestrator), flags (girdlespring), parts and restock (depot), orders and stats (datastore),
  incidents and stats (detector), shipments (dispatch), telemetry summary (recorder),
  users and audit (detractor).
- Storefront: `POST /bff/storefront/orders` and `POST /bff/storefront/quotes` place real orders
  through the gateway using the signed-in user's token, so a person in a browser drives the same
  path as the grammeter.
- Kubernetes (read only, plus nothing): list pods and deployments labeled
  `app.kubernetes.io/part-of=encabulator`. The console does not mutate the cluster; it asks the
  malaise engine to.
- All upstream URLs, the Prometheus URL, and the external Grafana and Jaeger links come from
  environment variables.

**Topology model** (`topology` event):

- Nodes: the 15 services and 4 backing stores from a static registry (name, display name, plane,
  language, kind), merged with live pods from the Kubernetes API (name, phase, ready, restarts)
  and per-pod request rate, p50, p99, and error ratio from Prometheus
  (`http_server_request_duration_seconds` by `pod`). A service node's figures roll up from its
  pods: summed rate, rate-weighted p50, worst pod p99, summed errors over summed rate.
- Edges: from the OTel service graph metrics in Prometheus
  (`traces_service_graph_request_total`, `traces_service_graph_request_failed_total`,
  `traces_service_graph_request_server_seconds_bucket`) by `client`, `server` and
  `connection_type`, over a 1 minute window: rate, error ratio, p50, p99. Database and cache edges
  come from client spans with `db.system.name` or `db.system` (`connection_type="database"`; the
  latency is the client span's). A messaging edge is a `producer → consumer` pair
  (`connection_type="messaging_system"`) and is drawn through the NATS node as two legs:
  `producer → nats` with the producer's latency (`traces_service_graph_request_client_seconds_bucket`)
  and `nats → consumer` with the consumer's. Pairs that land on the same leg add their rates and
  failures and keep the worse latency; the connector does not say which side of a failed pair
  failed, so it counts against both legs. The connector pairs each message with one consumer, so a
  message several services consume can under-count the delivery legs; nothing is added to make
  up for it.
- Store figures: a backing store is not instrumented and serves no HTTP metrics, so its node's rate,
  p50, p99 and error ratio are those of the edges into it (for NATS, the publish legs): summed
  rate, rate-weighted p50, worst p99, summed failures over summed rate, and the health colour from
  those. A store plate therefore reads "no data" only when no edge into it carries a value.
- An edge with no traffic is drawn idle. A missing metric is shown as "no data", never invented.

**UI**:

- *Factory Floor*: animated topology (particles proportional to edge RPS, colored by health: green
  under 100ms p99 and under 1% errors, yellow under 500ms, red otherwise, gray when no data), pods
  as dots on each node, hover for details, plus headline stats (orders/min, units in fabrication,
  refabrication rate, stock levels) and the scenario narration ticker.
- *Grammeters*: traffic controls (rate, pattern, period, journey mix) and live journey stats.
- *Malaise*: inject faults by target and kind, list active malaises with countdowns, clear.
- *Scenarios*: list, run, cancel, follow narration and step progress. The live stream carries the
  orchestrator's latest run whatever its status; only a `running` (or `pending`) run counts as
  active, locks the Run buttons and can be cancelled. A completed, failed or cancelled run is shown
  as the last run with its final status, step and narration, and does not block the next run. A
  cancel that fails, or finds the run already over, is reported on the page. The orchestrator
  reports steps as numbers (`currentStep`, `totalSteps`), shown as "24 of 50", and a failed run's
  `error` is shown as the reason it stopped. A page opened after a run ended reads the narration
  from the run itself, since the feed only carries lines narrated while the page was open.
- *Girdlespring*: view and edit flags.
- *Works*: storefront (quote, order, track) plus live order, shipment, and incident feeds, and the
  parts cage with restock.
- *Administration*: users and audit (admin).
- Links out to Grafana and Jaeger.
- Visual style: a 1940s industrial instrument panel (cream enamel, brass, green phosphor gauges)
  with dark mode. Animations pause when the tab is hidden; particles are capped and scale by
  opacity.

**Stack**: Node 24, Vue 3.5+, Vite, Pinia, Vue Router, Tailwind CSS 4, d3 for the topology,
Fastify 5 with `@fastify/static`, `@fastify/cookie`, `@kubernetes/client-node` 1.x, Vitest, and
Playwright for a smoke test.

## 9. Observability platform

Installed into the `encabulator-observability` namespace from pinned upstream charts:

- **kube-prometheus-stack**: Prometheus, Alertmanager, Grafana, kube-state-metrics, node exporter.
  Prometheus selects `ServiceMonitor` and `PodMonitor` objects labeled `encabulator.io/scrape=true`.
- **OpenTelemetry Collector** (deployment mode): OTLP receiver; `k8sattributes` and `batch`
  processors; `servicegraph` and `spanmetrics` connectors; exports traces to Jaeger over OTLP and
  exposes connector metrics on a Prometheus endpoint that Prometheus scrapes.
- **Jaeger v2** (all-in-one with in-memory or Badger storage for v1).
- **Grafana dashboards** provisioned from ConfigMaps: *Encabulator Works* (business flow), *Service
  RED* (per service), and *Backing Stores*.
- **Alerts**: PrometheusRules for high error ratio, high p99, casing line backlog, stock depletion,
  and pod crash loops.

**KEDA** (`keda` namespace) scales `logarithmic-casing-line` on JetStream consumer lag.
`spurving-bearing-quoter` and `panametric-fam-processor` use CPU-based HPAs. Every service has a
PodDisruptionBudget when it runs more than one replica.

## 10. Deployment (v1)

One Helm chart, `charts/encabulator`, deploys the application and its backing stores. The
observability platform and KEDA are installed first from upstream charts with values in
`platform/`. The target is any conformant Kubernetes cluster with a default StorageClass; the
reference target is Amazon EKS (Auto Mode or managed node groups), and kind works for development.

```bash
make platform          # observability + KEDA, pinned chart versions
make deploy            # charts/encabulator into namespace encabulator
make console           # port-forward the console to http://localhost:8080
```

Chart structure:

- `values.yaml` holds a `services:` map with one entry per service (image, replicas, resources,
  env, autoscaling, which backing stores it needs). A shared set of templates renders every service
  the same way: Deployment, Service, ServiceAccount, ServiceMonitor, HPA, PDB.
- Backing stores are StatefulSets in the same chart (`postgres`, `valkey`, `nats`, `influxdb`),
  each individually disableable with an `external:` block for later phases.
- Secrets (database passwords, the JWT signing key, bootstrap passwords) are generated on first
  install with `randAlphaNum` and `genPrivateKey`, and preserved on upgrade with `lookup`.
- Every pod runs with `runAsNonRoot`, `readOnlyRootFilesystem`, all capabilities dropped,
  `seccompProfile: RuntimeDefault`, and no service account token unless it needs the Kubernetes API
  (console, malaise engine).
- Labels: `app.kubernetes.io/name`, `app.kubernetes.io/part-of=encabulator`,
  `app.kubernetes.io/version`, `encabulator.io/plane`, `encabulator.io/malaise-target=true`.
- The console is exposed by `ClusterIP` plus port-forward by default, with an optional `Ingress`
  (class configurable; `alb` on EKS).

## 11. Repository layout

```
charts/encabulator/          Helm chart for the application and backing stores
platform/                    Values and dashboards for upstream platform charts
libs/go/kit/                 Go service kit
libs/ts/kit/                 TypeScript service kit (@encabulator/kit)
libs/python/encabulator-kit/ Python service kit
services/<plane>-plane/<service>/
docs/                        Design, operations, glossary
scripts/                     Small helpers invoked by the Makefile
Makefile                     build, test, lint, images, platform, deploy
```

Go services reference the kit with a `replace` directive. TypeScript services reference it as a
`file:` dependency. Python services reference it as a uv path source.

## 12. Roadmap

Each phase is a tag on `main` with a working system.

| Tag | Phase | What changes |
|---|---|---|
| `v1.x` | Self-managed | Everything in this document, deployed with Helm |
| `v2.x` | GitOps | Argo CD reconciles the platform and application from this repository |
| `v3.x` | Managed services | ACK provisions RDS, ElastiCache, and Timestream for InfluxDB; the chart's `external:` blocks point at them |
| `v4.x` | Composition | kro packages each service and its AWS dependencies behind one `Encabulator*` custom resource |
| later | Cells | Multiple cells across accounts and regions, with progressive rollouts between them |

## Appendix A. Environment variables

Common to every service:

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTEL_SERVICE_NAME` | service name | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | e.g. `http://otel-collector.encabulator-observability:4318`; unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables the malaise middleware |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | Flag clients use defaults when unset |
| `NATS_URL` | `nats://nats:4222` | Only services that use NATS |

Service URLs are `<SERVICE_NAME_IN_UPPER_SNAKE>_URL`, for example
`SPURVING_BEARING_QUOTER_URL=http://spurving-bearing-quoter:8080`.

Backing stores:

| Variable | Example | Used by |
|---|---|---|
| `DATABASE_URL` | `postgres://hydrocoptic:<pw>@postgres:5432/hydrocoptic?sslmode=disable` | datastore, detractor, girdlespring |
| `VALKEY_URL` | `redis://default:<pw>@valkey:6379/0` | depot, quoter, dispatch, detector, malaise engine |
| `INFLUXDB_URL` | `http://influxdb:8181` | recorder |
| `INFLUXDB_TOKEN` | secret, may be empty | recorder |
| `INFLUXDB_DATABASE` | `sinusoidal` | recorder |

The chart builds `DATABASE_URL` and `VALKEY_URL` with Kubernetes `$(VAR)` expansion from secret-backed
`PGPASSWORD` and `VALKEY_PASSWORD` variables.

Every Valkey client accepts `rediss://` as well as `redis://`. TLS verifies the server certificate
against the client's default root store: the image's system store for Go, Rust and Python, and Node's
bundled store for TypeScript. An external Valkey such as ElastiCache Serverless, which only
accepts TLS and, without a user group, has no password, gets `rediss://:@<host>:6379/0`
(`valkey.external.tls=true`, empty `valkey.user` and password); with that shape no client sends AUTH.

Service-specific:

| Service | Variables |
|---|---|
| unilateral-phase-detractor | `JWT_PRIVATE_KEY` (PEM), `JWT_TTL_SECONDS` (3600), `BOOTSTRAP_ADMIN_PASSWORD`, `BOOTSTRAP_OPERATOR_PASSWORD` |
| logarithmic-casing-line | `LINE_CAPACITY` (2), `SINUSOIDAL_RECORDER_URL`, `PREFABULATED_AMULITE_DEPOT_URL` |
| cardinal-grammeter | `DINGLE_ARM_GATEWAY_URL`, `CUSTOMERS` (50), `CUSTOMER_PASSWORD_SEED` (secret; synthetic customer passwords are derived from it), `MAX_IN_FLIGHT` (256), `AUTOSTART` (false), `AUTOSTART_RPS` (5) |
| malaise-injection-engine | `VALKEY_URL`, `POD_NAMESPACE` (downward API), `OBSERVABILITY_NAMESPACE` (`encabulator-observability`) |
| demo-scenario-orchestrator | `CARDINAL_GRAMMETER_URL`, `MALAISE_ENGINE_URL`, `DIFFERENTIAL_GIRDLESPRING_URL`, `PREFABULATED_AMULITE_DEPOT_URL`, `PROMETHEUS_URL`, `SCENARIO_DIR` (`/etc/scenarios`) |
| encabulation-control-console | every service URL, `PROMETHEUS_URL`, `GRAFANA_EXTERNAL_URL`, `JAEGER_EXTERNAL_URL`, `SESSION_SECRET`, `POD_NAMESPACE`, `COOKIE_SECURE` (false) |

## Appendix B. Service directories

| Service | Directory |
|---|---|
| encabulation-control-console | `services/control-plane/encabulation-control-console` |
| cardinal-grammeter | `services/control-plane/cardinal-grammeter` |
| malaise-injection-engine | `services/control-plane/malaise-injection-engine` |
| demo-scenario-orchestrator | `services/control-plane/demo-scenario-orchestrator` |
| dingle-arm-gateway | `services/processing-plane/dingle-arm-gateway` |
| panametric-fam-processor | `services/processing-plane/panametric-fam-processor` |
| spurving-bearing-quoter | `services/processing-plane/spurving-bearing-quoter` |
| logarithmic-casing-line | `services/processing-plane/logarithmic-casing-line` |
| nofer-trunnion-dispatch | `services/processing-plane/nofer-trunnion-dispatch` |
| hydrocoptic-datastore | `services/data-plane/hydrocoptic-datastore` |
| prefabulated-amulite-depot | `services/data-plane/prefabulated-amulite-depot` |
| sinusoidal-recorder | `services/data-plane/sinusoidal-recorder` |
| unilateral-phase-detractor | `services/support-plane/unilateral-phase-detractor` |
| differential-girdlespring | `services/support-plane/differential-girdlespring` |
| side-fumbling-detector | `services/support-plane/side-fumbling-detector` |

Every service directory contains `README.md`, `Dockerfile`, and a `Makefile` with the targets
`build`, `test`, `lint`, and `run`. The root Makefile iterates over them.
