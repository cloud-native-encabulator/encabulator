# hydrocoptic-datastore

> Six hydrocoptic marzlevanes, fitted to the ambifacient lunar waneshaft, of pure persistence.

The order ledger and the system of record for the Encabulator Works. Orders arrive over HTTP from
the panametric fam processor; everything that happens to them afterwards arrives as events on the
`ENCABULATOR` JetStream stream. Plane: data. Language: Go. Design reference:
[section 8.6](../../../docs/design.md).

The ledger is the one place that has to be right when everything else is being deliberately broken,
so it is built to be dull: every write is idempotent, every status transition moves forward, and
every event is applied exactly once.

## API

| Method | Path | Description |
|---|---|---|
| POST | `/v1/orders` | Create an order with status `accepted`; idempotent on `id` |
| GET | `/v1/orders` | List orders, newest first, with `?customer=&status=&limit=&before=` |
| GET | `/v1/orders/{id}` | One order plus its `timeline` |
| GET | `/v1/stats` | Counts by status and 5-minute throughput |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness: Postgres and NATS |
| GET | `/metrics` | Prometheus text |

### POST /v1/orders

```json
{
  "id": "ord_01J...",
  "customer": "wendy",
  "model": "turbo",
  "quantity": 2,
  "options": {"dingleArm": true},
  "quote": {"id": "qte_01J...", "total": 4200.00, "currency": "USD"},
  "reservationId": "rsv_01J..."
}
```

`201` with the created order, or `200` with the stored order when the id has been seen before · the
processor can retry safely. Validation failures are `400 invalid_order`: `model` must be `turbo`,
`retro` or `hyper`, and `quantity` must be 1 to 10.

### Order JSON

```json
{
  "id": "ord_01J...", "customer": "wendy", "model": "turbo", "quantity": 2,
  "options": {"dingleArm": true}, "status": "fabricating",
  "stage": "align-spurving-bearings", "attempt": 1,
  "quote": {"id": "qte_01J...", "total": 4200.00, "currency": "USD"},
  "reservationId": "rsv_01J...", "serials": ["ENC-turbo-m4x1-9f3a"],
  "carrier": "", "trackingNumber": "",
  "createdAt": "2026-09-30T12:00:00.000Z", "updatedAt": "2026-09-30T12:00:04.120Z",
  "timeline": [{"time": "...", "type": "encabulator.fabrication.stage", "detail": "Stage 3/7 ..."}]
}
```

`timeline` appears on the single-order view only.

### Visibility

A caller without `operator`, `viewer` or `admin` in `X-Encabulator-Roles` only ever sees its own
orders, whatever the query string says: `GET /v1/orders` is forced to `customer = X-Encabulator-Subject`,
and `GET /v1/orders/{id}` on someone else's order returns `404`, not `403`, so order ids are not
probeable.

### Pagination

`limit` is 1 to 200 (default 50). `before` is an exclusive cursor and accepts either an order id or
an RFC 3339 timestamp; the response's `nextBefore` is the id to pass for the following page, and is
absent on the last page. An unknown cursor is treated as the end of the list.

### GET /v1/stats

```json
{
  "counts": {"accepted": 3, "fabricating": 12, "delivered": 140},
  "total": 155,
  "windowSeconds": 300,
  "throughput": {"ordersAccepted": 22, "unitsDelivered": 48}
}
```

`unitsDelivered` sums the quantity of every order whose delivery landed inside the window, read from
the timeline so a redelivered event cannot double-count.

## Events

The service consumes the whole stream on durable `hydrocoptic-datastore`, filter `encabulator.>`,
explicit ack, `AckWait` 30s, `MaxDeliver` 5. It reacts to:

| Subject | Effect on the order |
|---|---|
| `order.accepted` | `accepted`; creates the row if the POST has not landed yet |
| `fabrication.requested` | `fabricating`, records `attempt` |
| `fabrication.stage` | `fabricating`, records `stage` and the unit's serial |
| `fabrication.completed` | `inspecting`, records all serials, clears `stage` |
| `inspection.passed` | timeline only; dispatch owns the next transition |
| `inspection.failed` | `refabricating`, with the findings in the timeline |
| `order.scrapped` | `scrapped` |
| `shipment.dispatched` | `dispatched`, records carrier and tracking number |
| `shipment.delivered` | `delivered` |

