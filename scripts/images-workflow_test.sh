#!/usr/bin/env bash
# Offline tests for .github/workflows/images.yml and scripts/mirror-images.sh --check.
#
#   scripts/images-workflow_test.sh
#
# No network: curl is a stub that plays an anonymous GHCR. Reads the workflow with python3 and
# PyYAML, and runs its shell steps the way GitHub does (bash -eo pipefail). Covered:
#   - the workflow parses; the push trigger ignores docs-only paths and no code; the v* tag and
#     workflow_dispatch triggers are as they were
#   - no ignored path is in any image: every file a Dockerfile copies from the build context,
#     and every file the workflow itself reads, still starts a build
#   - a selection that names no service builds nothing and skips the build job, with a warning
#   - the `public` job needs the builds, runs only after they succeeded (or built nothing), and
#     holds no GHCR credential
#   - mirror-images.sh --check against the stub: every image public passes; a package whose
#     pull token is refused, or whose manifest answers 401 or 404, fails and is named; no
#     credential from the environment reaches curl
#   - the job's own step: checks the version the run built (1.2.3 for the tag v1.2.3), passes
#     when every image is public, and otherwise fails naming each package with its settings
#     link and the instruction to make it Public
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workflow="$root/.github/workflows/images.yml"
work="$(mktemp -d)"
# KEEP=1 keeps the scratch files (the path analysis is in paths) and prints where they are.
if [[ -n "${KEEP:-}" ]]; then trap 'echo "scratch files kept in $work"' EXIT; else trap 'rm -rf "$work"' EXIT; fi

