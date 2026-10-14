# sinusoidal-recorder

> Records sinusoidal repleneration, so that it can be reduced.

Unit telemetry for the Encabulator Works. The logarithmic casing line writes a batch of readings
after each fabrication stage; the side fumbling detector reads them back per serial and hunts for the
anomaly that marks a defective unit; the console reads the factory-wide summary for its gauges.
Plane: data. Language: Go. Storage: InfluxDB 3 Core. Design reference:
[section 8.8](../../../docs/design.md).

## API

| Method | Path | Description |
|---|---|---|
| POST | `/v1/readings` | Write a batch of up to 1,000 readings |
| GET | `/v1/readings?serial=&limit=` | Every reading for one unit, oldest first |
| GET | `/v1/summary?window=5m` | Per-metric count, mean and p95 across the whole factory |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness: InfluxDB |
| GET | `/metrics` | Prometheus text |

### POST /v1/readings

```json
{"readings": [
  {"serial": "ENC-turbo-m4x1-9f3a", "orderId": "ord_01J...", "model": "turbo",
   "stage": "fit-marzlevanes", "metric": "repleneration", "value": 1.0142,
   "time": "2026-09-30T12:00:00.120Z"}
]}
```

`202 {"written": 60}`. A batch holds 1 to 1,000 readings; `serial`, `stage` and `metric` are
required, `value` must be finite, and a missing `time` defaults to now. Anything else is
`400 invalid_readings` naming the offending index. A storage failure is `503 service_unavailable`.

Readings land in the InfluxDB table `readings` with tags `serial`, `order_id`, `model`, `stage`,
`metric` and the single field `value`. The three metrics the works produces are `repleneration`,
`marzlevane_alignment` and `grammeter_drift`, 20 samples of each per stage, 10 ms apart in synthetic
time.

### GET /v1/readings

```json
{"serial": "ENC-turbo-m4x1-9f3a", "readings": [{"serial": "...", "metric": "...", "value": 1.01, "time": "..."}]}
```

`limit` is 1 to 50,000 (default 5,000). An unknown serial is an empty array, not a 404: the detector
asks about units the recorder may not have heard of yet.

The read starts five minutes before the time in the serial. The casing line mints a serial as
`ENC-<model>-<base36 milliseconds>-<4 hex>` before the unit's first stage and stamps every reading
with the same clock afterwards, so every reading of the unit is later than that, and InfluxDB opens
only the Parquet files from then on. A serial in any other form reads the last six hours.

### GET /v1/summary

```json
{
  "window": "5m0s",
  "since": "2026-09-30T11:55:00.000Z",
  "metrics": [
    {"metric": "grammeter_drift", "count": 4200, "mean": 0.41, "p95": 3.18},
    {"metric": "marzlevane_alignment", "count": 4200, "mean": 0.02, "p95": 0.39},
    {"metric": "repleneration", "count": 4200, "mean": 1.0, "p95": 1.09}
  ]
}
```

`window` is a Go duration from `1s` to `24h` (default `5m`); anything else is `400 invalid_window`.
p95 uses DataFusion's `approx_percentile_cont`, so it is approximate by design.

### SQL safety

Every read is a parameterized Flight SQL query: the serial and the time lower bounds are bound
values, never interpolated into query text. The only value that reaches the SQL string is the row
limit, and only after being parsed as an integer and clamped to a fixed range. An integration test
pushes `' OR 1=1 --` and friends through the `serial` parameter and asserts they match nothing and
leave the table intact.

## InfluxDB 3 Core notes

Worth knowing, because InfluxDB 3 differs from 1.x and 2.x in ways that bite:

- **One port does both protocols.** 8181 serves the HTTP line-protocol write API *and* the Flight
  SQL (gRPC) query API. There is no second port to expose.
- **An empty token is supported.** InfluxDB 3 Core started with `--without-auth` accepts any token or
  none, which is how the in-cluster deployment runs in v1, so `INFLUXDB_TOKEN` may be empty. The Go
  client refuses to construct with an empty token, so the recorder substitutes a harmless
  placeholder; the server ignores it. Set a real token and it is sent as `Authorization: Bearer`.
- **`/health` is authenticated.** Unless the server was started with `--without-auth` or
  `--disable-authz health`, `GET /health` needs a token, so the readiness check sends one when it
  has one. Do not use `/ready`; InfluxDB 3 Core does not implement it.
