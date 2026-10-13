# charts/encabulator

One Helm chart for the whole Encabulator Works: fifteen services in four planes, plus the four
backing stores they persist to. The observability platform and KEDA are **not** here; they come
from pinned upstream charts with values in [`platform/`](../../platform/).

`docs/design.md` section 10 is the contract this chart implements. Where the two disagree, the
document wins.

```bash
make platform          # observability + KEDA first (brings the CRDs this chart uses)
make deploy            # this chart, into namespace `encabulator`
make console           # port-forward http://localhost:8080
make credentials       # the bootstrap admin and operator passwords
```

Or without the Makefile:

```bash
helm upgrade --install encabulator charts/encabulator \
  --namespace encabulator --create-namespace \
  --set keda.enabled=true \
  --wait
```

## What it renders

98 objects at the defaults. Every application workload is rendered from one shared block, so a
change to the Deployment template changes all fifteen at once.

| Per service | Rendered when |
|---|---|
| `Deployment` | always |
| `Service` (ClusterIP, port 8080 named `http`) | always |
| `ServiceAccount` | `serviceAccount.create` |
| `ServiceMonitor` (labelled `encabulator.io/scrape=true`) | `observability.serviceMonitor.enabled` |
| `HorizontalPodAutoscaler` | `autoscaling.enabled`, and KEDA does not own the workload |
| `PodDisruptionBudget` | the workload can run more than one replica |

| Chart-wide | Rendered when |
|---|---|
| 4 `StatefulSet`s + 8 `Service`s (VIP and headless per store) | each store's `enabled` and not `external.enabled` |
| `Secret`s: app, postgres, valkey, influxdb | `secrets.create` |
| `Role`/`RoleBinding` for the malaise engine and the console | those services are enabled |
| `ScaledObject` for `logarithmic-casing-line` | `keda.enabled` |
| `Ingress` for the console | `ingress.enabled` |
| `ConfigMap` for extra demo scenarios | the orchestrator is enabled |
| `PrometheusRule` | `observability.prometheusRule.enabled` |

Requires the Prometheus Operator CRDs (`ServiceMonitor`, `PrometheusRule`) and, for
`keda.enabled`, the KEDA CRD. Turn the corresponding values off to install without them.

## The `services:` map

Every workload is one entry. Defaults come from `defaults:` and are merged per service, so an
entry only states what differs.

```yaml
services:
  spurving-bearing-quoter:
    enabled: true
    plane: processing          # encabulator.io/plane label
    language: rust             # encabulator.io/language label
    replicas: 2
    stores:
      valkey: {}               # injects VALKEY_PASSWORD + VALKEY_URL
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 12
      targetCPUUtilizationPercentage: 65
    resources:
      requests: { cpu: 250m, memory: 64Mi }
      limits: { cpu: 1000m, memory: 256Mi }
    env:                       # extra environment; values go through `tpl`
      SOME_KNOB: "3"
    secretEnv:                 # environment from a Secret key
      SOME_TOKEN: { secret: app, key: session-secret }
```

`stores` is a map, and each key wires up the environment from Appendix A:

| Key | Environment injected |
|---|---|
| `postgres: {database: <name>}` | `PGPASSWORD` (from the Secret) then `DATABASE_URL`, built with Kubernetes `$(VAR)` expansion |
| `valkey: {}` | `VALKEY_PASSWORD` then `VALKEY_URL` |
| `nats: {}` | `NATS_URL` |
| `influxdb: {}` | `INFLUXDB_URL`, `INFLUXDB_DATABASE`, `INFLUXDB_TOKEN` |

The shared variables (`MALAISE_ENGINE_URL`, `DIFFERENTIAL_GIRDLESPRING_URL`, each
`<CALLEE>_URL`, `PROMETHEUS_URL`, `POD_NAMESPACE`) are not repeated per service. They come from
the `wiring:` block, which lists which services read what. `MALAISE_ENGINE_URL` in particular is
set only where it is wanted: an unset value disables the malaise middleware, so the grammeter and
the engine itself never get it, while the console and the orchestrator do because they *drive*
the engine rather than obey it.

## Images

`${global.registry}/<service>:<tag>`, where the tag defaults to `.Chart.AppVersion`.

```yaml
global:
  registry: ghcr.io/jlbutler/cloud-native-encabulator
  imageTag: ""     # empty means Chart.appVersion
```

Override one service with `services.<name>.image.repository`, `.tag` or `.digest` (a digest wins
over a tag).

## Autoscaling

- `spurving-bearing-quoter` and `panametric-fam-processor` have CPU HPAs, as in design section 9.
  The quoter is the interesting one: pricing deliberately burns 5-15 ms of CPU per quote.
