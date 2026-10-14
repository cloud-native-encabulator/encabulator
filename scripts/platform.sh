#!/usr/bin/env bash
# Install, template or uninstall the observability platform and KEDA.
#
#   scripts/platform.sh install     add repos, install the four charts in order, then the dashboards
#   scripts/platform.sh template    render every chart with its pinned values (no cluster needed)
#   scripts/platform.sh template-cell  render what Argo CD installs on a cell, and check its pins
#   scripts/platform.sh dashboards  (re)create the Grafana dashboard ConfigMaps only
#   scripts/platform.sh status       what is installed where
#   scripts/platform.sh uninstall   remove them again, in reverse order
#
# Order matters. kube-prometheus-stack brings the Prometheus Operator CRDs, which the
# collector's and Jaeger's ServiceMonitors need. KEDA brings the ScaledObject CRD, which
# charts/encabulator needs before `--set keda.enabled=true` will apply.
#
# Versions live in platform/versions.env. Nothing here is unpinned.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
platform_dir="$repo_root/platform"
# shellcheck source=../platform/versions.env
source "$platform_dir/versions.env"

: "${HELM:=helm}"
: "${KUBECTL:=kubectl}"
: "${HELM_TIMEOUT:=10m}"

# Progress goes to stderr so `scripts/platform.sh template` emits clean YAML on stdout.
log()  { printf '\033[1m==>\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[33m==> %s\033[0m\n' "$*" >&2; }
die()  { printf '\033[31m==> %s\033[0m\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is not installed"; }

add_repos() {
  log "adding pinned chart repositories"
  $HELM repo add prometheus-community "$KUBE_PROMETHEUS_STACK_REPO" >/dev/null
  $HELM repo add open-telemetry "$OPENTELEMETRY_COLLECTOR_REPO" >/dev/null
  $HELM repo add jaegertracing "$JAEGER_REPO" >/dev/null
  $HELM repo add kedacore "$KEDA_REPO" >/dev/null
  $HELM repo update prometheus-community open-telemetry jaegertracing kedacore >/dev/null
}

# install <release> <chart> <version> <namespace> <values-file>
install_chart() {
  local release="$1" chart="$2" version="$3" namespace="$4" values="$5"
  log "installing $chart@$version as $release in $namespace"
  $HELM upgrade --install "$release" "$chart" \
    --version "$version" \
    --namespace "$namespace" --create-namespace \
    --values "$values" \
    --wait --timeout "$HELM_TIMEOUT"
}

# template <release> <chart> <version> <namespace> <values-file> [extra helm flags...]
template_chart() {
  local release="$1" chart="$2" version="$3" namespace="$4" values="$5"
  shift 5
  $HELM template "$release" "$chart" \
    --version "$version" --namespace "$namespace" --values "$values" "$@"
}

dashboards() {
  local ns="$OBSERVABILITY_NAMESPACE"
  log "provisioning Grafana dashboards into $ns"
  $KUBECTL create namespace "$ns" --dry-run=client -o yaml | $KUBECTL apply -f - >/dev/null

  for file in "$platform_dir"/dashboards/*.json; do
    local base name
    base="$(basename "$file")"
    name="encabulator-dashboard-${base%.json}"
    # The Grafana sidecar watches for grafana_dashboard=1 in every namespace and reads the
    # grafana_folder annotation to decide where the dashboard lands.
    $KUBECTL create configmap "$name" \
      --namespace "$ns" \
      --from-file="$base=$file" \
      --dry-run=client -o yaml \
      | $KUBECTL label --local -f - -o yaml \
          grafana_dashboard=1 \
          app.kubernetes.io/part-of=encabulator \
      | $KUBECTL annotate --local -f - -o yaml \
          grafana_folder=Encabulator \
      | $KUBECTL apply -f -
  done
}

cmd_install() {
  need "$HELM"; need "$KUBECTL"
  add_repos

  # 1. Prometheus Operator CRDs, Prometheus, Alertmanager, Grafana, the exporters.
  install_chart "$KUBE_PROMETHEUS_STACK_RELEASE" prometheus-community/kube-prometheus-stack \
    "$KUBE_PROMETHEUS_STACK_VERSION" "$OBSERVABILITY_NAMESPACE" \
    "$platform_dir/kube-prometheus-stack.values.yaml"

  # 2. Jaeger before the collector, so the collector's OTLP exporter has somewhere to go.
  install_chart "$JAEGER_RELEASE" jaegertracing/jaeger \
    "$JAEGER_VERSION" "$OBSERVABILITY_NAMESPACE" \
    "$platform_dir/jaeger.values.yaml"

  # 3. The collector: OTLP in, Jaeger and a Prometheus endpoint out.
  install_chart "$OPENTELEMETRY_COLLECTOR_RELEASE" open-telemetry/opentelemetry-collector \
    "$OPENTELEMETRY_COLLECTOR_VERSION" "$OBSERVABILITY_NAMESPACE" \
    "$platform_dir/opentelemetry-collector.values.yaml"

  # 4. KEDA, in its own namespace, for the casing line's JetStream scaler.
  install_chart "$KEDA_RELEASE" kedacore/keda \
    "$KEDA_VERSION" "$KEDA_NAMESPACE" \
    "$platform_dir/keda.values.yaml"

  dashboards

  log "platform installed"
  cmd_status
  cat <<EOF

Next:
  make deploy                     # charts/encabulator (add KEDA_ENABLED=true for the scaler)
  make grafana                    # port-forward Grafana to http://localhost:3000
  make jaeger                     # port-forward Jaeger  to http://localhost:16686

Grafana admin password:
  kubectl -n $OBSERVABILITY_NAMESPACE get secret ${KUBE_PROMETHEUS_STACK_RELEASE}-grafana \\
    -o jsonpath='{.data.admin-password}' | base64 -d; echo
EOF
}

cmd_template() {
  need "$HELM"
  add_repos
  template_chart "$KUBE_PROMETHEUS_STACK_RELEASE" prometheus-community/kube-prometheus-stack \
    "$KUBE_PROMETHEUS_STACK_VERSION" "$OBSERVABILITY_NAMESPACE" \
    "$platform_dir/kube-prometheus-stack.values.yaml"
  template_chart "$JAEGER_RELEASE" jaegertracing/jaeger \
    "$JAEGER_VERSION" "$OBSERVABILITY_NAMESPACE" "$platform_dir/jaeger.values.yaml"
  template_chart "$OPENTELEMETRY_COLLECTOR_RELEASE" open-telemetry/opentelemetry-collector \
    "$OPENTELEMETRY_COLLECTOR_VERSION" "$OBSERVABILITY_NAMESPACE" \
    "$platform_dir/opentelemetry-collector.values.yaml"
  template_chart "$KEDA_RELEASE" kedacore/keda \
    "$KEDA_VERSION" "$KEDA_NAMESPACE" "$platform_dir/keda.values.yaml"
}

# The cell's platform entries in gitops/hub/appset.yaml (ApplicationSet encabulator-platform), one
# "release chart repoURL version" line per chart entry.
appset_entries() {
  awk '
    /^kind: ApplicationSet/ { doc = 0 }
    /^  name: encabulator-platform$/ { doc = 1 }
    !doc { next }
    /^ *- component:/ { if (chart != "") print release, chart, repo, version; release = chart = repo = version = "" }
    /^ *release:/ { release = $2 }
    /^ *chart:/   { chart = $2; gsub(/"/, "", chart) }
    /^ *repoURL:/ { repo = $2 }
    /^ *version:/ { version = $2 }
    /^ *- clusters:/ { if (chart != "") print release, chart, repo, version; release = chart = ""; doc = 0 }
  ' "$repo_root/gitops/hub/appset.yaml"
}

# Fail unless the cell entries pin exactly what versions.env pins, with the same release names.
check_cell_pins() {
  local entries want failures=0
  entries="$(appset_entries)"
  [ -n "$entries" ] || die "no chart entries found in gitops/hub/appset.yaml"
  for want in \
    "$KUBE_PROMETHEUS_STACK_RELEASE kube-prometheus-stack $KUBE_PROMETHEUS_STACK_REPO $KUBE_PROMETHEUS_STACK_VERSION" \
    "$JAEGER_RELEASE jaeger $JAEGER_REPO $JAEGER_VERSION" \
    "$OPENTELEMETRY_COLLECTOR_RELEASE opentelemetry-collector $OPENTELEMETRY_COLLECTOR_REPO $OPENTELEMETRY_COLLECTOR_VERSION"; do
    if grep -qxF "$want" <<<"$entries"; then
      log "cell pin ok: $want"
    else
      warn "gitops/hub/appset.yaml does not pin: $want"
      failures=$((failures + 1))
    fi
  done
  # Every chart entry must be one of the above: nothing on a cell that versions.env does not pin.
  if [ "$(wc -l <<<"$entries")" -ne 3 ]; then
    warn "gitops/hub/appset.yaml has chart entries versions.env does not pin:"
    printf '%s\n' "$entries" >&2
    failures=$((failures + 1))
  fi
  [ "$failures" -eq 0 ] || die "cell platform pins differ from platform/versions.env"
}

# The document whose metadata.name is $1, from a multi-document YAML stream on stdin.
yaml_doc() {
  awk -v want="  name: $1" '
    /^---$/ { if (hit) printf "%s", buf; buf = ""; hit = 0; next }
    { buf = buf $0 "\n"; if ($0 == want) hit = 1 }
    END { if (hit) printf "%s", buf }'
}

# The dashboards a cell gets (platform/kustomization.yaml) must equal what `dashboards` creates.
check_cell_dashboards() {
  local file base name built made
  built="$($KUBECTL kustomize "$platform_dir")"
  for file in "$platform_dir"/dashboards/*.json; do
    base="$(basename "$file")"
    name="encabulator-dashboard-${base%.json}"
    made="$($KUBECTL create configmap "$name" --namespace "$OBSERVABILITY_NAMESPACE" \
              --from-file="$base=$file" --dry-run=client -o yaml \
            | $KUBECTL label --local -f - -o yaml grafana_dashboard=1 app.kubernetes.io/part-of=encabulator \
            | $KUBECTL annotate --local -f - -o yaml grafana_folder=Encabulator \
            | grep -v '^  creationTimestamp: null$')"
    if ! diff <(printf '%s\n' "$made") <(yaml_doc "$name" <<<"$built") >&2; then
      die "platform/kustomization.yaml renders $name differently from scripts/platform.sh dashboards"
    fi
  done
  log "cell dashboards ok: platform/kustomization.yaml equals scripts/platform.sh dashboards"
}

# What Argo CD installs on a cell: each chart with the common values plus platform/cell/, CRDs
# included (Argo CD renders them), then the kustomize folder with the dashboards and the
# services' ServiceMonitor. No cluster needed.
cmd_template_cell() {
  need "$HELM"; need "$KUBECTL"
  check_cell_pins
  check_cell_dashboards
  add_repos
  local cell="$platform_dir/cell"
  template_chart "$KUBE_PROMETHEUS_STACK_RELEASE" prometheus-community/kube-prometheus-stack \
    "$KUBE_PROMETHEUS_STACK_VERSION" "$OBSERVABILITY_NAMESPACE" \
    "$platform_dir/kube-prometheus-stack.values.yaml" \
    --values "$cell/kube-prometheus-stack.values.yaml" --include-crds
  template_chart "$JAEGER_RELEASE" jaegertracing/jaeger \
    "$JAEGER_VERSION" "$OBSERVABILITY_NAMESPACE" "$platform_dir/jaeger.values.yaml" \
    --values "$cell/jaeger.values.yaml"
  template_chart "$OPENTELEMETRY_COLLECTOR_RELEASE" open-telemetry/opentelemetry-collector \
    "$OPENTELEMETRY_COLLECTOR_VERSION" "$OBSERVABILITY_NAMESPACE" \
    "$platform_dir/opentelemetry-collector.values.yaml" \
    --values "$cell/opentelemetry-collector.values.yaml"
  echo "---"
  $KUBECTL kustomize "$platform_dir"
}

cmd_status() {
  need "$HELM"
  log "releases"
  $HELM list --namespace "$OBSERVABILITY_NAMESPACE" 2>/dev/null || true
  $HELM list --namespace "$KEDA_NAMESPACE" 2>/dev/null || true
}

cmd_uninstall() {
  need "$HELM"; need "$KUBECTL"
  warn "uninstalling the platform (PersistentVolumeClaims and CRDs are left behind on purpose)"
  $HELM uninstall "$KEDA_RELEASE" -n "$KEDA_NAMESPACE" 2>/dev/null || true
  $HELM uninstall "$OPENTELEMETRY_COLLECTOR_RELEASE" -n "$OBSERVABILITY_NAMESPACE" 2>/dev/null || true
  $HELM uninstall "$JAEGER_RELEASE" -n "$OBSERVABILITY_NAMESPACE" 2>/dev/null || true
  $HELM uninstall "$KUBE_PROMETHEUS_STACK_RELEASE" -n "$OBSERVABILITY_NAMESPACE" 2>/dev/null || true
  $KUBECTL -n "$OBSERVABILITY_NAMESPACE" delete configmap \
    -l app.kubernetes.io/part-of=encabulator,grafana_dashboard=1 2>/dev/null || true
  log "done. Remove the namespaces yourself if you want the CRDs gone too."
}

case "${1:-install}" in
  install)    cmd_install ;;
  template)   cmd_template ;;
  template-cell) cmd_template_cell ;;
  dashboards) need "$KUBECTL"; dashboards ;;
  status)     cmd_status ;;
  uninstall)  cmd_uninstall ;;
  *) die "usage: $(basename "$0") {install|template|template-cell|dashboards|status|uninstall}" ;;
esac
