<div align="center">

# Cloud Native Encabulator

**A distributed system that is entirely real and completely ridiculous.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Services](https://img.shields.io/badge/services-15-brightgreen.svg)](docs/design.md)
[![Languages](https://img.shields.io/badge/languages-Go%20·%20TypeScript%20·%20Python%20·%20Rust-orange.svg)](#polyglot-on-purpose)

</div>

---

For a number of years now, work has been proceeding in order to bring perfection to the crudely
conceived idea of a microservices platform that would not only supply inverse reactive
observability for use in unilateral phase detractors, but would also be capable of automatically
synchronizing cardinal grammeters.

Such an instrument is the **Cloud Native Encabulator**.

The original machine had a base-plate of prefabulated amulite, surmounted by a malleable
logarithmic casing in such a way that the two main spurving bearings were in a direct line with the
panametric fam. This one has fifteen microservices, four backing stores, distributed tracing, and a
web console from which you can break any of it on purpose.

## What this actually is

A **realistic** distributed system you can run, drive, break, and observe. Not a toy with three
services and a `sleep(100)`. Not production either. The gap between those two is where most
learning happens, and almost nothing lives there.

The Encabulator Works is a factory that builds turbo encabulators to order:

1. A customer places an order through the **dingle arm gateway**.
2. The **spurving bearing quoter** prices it (in Rust, burning real CPU on a "pentametric harmonic").
3. The **prefabulated amulite depot** atomically reserves parts, or rejects the order for want of
   hydrocoptic marzlevanes.
4. The **panametric fam processor** runs the order saga and writes it to the ledger.
5. The **logarithmic casing line** fabricates each unit through seven stages, emitting real
   telemetry per stage.
6. The **side fumbling detector** inspects that telemetry and finds the defective units by
   statistics, not by being told.
7. The **nofer trunnion dispatch** ships what passes. Failures go back for refabrication, up to
   three attempts, then get scrapped.

Every arrow in that chain is a real network call or a real durable message. Every number on the
console comes from Prometheus, the Kubernetes API, or a service's own API. There is no
`Math.random()` anywhere in the visualization layer, which is a claim this project's previous
incarnation could not make.

## Why it exists

Three reasons, in order of honesty:

1. **It is a genuinely hard thing to demo well.** GitOps, infrastructure-as-Kubernetes-resources,
   and platform abstractions are only interesting when the thing being deployed is complicated
   enough to hurt. Fifteen services with real databases hurt exactly the right amount.
2. **It is a learning artifact.** The repository evolves in tagged phases, so you can check out any
   phase and see a complete, working system at that level of sophistication, then diff it against
   the next.
3. **The Turbo Encabulator is one of engineering's great jokes** and it deserves a Kubernetes
   tribute. The [1944 original](https://en.wikipedia.org/wiki/Turbo_encabulator) by John Hellins
   Quick is a masterpiece of technobabble; the 1977 Bud Haggart film and Mike Kraft's Rockwell
   "Retro Encabulator" are required viewing.

## Polyglot on purpose

Four languages, chosen where each actually fits rather than for variety's sake:

| Language | Services | Why |
|---|---|---|
| **Go** | 6 | Gateway, ledger, flags, telemetry, chaos, load generation. Fast startup, tiny images, excellent Kubernetes client. |
| **TypeScript** | 4 | Saga orchestration, inventory, shipping, and the console. One language across the browser and its backend-for-frontend. |
| **Python** | 4 | Identity, fabrication simulation, anomaly detection, scenario scripting. NumPy earns its place in the signal work. |
| **Rust** | 1 | The pricing engine, which deliberately burns CPU and is the natural autoscaling target. |

Polyglot is not decoration: it means the observability story has to work across four OpenTelemetry
SDKs, four metric libraries, and four container build patterns. That is the point.

## Quick start

Requires a Kubernetes cluster (Amazon EKS or [kind](https://kind.sigs.k8s.io/)), `helm`, `kubectl`,
and a container runtime ([Finch](https://runfinch.com/) or Docker) if you intend to build images.

```bash
git clone https://github.com/jlbutler/cloud-native-encabulator.git
cd cloud-native-encabulator

make platform     # Prometheus, Grafana, Jaeger, OpenTelemetry Collector, KEDA
make deploy       # the Encabulator itself, into namespace "encabulator"
make console      # port-forward, then open http://localhost:8080
```

The console's *Grammeters* page starts the traffic generator. Watch the Factory Floor light up,
then go to *Malaise* and start killing things.

Full instructions, including running services natively against `dev/compose.yaml`, are in the
[Development Guide](docs/DEVELOPMENT.md).

## The console

A 1940s instrument panel for a 2020s distributed system.

- **Factory Floor** — live topology built from OpenTelemetry service-graph metrics, with particle
  flows scaled to real request rates and colored by real latency and error rates. Pods appear as
  dots and come and go as Kubernetes scales them.
- **Grammeters** — drive synthetic customer traffic: steady, burst, ramp, or sine patterns, with a
  configurable mix of browse, quote, order, and track journeys.
- **Malaise** — inject latency, errors, congestion, or CPU burn into any service; kill pods;
  partition the network; or scale a deployment to zero. Everything expires and reverts on its own,
  so a forgotten fault never strands the demo.
- **Scenarios** — scripted, narrated demos (`marzlevane-shortage`, `hydrocoptic-outage`,
  `side-fumbling-outbreak`, `grand-tour`) that drive traffic, induce malaise, flip flags, and wait
  on Prometheus conditions.
- **Girdlespring** — feature flags with percentage rollouts, read live by five services.
- **Works** — a storefront where you can place a real order yourself and watch it traverse the
  entire system, plus live order, shipment, and incident feeds.

## Architecture

```
control      console · grammeter · malaise engine · scenario orchestrator
processing   gateway · processor · quoter · casing line · dispatch
data         order ledger · parts depot · telemetry recorder
support      identity · feature flags · inspection
stores       PostgreSQL 18 · Valkey 9 · NATS JetStream · InfluxDB 3
platform     Prometheus · Grafana · Jaeger · OpenTelemetry Collector · KEDA
```

[**docs/design.md**](docs/design.md) is the authoritative contract: every API, event, feature flag,
metric name, and convention. It is written to be read before the code, and the code is written to
match it. If they disagree, that is a bug.

## Roadmap

Each phase is a tag on `main` holding a complete, working system. The interesting part is the diff
between them.

| Tag | Phase | What changes |
|---|---|---|
| `v1.x` | **Self-managed** | Everything above, all open source, deployed with Helm. You are here. |
| `v2.x` | **GitOps** | Argo CD reconciles platform and application from this repository. |
| `v3.x` | **Managed services** | ACK provisions RDS, ElastiCache, and Timestream for InfluxDB; the chart points at them instead of the StatefulSets. |
| `v4.x` | **Composition** | kro packages each service with its AWS dependencies behind a single custom resource. |
| later | **Cells** | Multiple cells across accounts and regions, with progressive rollouts between them. |

## Documentation

- [System Design](docs/design.md) — the contract: services, APIs, events, conventions
- [Development Guide](docs/DEVELOPMENT.md) — build, test, run, and debug
- [Glossary](docs/GLOSSARY.md) — what every piece of nonsense actually means
- [Contributing](CONTRIBUTING.md) — including the DCO sign-off requirement

## A note on the jargon

Every silly name in this repository maps to a real technical concept, and the
[glossary](docs/GLOSSARY.md) gives you both. The console's tooltips do the same, so
"fluorescent skor motion" is labeled with what it actually is (requests per second).

The joke only works if the engineering underneath is serious. That is true of the 1944 original
too: Quick's paper is funny because it is written in flawless period engineering prose. We are
trying to hold the same bar.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

The Turbo Encabulator is a public-domain engineering in-joke dating to 1944. No affiliation with
General Electric, Rockwell Automation, Chrysler, or the SANS Institute is claimed or implied.
