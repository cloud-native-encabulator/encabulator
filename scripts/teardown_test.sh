#!/usr/bin/env bash
# Offline tests for the teardown: make remove-cell, make teardown, make gone-check and
# make mirror-images-delete.
#
#   scripts/teardown_test.sh               every case (what CI runs)
#   scripts/teardown_test.sh --mutations   then the mutation checks too (about 6 minutes)
#   scripts/teardown_test.sh case_helper   only the named cases, keeping their scratch files
#
# No AWS account, no cluster, no network. Each case runs the real Makefile and scripts in a
# scratch copy of the repository with aws, kubectl, helm and terraform replaced by
# scripts/teardown-fixtures/stub.py, which keeps a small simulated account and hub and plays
# the parts of kro, ACK, Argo CD and Terraform the order depends on (its header says how).
# AWS_PROFILE and every AWS_ variable are unset and KUBECONFIG is an empty file. Covered:
#   - make -n remove-cell and make -n teardown print the steps and call nothing
#   - remove-cell: the order of the steps, the guards (wrong kubectl context, a hub in another
#     account, no confirmation), every delete limited to encabulator* names and to the cell's
#     own VPC, a stop at a failing call and a re-run that goes on, a timeout that says so
#   - teardown: the whole run in order, Argo CD stopped before the stores, an empty Terraform
#     state as a clean "nothing to do", the destroy-plan guard refusing a foreign address,
#     another account or region, a replace, an untagged resource, state backups at mode 600
#   - mirror-images-delete: refuses when one repository lacks the tag or is in another account
#   - gone-check: --include-managed-resources, an AWS error is a FAIL, stale tagging-index
#     entries confirmed in their own service, instant fleets as WARN
#   - mutations: each guard or order rule weakened in a copy of the scripts makes a case fail
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stub="$root/scripts/teardown-fixtures/stub.py"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mutations=false
[[ "${1:-}" == "--mutations" ]] && mutations=true

