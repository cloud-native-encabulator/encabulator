---
name: encabulator-ops
description: Operate, explain and troubleshoot the Cloud Native Encabulator and its GitHub project, live on stage or at a desk. Use when asked whether the demo is healthy, why the hub or a cell is broken, or how something works. Covers Argo CD OutOfSync or Degraded, an ACK resource not synced (RDS, ElastiCache, IAM, EC2, EKS), a kro ResourceGraphDefinition or instance stuck, a cell not ACTIVE, ImagePullBackOff, CrashLoopBackOff, orders not delivered, KEDA scaling, make hub, make add-cell or another make target failing, a Terraform state lock, Identity Center or Argo CD login, EC2 capacity, a failing CI check or the images workflow. Also use to explain EKS Auto Mode, EKS Capabilities, Argo CD, ACK and kro to an audience. Reads first, answers in a few lines, and asks before any change.
---

# encabulator-ops

Help the presenter keep the Encabulator demo running in front of an audience: find what is
wrong, explain it in plain words, and propose the safest fix. The same rules make it a good
desk companion.

## Stage rules (always)

1. Read first. Before each command, say in one line what you are checking.
2. Answer in a few short lines the audience can follow. No walls of output.
3. Never print a secret or password. `make credentials` prints them; never run it on the shared
   screen and never echo what it prints.
4. Show 111122223333 in place of any account id, in commands and in output.
5. Ask the presenter before any command that changes the cluster or AWS, and say what it changes.
6. Prefer the GitOps path: a change in Git that Argo CD syncs, not `kubectl edit`.
7. Never run `make teardown`, `make remove-cell` or any delete on stage.
8. Every AWS resource carries the tag `auto-delete=no`; anything you propose to create must too.
9. If the cluster is unreachable, the talk goes on with the captured runs.

The reasons, the answer shape and the fallback are in
[references/stage-rules.md](references/stage-rules.md).

## First move

From the repository root, read-only, about 5 seconds:

```bash
make ops-health                                  # one screen: OK, INFO, WARN, FAIL and a VERDICT
make ops-health OPS_HEALTH_ARGS=--explain        # plus one line per finding saying what it means
make ops-health OPS_HEALTH_ARGS="--context use2" # a cell
```

`make ops-health` runs [scripts/ops-health.sh](scripts/ops-health.sh) from this skill. It only
runs `kubectl get` and `aws eks describe` or `list`, never reads a Secret, and masks account
ids. The script exits 0 with no FAIL, 1 with at least one, 2 when the cluster is unreachable;
`make ops-health` exits 0 for the first two, so read its VERDICT line.

## Then pick the playbook

| The screen says | Playbook in [references/playbooks.md](references/playbooks.md) |
|---|---|
| `argocd` OutOfSync, Degraded or retrying | Argo CD Application OutOfSync or Degraded |
| `ack` not synced, Terminal, ServerlessCacheNotFoundFault | ACK resource not synced |
| `kro` RGD not Active, instance IN_PROGRESS | kro instance stuck or RGD not Active |
| a cell IN_PROGRESS on the hub | Cell stuck before ACTIVE |
| `pods` ImagePullBackOff or ErrImagePull | ImagePullBackOff |
| `pods` CrashLoopBackOff | CrashLoopBackOff of a service |
| all green, but orders do not arrive | Orders not delivered |
| database errors in a service log | Database connection or adoption problems |
| the casing line does not scale | KEDA scaling |
| `make hub` stopped or Terraform is locked | Terraform state lock or an interrupted make hub |
| cannot log in to Argo CD, kubectl refused on a cell | Identity Center, Argo CD or kubectl access |
| `events` InsufficientInstanceCapacity, a quota | Capacity or quota errors |
| a red check on a pull request, images missing | GitHub Actions, CI and the images workflow |
| any other `make` target failing | make target failures |
| `cluster` unreachable | Cluster unreachable |

## Answer shape

```
Checking: <one line>
Found: <one or two lines, in the audience's words>
Next: <READ ONLY step, or CHANGES THINGS: what it changes, then ask "Shall I?">
```

## More

- [references/map.md](references/map.md): the architecture on one screen, where each piece lives,
  every make target and `.env` key, namespaces, contexts and ports, and two sentences per
  component for the audience.
- [references/project.md](references/project.md): working on the GitHub project: branches, DCO
  sign-off, CI and how to run it locally, the images workflow, proposing a fix live as a PR.
- [fixtures/fire-drill/README.md](fixtures/fire-drill/README.md): canned incidents to rehearse with.

This folder is the only copy. Claude Code and Kiro read it through `.claude/skills` and
`.kiro/skills`, which are symbolic links to `.agents/skills`; `scripts/check-agent-skills.sh`
checks them.
