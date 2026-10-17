#!/usr/bin/env bash
# Offline tests for ops-health.sh: every incident in ../fixtures/fire-drill/ plus a few edge
# cases, with kubectl and aws replaced by ../fixtures/fire-drill/stub.sh.
#
#   scripts/ops-health_test.sh               every case
#   scripts/ops-health_test.sh --mutations   then the mutation checks: each rule weakened in a
#                                            copy of the script must make a case fail
#
# No cluster, no AWS account, no network: KUBECONFIG is an empty file, every AWS_ variable is
# unset, and the stand-ins are first on PATH as well as named by KUBECTL and AWS. Covered:
#   - each incident reaches the right finding, the right explanation and the right exit status
#   - read-only: only kubectl config, api-resources and get, only aws describe and list, never a
#     Secret, nothing refused by the stand-in
#   - one screen (at most 25 lines, 40 with --explain), under 20 seconds
#   - every 12-digit number in the output is 111122223333
#   - an unreachable cluster stops after two calls with exit 2; a failing or missing AWS CLI,
#     --no-aws and a non-EKS context leave the kubectl checks standing
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
drills="$here/fixtures/fire-drill"
stub="$drills/stub.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/ops-health-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
script="$here/scripts/ops-health.sh"

for v in $(env | sed -n 's/^\(AWS_[A-Z_]*\)=.*/\1/p'); do unset "$v"; done
bin="$work/bin"; mkdir -p "$bin"
printf '#!/usr/bin/env bash\nSTUB_TOOL=kubectl exec "%s" "$@"\n' "$stub" >"$bin/kubectl"
printf '#!/usr/bin/env bash\nSTUB_TOOL=aws exec "%s" "$@"\n' "$stub" >"$bin/aws"
chmod 755 "$bin/kubectl" "$bin/aws"
: >"$work/kubeconfig"
export PATH="$bin:$PATH" KUBECONFIG="$work/kubeconfig" KUBECTL="$bin/kubectl" AWS="$bin/aws"

pass=0 fail=0 quiet=false
ok()  { pass=$((pass + 1)); $quiet || printf '  ok    %s\n' "$1"; }
bad() {
  # A mutant is killed by its first failing check, so the mutation run stops right there.
  fail=$((fail + 1)); $quiet && exit 1
  printf '  FAIL  %s\n' "$1"
  [[ -z "${2:-}" ]] || printf '%s\n' "$2" | tail -n 30 | sed 's/^/          /'
}
expect_status() { if [[ "$status" == "$1" ]]; then ok "$name: exit $1"; else bad "$name: exit $status, want $1" "$(cat "$out")"; fi; }
expect_in()     { if grep -qF -- "$1" "$out"; then ok "$name: shows: $1"; else bad "$name: missing: $1" "$(cat "$out")"; fi; }
expect_not_in() { if grep -qF -- "$1" "$out"; then bad "$name: should not show: $1" "$(grep -F -- "$1" "$out")"; else ok "$name: does not show: $1"; fi; }
expect_call()   { if grep -qF -- "$1" "$log"; then ok "$name: calls: $1"; else bad "$name: never called: $1" "$(cat "$log")"; fi; }
expect_no_call(){ if grep -qF -- "$1" "$log"; then bad "$name: should not call: $1" "$(grep -F -- "$1" "$log")"; else ok "$name: does not call: $1"; fi; }
no_aws_call()   { if grep -q '^aws ' "$log"; then bad "$name: should not call aws" "$(grep '^aws ' "$log")"; else ok "$name: no aws call"; fi; }

# A world made in the test: `## base healthy` plus the sections given on stdin.
make_world() {
  mkdir -p "$work/worlds/$1" "$work/worlds/healthy"
  cp "$drills/healthy/world.txt" "$work/worlds/healthy/world.txt"
  { echo "## base healthy"; cat; } >"$work/worlds/$1/world.txt"
}