pass=0 fail=0 quiet=false
ok()  { pass=$((pass + 1)); $quiet || printf '  ok    %s\n' "$1"; }
bad() {
  fail=$((fail + 1)); $quiet && return 0
  printf '  FAIL  %s\n' "$1"
  [[ -z "${2:-}" ]] || printf '%s\n' "$2" | tail -n 30 | sed 's/^/          /'
}
expect_status() { if [[ "$3" == "$2" ]]; then ok "$1"; else bad "$1 (exit $3, want $2)" "$(cat "${4:-/dev/null}")"; fi; }
expect_in()     { if grep -qF -- "$3" "$2"; then ok "$1"; else bad "$1: not found: $3" "$(cat "$2")"; fi; }
expect_not_in() { if grep -qF -- "$3" "$2"; then bad "$1: found: $3" "$(grep -F -- "$3" "$2" | head -n 5)"; else ok "$1"; fi; }
expect_eq()     { if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1" "got:  $2"$'\n'"want: $3"; fi; }
count_in()      { grep -cF -- "$2" "$1" || true; }
# expect_order NAME FILE MARKER...: each marker's first line comes after the one before.
expect_order() {
  local name="$1" file="$2" last=0 n m; shift 2
  for m in "$@"; do
    n="$(grep -nF -- "$m" "$file" | head -n 1 | cut -d: -f1)"
    if [[ -z "$n" || "$n" -le "$last" ]]; then bad "$name: out of order or missing: $m" "$(cat "$file")"; return; fi
    last="$n"
  done
  ok "$name ($# steps)"
}
world() { python3 -c 'import json,sys
w = json.load(open(sys.argv[1]))
try: print(eval(sys.argv[2]))
except Exception as e: print(f"world has no {e}")' "$state/world.json" "$1"; }

# new_case NAME [world settings...]: a fresh copy of what the teardown uses, stubs, a world.
cases=0 src="$root"
new_case() {
  cases=$((cases + 1))
  case_dir="$work/case-$cases"; repo="$case_dir/repo"; bin="$case_dir/bin"
  state="$case_dir/state"; log="$case_dir/calls.log"; out="$case_dir/out"
  mkdir -p "$repo/terraform/brownfield" "$repo/gitops/hub" "$repo/docs" "$bin" "$state"
  : >"$log"; : >"$case_dir/kubeconfig"
  cp "$src/Makefile" "$src/VERSION" "$repo/"
  cp -R "$src/scripts" "$repo/scripts"
  cp -R "$src/gitops/hub/cells" "$repo/gitops/hub/cells"
  cp -R "$src/docs/teardown" "$repo/docs/teardown"
  mkdir -p "$repo/platform"; cp "$src/platform/versions.env" "$repo/platform/"
  local t
  for t in aws kubectl helm terraform; do ln -s "$stub" "$bin/$t"; done
  STUB_STATE="$state" python3 "$stub" init "${@:2}"
  $quiet || printf '\n%s\n' "$1"
}
# run [VAR=value ...] -- COMMAND...: in the case's repository with nothing from this shell
# but PATH (stubs first) and HOME, stdin from /dev/null; output to $out, status to $status.
run() {
  local envs=()
  while [[ $# -gt 0 && "$1" != "--" ]]; do envs+=("$1"); shift; done
  shift
  (cd "$repo" && umask 022 && env -i PATH="$bin:$PATH" HOME="$HOME" TERM=dumb LC_ALL=C \
      STUB_LOG="$log" STUB_STATE="$state" KUBECONFIG="$case_dir/kubeconfig" \
      TEARDOWN_POLL=0 TEARDOWN_SETTLE=0 \
      TEARDOWN_TIMEOUT_SYNC=3 TEARDOWN_TIMEOUT_CAPABILITIES=3 TEARDOWN_TIMEOUT_VOLUMES=3 \
      TEARDOWN_TIMEOUT_STORES=3 TEARDOWN_TIMEOUT_CELL=5 TEARDOWN_TIMEOUT_ARGOCD=3 \
      GONE_ALL_REGIONS=yes \
      ${envs[@]+"${envs[@]}"} "$@") </dev/null >"$out" 2>&1
  status=$?
}
# aws_deletes: every delete-shaped AWS call the stubs saw, and every adopt object applied.
aws_deletes() { grep -E '^aws [a-z0-9-]+ delete-' "$log" || true; }

cell_markers=(
  "annotate --overwrite roles.iam.services.k8s.aws encabulator-use2-node services.k8s.aws/deletion-policy=retain"
  "annotate --overwrite accessentries.eks.services.k8s.aws encabulator-use2-node services.k8s.aws/deletion-policy=retain"
  "aws ec2 describe-route-tables --region us-east-2"
  "-n encabulator-cells wait --for=condition=ACK.ResourceSynced routetables.ec2.services.k8s.aws/encabulator-use2-teardown-rtb-use2-leak0"
  "-n argocd delete applications.argoproj.io hub-cells"
  "-n encabulator-cells delete encabulatorcells encabulator-use2"
  "aws eks list-capabilities --region us-east-2 --cluster-name encabulator-use2"
  "--context encabulator-use2 -n encabulator delete statefulset nats influxdb"
  "--context encabulator-use2 -n encabulator delete pvc data-nats-0 data-influxdb-0"
  "--context encabulator-use2 get pv -o name"
  "wait --for=condition=ACK.ResourceSynced dbinstances.rds.services.k8s.aws/encabulator-use2-teardown-pg"
  "wait --for=condition=ACK.ResourceSynced serverlesscaches.elasticache.services.k8s.aws/encabulator-use2-teardown-valkey"
  "wait --for=condition=ACK.ResourceSynced dbsubnetgroups.rds.services.k8s.aws/encabulator-use2-teardown-subnet-group"
  "wait --for=condition=ACK.ResourceSynced serverlesscachesnapshots.elasticache.services.k8s.aws/encabulator-use2-teardown-snapshot-1"
  "wait --for=condition=ACK.ResourceSynced roles.iam.services.k8s.aws/encabulator-use2-teardown-node"
  "aws iam get-role --role-name encabulator-use2-node --query Role.RoleName"
)

# ================================================================== the cases
case_plans() {
  new_case "make -n: the plan, and nothing is called"
  run -- make -n remove-cell CELL=use2
  expect_status "make -n remove-cell works" 0 "$status" "$out"
  expect_in "names the cell and its region" "$out" "removes encabulator-use2 in us-east-2, in this order:"
  expect_in "lists step 1" "$out" " 1  check that kubectl points at the hub"
  expect_in "lists step 10" "$out" "10  delete the cell's retained node role"
  run -- make -n teardown
  expect_status "make -n teardown works" 0 "$status" "$out"
  expect_in "lists the Terraform step" "$out" "terraform destroy -refresh=false, after a guard has checked the plan"
  expect_in "lists the gone-check" "$out" "prove the account is clean (make gone-check)"
  run -- make -n mirror-images-delete
  expect_in "make -n mirror-images-delete says what it would delete" "$out" "deletes every ECR repository encabulator/*"
  expect_eq "make -n calls nothing" "$(cat "$log")" ""
  run -- make help
  for t in remove-cell teardown gone-check mirror-images-delete; do expect_in "make help lists $t" "$out" "$t"; done
}

case_remove_cell() {
  new_case "make remove-cell CELL=use2: every step, in order"
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_status "remove-cell succeeds" 0 "$status" "$out"
  expect_order "the steps ran in the proven order" "$log" "${cell_markers[@]}"
  expect_in "prints the step number and ET time" "$out" "[1/10] "
  expect_in "prints ET" "$out" " ET  check that kubectl points at the hub"
  expect_in "explains that the file must leave Git" "$out" "git rm gitops/hub/cells/use2.yaml"
  expect_in "ends by saying what is left" "$out" "Left for you: take its file out of Git"
  expect_eq "the cell is gone" "$(world '[n for n in w["eks"].get("us-east-2", {})]')" "[]"
  expect_eq "its stores are gone" "$(world 'list(w["rds"]["us-east-2"]) + list(w["caches"]["us-east-2"]) + list(w["subnet_groups"]["us-east-2"]) + list(w["snapshots"]["us-east-2"])')" "[]"
  expect_eq "its node role is gone" "$(world '[r for r in w["roles"] if r.startswith("encabulator-use2")]')" "[]"
  expect_eq "both leaked route tables went, the untagged look-alike stayed" \
    "$(world 'sorted(w["route_tables"].get("us-east-2", {}))')" "['rtb-use2-notours']"
  expect_eq "no cell volume was left in EC2" "$(world '[e for e in w["events"] if "left in EC2" in e or "re-created" in e or "cannot" in e]')" "[]"
  expect_eq "the hub's stores are untouched" "$(world 'sorted(w["rds"]["us-west-2"])')" "['encabulator-pg', 'other-db']"
  expect_eq "the hub keeps running" "$(world 'list(w["eks"]["us-west-2"])')" "['encabulator-usw2']"
  expect_eq "no AWS delete verb is used: ACK deletes" "$(aws_deletes)" ""
  expect_not_in "the hub's Argo CD keeps the other Applications" "$log" "delete applications.argoproj.io hub-brownfield"
  expect_eq "every adopt object is an encabulator* name" \
    "$(grep -oE 'ACK.ResourceSynced [a-z.0-9]+/[^ ]+' "$log" | grep -vcE '/encabulator-' || true)" "0"
  expect_eq "every adopted AWS resource is the cell's own" \
    "$(world 'sorted(e for e in w["events"] if e.startswith("ACK deleted"))')" \
    "['ACK deleted dbinstances us-east-2 encabulator-pg', 'ACK deleted dbsubnetgroups us-east-2 encabulator', 'ACK deleted roles us-west-2 encabulator-use2-node', 'ACK deleted routetables us-east-2 rtb-use2-leak0', 'ACK deleted routetables us-east-2 rtb-use2-leak1', 'ACK deleted serverlesscaches us-east-2 encabulator-valkey', 'ACK deleted serverlesscachesnapshots us-east-2 automatic.encabulator-valkey-2026-10-15-09-02']"

  : >"$log"
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_status "a second run succeeds" 0 "$status" "$out"
  expect_in "and says what is already gone" "$out" "EncabulatorCell encabulator-use2: already gone"
  expect_eq "and deletes nothing" "$(grep -cE ' delete |delete-' "$log")" "0"
}

case_remove_cell_guards() {
  new_case "make remove-cell: the guards" context=someone-else
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_status "a context that is not the hub is refused" 2 "$status" "$out"
  expect_in "with one sentence" "$out" "kubectl points at 'someone-else', not the hub encabulator-usw2. Run make kubeconfig first."
  expect_eq "and nothing is changed" "$(grep -cE ' delete | annotate | apply ' "$log")" "0"

  new_case "make remove-cell: a hub context that points elsewhere" server=https://OTHER.example
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_in "is refused" "$out" "not the encabulator-usw2 of this account"
  expect_eq "and nothing is changed" "$(grep -cE ' delete | annotate | apply ' "$log")" "0"

  new_case "make remove-cell: asks first"
  run -- make remove-cell CELL=use2
  expect_status "without a terminal or CONFIRM it refuses" 2 "$status" "$out"
  expect_in "and says how to go on" "$out" "add CONFIRM=encabulator-use2"
  run CONFIRM=yes -- make remove-cell CELL=use2
  expect_in "CONFIRM=yes is not the cell's name" "$out" "CONFIRM is 'yes', not encabulator-use2. Nothing was deleted."
  expect_eq "nothing is changed" "$(grep -cE ' delete | annotate | apply ' "$log")" "0"
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=../hub
  expect_in "a CELL that is not a cell name is refused" "$out" "usage: make remove-cell CELL=<cell>"

  new_case "make remove-cell: a store in another VPC is left alone"
  python3 - "$state/world.json" <<'PY'
import json, sys
w = json.load(open(sys.argv[1])); w["rds"]["us-east-2"]["encabulator-pg"]["vpc"] = "vpc-someone"
json.dump(w, open(sys.argv[1], "w"))
PY
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_status "it stops" 2 "$status" "$out"
  expect_in "with one sentence" "$out" "RDS instance encabulator-pg in us-east-2 is in vpc-someone, not in the VPC of encabulator-use2"
  expect_in "and the step and what is left" "$out" "Left: steps 8 to 10:"
  expect_eq "the instance is still there" "$(world 'w["rds"]["us-east-2"]["encabulator-pg"]["status"]')" "available"

  new_case "make remove-cell: deletion protection is never turned off" protect=yes
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_in "it stops and says so" "$out" "has deletion protection on. Turn it off yourself"

  new_case "make remove-cell: the route table kro tracks is never adopted" tracked_bare=yes
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_status "remove-cell succeeds" 0 "$status" "$out"
  expect_not_in "the tracked table is left to kro" "$log" "teardown-rtb-use2-tracked"
  expect_in "the leftovers still go" "$log" "teardown-rtb-use2-leak0"
}

case_remove_cell_resume() {
  new_case "make remove-cell: stops at a failing call, and a re-run goes on"
  run CONFIRM=encabulator-use2 STUB_FAIL_ONCE='rds describe-db-instances --region us-east-2' -- make remove-cell CELL=use2
  expect_status "the first run stops" 2 "$status" "$out"
  expect_in "names the AWS error" "$out" "ExpiredToken"
  expect_in "names the step and how to go on" "$out" "Stopped at step 8 at"
  expect_in "says to run it again" "$out" "run make remove-cell CELL=use2 again"
  expect_in "lists what is left" "$out" "Left: steps 8 to 10:"
  cp "$log" "$case_dir/first.log"; : >"$log"
  run CONFIRM=encabulator-use2 STUB_FAIL_ONCE='rds describe-db-instances --region us-east-2' -- make remove-cell CELL=use2
  expect_status "the re-run finishes" 0 "$status" "$out"
  expect_in "it sees the cell already being deleted" "$out" "EncabulatorCell encabulator-use2: already being deleted"
  expect_eq "it does not delete the cell object twice" "$(count_in "$log" "delete encabulatorcells")" "0"
  expect_eq "the cell is gone" "$(world '[n for n in w["eks"].get("us-east-2", {})]')" "[]"

  new_case "make remove-cell: an AWS error is never read as gone"
  run CONFIRM=encabulator-use2 STUB_FAIL='eks list-capabilities' -- make remove-cell CELL=use2
  expect_status "it stops" 2 "$status" "$out"
  expect_in "at the wait for the capabilities" "$out" "Stopped at step 6"
  expect_not_in "and never deletes a StatefulSet" "$log" "delete statefulset"

  new_case "make remove-cell: a wait that times out says so" stuck=us-east-2
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_status "it stops" 2 "$status" "$out"
  expect_in "a stuck store times out with a message" "$out" "Still waiting for RDS encabulator-pg in us-east-2 to be deleted"
  expect_in "and says to run it again" "$out" "run make remove-cell CELL=use2 again"
  expect_not_in "and does not go on to the subnet group" "$log" "dbsubnetgroups.rds.services.k8s.aws/encabulator-use2-teardown-subnet-group"

  new_case "make remove-cell: no access to the cell yet: make cell-access first" cell_access=no
  run CONFIRM=encabulator-use2 -- make remove-cell CELL=use2
  expect_status "remove-cell succeeds" 0 "$status" "$out"
  expect_in "it says so" "$out" "giving your AWS identity cluster admin on encabulator-use2 (make cell-access CELL=use2)"
  expect_order "access comes before the StatefulSets" "$log" "aws eks create-access-entry" "delete statefulset nats influxdb"
}

case_teardown() {
  new_case "make teardown: the whole project, in order"
  printf '{"backend": {"type": "local", "config": {}}}\n' >/dev/null
  run CONFIRM=encabulator-usw2 TF_VAR_idc_instance_arn=arn:aws:sso:::instance/ssoins-1111111111111111 -- make teardown
  expect_status "teardown succeeds" 0 "$status" "$out"
  expect_order "the steps ran in order" "$log" \
    "kubectl config current-context" \
    "-n encabulator-cells delete encabulatorcells encabulator-use2" \
    "wait --for=condition=ACK.ResourceSynced roles.iam.services.k8s.aws/encabulator-use2-teardown-node" \
    "-n argocd delete applicationsets.argoproj.io hub-brownfield" \
    "-n argocd delete applications.argoproj.io hub-cells hub-cell-apps hub-rgds" \
    "-n argocd delete applicationsets.argoproj.io encabulator encabulator-platform" \
    "helm uninstall encabulator --namespace encabulator" \
    "-n encabulator delete pvc -l app.kubernetes.io/part-of=encabulator" \
    "helm uninstall prometheus-stack" \
    "--context encabulator-usw2 get pv -o name" \
    "wait --for=condition=ACK.ResourceSynced dbinstances.rds.services.k8s.aws/encabulator-usw2-teardown-pg" \
    "wait --for=condition=ACK.ResourceSynced dbsubnetgroups.rds.services.k8s.aws/encabulator-usw2-teardown-subnet-group" \
    "wait --for=condition=ACK.ResourceSynced serverlesscachesnapshots.elasticache.services.k8s.aws/encabulator-usw2-teardown-snapshot-1" \
    "aws ecr delete-repository --region us-west-2 --repository-name encabulator/" \
    "terraform -chdir=terraform/brownfield plan -destroy -refresh=false" \
    "terraform -chdir=terraform/brownfield apply" \
    "aws resourcegroupstaggingapi get-resources"
  expect_in "asks for the hub's name first" "$out" "CONFIRM=encabulator-usw2: going on without asking."
  expect_in "prints the inventory" "$out" "cell encabulator-use2 (us-east-2): its cluster, VPC, roles and stores"
  expect_in "names the data that is lost" "$out" "Data that is lost"
  expect_in "the guard passed" "$out" "GUARD OK"
  expect_in "Terraform ran" "$out" "Apply complete! Resources: 0 added, 0 changed, 30 destroyed."
  expect_in "the gone-check passed" "$out" "Nothing of the project is left"
  expect_in "ends by saying what is left" "$out" "Left for you: nothing."
  expect_eq "no new store was created behind the teardown" "$(world '[e for e in w["events"] if "new, empty" in e or "re-created the ApplicationSet" in e]')" "[]"
  expect_eq "the only AWS deletes are the ECR repositories" "$(aws_deletes | grep -vc '^aws ecr delete-repository --region us-west-2 --repository-name encabulator/' || true)" "0"
  expect_eq "every ECR delete is an encabulator/ repository with --force" \
    "$(aws_deletes | grep -c -- '--repository-name encabulator/[a-z-]* --force$')" "$(aws_deletes | grep -c .)"
  expect_eq "other/app and other-db stayed" "$(world '"other/app" in w["ecr"]["us-west-2"] and "other-db" in w["rds"]["us-west-2"] and "other-role" in w["roles"]')" "True"
  # shellcheck disable=SC2012 # one known file name
  expect_eq "the state backup is mode 600" "$(ls -l "$repo"/terraform/brownfield/terraform.tfstate.pre-destroy-* | cut -c1-10)" "-rw-------"
  # shellcheck disable=SC2012
  expect_eq "the state Terraform wrote is mode 600" "$(ls -l "$repo/terraform/brownfield/terraform.tfstate" | cut -c1-10)" "-rw-------"
  expect_not_in "the TF_VAR value reached terraform, no secret is printed" "$out" "generated-password-stub"

  : >"$log"
  run CONFIRM=encabulator-usw2 -- make teardown
  expect_status "running it again is safe" 0 "$status" "$out"
  expect_in "the empty state is a clean exit" "$out" "The Terraform state is empty: the hub is already destroyed, nothing to do."
  expect_not_in "no REFUSED on an empty state" "$out" "REFUSED"
  expect_eq "and deletes nothing" "$(grep -cE ' delete |delete-|terraform .* apply' "$log")" "0"
}

case_teardown_guards() {
  new_case "make teardown: asks first"
  run -- make teardown
  expect_status "without CONFIRM it refuses" 2 "$status" "$out"
  expect_in "and says how" "$out" "add CONFIRM=encabulator-usw2"
  expect_eq "nothing is changed" "$(grep -cE ' delete | annotate | apply |delete-' "$log")" "0"

  new_case "make teardown: the hub's state already empty" state=empty hub=gone cells=
  run CONFIRM=encabulator-usw2 -- make teardown
  expect_status "is a clean run" 0 "$status" "$out"
  expect_in "says nothing to do" "$out" "the hub is already destroyed, nothing to do."

  new_case "make teardown: before the hand-off Terraform deletes the stores" pre_handoff=yes cells=
  run CONFIRM=encabulator-usw2 -- make teardown
  expect_status "succeeds" 0 "$status" "$out"
  expect_in "skips the store step" "$out" "Terraform still owns the hub's stores (before the hand-off)"
  expect_in "but deletes the cache snapshots, which outlive Terraform" "$log" "serverlesscachesnapshots.elasticache.services.k8s.aws/encabulator-usw2-teardown-snapshot-1"
  expect_not_in "and adopts no store" "$log" "encabulator-usw2-teardown-pg"

  new_case "make teardown: a LoadBalancer Service stops it" lb_service=encabulator/web cells=
  run CONFIRM=encabulator-usw2 -- make teardown
  expect_status "it stops" 2 "$status" "$out"
  expect_in "names the Service" "$out" "Service encabulator/web  is of type LoadBalancer"
  expect_not_in "before any store is deleted" "$log" "dbinstances.rds.services.k8s.aws/encabulator-usw2-teardown-pg"

  new_case "make teardown TEARDOWN_STORES=cli: the AWS CLI deletes the stores" cells=
  run CONFIRM=encabulator-usw2 TEARDOWN_STORES=cli -- make teardown
  expect_status "succeeds" 0 "$status" "$out"
  expect_in "RDS with no final snapshot and no backups" "$log" "aws rds delete-db-instance --region us-west-2 --db-instance-identifier encabulator-pg --skip-final-snapshot --delete-automated-backups"
  expect_order "the subnet group after the instance is gone" "$log" "aws rds delete-db-instance" "aws rds delete-db-subnet-group --region us-west-2 --db-subnet-group-name encabulator"
  expect_in "the cache snapshot" "$log" "aws elasticache delete-serverless-cache-snapshot --region us-west-2 --serverless-cache-snapshot-name automatic.encabulator-valkey-2026-10-15-09-48"
  expect_eq "every AWS delete names an encabulator* resource" \
    "$(aws_deletes | grep -vcE -- '--(db-instance-identifier|db-subnet-group-name|serverless-cache-name|serverless-cache-snapshot-name|repository-name) (automatic\.)?encabulator' || true)" "0"
}

case_teardown_refusals() {
  local v want
  for v in foreign other-account replace; do
    case "$v" in
      foreign) want="aws_instance.someone_elses: not a resource of terraform/brownfield" ;;
      other-account) want="not the account of your credentials" ;;
      replace) want="only delete is allowed" ;;
    esac
    new_case "make teardown: the destroy-plan guard refuses a plan with $v" plan="$v" cells=
    run CONFIRM=encabulator-usw2 -- make teardown
    expect_status "it stops" 2 "$status" "$out"
    expect_in "names why" "$out" "$want"
    expect_in "says Terraform deleted nothing" "$out" "The guard refused the destroy plan (the REFUSED lines above), so Terraform deleted nothing."
    expect_not_in "and never applies" "$log" "terraform -chdir=terraform/brownfield apply"
    expect_eq "the hub is still there" "$(world '"encabulator-usw2" in w["eks"]["us-west-2"]')" "True"
  done
}

case_teardown_resume() {
  new_case "make teardown: stops at a failing step, and a re-run goes on"
  run CONFIRM=encabulator-usw2 STUB_FAIL_ONCE='terraform -chdir=terraform/brownfield plan' -- make teardown
  expect_status "the first run stops" 2 "$status" "$out"
  expect_in "at the Terraform step" "$out" "Stopped at step 7 at"
  expect_in "and lists what is left" "$out" "Left: steps 7 to 8:"
  expect_eq "the cell is already gone" "$(world '[n for n in w["eks"].get("us-east-2", {})]')" "[]"
  : >"$log"
  run CONFIRM=encabulator-usw2 STUB_FAIL_ONCE='terraform -chdir=terraform/brownfield plan' -- make teardown
  expect_status "the re-run finishes" 0 "$status" "$out"
  expect_in "it finds no cell to remove" "$out" "EncabulatorCell encabulator-use2: already gone"
  expect_in "and nothing left of the hub's stores" "$out" "RDS instance encabulator-pg (us-west-2): none"
  expect_eq "no ECR repository is deleted twice" "$(count_in "$log" "ecr delete-repository")" "0"
  expect_in "Terraform finishes" "$out" "Apply complete!"
  expect_in "and the gone-check passes" "$out" "Nothing of the project is left"
}

# The destroy-plan guard on its own, on the plans the stub's terraform would print.
case_plan_guard() {
  new_case "teardown-plan-guard.py: every refusal"
  local v want g=(python3 "$src/scripts/teardown-plan-guard.py" --account 111122223333 --region us-west-2 --cluster encabulator-usw2)
  python3 "$stub" plan >"$case_dir/plan.json"
  "${g[@]}" <"$case_dir/plan.json" >"$out" 2>&1; status=$?
  expect_status "a plan that only deletes the hub passes" 0 "$status" "$out"
  expect_in "and says so" "$out" "GUARD OK"
  for v in empty foreign other-account other-region replace untagged other-name; do
    case "$v" in
      empty) want="the plan deletes nothing" ;;
      foreign) want="REFUSED: aws_instance.someone_elses: not a resource of terraform/brownfield" ;;
      other-account) want="REFUSED: an ARN is in account <account-id>, not the account of your credentials" ;;
      other-region) want="in region eu-west-1, not us-west-2" ;;
      replace) want="only delete is allowed" ;;
      untagged) want="tag auto-delete=None, not no" ;;
      other-name) want="Name tag prod-vpc, not encabulator-usw2" ;;
    esac
    python3 "$stub" plan "$v" | "${g[@]}" >"$out" 2>&1; status=$?
    expect_status "refuses a plan with $v" 1 "$status" "$out"
    expect_in "names why ($v)" "$out" "$want"
  done
  echo 'not json' | "${g[@]}" >"$out" 2>&1; status=$?
  expect_status "refuses what is not a plan" 1 "$status" "$out"
  python3 "$src/scripts/teardown-plan-guard.py" --account x --region us-west-2 --cluster encabulator-usw2 <"$case_dir/plan.json" >"$out" 2>&1; status=$?
  expect_status "refuses an account that is not an id" 1 "$status" "$out"
}

