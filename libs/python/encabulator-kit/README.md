# encabulator-kit

> "The original machine had a base-plate of prefamulated amulite, surmounted by a malleable
> logarithmic casing…" · and every Python service in the works is surmounted by this kit.

`encabulator-kit` (import name `encabulator_kit`) implements **design section 7, cross-cutting
conventions**, for the Encabulator's FastAPI/asyncio services: `logarithmic-casing-line`,
`side-fumbling-detector`, `unilateral-phase-detractor`, and `demo-scenario-orchestrator`.

A service that uses the kit gets, for free and identically to its Go, TypeScript, and Rust
siblings: JSON logs with trace and request ids, OpenTelemetry over OTLP/HTTP, `/healthz`,
`/readyz`, `/metrics`, the RED histogram labelled with route templates, the house error shape,
graceful shutdown, malaise injection, feature flags, identity propagation, and JetStream.

```python
from encabulator_kit import CommonSettings, FlagClient, MalaiseController, create_app, run

settings = CommonSettings.from_env("side-fumbling-detector")
flags = FlagClient()  # DIFFERENTIAL_GIRDLESPRING_URL
malaise = MalaiseController(settings.service_name)  # MALAISE_ENGINE_URL
app = create_app(settings.service_name, flags=flags, malaise=malaise, lifespan=my_lifespan)

if __name__ == "__main__":
    run(app)  # uvicorn + graceful shutdown
```

## Installation

The kit is a **uv path source**. Each service declares it in its own `pyproject.toml`:

```toml
dependencies = ["encabulator-kit"]

[tool.uv.sources]
encabulator-kit = { path = "../../../libs/python/encabulator-kit", editable = true }
```

Requires Python 3.14 (the kit uses `uuid.uuid7()` and PEP 695 generics).

## API

### `create_app(service_name, ...) -> FastAPI`

`encabulator_kit.app`

Builds the standard application. Keyword arguments:

| Argument | Default | Meaning |
|---|---|---|
| `version` | `"1.0.0"` | Reported by `/readyz` and set as `service.version` |
| `description` | `None` | OpenAPI description |
| `lifespan` | `None` | Your own async context manager; runs inside the kit's |
| `malaise` | `None` | A `MalaiseController`; started/stopped by the lifespan and installed as middleware |
| `flags` | `None` | A `FlagClient`; started/stopped by the lifespan |
| `readiness_timeout` | `2.0` | Per-check timeout on `/readyz` |
| `validation_error_code` | `"invalid_request"` | Error code for request validation failures (the processor uses `invalid_order`) |
| `log_level` | `$LOG_LEVEL` | |
| `configure_logs` | `True` | Set `False` in tests that capture output themselves |
| `enable_tracing` | `True` | |

The app it returns carries:

- `GET /healthz` → `200 {"status": "ok"}` whenever the process is alive.
- `GET /readyz` → `200 {"status":"ok","service","version","checks"}` when every registered check
  passes, `503` otherwise, and `503 {"status":"shutting_down"}` from the moment SIGTERM arrives.
  Register only the service's **own** hard dependencies, never downstream services:

  ```python
  app.state.readiness.add("postgres", ping_postgres)  # async, raises on failure
  app.state.readiness.add("nats", lambda: nats_readiness(bus))
  ```

- `GET /metrics` → Prometheus text format from the default `prometheus_client` registry, which
  already includes process, platform, and GC collectors.
- `http_server_request_duration_seconds{http_request_method,http_route,http_response_status_code}`
  with the design's buckets. `http_route` is always the route **template**
  (`/v1/orders/{order_id}`), including when malaise short-circuits before routing; unroutable
  requests are labelled `unmatched`. Health, readiness, and metrics requests are deliberately
  **not** observed, so probe and scrape traffic does not dominate a quiet service's percentiles.
- `X-Request-Id`: accepted (bounded to 128 characters) or generated as a ULID, echoed on the
  response, and attached to every log line via a context variable.
- The error shape `{"error": {"code": "snake_case", "message": "Human readable"}}` for
  `EncabulatorError`, `HTTPException`, validation failures (400), injected malaise, and
  unhandled exceptions (500, logged with a stack trace, never leaked to the client).

### `run(app)` / `serve(app)`

`encabulator_kit.serve`