# run NAME WORLD [ARGS...]: run the script against WORLD (a fire-drill case or a made world),
# then the checks every run must pass.
cases=0
run() {
  name="$1"; local world="$2" start lines limit=25 n
  shift 2
  cases=$((cases + 1))
  out="$work/out-$cases"; log="$work/calls-$cases"; : >"$log"
  if [[ -f "$drills/$world/world.txt" ]]; then world="$drills/$world/world.txt"; else world="$work/worlds/$world/world.txt"; fi
  start=$SECONDS
  DRILL_WORLD="$world" DRILL_LOG="$log" "$script" "$@" >"$out" 2>&1
  status=$?
  [[ $((SECONDS - start)) -lt 20 ]] && ok "$name: under 20 seconds" || bad "$name: took $((SECONDS - start)) seconds"
  case " $* " in *" --explain "*) limit=40 ;; esac
  lines="$(wc -l <"$out" | tr -d ' ')"
  [[ "$lines" -le "$limit" ]] && ok "$name: one screen ($lines lines)" || bad "$name: $lines lines, more than $limit" "$(cat "$out")"
  if grep -q 'REFUSED' "$log"; then bad "$name: the stand-in refused a call" "$(grep REFUSED "$log")"; else ok "$name: nothing refused"; fi
  n="$(grep '^kubectl ' "$log" | sed -E 's/ --context [^ ]+//' \
       | awk '$2 != "config" && $2 != "get" && $2 != "api-resources"' | wc -l | tr -d ' ')"
  [[ "$n" -eq 0 ]] && ok "$name: kubectl only reads" || bad "$name: kubectl verbs beyond config, get, api-resources" "$(cat "$log")"
  n="$(grep '^aws ' "$log" | awk '$3 !~ /^(describe|list)-/' | wc -l | tr -d ' ')"
  [[ "$n" -eq 0 ]] && ok "$name: aws only describes and lists" || bad "$name: aws calls beyond describe and list" "$(cat "$log")"
  if grep -qiE '^kubectl get [^ ]*secret' "$log" || grep -qiE '^kubectl get .* [a-z.,]*secret' "$log"; then
    bad "$name: reads Secrets" "$(grep -i secret "$log")"
  else
    ok "$name: never reads a Secret"
  fi
  if grep -oE '(^|[^0-9])[0-9]{12}([^0-9]|$)' "$out" | grep -oE '[0-9]{12}' | grep -qvx 111122223333; then
    bad "$name: prints an account id" "$(grep -E '[0-9]{12}' "$out")"
  else
    ok "$name: account ids masked"
  fi
}