pass=0 fail=0
ok()  { pass=$((pass + 1)); printf '  ok    %s\n' "$1"; }
bad() {
  fail=$((fail + 1)); printf '  FAIL  %s\n' "$1"
  [[ -z "${2:-}" ]] || printf '%s\n' "$2" | tail -n 25 | sed 's/^/          /'
}
expect_status() { if [[ "$3" == "$2" ]]; then ok "$1"; else bad "$1 (exit $3, want $2)" "$(cat "${4:-/dev/null}")"; fi; }
expect_in()     { if grep -qF -- "$3" "$2"; then ok "$1"; else bad "$1: not found: $3" "$(cat "$2")"; fi; }
expect_not_in() { if grep -qF -- "$3" "$2"; then bad "$1: found: $3" "$(grep -F -- "$3" "$2")"; else ok "$1"; fi; }
expect_eq()     { if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1" "got:  $2"$'\n'"want: $3"; fi; }
section() { printf '\n%s\n' "$1"; }

services=()
while IFS= read -r s; do services+=("$s"); done < <("$root/scripts/services.sh" names)
version="$(tr -d '[:space:]' <"$root/VERSION")"
ghcr="ghcr.io/cloud-native-encabulator/encabulator"

# The workflow as JSON. PyYAML reads the key `on` as the boolean true (YAML 1.1); put it back.
section "the workflow parses"
wf="$work/workflow.json"
if python3 - "$workflow" >"$wf" 2>"$work/parse.err" <<'EOF'
import json, sys
import yaml
with open(sys.argv[1]) as f:
    doc = yaml.safe_load(f)
if True in doc:
    doc["on"] = doc.pop(True)
json.dump(doc, sys.stdout)
EOF
then ok "images.yml is valid YAML"
else bad "images.yml is valid YAML (python3 with PyYAML reads it)" "$(cat "$work/parse.err")"; echo '{}' >"$wf"
fi
q() { jq -c "$@" "$wf"; }
qr() { jq -r "$@" "$wf"; }

# =================================================================== triggers
section "the triggers"
expect_eq "it runs on push and by hand, nothing else" "$(q '.on | keys')" '["push","workflow_dispatch"]'
expect_eq "push: main and v* tags, filtered by paths-ignore only" "$(q '.on.push | keys')" '["branches","paths-ignore","tags"]'
expect_eq "push: the branch is main" "$(q '.on.push.branches')" '["main"]'
expect_eq "push: the tags are v*" "$(q '.on.push.tags')" '["v*"]'
expect_eq "workflow_dispatch: one optional services input, as before" "$(q '.on.workflow_dispatch')" \
  '{"inputs":{"services":{"description":"Space-separated service names to build (empty means all fifteen)","required":false,"default":""}}}'
expect_eq "push: the paths-ignore list" "$(q '.on.push["paths-ignore"]')" \
  '["docs/**",".agents/**",".claude/**",".kiro/**","AGENTS.md","CLAUDE.md","README.md","CONTRIBUTING.md","charts/**/*.md","gitops/**/*.md","platform/**/*.md","terraform/**/*.md"]'

# Which tracked files paths-ignore matches, and which files a build reads. The patterns may
# use only * and ** (GitHub's ?, +, [] and ! are refused here), so the matcher below is exact:
# * is anything but a slash, ** is anything, **/ is zero or more directories.
git -C "$root" ls-files >"$work/tracked"
python3 - "$root" "$work/tracked" "$wf" >"$work/paths" 2>&1 <<'EOF'
import fnmatch, json, re, shlex, sys
from pathlib import Path

root, tracked_file, wf_file = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
tracked = [l for l in open(tracked_file).read().splitlines() if l]
patterns = json.load(open(wf_file)).get("on", {}).get("push", {}).get("paths-ignore", [])

def to_regex(p):
    if not re.fullmatch(r"[A-Za-z0-9._/*-]+", p):
        print(f"unsupported-pattern {p}")
        return None
    out, i = "", 0
    while i < len(p):
        if p.startswith("**/", i):
            out += "(?:.*/)?"; i += 3
        elif p.startswith("**", i):
            out += ".*"; i += 2
        elif p[i] == "*":
            out += "[^/]*"; i += 1
        else:
            out += re.escape(p[i]); i += 1
    return re.compile(out + r"\Z")

regexes = [(p, to_regex(p)) for p in patterns]
regexes = [(p, r) for p, r in regexes if r]
def ignored(path):
    return any(r.match(path) for _, r in regexes)

for p, r in regexes:
    if not any(r.match(t) for t in tracked):
        print(f"dead-pattern {p}")
for t in tracked:
    if ignored(t):
        print(f"ignored {t}")
# A few paths that need not exist yet, to pin the matcher to GitHub's documented examples.
for probe in ["README.md", "docs/a/b.html", "charts/README.md", "charts/x/y/z.md",
              "services/x/README.md", "libs/go/kit/README.md", "charts/x/values.yaml"]:
    print(f"probe {probe} {'ignored' if ignored(probe) else 'builds'}")

# Every Dockerfile instruction that reads the build context (the repository root): COPY and
# ADD without --from, and RUN --mount=type=bind without from=. A source with a variable in it
# cannot be resolved here and is reported, so the test fails rather than guess.
def instructions(text):
    buf = ""
    for line in text.splitlines():
        s = line.rstrip()
        if s.lstrip().startswith("#") or (not buf and not s.strip()):
            continue
        if s.endswith("\\"):
            buf += s[:-1] + " "
            continue
        yield (buf + s).strip()
        buf = ""
    if buf.strip():
        yield buf.strip()

reads = set()
dockerfiles = [t for t in tracked if t.rsplit("/", 1)[-1] == "Dockerfile"]
for df in dockerfiles:
    reads.add(df)
    for ins in instructions((root / df).read_text()):
        op = ins.split(None, 1)[0].upper()
        if op in ("COPY", "ADD"):
            if "<<" in ins:
                print(f"unresolved {df} a here-document: {ins}")
                continue
            args = shlex.split(ins, posix=True)[1:]
            flags = [a for a in args if a.startswith("--")]
            if any(f.startswith("--from=") for f in flags):
                continue
            rest = [a for a in args if not a.startswith("--")]
            if rest and rest[0].startswith("["):
                rest = json.loads(ins[ins.index("["):])
            sources = rest[:-1]
        elif op == "RUN":
            sources = []
            for m in re.findall(r"--mount=(\S+)", ins):
                opts = dict(kv.split("=", 1) for kv in m.split(",") if "=" in kv)
                if opts.get("type") == "bind" and "from" not in opts:
                    sources.append(opts.get("source", opts.get("src", ".")))
        else:
            continue
        for src in sources:
            if "$" in src:
                print(f"unresolved {df} {src}")
                continue
            src = src.strip()
            while src.startswith("./"):
                src = src[2:]
            src = src.rstrip("/")
            for t in tracked:
                if src in ("", ".") or t == src or t.startswith(src + "/") or fnmatch.fnmatchcase(t, src):
                    reads.add(t)
print(f"dockerfiles {len(dockerfiles)}")
for t in sorted(reads):
    print(f"copied {t}")
EOF
grep -E '^(unsupported-pattern|dead-pattern|unresolved|Traceback)' "$work/paths" >"$work/paths.problems" || true
expect_eq "every pattern is plain * and ** and matches a tracked file; every Dockerfile source resolves" \
  "$(cat "$work/paths.problems")" ""
grep -E '^(Traceback|  File|[A-Za-z]+Error)' "$work/paths" >"$work/paths.errors" || true
expect_eq "the path analysis ran" "$(cat "$work/paths.errors")" ""
expect_eq "it read all ${#services[@]} Dockerfiles" "$(sed -n 's/^dockerfiles //p' "$work/paths")" "${#services[@]}"
expect_eq "the matcher agrees with GitHub's examples" "$(sed -n 's/^probe //p' "$work/paths")" "$(printf '%s\n' \
  "README.md ignored" "docs/a/b.html ignored" "charts/README.md ignored" "charts/x/y/z.md ignored" \
  "services/x/README.md builds" "libs/go/kit/README.md builds" "charts/x/values.yaml builds")"

sed -n 's/^ignored //p' "$work/paths" >"$work/ignored"
sed -n 's/^copied //p' "$work/paths" >"$work/copied"
# What the workflow reads besides the build contexts: discover, the build arguments, the check.
printf '%s\n' VERSION scripts/services.sh scripts/services.txt scripts/mirror-images.sh \
  .github/workflows/images.yml >>"$work/copied"
expect_eq "no file an image copies, and no file the workflow reads, is ignored" \
  "$(sort -u "$work/copied" | comm -12 - <(sort -u "$work/ignored"))" ""
expect_eq "only docs, the agent skill trees and markdown are ignored (no code path)" \
  "$(grep -vE '^(docs/|\.agents/|\.claude/|\.kiro/)|\.md$' "$work/ignored")" ""
expect_eq "nothing under scripts/, services/, libs/ or .github/ is ignored" \
  "$(grep -E '^(scripts|services|libs|\.github)/' "$work/ignored")" ""
expect_eq "under charts/, gitops/, platform/ and terraform/ only markdown is ignored" \
  "$(grep -E '^(charts|gitops|platform|terraform)/' "$work/ignored" | grep -v '\.md$')" ""
for f in docs/design.md AGENTS.md CLAUDE.md README.md CONTRIBUTING.md \
         .agents/skills/encabulator-ops/references/project.md .claude/skills .kiro/skills \
         terraform/brownfield/README.md; do
  if grep -qxF "$f" "$work/ignored"; then ok "a change to $f alone builds nothing"
  else bad "a change to $f alone builds nothing"; fi
done
for f in libs/go/kit/README.md libs/python/encabulator-kit/README.md \
         services/data-plane/hydrocoptic-datastore/README.md; do
  if grep -qxF "$f" "$work/copied" && ! grep -qxF "$f" "$work/ignored"; then
    ok "$f is copied into an image, so a change to it builds"
  else bad "$f is copied into an image, so a change to it builds"; fi
done

# =================================================================== the jobs
section "the build job and the images job"
expect_eq "build skips an empty selection instead of failing on an empty matrix" \
  "$(qr '.jobs.build.if')" "needs.discover.outputs.services != '[]'"
expect_eq "build still pushes the discovered version" \
  "$(qr '.jobs.build.steps[] | select(.id == "meta") | .with.tags' | head -n 1)" \
  'type=raw,value=${{ needs.discover.outputs.version }}'
expect_eq "images still needs discover and build" "$(q '.jobs.images.needs')" '["discover","build"]'
expect_eq "images still always runs" "$(qr '.jobs.images.if')" "always()"

section "the public job"
expect_eq "it exists" "$(qr '.jobs.public | type')" "object"
expect_eq "it needs discover and build" "$(q '.jobs.public.needs')" '["discover","build"]'
cond="$(qr '.jobs.public.if')"
case "$cond" in
  *"!cancelled()"*"needs.discover.result == 'success'"*"needs.build.result == 'success'"*"needs.build.result == 'skipped'"*)
    ok "it runs once every build succeeded, or when nothing was selected to build" ;;
  *) bad "it runs once every build succeeded, or when nothing was selected to build" "if: $cond" ;;