- `logarithmic-casing-line` is scaled by KEDA on JetStream consumer lag, which is *the*
  congestion signal in this system (design section 5). Enable with `--set keda.enabled=true`.

The `nats-jetstream` scaler polls the NATS **monitoring** port (8222), not the client port. That
is why the chart's NATS config enables `http: 8222` and the Service publishes it. With an
external NATS you must set `nats.external.monitoringEndpoint` as well as `nats.external.url`.

When anything other than the Deployment owns the replica count, `replicas` is omitted from the
Deployment so Helm and the autoscaler do not fight over it on every upgrade.

## Pod security

Design section 10, applied to every pod:

```yaml
podSecurityContext:       { runAsNonRoot: true, runAsUser: 65532, runAsGroup: 65532,
                            fsGroup: 65532, seccompProfile: { type: RuntimeDefault } }
containerSecurityContext: { allowPrivilegeEscalation: false, readOnlyRootFilesystem: true,
                            runAsNonRoot: true, capabilities: { drop: [ALL] } }
```

Because the root filesystem is read-only, every pod gets a `/tmp` `emptyDir`. Service accounts
have `automountServiceAccountToken: false` except the console and the malaise engine, the only
two workloads that talk to the Kubernetes API.

The stores run as the uid their images expect (postgres 999, valkey 999, influxdb 1500) and nats,
whose image has no `USER`, as 10000. They get the extra writable `emptyDir`s their software
insists on: `/var/run/postgresql` for the Postgres socket, `/run/valkey`, `/plugins` for InfluxDB.

## RBAC

Namespaced `Role`s only. Nothing in the Encabulator holds a `ClusterRole`.

| Subject | Resources | Verbs |
|---|---|---|
| `malaise-injection-engine` | `pods` | get, list, delete |
| | `deployments` | get, list, patch |
| | `deployments/scale` | get, patch |
| | `networkpolicies` | get, list, create, delete |
| `encabulation-control-console` | `pods`, `deployments` | get, list, watch |

## Secrets

Generated on first install and preserved on upgrade with `lookup`:

| Secret | Keys |
|---|---|
| `<release>-app` | `jwt-private-key` (`genPrivateKey "rsa"`), `bootstrap-admin-password`, `bootstrap-operator-password`, `session-secret`, `customer-password-seed` |
| `<release>-postgres` | `postgres-password` plus one per logical database |
| `<release>-valkey` | `valkey-password`, `requirepass.conf` |
| `<release>-influxdb` | `admin-token`, `admin-token.json` |

Two things worth knowing:

- `lookup` returns nothing during `helm template`, so a rendered manifest always shows fresh
  random values. That is expected. Only a real `helm upgrade` preserves them.
- Pin a value with `secrets.app.*`, or take the chart out of the secret business entirely with
  `secrets.existingSecret` and each store's `external.existingSecret`.

## InfluxDB 3 Core authentication

The awkward one, so it is written down.

InfluxDB 3 Core has no "set the admin token from an environment variable" option. The operator
token is normally minted interactively with `influxdb3 create token --admin` against a *running*
server. For automated deployments it also accepts an **offline admin token file**
(`--admin-token-file` / `INFLUXDB3_ADMIN_TOKEN_FILE`): JSON of the form

```json
{ "token": "apiv3_...", "name": "encabulator-admin" }
```

which the server SHA-512-hashes into its catalog at startup. The token must begin with `apiv3_`,
and the load is idempotent: a token of that name already in the catalog is skipped, so restarts
and upgrades are fine.

So the chart generates `apiv3_<randAlphaNum 48>` once, preserves it with `lookup`, and stores it
in one Secret twice: `admin-token` (the raw string, which the recorder reads as `INFLUXDB_TOKEN`)
and `admin-token.json` (the file this StatefulSet mounts). No init job, no interactive step, and
no `--without-auth`.

Two consequences:

- `INFLUXDB3_DISABLE_AUTHZ=health,ping,metrics` exempts exactly those three paths, so the kubelet
  probes and a Prometheus scrape do not need to carry the token. Writes and queries stay
  authenticated.
- InfluxDB logs a warning when the token file is group- or world-readable. Secret volume files are
  owned `root:fsGroup`, so the file *has* to be group-readable (`0440`) for uid 1500 to read it at
  all. The warning is cosmetic; there is no mode that both silences it and works.

`influxdb.auth.enabled: false` falls back to `INFLUXDB3_WITHOUT_AUTH`. That is for throwaway
clusters, and `helm install` prints a warning when you use it.

## InfluxDB 3 Core write latency

