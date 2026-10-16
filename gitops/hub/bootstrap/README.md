# gitops/hub/bootstrap

What the hub's Argo CD (EKS Argo CD capability on `encabulator-usw2`) needs before it can take
over: two AppProjects, the `hub-brownfield` ApplicationSet, three Applications, and a script that
registers the hub with its own Argo CD.

| File | Object | Syncs | Destination |
|---|---|---|---|
| `projects.yaml` | AppProjects `encabulator-hub`, `encabulator-cells` | - | - |
| `hub-brownfield.yaml` | ApplicationSet `hub-brownfield`, renders Application `hub-brownfield` | `gitops/hub/brownfield` | hub, ns `encabulator` |
| `hub-rgds.yaml` | Application `hub-rgds` | `gitops/hub/rgds` + `gitops/hub/rbac` | hub |
| `hub-cells.yaml` | Application `hub-cells` | `gitops/hub/cells` | hub, ns `encabulator-cells` |
| `hub-cell-apps.yaml` | Application `hub-cell-apps` | `gitops/hub/appset.yaml` only: the cell ApplicationSets `encabulator` (the app) and `encabulator-platform` (the observability platform) | hub, ns `argocd` |
| `register-hub.sh` | Secret `encabulator-usw2` in `argocd` (not committed) | - | - |

All four sync from `https://github.com/cloud-native-encabulator/encabulator` at `main`, with
automated sync, prune, selfHeal and retry. Another repository or branch needs no edit to these
files: see [Deploy into your own account](#deploy-into-your-own-account). Destinations use the
Argo CD cluster name `encabulator-usw2`, so no cluster ARN or account id is in Git. The cell
ApplicationSets (`gitops/hub/appset.yaml`) use project `encabulator-cells`, which can deploy to
any `encabulator-*` cluster except the hub, from this repository and from the chart repositories
of the platform charts.

## Bring up the hub (the day before)

```bash
# 1. Terraform has applied terraform/brownfield and the three capabilities are ACTIVE.
aws eks update-kubeconfig --region us-west-2 --name encabulator-usw2
kubectl config set-context --current --namespace encabulator   # the Act 1 commands assume it

# 2. Register the hub with its own Argo CD. Reads the cluster ARN, the DB subnet group's
#    subnets and the encabulator-db security group from AWS. --dry-run shows the Secret first.
make gitops-register REGISTER_ARGS=--dry-run
make gitops-register

# 3. Projects, the hub-brownfield ApplicationSet and the hub Applications, except hub-cells.
make gitops-bootstrap
kubectl get applications -n argocd

# 4. When it is time to build the cells in gitops/hub/cells/ (a second EKS cluster, a NAT
#    gateway and a database per cell; about 35 minutes; costs money):
make add-cell CELL=use2
```

`register-hub.sh` refuses to run if kubectl points at a different cluster, or if the cache's
subnets and security group differ from the DB subnet group's (the ApplicationSet writes the same
values into both CRs).

`projects.yaml` is not synced by any Application: after changing it, apply it again with
`make gitops-bootstrap`. Until it is applied, a cell's platform Applications report that their
chart repositories are not permitted in the project, and the app's Application is unaffected.

## Deploy into your own account

The repository Argo CD syncs from, its branch, and the registry the cells pull images from are
settings, not edits. They come from the environment (the `Makefile` reads them from `.env`), and
each is optional:

| Setting | Default | What it changes |
|---|---|---|
| `GIT_REPO_URL` | `https://github.com/cloud-native-encabulator/encabulator` | the repository of every Application on the hub and on every cell, and the one the AppProjects allow |
| `GIT_REVISION` | `main` | the branch, tag or commit they track |
| `REGISTRY` | none: the images' own registry | the registry of every cell's app images (`Encabulator.spec.registry`), for example your account's ECR mirror |
| `GIT_TOKEN_FILE` | none: a public repository | a path to a file that holds a token for a private repository (mode 600; the token itself never goes in `.env`) |

With none of them set, every object is exactly what the committed files say.

```bash
make gitops-register       # records GIT_REPO_URL, GIT_REVISION and REGISTRY on the hub's cluster Secret
make gitops-repo-access    # private repository only: Argo CD's repository Secret from GIT_TOKEN_FILE
make gitops-bootstrap      # projects and hub Applications for GIT_REPO_URL at GIT_REVISION; no cell
make add-cell CELL=use2    # builds the cells committed to gitops/hub/cells/ at GIT_REVISION
make cell-access CELL=use2 # optional: kubectl access to that cell for your own AWS role or user
```

