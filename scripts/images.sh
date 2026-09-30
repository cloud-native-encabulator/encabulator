#!/usr/bin/env bash
# Build (and optionally push) a container image for every service.
#
#   scripts/images.sh                 build every service for the host architecture
#   scripts/images.sh gateway quoter  build only the named services (prefix match)
#   PUSH=true scripts/images.sh       build and push
#   PLATFORMS=linux/amd64,linux/arm64 PUSH=true scripts/images.sh    multi-arch
#
# Design section 7: the build context is the repository root so a Dockerfile can COPY the
# shared kits, and the Dockerfile itself lives in the service directory. Images are tagged
# ${REGISTRY}/<service>:<VERSION> plus a short-sha tag.
#
# Multi-arch with a single `build --platform a,b` needs a builder that can emit a manifest
# list, which in practice means buildx/BuildKit and --push (no local daemon can hold a
# multi-arch image). PUSH is therefore forced on when PLATFORMS names more than one.
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root" || exit 1

: "${CONTAINER_RUNTIME:=finch}"
: "${REGISTRY:=ghcr.io/jlbutler/cloud-native-encabulator}"
: "${VERSION:=$(tr -d '[:space:]' <VERSION)}"
: "${PLATFORMS:=}"
: "${PUSH:=false}"

bold=$'\033[1m'; dim=$'\033[2m'; red=$'\033[31m'; yellow=$'\033[33m'; off=$'\033[0m'

command -v "$CONTAINER_RUNTIME" >/dev/null 2>&1 || {
  printf '%s==> %s is not installed. Set CONTAINER_RUNTIME=docker|finch|nerdctl|podman.%s\n' \
    "$red" "$CONTAINER_RUNTIME" "$off" >&2
  exit 1
}

sha="$(git rev-parse --short HEAD 2>/dev/null || echo nogit)"

if [[ "$PLATFORMS" == *,* && "$PUSH" != "true" ]]; then
  printf '%s==> PLATFORMS names more than one architecture, so PUSH is forced on: a local\n' "$yellow"
  printf '    image store cannot hold a manifest list.%s\n' "$off" >&2
  PUSH=true
fi

selected=("$@")
matches() {
  [ ${#selected[@]} -eq 0 ] && return 0
  local name="$1" pat
  for pat in "${selected[@]}"; do [[ "$name" == *"$pat"* ]] && return 0; done
  return 1
}

built=(); failed=(); missing=(); skipped=()
while read -r name dir; do
  if ! matches "$name"; then skipped+=("$name"); continue; fi
  if [ ! -f "$dir/Dockerfile" ]; then missing+=("$name"); continue; fi

  ref="$REGISTRY/$name:$VERSION"
  printf '%s==> %s%s\n' "$bold" "$ref" "$off"

  args=(build
        -f "$dir/Dockerfile"
        -t "$ref"
        -t "$REGISTRY/$name:$sha"
        --build-arg "VERSION=$VERSION"
        --build-arg "SERVICE=$name"
        --label "org.opencontainers.image.title=$name"
        --label "org.opencontainers.image.version=$VERSION"
        --label "org.opencontainers.image.revision=$sha"
        --label "org.opencontainers.image.source=https://github.com/jlbutler/cloud-native-encabulator")
  # Some corporate resolvers cannot see proxy.golang.org. Set BUILD_GOPROXY to a
  # reachable mirror (e.g. https://goproxy.io) without changing the host's own
  # GOPROXY. Ignored by services that are not written in Go.
  [ -n "${BUILD_GOPROXY:-}" ] && args+=(--build-arg "GOPROXY=$BUILD_GOPROXY")
  [ -n "$PLATFORMS" ] && args+=(--platform "$PLATFORMS")
  [ "$PUSH" = "true" ] && args+=(--push)
  args+=(.)

  # </dev/null keeps the build from swallowing the while-loop's stdin, which
  # would silently stop the iteration after the first image.
  if "$CONTAINER_RUNTIME" "${args[@]}" </dev/null; then built+=("$name"); else failed+=("$name"); fi
done < <("$repo_root/scripts/services.sh" pairs)

printf '\n%s==> built %d image(s)%s\n' "$bold" "${#built[@]}" "$off"
[ ${#missing[@]} -gt 0 ] && printf '%sno Dockerfile yet: %s%s\n' "$dim" "${missing[*]}" "$off"
[ ${#skipped[@]} -gt 0 ] && printf '%snot selected:      %s%s\n' "$dim" "${skipped[*]}" "$off"
if [ ${#failed[@]} -gt 0 ]; then
  printf '%sfailed: %s%s\n' "$red" "${failed[*]}" "$off"
  exit 1
fi
exit 0
