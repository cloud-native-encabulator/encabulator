# `@encabulator/kit`

The shared TypeScript service kit for the Cloud Native Encabulator. It is the differential
girdlespring of the Node side of the works: every cross-cutting convention in
[design section 7](../../../docs/design.md#7-cross-cutting-conventions) lives here exactly
once, so a service file contains only its own domain and none of the plumbing.

Consumed by the TypeScript services as a `file:` dependency, compiled to `dist/`.

- Node 24 LTS, ESM only, TypeScript strict
- Fastify 5, pino, prom-client, OpenTelemetry, the modular `@nats-io/*` v3 JetStream client
- Lint and format with Biome; tests with Vitest

## Install and build

```bash
make build      # npm ci + tsc, then touch READY
make test       # unit tests, plus container-backed integration tests when available
make lint       # biome check
```

Services depend on it like this:

```json
{ "dependencies": { "@encabulator/kit": "file:../../../libs/ts/kit" } }
```

`npm ci` in a service symlinks the kit, so **the kit must be built before a service
builds**. The service Makefiles and Dockerfiles both do that for you.

## Wiring a service

Import `@encabulator/kit/telemetry` first and only then dynamic-import the rest. ESM
hoists static imports, so this is what gives the OpenTelemetry instrumentations a chance to
patch `http`, `undici` and `ioredis` before those modules load.

```ts
// src/index.ts
import { startTelemetry } from '@encabulator/kit/telemetry';

const telemetry = startTelemetry({ serviceName: 'nofer-trunnion-dispatch' });
const { main } = await import('./main.js');
await main(telemetry);
```

```ts
// src/main.ts
import {
  createApp, createLogger, FlagClient, logLevel, MalaiseController,
  serveApp, servicePort, type Telemetry,
} from '@encabulator/kit';

export async function main(telemetry: Telemetry): Promise<void> {
  const logger = createLogger({ service: telemetry.serviceName, level: logLevel() });
  const flags = new FlagClient({ logger });
  const malaise = new MalaiseController({ serviceName: telemetry.serviceName, logger });
  await Promise.all([flags.start(), malaise.start()]);

  const app = await createApp({
    serviceName: telemetry.serviceName,
    logger, telemetry, malaise,
    readiness: [{ name: 'valkey', check: () => redis.ping() }],
    register: async (instance) => { instance.post('/v1/things', handler); },
  });

  await serveApp(app, {
    port: servicePort(), logger,
    shutdownHooks: [() => flags.stop(), () => malaise.stop(), () => telemetry.shutdown()],
  });
}
```

## API

### `app.ts` · the Fastify factory

| Export | Purpose |
|---|---|
| `createApp(opts)` | Builds the Fastify instance: `/healthz`, `/readyz`, `/metrics`, request ids, the error shape, the duration histogram, malaise hooks, `request.identity`. Does not listen, so tests can use `app.inject`. |
| `serveApp(app, opts)` | Listens, and traps SIGTERM/SIGINT: fail readiness, stop accepting, drain up to 20 s, run the shutdown hooks. |
| `ReadinessCheck` | `{ name, check }`. A check that throws means "not ready"; its message appears in the `/readyz` body. |
| `app.addReadinessCheck(check)` | Register a check after construction (once a client has connected, say). |
| `app.failReadiness()` | Make `/readyz` fail without closing the server. |

`/readyz` must only cover the service's **own** hard dependencies, never a downstream
service. `/healthz` is always 200 while the process lives.

The request-duration histogram is labelled with `http_route` taken from the Fastify route
template (`/v1/things/:id`), never the raw path. `/healthz`, `/readyz` and `/metrics` are
excluded from it so that Kubernetes probes do not swamp the console's per-pod request rate.

### `logger.ts`

`createLogger({ service, level, base?, destination? })` returns a pino logger that emits
`time` (RFC 3339 with ms), `level` as a name, `msg`, `service`, and `trace_id` / `span_id`
whenever a span is recording. `request_id` is added per request by the app factory.
Passwords and tokens are redacted.

### `telemetry.ts` (also exported as `@encabulator/kit/telemetry`)

`startTelemetry({ serviceName, serviceVersion?, endpoint?, instrumentations? })`.

- OTLP over HTTP/protobuf to `OTEL_EXPORTER_OTLP_ENDPOINT`; **unset means a no-op**, which
  keeps tests and local runs quiet.
- Resource carries `service.name`, `service.version`, `service.namespace=encabulator`.
- Instruments `http`, `undici` (so `fetch`), `ioredis`, and Fastify.
- `/healthz`, `/readyz`, `/metrics`, `/v1/directives` and `/v1/flags` are never traced.
- `withSuppressedTracing(fn)` and `suppressedContext()` for anything else that must stay
  out of the traces.

### `metrics.ts`

Uses prom-client's default registry, so service-local `new Counter({...})` shows up on
`/metrics` without extra wiring.

| Export | Metric |
|---|---|
| `httpServerDuration` | `http_server_request_duration_seconds{http_request_method,http_route,http_response_status_code}` |
| `httpClientDuration` | `http_client_request_duration_seconds{http_request_method,server_address,http_response_status_code}` |
| `malaiseApplied` | `encabulator_malaise_applied_total{kind}` |
| `messagesPublished` / `messagesConsumed` | `encabulator_messages_{published,consumed}_total{subject,result}` |
| `counter` / `gauge` / `histogram` | Idempotent factories for domain metrics; re-importing a module in tests will not throw. |
| `startDefaultMetrics()` | Node process and runtime metrics, once per process. |

### `http-client.ts`

`new HttpClient({ name, baseUrl, timeoutMs?, retries?, defaultHeaders?, logger? })`.
`name` is the target **service** name and becomes the `server_address` metric label.

- default timeout 3 s
- GET/PUT/DELETE retry twice, exponential backoff with 100 ms base and full jitter
- POST retries only with `idempotencyKey` (also sent as `Idempotency-Key`)
- retries network failures, timeouts, 429, 502, 503 and 504; never a 4xx
- non-2xx throws `HttpClientError` carrying the upstream `error.code`; pass
  `expect: [409]` to get the response back instead
- `withHeaders()` clones the client with extra headers, for identity propagation

### `errors.ts`

`HttpError(status, code, message, details?)` plus `badRequest`, `unauthorized`,
`forbidden`, `notFound`, `conflict`, `unavailable`, and `errorBody()`. Everything
serialises to `{"error": {"code": "snake_case", "message": "..."}}`.

### `flags.ts`

`new FlagClient({ baseUrl?, logger?, pollIntervalMs? })`, `await client.start()`.

- polls `GET /v1/flags` every 5 s with `If-None-Match`; 304 keeps the current set
- keeps the last good set when girdlespring is unreachable
- `FLAG_DEFAULTS` carries the section 6 defaults, used when the service is unreachable at
  startup or `DIFFERENTIAL_GIRDLESPRING_URL` is unset
- `boolean(key, subject?)`, `number(key)`, `string(key)`, `all()`, `raw(key)`
- `rolloutPercent` is evaluated as `fnv1a32(key + ":" + subject) % 100 < rolloutPercent`;
  outside the rollout a subject gets the negation of the configured value

### `malaise.ts`

`new MalaiseController({ serviceName, engineUrl?, logger? })`, `await controller.start()`.
Unset `MALAISE_ENGINE_URL` disables it entirely.

- polls `GET ${MALAISE_ENGINE_URL}/v1/directives?service=<name>` every 2 s, untraced
- an unreachable engine keeps the last directives until each one's `expiresAt`
- `applyInbound()` is what the app factory calls per request: `latency` sleeps
  `ms ± jitterMs`, `error` throws an `HttpError` with code `malaise_injected`,
  `congestion` returns a permit-release function and throws `503 congested` after
  `queueTimeoutMs`
- `cpu-burn` runs worker threads on a duty cycle, so the event loop stays responsive
  enough to answer probes while the pod's CPU climbs
- `wrapMessageHandler(fn)` for JetStream handlers: latency slows it, `error` and a
  congestion timeout throw `MalaiseNak`, which `consume()` turns into `msg.nak()`
- counts applications in `encabulator_malaise_applied_total{kind}`

### `nats.ts`

| Export | Purpose |
|---|---|
| `connectNats({ url?, name, logger? })` | Connects with infinite reconnect; logs status changes. |
| `ensureStream(nc, logger?)` | Creates `ENCABULATOR` (`encabulator.>`, file storage, limits retention, max age 24 h, 1 replica, duplicate window 2 m) if missing. Never updates an existing stream; tolerates a concurrent create. |
| `EventPublisher` | `publish(subject, data)` wraps data in the envelope; `publishEnvelope` takes one you built. Sets `Nats-Msg-Id` to the event id, injects `traceparent`, and opens a PRODUCER span. |
| `newEnvelope(type, source, data)` | `{ id: "evt_<uuidv7>", type, time, source, data }`. |
| `consume(nc, jsm, opts)` | Ensures the durable pull consumer (explicit ack, `AckWait` 30 s, `MaxDeliver` 5) and consumes it. Each message gets a CONSUMER span parented through the NATS headers; a handler that returns acks, a handler that throws naks. |
| `Subjects`, `Durables` | The subject and durable-name constants from the design. |
| `natsReadiness(nc)` | A `ReadinessCheck` for the connection. |

### `identity.ts`

`identityFromHeaders(headers)`, `identityHeaders(identity)`, `hasRole`, `hasAnyRole`,
`isStaff` (operator, viewer or admin), and the `X-Encabulator-Subject` /
`X-Encabulator-Roles` / `X-Request-Id` header names.

### `ids.ts`, `hash.ts`, `config.ts`, `paths.ts`

`newId('order')` → `ord_<uuidv7>` for every prefix in the design; `nowRfc3339()`;
`fnv1a32` / `rolloutBucket` / `inRollout` over UTF-8 bytes so Go, Python and Rust agree;
`envString` / `envInt` / `envBool` / `requireEnv` / `servicePort` / `logLevel`; and
`isInfraPath()` for the never-traced, never-malaised paths.

## Environment variables the kit reads

| Variable | Default | Effect |
|---|---|---|
| `PORT` | `8080` | `servicePort()` |
| `LOG_LEVEL` | `info` | `logLevel()` |
| `SERVICE_VERSION` | `1.0.0` | Resource attribute `service.version` |
| `OTEL_SERVICE_NAME` | per service | `serviceName(fallback)` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing entirely |
| `MALAISE_ENGINE_URL` | unset | Unset disables fault injection |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | Unset means built-in flag defaults |
| `NATS_URL` | `nats://nats:4222` | `connectNats()` |

## Testing

```bash
make test-unit   # no containers
make test        # adds the JetStream integration suite
```

The integration suite skips cleanly when `DOCKER_HOST` is unset. Locally:

```bash
source ../../../dev-env.sh    # Finch, Docker-API compatible
make test
```

`test/containers.ts` publishes containers on an explicitly chosen free host port: Finch's
CNI portmap plugin rejects the host-port-0 that Testcontainers would otherwise ask for.
