# gitops/app: the Encabulator as kro APIs

What Argo CD syncs to every cell (Act 2 and Act 3 of OPN315). No Helm: two kro
ResourceGraphDefinitions turn one `Encabulator` object into the AWS stores (through ACK), the
in-cluster stores and the fifteen services, matching what `charts/encabulator` renders.

```
gitops/app/
  kustomization.yaml            root Argo CD syncs (rbac, nodes, rgds, instance)
  rbac/namespace.yaml           namespace encabulator                              wave -1
  rbac/kro.yaml                 RBAC for the kro capability, group encabulator:kro  wave -1
  nodes/auto-mode.yaml          tagged Auto Mode NodeClass + NodePool               wave -1
  rgds/encabulator.yaml         RGD encabulator, kind Encabulator                   wave 0
  rgds/encabulator-service.yaml RGD encabulator-service, kind EncabulatorService    wave 0
  instance/                     the Encabulator instance (kustomize dir)            wave 1
  hack/                         validation: parity.py, rbac-check.py, validate-kind.sh
```

## Pinned versions

| What | Version | Where it is used |
|---|---|---|
| kro | 0.9.4 (`oci://registry.k8s.io/kro/charts/kro`, image `registry.k8s.io/kro/kro:v0.9.4`) | the RGDs; the EKS kro capability on cells |
| ACK rds-controller CRDs | 1.13.1 (`oci://public.ecr.aws/aws-controllers-k8s/rds-chart`) | DBSubnetGroup, DBInstance schemas |
| ACK elasticache-controller CRDs | 1.10.1 (`oci://public.ecr.aws/aws-controllers-k8s/elasticache-chart`) | ServerlessCache schema |
| RDS for PostgreSQL | engine `postgres`, major `18` (RDS picks the minor; 18.6 as of 2026-08) | `rgds/encabulator.yaml` |
| ElastiCache Serverless | engine `valkey`, major `8` (lead decision D2; 9.0 and 9.1 exist for Serverless) | `rgds/encabulator.yaml` |
| secret-init image | `docker.io/library/python:3.13.16-slim-trixie@sha256:70729b46...c2f` (Python 3.13.16, OpenSSL 3.5.7) | `secretInit` Job |
| db-init image | `docker.io/library/postgres:18.6-trixie@sha256:74935e72...4336` | `dbInit` Job |
| Service images | `<spec.registry>/<service>:<spec.imageTag>`, default `ghcr.io/jlbutler/cloud-native-encabulator/<service>:1.0.3` | `spec.registry`, `spec.imageTag` |
| NATS, InfluxDB images | `nats:2.12`, `influxdb:3-core` (the chart's tags, unchanged); InfluxDB flushes its WAL every 50 ms (`INFLUXDB3_WAL_FLUSH_INTERVAL`, the chart's `influxdb.walFlushInterval` default) and lets one query scan up to 4320 Parquet files (`INFLUXDB3_QUERY_FILE_LIMIT`, the chart's `influxdb.queryFileLimit` default) | StatefulSets |
| Validation | kind cluster on `kindest/node` (shared `opn315-cell`, Kubernetes 1.35.8), helm 4.2.4, kubectl 1.35.0, Python 3.12 + PyYAML 6.0.3 | `hack/` |

## The two APIs

```yaml
apiVersion: kro.run/v1alpha1
kind: Encabulator
metadata: {name: encabulator, namespace: encabulator}
spec:
  imageTag: "1.0.3"               # default 1.0.3
  registry: ghcr.io/jlbutler/cloud-native-encabulator  # the default; a mirror per cell, see below
  dbInstanceClass: db.t4g.medium  # default db.t4g.medium
  subnetIDs: [...]                # patched in per cell
  securityGroupID: sg-...         # patched in per cell, required
  services: [15 names]
status:
  databaseHost: ...               # from DBInstance status
  cacheHost: ...                  # from ServerlessCache status
```

`EncabulatorService` is the smaller API the Encabulator creates once per name: `spec.tag`,
`spec.registry` and
`spec.storesConfig` (the ConfigMap with the store endpoints). Everything that differs between
services lives in lookup tables inside `rgds/encabulator-service.yaml`, keyed by the instance
name, copied from the `services:` and `wiring:` blocks of `charts/encabulator/values.yaml`. A
name with no entry gets the chart's `defaults:`, so adding a name to `spec.services` adds a
service.

## How the graph is ordered

There are no sync waves or hooks inside the graph. Every `${...}` that reads another resource
is an edge, and `readyWhen` says when a resource counts as ready:

```
dbSubnets ─┐
secretInit ┴─> db (dbInstanceStatus == available) ─> dbInit (succeeded) ─┐
cache (status == available) ──────────────────────────────────────────────┴─> stores ConfigMap ─> 15 x EncabulatorService
storageClass ─> nats, influxdb (influxdb also reads its token Secret name from secretInit)
```

- `db` takes its admin password Secret name from the `secretInit` Job's annotations, so RDS
  is never asked to create an instance before its password exists.
- `stores` is the ConfigMap `encabulator-aws` (`POSTGRES_HOST`, `POSTGRES_PORT`, `VALKEY_HOST`,
  `VALKEY_PORT`), the same name and keys Terraform writes on the hub in Act 1, now built from
  ACK status. It also references `dbInit`, so it is published only once the databases exist.
- Each `EncabulatorService` reads that ConfigMap with an `externalRef`, so no service is
  created before the endpoints are real, and the hosts land in `DATABASE_URL` and `VALKEY_URL`.

The AWS resources carry the Act 1 annotations: `services.k8s.aws/adoption-policy:
adopt-or-create` (an `encabulator-pg` that already exists in the region is adopted, otherwise
created; with no `adoption-fields` annotation ACK takes the identifiers from the spec) and
`services.k8s.aws/deletion-policy: retain`. Tags are exactly the three contract tags (D3). The
`encabulator-gp3` StorageClass puts the same three on every PVC volume (`tagSpecification_1..3`)
and the DBInstance copies them to its backups (`copyTagsToSnapshot`); what RDS and ElastiCache
create without tags (their network interfaces, the VPC endpoints ElastiCache Serverless creates,
cache snapshots) is tagged by the hub's tagger (`docs/TAGGING.md`).

## Secrets kro cannot generate

The chart generates the database passwords, the JWT signing key, the session secret, the
customer password seed and the InfluxDB token with `randAlphaNum` / `genPrivateKey`. kro has
`random.seededString`, but it is seeded by a value such as the instance UID that anyone who
can read the object can see, so it is not a secret; and CEL cannot make an RSA key. So:

- `secretInit`, a Job with its own ServiceAccount and Role (create Secrets; get only the four
  names below), creates each Secret **only if it is absent**, never updates one and never
  prints a value. Re-running it is a no-op, so the RDS admin password never changes under
  the database. Secrets and key names match the chart:

  | Secret | Keys |
  |---|---|
  | `encabulator-db` (the chart's `postgres.external.existingSecret`) | `postgres-password`, `hydrocoptic-password`, `detractor-password`, `girdlespring-password` (24 alphanumeric) |
  | `encabulator-app` | `jwt-private-key` (RSA 4096, PKCS#1 PEM like `genPrivateKey "rsa"`), `bootstrap-admin-password`, `bootstrap-operator-password`, `session-secret`, `customer-password-seed` |
  | `encabulator-valkey` | `valkey-password` = "" (ElastiCache Serverless with no user group: TLS, no password) |
  | `encabulator-influxdb` | `admin-token` (`apiv3_` + 48), `admin-token.json` (the offline admin token file) |

- The DBInstance reads `masterUserPassword` from `encabulator-db`/`postgres-password`.
- `dbInit` (postgres:18.6) runs once the DBInstance is available: one role and one database
  per data-owning service, owner-only CONNECT, the chart's SQL with one RDS addition. The RDS
  admin user (`masterUsername`) is not a superuser, and since PostgreSQL 16 it may only create a database owned
  by a role it can `SET ROLE` to, so it grants itself each role first. Idempotent.

Kubernetes Secrets are not encrypted beyond what the cluster provides (EKS envelope
encryption with KMS). Rotating a value means deleting the Secret and re-running the Job, and
for the database passwords also changing them in PostgreSQL.

## RBAC for kro (`rbac/kro.yaml`)

Bound to the group `encabulator:kro` (lead decision D4): the cell's kro capability access entry
carries that Kubernetes group, so no role ARN or account id is in Git.

| ClusterRole | Bound by | Grants |
|---|---|---|
| `encabulator:kro:read` | ClusterRoleBinding | get/list/watch on the kinds the RGDs create (kro watches them cluster-wide) |
| `encabulator:kro:write` | RoleBinding in `encabulator` | create/update/patch/delete on those kinds, instance status and events; plus the permissions of the Roles kro creates (pods get/list/delete, deployments/scale, networkpolicies, secrets create and get on the four names), because Kubernetes only lets kro create a Role whose permissions it holds |
| `encabulator:kro:storageclass` | ClusterRoleBinding | create StorageClasses; update/patch/delete only `encabulator-gp3` |

kro never reads Secrets: it does not manage any, and nothing in the graph uses an `externalRef`
to one. No wildcards anywhere.

## Nodes (`nodes/auto-mode.yaml`)

Lead decision D8: the cell has no built-in node pools; this NodeClass (the cell's node role
`<cell>-node`, the three contract tags on instances, volumes and ENIs) and the NodePool
equivalent to the general-purpose pool replace them, same spec as the hub's. The role, the
subnets and the cluster security group are placeholders that the cell ApplicationSet replaces
with patches (the role from the cell's name, so each cell's nodes join with the role that has
the cell's EC2 access entry).

The NodePool never consolidates a node that still runs pods (a `nodes: "0"` budget for the
`Underutilized` reason). Without it, the nodes a rollout's surge pods add were repacked a few
minutes later, which moved `influxdb-0` (about 30 s down while its EBS volume re-attaches), the
console and the grammeter. Empty nodes are still removed, and drift (a new Auto Mode AMI, about
weekly) and `expireAfter` (336h) still replace nodes, so node upgrades keep happening. The cost is
underused nodes that stay until they are empty. Delete that budget to repack again.

## What the cell ApplicationSet patches

- `Encabulator/encabulator` in `encabulator`: `/spec/subnetIDs` and `/spec/securityGroupID`
  from the cluster Secret annotations `encabulator.io/subnet-ids` and
  `encabulator.io/security-group-id`, and `/spec/registry` from the optional
  `encabulator.io/registry` (an image mirror, set on the hub with `REGISTRY`; without it the
  images come from ghcr.io).
- every `eks.amazonaws.com` NodeClass: `/spec/role` (`<cell>-node`, from the cell's name),
  `/spec/subnetSelectorTerms` and `/spec/securityGroupSelectorTerms`.

Git keeps placeholders only (`subnet-0000...`, `sg-0000...`).

## Act 2 in the IDE

```sh
kubectl apply -f gitops/app/rgds/
kubectl get rgd
kubectl explain encabulator.spec
kubectl get encabulatorservices -n encabulator
kubectl get encabulator encabulator -n encabulator -o yaml | yq .status
```

## Differences from the chart, on purpose

- `app.kubernetes.io/managed-by: kro` instead of `Helm`, and no `helm.sh/chart` label.
- Not rendered: ServiceMonitor and PrometheusRule, the KEDA ScaledObject (off in the chart by
  default), the in-cluster postgres and valkey StatefulSets (RDS and ElastiCache replace them, as
  with the chart's `external` blocks). kro accepts an RGD only when every kind it names already
  exists, and the ServiceMonitor kind arrives with the observability platform, so the cell's
  ServiceMonitor for the fifteen services ships with that platform
  ([`platform/cell/`](../../platform/cell/)) and the app never waits for it.
- `DATABASE_URL` uses `sslmode=require`; `VALKEY_URL` is `rediss://:@<host>:6379/0`, the shape the
  chart renders with `values-brownfield.yaml` (`valkey.user: ""`), so no client sends `AUTH`.
- Autoscaled services (processor, quoter) leave `spec.replicas` to the HPA. kro 0.9 can only
  leave a field out with `omit()`, which is behind an alpha feature gate, so those two use a
  second Deployment node without `replicas`; YAML anchors keep the pod template single-sourced.
- kro reconciles: a field it sets that is changed by hand is put back. That is the point of
  the Day 2 demo, but it also means the malaise engine's `scale` malaise on a Deployment with
  a fixed replica count is reverted on the next reconcile. `pod-kill` and `partition` are
  unaffected.

## Validate it

```sh
gitops/app/hack/validate-kind.sh --kubeconfig <kind kubeconfig> --evidence <dir>
```

Refuses any API server that is not on 127.0.0.1. Installs kro and the ACK CRDs only if
missing (no ACK controller), applies `rbac/` and `rgds/`, waits for both RGDs to be Active
(which type-checks every CEL expression against the real ACK schemas), applies the instance
with `hack/kind/fake-aws.yaml` (a non-superuser "RDS" PostgreSQL so db-init runs for real, and
a pod quota so a shared cluster is not loaded with 26 service pods), fakes the DBInstance and
ServerlessCache status, waits for the instance to go ACTIVE and counts 15 Deployments, then
runs:

- `hack/parity.py`: renders `charts/encabulator` with `hack/cell-values.yaml` (RDS, ElastiCache,
  gp3) and compares, per service, env var names in order and every env entry, container, pod
  and deployment setting, labels, and the per-service Service, ServiceAccount, HPA, PDB, Role
  and RoleBinding; plus the store StatefulSets, Services, ConfigMap, StorageClass and Secret
  key names. `--helm-set` changes the chart side to prove the check fails on a real difference.
- `hack/rbac-check.py`: binds the `rbac/kro.yaml` ClusterRoles to a check-only group, then
  replays every kro-owned object as that group with a server-side dry-run apply (the API
  server's own authorization and RBAC escalation checks) and checks what must be refused.

## Not verified offline

Needs a real cell with the ACK capability, so it is untested here:

- ACK creating and adopting `encabulator-pg` and `encabulator-valkey`, and the real status
  fields kro waits for (`dbInstanceStatus`, ServerlessCache `status` and `endpoint`). The field
  names were checked against the 1.13.1 / 1.10.1 CRDs; values were faked.
- The ACK capability reading the `encabulator-db` Secret (it needs
  `AmazonEKSSecretReaderPolicy` scoped to namespace `encabulator` on the cell's ACK access
  entry).
- The services against ElastiCache Serverless itself. With `rediss://:@<host>` none of the five
  Valkey clients (go-redis, ioredis, redis-rs, redis-py) sends `AUTH`; all five pass PING/SET/GET
  over TLS against a TLS-only Valkey with that URL shape, but not against ElastiCache.
- The kro capability's identity carrying the `encabulator:kro` group, and the NodeClass and
  NodePool on Auto Mode (no Auto Mode CRDs on kind).
- The services themselves running against the real stores.

## Operational notes

- Job pod templates and StatefulSet `volumeClaimTemplates` are immutable. Changing them in an
  RGD fails the update for existing instances; delete the Job or StatefulSet (data stays on the
  PVC) and let kro recreate it. `kubectl delete statefulset <name> --cascade=orphan` keeps the
  store pod running until kro's new StatefulSet adopts it. The claim templates carry no
  `app.kubernetes.io/version`, so changing `spec.imageTag` never touches them.
- Deleting the instance deletes the Jobs but leaves their completed pods; the AWS resources stay
  (deletion-policy retain) and so do the generated Secrets (kro did not create them). Remove
  both deliberately at teardown.
