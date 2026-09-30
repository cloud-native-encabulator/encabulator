# differential-girdlespring

> The differential girdlespring is fitted to the "up" end of the grammeters, where it prevents
> reciprocation of the feature set without a written change record.

Feature flags for the Encabulator Works. One Postgres table, one poll-friendly list endpoint, and an
ETag so that five services checking in every five seconds cost almost nothing. Plane: support.
Language: Go. Design reference: [section 8.10](../../../docs/design.md).

Clients use the flag client in [`libs/go/kit/flags`](../../../libs/go/kit/README.md) (or its
TypeScript, Python and Rust equivalents): poll `GET /v1/flags` every five seconds with
`If-None-Match`, keep the last good set, and fall back to built-in defaults when the girdlespring is
unreachable at startup. A girdlespring outage therefore never stops the factory; it only freezes
the flags.

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/flags` | any | The whole flag set with a strong `ETag`; `If-None-Match` yields `304` |
| GET | `/v1/flags/{key}` | any | One flag |
| GET | `/v1/flags/{key}/evaluate?subject=` | any | The value a given subject sees, plus the reason |
| PUT | `/v1/flags/{key}` | operator, admin | Set a flag's value, rollout and description |
| GET | `/healthz` | · | Liveness |
| GET | `/readyz` | · | Readiness: Postgres |
| GET | `/metrics` | · | Prometheus text |

Flag JSON:

```json
{
  "key": "turbo-encabulation",
  "type": "boolean",
  "value": false,
  "rolloutPercent": 25,
  "description": "Halve stage durations; the quoter adds a 15% turbo surcharge.",
  "updatedAt": "2026-09-30T12:00:00.000Z",
  "updatedBy": "opal"
}
```

`PUT` body: `{"value": <boolean|number>, "rolloutPercent": 0-100, "description": "..."}`. The value
must match the flag's declared type, `rolloutPercent` is booleans only, and unknown keys are `404` -
the flag set is the one in the design, not whatever a caller invents. Omitting `description` keeps
the existing text; omitting `rolloutPercent` clears it. Every edit records `updatedBy` from
`X-Encabulator-Subject` and `updatedAt` from the database clock.

### Rollout evaluation

A boolean flag may carry `rolloutPercent`. A subject is inside the rollout when
`fnv1a32(key + ":" + subject) % 100 < rolloutPercent`; inside, the subject sees the flag's value,
and outside it sees the opposite. `/evaluate` reports which happened:

| `reason` | Meaning |
|---|---|
| `applies_to_everyone` | No `rolloutPercent`, or a number flag |
| `in_rollout` | Subject is inside the rollout; `value` is the flag's value |
| `out_of_rollout` | Subject is outside; `value` is inverted |

The same predicate is exported as `flags.InRollout` in the Go kit, and a test asserts that the
server and the client agree subject by subject.

### Errors

`{"error": {"code": "...", "message": "..."}}` with codes `forbidden` (not operator or admin),
`not_found`, `invalid_flag_value`, `invalid_rollout`, `invalid_description`, `invalid_json`.

## Flags it serves

Seeded on first start, insert-if-missing, and never overwritten afterwards · a restart will not undo
an operator's change.

| Key | Type | Default | Read by |
|---|---|---|---|
| `turbo-encabulation` | boolean | `false` | casing-line, quoter |
| `quote-cache` | boolean | `true` | quoter |
| `retro-encabulator-sales` | boolean | `true` | processor |
| `strict-side-fumbling` | boolean | `false` | detector |
| `amulite-foundry-rate` | number | `20` | depot |
| `gateway-rate-limit-rps` | number | `50` | gateway |
| `dingle-arm-damping` | number | `50` | casing-line |

## Environment

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | API, health and metrics on one port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTEL_SERVICE_NAME` | `differential-girdlespring` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables the malaise middleware |
| `DATABASE_URL` | **required** | `postgres://girdlespring:<pw>@postgres:5432/girdlespring?sslmode=disable` |
| `SERVICE_VERSION` | `dev` | Reported as `service.version` |

The service does not read `DIFFERENTIAL_GIRDLESPRING_URL`; it is the girdlespring.

## Metrics

Besides the conventional `http_server_request_duration_seconds` and the Go runtime metrics:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `encabulator_flag_updates_total` | counter | `key` | Flag edits accepted |
| `encabulator_flag_polls_total` | counter | `result` | `GET /v1/flags` outcomes: `modified` or `not_modified` |

`encabulator_flag_polls_total{result="not_modified"}` should dominate: if it does not, the ETag is
not doing its job.

## Schema and migrations

One table, `flags`, plus a `schema_migrations` bookkeeping table. Migrations are embedded SQL under
`internal/store/migrations/`, applied in filename order at startup, each in its own transaction, and
recorded by name so a restart or a second replica is a no-op. Adding a migration means adding a
file; nothing else changes.

## Running locally

```bash
docker run -d --name girdlespring-pg -p 5432:5432 \
  -e POSTGRES_USER=girdlespring -e POSTGRES_PASSWORD=prefamulated \
  -e POSTGRES_DB=girdlespring postgres:18

export DATABASE_URL='postgres://girdlespring:prefamulated@127.0.0.1:5432/girdlespring?sslmode=disable'
make run

curl -s localhost:8080/v1/flags | jq
curl -s localhost:8080/v1/flags/turbo-encabulation/evaluate?subject=wendy | jq
curl -sX PUT localhost:8080/v1/flags/turbo-encabulation \
  -H 'X-Encabulator-Subject: opal' -H 'X-Encabulator-Roles: operator' \
  -d '{"value":true,"rolloutPercent":25}' | jq
```

```bash
make build             # bin/differential-girdlespring
make test              # unit tests, no containers needed
make test-integration  # adds a real postgres:18 via Testcontainers
make lint              # gofmt -l plus go vet
make image             # builds from the repository root
```

`make test-integration` needs a container runtime; `source ../../../dev-env.sh` first. Without one
the integration tests skip rather than fail.

## Deployment notes

- Probes: `GET /healthz` for liveness, `GET /readyz` for readiness. Readiness checks Postgres only.
- Migrations run in-process at startup, so no Job or init container is needed. They are safe to run
  concurrently from several replicas.
- Stateless apart from Postgres; scale horizontally. The default in the chart is a single replica,
  which is plenty for a 5-second poll from a dozen pods.
- Needs no Kubernetes API access and no service account token.
- Resource hints: 50m CPU / 64Mi memory requests, 200m / 128Mi limits. The workload is one indexed
  read per poll.
