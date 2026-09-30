# cardinal-grammeter

> "Synchronize the cardinal grammeters." · stage seven of every encabulator ever built.

The grammeter is the Encabulator Works' crowd. It keeps a pool of synthetic customers who register
and log in through `dingle-arm-gateway` and then browse, quote, order and track, at a rate and shape
an operator dictates. Nothing here is fake: every journey is a real HTTP call over the real customer
path, so the console's topology, the quoter's HPA and the casing line's KEDA scaler all react to it
exactly as they would to a person in a browser.

It is a *load generator*, so it deliberately does not retry. A retry would both inflate the offered
rate and hide the failure the demo exists to show. What came back is what gets reported.

## Traffic model

### Journeys

| Journey | Default weight | Calls |
|---|---|---|
| `browse` | 40 | `GET /api/v1/parts` |
| `quote` | 25 | `POST /api/v1/quotes` |
| `order` | 20 | `POST /api/v1/orders` |
| `track` | 15 | `GET /api/v1/orders?limit=5`, then `GET /api/v1/orders/{id}` and `GET /api/v1/shipments/{orderId}` for one of them |

Requests are shaped like a plausible customer: models 60% `turbo`, 25% `retro`, 15% `hyper`;
quantity 1 to 3, with one order in ten asking for 4 to 8; the drawn reciprocation dingle arm 30% of
the time.

### Patterns

The rate is recomputed on every 50 ms tick from the base rate, the pattern and the time since the
run (or the pattern) started.

| Pattern | Instantaneous rate |
|---|---|
| `steady` | `rps` |
| `sine` | `rps × (1 + 0.8·sin(2πt/period))` · the modial interaction of magneto-reluctance |
| `burst` | `rps` for 80% of each period, `5 × rps` for the other 20% · capacitive diractance |
| `ramp` | 0 to `rps` linearly over one period, then hold |

`sine` averages out to `rps` over a period; `burst` averages 1.8 × `rps`; `ramp` holds at `rps`
once it has climbed.

### Scheduling

A fractional accumulator carries the remainder between ticks, so 0.1 rps really does produce six
journeys a minute rather than rounding to nothing. Journeys in flight are capped at `MAX_IN_FLIGHT`;
anything the cap refuses is counted as `dropped` and **never queued**, because a queue would turn an
overload into a growing lie about the offered rate.

### Customers and tokens

`CUSTOMERS` synthetic customers are named `grammeter-0001` upward, and each one's password is
derived as `HMAC-SHA256(CUSTOMER_PASSWORD_SEED, username)`. Nothing is stored: the same seed always
produces the same pool, so restarting the grammeter reuses the accounts instead of filling the
detractor with orphans.

A customer authenticates lazily on first use (register, tolerating the 409 from a previous run, then
log in), renews its token 60 seconds before expiry, and on a `401` from any call drops the token,
logs in again and retries that one call once.

### What counts as an error

A journey is recorded as failed on a transport error, a 5xx, or a 429. Business rejections are
normal factory life and are *not* errors: `409 insufficient_parts` means the marzlevane foundry is
behind, and a `404` on a shipment means the order has not been dispatched yet. Every status code,
including those, shows up in the `statuses` map and in
`encabulator_grammeter_requests_total{journey,status}`.

## API

One port serves the API, `/healthz`, `/readyz` and `/metrics`. The grammeter has no hard
dependencies of its own · the gateway is downstream · so `/readyz` is green as soon as the process
is up.

### `GET /v1/traffic`

Configuration plus live statistics.

```json
{
  "running": true,
  "rps": 25,
  "pattern": "sine",
  "periodSeconds": 60,
  "mix": {"browse": 40, "quote": 25, "order": 20, "track": 15},
  "customers": 50,
  "target": "http://dingle-arm-gateway:8080",
  "maxInFlight": 256,
  "tickMs": 50,
  "stats": {
    "targetRps": 41.2,
    "actualRps": 40.8,
    "inFlight": 7,
    "started": 18422,
    "dropped": 0,
    "journeys": {
      "browse": {"count": 1601, "errors": 0, "p50Ms": 12.4, "p99Ms": 61.9}
    },
    "statuses": {"200": 2400, "202": 480, "409": 12}
  }
}
```

