# Map of the Encabulator

## On one screen

```
Git (this repository) ──► Argo CD on the hub ──► the hub and every cell
                                                                      
Hub: encabulator-usw2, us-west-2, EKS Auto Mode, built by Terraform (terraform/brownfield)
  EKS Capabilities: Argo CD, ACK, kro (managed by EKS, nothing to install)
  App: 15 services on the Helm chart (charts/encabulator), namespace encabulator
  Stores: RDS for PostgreSQL encabulator-pg and ElastiCache Serverless encabulator-valkey,
          built by Terraform, then adopted by ACK (gitops/hub/brownfield), deletion-policy retain
          NATS and InfluxDB as StatefulSets in the cluster
  kro RGD EncabulatorCell (gitops/hub/rgds): one object builds a whole region
  Tagger CronJob (namespace encabulator-tagger): tags what AWS creates untagged, every 10 minutes

Cell: encabulator-use2, us-east-2, built by the hub's ACK and kro from gitops/hub/cells/use2.yaml
  VPC, NAT gateway, IAM roles, EKS Auto Mode cluster, ACK and kro capabilities, access entries
  Registered with the hub's Argo CD, which syncs gitops/app and the observability platform
  App: kro RGDs Encabulator and EncabulatorService (gitops/app), no Helm
  Stores: RDS and ElastiCache created by the cell's own ACK; NATS and InfluxDB in the cluster
```

The 15 services, in four planes (`make services` lists them from `scripts/services.txt`):

| Plane | Services |
|---|---|
| control | encabulation-control-console, cardinal-grammeter, malaise-injection-engine, demo-scenario-orchestrator |
| processing | dingle-arm-gateway, panametric-fam-processor, spurving-bearing-quoter, logarithmic-casing-line, nofer-trunnion-dispatch |
| data | hydrocoptic-datastore, prefabulated-amulite-depot, sinusoidal-recorder |
| support | unilateral-phase-detractor, differential-girdlespring, side-fumbling-detector |

An order: gateway, processor (the saga), quoter (price), depot (reserve parts), datastore (the
ledger), then over NATS: casing line (seven stages, readings to the recorder), side fumbling
detector (inspection), dispatch (shipping), delivered. [docs/design.md](../../../../docs/design.md)
section 4 has every step; [docs/GLOSSARY.md](../../../../docs/GLOSSARY.md) says what each silly
name really is.

## Where things live

| What | Where |
|---|---|
| The contract: APIs, events, flags, metrics | `docs/design.md` |
| Services | `services/<plane>-plane/<service>/` |
| Shared kits (logging, tracing, malaise) | `libs/go/kit`, `libs/ts/kit`, `libs/python/encabulator-kit` |
| Helm chart (self-managed and the hub) | `charts/encabulator` |
| Observability platform and KEDA | `platform/` |
| Hub stack (VPC, EKS, RDS, ElastiCache, capabilities, tagger) | `terraform/brownfield/` |
| ACK objects Git owns for the hub's stores | `gitops/hub/brownfield/` |
| Argo CD bootstrap for the hub | `gitops/hub/bootstrap/` |
| The cell RGD and the cells | `gitops/hub/rgds/encabulator-cell.yaml`, `gitops/hub/cells/` |
| The cell ApplicationSets | `gitops/hub/appset.yaml` |
| The app as kro APIs | `gitops/app/` |
| Teardown helpers | `docs/teardown/`, `scripts/teardown.sh` |
| The talk | `docs/opn315/` (`make talk`) |

## make targets

Read-only targets are marked. Everything else changes something and needs the presenter's yes.

| Target | What it does |
|---|---|
| `make help` | every target with its line (read-only) |
| `make ops-health` | this skill's one-screen health check (read-only) |
| `make status` | pods, deployments, autoscalers, stores in `encabulator` (read-only) |
| `make platform-status` | what the platform installed (read-only) |
| `make services`, `make version` | the 15 services; the image version (read-only) |
| `make prereqs` | tools, AWS credentials, `.env`, Identity Center; every problem with its fix (read-only) |
| `make tag-check` | fails on any AWS resource without `auto-delete=no` (read-only) |
| `make gone-check` | lists anything of the project left in the account (read-only) |
| `make gitops-render`, `make gitops-render-check`, `make gitops-test` | GitOps renders and offline tests (read-only) |
| `make brownfield-test`, `make brownfield-plan` | Terraform offline checks; a plan (read-only) |
| `make lint`, `make test`, `make lint-charts`, `make template` | code and chart checks (read-only) |
| `make skills-check` | the skill links resolve and this skill's tests pass (read-only) |
| `make kubeconfig` | points kubectl at the hub (changes your kubeconfig only) |
| `make console`, `make grafana`, `make jaeger`, `make prometheus` | port-forwards on localhost (8080, 3000, 16686, 9090) |
| `make credentials` | prints the passwords: off screen only |
| `make talk` | serves the slides on 127.0.0.1:8000 |
| `make hub` | builds the hub from `.env`, asks first (billable, about 25 minutes) |
| `make brownfield-init`, `make brownfield-apply`, `make wait-capabilities`, `make mirror-images` | single steps of `make hub` |
| `make platform`, `make deploy`, `make brownfield-deploy`, `make undeploy`, `make platform-uninstall` | install or remove the platform and the app |
| `make gitops-register`, `make gitops-repo-access`, `make gitops-bootstrap` | hand the hub to Argo CD |
| `make add-cell CELL=use2` | applies `hub-cells`: builds every cell in `gitops/hub/cells` (billable, about 35 minutes) |
| `make cell-access CELL=use2` | an EKS access entry on the cell for your AWS identity |
| `make gitops-validate KIND_KUBECONFIG=...` | `gitops/app` on a local kind cluster |
| `make remove-cell`, `make teardown`, `make mirror-images-delete` | delete things: never on stage |
| `make build`, `make images`, `make push`, `make fmt`, `make clean` | build, image and format tasks |
| `make dev-up`, `make dev-down`, `make dev-logs`, `make kind-up`, `make kind-down`, `make kind-load` | local development |

