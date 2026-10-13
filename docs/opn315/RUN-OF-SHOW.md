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
| 8:00-11:00 | 1 Adopt | Starting point in us-west-2: Terraform built VPC, EKS, RDS, ElastiCache and wrote the ConfigMap `encabulator-aws` (from data sources) + Secret `encabulator-db` | Baked |
| 11:00-13:00 | 1 Adopt | EKS Capabilities: Argo CD, ACK, kro on the hub cluster, roles `encabulator-usw2-argocd/-ack/-kro` | Baked (`describe-capability` live) |
| 13:00-24:00 | 1 Adopt | Adopt the RDS instance with ACK: `adopt`, read the spec back, commit it to Git, Terraform `removed {}` (`handoff.tf.example` becomes `handoff.tf`), change a setting from Git | Live |
| 24:00-26:00 | 1 Adopt | Same recipe for the serverless cache and the DB subnet group; the VPC and security group stay with Terraform | Baked |
| 26:00-40:00 | 2 Package | kro RGDs: `Encabulator` (stores + 15 services) and `EncabulatorService`; CEL wiring; `forEach`; graph order | Live (walk the RGD in the IDE, apply it) |
| 40:00-53:00 | 3 Replicate | Hub and spoke: `EncabulatorCell` on the hub creates the us-east-2 VPC, cluster, capabilities and Argo CD registration; ApplicationSet syncs the app; ACK on the cell creates RDS + ElastiCache | Live trigger, baked result |
| 53:00-56:00 | Close | Day 2: three reconcile loops (delete a Deployment, change the DB class in Git) | Live |
| 56:00-60:00 | Close | Recipe card (best practices), links, Q&A | Slides |

