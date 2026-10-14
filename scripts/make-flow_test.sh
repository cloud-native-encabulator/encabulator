#!/usr/bin/env bash
# Offline tests for the .env-driven AWS path: `make hub` and its steps.
#
#   scripts/make-flow_test.sh
#
# No AWS account, no cluster, no network. Each case runs the real Makefile and the real
# scripts in a scratch copy of the repository, with aws, terraform, kubectl, helm, docker and
# curl replaced by scripts/make-flow-fixtures/stub.sh, which records every call and fails on
# any it does not know. scripts/make-flow-fixtures/GNUmakefile stands in for the platform and
# GitOps targets, which have their own checks. Covered:
#   - no .env: the existing targets print the commands they printed before
#   - .env parsing (quotes, `=` in values, comments, export, last one wins) and precedence:
#     command line, then environment, then .env, then the defaults
#   - make prereqs: a full pass, and every failure, on a stubbed and on a stripped PATH
#   - make hub: the confirmation (refused without a terminal, on "n", on CONFIRM=no), the
#     whole run with CONFIRM=yes in order, a stop at a failing step, a safe re-run
#   - make -n hub: the .env values threaded into Terraform and the Helm deploy step
#   - make mirror-images: skipped when GHCR has every image, ECR repositories created with
#     the three tags (and scripts/tag-check.py passing on them), an existing version tag never
#     overwritten (FORCE too), REGISTRY recorded in .env with a .env.bak
#   - make kubeconfig and make wait-capabilities
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stub="$root/scripts/make-flow-fixtures/stub.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ecr="111122223333.dkr.ecr.us-west-2.amazonaws.com/encabulator"
ghcr="ghcr.io/jlbutler/cloud-native-encabulator"
arn="arn:aws:sso:::instance/ssoins-1111111111111111"
group="a1b2c3d4e5-11111111-2222-3333-4444-555555555555"
services=()
while IFS= read -r s; do services+=("$s"); done < <("$root/scripts/services.sh" names)
version="$(tr -d '[:space:]' <"$root/VERSION")"

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
count_in()      { grep -cF -- "$2" "$1" || true; }

# new_case NAME: a fresh copy of the repository pieces the flow uses, a stub bin directory,
# an empty call log and empty stub state. (Numbered directories: a name could hold a `:`,
# which would split PATH.)
cases=0
new_case() {
  cases=$((cases + 1))
  case_dir="$work/case-$cases"; repo="$case_dir/repo"; bin="$case_dir/bin"
  state="$case_dir/state"; log="$case_dir/calls.log"; out="$case_dir/out"
  mkdir -p "$repo/terraform/brownfield" "$bin" "$state"
  : >"$log"
  cp "$root/Makefile" "$root/VERSION" "$root/.env.example" "$repo/"
  cp -R "$root/scripts" "$repo/scripts"
  cp "$root/terraform/brownfield/deploy-app.sh" "$repo/terraform/brownfield/"
  cp "$root/scripts/make-flow-fixtures/GNUmakefile" "$repo/GNUmakefile"
  local t dir
  # scripts/images.sh builds only services that have a Dockerfile; the stub runtime reads none.
  while read -r _ dir; do
    mkdir -p "$repo/$dir"; cp "$root/$dir/Dockerfile" "$repo/$dir/Dockerfile"
  done < <("$root/scripts/services.sh" pairs)
  for t in aws terraform kubectl helm docker curl; do ln -s "$stub" "$bin/$t"; done
  printf '\n%s\n' "$1"
}

