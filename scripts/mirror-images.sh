#!/usr/bin/env bash
# Make sure the hub can pull every service image at VERSION.
#
#   scripts/mirror-images.sh            mirror when needed (make mirror-images)
#   scripts/mirror-images.sh --check    only report whether GHCR has every image (no AWS calls)
#
# With REGISTRY unset (the public images on GitHub Container Registry) and every service on
# GHCR at VERSION, there is nothing to do. Otherwise the images go to Amazon ECR in this
# account, in the hub region:
#   1. create the repositories encabulator/<service>, tagged at create time (scripts/ecr-repos.sh)
#   2. build linux/amd64 (the hub's nodes are amd64) and push <registry>/<service>:<VERSION>,
#      skipping every service whose VERSION tag is already in ECR: a version tag is never
#      overwritten, whatever FORCE says
#   3. record REGISTRY=<account>.dkr.ecr.<region>.amazonaws.com/encabulator in .env
#      (in place, keeping .env.bak) so every later step pulls from there
# Re-running is safe: what is already there is left alone.
#
# Reads VERSION, REGISTRY, CONTAINER_RUNTIME, HUB_REGION and DOTENV_FILE (make sets them).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

default_registry="ghcr.io/cloud-native-encabulator/encabulator"
ghcr_repository="${default_registry#ghcr.io/}"
: "${VERSION:=$(tr -d '[:space:]' <VERSION)}"
: "${REGISTRY:=$default_registry}"
: "${CONTAINER_RUNTIME:=finch}"
: "${HUB_REGION:=us-west-2}"
: "${DOTENV_FILE:=.env}"

bold=$'\033[1m'; dim=$'\033[2m'; off=$'\033[0m'
say() { printf '%s==> %s%s\n' "$bold" "$*" "$off"; }
die() { echo "mirror-images: $*" >&2; exit 1; }

names=()
while IFS= read -r name; do names+=("$name"); done < <("$repo_root/scripts/services.sh" names)

# ghcr_has SERVICE: the public GHCR image SERVICE:VERSION exists (anonymous pull token).
ghcr_has() {
  local svc="$1" token code
  token="$(curl -fsS --max-time 20 \
    "https://ghcr.io/token?scope=repository:${ghcr_repository}/${svc}:pull" 2>/dev/null \
    | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')" || return 1
  [[ -n "$token" ]] || return 1
  code="$(curl -sS --max-time 20 -o /dev/null -w '%{http_code}' --head \
    -H "Authorization: Bearer ${token}" \
    -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
    "https://ghcr.io/v2/${ghcr_repository}/${svc}/manifests/${VERSION}" 2>/dev/null)" || return 1
  [[ "$code" == "200" ]]
}

# ghcr_missing: print the services GHCR lacks at VERSION, one per line.
ghcr_missing() {
  local svc
  for svc in "${names[@]}"; do ghcr_has "$svc" || echo "$svc"; done
}

# ---------------------------------------------------------------------- --check
if [[ "${1:-}" == "--check" ]]; then
  command -v curl >/dev/null 2>&1 || { echo "curl is not installed"; exit 2; }
  missing="$(ghcr_missing)"
  if [[ -z "$missing" ]]; then
    echo "all ${#names[@]} images are on GHCR at $VERSION"
    exit 0
  fi
  echo "not on GHCR at $VERSION (or GHCR unreachable): $(echo "$missing" | paste -sd' ' -)"
  exit 1
