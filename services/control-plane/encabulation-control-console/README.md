# `encabulation-control-console`

> "The original machine had a base-plate of prefabulated amulite, surmounted by a malleable
> logarithmic casing in such a way that the two main spurving bearings were in a direct line
> with the pentametric fan." · J. H. Quick, *The Turbo-Encabulator in Industry*, 1944

The instrument panel of the Cloud Native Encabulator: a Vue 3 single-page app served by a
Fastify backend-for-frontend in one container, on one port. It is the only way a person
drives the factory · live topology, synthetic traffic, fault injection, scripted demos,
feature flags, the storefront, and user administration.

Specified by [design section 8.15](../../../docs/design.md#815-encabulation-control-console).

## The one rule

**Every number on this panel was measured.** There is no `Math.random`, no smoothing of a
missing value into a plausible one, and no edge drawn because the architecture diagram says
it should exist.

| On screen | Comes from |
|---|---|
| Nodes | A static registry of the 15 services and 4 backing stores, merged with live pods from the Kubernetes API |
| Pod dots | `app.kubernetes.io/part-of=encabulator` pods: name, phase, readiness, restarts |
| Per-pod rate, p50, p99, errors | `http_server_request_duration_seconds` by `pod`, in Prometheus |
| Edges | Only the OTel service graph: `traces_service_graph_request_total`, `…_failed_total`, `…_server_seconds_bucket`, over one minute |
| Headline gauges | The ledger's `/v1/stats`, the detector's `/v1/stats`, the depot's `/v1/parts`, the recorder's `/v1/summary` |
| Feeds and narration | The ledger, the detector, dispatch, and the orchestrator's run log |

Anything an upstream did not report is `null` all the way to the screen, where it renders as
the words **no data** and the plate names the upstream or the environment variable
responsible. A measured zero and an absent reading look different on purpose.

Three tests enforce this rather than trusting it: `src/app/__tests__/honesty.test.ts` fails
the build on a `Math.random` call or a hard-coded host in the SPA,
`src/server/__tests__/assemble.test.ts` proves the assembler invents no nodes or edges, and
`src/app/__tests__/components.test.ts` proves a `null` reaches the screen as "no data".

## Pages

| Page | Path | What it does |
|---|---|---|
| **Factory Floor** | `/` | Animated topology: particles per edge at a rate proportional to its measured requests per second, coloured by health (green under 100 ms p99 and 1% errors, amber under 500 ms, red beyond, grey for no data). Pod dots on each plate, hover for details. Headline gauges and the scenario narration tape. |
| **Grammeters** | `/grammeters` | Traffic controls: rate, pattern (steady, sine, burst, ramp), period, customer count, journey mix. Live per-journey counts, errors, p50, p99, and status codes. |
| **Malaise** | `/malaise` | Induce a fault by target and kind · latency, error, congestion, CPU burn, pod-kill, partition, scale · with countdowns to expiry, and node health beside it so cause and effect are visible together. |
| **Scenarios** | `/scenarios` | The scenario library, run and cancel, step progress, and the orchestrator's narration as it arrives. |
| **Girdlespring** | `/girdlespring` | Every feature flag, what reads it, and what it actually changes. Edits reach services within five seconds. |
| **Works** | `/works` | The storefront (quote, order, track) placing **real** orders through the real gateway with your own token, plus live order, incident and shipment feeds, and the parts cage with restock. |
| **Administration** | `/administration` | Users and the audit log. Admin only. |

Deep links work: the BFF serves `index.html` for any path that is not an API route or a file,
so `/works` survives a refresh and a pasted URL. An unknown `/bff/*` path still gets a JSON
404, because handing HTML to a `fetch()` would be a lie.

Two finishes, switched from the nameplate and remembered in `localStorage`: **enamel**
(cream panel, brass bezels, green phosphor gauge faces · a 1944 GE data sheet) and
**control room** (the same instrument at night). Type is a system stack, so nothing is fetched
from a font CDN when the conference wifi drops.

### Vocabulary

Labels are in the vocabulary of Quick's paper, the 1977 Rockwell film, and the 1988 SANS ICS
hyper-encabulator. Every one carries a tooltip with what it really means: "Fluorescent skor
motion" hovers to *Requests per second -
`rate(http_server_request_duration_seconds_count[1m])` from Prometheus*. The joke is the
label; the tooltip is not a joke. `src/app/lib/lexicon.ts` holds both halves, and a test
asserts the explanation is never just a restatement of the term.

## Authorisation

- Unauthenticated visitors are `guest` with the `viewer` role. Every gauge is live; every
  control is locked.
- Sign-in calls the identity service through the BFF; the RS256 JWT goes into an `httpOnly`,
  `SameSite=Strict`, signed cookie and is never handed to JavaScript.
- **Every mutating route requires `operator` or `admin`, enforced in the BFF.** User
  management and the audit log require `admin`. The SPA greys out controls a viewer cannot
  use, but that is cosmetic: `src/server/__tests__/roles.test.ts` walks the whole route table
  and asserts the server refuses each one for each role.
- The BFF forwards `X-Encabulator-Subject` and `X-Encabulator-Roles` from its own session and
  discards whatever the browser sent, so a client cannot promote itself in transit.
- The storefront needs a signed-in user of any role, not an operator, and uses that user's own
  bearer token through the gateway.

## The live stream

`GET /bff/stream` is Server-Sent Events. Four named events:

| Event | Cadence | Payload |
|---|---|---|
| `topology` | every 3 s | The whole snapshot: nodes, edges, unmapped peers, source health |
| `headline` | every 3 s | The Factory Floor gauges, each with its source status |
| `feed` | as rows appear | New orders, incidents and shipments, deduped by id |
| `scenario` | as it changes | The active run, its step, and its narration |

The collector polls upstreams **only while at least one client is connected**, and one poll is
fanned out to every client. A client attaching mid-cycle gets the cached snapshot immediately,
so the panel is never blank. The SPA closes the stream when the tab is hidden, which stops the
polling too: an unattended console costs the factory nothing.

The `headline` and `scenario` events are additions to the two the design names; `feed` carries
orders, incidents, shipments and narration as specified.

## Running locally

### Development, with fixtures and no cluster

```bash
make dev      # or: npm run dev
```

Three processes, one command: a fixture server on 9099, the BFF on 8080, and Vite on 5173.
Open <http://127.0.0.1:5173> and sign in as `dev` / `dev` (admin); `operator`, `viewer` and
`customer` work too, with any password.

Everything dev-only lives under **`dev/`**, and nothing under `dev/` enters the production
image · the Dockerfile copies explicit paths, and a test asserts the absence.

| File | What it is |
|---|---|
| `dev/fixture-server.ts` | One HTTP server answering every upstream *and* Prometheus. The numbers are constants you can read, moving only on a slow triangular wave so a gauge visibly responds. Not a simulator, and no random numbers. |
| `dev/cluster-fixture.ts` | A `ClusterReader` returning a fixed pod list, including one unready pod and one in `CrashLoopBackOff`, so the amber and red dots get exercised. |
| `dev/bff.ts` | The **production** `buildApp`, with only the Kubernetes reader replaced. Every route, role check and assembler you develop against is the shipped code. |
| `dev/dev.env` | Points every upstream at the fixture server. |

To develop the SPA against a real cluster instead, port-forward the real services, put their
addresses in your own env file, and run `npm run dev:web` with `BFF_DEV_TARGET` set.

### Production-shaped, in a container

```bash
make image
docker run --rm --read-only --tmpfs /tmp -u 65532:65532 -p 8080:8080 \
  -e SESSION_SECRET=$(openssl rand -hex 24) \
  -e HYDROCOPTIC_DATASTORE_URL=http://hydrocoptic-datastore:8080 \
  -e PROMETHEUS_URL=http://prometheus.encabulator-observability:9090 \
  ghcr.io/jlbutler/cloud-native-encabulator/encabulation-control-console:1.0.0
```

Unset upstreams are not an error: the console starts, the affected plates say which variable
is missing, and everything else works. That is deliberate · a half-configured console is more
useful on stage than one that will not boot.

## Environment variables

Common to every service (design Appendix A):

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | One port for the API, the SPA, health and metrics |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTEL_SERVICE_NAME` | `encabulation-control-console` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing entirely |
| `SERVICE_VERSION` | `1.0.0` | Resource attribute and `/metrics` label |

Console-specific:

| Variable | Default | Required | Notes |
|---|---|---|---|
| `SESSION_SECRET` | · | **yes** | Signs the session cookie. At least 16 characters; the process refuses to start without it. |
| `COOKIE_SECURE` | `false` | | Set `true` behind TLS |
| `SESSION_COOKIE_NAME` | `encabulator_session` | | |
| `POD_NAMESPACE` | `encabulator` | | Downward API. Namespace for the pod list and the Prometheus `namespace` label. |
| `KUBERNETES_ENABLED` | `true` | | `false` skips the API entirely; pods read "no data" |
| `PROMETHEUS_URL` | unset | | Unset means every metric on the panel reads "no data" |
| `GRAFANA_EXTERNAL_URL` | unset | | Link in the nameplate; hidden when unset |
| `JAEGER_EXTERNAL_URL` | unset | | Link in the nameplate; hidden when unset |
| `WEB_ROOT` | beside the BFF | | Where the built SPA lives; the image sets `/app/dist/web` |
| `TOPOLOGY_INTERVAL_MS` | `3000` | | Topology assembly cadence while clients are attached |
| `FEED_INTERVAL_MS` | `3000` | | Feed and headline cadence |
| `UPSTREAM_TIMEOUT_MS` | `3000` | | Outbound timeout, per the design's HTTP section |
| `PROMETHEUS_TIMEOUT_MS` | `5000` | | Prometheus gets longer; a busy factory is slow to query |

Upstream service URLs. Every one is optional; unset means the plates that need it say so.

| Variable | Used for |
|---|---|
| `UNILATERAL_PHASE_DETRACTOR_URL` | Sign-in, users, audit |
| `DINGLE_ARM_GATEWAY_URL` | The storefront |
| `CARDINAL_GRAMMETER_URL` | Traffic |
| `MALAISE_ENGINE_URL` | Malaise (`MALAISE_INJECTION_ENGINE_URL` also accepted) |
| `DEMO_SCENARIO_ORCHESTRATOR_URL` | Scenarios, runs, narration |
| `DIFFERENTIAL_GIRDLESPRING_URL` | Flags |
| `PREFABULATED_AMULITE_DEPOT_URL` | Parts cage, restock |
| `HYDROCOPTIC_DATASTORE_URL` | Orders, ledger stats |
| `SIDE_FUMBLING_DETECTOR_URL` | Incidents, inspection stats |
| `NOFER_TRUNNION_DISPATCH_URL` | Shipments |
| `SINUSOIDAL_RECORDER_URL` | Telemetry summary |

No variable has a `localhost` default. A test asserts that an environment with only
`SESSION_SECRET` set leaves every upstream `null`, and that the SPA bundle contains no
absolute URL at all: the browser reaches the BFF at the relative path `/bff`, so the console
works identically behind a port-forward, an ingress, or a sub-path.

## Endpoints

`/healthz` liveness, `/readyz` readiness, `/metrics` Prometheus, `/bff/*` the API, everything
else the SPA.

Readiness registers **no checks**. The console has no database and no NATS connection, so per
the design it must not check downstream services · and that is also what stops one sick
upstream from taking the console out of service exactly when an operator needs to look at it.

## Deployment notes

For the Helm chart author:

- **RBAC**: a namespaced `Role` in `encabulator` with `get`/`list` on `pods` and `deployments`
  in `apps`. Nothing else. No `watch`, no write verb, no cluster-scoped rule. The console
  never mutates the cluster; it asks the malaise injection engine to.
- **Service account token**: required (this is one of the two pods that needs the Kubernetes
  API). Set `automountServiceAccountToken: true`.
- **Probes**: `GET /healthz` for liveness, `GET /readyz` for readiness, both on the HTTP port.
  A `startupProbe` is unnecessary; the process is listening in well under a second.
- **Security context**: `runAsNonRoot`, `runAsUser: 65532`, `runAsGroup: 65532`,
  `readOnlyRootFilesystem: true`, all capabilities dropped, `seccompProfile: RuntimeDefault`.
  Mount an `emptyDir` at `/tmp`. Verified by running the image exactly that way.
- **Resources**: `requests: 100m / 192Mi`, `limits: 500m / 512Mi`. The BFF's work is one poll
  cycle every three seconds per *deployment*, not per client, and the particle animation runs
  in the browser.
- **Replicas**: one is enough and is the simplest thing to explain. More than one works · the
  BFF holds no state a client depends on beyond its own cached snapshot · but each replica
  polls independently, and an SSE client reconnecting lands on whichever pod the Service
  picks, so keep a `PodDisruptionBudget` if you scale up.
- **Ingress and SSE**: `/bff/stream` is a long-lived response. The BFF sends
  `Cache-Control: no-cache, no-transform` and `X-Accel-Buffering: no`, and a `: ping` comment
  every 15 seconds. Ingress controllers still need help:
  - nginx: `nginx.ingress.kubernetes.io/proxy-buffering: "off"` and
    `proxy-read-timeout: "3600"`.
  - ALB: raise the target group and load balancer idle timeout above 60 s
    (`alb.ingress.kubernetes.io/load-balancer-attributes:
    idle_timeout.timeout_seconds=3600`); the 15 s heartbeat covers anything shorter.
  - Any proxy: response buffering must be off, or the panel appears frozen while frames sit
    in a buffer.
- **ServiceMonitor**: label `encabulator.io/scrape=true`, scrape `/metrics` on the HTTP port.
  The console's own per-pod row on the Factory Floor comes from this scrape, so without it the
  console honestly reports no data about itself.
- `SESSION_SECRET` from a generated `Secret` · `randAlphaNum 32`, preserved on upgrade with
  `lookup`. Rotating it logs everyone out, which is harmless.
- `POD_NAMESPACE` from the downward API (`metadata.namespace`).

## Stack

Node 24, TypeScript 6 strict ESM, Vue 3.5, Vite 8, Pinia, Vue Router, Tailwind CSS 4, d3 for
the topology geometry, Fastify 5 with `@fastify/static` and `@fastify/cookie`,
`@kubernetes/client-node` 1.x (object-style API), Vitest, and ESLint with a flat config.

Logging, tracing, the health and metrics endpoints, the Fastify factory, and the outbound HTTP
client all come from [`@encabulator/kit`](../../../libs/ts/kit), consumed as a `file:`
dependency. The kit's `MalaiseController` is deliberately **not** used: the design says the
console does not apply malaise to itself.

## Performance

The topology is two layers over one geometry. An SVG layer carries everything structural and
re-renders only when a snapshot changes, once every three seconds. A canvas layer directly on
top carries the particles, drawn by a single `requestAnimationFrame` loop · never a DOM update
per particle. The loop caps particles at 900 globally and 24 per second per edge, stops
entirely when the tab is hidden, honours `prefers-reduced-motion`, and backs its spawn rate
off if frame times exceed 26 ms rather than letting the frame rate collapse. Lateral jitter
comes from a seeded PRNG (`src/app/lib/prng.ts`), not `Math.random`, so a given edge always
looks the same and a test can reproduce a frame exactly.

Turn on **show diagnostics** on the Factory Floor for live particle count, frame time, and the
current spawn multiplier.

## Tests

```bash
make test     # 263 tests, no containers, no cluster, no network
```

- **BFF role enforcement** · the whole proxy route table × every role.
- **Topology assembly** · recorded Prometheus and Kubernetes responses in, snapshot out;
  including NaN, missing series, unreachable sources, alias collapsing, and peers outside the
  registry.
- **SSE** · framing, fan-out, cached replay, heartbeat, and that polling starts on the first
  client and stops on the last.
- **Sessions** · cookie flags, expiry, a tampered signature, and that no token ever reaches a
  response body.
- **Static serving** · real files from a temp directory: cache headers, the history fallback,
  and that an API path never gets HTML.
- **Stores and components** · SSE application, feed deduplication, failure messages, and that
  a `null` measurement renders as "no data" everywhere it can appear.
- **The shell** · the app mounts, all seven pages resolve, exactly one stream is opened and it
  is released on unmount, and an unconfigured external link is hidden rather than broken.
- **Every view** · each page mounted twice, once with no data and once with a full snapshot;
  neither may throw, and neither may print `undefined`, `NaN` or `[object Object]` at a reader.
- **Honesty guards** · no `Math.random` call, no hard-coded host, and a real explanation behind
  every joke term.
- **Packaging guards** · the Dockerfile copies explicit paths and never `dev/`, ships no source
  maps, runs as 65532, and declares no `HEALTHCHECK`.

`docs/design.md` also calls for a Playwright smoke test. It is not here: it needs a running
cluster, so it belongs with the repository's end-to-end suite rather than in this package's
`make test`, which must pass without containers.
