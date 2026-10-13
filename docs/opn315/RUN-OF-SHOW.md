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
| 40:00-53:00 | 3 Replicate | Hub and spoke: `EncabulatorCell` on the hub creates the us-east-2 VPC, cluster, ACK and kro capabilities and Argo CD registration; ApplicationSet syncs the app; ACK on the cell creates RDS + ElastiCache; where the time goes (a captured run) | Live trigger, baked result |
| 53:00-56:00 | Close | Day 2: three reconcile loops (delete a Deployment, change the DB class in Git) | Live |
| 56:00-60:00 | Close | Recipe card (best practices), links, Q&A | Slides |

Act 3 timings, from a run of this demo (counted from the trigger): network about 3 minutes; EKS
Auto Mode cluster about 11 minutes; kro capability about 1.5 minutes and ACK capability about 7.5
minutes after the cluster; the cell is ACTIVE about 20 minutes after the trigger; the app is
Healthy about 14 minutes after that (ElastiCache Serverless 1.5 minutes, RDS 8.5 minutes); first
order at about 35 minutes. The critical path is the cluster, the ACK capability, the first sync's
retries and RDS. The "Where the time goes" slide shows these as a captured run.

Oven move (optional): commit `gitops/hub/cells/use2.yaml` at about 25:00, before Act 2, so the new
region builds while we explain kro. It starts the build only if `hub-cells` is staged as in "Act 3
trigger" below. From 25:00 the cell is registered by about 45:00 but the app is Healthy only near
59:00, so the pre-baked cell still carries the Act 3 result (open decision 5). Keep it running as
the fallback.

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

## Presenter notes for the live segments

Act 1, adopt:

- Always type `dbinstances.rds` (`kubectl get dbinstances.rds encabulator-pg`). With the ACK
  capability, the bare `dbinstance`/`dbinstances` resolves to the DocumentDB kind and prints "No
  resources found", on the hub and on the cell.

Act 3 trigger, how it is staged:

- `hub-cells` (`gitops/hub/bootstrap/hub-cells.yaml`) must already be applied and track a
  revision without `cells/use2.yaml`. Then the live `git push` of `use2.yaml` is the trigger.
  If `use2.yaml` is already on the revision `hub-cells` tracks, applying `hub-cells` is what
  starts the build (open decisions 2 and 3).
- The bake revision note still applies: the revision the hub tracks for the bake must not list
  `postgres.yaml` in `gitops/hub/brownfield/kustomization.yaml` yet (open decision 2).

Act 3, what the presenter types:

```sh
# hub context (encabulator-usw2)
kubectl -n argocd get applications                       # hub-brownfield, hub-rgds, hub-appset, hub-cells
git push                                                 # the commit that adds gitops/hub/cells/use2.yaml
kubectl get encabulatorcells -n encabulator-cells -w     # CLUSTER: CREATING -> ACTIVE, STATE: IN_PROGRESS -> ACTIVE
kubectl get clusters.eks.services.k8s.aws,capabilities.eks.services.k8s.aws -n encabulator-cells
kubectl get secrets -n argocd -l encabulator.io/cell=true    # encabulator-use2
kubectl get applications -n argocd                       # encabulator-use2 appears the same second
# cell context
aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2
kubectl --context use2 get encabulator,encabulatorservices -n encabulator
kubectl --context use2 get dbinstances.rds,serverlesscaches -n encabulator
kubectl --context use2 -n encabulator get encabulator encabulator -o yaml | yq .status
```

- First sync retries: the new cell's NodePool reads Degraded for about 10 seconds while its
  NodeClass validates, so Argo CD's first sync of `encabulator-use2` fails that wave and the UI
  shows "Retrying attempt #1". It heals on its own (backoff 30 s, factor 2); in the captured run
  it cost about 4.5 minutes.
- Presenter access: the cell's cluster is created without creator admin permissions, so
  `kubectl --context use2` is refused until the cell has an access entry for your role: set
  `spec.adminRoleName` on the cell (kro creates the entry), or create an access entry by hand.
- Image mirror: until 1.0.1 is on GHCR and its dingle-arm-gateway package is public, all 15
  images come from a mirror. Add the annotation to the cell's cluster Secret right after kro
  creates it, before Argo CD's first sync:
  `kubectl -n argocd wait --for=create secret/encabulator-use2 --timeout=40m && kubectl -n argocd annotate secret encabulator-use2 encabulator.io/registry=<account>.dkr.ecr.us-west-2.amazonaws.com/encabulator`.
  kro applies the Secret with server-side apply and does not own that annotation, so it keeps
  it; the ApplicationSet then sets `spec.registry` on the cell's instance.

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
   settings, exactly three tags in the spec): listed in `gitops/hub/brownfield/README.md`. In AWS,
   the ACK capability also adds five `eks:*` tags of its own to every resource it manages
   (`eks:controller-version`, `eks:eks-capability-arn`, `eks:kubernetes-api-group`,
   `eks:kubernetes-namespace`, `eks:kubernetes-resource-name`).
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
   For the live push to be the trigger, `hub-cells` must track a revision without `use2.yaml`,
   which cannot also be the revision that built the pre-baked cell under the same name. Decide
   whether the fallback is a second cell (another name and region) or the live commit is shown
   against the cell baked the day before.
