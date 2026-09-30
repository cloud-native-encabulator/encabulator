# `libs/go/kit` — the Go service kit

> The differential girdlespring is fitted to the "up" end of the grammeters; this package is fitted
> to the "up" end of every Go service.

`github.com/jlbutler/cloud-native-encabulator/libs/go/kit` implements the cross-cutting conventions
of [design section 7](../../../docs/design.md) so that a Go service can be written as domain logic
plus a handful of route registrations. Go services depend on it through a `replace` directive:

```
require github.com/jlbutler/cloud-native-encabulator/libs/go/kit v0.0.0
replace github.com/jlbutler/cloud-native-encabulator/libs/go/kit => ../../../libs/go/kit
```

## The short version

```go
func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	rt, err := kit.Bootstrap(ctx, kit.Options{
		Service:      "hydrocoptic-datastore",
		ApplyMalaise: true, // processing, data and support planes only
	})
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()

	rt.Server.AddReadyCheck("postgres", pool.Ping)
	rt.Server.Handle("POST /v1/orders", api.CreateOrder(store))
	rt.Server.Handle("GET /v1/orders/{id}", api.GetOrder(store))

	return rt.Run(ctx) // serves until SIGTERM, then drains
}
```

`Bootstrap` reads the environment, builds the logger, installs tracing, creates the metric
registry, mounts `/healthz`, `/readyz` and `/metrics`, builds the flag client, and installs the
malaise middleware. `Run` starts the flag and malaise pollers, serves, and drains.

## Packages

| Package | What it gives you |
|---|---|
| `kit` | `Bootstrap`, `Runtime`, `Runtime.Client`, `Runtime.Run` |
| `kit/config` | `String`, `Int`, `Float`, `Bool`, `Duration`, `Require`, `ServiceURL`, `URLVar` |
| `kit/logging` | `New`, `WithRequestID`, `RequestID` — JSON slog handler that decorates records from the context |
| `kit/telemetry` | `Setup` — OTLP/HTTP tracing, no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset |
| `kit/httpx` | `Server`, `Client`, middleware, `WriteJSON`, `WriteError`, `DecodeJSON`, `StatusError` |
| `kit/identity` | `Identity`, `FromRequest`, `Apply`, `Strip`, role constants |
| `kit/malaise` | `Injector` — directive poller, HTTP middleware, `Apply` / `WrapMessage` for consumers |
| `kit/flags` | `Client` — ETag-polling girdlespring client, `Defaults`, `InRollout` |
| `kit/events` | `Connect`, `EnsureStream`, `Publisher`, `Consume`, `Envelope`, subject constants |
| `kit/testsupport` | test-only: `RequireContainerRuntime`, `StartContainer`, `FreePort` |

## Logging

`logging.New(service, level)` writes one JSON object per line to stdout with `time` (RFC 3339 with
milliseconds), `level`, `msg` and `service`. `trace_id`, `span_id` and `request_id` are pulled from
the context at the moment a record is written, so use the `*Context` methods:

```go
log.InfoContext(ctx, "order accepted", slog.String("order_id", id))
```

## HTTP server

`httpx.NewServer` mounts `GET /healthz`, `GET /readyz` and `GET /metrics`. Those three are
deliberately outside the API middleware chain, so they are never traced, never metered, and never
subject to injected malaise.

Register API routes with Go 1.22 `ServeMux` patterns. The pattern's path becomes the `http_route`
metric label and the server span name, so `http_route` is always a template and never a raw path:

```go
s.Handle("GET /v1/orders/{id}", h) // http_route="/v1/orders/{id}"
```

Per-route chain, outermost first: OTel server span, `http_server_request_duration_seconds`, panic
recovery, then anything added with `Use` (the malaise middleware lands here). `RequestID` and a
second panic-recovery net wrap the whole mux.

`Use` bakes its middleware into each route at registration time, so **call `Use` before `Handle`**.
`Bootstrap` already does this for the malaise middleware.

`AddReadyCheck(name, fn)` adds a readiness probe. Readiness must only check the service's *own* hard
dependencies — its database, its NATS connection — never a downstream service. Checks run in
parallel with a 2s budget; `/readyz` reports each one by name. On SIGTERM, `/readyz` fails first,
then the listener closes, then in-flight requests drain for up to 20 seconds.

## HTTP client

`httpx.NewClient` produces a client aimed at one upstream service:

```go
quoter, _ := rt.Client("spurving-bearing-quoter") // reads SPURVING_BEARING_QUOTER_URL
status, err := quoter.JSON(ctx, http.MethodPost, "/v1/quotes", req, &quote,
	httpx.WithIdentity(id))
```

