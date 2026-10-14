# malaise-injection-engine

> Induces malaise for the purpose of demonstrating that the encabulator recovers from it. The
> engine is the only part of the works that is supposed to make things worse, and the only part
> that keeps a promise to put them back.

Two kinds of suffering are available. **App-level** malaise (`latency`, `error`, `congestion`,
`cpu-burn`) is published as *directives*: each service's kit middleware polls
`GET /v1/directives?service=<name>` every two seconds and applies them itself. **Kubernetes-level**
malaise (`pod-kill`, `partition`, `scale`) is applied by this service against the API server.

Every malaise has an expiry, and a 2-second reconcile loop honours it. The loop does not trust its
own memory: the objects it touches carry the malaise id as a label and the expiry as an annotation,
so a restart · even a restart with a wiped Valkey · finds the evidence and reverts it. A stranded
NetworkPolicy would be a much worse demo than a broken one.

## Kinds

### App-level

| Kind | Params | Effect (inside the target service) |
|---|---|---|
| `latency` | `ms` (1 to 60000), `jitterMs` (≤ `ms`), `percent` (default 100) | Sleep `ms ± jitterMs` before the handler |
| `error` | `status` (400 to 599), `percent` (default 100) | Respond `status` with `malaise_injected` |
| `congestion` | `maxConcurrent` (≥ 1), `queueTimeoutMs` (default 2000) | Limit concurrency; then 503 `congested` |
| `cpu-burn` | `millicores` (1 to 8000) | Burn CPU in the background until expiry |

App-level targets must be services in the catalog that actually run the middleware. The control
plane · console, grammeter, engine, orchestrator · does not inject malaise into itself, so naming one
of those is a `400 unknown_target`.

### Kubernetes-level

| Kind | Params | Effect |
|---|---|---|
| `pod-kill` | `count` (1 to 20, default 1) | Delete `count` random *ready* pods of the target. Instant. |
| `partition` | `from` (a name, a list of names, or `"*"`; default `"*"`) | NetworkPolicy isolating the target's pods |
| `scale` | `replicas` (0 to 20) | Patch the target Deployment's replicas |

Kubernetes-level targets may be **any** workload in the namespace labelled
`encabulator.io/malaise-target=true` · Pod, Deployment or StatefulSet · which is how the backing
stores (`postgres`, `valkey`, `nats`, `influxdb`) become targetable even though they are not
services in the catalog.

**`partition`** builds an ingress-only NetworkPolicy selecting `app.kubernetes.io/name=<target>`. The
observability namespace is *always* an allowed peer, so Prometheus keeps scraping the very pods being
broken and the console can show the damage. With named peers, "everything except those" is expressed
as a `NotIn` match expression, which also matches pods carrying no `app.kubernetes.io/name` label at
all:

```yaml
spec:
  podSelector:
    matchLabels: {app.kubernetes.io/name: hydrocoptic-datastore}
  policyTypes: [Ingress]
  ingress:
  - from:
    - namespaceSelector:
        matchLabels: {kubernetes.io/metadata.name: encabulator-observability}
    - podSelector:
        matchExpressions:
        - {key: app.kubernetes.io/name, operator: NotIn, values: [panametric-fam-processor]}
```

With `from: "*"` the pod-selector peer is omitted entirely, leaving only the observability namespace.

> A partition needs a CNI that enforces NetworkPolicy. On EKS this means the VPC CNI with network
> policy enabled, or Calico/Cilium. `kind`'s default CNI enforces nothing, so a partition there
> creates the object and changes no traffic.

**`scale`** remembers the true original replica count in `encabulator.io/original-replicas` before
changing it, and restores it on expiry. Scaling a Deployment that is already scaled keeps the *first*
original, so a second malaise cannot lose it. The target is resolved by Deployment name first, then
by the `app.kubernetes.io/name` label, so a Helm release name that differs from the service name
still works.

**`pod-kill`** is instant: the record comes back with `state: "completed"`, `endedAt` set, and the
pods it deleted in `detail.killedPods`. There is nothing to revert · the ReplicaSet makes the
replacement, which is the point.

## API

### `POST /v1/malaises`

```bash
curl -X POST localhost:8080/v1/malaises -H 'content-type: application/json' -d '{
  "target": "spurving-bearing-quoter",
  "kind": "latency",
  "params": {"ms": 400, "jitterMs": 100, "percent": 50},
  "durationSeconds": 120
}'
```

`durationSeconds` is 1 to 900 and defaults to 120. `201` with the record and a `Location` header:

```json
{
  "id": "mal_0199c7f0-...",
  "target": "spurving-bearing-quoter",
  "kind": "latency",
  "params": {"ms": 400, "jitterMs": 100, "percent": 50},
  "durationSeconds": 120,
  "state": "active",
  "createdAt": "2026-09-30T12:00:00Z",
  "expiresAt": "2026-09-30T12:02:00Z",
  "remainingSeconds": 120
}
```

Parameters belonging to other kinds are discarded rather than stored, so a record never claims a
parameter it does not honour.

| Error | Status | Code |
|---|---|---|
| Body is not JSON, or has unknown fields | 400 | `invalid_json` |
| Missing or out-of-range parameters | 400 | `invalid_malaise` |
| Target is not a catalogued service, or not labelled for malaise | 400 | `unknown_target` |
| A Kubernetes-level kind with no API server | 503 | `kubernetes_unavailable` |

### The rest

