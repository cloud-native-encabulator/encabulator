# AGENTS.md

Instructions for AI coding agents (Claude Code, Codex, Kiro and others) working in this
repository. People: start with [README.md](README.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

## What this is

The Cloud Native Encabulator: fifteen services in Go, TypeScript, Python and Rust, four
backing stores, tracing and a console, deployed five ways from self-managed Helm to a GitOps hub
on EKS Auto Mode that builds whole regions (cells) with Argo CD, ACK and kro.

| Path | What |
|---|---|
| `docs/design.md` | the contract: APIs, events, flags, metrics; update it with any such change |
| `services/<plane>-plane/<service>/` | the services; `scripts/services.txt` is the only list of them |
| `libs/` | the shared kits per language |
| `charts/encabulator/`, `platform/` | the Helm chart; the observability platform and KEDA |
| `terraform/brownfield/` | the hub: VPC, EKS Auto Mode, RDS, ElastiCache, the EKS capabilities |
| `gitops/hub/`, `gitops/app/` | Argo CD bootstrap, ACK adoption, the cell RGD, the app as kro APIs |
| `docs/opn315/` | the re:Invent talk and its run of show |
| `.agents/skills/` | agent skills; `.claude/skills` and `.kiro/skills` are symbolic links to it |

## Make targets

`make help` lists them all. The ones you will use most:

- Code: `make build`, `make test`, `make lint`, `make lint-charts`.
- Offline checks CI runs: `make gitops-render-check`, `make gitops-test`, `make skills-check`,
  `scripts/make-flow_test.sh`, `scripts/teardown_test.sh`, `scripts/images-workflow_test.sh`.
- Read-only on a live deployment: `make ops-health`, `make status`, `make prereqs`,
  `make tag-check`, `make gone-check`.
- They change or create things (ask first): `make hub`, `make add-cell`, `make deploy`,
  `make platform`, `make gitops-register`, `make gitops-bootstrap`, `make mirror-images`, and the
  teardown targets `make remove-cell`, `make teardown`, `make mirror-images-delete`.

## Rules

- Read before you change anything, and ask before any command that changes a cluster or an AWS
  account. Prefer a change in Git that Argo CD syncs over `kubectl edit`.
- Never print secrets: no `make credentials` output, Secret data, `.env`, tokens or Terraform
  state in what you show. Write account ids as 111122223333.
- Every AWS resource carries `auto-delete=no`, `project=encabulator` and `talk=opn315`
  (`docs/TAGGING.md`); `make tag-check` proves it.
- Commits are signed off (`git commit -s`); work on a branch and open a pull request. The
  repository allows rebase and squash merges only: once `ci` is green, merge with
  `gh pr merge <number> --rebase`. No em dashes in prose.
- Skills live once, in `.agents/skills`. Keep `.claude/skills` and `.kiro/skills` as symbolic
  links to it (`make skills-check` and CI check them); never copy a skill into them.

## Use the encabulator-ops skill

For anything about the running deployment or the GitHub project (is it healthy, why is a hub or
cell broken, how does a piece work, how to explain it, a failing check), use the
`encabulator-ops` skill in `.agents/skills/encabulator-ops`. It starts with `make ops-health`
and has a playbook per symptom. Its rules are written for a live demo in front of an audience:
short answers, read-only first, ask before every change.
