# gitops/hub/brownfield

The AWS resources Terraform built for the hub, adopted by ACK and owned by Git from Act 1 on.
Argo CD syncs this folder to the hub (`encabulator-usw2`, namespace `encabulator`) through the
`hub-brownfield` ApplicationSet in [`../bootstrap`](../bootstrap/README.md).

| File | Kind | AWS resource |
|---|---|---|
| `postgres.yaml` | `rds.services.k8s.aws/v1alpha1` DBInstance | RDS PostgreSQL `encabulator-pg` |
| `valkey.yaml` | `elasticache.services.k8s.aws/v1alpha1` ServerlessCache | ElastiCache Serverless `encabulator-valkey` |
| `subnet-group.yaml` | `rds.services.k8s.aws/v1alpha1` DBSubnetGroup | DB subnet group `encabulator` |
| `kustomization.yaml` | kustomize | lists the three; Argo CD appends the id patches here |

The VPC, the subnets and the security group `encabulator-db` stay with Terraform.

Every CR carries `services.k8s.aws/adoption-policy: adopt-or-create` (adopt it if it exists and
make it match this spec, create it if it does not), `services.k8s.aws/deletion-policy: retain`
(deleting the CR never deletes the AWS resource), and exactly the three contract tags.

## Environment ids are not in Git

| Field | How it is filled |
|---|---|
| DBInstance `kmsKeyID`, `availabilityZone` | left out; ACK late-initializes them from AWS |
| DBInstance `vpcSecurityGroupIDs` | left out; the rds-controller compares it only when the spec sets it |
| DBSubnetGroup `subnetIDs` | placeholder; patched from cluster Secret annotation `encabulator.io/subnet-ids` |
| ServerlessCache `subnetIDs` | placeholder; patched from `encabulator.io/subnet-ids` |
| ServerlessCache `securityGroupIDs` | placeholder; patched from `encabulator.io/security-group-id` |

The last three cannot be left out: neither controller late-initializes them, and an empty spec
field reads as drift on every reconcile (details and source files in the bootstrap README).
`kubectl kustomize gitops/hub/brownfield` therefore renders placeholders; do not apply it by hand.

## Values Terraform must set to the same thing

ACK reads these back from AWS and would otherwise keep trying to change them:

- RDS: engine `postgres` major `18`, `db.t4g.medium`, 20 GiB `gp3`, encrypted, not public,
  single-AZ, backup retention 7, admin user `postgres` (`masterUsername`), subnet group
  `encabulator`. No `max_allocated_storage`, no CloudWatch log exports, no Performance Insights.
- ServerlessCache: engine `valkey` major `8`, description `Encabulator Valkey cache`,
  `daily_snapshot_time = "09:00"`, `snapshot_retention_limit = 1`, no user group, the same subnets
  as the DB subnet group and the single security group `encabulator-db`.
- DB subnet group: description `Encabulator databases`.
- Tags on all three: exactly `auto-delete=no`, `project=encabulator`, `talk=opn315`. ACK removes
  tags that are not in the spec.
