# Act 3: the hub creates a new region

`EncabulatorCell` is a kro API on the hub cluster `encabulator-usw2`. One instance is one
region: kro turns it into ACK resources, ACK on the hub creates them in the region named on
the instance, and the last resource in the graph registers the new cluster with the hub's
Argo CD. The `encabulator` ApplicationSet then syncs `gitops/app` to it, and ACK on the cell
creates the app's RDS instance and ElastiCache cache. Next to the app, the
`encabulator-platform` ApplicationSet installs the observability platform (Prometheus, Grafana,
Jaeger, the OpenTelemetry collector and the dashboards) from the same pinned charts as the hub.
No Terraform for the cell, and nothing installed on it by hand.

| File | What it is |
|---|---|
| `gitops/hub/rgds/encabulator-cell.yaml` | RGD `encabulator-cell`, kind `EncabulatorCell` |
| `gitops/hub/cells/use2.yaml` | the us-east-2 cell (`encabulator-use2`), the whole difference between one region and two |
| `gitops/hub/appset.yaml` | ApplicationSet `encabulator`: one Application per registered cell; ApplicationSet `encabulator-platform`: one Application per platform chart per cell that has it switched on |
| `gitops/hub/rbac/kro-cell.yaml` | namespace `encabulator-cells` and the RBAC kro on the hub needs for this RGD |

## Design

### The graph