esac
case "$cond" in
  *always\(\)*|*failure*) bad "it never runs after a failed build" "if: $cond" ;;
  *) ok "it never runs after a failed build" ;;
esac
expect_eq "its token can only read the repository" "$(q '.jobs.public.permissions')" '{"contents":"read"}'
q '.jobs.public' >"$work/public.json"
expect_not_in "it names no secret" "$work/public.json" "secrets."
expect_not_in "it does not pass the job token on (github.token)" "$work/public.json" "github.token"
expect_not_in "it does not pass the job token on (GITHUB_TOKEN)" "$work/public.json" "GITHUB_TOKEN"
expect_not_in "it does not log in to a registry" "$work/public.json" "login-action"
expect_eq "every action it uses is the checkout" \
  "$(qr '[.jobs.public.steps[] | select(.uses) | .uses] | unique | join(" ")')" "actions/checkout@v7"
expect_eq "the checkout keeps no credential" \
  "$(qr '.jobs.public.steps[] | select(.uses) | .with["persist-credentials"]')" "false"
expect_eq "the workflow-wide environment holds no secret" \
  "$(q '.env | to_entries | map(select((.value | tostring) | test("secrets\\.|token"; "i"))) | length')" "0"
expect_eq "the check step's environment is only the version discover chose" \
  "$(q '[.jobs.public.steps[] | select(.run) | .env] | .[0]')" '{"VERSION":"${{ needs.discover.outputs.version }}"}'
