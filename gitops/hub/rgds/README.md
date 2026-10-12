# Act 3: the hub creates a new region

`EncabulatorCell` is a kro API on the hub cluster `encabulator-usw2`. One instance is one
region: kro turns it into ACK resources, ACK on the hub creates them in the region named on
the instance, and the last resource in the graph registers the new cluster with the hub's
Argo CD. The `encabulator` ApplicationSet then syncs `gitops/app` to it, and ACK on the cell
creates the app's RDS instance and ElastiCache cache. No Helm and no Terraform for the cell.

| File | What it is |
|---|---|
| `gitops/hub/rgds/encabulator-cell.yaml` | RGD `encabulator-cell`, kind `EncabulatorCell` |
| `gitops/hub/cells/use2.yaml` | the us-east-2 cell (`encabulator-use2`), the whole difference between one region and two |
| `gitops/hub/appset.yaml` | ApplicationSet `encabulator`: one Application per registered cell |
| `gitops/hub/rbac/kro-cell.yaml` | namespace `encabulator-cells` and the RBAC kro on the hub needs for this RGD |

## Design

### The graph

kro orders the resources from their references (the RGD's `status.topologicalOrder`):

```
VPC ─┬─ InternetGateway ── public RouteTable ── public Subnet × AZ ─┐
     │                                ElasticIPAddress ── NATGateway ┘─ private RouteTable ── private Subnet × AZ
     │
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
```

- Every ACK resource carries `services.k8s.aws/region: ${schema.spec.region}` and the three
  contract tags (`auto-delete=no`, `project=encabulator`, `talk=opn315`). EC2 resources also get
  a `Name` tag. EC2 and IAM tags are `[{key, value}]` lists; EKS tags are maps.
- Subnets: `vpcCIDR` must be a /16 (schema pattern). Private subnets are /19s at `x.y.0.0`,
  `x.y.32.0`, `x.y.64.0`; public subnets are /24s at `x.y.200.0`, `x.y.201.0`, `x.y.202.0`,
  the same split the hub's Terraform uses. Private subnets carry
  `kubernetes.io/role/internal-elb=1`, public ones `kubernetes.io/role/elb=1`. One NAT gateway
  in the first AZ keeps the demo cheap.
- Cluster: EKS Auto Mode with **no built-in node pools** (`computeConfig.nodePools: []`,
  lead decision D8). The built-in pools use the managed `default` NodeClass, which cannot carry
  custom tags; `gitops/app/nodes/` (item C) ships a tagged NodeClass with role
  `encabulator-use2-node` and a NodePool. Because of that the RGD creates the node role, its
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
  | `encabulator.io/ack-capability`, `kro-capability`, `argocd-access-entry` | ARNs | ordering: the Secret waits for them |

### The ApplicationSet

Cluster generator on `encabulator.io/cell=true`, `goTemplate: true` with
`missingkey=error` (the Argo CD recommendation; a missing annotation fails the render instead of
rendering an empty value). It renders Application `{{ .name }}` (= `encabulator-use2`) in
project `encabulator-cells`, source `gitops/app` at `carrlos`, destination the cluster ARN,
namespace `encabulator`, automated prune + self-heal, `CreateNamespace=true`, and retries while a
new cell's CRDs settle. Two kustomize patches (JSON 6902, `op: add`) carry the annotations into
`gitops/app` (lead decision D7: `gitops/app/kustomization.yaml` is the root):

- target `Encabulator/encabulator` in `encabulator`: `spec.subnetIDs` (the annotation split on
  `,`) and `spec.securityGroupID`;
- target any `NodeClass` (`eks.amazonaws.com`): `spec.subnetSelectorTerms` (one `id` per private
  subnet) and `spec.securityGroupSelectorTerms` (the cluster security group), matching the
  hub's NodeClass. If `gitops/app` has no NodeClass, this patch matches nothing and is a no-op.

`preserveResourcesOnDeletion: true`: deleting the ApplicationSet does not delete a running region.

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

- Hub kro capability access entry with group `encabulator:kro` (`terraform/`).
- Hub Argo CD capability role named `encabulator-usw2-argocd` (`spec.argoCDRoleName`).
- AppProject `encabulator-cells` in `argocd` (`gitops/hub/bootstrap/`).
- Hub ACK capability role (`encabulator-usw2-ack`) IAM permissions, in us-east-2 as well as
  us-west-2 (lead decision D5):
  - EC2: VPC, Subnet, InternetGateway, ElasticIPAddress, NATGateway, RouteTable and
    SecurityGroup create/describe/modify/delete plus `ec2:CreateTags`/`DeleteTags`
    (the ec2-controller's recommended policy is `AmazonEC2FullAccess`);
  - IAM on `arn:aws:iam::*:role/encabulator-use2-*`: `CreateRole`, `GetRole`, `UpdateRole`,
    `DeleteRole`, `UpdateAssumeRolePolicy`, `TagRole`, `UntagRole`, `ListRoleTags`,
    `AttachRolePolicy`, `DetachRolePolicy`, `ListAttachedRolePolicies`, `PutRolePolicy`,
    `GetRolePolicy`, `DeleteRolePolicy`, `ListRolePolicies` (from the iam-controller's
    recommended inline policy);
  - EKS: the eks-controller's recommended inline policy (`eks:*`, `iam:GetRole`,
    `iam:ListAttachedRolePolicies`, `ec2:DescribeSubnets`) and `iam:PassRole` on
    `encabulator-use2-*` (the cluster role is passed to EKS, the capability roles to
    `CreateCapability`).

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
- Resources EKS or AWS create implicitly carry no contract tags: the EKS cluster security
  group, the VPC's main route table, default security group and default network ACL. ACK
  cannot tag what it does not own; tag them after creation if the account requires it.
