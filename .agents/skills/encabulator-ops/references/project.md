# Working on the GitHub project

The repository is `cloud-native-encabulator/encabulator` on GitHub, default branch `main`,
Apache 2.0. [CONTRIBUTING.md](../../../../CONTRIBUTING.md) is the source for its conventions;
this page is the short form for an agent.

## Layout

See [map.md](map.md), "Where things live". `docs/design.md` is the contract: a change to an API,
an event, a metric, a flag or a convention updates it in the same commit. The service list comes
from `scripts/services.txt` and nowhere else.

## Branches, commits, pull requests

- Work on a branch, never on `main`. Push with `git push -u origin <branch>`.
- Every commit is signed off for the DCO: `git commit -s`, which adds
  `Signed-off-by: <name> <email>` from your git config.
- Commit subjects in the imperative, with a body that says why when it is not obvious.
- Prose: no em dashes; use a comma, a colon, a semicolon or two sentences.
- Open the pull request with `gh pr create --title "..." --body-file <file>`, and say what changed
  and how it was tested.
- The repository allows rebase and squash merges only. Once `ci` is green, merge with
  `gh pr merge <number> --rebase`, which keeps each signed-off commit as it is.
- After the merge the branch can go: `git push origin --delete <branch>` or the button on the PR.

## CI, and how to run it locally

`.github/workflows/ci.yml`, one required check named `ci`:

| Job | Locally |
|---|---|
| Go, Node, Python, Rust services and kits | `make lint` and `make test` (or `make -C services/<plane>-plane/<service> test`) |
| charts | `make lint-charts` (helm lint, helm template over value combinations, kubeconform) |
| shell | `shellcheck --severity=warning scripts/*.sh`, `scripts/make-flow_test.sh`, `scripts/teardown_test.sh`, `scripts/images-workflow_test.sh` |
| agent skills | `make skills-check` (the skill links, static checks, the health check tests) |
| gitops | `make gitops-render-check` and `make gitops-test` |

The Terraform contract tests are `make brownfield-test` (no AWS calls). The offline tests stub
aws, kubectl, helm and terraform; none needs an account or a cluster.

## The images workflow

`.github/workflows/images.yml` builds every service with the repository root as the build
context, for linux/amd64 and linux/arm64, and pushes to
`ghcr.io/cloud-native-encabulator/encabulator/<service>`:

- on a push to `main`: tags `VERSION` (from the `VERSION` file), `sha-<short commit>` and `latest`;
  a push that changes only docs (`docs/`, the skill trees, markdown no image copies; the
  `paths-ignore` list in the workflow) builds nothing;
- on a `v*` tag: the tag's version and `major.minor`, whatever the tag changes;
- by hand (`workflow_dispatch`), optionally for some services only.

Every merge to main that changes more than docs rewrites the version tag and `latest`, so anyone
who needs a fixed image pins `sha-<commit>` or a digest; a docs-only merge leaves both where they
are. Attestations are recorded because the repository is public:
`gh attestation verify oci://ghcr.io/cloud-native-encabulator/encabulator/<service>:<tag> --repo cloud-native-encabulator/encabulator`.
A new GHCR package starts private, and the cluster pulls anonymously, so a new service's package
has to be made public once. The workflow's last job, `every image pulls anonymously`, runs
`scripts/mirror-images.sh --check` with no credential for the version the run built, and fails
the run while any package is private. The fix is in the package's settings on GitHub (change its
visibility to Public; there is no API for it), then re-run the job.
`scripts/images-workflow_test.sh` tests the triggers and that job offline.

## Propose a fix live

Ask first: a branch and a pull request are visible to everyone. Then, in small steps:

```bash
git switch -c fix-<what>                     # CHANGES THINGS: a local branch
# edit the file in Git that owns the setting (never kubectl edit what Argo CD owns)
git diff                                     # show the change on screen
git commit -s -m "Fix <what>"
git push -u origin fix-<what>                # CHANGES THINGS: a public branch
gh pr create --title "Fix <what>" --body "Why, and what it changes."
gh pr checks --watch
```

Argo CD syncs `main` (or the revision `GIT_REVISION` names), so the cluster changes only after
the merge, or after pointing a test hub at the branch. Say so on stage.

## Read a failing check

```bash
gh pr checks <number>                          # which job is red
gh run view <run id> --log-failed | tail -n 40 # its last lines
```

Find the first error, not the last line; reproduce it with the local command in the table above,
fix it on the same branch and push again. The `ci` job only aggregates: open the job it names.