**Exactly once.** Each event id is claimed in `processed_events` inside the same transaction as the
update, so a crash between the two cannot lose or repeat an event, and a redelivery is a no-op.

**Out of order.** Status transitions are monotonic by rank, with the one documented exception:
`inspecting → refabricating → fabricating`. `delivered` and `scrapped` are terminal. A late
`fabrication.stage` still records its serial without dragging the status backwards.

**Orphans.** An event for an order that does not exist yet (and that cannot create it) is naked so
JetStream redelivers it after the processor's POST lands. `MaxDeliver` 5 bounds the retries.

## Environment

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | API, health and metrics on one port |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `hydrocoptic-datastore` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables the malaise middleware |
| `DATABASE_URL` | **required** | `postgres://hydrocoptic:<pw>@postgres:5432/hydrocoptic?sslmode=disable` |
| `NATS_URL` | `nats://nats:4222` | |
| `SERVICE_VERSION` | `dev` | Reported as `service.version` |

## Metrics

Besides `http_server_request_duration_seconds` and the Go runtime metrics:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `encabulator_ledger_orders_created_total` | counter | `model`, `result` | `result` is `created` or `replayed` |
| `encabulator_ledger_events_applied_total` | counter | `type`, `result` | `result` is `applied`, `duplicate`, `stale`, `orphan`, `ignored` or `error` |
| `encabulator_malaise_applied_total` | counter | `kind` | From the kit's malaise middleware |

A rising `orphan` or `error` rate means the ledger is behind or the schema and the event contract
have drifted apart.

## Schema and migrations

Four tables: `orders`, `order_timeline`, `processed_events`, and `schema_migrations`. Migrations are
embedded SQL under `internal/store/migrations/`, applied in filename order at startup, each in its
own transaction, recorded by name. Running them twice, or from several replicas at once, is a no-op.

## Running locally

```bash
docker run -d --name ledger-pg -p 5432:5432 \
  -e POSTGRES_USER=hydrocoptic -e POSTGRES_PASSWORD=marzlevane \
  -e POSTGRES_DB=hydrocoptic postgres:18
docker run -d --name ledger-nats -p 4222:4222 nats:2.12 --jetstream --store_dir=/tmp/jetstream

export DATABASE_URL='postgres://hydrocoptic:marzlevane@127.0.0.1:5432/hydrocoptic?sslmode=disable'
export NATS_URL='nats://127.0.0.1:4222'
make run

curl -sX POST localhost:8080/v1/orders -d '{
  "id":"ord_demo","customer":"wendy","model":"turbo","quantity":2,
  "options":{"dingleArm":true},"quote":{"id":"qte_demo","total":4200,"currency":"USD"}}' | jq
curl -s localhost:8080/v1/orders/ord_demo \
  -H 'X-Encabulator-Subject: wendy' -H 'X-Encabulator-Roles: customer' | jq
curl -s localhost:8080/v1/stats | jq
```

```bash
make build             # bin/hydrocoptic-datastore
make test              # unit tests: ledger rules and handlers, no containers
make test-integration  # adds real postgres:18 and nats:2.12 via Testcontainers
make lint              # gofmt -l plus go vet
make image             # builds from the repository root
```

`make test-integration` needs a container runtime; `source ../../../dev-env.sh` first. Without one
the integration tests skip rather than fail.

## Deployment notes

- Probes: `GET /healthz` for liveness, `GET /readyz` for readiness. Readiness checks Postgres and
  the NATS connection, never a downstream service.
- Migrations run in-process at startup; no Job or init container. Safe from several replicas.
- The durable consumer is shared across replicas, so scaling out spreads event load. Ordering within
  an order is not guaranteed across replicas, which is exactly why transitions are monotonic.
- Needs no Kubernetes API access and no service account token.
- Resource hints: 100m CPU / 128Mi memory requests, 500m / 512Mi limits. Event volume scales with
  the fabrication rate: seven stage events per unit, so a busy line is the dominant cost.
- The Postgres database is `hydrocoptic` with its own user and password from a Kubernetes Secret.
