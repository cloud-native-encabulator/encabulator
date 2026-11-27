# OPN315 commands, in the order you type them

Repo Argo CD syncs: https://github.com/cloud-native-encabulator/encabulator
Branch: main (.env has GIT_REPO_URL and GIT_REVISION=main; every hub Application tracks main).
Run everything from the repo root unless a line says otherwise.

## A. Day before (hub)

```sh
make prereqs                                    # read-only: tools, AWS creds, .env
make hub                                        # asks first; about 25 min; or by hand:
#   cd terraform/brownfield && terraform apply
#   $(terraform output -raw kubeconfig_command)
#   make platform && ./deploy-app.sh
for c in argocd ack kro; do aws eks describe-capability --cluster-name encabulator-usw2 --capability-name $c --query capability.status; done   # ACTIVE x3
kubectl config set-context --current --namespace encabulator
make gitops-register REGISTER_ARGS=--dry-run    # look at the Secret
make gitops-register                            # REQUIRED before any cell: kro reads the hub Secret
make gitops-bootstrap                           # projects + hub apps, NOT hub-cells
kubectl get applications -n argocd              # hub-brownfield, hub-rgds, hub-cell-apps
```

## B. The use2 trigger: pick ONE way

hub-cells is never applied by `make gitops-bootstrap`. That is why use2 does not sync.
use2.yaml is also already committed on main, so there is no new commit to push at 8:00.

Way 1, `make add-cell` (simplest, no push story):

```sh
make add-cell CELL=use2                         # applies hub-cells; checks use2.yaml is on main
kubectl get encabulatorcells -n encabulator-cells -w
```

Way 2, keep the deck's "push at 8:00" story (needs one prep commit):

```sh
# day before, on main
git rm gitops/hub/cells/use2.yaml
git commit -s -m "Remove use2 for the talk"; git push origin main
kubectl apply -f gitops/hub/bootstrap/hub-cells.yaml    # tracks main, folder now empty, builds nothing
# at 8:00 on stage
git checkout HEAD~1 -- gitops/hub/cells/use2.yaml       # or restore the file from your saved copy
git add gitops/hub/cells/use2.yaml
git commit -s -m "Add the us-east-2 cell"; git push origin main
kubectl -n argocd annotate application hub-cells argocd.argoproj.io/refresh=hard --overwrite   # skip the ~3 min Git poll
```

Either way, second terminal right after the trigger (only if you use the image mirror):

```sh
kubectl -n argocd wait --for=create secret/encabulator-use2 --timeout=40m && kubectl -n argocd annotate secret encabulator-use2 encabulator.io/registry=<account>.dkr.ecr.us-west-2.amazonaws.com/encabulator
```

## C. 30 minutes before

```sh
make ops-health
make tag-check                                  # exit 0; run 10+ min after your last change
make status
make cell-access CELL=use2                      # optional: kubectl on the cell for your role
aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2   # fallback cell
make talk                                       # deck on http://localhost:DECK_PORT, press P
```

Open: Argo CD UI, RDS console, the console (port-forward), phone hotspot.

## D. On stage

### 8:00 Act 1 start: trigger (section B), then

```sh
kubectl -n argocd get applications
aws eks describe-capability --cluster-name encabulator-usw2 --capability-name kro --query capability.status   # 11:00
```

### 13:00-24:00 Adopt

```sh
kubectl apply -f gitops/explore/postgres.yaml
kubectl get dbinstances.rds -w                  # type dbinstances.rds, never dbinstances
kubectl get dbinstances.rds encabulator-pg -o yaml | yq .spec
kubectl delete -f gitops/explore/postgres.yaml  # BEFORE the commit syncs
git add gitops/hub/brownfield/
git -C terraform/brownfield rm stores.tf
git -C terraform/brownfield mv handoff.tf.example handoff.tf
git commit -s -m "Adopt encabulator-pg"; git push origin main
# Argo CD UI: refresh hub-brownfield; wait until the DBInstance is synced
terraform -chdir=terraform/brownfield apply     # 3 leave state, 0 destroyed
terraform -chdir=terraform/brownfield plan      # No changes
```

Prepare first: `gitops/hub/brownfield/kustomization.yaml` must NOT list postgres.yaml
on the revision Argo CD tracks before this step (run of show, decision 2). On main it does,
so remove that line the day before and add it back live.

### 26:00-40:00 Package

```sh
kubectl apply -f gitops/app/rgds/
kubectl get rgd
kubectl explain encabulator.spec
kubectl --context use2 get encabulatorservices -n encabulator
kubectl --context use2 -n encabulator get encabulator encabulator -o yaml | yq .status
```

### 40:00-53:00 Replicate (reads what the trigger started)

```sh
kubectl get encabulatorcells -n encabulator-cells -w
kubectl get clusters.eks.services.k8s.aws,capabilities.eks.services.k8s.aws -n encabulator-cells
kubectl get secrets -n argocd -l encabulator.io/cell=true
kubectl get applications -n argocd
aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2
kubectl --context use2 get encabulator,dbinstances.rds,serverlesscaches -n encabulator
```

### 53:00-56:00 Day 2

```sh
kubectl --context use2 -n encabulator get deploy
kubectl --context use2 -n encabulator delete deployment <fixed-replica service>
# edit dbInstanceClass in gitops/app/instance/encabulator.yaml (now db.t4g.medium)
git commit -s -am "Change the DB class"; git push origin main
kubectl --context use2 get dbinstances.rds -n encabulator -w     # Modifying
```
