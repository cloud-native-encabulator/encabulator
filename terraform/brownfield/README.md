# terraform/brownfield: the Act 1 starting point

What a team built with Terraform before the OPN315 talk, in `us-west-2`:

| Piece | Name | File |
|---|---|---|
| VPC, 3 AZs, private + public subnets, one NAT gateway | `encabulator-usw2` | `network.tf` |
| Hub EKS Auto Mode cluster, the applying principal as cluster admin (explicit access entry) | `encabulator-usw2` | `eks.tf` |
| Tagged Auto Mode NodeClass + NodePool (built-in pools off) | `encabulator` | `nodes.tf`, `charts/auto-mode-nodes/` |
| RDS for PostgreSQL 18, DB subnet group | `encabulator-pg`, `encabulator` | `stores.tf` |
| ElastiCache serverless Valkey 8 | `encabulator-valkey` | `stores.tf` |
| Store security group (5432, 6379 from the cluster SG) | `encabulator-db` | `network.tf` |
| EKS capabilities Argo CD, ACK, kro + capability roles | `argocd`, `ack`, `kro`; roles `encabulator-usw2-argocd/-ack/-kro` | `capabilities.tf` |
| Namespace, ConfigMap, Secrets the Helm chart reads | `encabulator`, `encabulator-aws`, `encabulator-db`, `encabulator-valkey` | `kubernetes.tf` |
| Job that creates the 3 logical databases + roles | `encabulator-db-init` | `kubernetes.tf`, `sql/init-databases.sql` |
| Tagger CronJob: tags what AWS creates implicitly, hub and cells | `encabulator-tagger`, role `encabulator-usw2-tagger` | `tagger.tf`, `tagger/sweep.py` |
| The Act 1 hand-off, shipped disabled | `removed {}` blocks | `handoff.tf.example` |

Every AWS resource carries `auto-delete=no`, `project=encabulator`, `talk=opn315` (lead decision
D10): provider `default_tags` for what Terraform creates (including the VPC's main route table,
default security group and default network ACL, which the VPC module adopts, and every EKS access
entry, which Terraform creates instead of EKS), an `aws_ec2_tag` for the EKS-created cluster
security group, the NodeClass `tags` for Auto Mode instances, root volumes and ENIs, the
StorageClass `tagSpecification_N` in `charts/encabulator/values-brownfield.yaml` for PVC volumes,
`copy_tags_to_snapshot` for RDS backups, and the tagger CronJob for what AWS creates without a tag
request (requester-managed network interfaces, the VPC endpoints ElastiCache Serverless creates,
ElastiCache snapshots, Auto Mode instance profiles, the access entry EKS creates for its
service-linked role, and the same defaults in every cell).
`docs/TAGGING.md` has the full list.

## Prerequisites

- AWS credentials for the target account with permission to create IAM roles, EKS, RDS, ElastiCache
  and VPC resources. Whoever runs `terraform apply` becomes the cluster admin.
- An IAM Identity Center instance (the Argo CD capability signs users in with it).
- Terraform 1.14.9 (any 1.7+ works; `removed { lifecycle { destroy = false } }` needs 1.7).
  On this repo's dev hosts: `mise exec terraform@1.14.9 -- terraform ...`.
- AWS CLI v2 (the Kubernetes and Helm providers call `aws eks get-token`), kubectl, Helm 4.

## Command flow