qr '[.jobs.public.steps[] | select(.run) | .run] | .[0] // ""' >"$work/public-step.sh"
expect_in "it runs scripts/mirror-images.sh --check" "$work/public-step.sh" "./scripts/mirror-images.sh --check"
qr '.jobs.discover.steps[] | select(.id == "version") | .run' >"$work/version-step.sh"
qr '.jobs.discover.steps[] | select(.id == "matrix") | .run' >"$work/matrix-step.sh"
if command -v shellcheck >/dev/null 2>&1; then
  if shellcheck --severity=warning --shell=bash "$work/public-step.sh" >"$work/sc.out" 2>&1; then
    ok "shellcheck passes on the check step"
  else bad "shellcheck passes on the check step" "$(cat "$work/sc.out")"; fi
else
  printf '  skip  shellcheck is not installed (CI installs it)\n'
fi

# ============================================================ stub GHCR (curl)
# A scratch repository with what the steps and the script read, and a curl that plays GHCR:
# STUB_TOKEN_403 names services whose anonymous pull token is refused, STUB_401 and STUB_404
# services whose manifest answers that. It logs every call and anything it does not expect.
repo="$work/repo"; bin="$work/bin"; log="$work/curl.log"
mkdir -p "$repo/scripts" "$bin"
cp "$root/VERSION" "$repo/"
cp "$root/scripts/mirror-images.sh" "$root/scripts/services.sh" "$root/scripts/services.txt" "$repo/scripts/"
cat >"$bin/curl" <<'EOF'
#!/usr/bin/env bash
{ printf 'curl'; printf ' %q' "$@"; echo; } >>"$STUB_LOG"
url="${*: -1}"
listed() { case " $1 " in *" $2 "*) return 0 ;; esac; return 1; }
case "$url" in
  "https://ghcr.io/token?scope=repository:cloud-native-encabulator/encabulator/"*":pull")
    svc="${url##*/}"; svc="${svc%:pull}"
    for a in "$@"; do [[ "$a" == Authorization:* ]] && echo "UNEXPECTED credential on the token request" >>"$STUB_LOG"; done
    if listed "${STUB_TOKEN_403:-}" "$svc"; then
      echo "curl: (22) The requested URL returned error: 403" >&2; exit 22
    fi
    echo '{"token":"anonymous-pull-token"}' ;;
  "https://ghcr.io/v2/cloud-native-encabulator/encabulator/"*"/manifests/"*)
    svc="${url%/manifests/*}"; svc="${svc##*/}"; auth=""; prev=""
    for a in "$@"; do [[ "$prev" == "-H" && "$a" == Authorization:* ]] && auth="$a"; prev="$a"; done
    if [[ "$auth" != "Authorization: Bearer anonymous-pull-token" ]]; then
      echo "UNEXPECTED authorization: $auth" >>"$STUB_LOG"; printf '401'; exit 0
    fi
    if listed "${STUB_401:-}" "$svc"; then printf '401'
    elif listed "${STUB_404:-}" "$svc"; then printf '404'
    else printf '200'; fi ;;
  *) echo "UNEXPECTED curl $*" >>"$STUB_LOG"; exit 2 ;;
