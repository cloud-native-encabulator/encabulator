# Stage rules

The demo runs live in front of an audience. Every answer is on the big screen, so it has to be
calm, short, safe and correct. These rules hold off stage too.

## The rules and why

1. **Read first, and say what you are checking.** One line before each command, for example
   "Checking which Argo CD Applications are not Synced." The audience follows along, and the
   presenter can stop you before a wrong turn. Start with `make ops-health`.
2. **A few lines per answer.** Summarize output; quote only the line that matters. If a command
   prints more than a screen, filter it (`-o jsonpath`, `--tail=20`, `| head`).
3. **Never print a secret.** The bootstrap admin and operator passwords and the Grafana admin
   password come from `make credentials`; the presenter runs it off screen. Do not read Secret
   data (`kubectl get secret -o yaml`, `-o jsonpath='{.data...}'`), do not `cat` `.env`, Terraform
   state or a token file, and do not print environment variables that hold credentials. The
   Terraform state holds the generated passwords and stays at mode 600.
4. **Mask the account id.** In anything shown, write 111122223333 for the 12-digit account id:
   ARNs, ECR registry hosts (`111122223333.dkr.ecr.us-west-2.amazonaws.com/encabulator`), error
   messages. `make ops-health` masks for you; when you run kubectl or aws yourself, prefer
   `--query` and `-o jsonpath` that leave the id out, or pipe through
   `sed -E 's/[0-9]{12}/111122223333/g'`.
5. **Ask before any change.** Mark every proposed command READ ONLY or CHANGES THINGS. For a
   change, say what it changes and how to undo it, then ask "Shall I?" and wait for the
   presenter's yes. Changes include `kubectl apply`, `annotate`, `label`, `patch`, `scale`,
   `delete`, `rollout restart`, every `make` target that is not marked read-only in
   [map.md](map.md), `git push`, and every AWS call that is not a describe, list or get.
6. **GitOps first.** Argo CD syncs with selfHeal, so a `kubectl edit` of an object it owns is
   reverted within seconds and teaches the audience the wrong lesson. The fix is a commit (a
   branch and a pull request, see [project.md](project.md)) that Argo CD syncs. A refresh
   (`kubectl -n argocd annotate application <app> argocd.argoproj.io/refresh=normal`) is a small
   CHANGES THINGS step that only makes Argo CD read Git now.
7. **Never tear down on stage.** No `make teardown`, `make remove-cell`, `make undeploy`,
   `make platform-uninstall`, `make mirror-images-delete`, `kubectl delete` of a store object, an
   `EncabulatorCell`, an Application or an ApplicationSet. Deleting in the wrong order leaves AWS
   resources behind or, with `adoption-policy: adopt-or-create`, makes ACK create a new, empty
   database. Malaise faults are the exception: they are meant to be started and ended from the
   console, and they revert on their own.
8. **Tag everything.** Every AWS resource of the project carries `auto-delete=no`,
   `project=encabulator` and `talk=opn315` ([docs/TAGGING.md](../../../../docs/TAGGING.md)).
   Anything you propose to create carries them at create time, and `make tag-check` (read-only)
   proves it about 10 minutes later, after the tagger's next pass.
9. **If the cluster is unreachable, the talk goes on.** See below.

## Answer shape

```
Checking: which pods in encabulator are not ready.
Found: the gateway cannot pull its image; the registry answers 401, so the package is private.
Next: CHANGES THINGS: the maintainer sets the GHCR package to public in its GitHub settings.
      Until then the rest of the factory keeps running. Shall I show the read-only image check?
```

Keep the audience sentence free of jargon the slide has not introduced. Each playbook ends
with one you can use as is.

## Words that calm a room

- "This one heals on its own; here is how we can tell." (Argo CD retries, ACK pending, a cell
  still IN_PROGRESS inside its normal time.)
- "Nothing is lost: the database keeps running while we look." (`deletion-policy: retain`.)
- "That is a fault we injected on purpose; it expires by itself." (Malaise.)

## If the cluster is unreachable

`make ops-health` exits 2 and names the error (expired credentials, the wrong context, the
network). Do not debug credentials on stage. Say what happened in one line and switch to what
the repository already ships:

- The deck, served by `make talk` (localhost only): the "Where the time goes" slide shows a
  captured run of a cell build, step by step, and the IDE slides show the files.
- [docs/opn315/RUN-OF-SHOW.md](../../../../docs/opn315/RUN-OF-SHOW.md): the Act 3 timings from
  two captured runs, and "Verified on AWS", what was proven on a real hub and cell.
- The measured table in the top-level [README.md](../../../../README.md) ("This path has been run
  end to end in a fresh account"): `make hub` 24 minutes, the cell ACTIVE in 23, its
  Applications Healthy in 31, the first order delivered in 33.
- The prepared fallback cell, when only the live cell is in trouble (RUN-OF-SHOW, "Early
  start"): switch to it and say so.
- [fixtures/fire-drill](../fixtures/fire-drill/README.md): canned states of a healthy hub and of
  every incident here, which `make ops-health` can read through the fire drill runner.

Off stage, fix access with `make kubeconfig` (hub) or the command `make cell-access CELL=use2`
prints (cell), after refreshing AWS credentials in your shell.