One uvicorn worker per process, `PORT` (default 8080), idle keep-alive 60s, access log off (the
histogram replaces it). On SIGTERM the kit **fails readiness first**, waits
`SHUTDOWN_READINESS_GRACE_SECONDS` (default 3) for the endpoints controller to pull the pod out
of rotation, then stops accepting and drains in-flight requests for up to 20 seconds. A second
signal exits immediately.

Scale with replicas, not uvicorn workers: per-pod state (line capacity, run history, rate-limit
buckets) is meant to be per-pod.

### Logging · `configure_logging(service, level)`

`encabulator_kit.logs`

JSON lines on stdout: `time` (RFC 3339, milliseconds), `level`, `msg`, `service`, plus `trace_id`
and `span_id` when a recording span is active and `request_id` inside an HTTP request. Anything
in `extra=` becomes a top-level field:

```python
log.info("unit fabricated", extra={"order_id": order_id, "serial": serial})
```

Never log request bodies, tokens, or passwords.

### Tracing · `setup_tracing`, `instrument_app`, `untraced`

`encabulator_kit.telemetry`

OTLP over HTTP/protobuf to `OTEL_EXPORTER_OTLP_ENDPOINT`. **When that variable is unset tracing
is a complete no-op**: no provider, no instrumentation, no exporter retries, and logs carry no
`trace_id`. When it is set, the kit installs a `TracerProvider` with
`service.name` / `service.version` / `service.namespace=encabulator` and instruments FastAPI
(server spans, excluding `healthz`, `readyz`, `metrics`), httpx, asyncpg, redis, and SQLAlchemy.
Async SQLAlchemy engines want `instrument_sqlalchemy_engine(engine)`.

`untraced()` suppresses instrumentation for a block; the malaise (2s) and flag (5s) pollers use
it, so neither loop ever creates a span.

### Malaise · `MalaiseController`, `MalaiseMiddleware`

`encabulator_kit.malaise`

Polls `GET ${MALAISE_ENGINE_URL}/v1/directives?service=${OTEL_SERVICE_NAME}` every 2s, untraced.
Unset `MALAISE_ENGINE_URL` disables the middleware entirely. When the engine is unreachable the
last directives keep applying until each one's `expiresAt`.

| Kind | Params | Effect |
|---|---|---|
| `latency` | `ms`, `jitterMs`, `percent` | Sleep `ms ± jitterMs` before the handler |
| `error` | `status`, `percent` | Respond `status` with `malaise_injected`; **nak** for message handlers |
| `congestion` | `maxConcurrent`, `queueTimeoutMs` | Admit `maxConcurrent`; others wait, then `503 congested` |
| `cpu-burn` | `millicores` | Background threads burn ~`millicores` of CPU until expiry |

One `MalaiseController.guard()` drives both paths:

```python
async with malaise.guard():  # raises MalaiseInjected
    await do_the_work()

handler = malaise.wrap_message_handler(handler)  # or pass malaise= to PullConsumer
```

`cpu-burn` duty-cycles over a 10 ms window in daemon threads (one per whole core), so the event
loop still makes progress · the service gets slower, it does not die. Applications are counted in
`encabulator_malaise_applied_total{kind}`. `/healthz`, `/readyz`, and `/metrics` never see malaise.

### HTTP client · `ServiceClient`

`encabulator_kit.clients`

```python
datastore = ServiceClient(settings.datastore_url)  # service name from the host
order = await datastore.json("GET", f"/v1/orders/{oid}", identity=identity)
```

Default timeout 3s. GET/HEAD/PUT/DELETE/OPTIONS retry twice on transport errors and on
429/5xx with exponential backoff (100 ms base, full jitter). POST retries **only** with an
`idempotency_key`. Forwards `X-Request-Id` from the ambient context and the identity headers when
given an `Identity`. Every attempt is timed into
`http_client_request_duration_seconds{http_request_method,server_address,http_response_status_code}`
with `server_address` set to the target service name (transport failures record status `0`).
`.json(...)` raises `UpstreamError` carrying the upstream's own error code.

### Feature flags · `FlagClient`

`encabulator_kit.flags`

Polls `GET /v1/flags` every 5s with `If-None-Match`, keeps the last good set, and answers from
`DEFAULT_FLAGS` (the design section 6 defaults) until the first success · or forever, when
`DIFFERENTIAL_GIRDLESPRING_URL` is unset.

