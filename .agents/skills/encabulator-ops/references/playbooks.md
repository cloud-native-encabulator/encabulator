# Playbooks

One per symptom. Each starts with READ ONLY checks, says what their output means, then the fix,
marked READ ONLY or CHANGES THINGS (ask first, [stage-rules.md](stage-rules.md)), and ends with
a sentence for the audience. Commands assume the hub context `encabulator-usw2` unless they say
`--context use2`. The incidents come from real runs of this demo.

## Argo CD Application OutOfSync or Degraded

READ ONLY:

```bash
kubectl -n argocd get applications
kubectl -n argocd get application <app> -o jsonpath='{.status.operationState.phase}: {.status.operationState.message}{"\n"}'
kubectl -n argocd get application <app> -o jsonpath='{range .status.resources[*]}{.kind}/{.name} {.status} {.health.status}{"\n"}{end}'
```

What it means:

- `Retrying attempt #1` on a new cell's Application, with its NodePool Degraded: the NodePool
  reads Degraded for about 10 seconds while its NodeClass validates, the first sync fails that
  wave, and Argo CD retries (backoff 30 seconds, factor 2). It healed in 3 to 5 minutes in both
  captured runs. Wait.
- OutOfSync right after a push: Argo CD polls Git, and the poll took 2:17 in a captured run
  (up to about 3 minutes). Wait, or refresh.
- OutOfSync that stays, with operation `Failed`: the message names the object Git could not
  apply. Fix it in Git.
- OutOfSync that flips back and forth: someone changed an object by hand and selfHeal reverts it.
- Degraded: a child resource is unhealthy, usually a Deployment whose pods are not ready. Go to
  the pods playbooks below.
- A cell's platform Applications say their chart repositories are not permitted: `projects.yaml`
  was changed but not applied again (`make gitops-bootstrap`).

Fix:

- READ ONLY: wait out a retry and say why.
- CHANGES THINGS: refresh, so Argo CD reads Git now:
  `kubectl -n argocd annotate application <app> argocd.argoproj.io/refresh=normal`.
- CHANGES THINGS: the real fix is a commit on a branch and a pull request ([project.md](project.md)).
  Never `kubectl edit` an object Argo CD owns.

Audience: "Argo CD keeps comparing the cluster with Git. When they differ it puts the cluster
back, and when it cannot yet, it retries on its own."

## ACK resource not synced

READ ONLY (always the full kind names):

```bash
kubectl -n encabulator get dbinstances.rds.services.k8s.aws,serverlesscaches.elasticache.services.k8s.aws,dbsubnetgroups.rds.services.k8s.aws
kubectl -n encabulator get dbinstances.rds.services.k8s.aws encabulator-pg \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.message}{"\n"}{end}'
kubectl -n encabulator get dbinstances.rds.services.k8s.aws encabulator-pg -o jsonpath='{.status.dbInstanceStatus}{"\n"}'
```

On the hub, the cell's AWS objects are in `encabulator-cells` (`vpcs.ec2.services.k8s.aws`,
`natgateways.ec2.services.k8s.aws`, `clusters.eks.services.k8s.aws`, `roles.iam.services.k8s.aws`, ...).

What it means:

- `ACK.ResourceSynced=True`: AWS matches the spec. Done.
- False with `ACK.Recoverable`: the AWS error is in the message and ACK retries. A NAT gateway
  `resource is pending` and private subnets `Reconciling to sync additional fields` are normal on
  a new cell and clear by themselves.
- `ACK.Terminal`: ACK will not retry; the spec has an invalid value. It changes in Git.
- An adopted cache snapshot with `ServerlessCacheNotFoundFault`: ACK reads a snapshot by its
  cache's name, and the cache is gone. Adopting by `serverlessCacheSnapshotName` alone works
  (`docs/teardown/adopt-cache-snapshot.yaml`); the object then shows `ACK.ReferencesResolved`
  Unknown, and deleting it still deletes the snapshot. Teardown work, never on stage.
- An adopted store whose spec differs from AWS: ACK modifies AWS toward Git. That is the Act 1
  demo (backup retention 7 to 14 makes RDS show Modifying); unplanned, it means Git and Terraform
  disagree on a value listed in `gitops/hub/brownfield/README.md`.
- `DependencyViolation` or `DeleteConflict`: something still uses the resource. Teardown only.

Fix: wait for Recoverable; change Terminal specs in Git (CHANGES THINGS); never delete a store
object on stage (with `adoption-policy: adopt-or-create`, a re-applied object after its store
is gone makes ACK create a new, empty one).