esac
EOF
chmod +x "$bin/curl"

# run [VAR=value ...] -- COMMAND...: in the scratch repository, with only PATH (stub first),
# HOME, the stub's settings and the given variables, plus a few credentials that must never
# reach curl. Output to $out, status to $status.
out="$work/out"
run() {
  local envs=()
  while [[ $# -gt 0 && "$1" != "--" ]]; do envs+=("$1"); shift; done
  shift
  : >"$log"
  (cd "$repo" && env -i PATH="$bin:$PATH" HOME="$HOME" LC_ALL=C STUB_LOG="$log" \
      GITHUB_TOKEN=ghs_planted_job_token GH_TOKEN=gho_planted_cli_token \
      REGISTRY_PASSWORD=planted_registry_password \
      ${envs[@]+"${envs[@]}"} "$@") >"$out" 2>&1
  status=$?
}
no_credential() {
  expect_not_in "$1: no planted credential reached curl" "$log" "planted"
  if grep -qE -- " (-u|--user|--netrc|--netrc-file|-n) " "$log"; then bad "$1: curl got no user or netrc option" "$(cat "$log")"
  else ok "$1: curl got no user or netrc option"; fi
  expect_not_in "$1: every curl call was one GHCR answers" "$log" "UNEXPECTED"
}

section "mirror-images.sh --check against the stub"
run -- scripts/mirror-images.sh --check
expect_status "every image public: passes" 0 "$status" "$out"
expect_eq "and says so" "$(cat "$out")" "all ${#services[@]} images are on GHCR at $version"
expect_eq "it asked for every image" "$(grep -c "/manifests/$version\$" "$log")" "${#services[@]}"
no_credential "every image public"
for kind in STUB_TOKEN_403 STUB_401 STUB_404; do
  run "$kind=dingle-arm-gateway" -- scripts/mirror-images.sh --check
  expect_status "$kind for one package: fails" 1 "$status" "$out"
  expect_eq "$kind for one package: names it and only it" "$(cat "$out")" \
    "not on GHCR at $version (or GHCR unreachable): dingle-arm-gateway"
  no_credential "$kind for one package"
done
run STUB_401=cardinal-grammeter STUB_404=spurving-bearing-quoter -- scripts/mirror-images.sh --check
expect_status "two packages: fails" 1 "$status" "$out"
expect_in "two packages: names the first" "$out" "cardinal-grammeter"
expect_in "two packages: names the second" "$out" "spurving-bearing-quoter"

# ===================================================== the workflow's own steps
section "discover: the version and the selection"
for ref in refs/heads/main refs/tags/v1.2.3; do
  : >"$work/gh-output"
  run GITHUB_REF="$ref" GITHUB_OUTPUT="$work/gh-output" -- bash --noprofile --norc -eo pipefail "$work/version-step.sh"
  want="$version"; [[ "$ref" == refs/tags/* ]] && want="1.2.3"
  expect_eq "version for $ref" "$(sed -n 's/^version=//p' "$work/gh-output")" "$want"
done
select_services() {
  local script
  script="$(cat "$work/matrix-step.sh")"
  printf '%s\n' "${script//"\${{ inputs.services }}"/$1}" >"$work/matrix-run.sh"
  : >"$work/gh-output"
  run GITHUB_OUTPUT="$work/gh-output" -- bash --noprofile --norc -eo pipefail "$work/matrix-run.sh"
}
select_services ""
expect_eq "no selection: all ${#services[@]}" "$(sed -n 's/^services=//p' "$work/gh-output" | jq length)" "${#services[@]}"
select_services "dingle-arm-gateway"
expect_eq "one name: that service" "$(sed -n 's/^services=//p' "$work/gh-output" | jq -r '[.[].name] | join(" ")')" "dingle-arm-gateway"
select_services "no-such-service"
expect_status "a name that is no service: discover succeeds" 0 "$status" "$out"
expect_eq "and selects nothing, so build is skipped" "$(sed -n 's/^services=//p' "$work/gh-output")" "[]"
expect_in "and warns about the name" "$out" "::warning title=services::no service named no-such-service"

section "the public job's step, as GitHub runs it"
step_env=(IMAGE_PREFIX="$ghcr" GITHUB_REPOSITORY=cloud-native-encabulator/encabulator
          GITHUB_REPOSITORY_OWNER=cloud-native-encabulator)
run "${step_env[@]}" VERSION="$version" -- bash --noprofile --norc -eo pipefail "$work/public-step.sh"
expect_status "every image public: the job passes" 0 "$status" "$out"
expect_in "and says so" "$out" "all ${#services[@]} images are on GHCR at $version"
no_credential "the job, every image public"
run "${step_env[@]}" VERSION=1.2.3 -- bash --noprofile --norc -eo pipefail "$work/public-step.sh"
expect_eq "a tag build: it checks the version the run built, for every image" \
  "$(grep -c "/manifests/1.2.3\$" "$log")" "${#services[@]}"
settings="https://github.com/orgs/cloud-native-encabulator/packages/container/encabulator%2F"
run "${step_env[@]}" VERSION="$version" STUB_TOKEN_403=dingle-arm-gateway -- \
  bash --noprofile --norc -eo pipefail "$work/public-step.sh"
expect_status "one private package: the job fails" 1 "$status" "$out"
expect_in "it names the package" "$out" "::error title=dingle-arm-gateway cannot be pulled anonymously::$ghcr/dingle-arm-gateway:$version."
expect_in "with its settings page" "$out" "Open ${settings}dingle-arm-gateway/settings and change the package's visibility to Public."
expect_in "says there is no API for it" "$out" "there is no API for it"
expect_in "and to re-run" "$out" "Then re-run this job."
expect_eq "and names no other package" "$(grep -c '^::error' "$out")" "1"
no_credential "the job, one private package"
run "${step_env[@]}" VERSION="$version" STUB_401=cardinal-grammeter STUB_404=spurving-bearing-quoter -- \
  bash --noprofile --norc -eo pipefail "$work/public-step.sh"
expect_status "two packages: the job fails" 1 "$status" "$out"
expect_eq "with one error each" "$(grep '^::error' "$out" | sed 's/^::error title=\([^ ]*\) .*/\1/' | sort | paste -sd' ' -)" \
  "cardinal-grammeter spurving-bearing-quoter"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[[ $fail -eq 0 ]]
