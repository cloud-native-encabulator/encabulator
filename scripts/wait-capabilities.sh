#!/usr/bin/env bash
# Wait until the hub's three EKS capabilities (Argo CD, ACK, kro) are ACTIVE.
#
#   make wait-capabilities                       # up to CAPABILITY_TIMEOUT seconds (1800)
#   CAPABILITY_TIMEOUT=600 make wait-capabilities
#
# terraform apply returns once each capability is created, which can be before EKS reports
# it ACTIVE; the GitOps steps need all three. Prints one line per check, stops at once on a
# *_FAILED status, and on a timeout names every capability that is not ACTIVE yet.
# Reads HUB_REGION, HUB_CLUSTER, CAPABILITY_TIMEOUT and CAPABILITY_POLL (seconds, 20).
set -euo pipefail

: "${HUB_REGION:=us-west-2}"
: "${HUB_CLUSTER:=encabulator-usw2}"
: "${CAPABILITY_TIMEOUT:=1800}"
: "${CAPABILITY_POLL:=20}"
capabilities=(argocd ack kro)

die() { echo "wait-capabilities: $*" >&2; exit 1; }
[[ "$CAPABILITY_TIMEOUT" =~ ^[0-9]+$ ]] || die "CAPABILITY_TIMEOUT must be a number of seconds"
[[ "$CAPABILITY_POLL" =~ ^[0-9]+$ ]] || die "CAPABILITY_POLL must be a number of seconds"

status_of() {
  local name="$1" out
  if out="$(aws eks describe-capability --region "$HUB_REGION" --cluster-name "$HUB_CLUSTER" \
      --capability-name "$name" --query capability.status --output text 2>&1)"; then
    echo "$out"
  elif [[ "$out" == *ResourceNotFoundException* ]]; then
    echo "NOT_FOUND"
  else
    die "aws eks describe-capability $name failed: $out"
  fi
}

echo "Waiting up to ${CAPABILITY_TIMEOUT}s for the capabilities of $HUB_CLUSTER ($HUB_REGION) to be ACTIVE"
start=$SECONDS
while :; do
  line="" pending=() failed=()
  for name in "${capabilities[@]}"; do
    status="$(status_of "$name")"
    line+="${line:+, }$name $status"
    case "$status" in
      ACTIVE) ;;
      *_FAILED) failed+=("$name ($status)") ;;
      *) pending+=("$name ($status)") ;;
    esac
  done
  elapsed=$((SECONDS - start))
  printf '  %3ds  %s\n' "$elapsed" "$line"

  if [[ ${#failed[@]} -gt 0 ]]; then
    die "failed: ${failed[*]}. Read why with: aws eks describe-capability --region $HUB_REGION --cluster-name $HUB_CLUSTER --capability-name <name> --query capability.health"
  fi
  if [[ ${#pending[@]} -eq 0 ]]; then
    echo "All three capabilities are ACTIVE."
    exit 0
  fi
  if [[ $elapsed -ge $CAPABILITY_TIMEOUT ]]; then
    die "timed out after ${elapsed}s; not ACTIVE: ${pending[*]}. Run make wait-capabilities again to keep waiting."
  fi
  sleep "$CAPABILITY_POLL"
done