Audience: "ACK turns this Kubernetes object into AWS API calls and keeps calling until AWS
matches. The conditions are its notes on how that is going."

## kro instance stuck or RGD not Active

READ ONLY:

```bash
kubectl get resourcegraphdefinitions.kro.run
kubectl get encabulatorcells -n encabulator-cells                      # hub: CLUSTER and STATE columns
kubectl --context use2 -n encabulator get encabulator,encabulatorservices   # cell
kubectl -n encabulator-cells get encabulatorcells encabulator-use2 \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.message}{"\n"}{end}'
kubectl get resourcegraphdefinitions.kro.run encabulatorcell -o jsonpath='{.status.topologicalOrder}{"\n"}'
```

What it means:

- RGD not `Active`: kro rejected the definition (a schema or CEL error, named in its
  conditions), so no instance of that kind is reconciled.
- `IN_PROGRESS` with "waiting for external reference hub": the hub's Argo CD cluster Secret is
  missing, because `make gitops-register` has not run. kro still builds the cell's AWS
  resources, but Argo CD deploys nothing to the cell.
- `IN_PROGRESS` on a new cell: normal for about 20 minutes. The conditions name the resource it
  waits for, in topological order.
- An update to an existing instance fails: Job pod templates and StatefulSet
  `volumeClaimTemplates` are immutable (`gitops/app/README.md`, Operational notes).
- The cell's `Encabulator` is ACTIVE while some pods are still starting: kro marks it ACTIVE once
  its resources exist. In a captured run every workload was ready 40 seconds later.

Fix:

- CHANGES THINGS: `make gitops-register` writes the hub's cluster Secret; the cell goes on
  within seconds.
- CHANGES THINGS: fix an RGD in Git. For an immutable field, off stage: delete the Job, or the
  StatefulSet with `--cascade=orphan`, and let kro recreate it (the data stays on the PVC).

Audience: "kro reads one object, works out the order from the references between its pieces,
and creates each piece as soon as the ones it needs are ready."

## Cell stuck before ACTIVE

READ ONLY:

```bash
kubectl get encabulatorcells -n encabulator-cells
kubectl -n encabulator-cells get clusters.eks.services.k8s.aws,capabilities.eks.services.k8s.aws,natgateways.ec2.services.k8s.aws
aws eks describe-cluster --region us-east-2 --name encabulator-use2 --query cluster.status --output text
kubectl -n argocd get application hub-cells
```

What it means, against the captured runs (counted from the trigger):

| Minute | What should be true |
|---|---|
| 3 | network up (VPC, NAT gateway available) |
| 13 | the cluster ACTIVE |
| 15 to 20 | kro capability, then ACK capability |
| 20 to 24 | the cell ACTIVE and registered with Argo CD in the same second |
| 23 to 35 | Argo CD's first sync (3 to 5 minutes with retries), RDS about 5.5 minutes, the app Healthy |
| 33 to 37 | the first order delivered |

- Nothing started: `hub-cells` is not applied (`make add-cell` applies it, after checking the cell
  file is in `GIT_REPO_URL` at `GIT_REVISION`), or the push is waiting for Argo CD's Git poll.
- Cluster `CREATING`, NAT gateway pending, inside those times: normal.
- "waiting for external reference hub": see the kro playbook.
- Capacity or quota errors in ACK conditions or events: see Capacity or quota errors.

Fix on stage: if the live cell is not Healthy when the talk reaches it, switch to the prepared
fallback cell and say so (RUN-OF-SHOW, "Early start"). CHANGES THINGS: a refresh of `hub-cells`
right after the push skips the Git poll.

Audience: "Building a region is mostly waiting for AWS: the cluster alone takes about ten
minutes, and kro starts each piece the moment the one before it is ready."

## ImagePullBackOff

READ ONLY:

```bash
kubectl -n encabulator describe pod <pod> | tail -n 15     # the registry's answer is in Events
kubectl -n encabulator get pods -o jsonpath='{range .items[*]}{.spec.containers[0].image}{"\n"}{end}' | sort | uniq -c
scripts/mirror-images.sh --check                           # anonymous GHCR check, no AWS calls
kubectl --context use2 -n encabulator get encabulator encabulator -o jsonpath='{.spec.registry} {.spec.imageTag}{"\n"}'
```

What it means:

- `401 Unauthorized`, `denied`: the GHCR package is private. New packages start private; in the
  cold deploy of the public repository all 15 failed anonymous pulls until they were made public.
- `not found`, `manifest unknown`: the tag is not there. Images are tagged with `VERSION`, the
  short commit (`sha-<commit>`) and `latest`; `latest` and the version tag are pushed again on
  every merge to main, so they move.
