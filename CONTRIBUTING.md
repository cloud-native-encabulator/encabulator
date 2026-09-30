# Contributing

Contributions are welcome. This is a learning and demonstration project, so clarity matters more
than cleverness, and a change that makes the system easier to understand is as valuable as a new
feature.

## Ground rules

1. **[docs/design.md](docs/design.md) is the contract.** If your change alters an API, an event, a
   metric name, a feature flag, or a convention, update the design document in the same commit. If
   the code and the design disagree, that is a bug in one of them.
2. **Every arrow must be real.** No service may fake a call to another service, and nothing in the
   console may display a number it did not measure. Missing data is shown as "no data".
3. **Every service honors malaise.** New services in the processing, data, or support planes apply
   the fault-injection middleware from their language's kit.
4. **The jargon stays funny; the identifiers stay clear.** Encabulator vocabulary belongs in
   service names, READMEs, UI labels, scenario narration, and the occasional log message. Function
   and variable names describe what the code does.

## Developer Certificate of Origin

All commits must be signed off, certifying that you wrote the patch or have the right to submit it
under the Apache 2.0 license. See [developercertificate.org](https://developercertificate.org/).

```bash
git commit -s -m "Your commit message"
```

That appends a `Signed-off-by:` line using your `user.name` and `user.email`.

## Before you open a pull request

```bash
make lint      # all four languages plus helm lint
make test      # unit tests everywhere
```

Integration tests need a container runtime. With [Finch](https://runfinch.com/):

```bash
source dev-env.sh
make test-integration
```

If you touched the Helm chart:

```bash
make chart-validate    # helm lint, helm template across value combinations, kubeconform
```

## Adding a service

The system is deliberately large, and more moving parts make for a better demonstration. To add
one:

1. Write its section in `docs/design.md` first: purpose, API, events it publishes and consumes,
   metrics, environment variables, and which plane it belongs to.
2. Create `services/<plane>-plane/<name>/` with a `README.md`, `Dockerfile` (build context is the
   repository root), `Makefile` (`build`, `test`, `lint`, `run`), and tests.
3. Use your language's kit from `libs/` so you inherit logging, tracing, metrics, health endpoints,
   flags, and malaise for free.
4. Add it to the `services:` map in `charts/encabulator/values.yaml`.
5. Add it to the topology registry in the console and to the CI service matrix.
6. Add its terms to [docs/GLOSSARY.md](GLOSSARY.md) with a source citation, and pick a name from
   genuine Encabulator source material where you can. `docs/GLOSSARY.md` lists the canonical
   sources.

## Style

- **Go**: `gofmt`, `go vet`. Standard library HTTP routing; no third-party router.
- **TypeScript**: strict mode, ESM, no `any` without a comment explaining why.
- **Python**: typed, `ruff` for lint and format, `uv` for dependencies.
- **Rust**: `cargo fmt`, `cargo clippy -- -D warnings`.
- **Commit messages**: imperative mood, a concise subject line, and a body explaining why when the
  reason is not obvious.
- **Prose**: no em-dashes. Use a semicolon, a comma, a colon, or two sentences.

## Reporting problems

Open an issue describing what you expected, what happened, and how to reproduce it. If it is a
deployment problem, include the output of `make status` and the relevant pod logs.
