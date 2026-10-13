# platform/

Pinned values for the upstream charts that make the Encabulator observable, plus the Grafana
dashboards. Nothing in here is an Encabulator workload; those live in
[`charts/encabulator`](../charts/encabulator/).

```bash
make platform             # install all four, in order, then the dashboards
make platform-status      # what is installed where
make platform-uninstall   # remove them again
```

`scripts/platform.sh` does the work. Versions come from [`versions.env`](versions.env) and
nowhere else.

## What gets installed

| Chart | Version | App | Namespace | Values |
|---|---|---|---|---|
| `prometheus-community/kube-prometheus-stack` | 91.8.2 | Prometheus Operator v0.94.1 | `encabulator-observability` | [`kube-prometheus-stack.values.yaml`](kube-prometheus-stack.values.yaml) |
| `jaegertracing/jaeger` | 4.14.0 | Jaeger v2.21.0 | `encabulator-observability` | [`jaeger.values.yaml`](jaeger.values.yaml) |
| `open-telemetry/opentelemetry-collector` | 0.174.0 | Collector 0.161.0 | `encabulator-observability` | [`opentelemetry-collector.values.yaml`](opentelemetry-collector.values.yaml) |
| `kedacore/keda` | 2.21.0 | KEDA 2.21.0 | `keda` | [`keda.values.yaml`](keda.values.yaml) |

Checked against the live repositories on 2026-09-30; these were the latest releases.

The order is not arbitrary:

1. **kube-prometheus-stack** brings the Prometheus Operator CRDs. Everything that follows ships a
   `ServiceMonitor`, so the CRDs have to exist first.
2. **Jaeger** before the collector, so the collector's OTLP exporter has somewhere to send.
3. **Collector**, which receives OTLP from the services, forwards traces to Jaeger, and derives
   the service graph and span metrics.
4. **KEDA**, in its own namespace, bringing the `ScaledObject` CRD that
   `charts/encabulator --set keda.enabled=true` needs.

## How Prometheus finds the Encabulator

One decision shapes these files: Prometheus selects `ServiceMonitor` and `PodMonitor` objects
labelled `encabulator.io/scrape=true`, in any namespace (design section 9), instead of the chart's
default release-scoped selector.

That has a consequence people trip over. Once you replace the selector, the stack's *own* monitors
stop matching, and you lose kube-state-metrics, node-exporter and the kubelet. So the label is
applied to those too:

| Where | Key |
|---|---|
| everything kube-prometheus-stack creates | `commonLabels` |
| kube-state-metrics | `kube-state-metrics.prometheus.monitor.additionalLabels` |
| node-exporter | `prometheus-node-exporter.prometheus.monitor.additionalLabels` |
| Grafana | `grafana.serviceMonitor.labels` |
| Prometheus Operator | `prometheusOperator.serviceMonitor.additionalLabels` |
| the collector | `serviceMonitor.extraLabels` |
| Jaeger | a hand-written `ServiceMonitor` in `extraObjects` (the chart has no option) |
| KEDA | `prometheus.*.serviceMonitor.additionalLabels` |

`PrometheusRule` is the exception. Its selector is left **empty**, which selects every rule in
every namespace, because a single `LabelSelector` cannot express "the stack's built-in Kubernetes
alerts OR the Encabulator's own rules" (`matchExpressions` are ANDed). The Encabulator chart also
labels its `PrometheusRule` with `release: prometheus-stack` so it still works if you put the
default selector back.

## The collector, and why it is one replica

`mode: deployment`, `replicaCount: 1`, on purpose. The `service_graph` connector pairs a client
span with its server span to produce an edge, and spans of one trace spread across replicas do not
pair reliably. Scaling it out means putting the `loadbalancing` exporter in front, which v1 does
not need. If you do scale it, expect the console's topology edges to go patchy.

Two ports leave the collector:

- **8888**: the collector's own internal telemetry.
- **8889**: a `prometheus` exporter publishing the connector output. This is where
  `traces_service_graph_request_total`, `traces_service_graph_request_failed_total` and
  `traces_service_graph_request_server_seconds_bucket` come from, which is to say: this is where
  the console's Factory Floor gets every edge on the topology.

Both are scraped by the chart's `ServiceMonitor`.

### Virtual nodes are how the stores appear at all

Nothing instruments PostgreSQL, Valkey, NATS or InfluxDB. They only ever exist as the far end of a
client span. Two settings of the `service_graph` connector turn such a span into a server node:

