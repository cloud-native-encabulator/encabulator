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
request (requester-managed network interfaces, ElastiCache snapshots, Auto Mode instance profiles,
the access entry EKS creates for its service-linked role, and the same defaults in every cell).
`docs/TAGGING.md` has the full list.

## Prerequisites

- AWS credentials for the target account with permission to create IAM roles, EKS, RDS, ElastiCache
  and VPC resources. Whoever runs `terraform apply` becomes the cluster admin.
- An IAM Identity Center instance (the Argo CD capability signs users in with it).
- Terraform 1.14.9 (any 1.7+ works; `removed { lifecycle { destroy = false } }` needs 1.7).
  On this repo's dev hosts: `mise exec terraform@1.14.9 -- terraform ...`.
- AWS CLI v2 (the Kubernetes and Helm providers call `aws eks get-token`), kubectl, Helm 4.

## Command flow

```bash
cd terraform/brownfield
cp terraform.tfvars.example encabulator.auto.tfvars   # gitignored; fill in the Identity Center ARN
terraform init
terraform apply                                        # about 25 minutes, RDS is the long pole
$(terraform output -raw kubeconfig_command)
make -C ../.. platform && ./deploy-app.sh              # observability, then the app on RDS + ElastiCache
```

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
route tables, network ACLs, security groups and network interfaces that lack one, then to the
`encabulator-valkey` snapshots, to the instance profiles of the hub's and the cells' node roles, and
to the access entries EKS creates itself (Auto Mode's `AWSServiceRoleForAmazonEKS` entry).
The role can read those and write tags only when the request is exactly the three contract tags.
A CronJob rather than a one-time pass because requester-managed interfaces (RDS, ElastiCache, NAT
gateways, the EKS control plane) appear whenever those services create or replace them, and cells
come later. `kubectl -n encabulator-tagger logs job/<latest>` shows what it tagged.

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
                   # and auto-delete=no, the access entries and the tagger's limits (tags.tftest.hcl)
python3 tagger/test_sweep.py   # the tagger against a fake boto3
```

## Teardown

Before the hand-off, `terraform destroy` removes everything. After it, the three stores belong to
ACK with `deletion-policy: retain`: delete their ACK resources and then the RDS instance, cache and
subnet group yourself, then `terraform destroy` the rest.
