# Talk day: where are we, and what runs next

Use this when the presenter asks "what do I run next?" on the day of OPN315, or in a rehearsal.
Find the stage from the cluster with the read-only checks below, then give the next step from
[docs/opn315/COMMANDS.md](../../../../docs/opn315/COMMANDS.md), the cheat sheet the presenter
copies from. The timings and the reasons are in
[docs/opn315/RUN-OF-SHOW.md](../../../../docs/opn315/RUN-OF-SHOW.md). The stage rules hold
throughout: say what you check, a few lines per answer, ask before anything that changes things.

Repository and branch: Argo CD on the hub syncs `https://github.com/cloud-native-encabulator/encabulator`
at `main` (`GIT_REPO_URL`, `GIT_REVISION` in `.env`). A live push only counts if it lands there.

## Find the stage (read only)

```bash
make prereqs                                                   # tools, credentials, .env
aws eks list-clusters --region us-west-2 --query clusters --output text   # encabulator-usw2?
make ops-health                                                # the hub, once it exists
kubectl -n argocd get applications                             # which hub apps exist
kubectl -n encabulator get dbinstances.rds                     # always .rds
kubectl -n encabulator-cells get encabulatorcells              # a cell building or ACTIVE?
grep -n postgres gitops/hub/brownfield/kustomization.yaml      # in the revision Argo CD reads?
```

Also check for a `make hub` still running in a terminal before changing files in the checkout:
it reads the working tree (`terraform/brownfield`, the bootstrap files). Edit in a separate
`git worktree` while it runs.

## Stages, in order

| Stage | You can tell by | Next |
|---|---|---|
| 0. No hub | `encabulator-usw2` not in `list-clusters` | `make prereqs`, then `make hub` (CHANGES THINGS: billable, about 25 minutes, asks first) |
| 1. Hub building | `make hub` running; it prints the step it is on | wait; on `make hub stopped at step "<x>"`, read the error, fix, run `make hub` again (every step re-runs safely) |
| 2. Hub up | `make hub` printed "The hub is up"; `hub-brownfield`, `hub-rgds`, `hub-cell-apps` in `argocd`; no `hub-cells` | `make ops-health`; `make console` (off screen: `make credentials`); log in to Argo CD; open the RDS console |
| 3. Ready for the live adopt | `encabulator-pg` has no DBInstance, and `kustomization.yaml` on `main` has no `postgres.yaml` line | nothing; this is the Act 1 starting point |
| 3a. Adopted too early | a DBInstance `encabulator-pg` exists before Act 1 | tell the presenter: Act 1 is shown as "prepared earlier", or the presenter decides how to reset it (never delete a store object on stage) |
| 4. Ready for the trigger | no `hub-cells` Application, `gitops/hub/cells/use2.yaml` present on `main` | at 8:00: `make add-cell CELL=use2` (CHANGES THINGS: builds the cell, billable); or the live push, if `use2.yaml` was taken off `main` the day before |
| 5. Cell building | `encabulatorcells` IN_PROGRESS | nothing to do; ACTIVE about 20 minutes after the trigger, app Healthy 9 to 12 minutes later; IN_PROGRESS with "waiting for external reference hub" means `make gitops-register` was not run |
| 6. Cell ACTIVE | `encabulator-use2` in `kubectl -n argocd get applications` | `make cell-access CELL=use2` (CHANGES THINGS: an access entry), then `aws eks update-kubeconfig --region us-east-2 --name encabulator-use2 --alias use2` |
| 7. Before going on stage | all of the above green | `make tag-check` (10+ minutes after the last change, must exit 0); `export AWS_REGION=us-west-2` in the stage terminal; `make talk` |

## On stage, act by act

Give the commands from the matching section of `docs/opn315/COMMANDS.md`, one at a time, and
check the result before the next:

- 8:00, the trigger (stage 4).
- 13:00 to 24:00, Act 1 adopt. Order matters: apply `gitops/explore/postgres.yaml`, read the
  spec, delete the explore manifest, then commit with the `postgres.yaml` line added back to
  `kustomization.yaml`, push to `main`, refresh `hub-brownfield`, wait for the DBInstance to report
  synced, and only then `terraform -chdir=terraform/brownfield apply`.
- 26:00 to 40:00, Act 2: `kubectl apply -f gitops/app/rgds/`; the instance commands need
  `--context use2`.
- 40:00 to 53:00, Act 3: read-only commands against the hub and the cell.
- 53:00 to 56:00, Day 2: delete one fixed-replica Deployment on the cell; change
  `dbInstanceClass` in `gitops/app/instance/encabulator.yaml` and push.

## Known traps

- `kubectl get dbinstances` resolves to DocumentDB: type `dbinstances.rds`.
- `make gitops-bootstrap` never applies `hub-cells`; that is why `use2` does not build on its own.
- Argo CD's first sync of a new cell shows "Retrying attempt #1" for 3 to 5 minutes; it heals.
- Git is polled every 3 minutes; refresh the Application right after a live push.
- Image tag: `VERSION`, the chart and the RGD say `1.0.4`. Some docs still say `1.0.3`; trust the files.
