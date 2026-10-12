# spurving-bearing-quoter

> "The original machine had a base-plate of prefamulated amulite, surmounted by a malleable
> logarithmic casing in such a way that the two main spurving bearings were in a direct line with
> the pentametric fan."

This service aligns the spurving bearings with the pentametric fan, economically. Given a model, a
quantity, and whether the customer wants a drawn reciprocation dingle arm, it returns a price. It is
the pricing engine of the Encabulator Works: the panametric fam processor calls it before it will
reserve a single marzlevane, and the dingle arm gateway exposes it to customers who want a number
before they want an encabulator.

On the way to that number the quoter computes the **pentametric harmonic**, a convergent series of
five superimposed sinusoids that nobody reads and the price does not depend on. It exists because
the demo needs one service whose cost is arithmetic rather than waiting: five to fifteen
milliseconds of real CPU per quote, scaled by model and quantity. That makes the quoter the natural
target for a CPU-based HPA, and it makes a traffic ramp on the console produce pods rather than just
prettier graphs.

It is the repository's only Rust service, and it implements every cross-cutting convention in design
section 7 directly rather than through a shared kit: JSON logs carrying trace, span, and request
ids; OpenTelemetry with W3C propagation and Valkey client spans; `/healthz`, `/readyz`, `/metrics`;
the one error shape; the malaise middleware; and the differential girdlespring flag client.

- **Plane:** processing
- **Language:** Rust (edition 2024), axum, tokio
- **Owns:** Valkey keys `quote:*`
- **Spec:** [`docs/design.md`](../../../docs/design.md) section 8.3

## API

All bodies are JSON. Timestamps are RFC 3339 UTC with milliseconds. Errors use the repository-wide
shape `{"error": {"code": "snake_case_code", "message": "Human readable"}}`.

### `POST /v1/quotes`

```jsonc
// request
{"model": "hyper", "quantity": 5, "options": {"dingleArm": true}}
```

```jsonc
// 200 OK
{
  "id": "qte_01a0f2f0-f33d-763d-9a4f-20e3d3cb2dc6",
  "model": "hyper",
  "quantity": 5,
  "options": {"dingleArm": true},
  "unitPrice": 4408.0,
  "subtotal": 22040.0,
  "surcharges": [
    {"code": "volume_discount",    "amount": -1102.0},
    {"code": "turbo_encabulation", "amount": 3306.0}
  ],
  "total": 24244.0,
  "currency": "USD",
  "computedAt": "2026-09-30T15:31:12.317Z",
  "cached": false
}
```

`options` may be omitted entirely; `dingleArm` defaults to `false`.