- The image is not under `ghcr.io/cloud-native-encabulator/encabulator/` or an ECR mirror:
  `REGISTRY` in `.env` (hub) or the `encabulator.io/registry` annotation on the cell's Argo CD
  cluster Secret (cell) points elsewhere.
- Mixed tags in the count: a rollout is under way or a tag moved.

Fix:

- CHANGES THINGS: the maintainer makes the package public (the package's settings on GitHub,
  visibility). `scripts/mirror-images.sh --check` then reports every image on GHCR.
- CHANGES THINGS, off stage: `make mirror-images` builds the images into ECR in your account
  (needs Docker or Finch, creates tagged repositories) and records `REGISTRY` in `.env`.
- CHANGES THINGS: pin a tag that exists, in Git (`VERSION`, the `Encabulator` `spec.imageTag`).

Audience: "Kubernetes pulls each image from the registry by name. When the registry says no,
the pod waits and asks again, a little longer each time."

## CrashLoopBackOff of a service

READ ONLY:

```bash
kubectl -n encabulator logs <pod> --previous --tail=20
kubectl -n encabulator get pods nats-0 influxdb-0
kubectl -n encabulator get pods -l app.kubernetes.io/name=<service>
```

What it means: the log's last lines name what the service could not reach. Readiness reflects a
service's own dependencies only, so look for the dependency, not the next service.

| Log says | Dependency |
|---|---|
| `getaddrinfo EAI_AGAIN nats`, NATS connection refused | NATS (`nats-0`, in the cluster) |
| `DATABASE_URL`, connection to 5432 refused, password authentication failed | PostgreSQL (RDS on AWS) and the `encabulator-db` Secret |
| `VALKEY_URL`, 6379 | Valkey (ElastiCache Serverless on AWS: TLS, no password) |
| `INFLUXDB_URL`, 8181 | InfluxDB (`influxdb-0`) |
| `... is required` / `is not set` | a missing setting in the chart values or the RGD |
| last exit `OOMKilled` | the memory limit |

A store pod that is not ready (NATS recovering its JetStream state after a restart, for example)
makes the services that need it crash until it is back; they then recover by themselves.

Fix: READ ONLY wait for the dependency, and say so. Restarting the crashing pod does not help.
CHANGES THINGS, only once the dependency is back and the back-off is long:
`kubectl -n encabulator rollout restart deployment <service>`.

Audience: "This service exits when something it needs is missing, and Kubernetes keeps
restarting it, waiting longer each time, until that dependency is back."

## Orders not delivered

READ ONLY, in this order:

1. The console's Malaise page: an active fault (latency, errors, a partition, a service scaled
   to zero) explains most of it. Every fault expires and reverts by itself.
2. The order's status on the console's Works page (or `GET /api/v1/orders/{id}` through the
   gateway): `accepted`, `fabricating`, `inspecting`, `refabricating`, `dispatched`, `delivered`,
   `scrapped`.
3. The trace: `make jaeger`, search the service `dingle-arm-gateway`. One order is one trace:
   gateway, processor, quoter, depot, ledger, then fabrication, inspection and shipment linked
   through NATS. The last span tells you where it stopped.
4. Grafana: `make grafana`, dashboards "Encabulator Works", "Service RED" and "Encabulator
   Backing Stores".
5. The logs of the service at that stage (`kubectl -n encabulator logs deploy/<service> --tail=20`).

Where it stops:

| Status | Look at |
|---|---|
| rejected `insufficient_parts` (409) | the depot ran out: restock on the Works page or raise the `amulite-foundry-rate` flag |
| rejected 503 | quoter, depot or datastore unreachable (the processor compensates and releases parts) |
| stays `accepted` or `fabricating` | NATS, `logarithmic-casing-line`, `sinusoidal-recorder` |
| stays `inspecting` | `side-fumbling-detector` |
| `refabricating`, then `scrapped` | three failed inspections; the `strict-side-fumbling` flag raises false positives |
| `dispatched` | normal for 10 to 60 seconds of simulated transit |

The console says "no data": Prometheus is not installed, the collector receives no spans, or no
traffic has been sent yet (start it on the Grammeters page).

