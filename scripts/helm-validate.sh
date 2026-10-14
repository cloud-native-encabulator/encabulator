#!/usr/bin/env bash
# Lint charts/encabulator, render it across several value combinations, and validate every
# rendered object against the Kubernetes and CRD JSON schemas.
#
# Runs with KUBECONFIG=/dev/null so `lookup` cannot reach a cluster: this must never touch
# one. Used by `make lint-charts` and by .github/workflows/ci.yml.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
chart="$repo_root/charts/encabulator"

: "${HELM:=helm}"
: "${KUBECONFORM:=kubeconform}"
: "${KUBERNETES_VERSION:=1.31.0}"

export KUBECONFIG=/dev/null

log()  { printf '\033[1m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[31m==> %s\033[0m\n' "$*" >&2; failures=$((failures + 1)); }

failures=0
have_kubeconform=1
command -v "$KUBECONFORM" >/dev/null 2>&1 || {
  printf '\033[33m==> kubeconform not found; schema validation skipped.\033[0m\n' >&2
  printf '    install it with: go install github.com/yannh/kubeconform/cmd/kubeconform@latest\n' >&2
  have_kubeconform=0
}

# The CRDs the chart uses (ServiceMonitor, PrometheusRule, ScaledObject) are not in the
# upstream Kubernetes schemas, so point kubeconform at the community CRD catalogue too.
crd_schema='https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

log "helm lint"
$HELM lint "$chart" || fail "helm lint"

# A values file for the extraScenarios hook point, which is awkward to express with --set.
extra_scenarios_values="$(mktemp -t encabulator-scenarios.XXXXXX).yaml"
cat >"$extra_scenarios_values" <<'YAML'
extraScenarios:
  side-fumbling-marathon.yaml: |
    id: side-fumbling-marathon
    title: Side fumbling marathon
    description: Tighten inspection and leave it tightened.
    steps:
      - say: "Tightening the side fumbling thresholds."
      - flag: { key: strict-side-fumbling, value: true }
      - wait: 60s
YAML
trap 'rm -f "$extra_scenarios_values"' EXIT

# Each case is "description|helm flags..."
cases=(
  "defaults|"
  "keda and ingress on|--set keda.enabled=true --set ingress.enabled=true --set ingress.hosts[0].host=works.example.com"
  "nginx ingress|--set ingress.enabled=true --set ingress.className=nginx --set ingress.hosts[0].host=works.internal"
  "every store external|--set postgres.external.enabled=true --set postgres.external.host=pg.rds.example.com --set postgres.external.existingSecret=rds-creds --set valkey.external.enabled=true --set valkey.external.host=cache.example.com --set valkey.external.tls=true --set valkey.external.existingSecret=elasticache-creds --set nats.external.enabled=true --set nats.external.url=nats://mq.example.com:4222 --set nats.external.monitoringEndpoint=mq.example.com:8222 --set influxdb.external.enabled=true --set influxdb.external.url=https://ts.example.com:8086 --set influxdb.external.existingSecret=timestream-token"
  "no prometheus operator CRDs|--set observability.serviceMonitor.enabled=false --set observability.prometheusRule.enabled=false"
  "tracing disabled|--set observability.otlpEndpoint="
  "ephemeral stores, influx without auth|--set postgres.persistence.enabled=false --set valkey.persistence.enabled=false --set nats.persistence.enabled=false --set influxdb.persistence.enabled=false --set influxdb.auth.enabled=false"
  "autoscaling off everywhere|--set services.panametric-fam-processor.autoscaling.enabled=false --set services.spurving-bearing-quoter.autoscaling.enabled=false"
  "everything autoscaled|--set defaults.autoscaling.enabled=true --set keda.enabled=true"
  "externally managed secrets|--set secrets.existingSecret=encabulator-app-managed"
  "single service only|--set services.encabulation-control-console.enabled=false --set services.cardinal-grammeter.enabled=false --set services.demo-scenario-orchestrator.enabled=false"
  "extra scenarios configmap|--values $extra_scenarios_values"
)

for entry in "${cases[@]}"; do
  desc="${entry%%|*}"
  flags="${entry#*|}"
  # kubeconform filters by file extension, so the temp file needs one.
  rendered="$(mktemp -t encabulator-render.XXXXXX).yaml"
  # shellcheck disable=SC2086
  if ! $HELM template encabulator "$chart" --namespace encabulator $flags >"$rendered" 2>&1; then
    fail "template [$desc]"
    sed -n '1,20p' "$rendered" >&2
    rm -f "$rendered"
    continue
  fi
  count=$(grep -c '^kind:' "$rendered" || true)
  if [ "$have_kubeconform" = 1 ]; then
    if out=$($KUBECONFORM -strict -summary \
               -kubernetes-version "$KUBERNETES_VERSION" \
               -schema-location default \
               -schema-location "$crd_schema" \
               "$rendered" 2>&1); then
      log "ok  [$desc] $count objects :: ${out##*$'\n'}"
    else
      fail "kubeconform [$desc]"
      printf '%s\n' "$out" >&2
    fi
  else
    log "ok  [$desc] $count objects (rendered only)"
  fi
  rm -f "$rendered"
done

# volumeClaimTemplates are immutable: a release that renders them differently cannot upgrade an
# existing StatefulSet. Render two image tags and require byte-identical templates.
vct() {
  # shellcheck disable=SC2086
  $HELM template encabulator "$chart" --namespace encabulator "$@" \
    | awk '/^kind: StatefulSet/{s=1} /^---/{s=0; v=0} s && /^  volumeClaimTemplates:/{v=1} v'
}
if a="$(vct --set global.imageTag=0.0.1)" && b="$(vct --set global.imageTag=9.9.9)"; then
  if [ -z "$a" ]; then
    fail "volumeClaimTemplates: none rendered"
  elif [ "$a" != "$b" ]; then
    fail "volumeClaimTemplates change with the image tag (StatefulSet upgrades would fail)"
    diff <(printf '%s\n' "$a") <(printf '%s\n' "$b") >&2 || true
  elif grep -q 'helm.sh/chart\|app.kubernetes.io/version' <<<"$a"; then
    fail "volumeClaimTemplates carry a release label (helm.sh/chart or app.kubernetes.io/version)"
  else
    log "ok  [volumeClaimTemplates stable across releases]"
  fi