| Status | Code | When |
|---|---|---|
| 200 | | Priced, or served from the cache (`cached: true`) |
| 400 | `unknown_model` | `model` is not `turbo`, `retro`, or `hyper` |
| 400 | `invalid_quantity` | `quantity` is outside 1 to 10 |
| 400 | `invalid_request` | The body is not valid JSON, or a field is missing or mistyped |
| 415 | `unsupported_media_type` | `Content-Type` is not `application/json` |
| 503 | `malaise_injected` / `congested` | An injected fault (see [Malaise](#malaise)) |

### `GET /v1/quotes/{id}`

Returns a stored quote with `cached: true`, or:

| Status | Code | When |
|---|---|---|
| 404 | `quote_not_found` | The quote has expired (one hour), never existed, or `VALKEY_URL` is unset |
| 503 | `dependency_unavailable` | Valkey is configured but unreachable. A 404 here would be a lie the processor could act on |

### Ops routes

| Route | Purpose |
|---|---|
| `GET /healthz` | Liveness. `200 {"status":"ok"}` while the process runs |
| `GET /readyz` | Readiness. `200` normally, `503 {"status":"draining"}` after SIGTERM |
| `GET /metrics` | Prometheus text format |

`/readyz` also reports what the quoter knows about its soft dependencies, for operators only:

```json
{"status": "ok", "quoteCache": "connected", "flags": "live"}
```

Neither field can make readiness fail. **The quoter has no hard dependencies.** Valkey is soft
(pricing works without it) and the girdlespring has built-in defaults, so a red `/readyz` means one
thing: this pod is shutting down.

These three routes are deliberately excluded from tracing, from the request-duration histogram, and
from the malaise middleware. They still accept and echo `X-Request-Id`.

## Pricing

| Model | Name | Base price |
|---|---|---|
| `turbo` | Turbo Encabulator (1944) | 1944.00 |
| `retro` | Retro Encabulator (Rockwell) | 2749.00 |
| `hyper` | HyperEncabulator (SANS ICS) | 4096.00 |

1. `unitPrice` = base price + 312.00 if `dingleArm`
2. `subtotal` = `unitPrice` x `quantity`
3. `volume_discount` = -5% of `subtotal`, when `quantity >= 5`
4. `turbo_encabulation` = +15% of `subtotal`, when the `turbo-encabulation` flag is on
5. `total` = `subtotal` + every surcharge

Money is carried internally as integer cents and rounded half away from zero, so the arithmetic has
no floating-point drift and every surcharge is a whole number of cents. **Both percentages are taken
against the subtotal**, which makes them independent: the order in which they apply cannot change
the total.

Prices appear in JSON as numbers (`1944.0`, not `"1944.00"`).

### The pentametric harmonic

The series is iterated until its CPU budget is spent, rather than for a fixed number of terms:

```
budget_ms = 5.0 x model_weight x (1 + (quantity - 1) / 9) x PENTAMETRIC_BURN_SCALE
model_weight: turbo 1.0, retro 1.2, hyper 1.5
```

A single turbo targets 5 ms; ten hyper units target 15 ms. Budgeting by time rather than by
iteration count is deliberate: a fixed count burns 1 ms on an Apple M-series core and 30 ms on a
throttled vCPU, which would make the HPA behave differently on every cluster.

The burn runs on `spawn_blocking` behind a semaphore of `PENTAMETRIC_WORKERS` permits, so it never
occupies an async worker thread and a burst of quotes cannot stop the pod from answering `/healthz`.
There is a test for exactly that (`quoting_stays_responsive_under_a_full_pentametric_burn`).

## Caching

Governed by the `quote-cache` flag. Two key families, both under `quote:*`:

| Key | TTL | Written | Read by |
|---|---|---|---|
| `quote:{id}` | 1 hour | always | `GET /v1/quotes/{id}` |
| `quote:input:{model}:{quantity}:{dingleArm}:{turbo}` | 60 s | only with `quote-cache` on | `POST /v1/quotes` |

The input key carries every input that can move the price and nothing else, so two requests that
must produce the same money produce the same key. It is written out in full rather than hashed,
because `KEYS quote:input:*` reading like a sentence is worth more to a demo than sixteen bytes of
SHA-256, and the space is small and closed (3 models x 10 quantities x 2 options x 2 flag states =
120 keys).

`quote:{id}` is written regardless of the flag, so turning caching off does not break retrieval by
id. Both writes go in one pipeline: a cache fill costs one round trip.

**Valkey is a soft dependency.** The connection is established lazily, retried every five seconds,
and every failure is logged, counted in `encabulator_quote_cache_errors_total`, and otherwise
ignored. Losing Valkey mid-flight costs a cache hit and nothing else; there is an integration test
that kills the container and checks the quoter keeps pricing and stays ready.

## Feature flags

Polled from `${DIFFERENTIAL_GIRDLESPRING_URL}/v1/flags` every 5 s with `If-None-Match`. The last
good set is kept on `304`, on a parse failure, and on an unreachable girdlespring. With
`DIFFERENTIAL_GIRDLESPRING_URL` unset the built-in defaults apply forever.

| Flag | Default | Effect |
|---|---|---|
| `turbo-encabulation` | `false` | Adds the 15% turbo surcharge |
| `quote-cache` | `true` | Caches quotes in Valkey for 60 s |

Boolean flags may carry `rolloutPercent`, evaluated per subject as
`fnv1a32(key + ":" + subject) % 100 < rolloutPercent`, where the subject is the
`X-Encabulator-Subject` header. Because `turbo` is part of the cache key, a partial rollout caches
correctly for both cohorts.

## Malaise

Directives are polled from `${MALAISE_ENGINE_URL}/v1/directives?service=${OTEL_SERVICE_NAME}` every
2 s, untraced. With `MALAISE_ENGINE_URL` unset the middleware is inert. If the engine becomes
unreachable, the last directives keep being applied until each one's `expiresAt`, so a fault still
ends on schedule even when nobody can be asked about it.

| Kind | Params | Effect |
|---|---|---|
| `latency` | `ms`, `jitterMs`, `percent` | Sleeps `ms ± jitterMs` before the handler |
| `error` | `status`, `percent` | Responds `status` with `malaise_injected`, skipping the handler |
| `congestion` | `maxConcurrent`, `queueTimeoutMs` | Admits `maxConcurrent` at a time; the rest queue, then get `503 congested` |
| `cpu-burn` | `millicores` | Background threads burn roughly `millicores` on a duty cycle until expiry |

Applied in the order congestion, latency, error: a congested quoter still queues, a slow quoter
still fails slowly, and a request rejected by `error` has already paid the latency cost, which is
what a real failing dependency looks like. `percent` defaults to 100. Where several congestion
directives are active the tightest wins. Each application increments
`encabulator_malaise_applied_total{kind}`.

## Observability

### Logs

JSON lines on stdout. `time` (RFC 3339, ms), `level`, `msg`, `service`, plus `trace_id`, `span_id`,
and `request_id` when they exist, plus the span and event fields:

```json
{"time":"2026-09-30T15:31:12.318Z","level":"info","msg":"priced a quote","service":"spurving-bearing-quoter","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"28a191eab64b7f89","request_id":"req-smoke-1","compute_ms":11.018,"dingle_arm":true,"enduser.id":"customer-7","harmonic":1.93696906,"harmonic_terms":405504,"http.request.method":"POST","http.route":"/v1/quotes","model":"hyper","quantity":5,"quote_id":"qte_01a0f2f0-f33d-763d-9a4f-20e3d3cb2dc6","total":"24244.00","turbo":true,"url.path":"/v1/quotes"}
```

Request bodies, tokens, and passwords are never logged.

### Traces

OpenTelemetry over OTLP HTTP/protobuf. **With `OTEL_EXPORTER_OTLP_ENDPOINT` unset, tracing is a
no-op**: no exporter is built, no OpenTelemetry layer is installed, and `trace_id` simply does not
appear in the logs. Spans are created either way, so the code path is identical.

- Server spans named `{METHOD} {route}`, parented from inbound `traceparent`, with
  `http.request.method`, `http.route`, `url.path`, `http.response.status_code`, and `enduser.id`.
- Valkey client spans named `{OP} {collection}`, with `db.system=redis`, `db.system.name=valkey`,
  `db.namespace`, `db.operation.name`, `server.address`, `server.port`, and `peer.service=valkey`,
  so the console's topology draws a cache edge.
- Never traced: `/healthz`, `/readyz`, `/metrics`, the malaise poll, and the flag poll.

### Metrics

| Metric | Type | Labels |
|---|---|---|
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |
| `encabulator_quotes_total` | counter | `model`, `cached` |
| `encabulator_quote_compute_seconds` | histogram | |
| `encabulator_malaise_applied_total` | counter | `kind` |
| `encabulator_quote_cache_errors_total` | counter | `operation` |

Server buckets are the repository-wide set: 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10.
`http_route` is always the route template (`/v1/quotes/{id}`), never the raw path; unmatched
requests are labelled `unmatched` so a scanner cannot blow up the cardinality. Process metrics are
included on Linux.

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | One port serves the API, health, and metrics |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. `RUST_LOG` overrides it entirely |
| `OTEL_SERVICE_NAME` | `spurving-bearing-quoter` | Also the `service=` parameter sent to the malaise engine |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | e.g. `http://otel-collector.encabulator-observability:4318`. Unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables the malaise middleware |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | Unset means built-in flag defaults |
| `VALKEY_URL` | unset | e.g. `redis://default:<pw>@valkey:6379/0`, or `rediss://:@<host>:6379/0` for a TLS-only Valkey such as ElastiCache Serverless. Unset disables the quote cache |

Service-specific knobs, beyond appendix A:

| Variable | Default | Notes |
|---|---|---|
| `PENTAMETRIC_BURN_SCALE` | `1.0` | Multiplies the harmonic's CPU budget. `0` makes quoting nearly free; `2` doubles the HPA pressure |
| `PENTAMETRIC_WORKERS` | CPU count | Concurrent harmonic computations. Cap this at the pod's CPU limit |
| `SHUTDOWN_DELAY_SECONDS` | `5` | How long to keep serving after readiness starts failing, so Service endpoints can propagate. `0` closes the listener immediately |

The quoter reads no secret of its own beyond the password inside `VALKEY_URL`. The only TLS it speaks
is to Valkey when `VALKEY_URL` is `rediss://`: rustls with the ring provider, verifying the server
against the system root store (`/etc/ssl/certs` in the image; `SSL_CERT_FILE` overrides it). Every
other URL it is given is in-cluster plaintext HTTP.

### Timeouts and shutdown

Server read-header 5 s, idle connection 60 s, outbound client 3 s, Valkey 1 s (tighter than the
general budget, because a sick cache must never become the quoter's latency). Outbound GETs retry
twice with exponential backoff from a 100 ms base and full jitter.

On SIGTERM: readiness starts failing, the service keeps serving for `SHUTDOWN_DELAY_SECONDS`, the
listener closes, in-flight requests drain for up to 20 s, the tracer flushes, the process exits.
Budget `terminationGracePeriodSeconds` above `SHUTDOWN_DELAY_SECONDS + 20`; the defaults fit inside
Kubernetes' default of 30.

## Running locally

Nothing is required but a Rust toolchain. Every integration is optional and the service works with
all of them switched off.

```sh
make run                       # http://localhost:8080, no cache, no flags, no faults
curl -s localhost:8080/healthz
curl -s localhost:8080/readyz
curl -s localhost:8080/metrics | head

curl -s -X POST localhost:8080/v1/quotes \
  -H 'content-type: application/json' \
  -d '{"model":"hyper","quantity":5,"options":{"dingleArm":true}}' | jq
```

With a Valkey to cache into:

```sh
source ../../../dev-env.sh
finch run -d --name quoter-valkey -p 6379:6379 valkey/valkey:9

VALKEY_URL=redis://127.0.0.1:6379/0 LOG_LEVEL=debug cargo run
# the same request twice: the second answers with "cached": true
```

Wire it to the rest of the works by pointing it at the real services:

```sh
VALKEY_URL=redis://127.0.0.1:6379/0 \
DIFFERENTIAL_GIRDLESPRING_URL=http://localhost:8083 \
MALAISE_ENGINE_URL=http://localhost:8092 \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
cargo run
```

## Testing

```sh
make test              # everything; the Valkey tests skip without a container runtime
make test-unit         # unit tests plus handler tests, no containers
make test-integration   # requires: source ../../../dev-env.sh
make lint              # clippy -D warnings, plus cargo fmt --check
```

Unit tests cover the pricing arithmetic exhaustively (every model, quantity, option, and flag
combination), the cache key space (all 120 keys, checked for collisions), the malaise plan with a
deterministic dice roll, and the money rounding rule. Handler tests drive the real router through
`tower::ServiceExt::oneshot`. The Valkey tests run against a real `valkey/valkey:9` container,
including one that kills it mid-run to prove the soft dependency is soft.

> **Note on Testcontainers.** The design calls for Testcontainers. `testcontainers` 0.28 talks to
> the daemon through `bollard`, which requires a `Warnings` field in the `POST /containers/create`
> response; finch-daemon v0.23 omits it, so every Testcontainers-Rust call fails before the
> container is usable. `tests/support/mod.rs` therefore drives the same image through the container
> CLI: same behaviour, same clean skip when `DOCKER_HOST` is unset, no dependency on a client that
> cannot reach the daemon this repository ships with. The Go, Node, and Python Testcontainers
> clients tolerate the omission, so the other services are unaffected.

## Container image

```sh
make image        # multi-arch (linux/amd64, linux/arm64) via buildx
make image-load   # this architecture only, loaded into the local runtime
```

Multi-stage, with `cargo-chef` so a source-only change does not rebuild the dependency graph. Build
context is the repository root. The runtime image is `gcr.io/distroless/cc-debian13:nonroot`,
running as UID/GID 65532, compatible with a read-only root filesystem, with no `HEALTHCHECK` because
Kubernetes probes own health.

## Notes for the Helm chart

- **No hard dependencies.** `/readyz` cannot fail because of Valkey or the girdlespring; a red
  `/readyz` means the pod is terminating. Do not add a readiness gate on Valkey.
- **CPU requests drive the HPA.** The pentametric harmonic is the load. A request of `250m` with a
  limit of `1` gives an HPA on `averageUtilization: 70` something real to react to. Set
  `PENTAMETRIC_WORKERS` at or below the CPU limit's core count so a pod cannot oversubscribe itself.
- **Memory is small and flat.** 64 Mi request, 128 Mi limit is comfortable; there is no heap growth
  proportional to traffic.
- **`terminationGracePeriodSeconds: 30`** or more, to cover `SHUTDOWN_DELAY_SECONDS` (5) plus the
  20 s drain.
- **Valkey password** arrives inside `VALKEY_URL`, which the chart assembles with `$(VALKEY_PASSWORD)`
  expansion from the secret-backed variable.
- **`readOnlyRootFilesystem: true`** works as-is. The only writable path needed is `/tmp`, as an
  `emptyDir`.
- **No service account token.** The quoter never calls the Kubernetes API.