- **Queries do not auto-create.** Writes create the database and table implicitly, but a query
  against a table that does not exist yet is an error. The recorder therefore creates the database
  and the `readings` table at startup (both idempotent, both safe from several replicas), so
  `/v1/summary` answers `{"metrics": []}` from the moment the works opens instead of failing until
  the first unit is built.
- **Writes are visible after a short lag.** A read immediately after a write can miss rows still in
  the write buffer. The detector tolerates this; the integration tests poll.
- **Every read needs a time bound.** Core writes one Parquet file per gen1 block and does not
  compact them, so a query with no time bound opens every file the table has ever had, gets slower
  as the table ages, and fails outright once it would scan more files than `--query-file-limit`
  (432 by default): "Query would scan 432 Parquet files, exceeding the file limit". Both reads carry
  a lower time bound: the summary its window, the per-serial read the time in the serial.
- **Types on the way back.** Tags arrive as `string`, `value` as `float64`, `time` as `time.Time`.

## Tracing

Inbound requests get server spans from the kit. Every InfluxDB call the API makes (the batch
write, the per-serial read and the summary read) gets a `CLIENT` span named `write readings` or
`SELECT readings`, with `db.system.name=influxdb`, `db.namespace` (the database),
`db.collection.name=readings`, `db.operation.name`, `server.address` and `server.port`, and the
error status when the call fails. Nothing instruments InfluxDB itself, so these spans are the
only evidence of the recorder's traffic to it: the collector's `service_graph` connector turns a
client span carrying `db.system.name` into a database edge named `influxdb`, which is the
console's InfluxDB node. The readiness check against `/health` is not traced.

## Environment

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | API, health and metrics on one port |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `sinusoidal-recorder` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `MALAISE_ENGINE_URL` | unset | Unset disables the malaise middleware |
| `INFLUXDB_URL` | **required** | `http://influxdb:8181` |
| `INFLUXDB_TOKEN` | empty | May be empty when InfluxDB runs without auth |
| `INFLUXDB_DATABASE` | `sinusoidal` | |
| `SERVICE_VERSION` | `dev` | Reported as `service.version` |

## Metrics

Besides `http_server_request_duration_seconds` and the Go runtime metrics:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `encabulator_readings_written_total` | counter | · | Samples stored |
| `encabulator_readings_write_seconds` | histogram | · | Batch write duration |
| `encabulator_recorder_queries_total` | counter | `endpoint`, `result` | `endpoint` is `write`, `readings` or `summary`; `result` is `ok` or `error` |
| `encabulator_malaise_applied_total` | counter | `kind` | From the kit's malaise middleware |

## Running locally

```bash
docker run -d --name recorder-influx -p 8181:8181 influxdb:3-core \
  influxdb3 serve --node-id=local --object-store=memory --without-auth

export INFLUXDB_URL=http://127.0.0.1:8181
export INFLUXDB_DATABASE=sinusoidal
make run

curl -sX POST localhost:8080/v1/readings -d '{"readings":[
  {"serial":"ENC-demo","orderId":"ord_demo","model":"turbo","stage":"fit-marzlevanes",
   "metric":"repleneration","value":1.02,"time":"2026-09-30T12:00:00.000Z"}]}' | jq
curl -s 'localhost:8080/v1/readings?serial=ENC-demo' | jq
curl -s 'localhost:8080/v1/summary?window=5m' | jq
```

```bash
make build             # bin/sinusoidal-recorder
make test              # unit tests: handlers and validation, no containers
make test-integration  # adds a real influxdb:3-core via Testcontainers
make lint              # gofmt -l plus go vet
make image             # builds from the repository root
```

`make test-integration` needs a container runtime; `source ../../../dev-env.sh` first. Without one
the integration tests skip rather than fail.

## Deployment notes

- Probes: `GET /healthz` for liveness, `GET /readyz` for readiness. Readiness is an authenticated
  `GET /health` against InfluxDB and nothing else.
- InfluxDB must be reachable at startup for the database and table to be created; if it is not, the
  recorder logs a warning and carries on, and the first write creates them implicitly. It does not
  crash-loop waiting for the store.
- Stateless; scale horizontally. Database and table creation are idempotent from every replica.
- Needs no Kubernetes API access and no service account token.
- Resource hints: 100m CPU / 128Mi memory requests, 500m / 512Mi limits. Write volume is the driver:
  seven stages x 60 samples per unit, so a busy line is 420 points per unit.
- InfluxDB 3 Core itself is the one to watch on resources; it is memory-hungry relative to the
  application pods. The reference deployment uses a `file` object store on a PersistentVolume.