`targetRps` is the instantaneous rate the pattern asks for. `actualRps` is journeys completed per
second over the trailing 10 seconds, which tracks a burst instead of averaging it away; the
per-journey figures cover the trailing 60 seconds. `started` and `dropped` are cumulative since the
process started.

### `PUT /v1/traffic`

A partial update; every field is optional and an unknown field is a `400`.

```bash
curl -X PUT localhost:8080/v1/traffic -H 'content-type: application/json' -d '{
  "running": true, "rps": 25, "pattern": "burst", "periodSeconds": 30,
  "mix": {"browse": 10, "order": 90}, "customers": 100
}'
```

| Field | Range |
|---|---|
| `running` | boolean |
| `rps` | 0.1 to 500 |
| `pattern` | `steady`, `burst`, `ramp`, `sine` |
| `periodSeconds` | 1 to 3600 |
| `mix` | any subset of the four journey names, non-negative weights, at least one positive |
| `customers` | 1 to 1000 |

Rejections return `400 invalid_traffic` and change nothing. The phase clock restarts when a run
starts or the pattern or period changes, so a `ramp` really does begin at zero.

**The target cannot be changed.** It is fixed by `DINGLE_ARM_GATEWAY_URL` and there is no API for
it, by design: a traffic generator that can be pointed anywhere is a footgun.

### `GET /v1/journeys`

The journey names, the default mix, the pattern names and the rate bounds, so the console's controls
do not have to hard-code them.

## Environment

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `cardinal-grammeter` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `DINGLE_ARM_GATEWAY_URL` | **required** | The only upstream |
| `CUSTOMER_PASSWORD_SEED` | **required** | Secret; synthetic passwords are derived from it |
| `CUSTOMERS` | `50` | 1 to 1000 |
| `MAX_IN_FLIGHT` | `256` | Journeys in flight before dropping |
| `AUTOSTART` | `false` | Start generating immediately |
| `AUTOSTART_RPS` | `5` | The initial rate, used whether or not it autostarts |

`MALAISE_ENGINE_URL` is deliberately ignored: the grammeter does not induce malaise in itself, since
it would only be measuring its own limp.

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_grammeter_requests_total` | counter | `journey`, `status` (`error` when the call never got an answer) |
| `encabulator_grammeter_target_rps` | gauge | |
| `encabulator_grammeter_dropped_total` | counter | Journeys the in-flight cap refused |
| `encabulator_grammeter_in_flight` | gauge | |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |

Outbound calls carry W3C trace context, so the console's service graph draws the
`cardinal-grammeter → dingle-arm-gateway` edge from real spans.

## Running locally

```bash
make build            # bin/cardinal-grammeter
make test             # unit tests, no container runtime needed
make lint             # gofmt -l plus go vet
make run              # against DINGLE_ARM_GATEWAY_URL, default http://localhost:8081
make image            # container image, build context is the repository root
```

With the gateway port-forwarded to 8081:

```bash
make run &
curl -s localhost:8080/v1/traffic | jq .
curl -sX PUT localhost:8080/v1/traffic -H 'content-type: application/json' \
  -d '{"running":true,"rps":10,"pattern":"sine","periodSeconds":30}' | jq .stats
```

## Deployment notes

**Single replica.** The rate schedule, the in-flight cap and the customer pool are all per-process,
so two replicas would silently generate twice the requested traffic. Do not set an HPA on it and do
not give it a PodDisruptionBudget it cannot satisfy.

It needs no service account token, no volumes, and no backing store.

## Layout

| Package | What it holds |
|---|---|
| `internal/traffic` | The four patterns and the fractional-accumulator scheduler |
| `internal/journey` | Journey names, the weighted picker, request shaping, and the journey bodies |
| `internal/customer` | The synthetic customer pool, password derivation, token lifecycle |
| `internal/gateway` | The no-retry JSON client for the gateway |
| `internal/engine` | The 50 ms tick, the in-flight cap, and the traffic configuration |
| `internal/stats` | Rolling 60-second per-journey statistics and status counts |
| `internal/metrics` | Prometheus surface |
| `internal/api` | `/v1/traffic` and `/v1/journeys` |