InfluxDB 3 Core acknowledges a write only once the WAL flush that holds it has completed, every
1s by default (`--wal-flush-interval`, env `INFLUXDB3_WAL_FLUSH_INTERVAL`). With the default every
recorder write waits 0 to 1s, the recorder's p99 sits near 1s, and the console colours the
recorder and InfluxDB red. `influxdb.walFlushInterval` (default `50ms`) sets the env var on the
StatefulSet, which keeps writes under 100ms. The cost is more, smaller WAL files and object-store
writes, and a snapshot every 30s rather than every 10 minutes (600 WAL files per snapshot). An
empty value leaves InfluxDB's own default. Changing it restarts `influxdb-0` once; the data PVC
stays.

## Going external (phase 3)

Every store has an `external:` block, so pointing at RDS, ElastiCache and Timestream for InfluxDB
is a values change rather than a code change.

```yaml
postgres:
  external:
    enabled: true
    host: encabulator.cluster-xyz.eu-west-1.rds.amazonaws.com
    port: 5432
    sslmode: require
    existingSecret: encabulator-rds        # keys as in postgres.databases.<n>.passwordKey
valkey:
  external:
    enabled: true
    host: encabulator.serverless.euw1.cache.amazonaws.com
    port: 6379
    tls: true                              # rediss://
    existingSecret: encabulator-elasticache
nats:
  external:
    enabled: true
    url: nats://nats.shared.svc:4222
    monitoringEndpoint: nats.shared.svc:8222
influxdb:
  external:
    enabled: true
    url: https://abc.timestream-influxdb.eu-west-1.on.aws:8086
    existingSecret: encabulator-timestream
    existingSecretKey: token
```

With `external.enabled` the StatefulSet, its Services and its ConfigMap are not rendered, and the
services' `DATABASE_URL` / `VALKEY_URL` / `NATS_URL` / `INFLUXDB_URL` point at the endpoint you
gave. `helm-validate.sh` covers the all-external combination.

## Console ingress

Off by default: `ClusterIP` plus `make console` is enough for a demo.

```yaml
ingress:
  enabled: true
  className: alb                     # nginx, traefik, whatever you have
  hosts:
    - host: works.example.com
```

The default annotations exist because the console's live feed is Server-Sent Events, and a 60
second idle timeout cuts it every minute. They raise the ALB idle timeout to an hour and turn off
nginx proxy buffering. If you use a different controller, replace them with its equivalents.

## Alerts

`observability.prometheusRule.enabled` renders the alerts from design section 9: high error ratio,
high p99, casing line backlog, parts stock low and exhausted, pod crash looping, no successful
scrape, and a high refabrication rate. Thresholds are values, not hard-coded:

```yaml
observability:
  prometheusRule:
    errorRatio: 0.05
    latencyP99Seconds: 1.0
    casingLineBacklog: 100
    partsStockFloor: 50
    for: 5m
```

The casing line backlog alert reads `keda_scaler_metrics_value`, falling back to
`nats_consumer_num_pending` if you add a NATS Prometheus exporter later. v1 has no exporter, so
without KEDA that one alert has no data.

## Extra demo scenarios

The orchestrator ships six built-in scenarios inside its image and reads anything else from
`SCENARIO_DIR`:

```yaml
extraScenarios:
  marzlevane-drought.yaml: |
    id: marzlevane-drought
    title: Marzlevane drought
    steps:
      - say: "Stopping the amulite foundry."
      - flag: { key: amulite-foundry-rate, value: 0 }
      - waitFor: { promql: 'min(encabulator_parts_stock{sku="HYD-MARZ"})', op: "<", value: 10, timeout: 300s }
      - say: "Watch the 409s."
```

Keys become file names. The Deployment carries a checksum of this block, so an edit rolls the
orchestrator.

## Validating a change

```bash
make lint-charts
```

`scripts/helm-validate.sh` runs `helm lint`, then renders the chart across twelve value
combinations (KEDA on, ingress on, every store external, no Prometheus CRDs, tracing off,
ephemeral stores, autoscaling on and off, externally managed secrets, extra scenarios), and
validates every object against the Kubernetes and CRD JSON schemas with `kubeconform -strict`. It
forces `KUBECONFIG=/dev/null` so `lookup` cannot reach a cluster.

## Things this chart does not do

- **No `helm test` hooks.** Readiness probes and `--wait` already prove the factory came up.
- **No NATS subchart.** Design section 10 lists NATS as one of the four StatefulSets in *this*
  chart, it has to carry the chart's shared labels and pod security context, and a single-replica
  JetStream node is about sixty lines against the upstream chart's config-reloader and accounts
  surface. It also keeps `helm template` dependency-free, so CI needs no `helm dependency update`.
  Use `nats.external` when you want a real cluster.
- **No store metrics exporters.** The Backing Stores dashboard reads kube-state-metrics, cAdvisor
  and the OTel collector's service graph edges instead.
- **No replication in any store.** Single replica, one PVC each. `hydrocoptic-outage` is a demo
  scenario, and a store that cannot be killed is not much of a demo.