```yaml
database_name_attributes: [db.system.name, db.system]
virtual_node_peer_attributes:
  [peer.service, db.system, db.system.name, server.address,
   messaging.destination.name, db.name, db.namespace]
virtual_node_extra_label: true
```

- A client span carrying `db.system.name` or `db.system` is a database call. The connector does not
  wait for a server span: it emits the edge at once with `connection_type="database"`, uses the
  client span's duration as the edge latency, and names the server after the attribute's value
  (`postgresql`, `valkey`, `redis`), which the console maps to its `postgres` and `valkey` nodes.
  The console has one node per database *system*, so the server must not be named after the
  database: `db.namespace`/`db.name` would give `hydrocoptic`, `detractor`, `girdlespring` and the
  Redis index `0`, none of which is a node. Do not simply delete the key either: collector 0.161.0
  falls back to `[db.name]` when the list is empty.
- Any other client span that finds no server span within the store TTL becomes a *virtual* server
  node named after the first attribute in `virtual_node_peer_attributes` it carries, or `unknown`.
- Producer and consumer spans are paired into `connection_type="messaging_system"` edges between the
  publishing and the consuming service. An unpaired producer span never becomes a virtual node.

If a store is missing from the topology, the span is missing one of these. Design section 7
requires `peer.service` or `server.address` on outbound client spans and `db.system` plus
`db.name`/`db.namespace` on database and cache clients, which is exactly the set above.

Both connectors use their current component names (`service_graph`, `span_metrics`). The old
`servicegraph` / `spanmetrics` names still work in collector 0.161.0 but are deprecated, and the
chart's `rewriteDeprecatedComponentNames` would rename them anyway.

## Dashboards

Three, in [`dashboards/`](dashboards/), installed as ConfigMaps labelled `grafana_dashboard=1` and
annotated `grafana_folder=Encabulator`, which is what the kube-prometheus-stack Grafana sidecar
watches for.

| UID | Title | Reads |
|---|---|---|
| `encabulator-works` | Encabulator Works | the `encabulator_*` domain metrics: orders, units fabricated, stage durations, inspection results, parts stock, quote compute time, malaise applied, gateway throughput |
| `encabulator-service-red` | Encabulator Service RED | `http_server_request_duration_seconds`, `http_client_request_duration_seconds`, and the service graph edge table |
| `encabulator-backing-stores` | Encabulator Backing Stores | kube-state-metrics, cAdvisor, `kubelet_volume_stats_*`, the database and messaging service graph edges, and `keda_scaler_metrics_value` for casing line lag |

Every panel says "no data" when a metric is absent. Nothing here invents a number, which is design
principle 2 applied to the dashboards as well as the console.

Re-install just the dashboards after editing one:

```bash
./scripts/platform.sh dashboards
```

## Reaching the UIs

```bash
make grafana      # http://localhost:3000
make jaeger       # http://localhost:16686
make prometheus   # http://localhost:9090
make credentials  # includes the generated Grafana admin password
```

Grafana allows anonymous `Viewer` access so the console's "open in Grafana" link works without a
login prompt in the middle of a demo. Turn that off in
`grafana.grafana\.ini.auth\.anonymous.enabled` if the cluster is not yours.

## Storage, or the lack of it

Prometheus keeps 6 hours on an `emptyDir` and Jaeger keeps traces in memory. This is a demo: the
interesting window is the last few minutes, and a pod restart losing history is cheaper than
provisioning volumes. The commented-out `storageSpec` in the Prometheus values is the switch if
you disagree.

## Bumping a version

One line in `versions.env` at a time, then:

```bash
./scripts/platform.sh template > /dev/null   # catches renamed keys and schema rejections
make platform
```

CI runs that `template` step on every pull request, which is how a values key that upstream has
renamed gets caught before it reaches a cluster.

## What is deliberately not here

- **No Loki, no log pipeline.** Services log JSON lines to stdout (design section 7) and `kubectl
  logs` is enough for v1. The collector's `logs` pipeline is set to `null`.
- **No Alertmanager receiver.** The route sends everything to a `null` receiver. Nothing is paged
  from a demo; the alerts exist so the Alertmanager UI has content and the console can show what
  the factory thinks of itself.
- **No control-plane alerts.** `defaultRules` turns off the etcd, scheduler, controller-manager and
  kube-proxy rules, which fire permanently on EKS and every other managed control plane.
- **No Tempo.** Jaeger v2 is the trace store; the service graph comes from the collector's
  connector rather than from Tempo's metrics generator.
