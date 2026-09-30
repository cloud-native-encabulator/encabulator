# side-fumbling-detector

> "The lotus-o-delta type winding was used…so fitted to the ambifacient lunar waneshaft that side
> fumbling was effectively prevented." (J. H. Quick, 1944)

Prevention being hard, we settle for detection. The detector pulls
`encabulator.fabrication.completed`, fetches every unit's telemetry from `sinusoidal-recorder`,
looks for the anomaly the casing line may have grown in it, and publishes a verdict. Design
section 8.11.

- Plane: support (`encabulator.io/plane=support`)
- Language: Python 3.14, FastAPI, `nats-py`, httpx, numpy, `redis` (async, Valkey-compatible)
- Owns: Valkey keys `incident:*`
- Readiness depends on: its NATS connection and Valkey

## How detection works

The casing line never labels a defective unit. It writes telemetry, and a defective unit's
telemetry contains a spike, a drift, or a phase slip in one metric during one stage. All the
detector gets is the design section 4.3 contract:

- three metrics · `repleneration` (around 1.0), `marzlevane_alignment` (around 0), and
  `grammeter_drift` (a cumulative random walk around 0)
- 20 samples per metric per stage, 10 ms apart
- the nominal per-sample noise scale of each: 0.05, 0.2, and 0.5 per step

**The rule** (design section 4.3): fail a unit if any metric in any stage has a rolling z-score
above **4.0** (window 10), or if `repleneration` leaves **[0.7, 1.3]**. With
`strict-side-fumbling` on, that tightens to z > **3.0** and **[0.8, 1.2]**.

The design leaves four things open. `src/detector/analysis.py` decides them like this:

1. **The window is trailing and excludes the sample it scores.** If a sample sat inside its own
   window of ten, the largest possible z would be 9/√10 = 2.85 and neither threshold could ever
   fire. Scoring a sample against the ten before it is the only formulation under which 4.0 and 3.0
   mean anything.
2. **The window runs over the unit's whole series, not one stage.** A 20-sample stage would
   otherwise leave its first ten samples unscored, and a fault in them invisible.
3. **`grammeter_drift` is scored on its first differences.** It is a random walk by construction,
   so its level wanders freely and a z-score on the level would fire constantly. The drift *rate* is
   the quantity that is supposed to be stable, and a fault shows up as a step in the rate.
4. **The window's standard deviation has a floor** of 1.3 × the metric's nominal noise scale.
   Without it, ten unusually quiet samples manufacture enormous z-scores out of ordinary noise.

That floor is the one calibrated number here, and the tests pin what it buys:

| Mode | False positives on clean units | Defects caught |
|---|---|---|
| default (z > 4.0, band [0.7, 1.3]) | ~0% | ~86% |
| strict (z > 3.0, band [0.8, 1.2]) | ~13% | ~96% |

Which is precisely what design section 4.3 asks for: strict mode "catches subtler anomalies and
also produces false positives, so the refabrication rate visibly rises".

`tests/generator.py` is an **independent** implementation of the 4.3 signal contract, written for
these tests. The detector has never seen the casing line's generator and vice versa; if the two
drift apart, the tests notice.

## API

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/incidents?limit=50` | Recent units that fumbled, newest first (Valkey list, capped at 500) |
| GET | `/v1/stats` | `{inspected, passed, failed, failRate5m}` |
| GET | `/healthz`, `/readyz`, `/metrics` | Platform endpoints (see the kit) |

```json
GET /v1/incidents?limit=2
[
  {"id": "inc_01K6…", "orderId": "ord_01K6…", "serial": "ENC-hyper-MUO9N-0C41",
   "stage": "fit-marzlevanes", "metric": "marzlevane_alignment", "score": 7.412,
   "reason": "rolling z-score 7.41 exceeds 4.0", "strict": false,
   "time": "2026-09-30T12:00:03.412Z"}
]
```

`inspected`, `passed`, and `failed` are cumulative per pod; `failRate5m` is a rolling five-minute
window.

## Events

| Direction | Subject | Data |
|---|---|---|
| in | `encabulator.fabrication.completed` | `orderId, attempt, model, serials[], durationMs` |
| out | `encabulator.inspection.passed` | `orderId, attempt, serials[]` (every serial) |
| out | `encabulator.inspection.failed` | `orderId, attempt, serials[], findings[{serial, stage, metric, score, reason}]` |

On `inspection.failed`, `serials[]` holds the units that actually failed · the ones the findings
refer to. The processor refabricates the order as a whole.

Durable pull consumer `side-fumbling-detector`, filter `encabulator.fabrication.completed`,
explicit ack, `AckWait` 30s, `MaxDeliver` 5.

## Flags read

| Flag | Effect |
|---|---|
| `strict-side-fumbling` | Tightens both thresholds, and is recorded on each incident |

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `side-fumbling-detector` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | unset ⇒ tracing is a no-op |
| `MALAISE_ENGINE_URL` | unset | unset ⇒ no malaise middleware |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | unset ⇒ built-in flag defaults |
| `NATS_URL` | `nats://nats:4222` | |
| `SINUSOIDAL_RECORDER_URL` | unset | unset ⇒ every unit passes; set it in the chart |
| `VALKEY_URL` | unset | `redis://default:<pw>@valkey:6379/0`; unset ⇒ incidents are not recorded |
| `INSPECTION_CONCURRENCY` | `2` | orders inspected at once per pod |
| `SHUTDOWN_READINESS_GRACE_SECONDS` | `3` | |

## Failure behaviour

- **No readings yet for a serial**, or the recorder returns an error: the message is naked and
  retried. The casing line writes readings before it publishes `fabrication.completed`, so this is
  a transient race, and inspecting a unit with no data would pass it by default.
- **Valkey is unreachable**: logged and carried on. The verdict travels over NATS; losing the
  incident record is cosmetic. Readiness still reports Valkey as failed, so the pod leaves rotation
  while its HTTP reads would be empty.

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_units_inspected_total` | counter | `result` (`passed`, `failed`) |
| `encabulator_anomaly_score` | histogram | (worst z-score per unit, recorded whether it passed or failed) |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |
| `encabulator_malaise_applied_total` | counter | `kind` |

## Running locally

```bash
make build
make test                   # unit tests; NATS/Valkey tests skip without a container runtime
source ../../../dev-env.sh && make test    # adds the NATS 2.12 and Valkey 9 container tests
make lint
make run                    # needs a JetStream NATS; Valkey and the recorder are optional

finch run -d --name enc-nats -p 4222:4222 nats:2.12 -js
finch run -d --name enc-valkey -p 6379:6379 valkey/valkey:9
```

```bash
make image                  # build context is the repository root
```