all_cases() {
  run "healthy hub" healthy
  expect_status 0
  expect_in "encabulator-ops health  hub  context encabulator-usw2"
  expect_in "OK    aws      cluster encabulator-usw2 ACTIVE in us-west-2; capabilities: argocd ACTIVE, ack ACTIVE, kro ACTIVE"
  expect_in "OK    argocd   9 Applications Synced and Healthy"
  expect_in "OK    ack      11 ACK resources synced"
  expect_in "OK    app      17 of 17 workloads ready in encabulator"
  expect_in "INFO  images   ghcr.io/cloud-native-encabulator/encabulator/<service>:1.0.4"
  expect_in "VERDICT healthy: nothing needs fixing"
  expect_in "(--explain says what each line means"
  expect_not_in "WARN"
  expect_call "kubectl get --raw /readyz"
  expect_call "aws eks describe-capability --region us-west-2 --cluster-name encabulator-usw2 --capability-name kro"

  run "Argo CD first sync retrying" argocd-retrying --explain
  expect_status 0
  expect_in "WARN  argocd   encabulator-use2 OutOfSync/Degraded, retrying: Retrying attempt #1"
  expect_in "it heals in 3 to 5 minutes. Wait."
  expect_in "VERDICT no FAIL, 2 WARN"
  expect_not_in "FAIL  "

  make_world app-degraded <<'EOF'
## applications
encabulator-use2|Synced|Degraded|Succeeded|successfully synced (all tasks run)
hub-cells|Synced|Healthy|Succeeded|successfully synced (all tasks run)
EOF
  run "Argo CD Application Degraded" app-degraded --explain
  expect_status 1
  expect_in "FAIL  argocd   encabulator-use2 is Degraded (Synced)"
  expect_in "A resource of this Application is unhealthy"

  run "ImagePullBackOff on a cell" image-pull --explain
  expect_status 1
  expect_in "encabulator-ops health  cell  context use2"
  expect_in "INFO  argocd   no Argo CD on this cluster"
  expect_in "FAIL  pods     encabulator/dingle-arm-gateway-7f6d5c4b3-h8j9k ImagePullBackOff ghcr.io/cloud-native-encabulator/encabulator/dingle-arm-gateway:1.0.4"
  expect_in "the GHCR package is private"
  expect_in "scripts/mirror-images.sh --check"
  expect_in "FAIL  app      Deployment dingle-arm-gateway 0 of 2 ready"
  expect_in "VERDICT 3 FAIL, 0 WARN: start with pods: encabulator/dingle-arm-gateway"
  expect_no_call "applications.argoproj.io"

  make_world image-missing <<'EOF'
## pods
P|encabulator|dingle-arm-gateway-7f6d5c4b3-h8j9k|Pending|
C|gateway|false|ErrImagePull|0|ghcr.io/cloud-native-encabulator/encabulator/dingle-arm-gateway:1.0.9||failed to resolve reference: ghcr.io/cloud-native-encabulator/encabulator/dingle-arm-gateway:1.0.9: not found
P|encabulator|nofer-trunnion-dispatch-58c9d7f6b-z9x8c|Pending|
C|dispatch|false|ImagePullBackOff|0|ghcr.io/someone-else/encabulator-dispatch:1.0.4||Back-off pulling image
EOF
  run "a tag that does not exist and a wrong prefix" image-missing --explain
  expect_status 1
  expect_in "The tag is not in the registry"
  expect_in "check REGISTRY in .env"
  expect_in "INFO  images   ghcr.io/cloud-native-encabulator/encabulator/<service>:1.0.9 (1)"

  make_world mixed-tags <<'EOF'
## pods
P|encabulator|dingle-arm-gateway-6d9f7c8b5-x2k4p|Running|
C|gateway|true||0|ghcr.io/cloud-native-encabulator/encabulator/dingle-arm-gateway:1.0.4||
P|encabulator|dingle-arm-gateway-8a7b6c5d4-n7m6b|Running|
C|gateway|true||0|ghcr.io/cloud-native-encabulator/encabulator/dingle-arm-gateway:1.0.5||
EOF
  run "two image tags at once" mixed-tags --explain
  expect_status 0
  expect_in "WARN  images   ghcr.io/cloud-native-encabulator/encabulator/<service>:1.0.4 (1), ghcr.io/cloud-native-encabulator/encabulator/<service>:1.0.5 (1)"
  expect_in "a rollout is under way or a tag moved"

  run "ACK snapshot whose cache is gone" ack-snapshot --explain
  expect_status 1
  expect_in "FAIL  ack      ServerlessCacheSnapshot encabulator/automatic.encabulator-valkey-2026-10-15-09-02: ServerlessCacheNotFoundFault"
  expect_in "Adopt it by snapshot name only (docs/teardown/adopt-cache-snapshot.yaml)"
  expect_in "never on stage"

  run "kro cell waiting for the hub's registration" kro-waiting-hub --explain
  expect_status 0
  expect_in "WARN  kro      EncabulatorCell encabulator-cells/encabulator-use2 (cluster ACTIVE) IN_PROGRESS: waiting for external reference hub"
  expect_in "which make gitops-register writes"

  make_world rgd-inactive <<'EOF'
## rgds
encabulatorcell|Inactive|failed to build resourcegraphdefinition: unknown field spec.adminRole
EOF
  run "RGD not Active" rgd-inactive --explain
  expect_status 1
  expect_in "FAIL  kro      RGD encabulatorcell is Inactive: failed to build resourcegraphdefinition"
  expect_in "kro rejected this ResourceGraphDefinition"

  run "CrashLoopBackOff behind NATS" crashloop --explain
  expect_status 1
  expect_in "FAIL  pods     encabulator/panametric-fam-processor-7b6c5d4f9-q8w2e container processor CrashLoopBackOff, 6 restarts, last exit Error"
  expect_in "WARN  pods     encabulator/nats-0 container nats not ready"
  expect_in "start with encabulator/nats-0 container nats not ready (the crashing services depend on it)"
  expect_in "Fix the dependency; the service recovers by itself."

  run "cell not ACTIVE yet" cell-not-active --explain
  expect_status 0
  expect_in "WARN  kro      EncabulatorCell encabulator-cells/encabulator-use2 (cluster CREATING) IN_PROGRESS"
  expect_in "A new cell is IN_PROGRESS for about 20 minutes"
  expect_in "WARN  ack      Cluster encabulator-cells/encabulator-use2 waiting for AWS"

  run "unreachable cluster" unreachable --explain
  expect_status 2
  expect_in "FAIL  cluster  unreachable: error: You must be logged in to the server"
  expect_in "The talk goes on: show the captured runs"
  expect_not_in "aws  "
  if [[ "$(wc -l <"$log" | tr -d ' ')" -eq 2 ]]; then ok "$name: stops after two calls"; else bad "$name: more than two calls" "$(cat "$log")"; fi

  run "EC2 capacity while Auto Mode replaces a node" capacity --explain
  expect_status 0
  expect_in "WARN  events   NodeClaim/encabulator-x7k2p InsufficientCapacityError (2 events, newest)"
  expect_in "You can currently get c6a.large capacity by choosing us-west-2a, us-west-2c."
  expect_in "The full message lists the zones that have it"
  expect_in "OK    app      17 of 17 workloads ready"

  run "Malaise scaled a service to zero" malaise-scaled-zero --explain
  expect_status 0
  expect_in "WARN  app      Deployment sinusoidal-recorder scaled to 0 replicas"
  expect_in "reverts by itself when it expires"

  # An account id that only exists at run time, so no 12-digit id is written in this file.
  local fake; fake="$(printf '%s%s%s' 9876 5432 1098)"
  make_world mask <<EOF
## cluster
arn:aws:eks:us-west-2:$fake:cluster/encabulator-usw2
## pods
P|encabulator|dingle-arm-gateway-6d9f7c8b5-x2k4p|Running|
C|gateway|true||0|$fake.dkr.ecr.us-west-2.amazonaws.com/encabulator/dingle-arm-gateway:1.0.4||
EOF
  run "account ids masked" mask
  expect_status 0
  expect_in "111122223333.dkr.ecr.us-west-2.amazonaws.com/encabulator/<service>:1.0.4"
  expect_not_in "$fake"

  make_world aws-expired <<'EOF'
## aws (error)
An error occurred (ExpiredToken) when calling the DescribeCluster operation: The security token included in the request is expired
EOF
  run "AWS credentials expired, kubectl fine" aws-expired --explain
  expect_status 0
  expect_in "WARN  aws      aws eks describe-cluster failed: An error occurred (ExpiredToken)"
  expect_in "OK    argocd   9 Applications Synced and Healthy"

  run "--no-aws" healthy --no-aws
  expect_status 0
  no_aws_call

  AWS="$work/no-such-aws" run "no AWS CLI" healthy
  expect_status 0
  expect_in "INFO  aws      skipped (no AWS CLI)"

  make_world kind <<'EOF'
## cluster
kind-encabulator
## api-resources
pods
nodes
events
deployments.apps
statefulsets.apps
EOF
  run "a kind cluster" kind
  expect_status 0
  expect_in "INFO  aws      skipped (the context is not an EKS cluster ARN)"
  expect_in "INFO  ack      no ACK resources of this project here"
  expect_no_call "services.k8s.aws"

  run "--context" healthy --context use2 --no-aws
  expect_call "kubectl --context use2 get --raw /readyz"

  run "bad usage" healthy --bogus
  expect_status 64
}