4. Presenter access to the cell: `cells/use2.yaml` leaves `spec.adminRoleName` empty and the cell
   is created without creator admin permissions, so `kubectl --context use2` is refused until the
   cell you show sets it or has an access entry for your role made by hand.
5. Oven move timing. Committed at 25:00, the cell is registered by about 45:00 and the app is
   Healthy near 59:00, after Act 3. To show the live cell's app in Act 3, the commit would have to
   land by about 8:00 (start of Act 1); otherwise the oven move shows the live registration and
   the pre-baked cell shows the result.

## Verified on AWS

Confirmed on the hub (us-west-2) and on a cell built from `cells/use2.yaml` (us-east-2):

- Creating a capability when its access entry already exists: all three on the hub (Terraform),
  ACK and kro on the cell (`EncabulatorCell`). EKS adds its default policies to the tagged entries.
- Auto Mode with no built-in node pools (`nodePools: []`): nodes come up from the custom NodeClass
  + NodePool and the EC2 access entry, on the hub and on the cell.
- A clean `terraform plan` ("No changes") after the hand-off (`stores.tf` deleted, `handoff.tf`
  in place).
- ElastiCache Serverless with no AUTH, with the 1.0.1 images: every client connects over TLS with
  an empty password; the quoter (rustls, system roots) returns `cached:true`.
- DBInstance port: the adopted `encabulator-pg` and the cell's kro-created instance both stay on
  5432 with no pending changes, and ACK makes no Modify call after it settles.
- Adoption on the capability's controller versions (rds-v1.12.1, elasticache-v1.9.2):
  `adoption-fields` key `dbInstanceIdentifier`, `adopt` and `adopt-or-create`.
- The status fields kro waits for (`dbInstanceStatus`, ServerlessCache `status` and `endpoint`),
  the ACK capability reading the `encabulator-db` Secret, and kro's RBAC on the group
  `encabulator:kro`: the cell's graph went ACTIVE and an order was delivered through its gateway.
- Every AWS resource tagged `auto-delete=no`, including the ones AWS creates without a tag
  request (main route tables, default security groups and network ACLs, EKS cluster security
  groups, requester-managed network interfaces, the VPC endpoints ElastiCache Serverless
  creates), which the hub's tagger CronJob tags every 10 minutes (`docs/TAGGING.md`).
  `scripts/tag-check.py` exits 0 for both regions, including Auto Mode nodes and those VPC
  endpoints.

Still open:

- Valkey 9 on ElastiCache Serverless: 9.0 and 9.1 are listed in us-west-2, not tried on
  Serverless (8 for now).
- The arm64 build of the quoter image (rustls with the ring provider); only amd64 has been built.

## Prep checklist (the day before)

- Hub: in `terraform/brownfield`, `terraform apply` (about 25 minutes), then
  `$(terraform output -raw kubeconfig_command)` and `make -C ../.. platform && ./deploy-app.sh`.
  Confirm the console shows traffic.
- Wait for the three capabilities to be ACTIVE (`aws eks describe-capability`).
- If the repo is still private, give Argo CD a repository Secret in `argocd` (a read-only GitHub
  token) before the bootstrap, or every Application fails to fetch.
- `kubectl config set-context --current --namespace encabulator` (the Act 1 commands assume it),
  `./gitops/hub/bootstrap/register-hub.sh --dry-run`, then without `--dry-run`, then
  `kubectl apply -f` every file in `gitops/hub/bootstrap/` except `hub-cells.yaml`. Apply
  `hub-cells.yaml` once its `targetRevision` is the revision staged for the Act 3 trigger
  (presenter notes above).
- Point the hub at the bake revision (open decision 2). The cache and the DB subnet group report
  synced; `encabulator-pg` is not adopted yet.
- Bake one cell end to end with `spec.adminRoleName` set (or an access entry for your role), then
  `aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2`. Leave it
  running as the fallback.
- Log in to the Argo CD UI (IAM Identity Center); open the RDS console.
- Images: pin the image tag (`1.0.1`, the chart's appVersion and the RGD default). Until 1.0.1 is
  on GHCR and its dingle-arm-gateway package is public, all 15 images come from a mirror
  (`<account>.dkr.ecr.us-west-2.amazonaws.com/encabulator/<service>:1.0.1`): point the hub's
  chart at it (the Makefile's `REGISTRY`, which sets `global.registry`), and give each cell's
  cluster Secret the `encabulator.io/registry` annotation right after kro creates it (presenter
  notes above). Pre-pull images; phone hotspot; a recording of each live segment.
- Last: at least 10 minutes after the last change (one tagger pass), run `scripts/tag-check.py`
  against the account. It checks the hub (`us-west-2:encabulator-usw2`) and the cell
  (`us-east-2:encabulator-use2`) with read-only calls and exits 1 naming every resource without
  `auto-delete=no`. Fix any it names before the talk. The tagger does not tag the three VPC
  endpoints ElastiCache Serverless creates in each VPC; if tag-check names them, tag them by hand.

## Teardown

Every ACK resource that holds data uses `deletion-policy: retain`, so deleting its custom resource
never deletes the AWS resource. Delete the ACK resources, then the RDS instances, caches and
subnet groups yourself, then `terraform destroy` the hub (`terraform/brownfield/README.md`,
Teardown). The generated Secrets on the cell stay until you delete them.
