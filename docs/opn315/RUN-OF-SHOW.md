# OPN315 run of show: Bootstrapping GitOps on Amazon EKS with Argo CD, ACK, and kro

Code Talk, 60 minutes, level 300. Carlos Santana and Jesse Butler.
Deck: [index.html](index.html) in this folder (open it in a browser; N = notes, O = slide list, F = fullscreen).

Format: a cooking show. Each act does one thing live. Anything that has to wait (cluster
creation, capabilities becoming ACTIVE, RDS or ElastiCache creation) is baked before the session.
Slides carry diagrams and partial YAML. The full files are shown in the IDE.

## Storyline

| Time | Act | On stage | Live or baked |
|---|---|---|---|
| 0:00-2:00 | Open | Title, who we are | - |
| 2:00-6:00 | Open | Encabulator console: Factory Floor, place an order | Live (app already running) |
| 6:00-8:00 | Open | Two toolchains (Terraform + Helm, ConfigMap hand-off); the menu | Slides |
| 8:00-11:00 | 1 Adopt | Starting point in us-west-2: Terraform built VPC, EKS, RDS, ElastiCache and wrote a ConfigMap + Secret | Baked |
| 11:00-13:00 | 1 Adopt | EKS Capabilities: Argo CD, ACK, kro on the hub cluster | Baked (`describe-capability` live) |
| 13:00-24:00 | 1 Adopt | Adopt the RDS instance with ACK: `adopt`, read the spec back, commit it to Git, Terraform `removed {}`, change a setting from Git | Live |
| 24:00-26:00 | 1 Adopt | Same recipe for ElastiCache, the DB subnet group and the security group | Baked |
| 26:00-40:00 | 2 Package | kro RGDs: `Encabulator` (stores + 15 services) and `EncabulatorService`; CEL wiring; `forEach`; graph order | Live (walk the RGD in the IDE, apply it) |
| 40:00-53:00 | 3 Replicate | Hub and spoke: `EncabulatorCell` on the hub creates the us-east-2 cluster + capabilities + Argo CD registration; ApplicationSet syncs the app; ACK on the spoke creates RDS + ElastiCache | Live trigger, baked result |
| 53:00-56:00 | Close | Day 2: three reconcile loops (delete a Deployment, change the DB class in Git) | Live |
| 56:00-60:00 | Close | Recipe card (best practices), links, Q&A | Slides |

Oven move (optional): commit `gitops/hub/cells/use2.yaml` at about 25:00, before Act 2. The new
region builds while we explain kro; by 40:00 it is ready to show. Keep a pre-baked cell as the
fallback.

## Architecture we demo

- Hub: `encabulator-usw2` in us-west-2. Argo CD, ACK and kro capabilities. Runs the brownfield app.
- Cell: `encabulator-use2` in us-east-2. ACK and kro capabilities, created from the hub by ACK.
  Argo CD on the hub deploys to it.
- Stores: PostgreSQL -> Amazon RDS, Valkey -> Amazon ElastiCache (serverless). NATS JetStream and
  InfluxDB 3 stay in-cluster StatefulSets: ACK has no Timestream for InfluxDB controller (checked
  against the ACK API reference list), and NATS has no AWS-managed equivalent.
- No Helm for the app on the cell: kro is the package. The brownfield hub app stays on its Helm
  chart; only its AWS resources change owner.

## Repo layout to build (proposed paths, used on the IDE cue slides)

```
terraform/brownfield/          VPC, hub EKS cluster, RDS, ElastiCache, ConfigMap + Secret hand-off
  handoff.tf                   the `removed {}` blocks added in Act 1
gitops/hub/
  bootstrap/                   Argo CD Applications for brownfield/, rgds/, appset.yaml
  brownfield/                  ACK CRs adopted from Terraform (DBInstance, ServerlessCache, ...)
  rgds/encabulator-cell.yaml   EncabulatorCell: EKS Cluster, Capabilities, AccessEntry, cluster Secret
  cells/use2.yaml              one EncabulatorCell instance = one new region
  appset.yaml                  ApplicationSet, cluster generator on encabulator.io/cell=true
gitops/app/
  rgds/encabulator.yaml        Encabulator: subnet group, SG, RDS, ElastiCache, NATS, InfluxDB, ConfigMap, forEach services
  rgds/encabulator-service.yaml  EncabulatorService: ServiceAccount, Deployment, Service, HPA, PDB
  instance/encabulator.yaml    the Encabulator instance the ApplicationSet syncs to each cell
```

The chart already supports external stores (`postgres.external.*`, `valkey.external.*`,
`existingSecret`), so the brownfield Terraform only needs to write the endpoint values and a
Secret with the per-database password keys the chart reads.

## Decisions still open

1. How region and subnet IDs reach the cell's `Encabulator` instance: the plan is cluster-Secret
   annotations, then ApplicationSet template, then a kustomize patch. A per-cell folder in Git is
   the alternative.
2. DB credentials on the cell: RDS-managed secret in Secrets Manager with a sync to Kubernetes, or
   a kro-generated Secret. The services read `PGPASSWORD`/`DATABASE_URL`, so they need no code change.
3. Who drives which act, and who types.
4. VPC for the cell: created by ACK EC2 inside `EncabulatorCell`, or pre-created and passed in.

## Verify on a cluster before the talk

- `adoption-fields` key for RDS DBInstance (`dbInstanceIdentifier`). The ACK docs table lists other
  kinds, not DBInstance.
- `adopt-or-create` on DBInstance and ServerlessCache. Resource adoption is a Beta feature in ACK.
- `status.endpoint.address` on ServerlessCache (DBInstance `status.endpoint` is confirmed in the
  rds-controller types).
- ACK EKS `Cluster` field names for the VPC config and Auto Mode.
- kro `forEach` over an instance-chained RGD (`EncabulatorService`) and the order it builds in.
- Argo CD capability: access entry plus RBAC on the cell, created by ACK `AccessEntry`.

## Prep checklist (the day before)

- Terraform apply the brownfield stack; deploy the app with Helm; confirm the console shows traffic.
- Create the three capabilities on the hub and wait for ACTIVE; grant kro and Argo CD RBAC.
- Bake one cell end to end; leave it running as the fallback.
- Pre-pull images; pin the image tag; phone hotspot; a recording of each live segment.
- After the talk: every ACK resource holding data uses `deletion-policy: retain`, so remove the AWS
  resources deliberately at teardown.