echo "ops-health.sh"
all_cases
echo "$pass passed, $fail failed, $cases runs"
[[ $fail -eq 0 ]] || exit 1
[[ "${1:-}" == "--mutations" ]] || exit 0

# ----------------------------------------------------------------------- mutations
# mutant NAME FROM TO: replace the first FROM with TO in a copy; some case must then fail.
echo
echo "mutation checks (each rule weakened in a copy must make a case fail)"
killed=0 survived=0
mutant() {
  local m="$work/mutant.sh"
  # Through the environment: awk -v would turn the \t in a pattern into a tab.
  FROM="$2" TO="$3" awk 'done == 0 && (i = index($0, ENVIRON["FROM"])) {
      $0 = substr($0, 1, i - 1) ENVIRON["TO"] substr($0, i + length(ENVIRON["FROM"])); done = 1 } { print }' \
    "$here/scripts/ops-health.sh" >"$m"
  chmod 755 "$m"
  if cmp -s "$m" "$here/scripts/ops-health.sh"; then
    printf '  FAIL  %s: the mutation did not apply\n' "$1"; survived=$((survived + 1)); return
  fi
  if ( script="$m"; quiet=true; all_cases ) >/dev/null 2>&1; then
    printf '  FAIL  survived: %s\n' "$1"; survived=$((survived + 1))
  else
    printf '  ok    killed: %s\n' "$1"; killed=$((killed + 1))
  fi
}
mutant "ImagePullBackOff not detected" 'ImagePullBackOff|ErrImagePull|InvalidImageName' 'NoSuchReason'
mutant "a 401 is not read as a private package" '401|403|[Uu]nauthorized|denied|forbidden' 'NoSuchAnswer'
mutant "CrashLoopBackOff not detected" 'reason == "CrashLoopBackOff"' 'reason == "NoSuchReason"'
mutant "the store before its dependents" 'grep -q $'"'"'\tpods.crashloop\t'"'"' "$findings"' 'false'
mutant "Argo CD Degraded not detected" 'health == "Degraded"' 'health == "NoSuchHealth"'
mutant "Argo CD retry treated as a failure" 'phase == "Running" && msg ~ /[Rr]etrying/' 'phase == "NoSuchPhase"'
mutant "RGD state not checked" 'if ($2 != "Active")' 'if (0)'
mutant "kro waiting for the hub not recognized" '/external reference hub/' '/no such message/'
mutant "kro IN_PROGRESS not reported" 'if (state == "ACTIVE") next' 'next'
mutant "ACK snapshot fault not recognized" '/ServerlessCacheNotFoundFault/' '/NoSuchFault/'
mutant "ACK pending not recognized" '/pending|is not available|being created|CREATING/' '/no such state/'
mutant "scaled to zero not reported" 'want == 0 {' 'want == -1 {'
mutant "capacity events not recognized" '/InsufficientInstanceCapacity|insufficient capacity|InsufficientCapacity/' '/no such event/'
mutant "an unreachable cluster exits 0" 'return 2' 'return 0'
mutant "account ids not masked" "mask() { sed" "mask() { cat; }; unused() { sed"
mutant "a Secret is read" 'kget nodes -o' 'kget secrets,nodes -o'
mutant "AWS failure stops the check" 'add WARN aws aws.unavailable' 'exit 1; add WARN aws aws.unavailable'
mutant "mixed image tags not reported" '(nt > 1 ? "WARN" : "INFO")' '"INFO"'
mutant "a wrong image prefix not recognized" 'image !~ ("^" prefix) && image !~ /\.dkr\.ecr\./' '0'
echo "$killed killed, $survived survived"
[[ $survived -eq 0 ]]
