#!/usr/bin/env bash
# Install, template or uninstall the observability platform and KEDA.
#
#   scripts/platform.sh install     add repos, install the four charts in order, then the dashboards
#   scripts/platform.sh template    render every chart with its pinned values (no cluster needed)
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

# template <release> <chart> <version> <namespace> <values-file>
template_chart() {
  local release="$1" chart="$2" version="$3" namespace="$4" values="$5"
  $HELM template "$release" "$chart" \
    --version "$version" --namespace "$namespace" --values "$values"
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
  dashboards) need "$KUBECTL"; dashboards ;;
  status)     cmd_status ;;
  uninstall)  cmd_uninstall ;;
  *) die "usage: $(basename "$0") {install|template|dashboards|status|uninstall}" ;;
esac