| Method | Path | Returns |
|---|---|---|
| `GET` | `/v1/malaises?state=active\|all&limit=100` | `{malaises: [...], count}`; active is soonest-expiry first, history is newest first |
| `GET` | `/v1/malaises/{id}` | One record, or 404 |
| `DELETE` | `/v1/malaises/{id}` | Ends one early, reverting any cluster change |
| `DELETE` | `/v1/malaises` | `{cleared: n}` · ends everything active |
| `GET` | `/v1/directives?service=` | `{directives: [{id, kind, params, expiresAt}]}` for the middleware |
| `GET` | `/v1/targets` | The catalog, the app-level subset, the kinds, and whether Kubernetes is armed |

`GET /v1/directives` is the hot path: every service in three planes polls it every two seconds. It
returns only *app-level, still-active, unexpired* malaises for that one service, never the
Kubernetes-level ones.

## State

Everything lives under the `{malaise}:*` keys this service owns in Valkey:

| Key | Type | Holds |
|---|---|---|
| `{malaise}:record:<id>` | string | The JSON record, TTL = expiry + 2h so recent history is readable |
| `{malaise}:active` | sorted set | Ids scored by expiry, so "what is due?" is one round trip |
| `{malaise}:history` | sorted set | Ids scored by creation, trimmed to 500 |

Plus the cluster itself: labels `encabulator.io/malaise-id` and
`app.kubernetes.io/managed-by=malaise-injection-engine`, annotations
`encabulator.io/expires-at` and `encabulator.io/original-replicas`.

The reconcile loop runs once at startup before the server accepts traffic, then every 2 seconds. It
expires due malaises, reverts their cluster changes, and sweeps for objects whose annotation says
they are overdue regardless of what the store remembers.

## Environment

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | |
| `OTEL_SERVICE_NAME` | `malaise-injection-engine` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | Unset disables tracing |
| `VALKEY_URL` | **required** | e.g. `redis://default:$(VALKEY_PASSWORD)@valkey:6379/0` |
| `POD_NAMESPACE` | `encabulator` | Downward API; the only namespace the engine touches |
| `OBSERVABILITY_NAMESPACE` | `encabulator-observability` | Always allowed through a partition |

`MALAISE_ENGINE_URL` is deliberately ignored: somebody has to stay well enough to stop the others
suffering.

## Readiness

- `valkey` · a `PING`. The engine's own database, so a hard dependency.
- `kubernetes` · a one-item pod list, registered only when the in-cluster config was found.

Outside a cluster the engine logs a warning, skips the Kubernetes readiness check, and serves the
app-level kinds normally, which makes `make run` useful.

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `encabulator_malaises_active` | gauge | `kind` (zero published for every kind at startup) |
| `encabulator_malaises_total` | counter | `kind`, `target` |
| `http_server_request_duration_seconds` | histogram | `http_request_method`, `http_route`, `http_response_status_code` |

## RBAC

The engine needs a **namespaced Role in `encabulator` only**. It never reads or writes anything
outside `POD_NAMESPACE`. This sample is for reference; the chart author owns the real one.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: malaise-injection-engine
  namespace: encabulator
rules:
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, delete]
  - apiGroups: [apps]
    resources: [deployments]
    verbs: [get, list, patch]
  - apiGroups: [apps]
    resources: [deployments/scale]
    verbs: [get, patch]
  - apiGroups: [apps]
    resources: [statefulsets]
    verbs: [get, list]
  - apiGroups: [networking.k8s.io]
    resources: [networkpolicies]
    verbs: [get, list, create, delete]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: malaise-injection-engine
  namespace: encabulator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: malaise-injection-engine
subjects:
  - kind: ServiceAccount
    name: malaise-injection-engine
    namespace: encabulator
```

`statefulsets get,list` is beyond the design's list; target validation needs it so that the backing
stores can be named. `deployments/scale` is listed because the design calls for it; the replica
change is made as a single merge patch on the Deployment (covered by `deployments patch`) so that the
replica count, the malaise-id label and the expiry annotation land atomically.

The pod needs its service account token mounted. Nothing else: no volumes beyond `/tmp`, and the root
filesystem can be read-only.

## Running locally

```bash
make build                                     # bin/malaise-injection-engine
make test                                      # unit tests: miniredis + client-go fake clientset
source ../../../dev-env.sh && make test-integration   # adds the Testcontainers Valkey test
make lint                                      # gofmt -l plus go vet
make run                                       # needs a Valkey on localhost:6379
make image                                     # context is the repository root
```

```bash
finch run -d --name valkey -p 6379:6379 valkey/valkey:9
make run &
curl -sX POST localhost:8080/v1/malaises -H 'content-type: application/json' \
  -d '{"target":"spurving-bearing-quoter","kind":"cpu-burn","params":{"millicores":500},"durationSeconds":30}' | jq .
curl -s 'localhost:8080/v1/directives?service=spurving-bearing-quoter' | jq .
curl -sX DELETE localhost:8080/v1/malaises | jq .
```

## Layout

| Package | What it holds |
|---|---|
| `internal/malaise` | Kinds, parameter validation and normalisation, the record, the directive |
| `internal/catalog` | The 15 services and the app-level subset |
| `internal/store` | Valkey persistence under `{malaise}:*` |
| `internal/kube` | Apply, revert and sweep against the Kubernetes API |
| `internal/engine` | Lifecycle, the reconcile loop, metrics |
| `internal/api` | The HTTP surface |
