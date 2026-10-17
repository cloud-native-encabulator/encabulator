#!/usr/bin/env bash
# Contract test for the jsonpath templates in ../scripts/ops-health.sh, with no cluster.
#
#   tests/jsonpath_test.sh
#
# The fire-drill worlds hold what kubectl prints for each template. This test proves that shape:
# it renders every template over a real-shaped object in tests/objects/ with kubectl's own
# jsonpath engine (`kubectl label --local`, which never contacts an API server; KUBECONFIG is an
# empty file), compares the line with tests/objects/<name>.expected (`|` stands for a tab), and
# then runs ops-health.sh on a world built from those rendered lines.
# Without kubectl on PATH it says so and exits 0 (1 with REQUIRE_KUBECTL=1).
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$here/scripts/ops-health.sh"
objects="$here/tests/objects"
work="$(mktemp -d "${TMPDIR:-/tmp}/jsonpath-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT

if ! command -v kubectl >/dev/null 2>&1; then
  # CI sets REQUIRE_KUBECTL=1, so a runner without kubectl fails instead of skipping.
  echo "jsonpath_test: SKIP, kubectl is not installed"
  [[ "${REQUIRE_KUBECTL:-}" == "1" ]] && exit 1
  exit 0
fi
for v in $(env | sed -n 's/^\(AWS_[A-Z_]*\)=.*/\1/p'); do unset "$v"; done
: >"$work/kubeconfig"
export KUBECONFIG="$work/kubeconfig"

# The templates, exactly as the script defines them.
eval "$(grep -E '^(CONTAINER_FIELDS|[A-Z]+_ITEM)=' "$script")"

pass=0 fail=0
ok()  { pass=$((pass + 1)); printf '  ok    %s\n' "$1"; }
bad() { fail=$((fail + 1)); printf '  FAIL  %s\n' "$1"; [[ -z "${2:-}" ]] || printf '%s\n' "$2" | sed 's/^/          /'; }

echo "jsonpath templates ($(kubectl version --client 2>/dev/null | head -n 1))"
world="$work/drills/rendered/world.txt"
mkdir -p "$work/drills/rendered" "$work/drills/healthy"
cp "$here/fixtures/fire-drill/healthy/world.txt" "$work/drills/healthy/world.txt"
echo "## base healthy" >"$world"
for pair in apps:APPS_ITEM rgds:RGDS_ITEM kro:KRO_ITEM ack:ACK_ITEM ready:READY_ITEM \
            nodes:NODES_ITEM workloads:WORK_ITEM pods:PODS_ITEM events:EVENTS_ITEM; do
  name="${pair%%:*}" var="${pair#*:}"
  if ! got="$(kubectl label --local -f "$objects/$name.json" ops-health-test=1 -o "jsonpath=${!var}" 2>&1)"; then
    bad "$var renders $name.json" "$got"; continue
  fi
  want="$(tr '|' '\t' <"$objects/$name.expected")"
  if [[ "$got" == "$want" ]]; then ok "$var renders $name.json as expected"; else
    bad "$var renders $name.json" "got:  $(printf '%s' "$got" | tr '\t' '|')"$'\n'"want: $(cat "$objects/$name.expected")"
  fi
  section="$name"; [[ "$name" == "apps" ]] && section="applications"
  { echo "## $section"; printf '%s\n' "$got" | tr '\t' '|'; } >>"$world"
done

# What the script makes of kubectl's own output.
stub="$here/fixtures/fire-drill/stub.sh"
mkdir -p "$work/bin"
printf '#!/usr/bin/env bash\nSTUB_TOOL=kubectl exec "%s" "$@"\n' "$stub" >"$work/bin/kubectl"
chmod 755 "$work/bin/kubectl"
out="$work/out"
DRILL_WORLD="$world" KUBECTL="$work/bin/kubectl" "$script" --no-aws >"$out" 2>&1
status=$?
for want in \
  "WARN  argocd   encabulator-use2 OutOfSync/Degraded, retrying: Retrying attempt #1" \
  "FAIL  kro      RGD encabulatorcell is Inactive: failed to build resourcegraphdefinition" \
  "WARN  kro      EncabulatorCell encabulator-cells/encabulator-use2 (cluster ACTIVE) IN_PROGRESS: waiting for external reference hub" \
  "FAIL  ack      ServerlessCacheSnapshot encabulator/automatic.encabulator-valkey-2026-10-15-09-02: ServerlessCacheNotFoundFault" \
  "WARN  nodes    NodePool encabulator is not Ready: NodeClassReady=False" \
  "OK    nodes    1 of 1 nodes Ready" \
  "FAIL  app      Deployment dingle-arm-gateway 0 of 2 ready" \
  "FAIL  pods     encabulator/panametric-fam-processor-7b6c5d4f9-q8w2e container processor CrashLoopBackOff, 6 restarts, last exit Error" \
  "INFO  events   Pod/dingle-arm-gateway-7f6d5c4b3-h8j9k BackOff"; do
  if grep -qF -- "$want" "$out"; then ok "reads kubectl's output: $want"; else bad "reads kubectl's output: $want" "$(cat "$out")"; fi
done
[[ $status -eq 1 ]] && ok "exit 1 on kubectl's output" || bad "exit $status on kubectl's output, want 1"
echo "$pass passed, $fail failed"
[[ $fail -eq 0 ]]