```python
if flags.bool(flags_mod.TURBO_ENCABULATION, subject=customer):
    duration /= 2
damping = flags.number(flags_mod.DINGLE_ARM_DAMPING)
```

Boolean flags with a `rolloutPercent` evaluate as `fnv1a32(f"{key}:{subject}") % 100 < percent`.
With no subject to bucket, only a 100% rollout applies.

### Identity · `Identity`, `CurrentIdentity`, `require_roles`

`encabulator_kit.identity`

```python
@app.get("/v1/audit", dependencies=[require_roles("admin")])
async def audit(identity: CurrentIdentity) -> list[AuditEntry]: ...
```

`Identity.from_headers` reads `X-Encabulator-Subject` / `X-Encabulator-Roles`;
`identity.to_headers()` writes them for an outbound call. `is_privileged` is true for
`viewer`, `operator`, or `admin` · the datastore's cross-customer visibility rule.
`strip_identity_headers` drops every inbound `X-Encabulator-*` header (the gateway's job).

### Events · `EventBus`, `PullConsumer`, `Envelope`

`encabulator_kit.messaging`

```python
bus = await open_bus("side-fumbling-detector", settings.nats_url)  # connect + ensure stream
await bus.publish(Subjects.INSPECTION_PASSED, {"orderId": oid, "attempt": 1, "serials": serials})

consumer = PullConsumer(
    bus,
    durable="side-fumbling-detector",
    filter_subject=Subjects.FABRICATION_COMPLETED,
    handler=handle,
    concurrency=1,
    malaise=malaise,
)
await consumer.start()
```

- `STREAM_CONFIG` is exactly the design's stream: `ENCABULATOR`, `encabulator.>`, file storage,
  limits retention, max age 24h, 1 replica, duplicate window 2m. `ensure_stream` creates it if
  missing and **never updates** an existing stream.
- `publish` builds the envelope (`evt_<ULID>` id, `type` = subject, RFC 3339 `time`, `source`),
  sets `Nats-Msg-Id` for JetStream dedupe, injects `traceparent` / `tracestate`, and wraps the
  send in a `PRODUCER` span (`messaging.system=nats`, `messaging.destination.name=<subject>`).
- `PullConsumer` is a durable pull consumer: explicit ack, `AckWait` 30s, `MaxDeliver` 5,
  configurable `MaxAckPending` and `concurrency`. Each message is handled inside a `CONSUMER`
  span parented to the producer's, gets in-progress acks every `AckWait / 2` so slow work is not
  redelivered underneath it, is acked on success, naked on failure or injected malaise, and
  terminated if the body will not decode.
- `Subjects` names all nine event subjects.

### Ids · `new_id`, `new_ulid`, `new_event_id`

`encabulator_kit.ids`. `new_id("ord")` → `ord_01K6...`: a prefix plus a 26-character Crockford
base32 ULID, so ids sort by creation time.

### Test helpers · `encabulator_kit.testing`

`container_runtime_available()`, `requires_container_runtime()`, and
`postgres_container()` / `nats_container()` / `valkey_container()` context managers over
Testcontainers. Integration tests **skip cleanly** when `DOCKER_HOST` is unset and no Docker
socket exists. Dev-group only; runtime images never install it.

## Environment variables

The kit reads only the common variables from design Appendix A:

| Variable | Default | Used for |
|---|---|---|
| `PORT` | `8080` | listen port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTEL_SERVICE_NAME` | the name passed in | service name in logs, spans, and malaise polling |
| `SERVICE_VERSION` | `0.0.0` | `service.version` resource attribute |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | unset ⇒ tracing is a no-op |
| `MALAISE_ENGINE_URL` | unset | unset ⇒ no malaise middleware |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | unset ⇒ built-in flag defaults |
| `NATS_URL` | `nats://nats:4222` | JetStream |
| `SHUTDOWN_READINESS_GRACE_SECONDS` | `3` | readiness-fail delay before draining |

## Metrics the kit emits

| Metric | Type | Labels |
|---|---|---|
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |
| `encabulator_malaise_applied_total` | counter | `kind` |

Plus `prometheus_client`'s default process, platform, and GC collectors.

## Developing

```bash
make test           # unit tests; integration tests skip without a container runtime
source ../../../dev-env.sh && make test    # adds the NATS container tests
make lint           # ruff check + ruff format --check
make fmt            # ruff format
```
