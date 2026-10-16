#!/usr/bin/env bash
# Create or destroy a local kind cluster sized for the whole Encabulator.
#
#   scripts/kind.sh up      create the cluster (idempotent) and load nothing else
#   scripts/kind.sh down    delete it
#   scripts/kind.sh load    load locally built images into the cluster's containerd
#
# kind is not assumed to be installed. If it is missing this prints how to get it and exits
# non-zero without doing anything, so `make kind-up` degrades instead of exploding.
#
# Note on container runtimes: kind speaks to Docker, Podman or nerdctl. Finch is
# Docker-API-compatible, so exporting DOCKER_HOST (see dev-env.sh at the repository root)
# is usually enough; KIND_EXPERIMENTAL_PROVIDER=podman|nerdctl is the other route.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

: "${KIND:=kind}"
: "${KIND_CLUSTER:=encabulator}"
: "${KIND_NODE_IMAGE:=kindest/node:v1.34.0}"
: "${CONTAINER_RUNTIME:=finch}"
: "${REGISTRY:=ghcr.io/cloud-native-encabulator/encabulator}"

log()  { printf '\033[1m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m==> %s\033[0m\n' "$*" >&2; }

require_kind() {
  if ! command -v "$KIND" >/dev/null 2>&1; then
    cat >&2 <<'EOF'
==> kind is not installed, so there is no local cluster to talk to.

    Install it with one of:
      brew install kind
      go install sigs.k8s.io/kind@latest

    Then re-run `make kind-up`. The Encabulator needs roughly 6 CPUs and 10 GiB of memory
    for all fifteen services plus the observability platform, so give your container
    runtime's VM at least that (`finch vm stop && finch vm init --cpus 8 --memory 12GiB`
    if you are starting from the default).

    Everything else in this repository works without kind: `make lint-charts`, and
    `make platform` / `make deploy` against any conformant cluster with a default
    StorageClass, and `make dev-up` plus each service's own `make run` for running
    services natively against containerised stores.

    Nothing was created.
EOF
    return 1
  fi
}

cmd_up() {
  require_kind || return 1
  if "$KIND" get clusters 2>/dev/null | grep -qx "$KIND_CLUSTER"; then
    log "kind cluster '$KIND_CLUSTER' already exists"
  else
    log "creating kind cluster '$KIND_CLUSTER' on $KIND_NODE_IMAGE"
    "$KIND" create cluster --name "$KIND_CLUSTER" --wait 120s --config=- <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${KIND_CLUSTER}
nodes:
  - role: control-plane
    image: ${KIND_NODE_IMAGE}
    kubeadmConfigPatches:
      # Let an ingress controller bind 80/443 on the node if one gets installed later.
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
    extraPortMappings:
      - containerPort: 30080
        hostPort: 8080
        protocol: TCP
  - role: worker
    image: ${KIND_NODE_IMAGE}
  - role: worker
    image: ${KIND_NODE_IMAGE}
EOF
  fi

  log "kind provides a default StorageClass (standard), which the chart's PVCs need"
  kubectl --context "kind-${KIND_CLUSTER}" get storageclass || true

  cat <<EOF

Next:
  kubectl config use-context kind-${KIND_CLUSTER}
  make platform
  make images && make kind-load     # or set REGISTRY to something the cluster can pull from
  make deploy
  make console
EOF
}

cmd_down() {
  require_kind || return 1
  log "deleting kind cluster '$KIND_CLUSTER'"
  "$KIND" delete cluster --name "$KIND_CLUSTER"
}

cmd_load() {
  require_kind || return 1
  local version
  version="$(tr -d '[:space:]' <"$repo_root/VERSION")"
  # kind load docker-image needs the image in the local daemon; with Finch that is the
  # Finch VM's containerd, which kind cannot read directly, so go through a tar archive.
  while read -r name; do
    local ref="$REGISTRY/$name:$version"
    log "loading $ref"
    local tar
    tar="$(mktemp -t "encabulator-$name.XXXXXX").tar"
    "$CONTAINER_RUNTIME" save -o "$tar" "$ref"
    "$KIND" load image-archive --name "$KIND_CLUSTER" "$tar"
    rm -f "$tar"
  done < <("$repo_root/scripts/services.sh" names)
}

case "${1:-up}" in
  # A missing kind is a documented no-op rather than a build failure, so `make kind-up`
  # on a machine without it prints guidance instead of a make error.
  up)   cmd_up   || exit 0 ;;
  down) cmd_down || exit 0 ;;
  load) cmd_load || exit 0 ;;
  *) warn "usage: $(basename "$0") {up|down|load}"; exit 2 ;;
esac
