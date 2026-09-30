# logarithmic-casing-line

> "…surmounted by a malleable logarithmic casing in such a way that the two main spurving bearings
> were in a direct line with the pentametric fan." (J. H. Quick, 1944)

The fabrication workers. The casing line pulls `encabulator.fabrication.requested` off JetStream,
assigns each unit a serial, walks it through the seven canonical assembly stages, writes the
sinusoidal repleneration telemetry that inspection will judge it by, and announces when the order
comes off the line. Design section 8.4.

- Plane: processing (`encabulator.io/plane=processing`)
- Language: Python 3.14, FastAPI, `nats-py`, httpx, numpy
- Owns: nothing
- Readiness depends on: its NATS connection

## What it does

For each `fabrication.requested` message (design section 4.2):

1. Take a slot. `LINE_CAPACITY` orders are fabricated at once per pod; units *within* an order are
   built one after another.
2. For each unit, mint a serial `ENC-<model>-<base36 ms>-<4 hex>` and run the seven stages:
   1. `prelaminate-base-plate` (300 ms)
   2. `surmount-logarithmic-casing` (400 ms)
   3. `align-spurving-bearings` (500 ms)
   4. `fit-marzlevanes` (700 ms)
   5. `wind-lotus-o-delta` (600 ms)
   6. `connect-tremie-pipes` (400 ms)
   7. `synchronize-cardinal-grammeters` (300 ms)

   Each base duration is multiplied by the model's stage time factor (turbo 1.0, retro 1.3, hyper
   1.8), halved when `turbo-encabulation` is on, and jittered ±25%.
3. Per stage: generate 20 samples of three metrics, `POST` them to `sinusoidal-recorder /v1/readings`
   as one batch, and publish `encabulator.fabrication.stage`.
4. When every unit is done: `POST depot /v1/reservations/{id}/commit` (first attempt only), then
   publish `encabulator.fabrication.completed`.

Injected malaise applies per message: `latency` slows the line down, `error` naks the message,
`congestion` limits how many orders are in the handler at once.

### Defects are grown, not labelled

**The line never says which units are defective.** It decides, per unit, whether the telemetry
will contain an anomaly · with probability equal to the model's base defect rate (turbo 4%, retro
3%, hyper 6%), reduced by `dingle-arm-damping` percent when a drawn reciprocation dingle arm was
ordered · and then it generates telemetry that contains one. `side-fumbling-detector` has to find
it in the numbers. The two services share nothing but the section 4.3 contract.

The signals (`src/casing_line/signals.py`):

| Metric | Shape | Noise |
|---|---|---|
| `repleneration` | sinusoidal around 1.0, one cycle per stage, amplitude 0.03 | sigma 0.05 |
| `marzlevane_alignment` | degrees of misalignment around 0 | sigma 0.2 |
| `grammeter_drift` | cumulative ppm around 0, a random walk over the whole unit | step sigma 0.5 |

An anomaly lands in one metric during one stage, as one of three shapes, injected into the
metric's *innovation* (the per-sample noise for the first two, the per-sample step for the walk) so
all three share one fault geometry:

- `spike` · one sample thrown off, and half as much again on the next.
- `drift` · the metric steps to a new level and holds it for the rest of the stage. (A linear ramp
  would be a worse fault than it looks: a trailing window's standard deviation grows with the
  slope, so a ramp partly hides itself.)
- `phase-slip` · the lotus-o-delta winding slips half a cycle and the repleneration starts swinging
  at the fault's magnitude. Repleneration only: nothing else has a phase to slip.

Magnitudes are drawn uniformly from 4 to 12 times the metric's noise sigma. That spread is
deliberate: it straddles the detector's two thresholds, so default inspection catches roughly 86%
of defects and strict inspection roughly 96%. That difference, plus strict mode's false positives,
is what makes the `side-fumbling-outbreak` scenario visible on the console.

The repleneration amplitude is small (0.03) for a reason: the strict acceptable band is
[0.8, 1.2], which is only four noise sigmas wide, so a larger swing would put clean units outside
it constantly.

## API

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/line` | What this pod is fabricating right now |
| GET | `/healthz`, `/readyz`, `/metrics` | Platform endpoints (see the kit) |

```json
GET /v1/line
{
  "capacity": 2,
  "activeOrders": 1,
  "activeUnits": [
    {"orderId": "ord_01K6…", "serial": "ENC-retro-MUO9N-4A2C", "stage": "fit-marzlevanes"}
  ],
  "unitsCompleted": 417
}
```

This is per pod, not per fleet: the console sums it across replicas.

## Events

| Direction | Subject | Data |
|---|---|---|
| in | `encabulator.fabrication.requested` | `orderId, attempt, model, quantity, options, reservationId` |
| out | `encabulator.fabrication.stage` | `orderId, attempt, serial, stage, stageIndex, durationMs` |
| out | `encabulator.fabrication.completed` | `orderId, attempt, model, serials[], durationMs` |

Durable pull consumer `logarithmic-casing-line`, filter `encabulator.fabrication.requested`,
explicit ack, `AckWait` 30s, `MaxDeliver` 5, `MaxAckPending` 64, with in-progress acks while a
long order is on the line. **Consumer lag on this durable is the congestion signal KEDA scales on.**

## Flags read

| Flag | Effect |
|---|---|
| `turbo-encabulation` | Halves every stage duration |
| `dingle-arm-damping` | Percent by which a fitted dingle arm reduces the defect rate (default 50) |

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `logarithmic-casing-line` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | unset ⇒ tracing is a no-op |
| `MALAISE_ENGINE_URL` | unset | unset ⇒ no malaise middleware |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | unset ⇒ built-in flag defaults |
| `NATS_URL` | `nats://nats:4222` | |
| `LINE_CAPACITY` | `2` | orders fabricated at once per pod; also the consumer's concurrency |
| `SINUSOIDAL_RECORDER_URL` | unset | unset ⇒ readings are discarded (local runs only) |
| `PREFABULATED_AMULITE_DEPOT_URL` | unset | unset ⇒ reservations are not committed |
| `SHUTDOWN_READINESS_GRACE_SECONDS` | `3` | |

## Failure behaviour

- **Recorder write fails** (after the kit's retries): the message is naked and the order is
  refabricated. Losing readings would blind the detector, so it is treated as fatal to the message.
- **Depot commit fails**: logged and carried on. The parts stay reserved and the depot's sweeper
  releases them after 15 minutes; the order is not held up for bookkeeping.
- **Unknown model**: logged and acked. Redelivering it five times would not make the model exist.

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_units_fabricated_total` | counter | `model` |
| `encabulator_stage_duration_seconds` | histogram | `stage` |
| `encabulator_line_active_orders` | gauge | |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |
| `encabulator_malaise_applied_total` | counter | `kind` |

## Running locally

```bash
make build
make test                   # unit tests; NATS tests skip without a container runtime
source ../../../dev-env.sh && make test    # adds the NATS 2.12 container tests
make lint
make run                    # needs a JetStream-enabled NATS on localhost:4222

finch run -d --name enc-nats -p 4222:4222 nats:2.12 -js
```

```bash
make image                  # build context is the repository root
```