What each Argo CD object reads:

| Object | Applied by | Repository and revision from |
|---|---|---|
| AppProjects, `hub-rgds`, `hub-cell-apps`, `hub-brownfield`, `hub-cells` | `make gitops-bootstrap`, `make add-cell` | the settings, rendered into the files when they are applied (`make gitops-render` prints the result) |
| ApplicationSets `encabulator` and `encabulator-platform` (`gitops/hub/appset.yaml`) | Argo CD, from Git | the cell's cluster Secret: `encabulator.io/repo-url` and `encabulator.io/revision`, defaults as above |
| each cell's cluster Secret | kro, from the `EncabulatorCell` RGD | the hub's cluster Secret: kro copies `encabulator.io/repo-url`, `encabulator.io/revision` and `encabulator.io/registry` from it ([`../rgds/README.md`](../rgds/README.md#the-hubs-settings)) |

`appset.yaml` is synced from Git, so it cannot be rendered; it reads the cell's Secret instead,
and the cell's Secret carries what the hub was registered with. A cell added later by committing
a file to `gitops/hub/cells/` gets the same values, with no command.

Only settings that differ from the defaults are recorded on the hub, so a hub registered without
them, or with the default values, keeps the Secret it had. To change a setting later, change it
and run `make gitops-register gitops-bootstrap` again; a setting you remove is removed from the
hub's Secret and from every cell's Secret within seconds. `REGISTRY` set on the hub replaces an
`encabulator.io/registry` annotation added to a cell's Secret by hand; with no `REGISTRY` on the
hub, a hand-set one stays.

### Cells are off until you add one

`make gitops-bootstrap` never applies `hub-cells`, because `hub-cells` builds every cell in
`gitops/hub/cells/` straight away. `make add-cell CELL=<cell>` applies it. It first checks that
`gitops/hub/cells/<cell>.yaml` is in `GIT_REPO_URL` at `GIT_REVISION` (the revision Argo CD
reads, not just your checkout) and says so if it is not; when the repository cannot be read from
your machine, it checks your checkout and tells you to make sure the file is pushed. Then it
prints how to watch the build:

```bash
kubectl get encabulatorcells -n encabulator-cells -w
```

The cell is ACTIVE in about 20 minutes and its app is Healthy 9 to 12 minutes after that.
Running `make add-cell` again is harmless. Every cell in the folder is built, not only `CELL`.
Removing a cell takes more than deleting its file; the order is in
[Remove a cell](../../../terraform/brownfield/README.md#remove-a-cell).

A cell registers with Argo CD only once the hub's cluster Secret exists, because kro reads the
settings from it: run `make gitops-register` first. Without it, kro still builds the cell's AWS
resources, the `EncabulatorCell` stays `IN_PROGRESS` ("waiting for external reference hub"), and
Argo CD deploys nothing to the cell until the hub is registered. A running cell whose hub Secret
is deleted keeps everything it has, including its own cluster Secret, and goes back to `ACTIVE`
when the hub is registered again.

### A private repository

Put a token that can read the repository in a file, `chmod 600` it, and set `GIT_TOKEN_FILE` to
its path (`GIT_REPO_URL` must be an `https://` URL). `make gitops-repo-access` creates the Argo
CD repository Secret `repo-<repository name>` in `argocd`, labelled
`argocd.argoproj.io/secret-type: repository`, with `url` set to `GIT_REPO_URL`, user
`x-access-token` (GitHub's convention for tokens) and the file's content as the password. It
refuses a file that is empty or readable by anyone else, and never prints the token or passes it
on a command line. Run it again after rotating the token. With `GIT_TOKEN_FILE` unset it does
nothing and says so.

### kubectl on a cell

The cell's cluster grants access to the hub's Argo CD and, optionally, to the IAM role named in
the cell file (`spec.adminRoleName`). `make cell-access CELL=<cell>` gives your own AWS identity
access without putting a role name in Git: it creates an EKS access entry on the cell's cluster
for the role (with its path) or user you are signed in as, tagged `auto-delete=no`,
`project=encabulator`, `talk=opn315`, associates `AmazonEKSClusterAdminPolicy`, and prints the
`aws eks update-kubeconfig` command. When your role cannot read IAM, set `PRINCIPAL_ARN` to its
ARN. To remove the access, delete the access entry
(`aws eks delete-access-entry --cluster-name <cluster> --principal-arn <arn>`).

## What every cell gets

The `encabulator` ApplicationSet syncs `gitops/app` to each cell. The `encabulator-platform`
ApplicationSet adds the observability platform next to it, from the same pinned charts as the
hub (`platform/versions.env`) with a cell overlay (`platform/cell/`), into
`encabulator-observability`:

```bash
kubectl get applications -n argocd -l encabulator.io/platform=observability
# <cell>-prometheus-stack, <cell>-jaeger, <cell>-opentelemetry-collector, <cell>-observability-config
```

The platform Applications and the app's Application do not wait for each other. A cell opts out
with `observability: false` in its cell file; the design and the switch are in
[`../rgds/README.md`](../rgds/README.md#the-platform-applicationset).

### Adopting a cell that already runs the platform

When a cell already runs these charts with Helm under the same release names and namespace (as
`scripts/platform.sh install` does), Argo CD adopts the objects in place: server-side apply
takes over the fields, and every object keeps its identity, so no pod is recreated when the
rendered spec equals the live one. To read the diff before the first sync, hold the cell's
platform Applications in manual sync first:

```bash
kubectl -n argocd annotate secret <cell> encabulator.io/platform-sync=manual   # before the ApplicationSet exists
# ... the Applications appear without automated sync; read their diff, sync once by hand ...
kubectl -n argocd annotate secret <cell> encabulator.io/platform-sync-         # automated sync from now on
```

Helm keeps its release records (`sh.helm.release.v1.*` Secrets in the namespace). Argo CD does
not read or remove them. Once Argo CD owns the platform, do not run `helm upgrade` or
`helm uninstall` for these releases on that cell.

## Act 1 live: adopt the RDS instance (matches the deck's "IDE: adopt" slide)

```bash
kubectl apply -f gitops/explore/postgres.yaml        # adopt, no spec
kubectl get dbinstance.rds -w                        # STATUS goes available
kubectl get dbinstance.rds encabulator-pg -o yaml | yq .spec
kubectl delete -f gitops/explore/postgres.yaml       # retain: the database stays
git add gitops/hub/brownfield/
git commit -s -m "Adopt encabulator-pg"; git push
terraform -chdir=terraform/brownfield apply          # removed {} drops it from state, no destroy
```

Then change a setting from Git: `backupRetentionPeriod: 14` in
`gitops/hub/brownfield/postgres.yaml`, commit, push, and the RDS console shows Modifying.

Notes for the stage:

- The read-back spec contains `kmsKeyID` (an ARN with the account id) and `availabilityZone`.
  Do not paste it into Git. Show it, then commit the prepared `gitops/hub/brownfield/postgres.yaml`,
  which keeps only the fields Git owns.
- Delete the explore CR before the brownfield CR syncs: both are DBInstance `encabulator-pg` in
  namespace `encabulator`, so they are the same Kubernetes object.
- This branch holds the state AFTER Act 1 (postgres.yaml present and listed in
  `kustomization.yaml`). The revision Argo CD tracks during the preparation must not list
  `postgres.yaml` yet, or ACK adopts the instance the day before and the live `adopt` step collides
  with it. On stage, the commit adds the file and its line in `kustomization.yaml`.

## Why hub-brownfield is an ApplicationSet

Two of the three CRs need environment ids in their spec, and those ids must not be in Git
(see [`../brownfield/README.md`](../brownfield/README.md)). `register-hub.sh` puts them on the hub's
cluster Secret as `encabulator.io/subnet-ids` and `encabulator.io/security-group-id` (the same
keys the cells use) plus label `encabulator.io/hub: "true"`. The ApplicationSet's cluster generator
selects that label and renders Application `hub-brownfield` with `kustomize.patches` that replace
the placeholders. Argo CD appends those patches to `gitops/hub/brownfield/kustomization.yaml` at
render time (argo-cd v3.5.5 `util/kustomize/kustomize.go`), which is why that folder needs a
kustomization file.

## No sync waves

Nothing here relies on sync waves, so no Argo CD health customization for `Application` is needed.
Ordering is handled by retry instead: `hub-cells` uses `SkipDryRunOnMissingResource=true` and
retries until kro has turned the RGD from `hub-rgds` into the `EncabulatorCell` CRD. Argo CD
v3.5.5 ships no health check for `*.services.k8s.aws` kinds, so ACK CRs show no health in the UI;
read `kubectl get dbinstance.rds,serverlesscache,dbsubnetgroup.rds` for the ACK `Synced` condition.
The EKS ACK capability installs the CRDs of every ACK controller, and DocumentDB also has
`DBInstance` and `DBSubnetGroup` kinds, so a bare `kubectl get dbinstance` lists the DocumentDB
kind and shows nothing. Keep the `.rds` group suffix.

## Verified in the controller source

Read at the pinned tags (clones under the gitignored `tmp/src/`):

- `adoption-fields` key for RDS DBInstance is `dbInstanceIdentifier`:
  rds-controller v1.13.1 `pkg/resource/db_instance/resource.go` `PopulateResourceFromAnnotation`
  (line 99: `fields["dbInstanceIdentifier"]`). DBSubnetGroup uses `name`, ServerlessCache uses
  `serverlessCacheName` (elasticache-controller v1.10.1
  `pkg/resource/serverless_cache/resource.go`).
- `adopt` requires `adoption-fields`; `adopt-or-create` falls back to the spec's identifier:
  runtime v0.65.0 `pkg/runtime/reconciler.go` `handlePopulation` (line 450). Only `adopt` writes
  the AWS spec back into the CR; `adopt-or-create` marks the CR managed and then reconciles AWS
  toward the spec (`Sync`, lines 606 and 664; `setResourceManagedAndAdopted`, line 1504).
  Feature gate `ResourceAdoption` is Beta and on by default (`pkg/featuregate/features.go` line 50).
- DBInstance late-initialized fields include `KMSKeyID` and `AvailabilityZone` (rds-controller
  `generator.yaml`, `DBInstance` at line 305). `VpcSecurityGroupIds` is not late-initialized, but
  `templates/hooks/db_instance/sdk_read_many_post_set_output.go.tpl` line 98 copies observed SG ids
  into the spec only when the desired spec lists them, so leaving it out causes no drift.
- DBSubnetGroup `subnetIDs` is not late-initialized and is always copied from AWS
  (`templates/hooks/db_subnet_group/sdk_read_many_post_set_output.go.tpl` line 17), and
  `ModifyDBSubnetGroup` requires it. It must be in the spec.
- ServerlessCache has no late-initialized fields (elasticache-controller `generator.yaml` line
  387). `pkg/resource/serverless_cache/sdk.go` lines 197 and 223 copy security group and subnet ids
  from AWS, and `hooks.go` `customUpdateServerlessCache` (line 111) has no case for `SubnetIDs` and
  always requeues (line 184), so an empty field loops forever. Both must be in the spec.
- Endpoints: DBInstance `status.endpoint.address` and ServerlessCache `status.endpoint.address`
  (`status.endpoint.port`) exist in the pinned CRDs.

## Pinned versions

| What | Version | Use |
|---|---|---|
| ACK rds-controller | v1.13.1 (`helm/crds`) | CRDs for DBInstance, DBSubnetGroup; source read |
| ACK elasticache-controller | v1.10.1 (`helm/crds`) | CRD for ServerlessCache; source read |
| ACK runtime | v0.65.0 (the version both controllers build with) | adoption semantics |
| ACK ec2-controller | v1.22.0 | source read only; not installed, nothing here uses EC2 kinds |
| Argo CD | v3.5.5 (`manifests/crds`) | Application, AppProject, ApplicationSet CRDs; kustomize patch code |
| kind | v0.33.0 (`kind-linux-amd64`, sha256 `aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d`) | validation cluster |
| kind node image | `kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5` | validation cluster |
| kubectl / kustomize | v1.35.0 / v5.7.1 | dry-run, kustomize build |
| helm | v4.2.4 | renders the ApplicationSet patch templates offline (same Go template + sprig) |

The EKS capabilities run AWS-managed controller versions, which can differ from these pins.
Confirm on the hub before the talk: `kubectl get crd dbinstances.rds.services.k8s.aws -o yaml`
and the Argo CD version in the capability console.
