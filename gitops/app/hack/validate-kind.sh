#!/usr/bin/env bash
# Validate gitops/app end to end on a local kind cluster, with no AWS account.
#
#   gitops/app/hack/validate-kind.sh --kubeconfig PATH [--evidence DIR]
#
# What it does:
#   1. refuses any cluster whose API server is not on 127.0.0.1 (kind), so it can never
#      touch an EKS cluster
#   2. installs kro 0.9.4 and the ACK rds + elasticache CRDs only if they are missing
#      (CRDs only: no ACK controller, nothing talks to AWS)
#   3. applies rbac/ and rgds/ and waits for both RGDs to be Active, which proves the CEL
#      type-checks against the real ACK schemas
#   4. applies the test fixtures (hack/kind/fake-aws.yaml) and the instance
#   5. fakes the ACK status kro waits for: DBInstance available with an endpoint (pointing
#      at fake-rds, so db-init runs for real) and ServerlessCache available
#   6. waits for the instance to go ACTIVE and counts the 15 service Deployments
#   7. runs parity.py (kro vs helm template) and rbac-check.py (gitops/app/rbac is enough)
#
# Leaves everything in place for inspection; delete the instance and the namespace yourself.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
app="$(cd "$here/.." && pwd)"

KRO_VERSION=0.9.4
ACK_RDS_VERSION=1.13.1
ACK_ELASTICACHE_VERSION=1.10.1
NS=encabulator

kubeconfig="" evidence=""
while [ $# -gt 0 ]; do
  case "$1" in
    --kubeconfig) kubeconfig="$2"; shift 2 ;;
    --evidence) evidence="$2"; shift 2 ;;
    *) echo "usage: $0 --kubeconfig PATH [--evidence DIR]" >&2; exit 2 ;;
  esac
done
[ -n "$kubeconfig" ] || { echo "usage: $0 --kubeconfig PATH [--evidence DIR]" >&2; exit 2; }
evidence="${evidence:-$(mktemp -d)}"
mkdir -p "$evidence"

k() { kubectl --kubeconfig "$kubeconfig" "$@"; }
log() { printf '\033[1m==>\033[0m %s\n' "$*"; }
save() { local name="$1"; shift; "$@" >"$evidence/$name" 2>&1 || true; }

server="$(k config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
case "$server" in
  https://127.0.0.1:*|https://localhost:*) ;;
  *) echo "refusing: $server is not a local kind API server" >&2; exit 1 ;;
esac
log "cluster $server"

if ! k get crd resourcegraphdefinitions.kro.run >/dev/null 2>&1; then
  log "installing kro $KRO_VERSION"
  helm --kubeconfig "$kubeconfig" install kro oci://registry.k8s.io/kro/charts/kro \
    --version "$KRO_VERSION" --namespace kro-system --create-namespace --wait
else
  log "kro already installed: $(k get deploy -A -l app.kubernetes.io/name=kro \
    -o jsonpath='{.items[0].spec.template.spec.containers[0].image}' 2>/dev/null || echo unknown)"
fi

install_ack_crds() {
  local chart="$1" version="$2" group="$3" probe="$4" tmp
  if k get crd "$probe" >/dev/null 2>&1; then
    log "ACK $group CRDs present"
    return
  fi
  log "installing ACK $group CRDs from $chart $version (CRDs only, no controller)"
  tmp="$(mktemp -d)"
  helm pull "oci://public.ecr.aws/aws-controllers-k8s/$chart" --version "$version" --untar --untardir "$tmp"
  # Only this service's CRDs; the shared services.k8s.aws ones may belong to another controller.
  for crd in "$tmp/$chart/crds/$group"_*.yaml; do
    k apply --server-side -f "$crd" >/dev/null
  done
  rm -r "$tmp"
}
install_ack_crds rds-chart "$ACK_RDS_VERSION" rds.services.k8s.aws dbinstances.rds.services.k8s.aws
install_ack_crds elasticache-chart "$ACK_ELASTICACHE_VERSION" elasticache.services.k8s.aws serverlesscaches.elasticache.services.k8s.aws