# The route-table filter and the template renderer on their own.
case_helper() {
  new_case "teardown-helper.py: which route tables are leftovers"
  local h="$src/scripts/teardown-helper.py"
  python3 - >"$case_dir/rts.json" <<'PY'
import json
tags = [{"Key": "Name", "Value": "encabulator-use2-private"}, {"Key": "auto-delete", "Value": "no"},
        {"Key": "project", "Value": "encabulator"}, {"Key": "talk", "Value": "opn315"}]
local = [{"GatewayId": "local"}]
rt = lambda i, **kw: dict({"RouteTableId": i, "Tags": tags, "Associations": [], "Routes": local}, **kw)
print(json.dumps({"RouteTables": [
    rt("rtb-leak"),
    rt("rtb-tracked"),
    rt("rtb-other-name", Tags=[dict(t, Value="encabulator-use2-public") if t["Key"] == "Name" else t for t in tags]),
    rt("rtb-no-project", Tags=[t for t in tags if t["Key"] != "project"]),
    rt("rtb-associated", Associations=[{"Main": False}]),
    rt("rtb-main", Associations=[{"Main": True}]),
    rt("rtb-nat", Routes=local + [{"NatGatewayId": "nat-1"}]),
]}))
PY
  python3 "$h" leaked-route-tables encabulator-use2-private "rtb-tracked rtb-x" <"$case_dir/rts.json" >"$out" 2>&1
  expect_eq "only the untracked, unassociated, local-only, tagged table" "$(cat "$out")" "rtb-leak"
  python3 "$h" render "$src/docs/teardown/adopt-route-table.yaml" RouteTable name=x region=us-east-2 id=rtb-1 rtb=rtb-1 >"$out" 2>&1; status=$?
  expect_status "a template with a placeholder left is refused" 2 "$status" "$out"
  expect_in "and names it" "$out" "placeholders left in RouteTable: vpcID: <vpc-id>"
  python3 "$h" render "$src/docs/teardown/adopt-hub-stores.yaml" DBInstance name=x namespace=encabulator-cells region=eu-west-1 id=encabulator-pg >"$out" 2>&1
  expect_in "the region is filled in" "$out" "services.k8s.aws/region: eu-west-1"
  expect_in "the object is labelled" "$out" 'encabulator.io/teardown: "true"'
  expect_in "deletion-policy delete" "$out" "services.k8s.aws/deletion-policy: delete"
  expect_in "adoption-policy adopt (never create)" "$out" "services.k8s.aws/adoption-policy: adopt"
  expect_in "and no final snapshot" "$out" 'rds.services.k8s.aws/skip-final-snapshot: "true"'
}

