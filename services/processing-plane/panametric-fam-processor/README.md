# panametric-fam-processor

> Quick wrote "pentametric fan"; the Rockwell film says "panametric fam". We are not here to
> adjudicate.

The order saga. Everything a customer asks the Encabulator Works to build passes through
this service, and its job is to make sure the works never half-takes an order: price it,
reserve the parts, write it to the ledger, then announce it. If the ledger write fails, the
parts go back on the shelf before the customer hears anything. That compensation is the
whole reason this service exists.

It also owns the refabrication decision. When the side-fumbling detector rejects a batch,
the processor decides whether the works tries again or writes the order off.

- Plane: `processing` · Language: TypeScript (Node 24, Fastify 5) · Owns no state
- Design: [section 8.2](../../../docs/design.md#82-panametric-fam-processor)

## The saga

```
POST /v1/orders
  │
  ├─ 1. validate the body, take the customer from X-Encabulator-Subject   → 400 invalid_order
  ├─ 2. read cached flags: retro withdrawn?                               → 409 model_unavailable
  ├─ 3. POST spurving-bearing-quoter /v1/quotes                           → 400 / 503
  ├─ 4. POST prefabulated-amulite-depot /v1/reservations                  → 409 insufficient_parts
  ├─ 5. POST hydrocoptic-datastore /v1/orders                     ─┐
  │        on failure: release the reservation, then              │ compensation
  │        503 dependency_unavailable                             ─┘
  ├─ 6. publish encabulator.order.accepted
  └─ 6. publish encabulator.fabrication.requested (attempt 1)             → 202 with the order
```

Step 4 sends the order id as both the reservation's `orderId` and the request's
`Idempotency-Key`, so a retried POST can never reserve twice. Step 5 is idempotent on `id`
at the datastore, so it is safe to retry too.

Nothing reaches the ledger unless it was priced and reserved, which is why a rejected order
never appears in an order list.

## Refabrication and scrapping

On `encabulator.inspection.failed`, the processor reads the order back from the ledger (as an
internal `operator`, since `inspection.failed` carries only serials and findings) and:

- attempt 1 or 2 → publish `fabrication.requested` with `attempt + 1`
- attempt 3 → publish `order.scrapped` with a reason naming the worst finding

A ledger outage makes the handler throw, which naks the message; JetStream redelivers it up
to `MaxDeliver` 5.

## Circuit breakers

One breaker per dependency (quoter, depot, datastore): open after five consecutive failures,
half-open after ten seconds, and only one probe gets through while half-open. An open breaker
fails fast with `503 dependency_unavailable` instead of adding load to a sick service.

Only *dependency* failures count. A `409 insufficient_parts` is the depot working correctly,
so a marzlevane shortage never trips the depot breaker; a business answer during a half-open
probe is treated as proof of life and closes the breaker.

State is exported as `encabulator_circuit_breaker_state{dependency}`: `0` closed, `1`
half-open, `2` open.

## API

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/v1/orders` | `customer`+ (via the gateway) | Place an order; `202` with the ledger's representation |
| GET | `/healthz`, `/readyz`, `/metrics` | any | Liveness, readiness (NATS), Prometheus |

Reads go to `hydrocoptic-datastore`, not here.

```json
// request
{ "model": "turbo", "quantity": 2, "options": { "dingleArm": true } }
```

`model` is `turbo`, `retro` or `hyper`; `quantity` is 1 to 10. A `customer` field in the body
is ignored: identity comes from `X-Encabulator-Subject`.

| Status | Code | When |
|---|---|---|
| 202 | | The order is on the books |
| 400 | `invalid_order` | Bad model, bad quantity, no identity, or the quoter refused it |
| 409 | `model_unavailable` | `retro-encabulator-sales` is off |
| 409 | `insufficient_parts` | The depot is short; the response carries the short SKUs |
| 503 | `dependency_unavailable` | A dependency is down or its breaker is open |

## Events

| Direction | Subject | Data |
|---|---|---|
| out | `encabulator.order.accepted` | `orderId, customer, model, quantity, options, total, currency` |
| out | `encabulator.fabrication.requested` | `orderId, attempt, model, quantity, options, reservationId` |
| out | `encabulator.order.scrapped` | `orderId, attempt, reason` |
| in | `encabulator.inspection.failed` | `orderId, attempt, serials[], findings[]` |

Durable consumer `panametric-fam-processor`, explicit ack, `AckWait` 30 s, `MaxDeliver` 5.

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `NATS_URL` | `nats://nats:4222` | |
| `SPURVING_BEARING_QUOTER_URL` | **required** | `http://spurving-bearing-quoter:8080` |
| `PREFABULATED_AMULITE_DEPOT_URL` | **required** | `http://prefabulated-amulite-depot:8080` |
| `HYDROCOPTIC_DATASTORE_URL` | **required** | `http://hydrocoptic-datastore:8080` |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | Unset means built-in flag defaults |
| `MALAISE_ENGINE_URL` | unset | Unset disables fault injection |
| `OTEL_SERVICE_NAME` | `panametric-fam-processor` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `SERVICE_VERSION` | `1.0.0` | Trace resource attribute |

The three `*_URL` variables are required: the processor exits at startup without them,
because a processor that cannot reach its neighbours has nothing to offer.

## Feature flags read

| Flag | Default | Effect |
|---|---|---|
| `retro-encabulator-sales` | `true` | When false, `retro` orders are rejected with `model_unavailable` |

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_orders_total` | counter | `model`, `result` (`accepted`, `rejected`, `failed`) |
| `encabulator_refabrications_total` | counter | |
| `encabulator_orders_scrapped_total` | counter | |
| `encabulator_circuit_breaker_state` | gauge | `dependency` |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |
| `encabulator_malaise_applied_total` | counter | `kind` |

Plus the Node process and runtime defaults. `result=rejected` means the works said no on
purpose; `result=failed` means something broke.

## Running locally

```bash
source ../../../dev-env.sh
docker run -d --name proc-nats -p 4222:4222 nats:2.12 -js

make build
NATS_URL=nats://127.0.0.1:4222 \
SPURVING_BEARING_QUOTER_URL=http://127.0.0.1:8081 \
PREFABULATED_AMULITE_DEPOT_URL=http://127.0.0.1:8082 \
HYDROCOPTIC_DATASTORE_URL=http://127.0.0.1:8083 \
LOG_LEVEL=debug make run

curl -s -XPOST localhost:8080/v1/orders \
  -H 'content-type: application/json' \
  -H 'x-encabulator-subject: alice' -H 'x-encabulator-roles: customer' \
  -d '{"model":"turbo","quantity":2,"options":{"dingleArm":true}}' | jq
```

## Tests

```bash
make test-unit   # validation, attempt arithmetic, circuit breaker, and the whole saga
make test        # adds the real-JetStream suite
```

The saga tests are not mocked at the HTTP boundary: the quoter, depot and datastore are real
Fastify servers on real sockets with programmable behaviour, so the HTTP client, its retries
and the circuit breakers all behave as they will in the cluster. The compensation paths are
covered explicitly, including the case where the compensating release *also* fails. The
integration suite adds real NATS and skips cleanly when `DOCKER_HOST` is unset.

## Container

```bash
make image   # build context is the repository root
```

Multi-stage: the kit is built first, then the service, then production dependencies only.
Runtime is `gcr.io/distroless/nodejs24-debian13` as UID 65532, read-only-root-filesystem
compatible.