log "applying rbac/ and rgds/"
k apply -f "$app/rbac/"
k apply -f "$app/rgds/"
for rgd in encabulator-service encabulator; do
  k wait --for=jsonpath='{.status.state}'=Active "rgd/$rgd" --timeout=120s
done
save rgds.txt k get rgd encabulator encabulator-service -o wide
save rgd-status.yaml k get rgd encabulator encabulator-service -o yaml
save explain-encabulator-spec.txt k explain encabulator.spec

log "applying the test fixtures and the instance"
k apply -f "$here/kind/fake-aws.yaml"
k apply -k "$app/instance"

wait_for() { # wait_for <description> <command...>
  local desc="$1"; shift
  for _ in $(seq 1 120); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 5
  done
  echo "timed out waiting for $desc" >&2
  return 1
}

log "waiting for kro to create the ServerlessCache and the DBInstance"
wait_for "serverlesscache" k -n "$NS" get serverlesscache encabulator-valkey
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
synced='[{"type":"ACK.ResourceSynced","status":"True","lastTransitionTime":"'"$now"'"}]'
k -n "$NS" patch serverlesscache encabulator-valkey --subresource=status --type=merge -p \
  '{"status":{"status":"available","endpoint":{"address":"encabulator-valkey-fake.serverless.use2.cache.amazonaws.com","port":6379},"conditions":'"$synced"'}}'
# The DBInstance only appears after secret-init has created its password Secret.
wait_for "dbinstance (after secret-init)" k -n "$NS" get dbinstance encabulator-pg
k -n "$NS" patch dbinstance encabulator-pg --subresource=status --type=merge -p \
  '{"status":{"dbInstanceStatus":"available","endpoint":{"address":"fake-rds.'"$NS"'.svc.cluster.local","port":5432},"conditions":'"$synced"'}}'

log "waiting for db-init against fake-rds and for the instance to go ACTIVE"
wait_for "db-init" k -n "$NS" wait --for=condition=complete job/encabulator-db-init --timeout=5s
wait_for "instance ACTIVE" sh -c "kubectl --kubeconfig '$kubeconfig' -n $NS get encabulator encabulator -o jsonpath='{.status.state}' | grep -qx ACTIVE"

deployments="$(k -n "$NS" get deploy -l app.kubernetes.io/component=service,app.kubernetes.io/part-of=encabulator -o name | wc -l)"
log "service Deployments: $deployments"

save instance.yaml k -n "$NS" get encabulator encabulator -o yaml
save encabulatorservices.txt k -n "$NS" get encabulatorservices
save deployments.txt k -n "$NS" get deploy -o wide
save objects.txt k -n "$NS" get sa,svc,deploy,sts,hpa,pdb,role,rolebinding,job,configmap,dbinstance,dbsubnetgroup,serverlesscache
save storageclass.txt k get storageclass encabulator-gp3 -o yaml
save configmap-encabulator-aws.yaml k -n "$NS" get configmap encabulator-aws -o yaml
save secret-init.log k -n "$NS" logs job/encabulator-secret-init
save db-init.log k -n "$NS" logs job/encabulator-db-init
save secrets-keys.txt sh -c "kubectl --kubeconfig '$kubeconfig' -n $NS get secret encabulator-db encabulator-app encabulator-valkey encabulator-influxdb -o go-template='{{range .items}}{{.metadata.name}}:{{range \$k, \$v := .data}} {{\$k}}{{end}}{{\"\\n\"}}{{end}}'"
save fake-rds-databases.txt k -n "$NS" exec deploy/fake-rds -- psql -U rdsadmin -d postgres -c '\l' -c '\du'

status=0
log "parity: kro vs helm template"
python3 "$here/parity.py" --kubeconfig "$kubeconfig" --namespace "$NS" --report "$evidence/parity.txt" || status=1
log "rbac: replay as group encabulator:kro"
python3 "$here/rbac-check.py" --kubeconfig "$kubeconfig" --namespace "$NS" >"$evidence/rbac-check.txt" 2>&1 || status=1
tail -n 3 "$evidence/rbac-check.txt"

if [ "$deployments" -ne 15 ]; then
  echo "expected 15 service Deployments, found $deployments" >&2
  status=1
fi
log "evidence in $evidence"
exit "$status"