case_ecr() {
  new_case "make mirror-images-delete: refuses when a repository lacks the tag" untagged_repo=yes
  run CONFIRM=encabulator-usw2 -- make mirror-images-delete
  expect_status "it stops" 2 "$status" "$out"
  expect_in "names it" "$out" "ECR repository encabulator/untagged: no tag project=encabulator, so no repository was deleted."
  expect_eq "and deletes none" "$(count_in "$log" "ecr delete-repository")" "0"

  new_case "make mirror-images-delete: refuses a repository of another account" foreign_repo_account=yes
  run CONFIRM=encabulator-usw2 -- make mirror-images-delete
  expect_in "names it" "$out" "encabulator/foreign: not in this account and region"
  expect_eq "and deletes none" "$(count_in "$log" "ecr delete-repository")" "0"

  new_case "make mirror-images-delete: deletes the tagged encabulator/ repositories"
  run CONFIRM=encabulator-usw2 -- make mirror-images-delete
  expect_status "succeeds" 0 "$status" "$out"
  expect_eq "one delete per service" "$(count_in "$log" "ecr delete-repository")" "$("$root/scripts/services.sh" count)"
  expect_eq "other/app stays" "$(world '"other/app" in w["ecr"]["us-west-2"]')" "True"
  run CONFIRM=encabulator-usw2 -- make mirror-images-delete
  expect_in "a re-run has nothing to do" "$out" "nothing to delete"
}

