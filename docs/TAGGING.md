# auto-delete=no on every AWS resource

Every AWS resource the OPN315 stacks create carries `auto-delete=no` (with `project=encabulator`
and `talk=opn315`), the hub's and every cell's, whoever creates it: Terraform, ACK, kro, EKS or
the service itself. Two ways to get there:

| When | How | Covers |
|---|---|---|
| create time | Terraform provider `default_tags` (`terraform/brownfield/providers.tf`) | everything the hub Terraform creates, including the VPC's main route table, default security group and default network ACL (the VPC module adopts them, `network.tf`) and the capability and admin EKS access entries (created by Terraform, not by EKS) |
| create time | `aws_ec2_tag` (`network.tf`) | the hub's EKS cluster security group |
| create time | ACK `spec.tags` | the hub's adopted stores (`gitops/hub/brownfield`), every resource of the `EncabulatorCell` RGD and the cell's stores (`gitops/app/rgds/encabulator.yaml`) |
| create time | NodeClass `spec.tags` (hub: `terraform/brownfield/nodes.tf`, cells: `gitops/app/nodes`) | Auto Mode instances, root volumes, network interfaces, launch templates |
| create time | StorageClass `tagSpecification_1..3` (hub: `charts/encabulator/values-brownfield.yaml`, cells: the `encabulator-gp3` StorageClass in `gitops/app/rgds/encabulator.yaml`) | EBS volumes for the NATS and InfluxDB PVCs |
| create time | RDS copy tags to snapshot (hub: `copy_tags_to_snapshot`, cells: `copyTagsToSnapshot`) | automated backups |
| post-pass | the tagger CronJob on the hub, every 10 minutes (`terraform/brownfield/tagger.tf`, `tagger/sweep.py`) | what AWS creates without a tag request: a cell's main route table, default security group, default network ACL and EKS cluster security group; the requester-managed network interfaces of the EKS control plane, NAT gateways, RDS and ElastiCache in the hub and every cell; ElastiCache serverless snapshots; Auto Mode's instance profiles; the access entry EKS creates for its service-linked role on the hub and every cell |

Nothing creates load balancers (no Ingress, no Service of type `LoadBalancer`), and the cell
cluster has no creator access entry (`bootstrapClusterCreatorAdminPermissions: false`).

## The tagger

A CronJob `encabulator-tagger` in namespace `encabulator-tagger` on the hub, installed by
`terraform/brownfield`. Every 10 minutes it finds the VPCs tagged `project=encabulator` and
`talk=opn315` in us-west-2 and us-east-2 (`var.ack_regions`) and adds the three tags to every route
table, network ACL, security group and network interface in them that lacks one, then to the
`encabulator-valkey` snapshots, the instance profiles of `encabulator-usw2-node` and
`encabulator-use2-node`, and the access entries EKS creates itself on `encabulator-usw2` and
`encabulator-use2` (Auto Mode adds one, untagged, for the `AWSServiceRoleForAmazonEKS`
service-linked role). It only ever adds those three tags.

- Identity: IAM role `encabulator-usw2-tagger` through EKS Pod Identity, trusted only for its own
  service account on the hub. It may describe, and it may write tags only when the request is
  exactly `auto-delete=no`, `project=encabulator`, `talk=opn315` (`aws:TagKeys` and
  `aws:RequestTag` conditions); every other tag key or value is denied.