From the repository root, `make hub` runs all of this from one `.env` file (see the
[README quick start](../../README.md#on-aws-from-terraform-to-gitops)): it passes
`IDC_INSTANCE_ARN`, `IDC_REGION` and `ARGOCD_ADMIN_GROUP_ID` to Terraform as `TF_VAR_` variables,
so no file here needs editing. By hand:

```bash
cd terraform/brownfield
cp terraform.tfvars.example encabulator.auto.tfvars   # gitignored; fill in the Identity Center ARN
terraform init
terraform apply                                        # about 25 minutes, RDS is the long pole
$(terraform output -raw kubeconfig_command)
make -C ../.. platform && ./deploy-app.sh              # observability, then the app on RDS + ElastiCache
```

Who may sign in to Argo CD comes from `argocd_rbac_role_mappings` (a list of roles and Identity
Center users or groups) or, for the common case of one admin group, from `argocd_admin_group_id`
alone, which gives that group the `ADMIN` role. When the list is set it wins. An
`encabulator.auto.tfvars` takes precedence over `TF_VAR_` values, so a stack that has one keeps
planning exactly as it did.

`deploy-app.sh` reads the store endpoints from the ConfigMap `encabulator-aws` and runs `make deploy`
with `values-eks-auto-mode.yaml`, `values-brownfield.yaml` and `--set postgres.external.host=...
--set valkey.external.host=...` (`DRY_RUN=1` prints the command).

State is local (`terraform.tfstate`, gitignored) and holds the generated passwords. Use a remote
backend if more than one person runs the stack.

## Act 1 hand-off

After ACK has adopted the three stores (`gitops/hub/brownfield`, each `ACK.ResourceSynced=True`):

```bash
git rm stores.tf
git mv handoff.tf.example handoff.tf
terraform plan    # aws_db_instance.pg, aws_elasticache_serverless_cache.valkey and
                  # aws_db_subnet_group.encabulator "will no longer be managed by Terraform"
terraform apply
terraform plan    # No changes
```

`destroy = false` removes them from state and leaves them running. The VPC and the security group
stay with Terraform. The ConfigMap is built from `data.aws_db_instance` and
`data.aws_elasticache_serverless_cache`, looked up by name, so it does not change when the managed
resources leave the configuration. `handoff.tf.example` also swaps the two lookup locals from
`stores.tf`: before the hand-off they come from the resources' ARNs (unknown until the stores exist,
so the first apply reads the data sources after creating the stores); after it they are the fixed
names.

ACK compares what it adopts with its own resources, so the store values here and in
`gitops/hub/brownfield` must agree (contract D3): engine major 18, `db.t4g.medium`, 20 GiB gp3,
encrypted, private, single-AZ, 7-day backups, no `max_allocated_storage`, no log exports, no
Performance Insights, exactly the three tags (no `Name`). ServerlessCache: description
`Encabulator Valkey cache`, `dailySnapshotTime` `09:00`, `snapshotRetentionLimit` 1. DB subnet group
description `Encabulator databases`. The cache uses the DB subnet group's subnets and
`encabulator-db` as its only security group, which `gitops/hub/bootstrap/register-hub.sh` checks.
`copy_tags_to_snapshot` and `auto_minor_version_upgrade` are true here; the rds controller
late-initializes both, so the ACK resource does not need to list them.

## How the pieces work

**Databases and roles.** RDS is private, so the SQL runs inside the cluster as the Kubernetes Job
`encabulator-db-init` (image `postgres:18.6-alpine3.24`, pinned by digest). It connects as the
master user over TLS with the endpoint from the ConfigMap and the passwords from the Secret, and
runs `sql/init-databases.sql`: create each role and database if missing, reset each role's password
to the Secret's value, hand each database's `public` schema to its owner. The RDS master user is not
a superuser, so the script grants itself each role first (PostgreSQL 16+ only lets it create a
database owned by a role it can `SET ROLE` to). Re-running is safe; Terraform replaces the Job when
the script changes. A Job was picked over a `postgresql` provider (needs network access to the
private endpoint from wherever Terraform runs) and over a Lambda (more IAM, VPC config and
packaging for three statements).

**Valkey without a password.** ElastiCache serverless with no user group is protected by its
security group only. The chart always reads `VALKEY_PASSWORD` from a Secret, so Terraform writes
`encabulator-valkey` with an empty `valkey-password`, and `values-brownfield.yaml` sets an empty
`valkey.user`. The rendered URL is `rediss://:@<host>:6379/0`, so no client sends `AUTH`.

**Auto Mode nodes.** The built-in node pools use the EKS-managed `default` NodeClass, which EKS
reverts if edited, so their instances can never be tagged. The cluster has Auto Mode on with
`node_pools = []`; `nodes.tf` installs a NodeClass `encabulator` with the three tags and a NodePool
`encabulator` shaped like the built-in general-purpose pool (C/M/R, generation 5+, amd64,
on-demand), plus the EC2 access entry with `AmazonEKSAutoNodePolicy` that a custom NodeClass needs.
The cluster role gets the custom-tags policy from the EKS Auto Mode guide. The manifests are a
local Helm chart (`charts/auto-mode-nodes`) because `kubernetes_manifest` needs the CRDs at plan
time, before the cluster exists. Rendered, for copying into other clusters:

```yaml
apiVersion: eks.amazonaws.com/v1
kind: NodeClass
metadata:
  name: encabulator
spec:
  role: "encabulator-usw2-node"
  subnetSelectorTerms:
    - id: "subnet-0aaaaaaaaaaaaaaa1"   # one per private subnet
  securityGroupSelectorTerms:
    - id: "sg-0bbbbbbbbbbbbbbb1"       # the cluster security group
  tags:
    auto-delete: "no"
    project: "encabulator"
    talk: "opn315"
---
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: encabulator
spec:
  template:
    spec:
      nodeClassRef: { group: eks.amazonaws.com, kind: NodeClass, name: encabulator }
      requirements:
        - { key: karpenter.sh/capacity-type, operator: In, values: ["on-demand"] }
        - { key: eks.amazonaws.com/instance-category, operator: In, values: ["c", "m", "r"] }
        - { key: eks.amazonaws.com/instance-generation, operator: Gt, values: ["4"] }
        - { key: kubernetes.io/arch, operator: In, values: ["amd64"] }
        - { key: kubernetes.io/os, operator: In, values: ["linux"] }
      expireAfter: 336h
      terminationGracePeriod: 24h
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 30s
    budgets: [{ nodes: "10%" }]
  limits:
    cpu: "64"
```

**Capabilities.** Each capability role trusts `capabilities.eks.amazonaws.com`. Terraform creates
all three capability access entries before the capabilities, so they carry the contract tags (an
entry EKS creates carries none); EKS adds each capability's default access policies to its entry.
The kro entry also carries the Kubernetes group `encabulator:kro`, which `gitops/hub/rbac` and
`gitops/app/rbac` bind (RBAC never names a role ARN). The principal running `terraform apply` gets
its own explicit cluster-admin entry (`bootstrap_cluster_creator_admin_permissions = false`; the
EKS-created creator entry would be untagged). Extra hub permissions:

| Capability | Kubernetes permissions added here | AWS permissions |
|---|---|---|
| Argo CD | `AmazonEKSClusterAdminPolicy`, cluster scope (demo only, `argocd_cluster_admin`) | none (public Git repo) |
| ACK | `AmazonEKSSecretReaderPolicy` on namespace `encabulator` | inline policy below |
| kro | group `encabulator:kro`; `AmazonEKSClusterAdminPolicy` only if `kro_cluster_admin = true` | none |

The ACK policy allows, in `us-west-2` and `us-east-2` only: `rds:*`, `elasticache:*`, the EC2 calls
for VPCs, subnets, security groups, route tables, internet and NAT gateways and Elastic IPs;
`eks:*` on the cell clusters only (`encabulator-use2` and its access entries, capabilities, add-ons);
IAM role management and `iam:PassRole` (to EKS, EKS capabilities, EC2) on `encabulator-use2-*` only;
service-linked roles for EKS, RDS and ElastiCache. The hub cluster and the hub's own roles
(`encabulator-usw2-*`) are outside it, so ACK cannot change the hub or widen its own permissions.

**The tagger.** A CronJob in namespace `encabulator-tagger`, every 10 minutes, running
`tagger/sweep.py` (Python + boto3 in the AWS Lambda Python base image, pinned by digest) as IAM role
`encabulator-usw2-tagger` through EKS Pod Identity. In every VPC tagged `project=encabulator` and
`talk=opn315` in `var.ack_regions` (the hub's and every cell's) it adds the three contract tags to
route tables, network ACLs, security groups, network interfaces and VPC endpoints that lack one
(ElastiCache Serverless creates three endpoints per VPC, tagged only
`AmazonElastiCacheManaged=true`), then to the `encabulator-valkey` snapshots, to the instance
profiles of the hub's and the cells' node roles, and to the access entries EKS creates itself
(Auto Mode's `AWSServiceRoleForAmazonEKS` entry).
The role can read those and write tags only when the request is exactly the three contract tags.
A CronJob rather than a one-time pass because requester-managed interfaces and endpoints (RDS,
ElastiCache, NAT gateways, the EKS control plane) appear whenever those services create or
replace them, and cells come later. `kubectl -n encabulator-tagger logs job/<latest>` shows what
it checked and tagged.

## Demo-only choices

- Argo CD capability is cluster admin on the hub. Scope it with namespaced access policies or RBAC.
- ACK has whole-service `rds:*` and `elasticache:*` in two regions on the capability role. Use IAM
  role selectors (one role per namespace) for real use.
- One NAT gateway, single-AZ RDS, no final snapshot, no deletion protection, local state.
- The EKS API endpoint is public (and private). Restrict `public_access_cidrs` or go private.

## Pinned versions

| What | Version |
|---|---|
| Terraform | 1.14.9 tested (`>= 1.7.0, < 2.0.0`) |
| hashicorp/aws | 6.68.0 (has `aws_eks_capability`) |
| hashicorp/kubernetes | 3.3.0 |
| hashicorp/helm | 3.3.0 |
| hashicorp/random | 3.9.1 |
| terraform-aws-modules/vpc/aws | 6.7.3 |
| EKS Kubernetes | 1.37 (`kubernetes_version`) |
| RDS for PostgreSQL | major 18 (`postgres_engine_version`; 19 is beta only) |
| ElastiCache Valkey | major 8 (`valkey_major_engine_version`, contract D2) |
| Job image | `postgres:18.6-alpine3.24@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873` |
| Tagger image | `public.ecr.aws/lambda/python:3.13@sha256:40dcef2531da40c0fae6d11ee28e12313af9723fc1f0f31c1728c22e5112ec3c` (Python 3.13.16, boto3 1.42.97) |

Provider hashes are in `.terraform.lock.hcl` (committed).

## Offline checks

```bash
terraform fmt -check -recursive
terraform init -backend=false && terraform validate
terraform test     # plan with fake AWS credentials and mocked Kubernetes/Helm; asserts names,
                   # tags, store values, hand-off keys and the ACK policy scope (brownfield.tftest.hcl)
                   # and auto-delete=no, the access entries and the tagger's limits (tags.tftest.hcl);
                   # both ways of naming the Argo CD admins give the same mapping
python3 tagger/test_sweep.py   # the tagger against a fake boto3
```

## Teardown

Cells first, then the hub. Each step needs the one before it:

1. [Remove every cell](#remove-a-cell) while the hub still runs. The hub's kro and ACK are what
   delete a cell. Once the hub is gone, nothing deletes the cell's VPC, NAT gateway, cluster and
   roles, and each one has to be found and deleted by hand.
2. [Stop Argo CD on the hub](#stop-argo-cd-on-the-hub), so it does not re-apply what you delete
   next.
3. [Remove the app, its volumes and the platform](#the-app-its-volumes-and-the-platform).
4. [Delete the hub's stores](#the-hubs-stores) and their snapshots.
5. [Delete the ECR repositories](#ecr-repositories) that `make mirror-images` created.
6. [`terraform destroy -refresh=false`](#terraform-destroy).
7. [Prove the account is clean](#prove-the-account-is-clean).

The commands run from the repository root, with the AWS CLI v2, credentials for the account and
kubectl pointed at the hub (`make kubeconfig`: context `encabulator-usw2`, namespace
`encabulator`). They name the cell `encabulator-use2` in us-east-2 (`gitops/hub/cells/use2.yaml`);
for another cell, change the name and the region. The whole teardown takes about an hour, most of
it waiting. It deletes the data: no final snapshot of any store is kept.

Before the hand-off (`stores.tf` in place, no `handoff.tf`), Terraform still owns the three
stores: do every step except [The hub's stores](#the-hubs-stores), and `terraform destroy`
deletes them with the rest, without `-refresh=false` (not exercised).

### Remove a cell

kubectl on the cell: `make cell-access CELL=use2` gives your AWS identity cluster admin on the
cell's cluster, then
`aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2`. The steps
below need no more than permission to delete StatefulSets and PersistentVolumeClaims in the
cell's namespace `encabulator`, so a narrower access entry works too.

First, look for untracked route tables. The cell graph creates the private route table only once
the NAT gateway is available ([`gitops/hub/rgds/README.md`](../../gitops/hub/rgds/README.md#the-graph)).
A cell built without that wait can hold extra `encabulator-use2-private` route tables, one per
failed attempt, with no subnet association and only the local route. ACK does not track them,
and the VPC delete fails while they exist. Compare the VPC's tables with the one the hub tracks:

```bash
vpc=$(aws ec2 describe-vpcs --region us-east-2 --filters Name=tag:Name,Values=encabulator-use2 \
  --query 'Vpcs[].VpcId' --output text)
kubectl -n encabulator-cells get routetables.ec2.services.k8s.aws encabulator-use2-private \
  -o jsonpath='{.status.routeTableID}{"\n"}'
aws ec2 describe-route-tables --region us-east-2 \
  --filters Name=vpc-id,Values="$vpc" Name=tag:Name,Values=encabulator-use2-private \
  --query 'RouteTables[].[RouteTableId,length(Associations)]' --output text
```

Delete every other table with 0 associations before you delete the cell: adopt it with
[`docs/teardown/adopt-route-table.yaml`](../../docs/teardown/adopt-route-table.yaml)
(`deletion-policy: delete`), wait for `ACK.ResourceSynced`, and delete the object.
`aws ec2 delete-route-table --region us-east-2 --route-table-id <rtb-id>` does the same
(not exercised).

Then remove the cell:

```bash
# 1. On the hub: keep the node role and the node access entry when kro deletes the cell.
kubectl -n encabulator-cells annotate roles.iam.services.k8s.aws encabulator-use2-node \
  services.k8s.aws/deletion-policy=retain
kubectl -n encabulator-cells annotate accessentries.eks.services.k8s.aws encabulator-use2-node \
  services.k8s.aws/deletion-policy=retain

# 2. Stop Argo CD from re-creating the cell, then delete it. hub-cells has no resources
#    finalizer, so deleting it leaves the EncabulatorCell (and every other cell) in place.
kubectl -n argocd delete application hub-cells
kubectl -n encabulator-cells delete encabulatorcells encabulator-use2 --wait=false

# 3. Within a minute or two kro has removed the cell's Argo CD cluster Secret (the cell's
#    Applications go, the app keeps running) and the cell's kro and ACK capabilities.
aws eks list-capabilities --region us-east-2 --cluster-name encabulator-use2   # no KRO, no ACK
kubectl -n argocd get applications                                              # no encabulator-use2*

# 4. On the cell: nothing re-creates the StatefulSets now. Delete them, then their volumes.
kubectl --context use2 -n encabulator delete statefulset nats influxdb
kubectl --context use2 -n encabulator delete pvc data-nats-0 data-influxdb-0
kubectl --context use2 get pv                    # repeat until it lists nothing

# 5. The cell's stores, once list-capabilities shows no ACK capability.
aws rds delete-db-instance --region us-east-2 --db-instance-identifier encabulator-pg \
  --skip-final-snapshot --delete-automated-backups
aws elasticache delete-serverless-cache --region us-east-2 --serverless-cache-name encabulator-valkey
aws rds wait db-instance-deleted --region us-east-2 --db-instance-identifier encabulator-pg
aws rds delete-db-subnet-group --region us-east-2 --db-subnet-group-name encabulator

# 6. Wait until the cell is gone, about 20 minutes after step 2.
kubectl -n encabulator-cells get encabulatorcells        # encabulator-use2 is no longer listed

# 7. The node role, now that the cluster is gone: adopt it into the hub's ACK with
#    deletion-policy delete, then delete the object, and ACK deletes the role.
kubectl apply -f docs/teardown/adopt-node-role.yaml
kubectl -n encabulator-cells wait --for=condition=ACK.ResourceSynced \
  roles.iam.services.k8s.aws/encabulator-use2-node
kubectl delete -f docs/teardown/adopt-node-role.yaml
```

Why each step is there:

- Node role (steps 1 and 7). kro deletes the hub's Role object before the cluster, while EKS
  Auto Mode still holds the role in the cluster's instance profile, and the hub's ACK role may
  not remove a role from an instance profile (`capabilities.tf`). Without `retain`, kro waits on
  the role and the cell is never deleted. The instance profile goes with the cluster, and step 7
  deletes the role. With the IAM CLI instead of step 7: `aws iam detach-role-policy` for its two
  policies, then `aws iam delete-role --role-name encabulator-use2-node` (not exercised).
- Node access entry (step 1). kro deletes it early. With `retain`, EKS keeps it, so the nodes
  keep their API access and the volumes in step 4 can still be detached and deleted. The entry
  goes with the cluster.
- Volumes (step 4). `encabulator-gp3` deletes a volume with its PVC, but a volume still bound
  when the cluster goes stays in EC2 as `available` and keeps costing money. While the cell's
  kro and Argo CD run, they re-create a deleted StatefulSet, and the StatefulSet re-creates a
  deleted PVC with a new volume. After step 3 neither runs. With cluster admin on the cell
  (`make cell-access` grants it; `AmazonEKSEditPolicy` does not), a ResourceQuota is the other
  way: apply it in the cell's `encabulator` namespace before step 2, then delete the PVCs and
  their pods, and the PVCs cannot come back.

  ```yaml
  apiVersion: v1
  kind: ResourceQuota
  metadata:
    name: no-pvc
    namespace: encabulator
  spec:
    hard:
      count/persistentvolumeclaims: "0"
  ```

- Stores (step 5). The cell's ACK created them with `deletion-policy: retain`, so they stay
  after the cell is removed, and they keep the cell's `encabulator-db` security group in use
  (kro waits on it, which leaves time for this step). While the cell's ACK runs,
  `adoption-policy: adopt-or-create` re-creates a deleted store, so step 5 waits for step 3.
  The cache's automatic snapshot stays: see [Snapshots](#snapshots).
- To remove one cell and keep the others, remove its file (`gitops/hub/cells/use2.yaml`) from
  the revision `hub-cells` tracks instead of deleting `hub-cells`, and Argo CD prunes the
  `EncabulatorCell` (not exercised). Do step 1 before that commit lands.

### Stop Argo CD on the hub

```bash
kubectl -n argocd delete applicationset hub-brownfield     # its Application and the three store objects go
kubectl -n argocd delete application hub-cells hub-cell-apps hub-rgds --ignore-not-found
kubectl -n argocd delete applicationset encabulator encabulator-platform
kubectl -n argocd get applications,applicationsets         # No resources found
kubectl -n encabulator get dbinstances.rds,serverlesscaches,dbsubnetgroups.rds   # none, and none a few minutes later
```

Every hub Application syncs automatically with `selfHeal`, so a store object or an
`EncabulatorCell` deleted by hand comes back within seconds. The three store objects use
`adoption-policy: adopt-or-create`: one that Argo CD re-applies after its store is deleted makes
ACK create a new, empty store
([how adoption works](../../gitops/hub/bootstrap/README.md#verified-in-the-controller-source)).
The ApplicationSet renders the Application `hub-brownfield` with the resources finalizer, so
deleting the ApplicationSet deletes the three store objects too (about 4 minutes), and
`deletion-policy: retain` keeps RDS and the cache running for [The hub's stores](#the-hubs-stores).
`hub-cell-apps` and `hub-rgds` have no finalizer: the RGD, the RBAC and the two cell
ApplicationSets stay. Delete `hub-cell-apps` before the ApplicationSets, or it re-creates them;
with no cell left they have no Applications (`preserveResourcesOnDeletion`). Delete `hub-rgds`
only once every cell is gone.

### The app, its volumes and the platform

```bash
make undeploy                                                                # the Helm release encabulator
kubectl -n encabulator delete pvc -l app.kubernetes.io/part-of=encabulator   # the selector make undeploy prints
make platform-uninstall                                                      # Prometheus, Grafana, Jaeger, the collector, KEDA
kubectl get pv                                                               # repeat until it lists nothing
kubectl get svc -A                                                           # no Service of type LoadBalancer
```

`make undeploy` and `make platform-uninstall` leave the PVCs on purpose. The hub's
`data-nats-0` (8 GiB) and `data-influxdb-0` (16 GiB) are EBS volumes: deleting the PVCs while
the cluster runs deletes them, and a volume left when the cluster goes stays in EC2. A
`LoadBalancer` Service's load balancer and its network interfaces would keep the VPC from being
deleted. The Helm release `encabulator-nodes` stays: Terraform removes it.

### The hub's stores

After the hand-off nothing owns them. In us-west-2:

```bash
aws rds delete-db-instance --region us-west-2 --db-instance-identifier encabulator-pg \
  --skip-final-snapshot --delete-automated-backups
aws elasticache delete-serverless-cache --region us-west-2 --serverless-cache-name encabulator-valkey
aws rds wait db-instance-deleted --region us-west-2 --db-instance-identifier encabulator-pg
aws rds delete-db-subnet-group --region us-west-2 --db-subnet-group-name encabulator
```

RDS refuses to delete the subnet group while the instance exists, hence the wait (about 7
minutes). `--delete-automated-backups` removes the instance's automated backups; they stay listed
for a few minutes after the instance is gone.

When your credentials may not delete RDS or ElastiCache resources, the hub's ACK can, while the
hub runs. [`docs/teardown/adopt-hub-stores.yaml`](../../docs/teardown/adopt-hub-stores.yaml)
adopts the instance and the cache with `adoption-policy: adopt` (read the existing resource,
never create one) and `deletion-policy: delete`, plus
`rds.services.k8s.aws/skip-final-snapshot: "true"` and
`rds.services.k8s.aws/delete-automated-backups: "true"` (the rds controller keeps the automated
backups unless told otherwise). Fill in the ids each file names in its header, then:

```bash
kubectl apply -f docs/teardown/adopt-hub-stores.yaml
kubectl -n encabulator wait --for=condition=ACK.ResourceSynced \
  dbinstances.rds/encabulator-pg serverlesscaches/encabulator-valkey
kubectl delete -f docs/teardown/adopt-hub-stores.yaml
aws rds wait db-instance-deleted --region us-west-2 --db-instance-identifier encabulator-pg
kubectl apply -f docs/teardown/adopt-hub-subnet-group.yaml
kubectl -n encabulator wait --for=condition=ACK.ResourceSynced dbsubnetgroups.rds/encabulator
kubectl delete -f docs/teardown/adopt-hub-subnet-group.yaml
```

#### Snapshots

Each Valkey cache takes an automatic snapshot every day at 09:00 UTC and keeps it for one day
(`snapshotRetentionLimit: 1`). The snapshot stays after the cache is deleted, until it expires.
List them in us-west-2 and in every cell's region:

```bash
aws elasticache describe-serverless-cache-snapshots --region us-west-2 \
  --query 'ServerlessCacheSnapshots[?contains(ServerlessCacheSnapshotName, `encabulator`)].ServerlessCacheSnapshotName' \
  --output text
```

Let each one expire, or delete it through the hub's ACK with
[`docs/teardown/adopt-cache-snapshot.yaml`](../../docs/teardown/adopt-cache-snapshot.yaml)
(fill in the region and the snapshot name, apply, wait for `ACK.ResourceSynced`, delete the
object), or with
`aws elasticache delete-serverless-cache-snapshot --region <region> --serverless-cache-snapshot-name <name>`
(not exercised).

### ECR repositories

`make mirror-images` creates one repository per service (`encabulator/<service>`) in us-west-2
when the release is not on GitHub Container Registry, and nothing else owns or deletes them.
`--force` deletes the images with each repository. The loop deletes every repository whose name
starts with `encabulator/`, so read the list first if the account has others:

```bash
for r in $(aws ecr describe-repositories --region us-west-2 \
    --query 'repositories[?starts_with(repositoryName, `encabulator/`)].repositoryName' --output text); do
  aws ecr delete-repository --region us-west-2 --repository-name "$r" --force
done
```

### Terraform destroy

```bash
terraform -chdir=terraform/brownfield state list               # prints nothing: already destroyed
terraform -chdir=terraform/brownfield destroy -refresh=false   # about 6 minutes
chmod 600 terraform/brownfield/terraform.tfstate*
```

- `-refresh=false`: after the hand-off the stack still reads the two stores by name
  (`data.aws_db_instance.pg` and `data.aws_elasticache_serverless_cache.valkey`, for the
  ConfigMap `encabulator-aws`). With the stores deleted, a refresh fails with
  `couldn't find resource`. The stores cannot go last instead: they use the stack's subnets and
  its `encabulator-db` security group, so the VPC delete would wait on them. With
  `-refresh=false`, Terraform plans from the state alone and deletes every resource in it.
- Terraform needs the variables the stack was built with. `make hub` passes `IDC_INSTANCE_ARN`
  from `.env` as `TF_VAR_idc_instance_arn`, so set it the same way
  (`TF_VAR_idc_instance_arn=<IDC_INSTANCE_ARN> terraform -chdir=terraform/brownfield destroy -refresh=false`);
  a stack built from `encabulator.auto.tfvars` reads it from that file.
- A `terraform state list` that prints nothing means the hub is already destroyed, and
  `terraform plan -destroy` reports `No changes`: that is the expected end state.
- The state holds the generated database and app passwords. Keep `terraform.tfstate` and
  `terraform.tfstate.backup` at mode 600, and copy them with `install -m 600`, because a
  `cp -p` copy keeps the source's mode.

### Prove the account is clean

No single command says "clean": ask each service. For us-west-2 and every cell's region, each
of these prints nothing when nothing of the project is left:

```bash
r=us-west-2
aws eks list-clusters --region $r --query 'clusters[?starts_with(@, `encabulator`)]' --output text
aws ec2 describe-vpcs --region $r --filters Name=tag:project,Values=encabulator --query 'Vpcs[].VpcId' --output text
aws ec2 describe-instances --region $r --include-managed-resources \
  --filters Name=tag:project,Values=encabulator Name=instance-state-name,Values=pending,running,stopping,stopped,shutting-down \
  --query 'Reservations[].Instances[].InstanceId' --output text
aws ec2 describe-volumes --region $r --include-managed-resources \
  --filters Name=tag:project,Values=encabulator --query 'Volumes[].VolumeId' --output text
aws ec2 describe-network-interfaces --region $r --include-managed-resources \
  --filters Name=tag:project,Values=encabulator --query 'NetworkInterfaces[].NetworkInterfaceId' --output text
aws rds describe-db-instances --region $r \
  --query 'DBInstances[?starts_with(DBInstanceIdentifier, `encabulator`)].DBInstanceIdentifier' --output text
aws rds describe-db-instance-automated-backups --region $r \
  --query 'DBInstanceAutomatedBackups[?starts_with(DBInstanceIdentifier, `encabulator`)].DBInstanceIdentifier' --output text
aws elasticache describe-serverless-caches --region $r \
  --query 'ServerlessCaches[?starts_with(ServerlessCacheName, `encabulator`)].ServerlessCacheName' --output text
aws elasticache describe-serverless-cache-snapshots --region $r \
  --query 'ServerlessCacheSnapshots[?contains(ServerlessCacheSnapshotName, `encabulator`)].ServerlessCacheSnapshotName' --output text
aws ecr describe-repositories --region $r \
  --query 'repositories[?starts_with(repositoryName, `encabulator`)].repositoryName' --output text
aws resourcegroupstaggingapi get-resources --region $r --tag-filters Key=project,Values=encabulator \
  --query 'ResourceTagMappingList[].ResourceARN' --output text
# global
aws iam list-roles --query 'Roles[?starts_with(RoleName, `encabulator`)].RoleName' --output text
aws iam list-instance-profiles \
  --query 'InstanceProfiles[?contains(InstanceProfileName, `encabulator`) || contains(Path, `encabulator`)].InstanceProfileName' \
  --output text
```

The same goes for subnets, route tables, NAT and internet gateways, Elastic IPs, VPC endpoints,
security groups, EBS snapshots, RDS subnet groups and snapshots, log groups and load balancers,
for the tagging index of every other enabled region, and for the Identity Center application of
the Argo CD capability. Read the answers this way:

- `--include-managed-resources` on the EC2 instance, volume and network interface calls: without
  it EC2 leaves out what EKS Auto Mode manages, and the check reads clean while nodes still run.
- An AWS error (expired credentials, access denied, throttling) is a failure, never "gone".
- The tagging index lags by hours: it can still list instances, network interfaces, VPC
  endpoints and snapshots that are gone. Check each ARN it lists in its own service; a not-found
  answer there means gone. An instance terminated long ago answers
  `aws ec2 describe-instances --instance-ids` with no reservation and no error: gone too.
- `fleet/` entries in the index are the EC2 instant fleets EKS Auto Mode records for each node
  launch. They stay listed, hold no instances and cost nothing; there is nothing to delete.
- `make tag-check` checks the tags of the stacks it finds. It does not prove that nothing is left.