Oven move (optional): commit `gitops/hub/cells/use2.yaml` at about 25:00, before Act 2. The new
region builds while we explain kro (EKS documents about 15 minutes for an Auto Mode cluster, and
the cell's RDS instance comes after it). Keep a pre-baked cell as the fallback.

## Architecture we demo

- Hub: `encabulator-usw2` in us-west-2, EKS Auto Mode with a tagged NodeClass + NodePool (no
  built-in node pools). Argo CD, ACK and kro capabilities, created by Terraform with roles
  `encabulator-usw2-argocd`, `-ack`, `-kro`. Runs the brownfield app on its Helm chart.
- Cell: `encabulator-use2` in us-east-2. Built from the hub by ACK through the `EncabulatorCell`
  graph: VPC (/16, one NAT gateway), IAM roles `encabulator-use2-*`, Auto Mode cluster with no
  built-in node pools, access entries, ACK and kro capabilities, the `encabulator-db` security
  group, and the Argo CD cluster Secret. Argo CD on the hub deploys `gitops/app` to it.
- Stores: PostgreSQL -> Amazon RDS for PostgreSQL 18, Valkey -> Amazon ElastiCache Serverless
  (Valkey 8, TLS, no password, access by security group). NATS JetStream and InfluxDB 3 stay
  in-cluster StatefulSets: ACK has no Timestream for InfluxDB controller, and NATS has no
  AWS-managed equivalent.
- No Helm for the app on the cell: kro is the package. The brownfield hub app stays on its Helm
  chart; only its AWS resources change owner.

## Repo layout (the paths on the IDE cue slides)

```
terraform/brownfield/            hub stack: VPC, hub EKS Auto Mode cluster, RDS, ElastiCache, capabilities
  stores.tf                      RDS instance, DB subnet group, serverless cache (deleted live in Act 1)
  kubernetes.tf                  ConfigMap encabulator-aws (from data sources), Secrets, db-init Job
  capabilities.tf                Argo CD, ACK and kro capabilities and their roles
  handoff.tf.example             the removed {} blocks, renamed to handoff.tf live in Act 1
  deploy-app.sh                  the Helm chart on RDS + ElastiCache
gitops/explore/postgres.yaml     adopt-only DBInstance, applied by hand in Act 1 and then deleted
gitops/hub/
  bootstrap/                     AppProjects, hub-brownfield ApplicationSet, hub-rgds, hub-cells, hub-appset,
                                 register-hub.sh (the hub's cluster Secret, not committed)
  brownfield/                    ACK CRs Git owns after Act 1: postgres.yaml, valkey.yaml, subnet-group.yaml,
                                 kustomization.yaml (subnet and SG ids patched in from the cluster Secret)
  rgds/encabulator-cell.yaml     EncabulatorCell: VPC, IAM roles, EKS cluster, access entries, capabilities,
                                 security group, Argo CD cluster Secret
  rbac/kro-cell.yaml             namespace encabulator-cells and kro's RBAC on the hub (group encabulator:kro)
  cells/use2.yaml                one EncabulatorCell instance = one new region (13 lines)
  appset.yaml                    ApplicationSet encabulator: cluster generator on encabulator.io/cell=true
gitops/app/                      what Argo CD syncs to every cell; kustomization.yaml is the root
  rbac/                          namespace encabulator and kro's RBAC (group encabulator:kro)
  nodes/auto-mode.yaml           tagged Auto Mode NodeClass + NodePool
  rgds/encabulator.yaml          Encabulator: DB subnet group, RDS, ElastiCache, generated Secrets, db-init,
                                 NATS, InfluxDB, ConfigMap encabulator-aws, forEach services
  rgds/encabulator-service.yaml  EncabulatorService: ServiceAccount, Deployment, Service (+ HPA, PDB, Role)
  instance/encabulator.yaml      the Encabulator instance; subnet and SG ids patched in per cell
```

Each folder's README has the details and the pinned versions.

## Decisions

Decided (OPN315 build contract, lead decisions D1-D10):

1. Environment ids never enter Git. Cells: kro writes the subnet, security group and cluster
   security group ids as annotations on the cell's Argo CD cluster Secret; the `encabulator`
   ApplicationSet (goTemplate) patches them into the Encabulator instance and the NodeClass. Hub:
   `register-hub.sh` writes the same annotations on the hub's cluster Secret, and `hub-brownfield`
   is a one-cluster ApplicationSet that patches them into the DB subnet group and the cache.
2. DB credentials on the cell: a one-time `secret-init` Job in the Encabulator graph creates
   `encabulator-db` (and the app, Valkey and InfluxDB Secrets) only if absent; the DBInstance reads
   `masterUserPassword` from it. No Secrets Manager sync. Services read `PGPASSWORD`/`DATABASE_URL`
   as before.
3. VPC for the cell: created by ACK EC2 inside `EncabulatorCell`.
4. Store values Terraform and ACK share (engine major 18, Valkey 8, descriptions, snapshot
   settings, exactly three tags): listed in `gitops/hub/brownfield/README.md`.
5. kro RBAC binds the Kubernetes group `encabulator:kro` on its access entry, never a role ARN.
6. Argo CD is cluster admin on the hub and the cells: a demo setting, called out as such.
7. Every AWS resource carries `auto-delete=no`, `project=encabulator`, `talk=opn315`: tagged at
   create time where the API allows, and by the hub's tagger CronJob for what AWS creates
   untagged (`docs/TAGGING.md`).

Still open:

1. Who drives which act, and who types.
2. Bake revision. The hub Applications track `carrlos`, where `gitops/hub/brownfield/kustomization.yaml`
   already lists `postgres.yaml`. The bake needs a revision without that line, or ACK adopts the
   instance the day before and the live `adopt` step collides with it; the live commit then has to
   land on the revision Argo CD tracks. Switch every `targetRevision` to the release branch before
   the talk.
3. Live cell and fallback cell. Both would come from `cells/use2.yaml` as `encabulator-use2`.
   Decide whether the fallback is a second cell (another name and region) or the live commit is
   shown against the cell baked the day before.
4. Presenter access to the cell: `cells/use2.yaml` leaves `spec.adminRoleName` empty, so
   `kubectl --context use2` is refused until the cell you show sets it.

## Verify on a cluster before the talk

Checked offline (controller source, CRDs, kind clusters) but not yet against AWS:

- Creating a capability when its access entry already exists: all three on the hub (Terraform),
  ACK and kro on the cell (`EncabulatorCell`).
- Auto Mode with no built-in node pools: nodes come up from the custom NodeClass + NodePool and
  the EC2 access entry, on the hub and on a cell.
- A clean `terraform plan` after the hand-off (`stores.tf` deleted, `handoff.tf` in place).
- ElastiCache Serverless with no AUTH: every client connects over TLS with an empty password, and
  each client's root store accepts the cache's certificate chain (the quoter uses rustls with the
  system root store).
- Valkey 9 on ElastiCache Serverless (8 for now).
- DBInstance port drift: the adopted `encabulator-pg` and the cell's kro-created instance settle
  with no port change (Terraform sets `port = 5432`; the ACK specs leave the port out).
- The arm64 build of the quoter image (rustls with the ring provider); only amd64 has been built.
- Every AWS resource tagged `auto-delete=no`, including the ones AWS creates without a tag
  request (main route tables, default security groups and network ACLs, EKS cluster security
  groups, requester-managed network interfaces, the VPC endpoints ElastiCache Serverless
  creates), which the hub's tagger CronJob tags every 10 minutes (`docs/TAGGING.md`).
  `scripts/tag-check.py` checks the live account.
- Adoption on the capability's controller versions: `adoption-fields` key `dbInstanceIdentifier`,
  `adopt` versus `adopt-or-create` (resource adoption is a Beta feature in ACK).
- The status fields kro waits for (`dbInstanceStatus`, ServerlessCache `status` and `endpoint`).
- The ACK capability reading the `encabulator-db` Secret, and the kro capability's identity
  carrying the group `encabulator:kro`.

## Prep checklist (the day before)

- Hub: in `terraform/brownfield`, `terraform apply` (about 25 minutes), then
  `$(terraform output -raw kubeconfig_command)` and `make -C ../.. platform && ./deploy-app.sh`.
  Confirm the console shows traffic.
- Wait for the three capabilities to be ACTIVE (`aws eks describe-capability`).
- `kubectl config set-context --current --namespace encabulator` (the Act 1 commands assume it),
  `./gitops/hub/bootstrap/register-hub.sh --dry-run`, then without `--dry-run`, then
  `kubectl apply -f gitops/hub/bootstrap/`.
- Point the hub at the bake revision (open decision 2). The cache and the DB subnet group report
  synced; `encabulator-pg` is not adopted yet.
- Bake one cell end to end with `spec.adminRoleName` set, then
  `aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2`. Leave it
  running as the fallback.
- Log in to the Argo CD UI (IAM Identity Center); open the RDS console.
- Pre-pull images; pin the image tag (`1.0.0`); phone hotspot; a recording of each live segment.
- Last: at least 10 minutes after the last change (one tagger pass), run `scripts/tag-check.py`
  against the account. It checks the hub (`us-west-2:encabulator-usw2`) and the cell
  (`us-east-2:encabulator-use2`) with read-only calls and exits 1 naming every resource without
  `auto-delete=no`. Fix any it names before the talk.

## Teardown

Every ACK resource that holds data uses `deletion-policy: retain`, so deleting its custom resource
never deletes the AWS resource. Delete the ACK resources, then the RDS instances, caches and
subnet groups yourself, then `terraform destroy` the hub (`terraform/brownfield/README.md`,
Teardown). The generated Secrets on the cell stay until you delete them.
