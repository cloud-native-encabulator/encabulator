# demo-scenario-orchestrator

> "For a number of years now, work has been proceeding in order to bring to perfection the crudely
> conceived idea of a machine that would not only supply inverse reactive current…"

Rehearses the demo so the presenter does not have to. Each scenario is a script: narration the
console shows on its ticker, traffic sent to the cardinal grammeter, faults injected through the
malaise engine, flags flipped in the differential girdlespring, and waits on real Prometheus
queries. Design section 8.14.

- Plane: control (`encabulator.io/plane=control`)
- Language: Python 3.14, FastAPI, httpx, PyYAML
- Owns: in-memory run history
- Readiness depends on: nothing. Every upstream is optional, and the orchestrator never blocks
  readiness on somebody else's health.
- Does **not** apply malaise to itself (design section 7).

## Built-in scenarios

| Id | Steps | What the audience sees |
|---|---|---|
| `morning-shift` | 14 | Traffic ramps from a trickle to a sinusoidal load. Nothing breaks, which is the point. |
| `marzlevane-shortage` | 17 | The amulite foundry is switched off, the cage empties, reservations return 409 `insufficient_parts`, then the foundry is relit and the works recovers unassisted. |
| `hydrocoptic-outage` | 15 | A PostgreSQL pod is deleted for real. The processor's circuit breaker opens, reservations are compensated, and the ledger catches up from JetStream when the pod returns. |
| `side-fumbling-outbreak` | 17 | The casing line is slowed and inspection tightened. Consumer lag builds, KEDA scales, and the refabrication rate visibly rises · partly from real defects, partly from strict mode's false positives. |
| `dingle-arm-congestion` | 16 | The gateway is congested, then rate limited, so `503 congested` and `429 rate_limited` can be told apart on one screen. |
| `grand-tour` | 50 | All five acts in order, narrated in full encabulator prose. Roughly twenty minutes. |

```bash
make scenarios      # list and validate the built-ins without starting a server
```

## Scenario format

Built-ins live in the image at `src/orchestrator/scenarios/*.yaml`. Extras are mounted from a
ConfigMap at `SCENARIO_DIR` (default `/etc/scenarios`) and **may override a built-in by reusing its
id**, which is how a presenter tweaks a scenario without rebuilding anything. A malformed mounted
file is logged and skipped, never fatal.

```yaml
id: house-special
title: House Special
description: A short one.
steps:
  - say: "Narration, shown on the console's ticker."
  - traffic: {running: true, rps: 8, pattern: sine, periodSeconds: 120, customers: 40}
  - malaise:
      ref: slow-line                  # the handle clearMalaise uses
      target: logarithmic-casing-line
      kind: latency
      params: {ms: 900, jitterMs: 300, percent: 100}
      durationSeconds: 300
      narration: "Optional: what to say instead of the generated summary."
  - flag: {key: strict-side-fumbling, value: true}     # add keep: true to leave it set
  - wait: 30s
  - waitFor: {promql: rate(encabulator_refabrications_total[2m]), op: ">", value: 0, timeout: 300s}
  - restock: {sku: HYD-MARZ, quantity: 3000}
  - clearMalaise: slow-line           # or `all`
```

| Step | Becomes |
|---|---|
| `say` | a narration entry in the run log |
| `traffic` | `PUT cardinal-grammeter /v1/traffic` with the mapping as the body |
| `malaise` | `POST malaise-injection-engine /v1/malaises`; the created id is remembered under `ref` |
| `clearMalaise` | `DELETE /v1/malaises/{id}` for one `ref`, or every malaise this run created |
| `flag` | `PUT differential-girdlespring /v1/flags/{key}`, restored at the end unless `keep: true` |
| `wait` | sleep. `30s`, `2m`, `500ms`, `1h`, or a bare number of seconds |
| `waitFor` | poll Prometheus `/api/v1/query` every `interval` (default 2s) until `promql op value`, up to `timeout` (default 120s) |
| `restock` | `POST prefabulated-amulite-depot /v1/parts/{sku}/restock` |

`restock` is an **addition** to the design's list of step types. Design Appendix A gives the
orchestrator a `PREFABULATED_AMULITE_DEPOT_URL` but no step that uses it; pouring fresh castings
into the cage is what it is for, and it gives `marzlevane-shortage` a satisfying recovery beat that
does not depend on waiting out the foundry's 10-second tick.

A `waitFor` that times out logs a warning and the run **carries on**. A demo should not abort
because a metric took longer than expected to move; the narration says what was observed and the
presenter can draw their own conclusion.