- Why on the hub and not per cell: the hub is where the cells come from, and the interfaces it
  has to catch keep appearing (RDS and ElastiCache on a cell only after Argo CD syncs the app,
  and EKS replaces control-plane interfaces over the cluster's life), so a one-time pass would
  miss them. ACK cannot give hub pods an AWS identity (the hub is outside its permissions), so
  Terraform creates the role and the Pod Identity association.
- Watch it: `kubectl -n encabulator-tagger get cronjob,jobs` and
  `kubectl -n encabulator-tagger logs job/<latest>` (one JSON line per resource tagged, then a
  summary). `DRY_RUN=1` in the container env prints instead of tagging.
- Tests: `python3 terraform/brownfield/tagger/test_sweep.py` (offline, fake boto3).

## Checking a live account

```sh
scripts/tag-check.py                                   # hub us-west-2:encabulator-usw2 and cell us-east-2:encabulator-use2
scripts/tag-check.py --stack us-east-2:encabulator-use2
scripts/tag-check.py --record tmp/tag-check-live.json  # also save the responses (account ids inside: keep out of Git)
```

Read-only AWS CLI calls (describe and list) with whatever credentials the CLI finds. One line per
resource; exit 1 naming every resource without `auto-delete=no`. Run it at least 10 minutes after
the last change so the tagger has had a pass. It finds resources by structure (the VPC tagged
`Name=<stack>` and what is in it, the EKS cluster `<stack>`, roles `<stack>-*`), never by the tag
it checks. ECR repositories named `encabulator/*` in each stack's region are checked too (once per
region): no stack creates them, but a live run that mirrors images does. Offline tests:
`scripts/tag-check_test.py` (the tests' fixtures are in `scripts/tag-check-fixtures/`).

## The audit

`scripts/tag-check-audit.py` lists, without AWS, every AWS resource the four stacks create and
the tag path of each; it fails on a resource without one. For the hub it reads the Terraform
plan (`terraform test -filter=tests/tags.tftest.hcl -verbose`, fake credentials, mocked
Kubernetes and Helm), so a resource added to `terraform/brownfield` shows up here with its
planned `tags_all`.

```sh
scripts/tag-check-audit.py --terraform "mise exec terraform@1.14.9 -- terraform"
scripts/tag-check-audit.py --terraform "..." --write-doc docs/TAGGING.md   # refresh the tables below
```

## Not verified offline

- That the tagger's tag writes succeed against AWS with the conditions in its policy, and that
  Pod Identity works with `automountServiceAccountToken: false` on the tagger's pod.
- That EKS adds a capability's default access policies to an access entry that already exists
  (the hub now pre-creates all three; the cell did already).
- Whether Auto Mode puts the NodeClass tags on launch templates and instance profiles, and
  whether ElastiCache serverless snapshots inherit the cache's tags. The tagger and the check
  cover snapshots and instance profiles either way; launch templates are checked only.

## Audit

<!-- tag-check-audit:begin (generated by scripts/tag-check-audit.py --write-doc; do not edit) -->

### hub (Terraform, us-west-2)

| Resource | Created by | Tag path |
|---|---|---|
| `aws_db_instance.pg` | Terraform | create time: provider default_tags |
| `aws_db_subnet_group.encabulator` | Terraform | create time: provider default_tags |
| `aws_ec2_tag.cluster_security_group` x3 | Terraform | no tags of its own: a tag on a resource EKS created (the cluster security group) |
| `aws_eks_access_entry.admin` | Terraform | create time: provider default_tags |
| `aws_eks_access_entry.capability` x2 | Terraform | create time: provider default_tags |
| `aws_eks_access_entry.kro` | Terraform | create time: provider default_tags |
| `aws_eks_access_entry.node` | Terraform | create time: provider default_tags |
| `aws_eks_access_policy_association.ack_secret_reader` | Terraform | no tags of its own: access policy, part of its EKS access entry |
| `aws_eks_access_policy_association.admin` | Terraform | no tags of its own: access policy, part of its EKS access entry |
| `aws_eks_access_policy_association.argocd_cluster_admin` | Terraform | no tags of its own: access policy, part of its EKS access entry |
| `aws_eks_access_policy_association.node` | Terraform | no tags of its own: access policy, part of its EKS access entry |
| `aws_eks_capability.ack` | Terraform | create time: provider default_tags |
| `aws_eks_capability.argocd` | Terraform | create time: provider default_tags |
| `aws_eks_capability.kro` | Terraform | create time: provider default_tags |
| `aws_eks_cluster.hub` | Terraform | create time: provider default_tags |
| `aws_eks_pod_identity_association.tagger` | Terraform | create time: provider default_tags |
| `aws_elasticache_serverless_cache.valkey` | Terraform | create time: provider default_tags |
| `aws_iam_role.capability` x3 | Terraform | create time: provider default_tags |
| `aws_iam_role.cluster` | Terraform | create time: provider default_tags |
| `aws_iam_role.node` | Terraform | create time: provider default_tags |
| `aws_iam_role.tagger` | Terraform | create time: provider default_tags |
| `aws_iam_role_policy.ack` | Terraform | no tags of its own: inline policy, part of its IAM role |
| `aws_iam_role_policy.cluster_custom_tags` | Terraform | no tags of its own: inline policy, part of its IAM role |
| `aws_iam_role_policy.tagger` | Terraform | no tags of its own: inline policy, part of its IAM role |
| `aws_iam_role_policy_attachment.cluster` x5 | Terraform | no tags of its own: managed policy attachment, part of its IAM role |
| `aws_iam_role_policy_attachment.node` x2 | Terraform | no tags of its own: managed policy attachment, part of its IAM role |
| `aws_security_group.db` | Terraform | create time: provider default_tags |
| `aws_vpc_security_group_ingress_rule.db` x2 | Terraform | create time: provider default_tags |
| `module.vpc.aws_default_network_acl.this` | EC2, with the VPC; adopted by the VPC module | create time: provider default_tags |
| `module.vpc.aws_default_route_table.default` | EC2, with the VPC; adopted by the VPC module | create time: provider default_tags |
| `module.vpc.aws_default_security_group.this` | EC2, with the VPC; adopted by the VPC module | create time: provider default_tags |
| `module.vpc.aws_eip.nat` | Terraform | create time: provider default_tags |
| `module.vpc.aws_internet_gateway.this` | Terraform | create time: provider default_tags |
| `module.vpc.aws_nat_gateway.this` | Terraform | create time: provider default_tags |
| `module.vpc.aws_route.private_nat_gateway` | Terraform | no tags of its own: route, part of its route table |
| `module.vpc.aws_route.public_internet_gateway` | Terraform | no tags of its own: route, part of its route table |
| `module.vpc.aws_route_table.private` | Terraform | create time: provider default_tags |
| `module.vpc.aws_route_table.public` | Terraform | create time: provider default_tags |
| `module.vpc.aws_route_table_association.private` x3 | Terraform | no tags of its own: association, part of its route table |
| `module.vpc.aws_route_table_association.public` x3 | Terraform | no tags of its own: association, part of its route table |
| `module.vpc.aws_subnet.private` x3 | Terraform | create time: provider default_tags |
| `module.vpc.aws_subnet.public` x3 | Terraform | create time: provider default_tags |
| `module.vpc.aws_vpc.this` | Terraform | create time: provider default_tags |
| main route table, default security group, default network ACL | EC2, with the VPC | create time: adopted by the VPC module (manage_default_*), default_tags |
| EKS cluster security group | EKS | create time (right after): `aws_ec2_tag.cluster_security_group` |
| EKS access entry for the applying principal | Terraform (EKS would create it untagged) | create time: `aws_eks_access_entry.admin`, bootstrap_cluster_creator_admin_permissions = false |
| EKS access entries of the Argo CD, ACK and kro capabilities | Terraform, before each capability (EKS would create them untagged) | create time: provider default_tags |
| EC2 instances, root volumes, network interfaces, launch templates of Auto Mode nodes | EKS Auto Mode | create time: NodeClass `encabulator` spec.tags (nodes.tf, custom-tags policy on the cluster role) |
| EBS volumes for the NATS and InfluxDB PVCs | EKS Auto Mode EBS driver | create time: StorageClass `encabulator-gp3` tagSpecification_1..3 (charts/encabulator/values-brownfield.yaml) |
| RDS automated backups of `encabulator-pg` | RDS | create time: copy_tags_to_snapshot = true |
| requester-managed network interfaces: EKS control plane, NAT gateway, RDS, ElastiCache | EKS, VPC, RDS, ElastiCache | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| ElastiCache serverless snapshots of `encabulator-valkey` (daily) | ElastiCache | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| IAM instance profile of `encabulator-usw2-node` | EKS Auto Mode | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| EKS access entry of the service-linked role AWSServiceRoleForAmazonEKS | EKS Auto Mode | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| load balancers, their security groups and target groups | EKS Auto Mode | none created: no Ingress and no Service of type LoadBalancer (chart rendered with the hub overlays) |

### hub (ACK, Act 1)

| Resource | Created by | Tag path |
|---|---|---|
| DBInstance `encabulator-pg` (gitops/hub/brownfield/postgres.yaml) | Terraform; ACK adopts it (adopt-or-create) | ACK spec.tags (same three as Terraform, D3) |
| DBSubnetGroup `encabulator` (gitops/hub/brownfield/subnet-group.yaml) | Terraform; ACK adopts it (adopt-or-create) | ACK spec.tags (same three as Terraform, D3) |
| ServerlessCache `encabulator-valkey` (gitops/hub/brownfield/valkey.yaml) | Terraform; ACK adopts it (adopt-or-create) | ACK spec.tags (same three as Terraform, D3) |
| DBInstance `encabulator-pg` (gitops/explore/postgres.yaml) | nothing: adopts the Terraform resource and reads its spec back | tagged by Terraform; ACK writes no tags |

### cell (EncabulatorCell RGD, hub ACK)

| Resource | Created by | Tag path |
|---|---|---|
| VPC `${schema.metadata.name}` | ACK on the hub | create time: ACK spec.tags |
| InternetGateway `${schema.metadata.name}` | ACK on the hub | create time: ACK spec.tags |
| RouteTable `${schema.metadata.name}-public` | ACK on the hub | create time: ACK spec.tags |
| Subnet `${schema.metadata.name}-public-${az}` per AZ | ACK on the hub | create time: ACK spec.tags |
| ElasticIPAddress `${schema.metadata.name}-nat` | ACK on the hub | create time: ACK spec.tags |
| NATGateway `${schema.metadata.name}` | ACK on the hub | create time: ACK spec.tags |
| RouteTable `${schema.metadata.name}-private` | ACK on the hub | create time: ACK spec.tags |
| Subnet `${schema.metadata.name}-private-${az}` per AZ | ACK on the hub | create time: ACK spec.tags |
| Role `${schema.metadata.name}-cluster` | ACK on the hub | create time: ACK spec.tags |
| Role `${schema.metadata.name}-node` | ACK on the hub | create time: ACK spec.tags |
| Role `${schema.metadata.name}-ack` | ACK on the hub | create time: ACK spec.tags |
| Role `${schema.metadata.name}-kro` | ACK on the hub | create time: ACK spec.tags |
| Cluster `${schema.metadata.name}` | ACK on the hub | create time: ACK spec.tags |
| AccessEntry `${schema.metadata.name}-node` | ACK on the hub | create time: ACK spec.tags |
| AccessEntry `${schema.metadata.name}-ack` | ACK on the hub | create time: ACK spec.tags |
| AccessEntry `${schema.metadata.name}-kro` | ACK on the hub | create time: ACK spec.tags |
| Capability `${schema.metadata.name}-ack` | ACK on the hub | create time: ACK spec.tags |
| Capability `${schema.metadata.name}-kro` | ACK on the hub | create time: ACK spec.tags |
| AccessEntry `${schema.metadata.name}-argocd` | ACK on the hub | create time: ACK spec.tags |
| AccessEntry `${schema.metadata.name}-admin` (optional) | ACK on the hub | create time: ACK spec.tags |
| SecurityGroup `${schema.metadata.name}-db` | ACK on the hub | create time: ACK spec.tags |
| main route table, default security group, default network ACL | EC2, with the VPC | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf); the VPC carries project + talk, its region is swept (encabulator-use2 in us-east-2) |
| EKS cluster security group, EKS control-plane network interfaces | EKS | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| NAT gateway network interface | VPC | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| EKS access entry for the cluster creator | nobody | not created: bootstrapClusterCreatorAdminPermissions false |
| EKS access entries of the ACK and kro capabilities | ACK, before each capability (EKS would create them untagged) | create time: ACK spec.tags (AccessEntry rows above) |
| IAM instance profile of the cell's node role | EKS Auto Mode | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| EKS access entry of the service-linked role AWSServiceRoleForAmazonEKS | EKS Auto Mode | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |

### cell (Encabulator RGDs, cell ACK + Auto Mode)

| Resource | Created by | Tag path |
|---|---|---|
| DBSubnetGroup `${schema.metadata.name}` | ACK on the cell | create time: ACK spec.tags |
| DBInstance `${schema.metadata.name}-pg` | ACK on the cell | create time: ACK spec.tags |
| ServerlessCache `${schema.metadata.name}-valkey` | ACK on the cell | create time: ACK spec.tags |
| EncabulatorService (x15) | kro | no AWS resources: Kubernetes objects only |
| RDS automated backups | RDS | create time: DBInstance copyTagsToSnapshot true |
| RDS and ElastiCache requester-managed network interfaces | RDS, ElastiCache | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| ElastiCache serverless snapshots of `encabulator-valkey` (daily) | ElastiCache | post-pass: tagger CronJob on the hub (terraform/brownfield/tagger.tf) |
| EBS volumes for PVCs of StorageClass `encabulator-gp3` | EKS Auto Mode EBS driver | create time: StorageClass tagSpecification_1..3 |
| EC2 instances, root volumes, network interfaces, launch templates (NodeClass `encabulator`) | EKS Auto Mode | create time: NodeClass spec.tags (custom-tags policy on the cell's cluster role) |
| load balancers, their security groups and target groups | EKS Auto Mode | none created: no Ingress and no Service of type LoadBalancer in gitops/app |

### account-level (not created by these stacks)

| Resource | Created by | Tag path |
|---|---|---|
| AWS managed KMS keys `aws/rds`, `aws/ebs` | AWS | none needed: one per account and Region, owned by AWS; cannot be tagged or deleted |
| default RDS parameter group and option group for postgres18 | AWS | none needed: RDS defaults shared by every instance in the Region; cannot be deleted |
| default DHCP option set | AWS | none needed: one per Region, shared by every VPC |
| service-linked roles AWSServiceRoleForAmazonEKS, -RDS, -ElastiCache | AWS | none needed: one per account, created the first time a service is used |

<!-- tag-check-audit:end -->