case_gone_check() {
  new_case "make gone-check: lists a stack that is still there"
  run -- make gone-check
  expect_status "fails while the hub and cell exist" 2 "$status" "$out"
  expect_in "names the cluster" "$out" "FAIL   us-west-2 EKS clusters encabulator*: encabulator-usw2"
  expect_in "looks in the cell's region from gitops/hub/cells" "$out" "FAIL   us-east-2 EKS clusters encabulator*: encabulator-use2"
  expect_in "names the IAM roles" "$out" "FAIL   IAM roles encabulator*"
  expect_not_in "makes no change" "$log" " delete"

  new_case "make gone-check: nothing left" cells= hub=gone
  python3 - "$state/world.json" <<'PY'
import json, sys
w = json.load(open(sys.argv[1]))
for r in list(w["rds"]): w["rds"][r] = {k: v for k, v in w["rds"][r].items() if not k.startswith("encabulator")}
w["caches"] = {}; w["subnet_groups"] = {}; w["snapshots"] = {}; w["vpcs"] = {}; w["ecr"] = {"us-west-2": {"other/app": {}}}
w["roles"] = {"other-role": {}}
w["instances"] = {"i-old": {"region": "us-west-2", "managed": True, "state": "terminated"}}
w["tagging"] = {"us-west-2": ["arn:aws:ec2:us-west-2:111122223333:instance/i-old", "arn:aws:ec2:us-west-2:111122223333:instance/i-purged",
                              "arn:aws:ec2:us-west-2:111122223333:network-interface/eni-1",
                              "arn:aws:ec2:us-west-2:111122223333:fleet/fleet-1"]}
json.dump(w, open(sys.argv[1], "w"))
PY
  run GONE_REGIONS=eu-west-1 -- make gone-check
  expect_status "passes" 0 "$status" "$out"
  expect_in "stale index entries are confirmed in their own service" "$out" "3 stale entries, each confirmed gone in its own service"
  expect_in "a purged instance counts as gone" "$out" "2 ec2:instance"
  expect_in "instant fleets are a WARN" "$out" "WARN   us-west-2 1 EC2 instant-fleet records"
  expect_in "GONE_REGIONS adds a region" "$out" "== eu-west-1"
  expect_in "says nothing is left" "$out" "Nothing of the project is left"
  expect_not_in "account ids are not printed" "$out" "111122223333"

  run STUB_FAIL='ec2 describe-volumes --region us-west-2' -- make gone-check
  expect_status "an AWS error fails" 2 "$status" "$out"
  expect_in "and is named, never gone" "$out" "FAIL   us-west-2 volumes: AWS error"

  python3 - "$state/world.json" <<'PY'
import json, sys
w = json.load(open(sys.argv[1]))
w["instances"]["i-node"] = {"region": "us-west-2", "managed": True, "state": "running"}
json.dump(w, open(sys.argv[1], "w"))
PY
  run -- make gone-check
  expect_status "an Auto Mode node fails it" 2 "$status" "$out"
  expect_in "found only with --include-managed-resources" "$out" "FAIL   us-west-2 instances: i-node"

  run STUB_FAIL='sts get-caller-identity' -- make gone-check
  expect_status "no credentials fails at once" 2 "$status" "$out"
  expect_in "and says so" "$out" "no working AWS credentials"
}