## API

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/scenarios` | Every scenario, with step summaries |
| GET | `/v1/scenarios/{id}` | One scenario |
| POST | `/v1/scenarios/{id}/runs` | Start a run. `202` with the run, `409 run_in_progress` if one is active |
| GET | `/v1/runs?limit=20` | Run history, newest first, without the log |
| GET | `/v1/runs/{id}` | Status, current step, and the full narration log |
| POST | `/v1/runs/{id}/cancel` | Stop the run, put the flags back, clear the malaises it created |
| GET | `/v1/narration` | The active run, for the console's ticker |
| GET | `/healthz`, `/readyz`, `/metrics` | Platform endpoints (see the kit) |

```json
GET /v1/runs/run_01K6…
{
  "id": "run_01K6…", "scenarioId": "grand-tour", "title": "The Grand Tour",
  "status": "running", "currentStep": 12, "totalSteps": 50,
  "startedAt": "2026-09-30T12:00:00.000Z", "finishedAt": null,
  "log": [
    {"time": "…", "step": 11, "kind": "malaise", "level": "info",
     "message": "Inducing malaise: one PostgreSQL pod, deleted, for real…"},
    {"time": "…", "step": 12, "kind": "waitFor", "level": "info",
     "message": "Watching the gauges until sum(rate(…)) > 0."}
  ]
}
```

Runs execute as asyncio tasks, never inside a request. **One run at a time**: two scenarios
fighting over the same flags and the same traffic generator would tell the audience nothing.

Whatever a run changed, it puts back · flags to their previous values unless the step said
`keep: true`, any malaise it created cleared, and the grammeters set back to the settings they
had when the run started (`running`, `rps`, `pattern`, `periodSeconds`, `customers`, `mix`, read
from `GET /v1/traffic` at the start, put back only if a `traffic` step ran) · whether it completed,
failed, or was cancelled. The cleanup is shielded from the cancellation that triggered it, and
`POST /cancel` does not return until the restoration has finished. Run history is capped
(`RUN_HISTORY`, default 50) and lives only in memory: a restart loses the history, not the factory.

**Managed stores.** Where PostgreSQL or Valkey is a managed AWS service (RDS, ElastiCache) there is
no store pod, and the malaise engine answers a `pod-kill` on `postgres` or `valkey` with
`400 unknown_target`. That one answer, for those two targets, does not fail the run: the
orchestrator narrates that the store is a managed AWS service with no pod to delete, skips the rest
of that act up to and including the `clearMalaise` that names its `ref` (a `clearMalaise: all`
still runs, to lift the act's other malaises), and carries on. Every other error still fails the
run. The built-in `hydrocoptic-outage` and `grand-tour` both have such an act.

## Identity

The orchestrator drives control-plane services on the presenter's behalf and asserts
`X-Encabulator-Subject: demo-scenario-orchestrator` with `X-Encabulator-Roles: operator`, which is
what girdlespring and the malaise engine require for writes. Internal services trust these headers
(design section 7), so the orchestrator must not be reachable from outside the cluster.

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `demo-scenario-orchestrator` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | unset ⇒ tracing is a no-op |
| `CARDINAL_GRAMMETER_URL` | unset | unset ⇒ `traffic` steps are logged no-ops |
| `MALAISE_ENGINE_URL` | unset | unset ⇒ `malaise` steps are logged no-ops. Used as a *client*; the orchestrator does not apply malaise to itself |
| `DIFFERENTIAL_GIRDLESPRING_URL` | unset | unset ⇒ `flag` steps are logged no-ops |
| `PREFABULATED_AMULITE_DEPOT_URL` | unset | unset ⇒ `restock` steps are logged no-ops |
| `PROMETHEUS_URL` | unset | unset ⇒ `waitFor` steps return immediately with a warning |
| `SCENARIO_DIR` | `/etc/scenarios` | ConfigMap mount for extra scenarios |
| `RUN_HISTORY` | `50` | runs kept in memory |
| `SHUTDOWN_READINESS_GRACE_SECONDS` | `3` | |

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_scenario_runs_total` | counter | `scenario`, `result` (`started`, `completed`, `failed`, `cancelled`) |
| `encabulator_scenario_steps_total` | counter | `kind` |
| `encabulator_scenario_active` | gauge | 1 while a run is in flight |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_client_request_duration_seconds` | histogram | `http_request_method`, `server_address`, `http_response_status_code` |

## Running locally

```bash
make build
make test                   # no containers needed: every upstream is faked
make lint
make scenarios              # validate the built-ins
make run                    # every upstream optional; steps that have no URL are logged and skipped
```

```bash
make image                  # build context is the repository root
```

Because this service must be a single replica (the one-run-at-a-time rule and the run history are
both per pod), run exactly one.
