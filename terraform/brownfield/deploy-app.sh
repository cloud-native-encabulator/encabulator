#!/usr/bin/env bash
# Deploy charts/encabulator onto the brownfield hub, pointed at RDS and ElastiCache.
#
# Reads the store endpoints from the ConfigMap encabulator-aws (written by this Terraform
# stack) and passes them to `make deploy` with values-eks-auto-mode.yaml and
# values-brownfield.yaml. Run `make platform` first (the chart renders ServiceMonitors).
#
#   ./deploy-app.sh                 # deploy
#   DRY_RUN=1 ./deploy-app.sh       # print the helm command instead
#   EXTRA_HELM_ARGS="--set logLevel=debug" ./deploy-app.sh
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../.." && pwd)"
ns="encabulator"
cm="encabulator-aws"

read_key() {
  kubectl --namespace "$ns" get configmap "$cm" --output "jsonpath={.data.$1}"
}

pg_host="$(read_key POSTGRES_HOST)"
pg_port="$(read_key POSTGRES_PORT)"
vk_host="$(read_key VALKEY_HOST)"
vk_port="$(read_key VALKEY_PORT)"

for name in pg_host pg_port vk_host vk_port; do
  if [[ -z "${!name}" ]]; then
    echo "error: ConfigMap $ns/$cm is missing a value ($name). Did terraform apply finish?" >&2
    exit 1
  fi
done

helm_args=(
  "-f charts/encabulator/values-eks-auto-mode.yaml"
  "-f charts/encabulator/values-brownfield.yaml"
  "--set postgres.external.host=$pg_host"
  "--set postgres.external.port=$pg_port"
  "--set valkey.external.host=$vk_host"
  "--set valkey.external.port=$vk_port"
)

make_flags=()
if [[ "${DRY_RUN:-0}" == "1" ]]; then
  make_flags+=(--dry-run)
fi

make --directory "$root" "${make_flags[@]}" deploy \
  NAMESPACE="$ns" RELEASE="encabulator" \
  HELM_ARGS="${helm_args[*]} ${EXTRA_HELM_ARGS:-}"
