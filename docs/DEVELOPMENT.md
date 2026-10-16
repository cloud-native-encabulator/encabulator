# Development Guide

How to build, test, run, and debug the Cloud Native Encabulator.
[docs/design.md](design.md) is the contract; read it before changing behavior.

## Prerequisites

| Tool | Version | Needed for |
|---|---|---|
| `make` | any | everything |
| `helm` | 3.16+ | deploying |
| `kubectl` | 1.31+ | deploying |
| Go | 1.27+ | the six Go services and the Go kit |
| Node.js | 24+ | the four TypeScript services and the TS kit |
| Python + `uv` | 3.14 / uv 0.9+ | the four Python services and the Python kit |
| Rust | 1.96+ | the quoter |
| A container runtime | [Finch](https://runfinch.com/) or Docker | images and integration tests |
| A cluster | Amazon EKS, or [kind](https://kind.sigs.k8s.io/) | deploying |

You only need the toolchain for the language you are touching. `make build` skips what it cannot
find rather than failing.

`make services` lists all fifteen services with their directory, language, and plane.

## Container runtime

Integration tests and image builds need a Docker-compatible socket. With Finch:

```bash
finch vm start
source dev-env.sh     # sets DOCKER_HOST, and BUILD_GOPROXY if this machine needs it
```

`dev-env.sh` sets:

- `DOCKER_HOST`, pointing at the Finch daemon socket, which Testcontainers reads in all four
  languages.
- `TESTCONTAINERS_RYUK_DISABLED=true`, because the reaper wants a privileged container.
- `BUILD_GOPROXY`, a Go module proxy for image builds only. See
  [corporate DNS](#go-image-builds-and-corporate-dns).

With Docker Desktop or Colima, `DOCKER_HOST` is already set and you can skip the file.

## Build, test, lint

```bash
make build            # every service and library that has a Makefile
make test             # unit tests everywhere; integration tests if DOCKER_HOST is set
make lint             # gofmt and go vet, Biome or ESLint, ruff, clippy and rustfmt
make fmt              # format in place
make lint-charts      # helm lint, helm template across value combinations, kubeconform
```

Each service has the same four targets, so you can work on one in isolation:

```bash
cd services/data-plane/hydrocoptic-datastore
make test
```

Integration tests skip cleanly when `DOCKER_HOST` is unset, which is how CI runs them. With a
runtime available they start real Postgres, Valkey, NATS, or InfluxDB containers. If a suite passes
suspiciously fast, check whether it skipped.

## Running services natively

Start the backing stores in containers and run the service you are editing on your machine:

```bash
source dev-env.sh
make dev-up           # Postgres, Valkey, NATS, InfluxDB, Jaeger
make dev-logs         # follow them
```

`dev/env` holds the connection strings those containers expose. Point a service at them and run it:

```bash
cd services/support-plane/differential-girdlespring
set -a && . ../../../dev/env && set +a
make run
```

Every service takes its whole configuration from environment variables (Appendix A of the design),
so any subset of the system can run this way. A service whose dependency is missing fails its
readiness check and says which dependency in its logs, rather than crashing.

`make dev-down` stops the stores; `make dev-down CLEAN=true` also deletes their volumes.

Jaeger from the dev stack is at <http://localhost:16686>. To send it traces, set
`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`. With that variable unset, tracing is a no-op,
which keeps test output quiet.

## Images

```bash
source dev-env.sh
make images                                  # all fifteen, tagged from VERSION
./scripts/images.sh hydrocoptic-datastore    # just one, by substring
make push                                    # build and push to the registry
make images PLATFORMS=linux/amd64,linux/arm64
```

Every Dockerfile takes the **repository root** as its build context, because the services copy in
their language's kit. Build them as `-f services/<plane>-plane/<name>/Dockerfile .`, never from
inside the service directory.

Images default to `ghcr.io/jlbutler/cloud-native-encabulator/<service>:<VERSION>`. Override with
`REGISTRY=` and `VERSION=`. In normal use you do not build images at all: the `images.yml` workflow
builds multi-architecture images on every push to `main` and on tags.

### Go image builds and corporate DNS

Some corporate resolvers, and some VPN configurations, cannot resolve `proxy.golang.org` from
inside a build VM even though the host resolves it fine. The symptom is:

```
go: ...: dial tcp: lookup proxy.golang.org on 192.168.5.2:53: i/o timeout
```

Every Go Dockerfile therefore takes an overridable `GOPROXY` build argument. Set `BUILD_GOPROXY` to
a reachable mirror and the build scripts pass it through:

```bash
BUILD_GOPROXY=https://goproxy.io make images
```

This changes only the container build; your host's `GOPROXY` is untouched, and CI uses Go's
default. Module hashes are still verified against each `go.sum`, so a mirror cannot substitute
different code.

## Deploying

```bash
make platform      # Prometheus, Grafana, Jaeger, OTel Collector, KEDA (pinned upstream charts)
make deploy        # charts/encabulator into namespace "encabulator"
make status        # pods, deployments, autoscalers, stores
make console       # port-forward, then open http://localhost:8080
make credentials   # bootstrap admin and operator passwords, and Grafana's
```

Install the platform first: the chart's `ServiceMonitor` and `PrometheusRule` objects need the
Prometheus operator's custom resource definitions, and `make deploy` will tell you if they are
missing. You can also run without it (`--set prometheusOperator.crds=false`), but the console will
honestly report that it has no metrics.

Useful overrides:

```bash
make deploy NAMESPACE=encabulator-dev
make deploy HELM_ARGS='--set keda.enabled=false'
make deploy HELM_ARGS='--set services.cardinal-grammeter.env.AUTOSTART=true'
```

`make undeploy` removes the release and leaves the PersistentVolumeClaims, so redeploying keeps
your orders. Delete them by hand to start clean.

### On kind

`make kind-up` creates a local cluster, and `make kind-load` copies locally built images into it so
the chart does not try to pull them. Network partition malaise does nothing on kind's default CNI,
because it does not enforce NetworkPolicy. Everything else works.

### On EKS

Any cluster with a default StorageClass works, including EKS Auto Mode. The chart needs no cloud
resources of its own in v1: all four backing stores run in the cluster. Expose the console with
`--set services.encabulation-control-console.ingress.enabled=true` and an ingress class, or keep
using `make console`.

If you expose the console through an ingress, raise the idle timeout and turn off response
buffering, or its live stream will appear frozen. The chart's ingress template sets the right
annotations for the ALB and nginx controllers.

### On AWS with managed stores, then GitOps

The chart can also point at Amazon RDS and Amazon ElastiCache instead of the in-cluster stores, and
the repository carries the whole path from Terraform to GitOps. Each step has a target; `make help`
lists them all, and each directory's README has the detail.

```bash
make brownfield-test     # offline contract checks for terraform/brownfield, no AWS calls
make brownfield-plan     # terraform plan (needs credentials and terraform/brownfield/encabulator.auto.tfvars)
make brownfield-apply    # VPC, EKS Auto Mode, RDS, ElastiCache, Argo CD, ACK and kro capabilities
make platform && make brownfield-deploy   # observability, then the app on RDS and ElastiCache
make gitops-register     # register the hub with its own Argo CD (REGISTER_ARGS=--dry-run first)
make gitops-bootstrap    # Argo CD projects, the hub ApplicationSet and the hub Applications
make gitops-validate KIND_KUBECONFIG=...  # the kro APIs on a local kind cluster, no AWS
make tag-check           # read-only: fail on any AWS resource without the auto-delete=no tag
make talk                # serve the OPN315 slides on http://localhost:8000
```

Set `TERRAFORM` to use a wrapper such as `mise exec terraform@1.14.9 -- terraform`. These targets
create billable resources; see the README for what, and its [Tear down](../README.md#tear-down)
section for removing them. To build the whole hub from one gitignored `.env` file instead
(`make prereqs`, then `make hub`), follow the
[README quick start](../README.md#on-aws-from-terraform-to-gitops).

`make platform` is for the hub only. A cell that the hub builds from `gitops/hub/cells` gets the
app and the observability platform from Argo CD on the hub, with no `make` step on the cell. To
see what a cell's platform renders to, run `./scripts/platform.sh template-cell`; the layout is
in [`platform/README.md`](../platform/README.md#on-a-cell).

## Debugging

- **A pod is not ready.** `kubectl logs` names the failing dependency. Readiness reflects a
  service's *own* dependencies only, never downstream services, so a single failure does not
  cascade into a wall of red.
- **The console shows "no data".** That is the honest answer, not a bug: either Prometheus is not
  installed, or the OpenTelemetry Collector is not receiving spans, or nothing has sent traffic
  yet. Start traffic on the Grammeters page.
- **No edges on the Factory Floor.** Edges come from the collector's service-graph metrics, which
  need a minute of traffic before they appear. Check that
  `traces_service_graph_request_total` exists in Prometheus.
- **Services connect but a store has no line.** The console only draws a line when the far end of the
  service-graph edge is named `postgres`, `valkey`, `nats` or `influxdb` (or an alias in
  `src/shared/registry.ts`). Look at the `server` label of `traces_service_graph_request_total`: the
  collector's `database_name_attributes` in `platform/opentelemetry-collector.values.yaml` must name
  database servers after the database system, not after the database. Client spans need
  `db.system` or `db.system.name`.
- **Orders are rejected with `insufficient_parts`.** The depot ran out. Either restock on the Works
  page or raise the `amulite-foundry-rate` flag.
- **A fault will not go away.** Every malaise expires on its own, and the engine reverts
  Kubernetes-level changes from object annotations, so even a restarted engine cleans up. To force
  it: `DELETE /v1/malaises` through the console's Malaise page.
- **Traces.** `make jaeger`, then search by service. One order should produce a single trace
  spanning the gateway, processor, quoter, depot, ledger, and then the asynchronous fabrication,
  inspection, and shipment spans linked through NATS.

## Repository layout

```
charts/encabulator/          Helm chart: the application and its backing stores
platform/                    Pinned values and dashboards for upstream platform charts; platform/cell/ is what a cell adds
terraform/brownfield/        The AWS starting point: VPC, EKS Auto Mode, RDS, ElastiCache, capabilities
gitops/                      What Argo CD syncs: the hub (adoption, cells) and the application as kro APIs
docs/opn315/                 The OPN315 talk: slides and run of show
libs/go/kit/                 Go service kit
libs/ts/kit/                 TypeScript service kit (@encabulator/kit)
libs/python/encabulator-kit/ Python service kit
services/<plane>-plane/<service>/
scripts/                     Helpers the Makefile calls; services.txt is the service registry
dev/                         compose.yaml and env for running services natively
docs/                        design.md (the contract), this guide, the glossary
```

`scripts/services.txt` is the single source of truth for the service list. The Makefile, the image
builds, and CI all read it, so adding a service means adding one line there.

## Conventions worth knowing before you write code

Section 7 of the design specifies these, and each language's kit implements them, so you inherit
them by using the kit:

- Port 8080 serves the API, `/healthz`, `/readyz`, and `/metrics`.
- Logs are JSON on stdout with `trace_id` and `span_id` when a span is active.
- `http_server_request_duration_seconds` is labeled with the route *template*, never the raw path,
  so high-cardinality labels cannot blow up Prometheus.
- Domain metrics are prefixed `encabulator_`. Dashboards and alerts depend on the exact label names
  in the design; do not add a `service` label, since the ServiceMonitor adds one.
- Services in the processing, data, and support planes apply the malaise middleware. Control-plane
  services that *drive* the engine do not, so they cannot inject faults into themselves.
- Configuration comes from environment variables only, with service URLs named `<NAME>_URL`.