`make -n <target>` prints what a target would run and runs nothing. `make hub` stops at the first
failing step, names it, and is safe to run again.

## .env keys

`.env` is gitignored and holds no secret. Copy it from `.env.example`.

| Key | Meaning |
|---|---|
| `IDC_INSTANCE_ARN` | the IAM Identity Center instance Argo CD signs users in with (required) |
| `ARGOCD_ADMIN_GROUP_ID` | an Identity Center GROUP whose members are Argo CD admins; a user id gives nobody access (required) |
| `IDC_REGION` | Identity Center's region when it is not us-west-2 |
| `GIT_REPO_URL`, `GIT_REVISION` | the repository and branch Argo CD syncs (defaults: this repository, `main`) |
| `GIT_TOKEN_FILE` | path to a mode 600 token file, for a private repository only |
| `REGISTRY` | where the cluster pulls images; empty means GHCR; `make mirror-images` writes the ECR mirror here |
| `CONTAINER_RUNTIME` | docker or finch, for building images when they must be mirrored |

`TERRAFORM` is not read from `.env`: export it in the shell or put it on the make command line,
for example `TERRAFORM="mise exec terraform@1.14.9 -- terraform"`.

## Namespaces, contexts, ports

| Namespace | What |
|---|---|
| `encabulator` | the app, NATS, InfluxDB, the store objects (`dbinstances.rds.services.k8s.aws` and friends) |
| `encabulator-observability` | Prometheus, Grafana, Jaeger, the OpenTelemetry Collector |
| `keda` | KEDA, when installed |
| `argocd` | Argo CD's Applications, ApplicationSets and cluster Secrets (the controller itself is managed by EKS) |
| `encabulator-cells` | each `EncabulatorCell` and the hub's ACK objects that build it |
| `encabulator-tagger` | the tagger CronJob |

| kubectl context | Cluster |
|---|---|
| `encabulator-usw2` | the hub (`make kubeconfig`; namespace `encabulator`) |
| `use2` | the cell, from the command `make cell-access CELL=use2` prints (the run of show adds `--alias use2`) |

That command also switches kubectl to the cell; `kubectl config use-context encabulator-usw2`
switches back. Every service listens on 8080. PostgreSQL 5432, Valkey 6379 (TLS, no password on
ElastiCache), NATS 4222, InfluxDB 8181.

Always name ACK kinds in full: a bare `dbinstances` resolves to the DocumentDB kind and prints
"No resources found". Use `dbinstances.rds.services.k8s.aws` (or `dbinstances.rds`).

## Two sentences for the audience

- **EKS Auto Mode**: EKS runs the nodes for us and starts them when pods need them. Our NodeClass
  and NodePool say which instance families to use and tag every node.
- **EKS Capabilities**: Argo CD, ACK and kro run as managed parts of the EKS cluster, so there is
  nothing of them to install, patch or scale. Each one has its own IAM role.
- **Argo CD**: it compares the cluster with Git all the time and puts the cluster back when they
  differ. A change is a commit, and the history of the system is the Git log.
- **ACK**: it turns a Kubernetes object into AWS API calls and keeps calling until AWS matches.
  It can adopt a resource that already exists, so we took over the database Terraform built
  without touching it.
- **kro**: one Kubernetes object stands for a whole application or a whole region. kro works out
  the order from the references and creates each piece once the pieces it needs are ready.
- **A cell**: one 13-line file in Git becomes a new region, with its own VPC, cluster, database
  and the app. The hub builds it with ACK and kro, and Argo CD fills it.
- **Terraform hand-off**: the `removed` blocks make Terraform forget the stores without
  destroying them. From then on Git owns their settings.
- **The tagger**: some resources AWS creates on its own carry no tags. A small job tags them
  every 10 minutes, so `make tag-check` can prove every resource is accounted for.
- **Malaise**: the console can break any service on purpose (latency, errors, pod kills, scale to
  zero). Every fault expires and reverts by itself.
- **Observability**: every order is one trace across four languages, and the console's Factory
  Floor draws the live service graph from those traces.
