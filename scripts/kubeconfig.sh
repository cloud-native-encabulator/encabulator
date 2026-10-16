#!/usr/bin/env bash
# Point kubectl at the hub that terraform/brownfield built, and check that it does.
#
#   make kubeconfig
#
# Runs the command Terraform prints as output kubeconfig_command (aws eks update-kubeconfig)
# with a stable context name, encabulator-usw2, sets that context's namespace to encabulator
# (the GitOps steps assume it), then checks that the current context's API server is the
# hub's endpoint. Reads TERRAFORM, TF_DIR, KUBECTL, NAMESPACE, HUB_REGION and HUB_CLUSTER.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

: "${TERRAFORM:=terraform}"
: "${TF_DIR:=terraform/brownfield}"
: "${KUBECTL:=kubectl}"
: "${NAMESPACE:=encabulator}"
: "${HUB_REGION:=us-west-2}"
: "${HUB_CLUSTER:=encabulator-usw2}"

die() { echo "kubeconfig: $*" >&2; exit 1; }
# shellcheck source=terraform-error.sh
source "$repo_root/scripts/terraform-error.sh"

read -r -a terraform <<<"$TERRAFORM"
read -r -a kubectl <<<"$KUBECTL"

# Terraform's error output is kept: "no such output" means the hub is not built yet, anything
# else (a version manager's shim with no version set, a backend that is not initialized) is
# passed on as Terraform says it.
errors="$(mktemp)"
trap 'rm -f "$errors"' EXIT
tf_rc=0
command="$("${terraform[@]}" -chdir="$TF_DIR" output -raw kubeconfig_command 2>"$errors")" || tf_rc=$?
if [[ $tf_rc -eq 0 && -z "$command" ]] \
  || { [[ $tf_rc -ne 0 ]] && grep -qiE 'no outputs found|output( "[^"]*")? not found' "$errors"; }; then
  die "Terraform has no kubeconfig_command output yet: run make brownfield-apply first"
elif [[ $tf_rc -ne 0 ]]; then
  echo "kubeconfig: Terraform failed: $(terraform_error "$errors")" >&2
  die "$(terraform_hint), and run make kubeconfig again"
fi

# Run only the command shape this stack prints, never arbitrary output.
pattern='^aws eks update-kubeconfig --region ([a-z0-9-]+) --name ([A-Za-z0-9_-]+)$'
[[ "$command" =~ $pattern ]] || die "unexpected kubeconfig_command output: $command"
region="${BASH_REMATCH[1]}"
name="${BASH_REMATCH[2]}"
[[ "$region" == "$HUB_REGION" && "$name" == "$HUB_CLUSTER" ]] \
  || die "Terraform built $name in $region, expected $HUB_CLUSTER in $HUB_REGION"

aws eks update-kubeconfig --region "$region" --name "$name" --alias "$HUB_CLUSTER"
"${kubectl[@]}" config use-context "$HUB_CLUSTER" >/dev/null
"${kubectl[@]}" config set-context --current --namespace "$NAMESPACE" >/dev/null

endpoint="$(aws eks describe-cluster --region "$region" --name "$name" \
  --query cluster.endpoint --output text)"
context="$("${kubectl[@]}" config current-context)"
server="$("${kubectl[@]}" config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
[[ "$context" == "$HUB_CLUSTER" ]] || die "current context is $context, not $HUB_CLUSTER"
[[ -n "$endpoint" && "$server" == "$endpoint" ]] \
  || die "context $HUB_CLUSTER points at $server, not the $HUB_CLUSTER endpoint $endpoint"

echo "kubectl points at $HUB_CLUSTER ($region), context $HUB_CLUSTER, namespace $NAMESPACE"