case_unexpected() {
  new_case "every stub call was one the stubs know"
  cat "$work"/case-*/calls.log >"$work/all.log"
  expect_not_in "no stub saw an unexpected call" "$work/all.log" "UNEXPECTED"
}

all_cases=(case_plans case_helper case_plan_guard case_remove_cell case_remove_cell_guards case_remove_cell_resume
           case_teardown case_teardown_guards case_teardown_refusals case_teardown_resume case_ecr case_gone_check)
# scripts/teardown_test.sh case_gone_check ...: only those cases, no mutations, work kept.
if [[ "${1:-}" == case_* ]]; then all_cases=("$@"); mutations=false; trap - EXIT; echo "work: $work"; fi
make_version="$(make --version 2>/dev/null)"
printf 'with %s, bash %s, %s\n' "${make_version%%$'\n'*}" "$BASH_VERSION" "$(python3 --version)"
for c in "${all_cases[@]}"; do "$c"; done
case_unexpected
printf '\n%d passed, %d failed\n' "$pass" "$fail"
total_fail=$fail

# ============================================================== mutation checks
# Each line weakens one rule in a copy of the scripts; the named case must then fail.
#   case | file | perl substitution
mutation_list() {
  cat <<'EOF'
case_remove_cell_guards|scripts/teardown.sh|s/\[\[ "\$ctx" == "\$HUB_CLUSTER" \]\] \\/true \\/
case_remove_cell_guards|scripts/teardown.sh|s/\[\[ "\$server" == "\$endpoint" \]\] \\/true \\/
case_remove_cell_guards|scripts/teardown.sh|s/\[\[ -n "\$vpc" && "\$2" == "\$vpc" \]\] \\/true \\/
case_remove_cell_guards|scripts/teardown.sh|s/\[\[ "\$protect" == False \]\]/true/
case_remove_cell_guards|scripts/teardown.sh|s/\[\[ "\$CONFIRM" == "\$word" \]\] \|\| die/true || die/
case_remove_cell|scripts/teardown.sh|s/fns=\(cell_check cell_keep_node_role /fns=(cell_check true /
case_remove_cell|scripts/teardown.sh|s/cell_stores cell_wait_gone cell_node_role\)/cell_wait_gone cell_stores cell_node_role)/
case_remove_cell|scripts/teardown.sh|s/cell_delete_object\n?\s+cell_wait_controllers cell_volumes/cell_delete_object cell_volumes cell_wait_controllers/
case_remove_cell|scripts/teardown.sh|s/cell_route_tables cell_stop_argocd/true cell_stop_argocd/
case_helper|scripts/teardown-helper.py|s/if rt\["RouteTableId"\] in tracked:/if False:/
case_helper|scripts/teardown-helper.py|s/if tags.get\("Name"\) != name or any/if any/
case_helper|scripts/teardown-helper.py|s/if rt.get\("Associations"\):/if False:/
case_helper|scripts/teardown-helper.py|s/r.get\("GatewayId"\) != "local"/False/
case_helper|scripts/teardown-helper.py|s/if left:/if False:/
case_remove_cell_guards|scripts/teardown-helper.py|s/if rt\["RouteTableId"\] in tracked:/if False:/
case_remove_cell_resume|scripts/teardown.sh|s/\[\[ "\$out" =~ \$pattern \]\] && return 0/return 0/
case_plan_guard|scripts/teardown-plan-guard.py|s/if base\(a\) not in known:/if False:/
case_plan_guard|scripts/teardown-plan-guard.py|s/if acc != account:/if False:/
case_plan_guard|scripts/teardown-plan-guard.py|s/parts\[3\] not in \(region, ""\)/False/
case_plan_guard|scripts/teardown-plan-guard.py|s/!= \["delete"\]:/== ["never"]:/
case_plan_guard|scripts/teardown-plan-guard.py|s/if tags.get\(k\) != v:/if False:/
case_plan_guard|scripts/teardown-plan-guard.py|s/!= cluster:\n/== "never":\n/
case_plan_guard|scripts/teardown-plan-guard.py|s/if not changes:/if False:/
case_teardown_refusals|scripts/teardown.sh|s/\[\[ \$rc -eq 0 \]\] \|\| die "The guard refused/true || die "The guard refused/
case_teardown_guards|scripts/teardown.sh|s/\[\[ -z "\$lbs" \]\]/true/
case_teardown_guards|scripts/teardown.sh|s/if \[\[ -z "\$list" \]\]; then\n    protect_state\n    say "The Terraform state is empty/if false; then\n    protect_state\n    say "The Terraform state is empty/
case_teardown_guards|scripts/teardown.sh|s/confirm "\$HUB_CLUSTER" "all of it"/true/
case_teardown|scripts/teardown.sh|s/fns=\(hub_check hub_remove_cells hub_stop_argocd hub_app hub_stores/fns=(hub_check hub_remove_cells hub_stores hub_stop_argocd hub_app/
case_teardown|scripts/teardown.sh|s/install -m 600 "\$work\/state"/cp "\$work\/state"/
case_teardown|scripts/teardown.sh|s/kc -n argocd delete applications.argoproj.io hub-cells hub-cell-apps hub-rgds --ignore-not-found --wait=false >\/dev\/null\n  (kc -n argocd delete applicationsets[^\n]*)\n/$1\n  kc -n argocd delete applications.argoproj.io hub-cells hub-cell-apps hub-rgds --ignore-not-found --wait=false >\/dev\/null\n/
case_ecr|scripts/teardown.sh|s/\|\| bad\+=\("\$name: no tag project=encabulator"\)/|| true/
case_ecr|scripts/teardown.sh|s/\|\| bad\+=\("\$name: not in this account and region"\)/|| true/
case_gone_check|scripts/gone-check.sh|s/--include-managed-resources \\\n    --filters "\$q" "Name=instance-state-name/\\\n    --filters "\$q" "Name=instance-state-name/
case_gone_check|scripts/gone-check.sh|s/if \[\[ "\$2" -ne 0 \]\]; then fail/if false; then fail/
case_gone_check|scripts/gone-check.sh|s/\[\[ -n "\$s" \]\] \|\| s=terminated/true/
EOF
}

if $mutations; then
  printf '\nmutation checks: each weakened rule must make its case fail\n'
  killed=0 survived=0 n=0
  while IFS='|' read -r mcase mfile mexpr; do
    n=$((n + 1))
    mut="$work/mutant-$n"; rm -rf "$mut"; mkdir -p "$mut"
    cp -R "$root/scripts" "$root/Makefile" "$root/VERSION" "$mut/"
    mkdir -p "$mut/gitops/hub" "$mut/docs" "$mut/platform"
    cp -R "$root/gitops/hub/cells" "$mut/gitops/hub/"; cp -R "$root/docs/teardown" "$mut/docs/"; cp "$root/platform/versions.env" "$mut/platform/"
    cp "$mut/$mfile" "$mut/$mfile.orig"
    perl -0pi -e "$mexpr" "$mut/$mfile"
    if cmp -s "$mut/$mfile" "$mut/$mfile.orig"; then
      printf '  FAIL  mutation %d did not apply: %s %s\n' "$n" "$mfile" "$mexpr"; survived=$((survived + 1)); continue
    fi
    rm "$mut/$mfile.orig"
    src="$mut" quiet=true pass=0 fail=0
    "$mcase"
    src="$root" quiet=false
    if [[ $fail -gt 0 ]]; then
      killed=$((killed + 1)); printf '  ok    mutation %d (%s, %s): %d check(s) fail\n' "$n" "$mfile" "$mcase" "$fail"
    else
      survived=$((survived + 1)); printf '  FAIL  mutation %d survived: %s %s\n' "$n" "$mfile" "$mexpr"
    fi
  done < <(mutation_list)
  printf '\n%d of %d mutations make a case fail\n' "$killed" "$n"
  [[ $survived -eq 0 ]] || total_fail=$((total_fail + survived))
fi
[[ $total_fail -eq 0 ]]