fi
[[ $# -eq 0 ]] || die "usage: mirror-images.sh [--check]"

if [[ -n "${FORCE:-}" ]]; then
  echo "FORCE is ignored: mirror-images never overwrites a version tag that is already in ECR."
fi

# ---------------------------------------------------------------- GHCR is enough
if [[ "$REGISTRY" == "$default_registry" ]]; then
  command -v curl >/dev/null 2>&1 || die "curl is not installed (it checks GHCR for the images)"
  say "checking GHCR for the ${#names[@]} images at $VERSION"
  missing="$(ghcr_missing)"
  if [[ -z "$missing" ]]; then
    say "nothing to mirror: every image is on GHCR at $VERSION"
    exit 0
  fi
  printf '%snot on GHCR at %s: %s%s\n' "$dim" "$VERSION" "$(echo "$missing" | paste -sd' ' -)" "$off"
fi

# ------------------------------------------------------------------- ECR target
command -v aws >/dev/null 2>&1 || die "the AWS CLI is not installed"
account="$(aws sts get-caller-identity --query Account --output text)" \
  || die "no working AWS credentials (aws sts get-caller-identity failed)"
[[ "$account" =~ ^[0-9]{12}$ ]] || die "unexpected account id from aws sts get-caller-identity"
host="${account}.dkr.ecr.${HUB_REGION}.amazonaws.com"
target="${host}/encabulator"

if [[ "$REGISTRY" != "$default_registry" && "$REGISTRY" != "$target" ]]; then
  say "REGISTRY is set to $REGISTRY: using it as it is"
  echo "mirror-images only fills this account's ECR registry in $HUB_REGION. To mirror there,"
  echo "empty REGISTRY in $DOTENV_FILE; to use your own registry, push to it with make push."
  exit 0
fi

# Which services still need their VERSION tag in ECR. A repository that does not exist yet
# counts as missing; any other error stops the run.
ecr_has() {
  local svc="$1" out
  if out="$(aws ecr describe-images --region "$HUB_REGION" --repository-name "encabulator/$svc" \
      --image-ids "imageTag=$VERSION" --query 'imageDetails[0].imageTags' --output text 2>&1)"; then
    return 0
  fi
  case "$out" in
    *ImageNotFoundException*|*RepositoryNotFoundException*) return 1 ;;
    *) die "aws ecr describe-images for encabulator/$svc failed: $out" ;;
  esac
}

say "checking ECR ($HUB_REGION) for the ${#names[@]} images at $VERSION"
to_build=()
for svc in "${names[@]}"; do
  if ecr_has "$svc"; then
    printf '  %-30s %s\n' "$svc" "already in ECR at $VERSION, left as it is"
  else
    to_build+=("$svc")
  fi
done

if [[ ${#to_build[@]} -gt 0 ]]; then
  command -v "$CONTAINER_RUNTIME" >/dev/null 2>&1 \
    || die "$CONTAINER_RUNTIME is not installed: set CONTAINER_RUNTIME=docker or finch in $DOTENV_FILE"

  # scripts/images.sh selects services by substring. Only build when every name selects
  # exactly itself, so no other service's version tag can be pushed over.
  for svc in "${to_build[@]}"; do
    count=0
    for other in "${names[@]}"; do [[ "$other" == *"$svc"* ]] && count=$((count + 1)); done
    [[ $count -eq 1 ]] || die "service name $svc also selects another service; refusing to build"
  done

  say "creating the ECR repositories (tagged auto-delete=no, project=encabulator, talk=opn315)"
  "$repo_root/scripts/ecr-repos.sh" "$HUB_REGION"

  say "building and pushing ${#to_build[@]} image(s) for linux/amd64 to $target"
  REGISTRY="$target" VERSION="$VERSION" PLATFORMS=linux/amd64 PUSH=true \
    "$repo_root/scripts/images.sh" "${to_build[@]}"

  for svc in "${to_build[@]}"; do
    ecr_has "$svc" || die "encabulator/$svc:$VERSION is not in ECR after the push"
  done
fi

# --------------------------------------------------------------- record in .env
if [[ -f "$DOTENV_FILE" ]]; then
  "$repo_root/scripts/dotenv.sh" set "$DOTENV_FILE" REGISTRY "$target"
  say "REGISTRY=$target (recorded in $DOTENV_FILE)"
else
  say "REGISTRY=$target"
  echo "Add that line to $DOTENV_FILE (cp .env.example .env) so the next steps use it."
fi
