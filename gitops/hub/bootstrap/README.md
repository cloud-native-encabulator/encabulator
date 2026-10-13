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
| `hub-appset.yaml` | Application `hub-appset` | `gitops/hub/appset.yaml` only | hub, ns `argocd` |
| `register-hub.sh` | Secret `encabulator-usw2` in `argocd` (not committed) | - | - |

All four sync from `https://github.com/jlbutler/cloud-native-encabulator` at `carrlos`, with
automated sync, prune, selfHeal and retry. Destinations use the Argo CD cluster name
`encabulator-usw2`, so no cluster ARN or account id is in Git. The cell ApplicationSet
(`gitops/hub/appset.yaml`) uses project `encabulator-cells`, which can deploy to any
`encabulator-*` cluster except the hub.

## Bring up the hub (the day before)

```bash
# 1. Terraform has applied terraform/brownfield and the three capabilities are ACTIVE.
aws eks update-kubeconfig --region us-west-2 --name encabulator-usw2
kubectl config set-context --current --namespace encabulator   # the Act 1 commands assume it

# 2. Register the hub with its own Argo CD. Reads the cluster ARN, the DB subnet group's
#    subnets and the encabulator-db security group from AWS. --dry-run shows the Secret first.
./gitops/hub/bootstrap/register-hub.sh --dry-run
./gitops/hub/bootstrap/register-hub.sh

# 3. Projects, the hub-brownfield ApplicationSet and the three Applications.
kubectl apply -f gitops/hub/bootstrap/
kubectl get applications -n argocd
```

`register-hub.sh` refuses to run if kubectl points at a different cluster, or if the cache's
subnets and security group differ from the DB subnet group's (the ApplicationSet writes the same
values into both CRs).

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
  `kustomization.yaml`). The revision Argo CD tracks during the bake must not list
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
