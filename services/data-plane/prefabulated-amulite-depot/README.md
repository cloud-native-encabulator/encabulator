# prefabulated-amulite-depot

> "The original machine had a base-plate of prefamulated amulite..."

The base-plate of prefamulated amulite, and everything else in the parts cage. The depot is
the data-plane service that knows how many marzlevanes are on the shelf, hands out
all-or-nothing reservations against them, and runs the amulite foundry that keeps the works
supplied. When the factory runs dry, this is the service that says so: a `409
insufficient_parts` here is the reason an order gets rejected before it ever reaches the
ledger.

- Plane: `data` · Language: TypeScript (Node 24, Fastify 5) · Owns Valkey keys `depot:*`
- Design: [section 8.7](../../../docs/design.md#87-prefabulated-amulite-depot)

## What it does

**Reservations are atomic.** One Lua script checks every SKU in the bill of materials and
only then decrements them all, so a reservation either takes every part it needs or takes
nothing. Twelve concurrent orders against five base-plates produce five reservations and
seven refusals, never a negative shelf.

**Reservations are idempotent on `orderId`.** The processor can retry a POST without
double-reserving; a replay returns the existing reservation with `200` instead of `201`.

**Reservations expire.** An uncommitted reservation is pending for fifteen minutes. A
sweeper runs every thirty seconds, claims each due reservation with an atomic `ZREM` so
replicas do not sweep the same one twice, and returns the parts to stock.

**The amulite foundry.** Every ten seconds the depot adds `amulite-foundry-rate` units of
each SKU, six times that for `HYD-MARZ`, up to a ceiling of 5,000 per SKU. Setting the flag
to `0` stops the foundry, which is how the `marzlevane-shortage` scenario starves the
factory on demand.

## API

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/v1/parts` | any | `[{sku, name, stock, reserved, perUnit}]` |
| POST | `/v1/reservations` | internal | Reserve a bill of materials |
| GET | `/v1/reservations/{id}` | internal | Fetch a reservation |
| POST | `/v1/reservations/{id}/commit` | internal | Mark the parts consumed |
| POST | `/v1/reservations/{id}/release` | internal | Put the parts back |
| POST | `/v1/parts/{sku}/restock` | `operator` or `admin` | Add stock by hand |
| GET | `/healthz`, `/readyz`, `/metrics` | any | Liveness, readiness (Valkey), Prometheus |

### `POST /v1/reservations`

```json
{ "orderId": "ord_018f...", "model": "turbo", "quantity": 2, "options": { "dingleArm": true } }
```

`201` (or `200` on an idempotent replay):

```json
{
  "id": "rsv_018f...",
  "orderId": "ord_018f...",
  "model": "turbo",
  "quantity": 2,
  "options": { "dingleArm": true },
  "items": [{ "sku": "AMU-BASE", "quantity": 2 }, { "sku": "HYD-MARZ", "quantity": 12 }],
  "status": "pending",
  "createdAt": "2026-09-30T12:00:00.000Z",
  "expiresAt": "2026-09-30T12:15:00.000Z"
}
```

`409` when the cage is short, naming every SKU that came up short:

```json
{
  "error": {
    "code": "insufficient_parts",
    "message": "The parts cage is short; the foundry has been notified.",
    "short": [{ "sku": "HYD-MARZ", "available": 1, "needed": 6 }]
  }
}
```

`400 invalid_reservation` for an unknown model or a quantity outside 1 to 10.

Reservation status moves `pending` to one of `committed`, `released` or `expired`, and never
moves again. Committing or releasing an already-settled reservation returns `409
reservation_not_pending`; repeating the same transition is a no-op that returns `200`.

## Parts catalog

Seeded on first start, 1,000 of each SKU and 6,000 marzlevanes. Never overwritten on
restart.

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
| `DNG-ARM` | Drawn reciprocation dingle arm | 1, only with `options.dingleArm` |

## Valkey keyspace

| Key | Type | Purpose |
|---|---|---|
| `depot:stock:<sku>` | string (integer) | Units on the shelf |
| `depot:reserved:<sku>` | string (integer) | Units reserved, not yet consumed |
| `depot:reservation:<id>` | string (JSON) | The reservation record, 24 h TTL |
| `depot:order:<orderId>` | string | Idempotency index, dropped on release |
| `depot:pending` | sorted set | Pending reservation ids scored by expiry, read by the sweeper |

`reserve` declares every key it touches in `KEYS`. `commit` and `release` do the same, and
use `cjson` only to read the current status before applying the transition, so a concurrent
duplicate call can never double-count.

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `VALKEY_URL` | **required** | `redis://default:<pw>@valkey:6379/0` |
| `OTEL_SERVICE_NAME` | `prefabulated-amulite-depot` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables fault injection |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | Unset means the foundry runs at the default rate of 20 |
| `SERVICE_VERSION` | `1.0.0` | Trace resource attribute |

## Feature flags read

| Flag | Default | Effect |
|---|---|---|
| `amulite-foundry-rate` | `20` | Units of each SKU poured every 10 s; `0` stops the foundry |

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_parts_stock` | gauge | `sku` |
| `encabulator_parts_reserved` | gauge | `sku` |
| `encabulator_reservations_total` | counter | `result` (`created`, `duplicate`, `insufficient_parts`, `invalid`, `committed`, `released`, `expired`) |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `encabulator_malaise_applied_total` | counter | `kind` |

Plus the Node process and runtime defaults.

## Running locally

```bash
source ../../../dev-env.sh
docker run -d --name depot-valkey -p 6379:6379 valkey/valkey:9

make build
VALKEY_URL=redis://127.0.0.1:6379/0 LOG_LEVEL=debug make run

curl -s localhost:8080/v1/parts | jq
curl -s -XPOST localhost:8080/v1/reservations \
  -H 'content-type: application/json' \
  -d '{"orderId":"ord_local","model":"turbo","quantity":2,"options":{"dingleArm":true}}' | jq
```

## Tests

```bash
make test-unit   # catalog and bill-of-materials logic, no containers
make test        # adds the Valkey suite: Lua atomicity, concurrency, sweeper, foundry, HTTP
```

The Valkey suite uses Testcontainers and skips cleanly when `DOCKER_HOST` is unset.

## Container

```bash
make image   # build context is the repository root
```

Multi-stage: the kit is built first, then the service, then production dependencies only.
Runtime is `gcr.io/distroless/nodejs24-debian13` as UID 65532, read-only-root-filesystem
compatible (nothing is written outside `/tmp`).