# in_repo [VAR=value ...] -- COMMAND...: run COMMAND in the case's repository with nothing
# from this shell's environment but PATH (stubs first) and HOME; output to $out, status to
# $status. mk is the same with stdin from /dev/null, so nothing can wait for a terminal.
in_repo() {
  local envs=()
  while [[ $# -gt 0 && "$1" != "--" ]]; do envs+=("$1"); shift; done
  shift
  (cd "$repo" && env -i PATH="$bin:$PATH" HOME="$HOME" TERM=dumb LC_ALL=C \
      STUB_LOG="$log" STUB_STATE="$state" KUBECONFIG="$case_dir/kubeconfig" \
      ${envs[@]+"${envs[@]}"} "$@") >"$out.raw" 2>&1
  status=$?
  # The stand-ins in GNUmakefile replace three recipes on purpose; drop make's notes on that.
  grep -vE "warning: (overriding|ignoring old) recipe for target" "$out.raw" >"$out" || true
}
mk() { in_repo "$@" </dev/null; }

# write_env [extra lines...]: a .env with valid values and REGISTRY empty.
write_env() {
  {
    echo "IDC_INSTANCE_ARN=$arn"
    echo "ARGOCD_ADMIN_GROUP_ID=$group"
    echo "GIT_REPO_URL=https://github.com/example/encabulator-fork"
    echo "GIT_REVISION=main"
    echo "GIT_TOKEN_FILE="
    echo "REGISTRY="
    echo "CONTAINER_RUNTIME=docker"
    local line
    for line in "$@"; do echo "$line"; done
  } >"$repo/.env"
}

# ecr_repo_with SERVICE [TAG LINES...]: an ECR repository that already exists in the stub.
ecr_repo_with() {
  local svc="$1"; shift
  mkdir -p "$state/ecr/encabulator/$svc"
  printf '%s\n' "$@" >"$state/ecr/encabulator/$svc/tags"
}

contract_tags=("Key=auto-delete,Value=no" "Key=project,Value=encabulator" "Key=talk,Value=opn315")

# ================================================================== no .env
new_case "no .env: the existing targets are unchanged"
mk -- make help; expect_status "make help works without .env" 0 "$status" "$out"
expect_in "help lists make hub" "$out" "hub "
expect_in "help lists make prereqs" "$out" "prereqs "
expect_in "help shows the default registry" "$out" "$ghcr"
mk -- make -n brownfield-plan
expect_eq "brownfield-plan prints what it printed before" "$(cat "$out")" "terraform -chdir=terraform/brownfield plan"
mk -- make -n brownfield-apply
expect_eq "brownfield-apply prints what it printed before" "$(cat "$out")" "terraform -chdir=terraform/brownfield apply"
mk -- make -n brownfield-init brownfield-test
expect_eq "brownfield-init and brownfield-test unchanged" "$(cat "$out")" \
  "terraform -chdir=terraform/brownfield init"$'\n'"terraform -chdir=terraform/brownfield init -backend=false"$'\n'"terraform -chdir=terraform/brownfield test"
mk -- make -n deploy
expect_in "make deploy still uses GHCR" "$out" "--set global.registry=$ghcr"
mk -- make print-env
expect_eq "no .env: only the variables exported before reach recipes" "$(cat "$out")" \
  "CONTAINER_RUNTIME=finch"$'\n'"REGISTRY=$ghcr"
expect_eq "nothing was called" "$(cat "$log")" ""

# ============================================================ .env parsing
new_case ".env parsing and precedence"
{
  echo "# a comment line, and a blank line next"
  echo ""
  echo "IDC_INSTANCE_ARN=$arn"
  echo "export ARGOCD_ADMIN_GROUP_ID = \"$group\""
  echo "GIT_REPO_URL='https://git.example.com/team/encabulator.git?a=b&c=d'   # quoted, with = and &"
  echo "GIT_REVISION=feature/env-flow # an inline comment"
  echo "GIT_REVISION=release/2"
  echo "GIT_TOKEN_FILE=\"/home/someone/my tokens/encabulator.token\""
  printf 'REGISTRY=registry.example.com/team\r\n'
  echo "CONTAINER_RUNTIME=docker"
  echo "IDC_REGION="
  echo "NOT_A_SETTING=never-printed-7f3a9c"
} >"$repo/.env"
mk -- make print-env
expect_eq ".env values reach the recipes, parsed" "$(cat "$out")" "$(printf '%s\n' \
  "ARGOCD_ADMIN_GROUP_ID=$group" \
  "CONTAINER_RUNTIME=docker" \
  "GIT_REPO_URL=https://git.example.com/team/encabulator.git?a=b&c=d" \
  "GIT_REVISION=release/2" \
  "GIT_TOKEN_FILE=/home/someone/my tokens/encabulator.token" \
  "IDC_INSTANCE_ARN=$arn" \
  "REGISTRY=registry.example.com/team")"
mk -- make -n brownfield-apply
expect_eq "Terraform gets the Identity Center values as TF_VAR_ on its command line" "$(cat "$out")" \
  "TF_VAR_idc_instance_arn='$arn' TF_VAR_argocd_admin_group_id='$group' terraform -chdir=terraform/brownfield apply"
mk REGISTRY=env.example.com/x GIT_REVISION=from-env -- make print-env
expect_in "the shell environment wins over .env (REGISTRY)" "$out" "REGISTRY=env.example.com/x"
expect_in "the shell environment wins over .env (GIT_REVISION)" "$out" "GIT_REVISION=from-env"
mk REGISTRY=env.example.com/x -- make print-env REGISTRY=cli.example.com/y
expect_in "the command line wins over the environment" "$out" "REGISTRY=cli.example.com/y"
mk -- make -n deploy
expect_in "make deploy uses REGISTRY from .env" "$out" "--set global.registry=registry.example.com/team"
for target in help print-env "-n hub" prereqs; do
  # shellcheck disable=SC2086 # target holds make flags on purpose
  mk -- make $target
  expect_not_in "no target prints the rest of .env (make $target)" "$out" "never-printed-7f3a9c"
done
printf 'REGISTRY=\n' >>"$repo/.env"
mk -- make print-env
expect_in "an empty value means the default" "$out" "REGISTRY=$ghcr"
# shellcheck disable=SC2016 # a literal $HOME is the point
printf 'GIT_REVISION=$HOME\n' >>"$repo/.env"
mk -- make print-env
expect_in "a value with \$ is refused with a message" "$out" "contains \$ or a backtick"
expect_not_in "a refused value is not used" "$out" "GIT_REVISION="

# ------------------------------------------------------------ dotenv.sh set
new_case "dotenv.sh set: in place, idempotent, with a backup"
write_env "# keep me"
chmod 600 "$repo/.env"
cp "$repo/.env" "$case_dir/before"
in_repo -- scripts/dotenv.sh set .env REGISTRY "$ecr"
expect_status "set succeeds" 0 "$status" "$out"
expect_eq "only the REGISTRY line changed" "$(diff "$case_dir/before" "$repo/.env" | grep '^[<>]')" \
  "< REGISTRY="$'\n'"> REGISTRY=$ecr"
expect_eq ".env.bak is the file before the change" "$(cmp "$case_dir/before" "$repo/.env.bak" && echo same)" "same"
# shellcheck disable=SC2012 # one known file name; ls -l is the portable way to read its mode
expect_eq "the file mode is kept" "$(ls -l "$repo/.env" | cut -c1-10)" "-rw-------"
cp "$repo/.env" "$case_dir/after"; : >"$repo/.env.bak"
in_repo -- scripts/dotenv.sh set .env REGISTRY "$ecr"
expect_eq "setting the same value again changes nothing" "$(cmp "$case_dir/after" "$repo/.env" && echo same)" "same"
expect_eq "and writes no new backup" "$(wc -c <"$repo/.env.bak" | tr -d ' ')" "0"
in_repo -- scripts/dotenv.sh set .env NEW_KEY value
expect_eq "a missing key is appended" "$(tail -n 1 "$repo/.env")" "NEW_KEY=value"
in_repo -- scripts/dotenv.sh set .env REGISTRY 'bad value'
expect_status "set refuses a value it cannot write plainly" 2 "$status" "$out"

# ================================================================= prereqs
new_case "make prereqs: everything in place"
write_env
mk -- make prereqs
expect_status "prereqs passes" 0 "$status" "$out"
expect_in "prints the principal" "$out" "AWS credentials: arn:aws:sts::111122223333:assumed-role/Admin/tester"
expect_in "finds the Identity Center instance" "$out" "Identity Center instance found in us-west-2"
expect_in "finds the admin group" "$out" "Argo CD admin group: Encabulator admins"
expect_in "says so" "$out" "Everything make hub needs is in place."
expect_not_in "only read-only AWS calls" "$log" "create"

new_case "make prereqs: on a host without the tools"
bare="$case_dir/bare"; mkdir -p "$bare"
for t in bash dirname head grep sed awk tr; do ln -s "$(command -v "$t")" "$bare/$t"; done
(cd "$repo" && env -i PATH="$bare" HOME="$HOME" "$bare/bash" scripts/prereqs.sh) >"$out" 2>&1
status=$?
expect_status "prereqs fails" 1 "$status" "$out"
for missing in "GNU make not found" "git not found" "AWS CLI not found" "Terraform not found" \
  "kubectl not found" "helm not found" "python3 not found" "curl not found" ".env not found" \
  "IDC_INSTANCE_ARN is not set" "ARGOCD_ADMIN_GROUP_ID is not set"; do
  expect_in "names: $missing" "$out" "$missing"
done
expect_in "lists every problem at once" "$out" "11 problem(s) to fix before make hub"
expect_eq "every problem has its fix" "$(count_in "$out" "fix: ")" "11"

new_case "make prereqs: each failure"
cp "$repo/.env.example" "$repo/.env"
mk -- make prereqs
expect_status "placeholders fail (make exits 2 when a recipe fails)" 2 "$status" "$out"
expect_in "placeholder ARN named" "$out" "IDC_INSTANCE_ARN is still the placeholder"
expect_in "placeholder group named" "$out" "ARGOCD_ADMIN_GROUP_ID is still the placeholder"
write_env "IDC_INSTANCE_ARN=arn:aws:iam::111122223333:role/x" "ARGOCD_ADMIN_GROUP_ID=admins"
mk -- make prereqs
expect_in "a malformed ARN is named" "$out" "IDC_INSTANCE_ARN does not look like"
expect_in "a malformed group id is named" "$out" "ARGOCD_ADMIN_GROUP_ID does not look like"
write_env
mk STUB_AWS_VERSION="aws-cli/1.33.0 Python/3.9" -- make prereqs
expect_status "AWS CLI v1 fails" 2 "$status" "$out"
expect_in "AWS CLI v1 named" "$out" "AWS CLI v2 needed"
mk STUB_AWS_CREDS=no -- make prereqs
expect_in "missing credentials named" "$out" "no working AWS credentials"
mk STUB_TF_VERSION=v1.6.6 -- make prereqs
expect_in "Terraform 1.6 refused" "$out" "Terraform 1.7 or newer 1.x needed"
mk STUB_TF_VERSION=v2.0.0 -- make prereqs
expect_in "Terraform 2 refused" "$out" "Terraform 1.7 or newer 1.x needed"
mk STUB_HELM_VERSION=v3.21.3+g1234567 -- make prereqs
expect_in "Helm 3 refused" "$out" "Helm 4 needed"
mk STUB_IDC_REGION=us-east-1 -- make prereqs
expect_status "Identity Center in another region fails" 2 "$status" "$out"
expect_in "and says to set IDC_REGION" "$out" "set IDC_REGION"
write_env "IDC_REGION=us-east-1"
mk STUB_IDC_REGION=us-east-1 -- make prereqs
expect_status "IDC_REGION finds it" 0 "$status" "$out"
write_env "ARGOCD_ADMIN_GROUP_ID=a1b2c3d4e5-99999999-2222-3333-4444-555555555555"
mk -- make prereqs
expect_in "an unknown group is named" "$out" "not found in that Identity Center"
write_env "CONTAINER_RUNTIME=no-such-runtime"
mk STUB_GHCR_HAVE=none -- make prereqs
expect_status "a missing runtime is only a warning" 0 "$status" "$out"
expect_in "it says when mirroring needs one" "$out" "make mirror-images will build them"
mk STUB_GHCR_HAVE=all -- make prereqs
expect_in "and when it does not" "$out" "no container runtime needed: every image is on GHCR"
write_env
: >"$repo/terraform/brownfield/encabulator.auto.tfvars"
mk -- make prereqs
expect_in "an encabulator.auto.tfvars is pointed out" "$out" "encabulator.auto.tfvars exists"

# ======================================================== hub: confirmation
new_case "make hub: asks first"
write_env
mk -- make hub
expect_status "without a terminal or CONFIRM it refuses" 2 "$status" "$out"
expect_in "and says how to go on" "$out" "make hub CONFIRM=yes"
expect_not_in "nothing ran after the checks" "$log" "terraform -chdir"
mk -- make hub CONFIRM=no
expect_in "CONFIRM=no refuses" "$out" "CONFIRM must be empty or yes"
expect_not_in "CONFIRM=no ran nothing" "$log" "terraform -chdir"
if script --version 2>/dev/null | grep -q util-linux; then
  in_repo -- script -qec "make --no-print-directory hub" /dev/null <<<"n"
  expect_in "on a terminal it asks" "$out" "This creates billable AWS resources (about 25 minutes). Continue? [y/N]"
  expect_in "an n stops it" "$out" "Stopped. This run changed nothing."
  expect_not_in "and nothing ran" "$log" "terraform -chdir"
  in_repo STUB_GHCR_HAVE=all -- script -qec "make --no-print-directory hub" /dev/null <<<"y"
  expect_in "a y goes on" "$log" "terraform -chdir=terraform/brownfield apply -auto-approve"
else
  echo "  skip  terminal prompt checks (needs util-linux script)"
fi

# ======================================================== hub: the whole run
new_case "make hub CONFIRM=yes: every step, in order"
write_env "GIT_TOKEN_FILE=/home/someone/.config/encabulator/token"
mk STUB_GHCR_HAVE=none -- make hub CONFIRM=yes
expect_status "make hub succeeds" 0 "$status" "$out"
expect_in "ends with the next steps" "$out" "make add-cell CELL=use2"
markers=(
  "aws sts get-caller-identity"
  "terraform -chdir=terraform/brownfield init"
  "terraform -chdir=terraform/brownfield apply -auto-approve | TF_VAR_argocd_admin_group_id=$group TF_VAR_idc_instance_arn=$arn"
  "terraform -chdir=terraform/brownfield output -raw kubeconfig_command"
  "aws eks update-kubeconfig --region us-west-2 --name encabulator-usw2 --alias encabulator-usw2"
  "aws eks describe-capability --region us-west-2 --cluster-name encabulator-usw2 --capability-name kro"
  "curl https://ghcr.io/v2/jlbutler/cloud-native-encabulator/"
  "aws ecr create-repository --region us-west-2 --repository-name encabulator/"
  "docker build"
  "target platform"
  "helm upgrade --install encabulator charts/encabulator"
  "target gitops-register"
  "target gitops-repo-access"
  "target gitops-bootstrap"
)
last=0 order_ok=true
for m in "${markers[@]}"; do
  n="$(grep -nF -- "$m" "$log" | head -n 1 | cut -d: -f1)"
  if [[ -z "$n" || "$n" -le "$last" ]]; then order_ok=false; bad "step out of order or missing: $m" "$(cat "$log")"; break; fi
  last="$n"
done
$order_ok && ok "the steps ran in order: ${#markers[@]} markers"
expect_in "Helm deploys from the ECR mirror" "$log" "--set global.registry=$ecr"
expect_in "Helm deploys VERSION" "$log" "--set global.imageTag=$version"
expect_in "the GitOps steps get the registry, the Git settings and the hub region" "$log" \
  "target gitops-register | REGISTRY=$ecr GIT_REPO_URL=https://github.com/example/encabulator-fork GIT_REVISION=main GIT_TOKEN_FILE=/home/someone/.config/encabulator/token AWS_REGION=us-west-2"
expect_in ".env now names the mirror" "$repo/.env" "REGISTRY=$ecr"
cp "$log" "$case_dir/first.log"; : >"$log"
mk STUB_GHCR_HAVE=none -- make hub CONFIRM=yes
expect_status "re-running make hub succeeds" 0 "$status" "$out"
expect_eq "the re-run creates no repository" "$(count_in "$log" "ecr create-repository")" "0"
expect_eq "the re-run builds no image" "$(count_in "$log" "docker build")" "0"
expect_in "the re-run still deploys from the mirror" "$log" "--set global.registry=$ecr"

new_case "make hub CONFIRM=yes: stops at a failing step"
write_env
mk STUB_TF_APPLY_FAIL=1 -- make hub CONFIRM=yes
expect_status "make hub fails" 2 "$status" "$out"
expect_in "names the step" "$out" 'make hub stopped at step "brownfield-apply".'
expect_in "says how to resume" "$out" "run make hub again"
expect_not_in "nothing after it ran" "$log" "update-kubeconfig"
expect_not_in "no GitOps step ran" "$log" "target gitops"

# ================================================================== make -n hub
new_case "make -n hub: the values on their way to Terraform and Helm"
write_env
mk -- make -n hub
expect_status "make -n hub works" 0 "$status" "$out"
expect_in "Terraform apply gets the .env values" "$out" \
  "TF_VAR_idc_instance_arn='$arn' TF_VAR_argocd_admin_group_id='$group' terraform -chdir=terraform/brownfield apply -auto-approve"
expect_in "the confirmation is part of it" "$out" "./scripts/confirm.sh"
expect_in "the deploy step gets the registry" "$out" "REGISTRY='$ghcr' ./terraform/brownfield/deploy-app.sh"
write_env "REGISTRY=$ecr"
mk -- make -n hub
expect_in "with a mirror in .env, the deploy step gets the mirror" "$out" "REGISTRY='$ecr' ./terraform/brownfield/deploy-app.sh"
expect_in "the GitOps steps get the hub region" "$out" "AWS_REGION=us-west-2"
expect_eq "make -n hub calls nothing" "$(cat "$log")" ""

# ============================================================ mirror-images
new_case "make mirror-images: GHCR has every image"
write_env; cp "$repo/.env" "$case_dir/before"
mk STUB_GHCR_HAVE=all -- make mirror-images
expect_status "succeeds" 0 "$status" "$out"
expect_in "says there is nothing to mirror" "$out" "nothing to mirror"
expect_eq "makes no AWS call" "$(count_in "$log" "aws ")" "0"
expect_eq "leaves .env alone" "$(cmp "$case_dir/before" "$repo/.env" && echo same)" "same"

new_case "make mirror-images: GHCR lacks the release"
write_env; cp "$repo/.env" "$case_dir/before"
mk STUB_GHCR_HAVE="dingle-arm-gateway" -- make mirror-images
expect_status "succeeds" 0 "$status" "$out"
expect_eq "creates one repository per service" "$(count_in "$log" "aws ecr create-repository")" "${#services[@]}"
expect_eq "each with the three contract tags at create time" \
  "$(grep -F "aws ecr create-repository" "$log" | grep -cF -- "--tags ${contract_tags[*]}")" "${#services[@]}"
expect_eq "builds and pushes every image for linux/amd64" \
  "$(grep -F "docker build" "$log" | grep -F -- "--platform linux/amd64" | grep -cF -- "--push")" "${#services[@]}"
expect_in "logs the runtime in to ECR" "$log" "docker login --username AWS --password-stdin 111122223333.dkr.ecr.us-west-2.amazonaws.com"
expect_in "records REGISTRY in .env" "$repo/.env" "REGISTRY=$ecr"
expect_eq "keeps the previous .env as .env.bak" "$(cmp "$case_dir/before" "$repo/.env.bak" && echo same)" "same"

# scripts/tag-check.py reads the repositories exactly as they were created.
python3 - "$root/scripts/tag-check-fixtures/tagged.json" "$state" "$case_dir" <<'PY'
import json, pathlib, sys
base, state, out = sys.argv[1], pathlib.Path(sys.argv[2]), pathlib.Path(sys.argv[3])
def fixture(tag_lines_for):
    repos, calls = [], {}
    for d in sorted((state / "ecr" / "encabulator").iterdir()):
        name = f"encabulator/{d.name}"
        arn = f"arn:aws:ecr:us-west-2:111122223333:repository/{name}"
        repos.append({"repositoryName": name, "repositoryArn": arn})
        tags = [dict(zip(("Key", "Value"), l[len("Key="):].split(",Value=", 1))) for l in tag_lines_for(d)]
        calls[f"ecr list-tags-for-resource --region us-west-2 --resource-arn {arn}"] = {"tags": tags}
    calls["ecr describe-repositories --region us-west-2"] = {"repositories": repos}
    return {"base": base, "calls": calls}
created = lambda d: (d / "tags").read_text().split()
old_script = lambda d: ["Key=project,Value=cloud-native-encabulator"]
(out / "ecr-created.json").write_text(json.dumps(fixture(created)))
(out / "ecr-old-script.json").write_text(json.dumps(fixture(old_script)))
PY
in_repo -- python3 scripts/tag-check.py --fixtures "$case_dir/ecr-created.json"
expect_status "tag-check passes on the repositories mirror-images created" 0 "$status" "$out"
expect_eq "tag-check reads all of them as fully tagged" \
  "$(grep -F " ecr repository " "$out" | grep -F "ok " | grep -vcF "note: lacks")" "${#services[@]}"
in_repo -- python3 scripts/tag-check.py --fixtures "$case_dir/ecr-old-script.json"
expect_status "tag-check fails on repositories tagged the old way" 1 "$status" "$out"

cp "$repo/.env" "$case_dir/after"; : >"$log"
mk STUB_GHCR_HAVE="dingle-arm-gateway" -- make mirror-images
expect_status "a re-run succeeds" 0 "$status" "$out"
expect_eq "a re-run builds nothing" "$(count_in "$log" "docker build")" "0"
expect_eq "a re-run creates nothing" "$(count_in "$log" "ecr create-repository")" "0"
expect_eq "a re-run leaves .env as it is" "$(cmp "$case_dir/after" "$repo/.env" && echo same)" "same"

new_case "make mirror-images: an existing version tag is never overwritten"
write_env
for svc in dingle-arm-gateway spurving-bearing-quoter; do
  ecr_repo_with "$svc" "${contract_tags[@]}"
  : >"$state/ecr/encabulator/$svc/image-$version"
done
mk STUB_GHCR_HAVE=none FORCE=1 -- make mirror-images
expect_status "succeeds" 0 "$status" "$out"
expect_in "FORCE is ignored, and says so" "$out" "FORCE is ignored"
expect_in "reports the tag that is already there" "$out" "already in ECR at $version, left as it is"
expect_eq "builds only the others" "$(count_in "$log" "docker build")" "$((${#services[@]} - 2))"
expect_not_in "never builds dingle-arm-gateway" "$log" "-t $ecr/dingle-arm-gateway:"
expect_not_in "never builds spurving-bearing-quoter" "$log" "-t $ecr/spurving-bearing-quoter:"

new_case "make mirror-images: a repository tagged the old way gets the contract tags"
write_env
ecr_repo_with nofer-trunnion-dispatch "Key=project,Value=cloud-native-encabulator"
mk STUB_GHCR_HAVE=none -- make mirror-images
expect_in "it is tagged" "$out" "tagged   encabulator/nofer-trunnion-dispatch"
expect_eq "with the three tags" "$(cat "$state/ecr/encabulator/nofer-trunnion-dispatch/tags")" \
  "$(printf '%s\n' "${contract_tags[@]}")"
expect_eq "and is not created again" "$(grep -F "create-repository" "$log" | grep -cF "nofer-trunnion-dispatch")" "0"

new_case "make mirror-images: REGISTRY names another registry"
write_env "REGISTRY=registry.example.com/team"
mk STUB_GHCR_HAVE=none -- make mirror-images
expect_status "succeeds" 0 "$status" "$out"
expect_in "uses it as it is" "$out" "REGISTRY is set to registry.example.com/team"
expect_eq "creates and builds nothing" "$(grep -cE "create-repository|docker build" "$log")" "0"

new_case "mirror-images.sh --check"
in_repo STUB_GHCR_HAVE="dingle-arm-gateway" -- scripts/mirror-images.sh --check
expect_status "fails when GHCR lacks images" 1 "$status" "$out"
expect_in "and names them" "$out" "spurving-bearing-quoter"
in_repo STUB_GHCR_HAVE=all -- scripts/mirror-images.sh --check
expect_status "passes when GHCR has them all" 0 "$status" "$out"

# ======================================================= kubeconfig and wait
new_case "make kubeconfig and make wait-capabilities"
write_env
mk -- make kubeconfig
expect_status "kubeconfig succeeds" 0 "$status" "$out"
expect_in "uses a stable context name" "$log" "aws eks update-kubeconfig --region us-west-2 --name encabulator-usw2 --alias encabulator-usw2"
expect_in "sets the namespace" "$log" "kubectl config set-context --current --namespace encabulator"
expect_in "checks the API server" "$out" "kubectl points at encabulator-usw2"
mk -- make wait-capabilities
expect_status "wait-capabilities succeeds when all are ACTIVE" 0 "$status" "$out"
expect_in "and says so" "$out" "All three capabilities are ACTIVE."
mk STUB_CAP_ACK=CREATING -- make wait-capabilities CAPABILITY_TIMEOUT=0
expect_status "a timeout fails" 2 "$status" "$out"
expect_in "and names the capability that is not ACTIVE" "$out" "not ACTIVE: ack (CREATING)"
mk STUB_CAP_KRO=CREATE_FAILED -- make wait-capabilities
expect_status "a failed capability stops it at once" 2 "$status" "$out"
expect_in "and names it" "$out" "failed: kro (CREATE_FAILED)"

# ========================================================================== end
new_case "every stub call was one the stubs know"
cat "$work"/*/calls.log >"$work/all.log"
expect_not_in "no stub saw an unexpected call" "$work/all.log" "UNEXPECTED"
printf '\n%d passed, %d failed\n' "$pass" "$fail"
[[ $fail -eq 0 ]]