- 3s default timeout, one connection pool per upstream.
- Client spans named `<METHOD> <service>` with `peer.service` set to the service name, and the
  `http_client_request_duration_seconds` metric labelled `server_address="<service name>"` rather
  than `host:port`, so the label stays readable and bounded.
- GET, HEAD, PUT, DELETE and OPTIONS retry twice on a connection error, 429, 502, 503 or 504, with
  full-jitter exponential backoff from a 100ms base. POST retries only when the caller passes
  `httpx.WithIdempotencyKey`.
- A non-2xx response carrying the standard error envelope comes back as a `*httpx.StatusError` with
  the upstream's code intact, so a handler can pass the upstream's `insufficient_parts` straight
  through.

## Malaise

```go
inj := rt.Malaise // nil when MALAISE_ENGINE_URL is unset; every method is nil-safe
```

`Bootstrap` installs `Injector.Middleware` on the API routes when `ApplyMalaise` is set. Message
consumers run their own handler through `Apply` (or `WrapMessage`, for a
single-argument handler):

```go
events.Consume(ctx, stream, cfg, func(ctx context.Context, env events.Envelope, _ jetstream.Msg) error {
	return rt.Malaise.Apply(ctx, func(ctx context.Context) error {
		return process(ctx, env)
	})
})
```

`Apply` returns `malaise.ErrInjected` when an error directive fires and
`malaise.ErrCongested` when a congestion queue times out; both are errors, so the kit's `Consume`
naks the message, which is exactly what the design asks for.

`latency`, `error`, `congestion` and `cpu-burn` are all honored; `cpu-burn` runs whole-core spinners
plus one duty-cycled spinner for the remainder, started and stopped by the poll loop. The poller
uses an uninstrumented HTTP client, so the 2s loop never appears in a trace, and it keeps the last
known directives when the engine is unreachable until each one's `expiresAt`.

## Flags

```go
if rt.Flags.Bool(flags.TurboEncabulation) { /* halve the stage durations */ }
rate := rt.Flags.Number(flags.GatewayRateLimitRPS)
if rt.Flags.BoolFor(flags.StrictSideFumbling, subject) { /* per-subject rollout */ }
```

`flags.Defaults()` holds the built-in fallbacks from design section 6; a key the girdlespring has
never served falls back to its default. `flags.InRollout(key, subject, pct)` is the shared rollout
predicate, `fnv1a32(key + ":" + subject) % 100 < pct`. A boolean flag carrying `rolloutPercent`
returns its value to subjects inside the rollout and the opposite value to everyone else.

## Events

```go
nc, js, err := events.Connect(os.Getenv("NATS_URL"), rt.Service)
stream, err := events.EnsureStream(ctx, js)      // creates ENCABULATOR if missing, never updates it
pub := events.NewPublisher(js, rt.Service)
_, err = pub.Publish(ctx, events.SubjectOrderAccepted, payload)

cc, err := events.Consume(ctx, stream, events.ConsumerConfig{
	Durable: "hydrocoptic-datastore",
	Filter:  "encabulator.>",
}, func(ctx context.Context, env events.Envelope, msg jetstream.Msg) error {
	return apply(ctx, env)
})
defer cc.Stop()
```

`Publish` sets `Nats-Msg-Id` to the event id for the stream's 2-minute duplicate window, injects
W3C trace context into the NATS headers, and records a `PRODUCER` span. `Consume` creates a durable
pull consumer with explicit ack, `AckWait` 30s and `MaxDeliver` 5, extracts the trace context, and
runs the handler inside a `CONSUMER` span that is a child of the producer's — which is what makes
the console's service graph draw the messaging edges. A handler returning an error naks; returning
nil acks; an undecodable message is terminated rather than looped.

## Testing

```bash
make test         # unit tests only
make test-integration   # requires a container runtime
make lint         # gofmt -l plus go vet
```

Integration tests call `testsupport.RequireContainerRuntime(t)` first and skip cleanly when
`DOCKER_HOST` is unset or unreachable. `testsupport.StartContainer` binds an explicit loopback host
port rather than letting the daemon choose, because Finch's containerd/CNI stack rejects the
"any free port" binding Testcontainers asks for by default.

## Deliberate omissions

The kit does not wrap Postgres, Valkey or InfluxDB. Each service owns its own store, its own
migrations and its own `db.*` span attributes; a lowest-common-denominator data layer would have
been a worse fit than four honest ones.