kro orders the resources from their references (the RGD's `status.topologicalOrder`):

```
VPC ─┬─ InternetGateway ── public RouteTable ── public Subnet × AZ ─┐
     │                                ElasticIPAddress ── NATGateway ┘─ private RouteTable ── private Subnet × AZ
     │                                                    readyWhen state == available
IAM Roles: cluster, node, ack, kro (no dependencies, created at once)
     │
EKS Cluster (private subnets, cluster role) ── readyWhen status == ACTIVE
     ├─ AccessEntry node   (EC2, AmazonEKSAutoNodePolicy)
     ├─ AccessEntry ack    (AmazonEKSACKPolicy + AmazonEKSSecretReaderPolicy in ns encabulator) ── Capability ACK
     ├─ AccessEntry kro    (AmazonEKSKROPolicy + group encabulator:kro)                         ── Capability KRO
     ├─ AccessEntry argocd (hub Argo CD role, AmazonEKSClusterAdminPolicy)
     ├─ AccessEntry admin  (optional, spec.adminRoleName)
     └─ SecurityGroup encabulator-db (5432 + 6379 from the cluster security group, in the VPC)
                                   │
argocd cluster Secret ◄────────────┘  waits for both capabilities ACTIVE and the Argo CD entry
         ▲
hub's argocd cluster Secret (external reference, read only): the repository, revision and registry
```

- Every ACK resource carries `services.k8s.aws/region: ${schema.spec.region}` and the three
  contract tags (`auto-delete=no`, `project=encabulator`, `talk=opn315`). EC2 resources also get
  a `Name` tag. EC2 and IAM tags are `[{key, value}]` lists; EKS tags are maps. What EC2 and EKS
  create on their own (main route table, default security group and network ACL, cluster
  security group, requester-managed network interfaces) is tagged by the hub's tagger CronJob
  (`docs/TAGGING.md`).
- Subnets: `vpcCIDR` must be a /16 (schema pattern). Private subnets are /19s at `x.y.0.0`,
  `x.y.32.0`, `x.y.64.0`; public subnets are /24s at `x.y.200.0`, `x.y.201.0`, `x.y.202.0`,
  the same split the hub's Terraform uses. Private subnets carry
  `kubernetes.io/role/internal-elb=1`, public ones `kubernetes.io/role/elb=1`. One NAT gateway
  in the first AZ keeps the demo cheap.
- NAT gateway: `readyWhen` `status.state == available`, so kro creates the private route table,
  and after it the private subnets and the cluster, only once the gateway is available. EC2
  refuses a route to a NAT gateway it does not know yet, and ACK keeps no id for a route table
  whose route failed, so a route table created before then would be left behind on every retry. The
  cluster therefore starts when the NAT gateway is available, not right after it is created.
- Cluster: EKS Auto Mode with **no built-in node pools** (`computeConfig.nodePools: []`,
  lead decision D8). The built-in pools use the managed `default` NodeClass, which cannot carry
  custom tags; `gitops/app/nodes/` (item C) ships a tagged NodeClass with the cell's node role
  (`<cell>-node`, for example `encabulator-use2-node`, set per cell by the ApplicationSet) and a
  NodePool. Because of that the RGD creates the node role, its
  `EC2` access entry with `AmazonEKSAutoNodePolicy` (EKS only creates one for the built-in
  pools), and gives the cluster role the custom-tags inline policy from the EKS Auto Mode
  cluster IAM role guide (with the cluster name written in place of the principal-tag variable).
  `bootstrapClusterCreatorAdminPermissions: false`: the creator is the hub's ACK role, which
  needs no Kubernetes access on the cell.
- No account id in Git. Every ARN is read from ACK status
  (`status.ackResourceMetadata.arn`). The two ARNs the RGD has to build (the hub's Argo CD role
  and the optional admin role) take the account from
  `cluster.status.ackResourceMetadata.ownerAccountID`.
- Capability access entries are created **before** the capabilities, so they can carry what
  EKS does not add by itself: the kro entry has Kubernetes group `encabulator:kro` (lead
  decision D4, bound by `gitops/app/rbac/`), the ACK entry adds secret read in `encabulator`
  (the rds controller reads the database password Secret; EKS capability security guide). Each
  entry also lists the capability's default access policy, so ACK never disassociates it.
  kro's own `examples/aws/eks-cluster-mgmt` pre-creates the capability access entries the same
  way (with Terraform).
- Registration is one more resource: the Argo CD cluster Secret in `argocd` with
  `server` = the cluster ARN (the EKS Argo CD capability addresses clusters by ARN), label
  `encabulator.io/cell: "true"`, project `encabulator-cells`, and annotations:

  | Annotation | Value | Used by |
  |---|---|---|
  | `encabulator.io/region` | `spec.region` | Application label |
  | `encabulator.io/subnet-ids` | private subnet ids, comma-separated | `Encabulator.spec.subnetIDs`, NodeClass subnets |
  | `encabulator.io/security-group-id` | `encabulator-db` group id | `Encabulator.spec.securityGroupID` |
  | `encabulator.io/cluster-security-group-id` | EKS cluster security group | NodeClass security group |
  | `encabulator.io/registry` (optional) | an image mirror, for example `<account>.dkr.ecr.<region>.amazonaws.com/encabulator` | `Encabulator.spec.registry`; absent means the RGD default `ghcr.io/jlbutler/cloud-native-encabulator` |
  | `encabulator.io/repo-url`, `encabulator.io/revision` (optional) | the repository and revision of the cell's Applications | `source.repoURL` and `targetRevision` of every Application the two ApplicationSets render; absent means this repository at `carrlos` |
  | `encabulator.io/ack-capability`, `kro-capability`, `argocd-access-entry` | ARNs | ordering: the Secret waits for them |

  and labels `argocd.argoproj.io/secret-type: cluster`, `encabulator.io/cell: "true"` and
  `encabulator.io/platform-observability`, which is `spec.observability` as `"true"` or
  `"false"`. The `encabulator-platform` ApplicationSet selects on it.

### The hub's settings

The three optional annotations, `encabulator.io/repo-url`, `encabulator.io/revision` and
`encabulator.io/registry`, come from the hub. `register-hub.sh` writes them on the hub's own
cluster Secret (`argocd/encabulator-usw2`) when `GIT_REPO_URL`, `GIT_REVISION` or `REGISTRY` is
set ([`../bootstrap/README.md`](../bootstrap/README.md#deploy-into-your-own-account)). The RGD
reads that Secret as an external reference (`id: hub`; kro reads it and never changes it) and
merges those three keys, when present, into the register Secret's annotations:

```yaml
annotations: >-
  ${ { "encabulator.io/region": schema.spec.region, ... }.merge(
  hub.metadata.?annotations.orValue({}).transformMap(k, v,
  k in ["encabulator.io/repo-url", "encabulator.io/revision", "encabulator.io/registry"], v)) }
```

The whole map is one expression so that a key the hub does not carry is not written at all: a
template entry per key would write it even when empty. A hub registered without the settings
therefore yields exactly the annotations a cell always had, and a cell added by committing a file
to `gitops/hub/cells/` gets the hub's settings with no extra step. kro applies the Secret with
server-side apply and owns only the keys it writes. An `encabulator.io/registry` added to a cell's
Secret by hand stays as long as the hub has none; once the hub has one, kro takes the key over,
and if the hub's is removed later, the key goes too.

kro waits for the hub's Secret: until it exists, a new cell's AWS resources are built but its
register Secret is not written, so Argo CD deploys nothing there, and the instance stays
`IN_PROGRESS` with `waiting for external reference "hub": not found`. An existing cell whose hub
Secret disappears keeps every object, including its register Secret, so the ApplicationSets keep
rendering its Applications as before; only its instance shows `IN_PROGRESS` with the same message. Both go back to
`ACTIVE` once the hub Secret is back. kro's existing grants already cover the read (`get` on
Secrets in `argocd`, `list` and `watch` on Secrets cluster-wide).

### The ApplicationSet

Cluster generator on `encabulator.io/cell=true`, `goTemplate: true` with
`missingkey=error` (the Argo CD recommendation; a missing template parameter fails the render
instead of rendering an empty value). It renders Application `{{ .name }}` (= `encabulator-use2`) in
project `encabulator-cells`, source `gitops/app` from the repository and revision on the cell's
Secret (`encabulator.io/repo-url`, `encabulator.io/revision`; by default this repository at
`carrlos`), destination the cluster ARN,
namespace `encabulator`, automated prune + self-heal, `CreateNamespace=true`, and retries while a
new cell's CRDs settle. Two kustomize patches (JSON 6902, `op: add`) carry the annotations into
`gitops/app` (lead decision D7: `gitops/app/kustomization.yaml` is the root):

- target `Encabulator/encabulator` in `encabulator`: `spec.subnetIDs` (the annotation split on
  `,`) and `spec.securityGroupID`, plus `spec.registry` only when the Secret has
  `encabulator.io/registry` (`missingkey=error` fails on a missing template parameter such as
  `.name`; `index` on a missing annotation returns empty, which `{{ with }}` skips);
- target any `NodeClass` (`eks.amazonaws.com`): `spec.role` (`replace`, the cell's node role
  `<cell>-node`, which the RGD creates with its EC2 access entry), `spec.subnetSelectorTerms`
  (one `id` per private subnet) and `spec.securityGroupSelectorTerms` (the cluster security
  group), matching the hub's NodeClass. If `gitops/app` has no NodeClass, this patch matches
  nothing and is a no-op.

`preserveResourcesOnDeletion: true`: deleting the ApplicationSet does not delete a running region.

### The platform ApplicationSet

`encabulator-platform`, in the same file, gives every cell the observability platform. A matrix
generator pairs a list of platform entries with the cluster Secrets: each entry names its
component, Helm release, namespace, chart repository, chart and version, and the cluster
generator's selector is rendered from the entry, so it matches only cells whose
`encabulator.io/platform-<component>` label is `"true"`. With `spec.observability` left at its
default, a cell gets four Applications, named after the cell:

| Application | Source | Into `encabulator-observability` |
|---|---|---|
| `<cell>-prometheus-stack` | kube-prometheus-stack 91.8.2 | Prometheus Operator and its CRDs, Prometheus, Alertmanager, Grafana, the exporters |
| `<cell>-jaeger` | jaeger 4.14.0 | Jaeger, traces in memory |
| `<cell>-opentelemetry-collector` | opentelemetry-collector 0.174.0 | the collector the services send OTLP to |
| `<cell>-observability-config` | `platform/` in this repository (kustomize) | the three Grafana dashboards and the ServiceMonitor for the fifteen services |

The charts take `platform/<chart>.values.yaml`, the hub's values, then `platform/cell/<chart>.values.yaml`
(through a second source with `ref: values`). Versions match `platform/versions.env`, which
`scripts/platform.sh template-cell` checks in CI. Release names and namespace are the hub's, so
Prometheus, Grafana and Jaeger have the same Service names on both and the console needs no
per-cell setting.

These Applications have no dependency on the app's and the app has none on them, so a new cell's
first order is not held back by the platform. Ordering inside the platform comes from Argo CD:
kube-prometheus-stack carries the Prometheus Operator CRDs in its own sync, and the other
Applications' ServiceMonitors skip the dry run (`SkipDryRunOnMissingResource=true`) and retry
until those CRDs exist. All four use server-side apply (the CRDs are too large for client-side
apply), automated sync with self-heal, and no automated prune, so a chart bump that drops a CRD
never deletes it or what uses it.

The switch is one field in the cell file, `true` by default, so `cells/use2.yaml` does not
need it:

```yaml
spec:
  observability: false   # build this cell without the observability platform
```

A cell with `observability: false` gets none of the four. Switching it off on a running cell
stops managing the platform and leaves it running (`preserveResourcesOnDeletion: true`).

To add a component, KEDA for example: add its entry to the list (component `keda`, its release,
namespace, chart and pin), a `keda` field with its default in this RGD's schema, the
`encabulator.io/platform-keda` label on the register Secret, a `platform/cell/keda.values.yaml`,
and its namespace in the `encabulator-cells` AppProject.

### RBAC on the hub (`gitops/hub/rbac/kro-cell.yaml`)

The hub kro capability's `AmazonEKSKROPolicy` covers RGDs, instances, CRDs, leases and events.
For the kinds this RGD creates, the file grants group `encabulator:kro`:

- `list`, `watch` cluster-wide on the 11 ACK kinds and Secrets. kro watches every managed kind
  with a metadata informer across all namespaces (kro v0.9.4 `pkg/watch/manager.go`,
  `metav1.NamespaceAll`), so namespace-scoped list/watch would break its watches;
- `get`, `create`, `update`, `patch`, `delete` on the ACK kinds in `encabulator-cells` only;
- the same verbs on Secrets in `argocd` only (names are cell names, so `resourceNames` cannot
  narrow it).

Group, not ARN: the capability's own RBAC subjects (`User arn:aws:sts::<account>:assumed-role/<role>/KRO`,
`Group eks-access-entry:<role ARN>`) contain the account id. The hub kro access entry carries
`kubernetes_groups = ["encabulator:kro"]` (set by `terraform/`, lead decision D4).

### Permissions on the cell

| Principal | Access entry | Why |
|---|---|---|
| `encabulator-use2-node` | `EC2`, `AmazonEKSAutoNodePolicy` | nodes of the custom NodeClass join |
| `encabulator-use2-ack` | `AmazonEKSACKPolicy` + `AmazonEKSSecretReaderPolicy` (ns `encabulator`) | RDS + ElastiCache from the app; IAM: `AmazonRDSFullAccess`, `AmazonElastiCacheFullAccess` |
| `encabulator-use2-kro` | `AmazonEKSKROPolicy`, group `encabulator:kro` | the app's RGDs; RBAC from `gitops/app/rbac/` |
| `encabulator-usw2-argocd` (hub) | `AmazonEKSClusterAdminPolicy` | deploys cluster-scoped objects (RGDs, ClusterRoles, NodeClass). **Demo setting**: production binds a group to narrower RBAC |
| `spec.adminRoleName` (optional) | `AmazonEKSClusterAdminPolicy` | `kubectl --context use2` for a presenter |

The ACK role uses AWS managed policies to keep the demo short. Production scopes it with IAM
role selectors (EKS ACK permissions guide).

### What the hub needs (owned by other items)

- The hub's own Argo CD cluster Secret `argocd/encabulator-usw2` (`register-hub.sh`): a cell
  registers with Argo CD only once it exists.
- Hub kro capability access entry with group `encabulator:kro` (`terraform/`).
- Hub Argo CD capability role named `encabulator-usw2-argocd` (`spec.argoCDRoleName`).
- AppProject `encabulator-cells` in `argocd` (`gitops/hub/bootstrap/`), which also allows the
  platform charts' repositories, namespace `encabulator-observability` and the cluster-scoped
  kinds kube-prometheus-stack ships (CRDs, admission webhooks).
- Hub ACK capability role (`encabulator-usw2-ack`) IAM permissions, in every cell region as well
  as us-west-2 (lead decision D5). `terraform/brownfield` scopes them with three lists: the
  regions (`var.ack_regions`), the cells' IAM role prefixes (`var.ack_iam_role_prefixes`) and
  the cells' cluster names (`var.ack_eks_cluster_names`). **A new cell name or region needs all
  three updated and applied on the hub before its file is committed**; otherwise ACK is denied
  at the first VPC and IAM call. The tagger reads the same lists.
  - EC2: VPC, Subnet, InternetGateway, ElasticIPAddress, NATGateway, RouteTable and
    SecurityGroup create/describe/modify/delete plus `ec2:CreateTags`/`DeleteTags`
    (the ec2-controller's recommended policy is `AmazonEC2FullAccess`);
  - IAM on `arn:aws:iam::*:role/<prefix>*` for each cell prefix (`encabulator-use2-` and so on):
    `CreateRole`, `GetRole`, `UpdateRole`,
    `DeleteRole`, `UpdateAssumeRolePolicy`, `TagRole`, `UntagRole`, `ListRoleTags`,
    `AttachRolePolicy`, `DetachRolePolicy`, `ListAttachedRolePolicies`, `PutRolePolicy`,
    `GetRolePolicy`, `DeleteRolePolicy`, `ListRolePolicies` (from the iam-controller's
    recommended inline policy);
  - EKS: the eks-controller's recommended inline policy (`eks:*`, `iam:GetRole`,
    `iam:ListAttachedRolePolicies`, `ec2:DescribeSubnets`) on the listed cell clusters, and
    `iam:PassRole` on the cell prefixes (the cluster role is passed to EKS, the capability roles
    to `CreateCapability`).

## Pinned versions

| What | Version | Where from |
|---|---|---|
| kro | 0.9.4 (chart `oci://registry.k8s.io/kro/charts/kro`, digest `sha256:4e8de4d34b7e2c3cede958e140e26cd6d34a377062e63caf355a636d2bf4a67c`, image `registry.k8s.io/kro/kro:v0.9.4`) | local validation; the EKS kro capability runs the version AWS manages |
| ACK ec2-controller CRDs | v1.22.0 (`helm/crds/`: vpcs, subnets, internetgateways, natgateways, elasticipaddresses, routetables, securitygroups) | github.com/aws-controllers-k8s/ec2-controller |
| ACK iam-controller CRDs | v1.10.0 (`roles`) | github.com/aws-controllers-k8s/iam-controller |
| ACK eks-controller CRDs | v1.24.1 (`clusters`, `capabilities`, `accessentries`) | github.com/aws-controllers-k8s/eks-controller |
| Argo CD CRDs | v3.5.5 (`manifests/crds/`); applicationset-controller `quay.io/argoproj/argocd:v3.5.5` for the render check | github.com/argoproj/argo-cd |
| EKS Kubernetes version (cell) | 1.37 (`spec.kubernetesVersion` default, same as the hub) | EKS Kubernetes versions page |
| kind | v0.33.0, node `kindest/node:v1.35.8@sha256:07b2536e30b803ed61d1677a79df6115f798ce64c80f9e22f6ed45afd09323c0` | local validation only |
| kubectl | v1.35.0 (kustomize v5.7.1) | local validation only |

The EKS ACK capability runs AWS-managed controller versions ("All GA controllers and
resources from upstream ACK are supported", EKS ACK comparison page). Every field the RGD uses
was checked against the CRDs above; re-check after an ACK release that changes them.

## Act 3 in the IDE

From the deck (slide "In the IDE: replicate"), with the namespace the cell lives in:

```sh
git add gitops/hub/cells/use2.yaml
git commit -m "Add the us-east-2 cell"; git push
kubectl get encabulatorcells -n encabulator-cells -w
kubectl get secrets -n argocd -l encabulator.io/cell=true
kubectl get applications -n argocd
kubectl --context use2 get encabulator,dbinstances,serverlesscaches -n encabulator
```

`--context use2` needs a kubeconfig entry and an access entry on the cell for your role:
set `spec.adminRoleName` on the cell, then
`aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2`.

Useful while it builds: `kubectl -n encabulator-cells get clusters.eks.services.k8s.aws,capabilities,accessentries`
and `kubectl -n encabulator-cells get encabulatorcell encabulator-use2 -o jsonpath='{.status}'`.

## How long the waits are

Only what AWS and Argo CD document; where they give no number, none is given here.

| Step | Documented wait | Source |
|---|---|---|
| Argo CD sees the commit | polls Git every 3 minutes (120 s + up to 60 s jitter) unless a webhook or refresh triggers it | [Argo CD FAQ](https://argo-cd.readthedocs.io/en/stable/faq/) |
| NAT gateway | starts `Pending`, usable once `Available`; no duration given | [VPC: work with NAT gateways](https://docs.aws.amazon.com/vpc/latest/userguide/nat-gateway-working-with.html) |
| EKS Auto Mode cluster | "Cluster creation typically takes about 15 minutes"; elsewhere "several minutes" | [EKS Auto Mode with the AWS CLI](https://docs.aws.amazon.com/eks/latest/userguide/automode-get-started-cli.html), [Create an Auto Mode cluster](https://docs.aws.amazon.com/eks/latest/userguide/create-cluster-auto.html) |
| ACK and kro capabilities | "The command returns immediately, but the capability takes some time to become active"; no duration given | [Create an ACK capability (CLI)](https://docs.aws.amazon.com/eks/latest/userguide/ack-create-cli.html), [Create a kro capability (CLI)](https://docs.aws.amazon.com/eks/latest/userguide/kro-create-cli.html) |
| IAM roles, access entries, security group, cluster Secret | no wait documented | |
| RDS + ElastiCache on the cell | item C (`gitops/app`) | |

The cluster is the long pole, which is why the talk bakes a cell beforehand.

## Validate locally (no AWS)

On a kind cluster with kro 0.9.4, the ACK CRDs above and the Argo CD CRDs, and no ACK or
Argo CD controllers:

1. `kubectl apply -f gitops/hub/rbac/kro-cell.yaml -f gitops/hub/rgds/encabulator-cell.yaml`;
   the RGD reaches `Active`, which proves every CEL expression type-checks against the CRDs.
2. `kubectl apply -f gitops/hub/cells/use2.yaml`, then patch fake ACK status onto each resource
   as it appears (ids, ARNs with the placeholder account `111122223333`, `ACTIVE`,
   `ACK.ResourceSynced`); kro walks the graph and writes `argocd/encabulator-use2`.
3. `kubectl apply --dry-run=server -f gitops/hub/appset.yaml`.

`kubectl auth can-i --as=test --as-group=encabulator:kro ...` checks the RBAC.

## Not verified offline

- Creating a capability when its access entry already exists (the kro example does it; not
  run here against EKS).
- ACK reconciliation against AWS: IAM policy document normalization, EC2 and EKS late
  initialization, deletion order.
- That the hub's tagger CronJob (`terraform/brownfield/tagger.tf`) tags what EC2 and EKS create in
  the cell without a tag request: the VPC's main route table, default security group and default
  network ACL, the EKS cluster security group and the requester-managed network interfaces. The
  RGD cannot tag them (ACK only tags what it owns); the tagger finds the cell's VPC by its
  `project` and `talk` tags in every region of `var.ack_regions`. See `docs/TAGGING.md`.