else
  fail "template [volumeClaimTemplates]"
fi

# InfluxDB's WAL flush interval decides how long every recorder write waits. The default must
# render as INFLUXDB3_WAL_FLUSH_INTERVAL=50ms, an empty value must leave InfluxDB's own default,
# and an external InfluxDB must not render the StatefulSet at all.
influx_env() {
  # shellcheck disable=SC2086
  $HELM template encabulator "$chart" --namespace encabulator "$@" \
    | awk '/^kind: StatefulSet/{s=1} /^---/{s=0} s' \
    | awk '/^  name: influxdb$/{f=1} f' \
    | grep -A1 'name: INFLUXDB3_WAL_FLUSH_INTERVAL' | grep 'value:' | tr -d ' "' || true
}
if [ "$(influx_env)" != "value:50ms" ]; then
  fail "influxdb: INFLUXDB3_WAL_FLUSH_INTERVAL is not 50ms by default (got '$(influx_env)')"
elif [ -n "$(influx_env --set influxdb.walFlushInterval=)" ]; then
  fail "influxdb: an empty walFlushInterval still renders INFLUXDB3_WAL_FLUSH_INTERVAL"
elif [ "$(influx_env --set influxdb.walFlushInterval=250ms)" != "value:250ms" ]; then
  fail "influxdb: walFlushInterval=250ms does not reach the StatefulSet"
else
  log "ok  [influxdb WAL flush interval: 50ms default, empty omits it, override applied]"
fi

# InfluxDB 3 Core refuses a query that would scan more Parquet files than its limit (432 by
# default, 7.2 hours of data at the chart's 1m gen1). The chart default must render as
# INFLUXDB3_QUERY_FILE_LIMIT=4320, an empty value must leave InfluxDB's own default.
influx_file_limit() {
  # shellcheck disable=SC2086
  $HELM template encabulator "$chart" --namespace encabulator "$@" \
    | awk '/^kind: StatefulSet/{s=1} /^---/{s=0} s' \
    | awk '/^  name: influxdb$/{f=1} f' \
    | grep -A1 'name: INFLUXDB3_QUERY_FILE_LIMIT' | grep 'value:' | tr -d ' "' || true
}
if [ "$(influx_file_limit)" != "value:4320" ]; then
  fail "influxdb: INFLUXDB3_QUERY_FILE_LIMIT is not 4320 by default (got '$(influx_file_limit)')"
elif [ -n "$(influx_file_limit --set influxdb.queryFileLimit=)" ]; then
  fail "influxdb: an empty queryFileLimit still renders INFLUXDB3_QUERY_FILE_LIMIT"
elif [ "$(influx_file_limit --set-string influxdb.queryFileLimit=1000)" != "value:1000" ]; then
  fail "influxdb: queryFileLimit=1000 does not reach the StatefulSet"
else
  log "ok  [influxdb query file limit: 4320 default, empty omits it, override applied]"
fi

# KEDA scales the casing line. Its operator polls NATS from the keda namespace, so the endpoint
# must carry the release namespace (a bare "nats" resolves to nats.keda). And the Deployment must
# leave `replicas` out, or the next helm upgrade fights the HPA KEDA owns.
rendered_object() {
  local kind="$1" name="$2" ns="$3"
  shift 3
  # shellcheck disable=SC2086
  $HELM template encabulator "$chart" --namespace "$ns" "$@" \
    | awk -v kind="kind: $kind" -v name="  name: $name" '
        function flush() { if (k && n) printf "%s", doc; doc = ""; k = 0; n = 0 }
        /^---/ { flush(); next }
        { doc = doc $0 "\n" }
        $0 == kind { k = 1 }
        $0 == name { n = 1 }
        END { flush() }'
}
keda_endpoint() {
  rendered_object ScaledObject logarithmic-casing-line "$1" --set keda.enabled=true \
    | grep 'natsServerMonitoringEndpoint:' | tr -d ' "' || true
}
casing_replicas() {
  rendered_object Deployment logarithmic-casing-line encabulator "$@" | grep -c '^  replicas:' || true
}
if [ "$(keda_endpoint encabulator)" != "natsServerMonitoringEndpoint:nats.encabulator.svc:8222" ]; then
  fail "keda: ScaledObject NATS endpoint is not nats.encabulator.svc:8222 (got '$(keda_endpoint encabulator)')"
elif [ "$(keda_endpoint factory)" != "natsServerMonitoringEndpoint:nats.factory.svc:8222" ]; then
  fail "keda: ScaledObject NATS endpoint does not follow the release namespace (got '$(keda_endpoint factory)')"
elif [ "$(casing_replicas --set keda.enabled=true)" != "0" ]; then
  fail "keda: the casing-line Deployment renders replicas while KEDA owns them"
elif [ "$(casing_replicas --set keda.enabled=false)" != "1" ]; then
  fail "keda: the casing-line Deployment does not render replicas when KEDA is off"
else
  log "ok  [keda: NATS endpoint carries the release namespace, casing line leaves replicas to KEDA]"
fi

if [ "$failures" -ne 0 ]; then
  printf '\033[31m==> %d check(s) failed\033[0m\n' "$failures" >&2
  exit 1
fi
log "all chart checks passed"