Fix: end the fault from the Malaise page (CHANGES THINGS, the console's own control) or let it
expire; restock from the Works page.

Audience: "Every arrow on that diagram is a real call or a real message, so the trace shows
exactly where this order stopped."

## Database connection or adoption problems

READ ONLY:

```bash
kubectl -n encabulator get dbinstances.rds.services.k8s.aws,serverlesscaches.elasticache.services.k8s.aws
kubectl -n encabulator get dbinstances.rds.services.k8s.aws encabulator-pg \
  -o jsonpath='{.metadata.annotations}{"\n"}{range .status.conditions[*]}{.type}={.status} {.message}{"\n"}{end}'
kubectl -n encabulator get configmap encabulator-aws -o yaml     # endpoints only, no secrets
kubectl -n encabulator logs job/encabulator-db-init --tail=20
```

What it means:

- The stores carry `services.k8s.aws/adoption-policy: adopt-or-create` and
  `services.k8s.aws/deletion-policy: retain`. Retain keeps RDS and the cache when their object
  is deleted. Adopt-or-create makes an object applied after its store is gone create a new,
  empty store, which is why the teardown stops Argo CD first.
- Before the hand-off Terraform owns the stores; after it (`handoff.tf` in place) Git does, and
  `terraform plan` says "No changes".
- Passwords: on the hub Terraform writes the `encabulator-db` Secret; on a cell a one-time
  `secret-init` Job creates it only if absent, and the DBInstance reads `masterUserPassword`
  from it. Never print it.
- The `encabulator-db-init` Job creates the three logical databases and roles over TLS; it is
  safe to re-run.
- ElastiCache Serverless has no AUTH: clients connect over TLS with an empty password
  (`rediss://:@<host>:6379/0`). An `AUTH` error means a client sends a password.
- RDS stays on port 5432 and ACK makes no Modify call once it settles.

Fix: CHANGES THINGS in Git only. Never delete a store object on stage.

Audience: "ACK adopted the database Terraform built without touching it. Git owns its settings
now, and deleting the Kubernetes object would leave the database running, on purpose."

## KEDA scaling

READ ONLY:

```bash
kubectl -n encabulator get scaledobjects.keda.sh,hpa
kubectl -n keda get pods
kubectl -n encabulator describe scaledobjects.keda.sh logarithmic-casing-line | tail -n 15
```

What it means: with `KEDA_ENABLED=true` the chart scales `logarithmic-casing-line` on
JetStream consumer lag (the `nats-jetstream` scaler on stream `ENCABULATOR`, 1 to 10 replicas);
otherwise a CPU HorizontalPodAutoscaler does. No ScaledObject: KEDA is off. Not Ready: KEDA
cannot read the NATS monitoring port. No scaling: there is no lag; start traffic on the
Grammeters page or a burst pattern.

Fix: CHANGES THINGS: `make deploy KEDA_ENABLED=true` on a self-managed cluster (needs
`make platform`, which installs KEDA).

Audience: "KEDA watches how many orders wait in the queue and adds casing line workers when the
queue grows, then removes them when it drains."

## Terraform state lock or an interrupted make hub

READ ONLY:

```bash
make -n hub                                   # the steps, nothing runs
make prereqs
ls -l terraform/brownfield/.terraform.tfstate.lock.info terraform/brownfield/terraform.tfstate
```

What it means:

- `make hub stopped at step "<step>"`: fix what it reports and run `make hub` again; every step
  is safe to re-run, and `make <step>` runs just that one.
- A single Ctrl-C: Terraform finishes the calls in flight and saves the state. Run `make hub`.
- The process was killed (a closed terminal, `kill -9`): what it was creating exists in AWS but
  not in the state. In one captured run that was the EKS cluster, the RDS instance and the NAT
  gateway, plus a stale lock note (`force-unlock` answers "LocalState not locked").
- A second `make hub` that wants to change the stores' tags: fixed by ignoring ACK's `eks:` tags;
  a plan after the hand-off says "No changes".

Fix, off stage: follow "If make hub was interrupted" in `terraform/brownfield/README.md`: wait
for the cluster and the instance, delete the lock note, adopt the lost resources with temporary
import blocks (a plain `terraform import` fails with "Invalid provider configuration"), correct
the state, plan, then `make hub`. All of it CHANGES THINGS. The state holds passwords: keep it at
mode 600 and never print it.

Audience: "Terraform keeps a record of what it built. If it is killed mid-build, AWS keeps
building, so we adopt those pieces back into the record instead of building them twice."

## Identity Center, Argo CD or kubectl access

READ ONLY:

```bash
make prereqs                                  # checks the Identity Center instance and the group
aws eks describe-capability --region us-west-2 --cluster-name encabulator-usw2 --capability-name argocd --query capability.status --output text
kubectl auth can-i list pods -n encabulator --context use2
```

What it means:

- Nobody can log in to Argo CD: `ARGOCD_ADMIN_GROUP_ID` must be a GROUP id; a user id gives
  nobody access. `make prereqs` says when Identity Center has no group yet.
- You can log in but see nothing: you are not a member of that group.
- The Identity Center commands print a second line `None`: they need `--no-paginate`.
- `kubectl --context use2` is refused: the cell's cluster is created without creator admin
  permissions. `make cell-access CELL=use2` adds an access entry for your AWS identity, or the
  cell file sets `spec.adminRoleName`.

Fix: CHANGES THINGS: add the presenter to the group (Identity Center console, Groups);
`make cell-access CELL=use2` (creates a tagged access entry). Log in to Argo CD before the talk.

Audience: "Argo CD signs people in with IAM Identity Center, so who may log in is a group in
Identity Center, not a password stored in the cluster."

## Capacity or quota errors

READ ONLY:

```bash
kubectl get events -A --field-selector type=Warning | grep -i capacity
kubectl get nodepools.karpenter.sh encabulator -o jsonpath='{.spec.template.spec.requirements}{"\n"}'
kubectl -n encabulator-cells get vpcs.ec2.services.k8s.aws,elasticipaddresses.ec2.services.k8s.aws,natgateways.ec2.services.k8s.aws
```

What it means:

- `InsufficientInstanceCapacity`: EC2 had no capacity for one instance type in one zone. Read the
  whole message: it ends with the zones that have capacity ("You can currently get ... by
  choosing ..."). The NodePool allows C, M and R families, generation 5 or newer, so Auto Mode
  tries other types. On a live hub it failed every 5 minutes to replace a node while every
  workload kept running: nothing moves when a replacement fails.
- Pods Pending for minutes with that error: the capacity is the cause.
- A quota on a new cell (VPCs, Elastic IPs, NAT gateways per region): the ACK object's
  conditions carry the AWS error code, for example `VpcLimitExceeded` or `AddressLimitExceeded`.

Fix: READ ONLY wait for a capacity retry. CHANGES THINGS, off stage: a quota increase, or a cell
in another region (a new file in `gitops/hub/cells/`, billable). Do not change the NodePool live.

Audience: "Auto Mode picks instances for us from a list of families, so when one type is sold
out in one zone, it simply picks another."

## GitHub Actions, CI and the images workflow

READ ONLY:

```bash
gh pr checks <number>
gh run list --workflow images.yml --limit 5
gh run view <run id> --log-failed | tail -n 40
scripts/mirror-images.sh --check
```

What it means: `ci.yml` runs on every pull request and push to main; its last job, `ci`, is the
one required check and fails if any job failed. `images.yml` runs on every push to main and on
`v*` tags: it builds all 15 services for amd64 and arm64 and pushes `VERSION`, `sha-<commit>` and
on main `latest`, with provenance and an SBOM. So the version tag moves on every merge to main,
docs-only merges included; pin `sha-<commit>` or a digest when it must not move. Packages it
creates start private on GHCR. [project.md](project.md) says how to run each check locally.

Fix: CHANGES THINGS: push a fix to the pull request's branch; re-run a flaky job with
`gh run rerun <run id> --failed`.

Audience: "Every merge builds and signs all fifteen images, and the cluster pulls exactly the
tag Git names."

## make target failures

READ ONLY: run it again with `make -n <target>` to see the steps, and read the one sentence it
stopped with. Most targets name the fix.

- `make prereqs` lists every problem at once, each with its fix; it is read-only.
- `'terraform version' fails`: Terraform comes from a version manager. Export
  `TERRAFORM="mise exec terraform@1.14.9 -- terraform"` in the shell or put it on the make
  command line; `.env` does not read it.
- `mirror-images: <runtime> is not installed`: set `CONTAINER_RUNTIME=docker` or `finch` in `.env`
  (only needed when images must be mirrored).
- `make tag-check` right after a change names a network interface or endpoint: the tagger has
  not passed yet; run it again 10 minutes later. A stack that is not built is reported `absent`
  and is not a failure; exit 3 means nothing was found at all (check the account and region).
- `register-hub.sh` refuses: kubectl points at another cluster (`make kubeconfig`), or the
  cache's subnets differ from the DB subnet group's.
- `make add-cell` says the cell file is not in the repository: push it to `GIT_REVISION` first.

Audience: "Every step can be run on its own and again, so when one stops, we fix that one thing
and carry on."

## Cluster unreachable

`make ops-health` exits 2 with the error. On stage, say it in one line and go to the captured
runs ([stage-rules.md](stage-rules.md), If the cluster is unreachable). Off stage: refresh AWS
credentials in your shell, then `make kubeconfig` (hub) or the command `make cell-access` prints
(cell).

Audience: "The cluster is fine; my laptop lost its credentials. Here is the same step from a
run we captured earlier."
