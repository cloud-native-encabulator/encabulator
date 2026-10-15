#!/usr/bin/env bash
# The variables in lower case (repo_url, revision, registry, the defaults) come from
# scripts/gitops-settings.sh, which shellcheck does not follow without -x.
# shellcheck disable=SC2154
#
# The hub's Argo CD steps, with the repository, revision and registry taken from the
# environment (the Makefile exports them from .env) instead of from edited files.
#
#   scripts/gitops.sh render [bootstrap|cells|all]  print the bootstrap manifests with the
#                                                   settings applied (default: all)
#   scripts/gitops.sh check                         fail unless the default render equals the
#                                                   committed files and a custom render uses
#                                                   the custom values everywhere
#   scripts/gitops.sh bootstrap                     apply everything in gitops/hub/bootstrap/
#                                                   except hub-cells (no cell is built)
#   scripts/gitops.sh add-cell [<cell>]             apply hub-cells, which builds every cell in
#                                                   gitops/hub/cells/ at the tracked revision
#   scripts/gitops.sh repo-access                   give Argo CD a token for a private
#                                                   repository (GIT_TOKEN_FILE)
#   scripts/gitops.sh cell-access [<cell>]          give the calling AWS principal cluster
#                                                   admin on a cell's EKS cluster
#
# Settings (all optional, see scripts/gitops-settings.sh): GIT_REPO_URL, GIT_REVISION,
# REGISTRY, GIT_TOKEN_FILE, and CELL for add-cell and cell-access. With none of them set,
# the render is exactly the committed files.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bootstrap_dir="$repo_root/gitops/hub/bootstrap"
cells_dir="$repo_root/gitops/hub/cells"
# shellcheck source=gitops-settings.sh
source "$repo_root/scripts/gitops-settings.sh"

: "${KUBECTL:=kubectl}"
: "${AWS:=aws}"
: "${GIT:=git}"
hub_name="encabulator-usw2"

# Applied by `bootstrap`, in this order: the projects first, then what uses them.
bootstrap_files=(projects.yaml hub-rgds.yaml hub-cell-apps.yaml hub-brownfield.yaml)
cells_file=hub-cells.yaml

log()  { printf '\033[1m==>\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[33m==> %s\033[0m\n' "$*" >&2; }
die()  { printf '\033[31m==> %s\033[0m\n' "$*" >&2; exit 1; }

# A custom revision is quoted, so a tag such as 1.10 stays a string in YAML.
revision_yaml() {
  if [[ "$revision" == "$default_revision" ]]; then printf '%s' "$revision"
  else printf '"%s"' "$revision"; fi
}

# One bootstrap file with the settings applied: every occurrence of the default repository
# URL, and every `targetRevision: <default>` line. Nothing else changes.
render_file() {
  local line rev
  rev="$(revision_yaml)"
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line//"$default_repo_url"/"$repo_url"}"
    if [[ "$line" =~ ^([[:space:]]*(-[[:space:]]+)?targetRevision:)[[:space:]]+"$default_revision"$ ]]; then
      line="${BASH_REMATCH[1]} $rev"
    fi
    printf '%s\n' "$line"
  done <"$1"
}

render() {
  local what="${1:-all}" files f first=1
  case "$what" in
    bootstrap) files=("${bootstrap_files[@]}") ;;
    cells)     files=("$cells_file") ;;
    all)       files=("${bootstrap_files[@]}" "$cells_file") ;;
    *) die "render: expected bootstrap, cells or all, got '$what'" ;;
  esac
  for f in "${files[@]}"; do
    [[ $first -eq 1 ]] || printf -- '---\n'
    first=0
    render_file "$bootstrap_dir/$f"
  done
}

# ----------------------------------------------------------------------------- check
# Where the settings land in the bootstrap files. A new source that names the repository
# changes these counts, and the check says so: add it here once it is rendered as well.
#                       file              repository URL  targetRevision
expected_counts=(
  "projects.yaml        2 0"
  "hub-rgds.yaml        2 2"
  "hub-cell-apps.yaml   1 1"
  "hub-brownfield.yaml  1 1"
  "hub-cells.yaml       1 1"
)

check() {
  local failures=0 f want_url want_rev got_url got_rev out entry total_url=0 total_rev=0
  local custom_url="https://git.example.com/example-org/encabulator-fork.git"
  local custom_rev="example-release"

  log "default render equals the committed files"
  for f in "${bootstrap_files[@]}" "$cells_file"; do
    if out="$(diff -u "$bootstrap_dir/$f" <(GIT_REPO_URL='' GIT_REVISION='' render_one "$f"))"; then
      log "  same: gitops/hub/bootstrap/$f"
    else
      warn "  differs: gitops/hub/bootstrap/$f"
      printf '%s\n' "$out" >&2
      failures=$((failures + 1))
    fi
  done
  # Every bootstrap file is rendered by one of the two lists above.
  for f in "$bootstrap_dir"/*.yaml; do
    case " ${bootstrap_files[*]} $cells_file " in
      *" $(basename "$f") "*) ;;
      *) warn "  gitops/hub/bootstrap/$(basename "$f") is not rendered by scripts/gitops.sh"
         failures=$((failures + 1)) ;;
    esac
  done

  log "custom render ($custom_url at $custom_rev) uses the custom values everywhere"
  for entry in "${expected_counts[@]}"; do
    read -r f want_url want_rev <<<"$entry"
    out="$(GIT_REPO_URL="$custom_url" GIT_REVISION="$custom_rev" render_one "$f")"
    got_url="$(grep -cF -- "$custom_url" <<<"$out" || true)"
    got_rev="$(grep -cE -- "targetRevision: \"$custom_rev\"\$" <<<"$out" || true)"
    total_url=$((total_url + got_url))
    total_rev=$((total_rev + got_rev))
    if [[ "$got_url" -ne "$want_url" || "$got_rev" -ne "$want_rev" ]]; then
      warn "  $f: repository URL $got_url (want $want_url), targetRevision $got_rev (want $want_rev)"
      failures=$((failures + 1))
    else
      log "  $f: repository URL $got_url, targetRevision $got_rev"
    fi
    if grep -qF -- "$default_repo_url" <<<"$out"; then
      warn "  $f: still names $default_repo_url"
      failures=$((failures + 1))
    fi
    if grep -qwF -- "$default_revision" <<<"$out"; then
      warn "  $f: still names the revision $default_revision"
      failures=$((failures + 1))
    fi
  done
  log "  total: repository URL $total_url, targetRevision $total_rev"

  # gitops/hub/appset.yaml is synced from Git, so it cannot be rendered: it reads the values
  # from the cell's cluster Secret and falls back to the same defaults as this script.
  log "gitops/hub/appset.yaml falls back to the same defaults"
  local appset="$repo_root/gitops/hub/appset.yaml"
  got_url="$(grep -cF -- "default \"$default_repo_url\"" "$appset" || true)"
  got_rev="$(grep -cF -- "default \"$default_revision\"" "$appset" || true)"
  if [[ "$got_url" -ne 2 || "$got_rev" -ne 2 ]]; then
    warn "  defaults in appset.yaml: repository URL $got_url (want 2), revision $got_rev (want 2)"
    failures=$((failures + 1))
  else
    log "  repository URL 2, revision 2"
  fi
  if grep -nE '^[[:space:]]*(- )?(repoURL|targetRevision): *"?('"$default_revision"'|https://github\.com/jlbutler/cloud-native-encabulator)' "$appset" >&2; then
    warn "  appset.yaml sets this repository or revision directly instead of through the defaults"
    failures=$((failures + 1))
  fi

  [[ $failures -eq 0 ]] || die "$failures check(s) failed"
  log "render check passed"
}

render_one() {
  # Re-read the settings so a per-call environment (as in check) takes effect.
  # shellcheck source=gitops-settings.sh
  source "$repo_root/scripts/gitops-settings.sh"
  render_file "$bootstrap_dir/$1"
}

# --------------------------------------------------------------------------- cluster steps
# The hub's own cluster Secret proves kubectl points at a registered hub. The cells and
# hub-brownfield read it too, so nothing below works without it.
require_hub() {
  "$KUBECTL" -n argocd get secret "$hub_name" -o name >/dev/null 2>&1 \
    || die "no Argo CD cluster Secret argocd/$hub_name here: point kubectl at the hub and run \`make gitops-register\` first"
  [[ "$("$KUBECTL" -n argocd get secret "$hub_name" -o jsonpath='{.metadata.labels.encabulator\.io/hub}')" == "true" ]] \
    || die "argocd/$hub_name has no encabulator.io/hub=true label: run \`make gitops-register\`"
}

settings_line() {
  printf 'repository %s at %s' "$repo_url" "$revision"
  [[ -z "$registry" ]] || printf ', images from %s' "$registry"
}

bootstrap() {
  require_hub
  log "applying the Argo CD projects and hub Applications: $(settings_line)"
  render bootstrap | "$KUBECTL" apply -f -
  log "cells are off: hub-cells is not applied, so no cell is built."
  log "to build one, commit gitops/hub/cells/<cell>.yaml to $revision and run \`make add-cell CELL=<cell>\`"
}

# The cell file must be in the revision Argo CD syncs, not just in this checkout.
cell_in_revision() {
  local path="gitops/hub/cells/$1.yaml" err
  if err="$("$GIT" -C "$repo_root" fetch --quiet --no-tags "$repo_url" "$revision" 2>&1)"; then
    "$GIT" -C "$repo_root" cat-file -e "FETCH_HEAD:$path" 2>/dev/null \
      || die "$path is not in $repo_url at $revision: commit and push it there first"
    log "$path is in $repo_url at $revision"
  else
    warn "could not read $revision from $repo_url ($err)"
    [[ -f "$repo_root/$path" ]] || die "$path does not exist"
    warn "found $path in this checkout; make sure it is pushed to $revision, or nothing is built"
  fi
}

add_cell() {
  local cell="${1:-}" name region known=() f
  for f in "$cells_dir"/*.yaml; do [[ -e "$f" ]] && known+=("$(basename "$f" .yaml)"); done
  [[ "$cell" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] \
    || die "usage: make add-cell CELL=<cell>, where gitops/hub/cells/<cell>.yaml exists (here: ${known[*]:-none})"
  require_hub
  cell_in_revision "$cell"
  name="$(awk '/^metadata:/ { m = 1; next } /^[^ #]/ { m = 0 } m && /^  name:/ { print $2; exit }' "$cells_dir/$cell.yaml" 2>/dev/null || true)"
  region="$(awk '/^spec:/ { s = 1; next } /^[^ #]/ { s = 0 } s && /^  region:/ { print $2; exit }' "$cells_dir/$cell.yaml" 2>/dev/null || true)"
  log "applying hub-cells: $(settings_line)"
  render cells | "$KUBECTL" apply -f -
  log "Argo CD now builds every cell in gitops/hub/cells/ at $revision${name:+, including $name${region:+ in $region}}."
  log "watch:  kubectl get encabulatorcells -n encabulator-cells -w"
  log "expect the cell ACTIVE in about 20 minutes and the app Healthy 9 to 12 minutes after that."
}

repo_access() {
  local file="${GIT_TOKEN_FILE:-}" mode name token
  if [[ -z "$file" ]]; then
    log "GIT_TOKEN_FILE is not set: nothing to do (Argo CD reads $repo_url without credentials)"
    return 0
  fi
  [[ -f "$file" ]] || die "GIT_TOKEN_FILE $file does not exist"
  mode="$(stat -c '%a' "$file" 2>/dev/null || stat -f '%Lp' "$file")"
  [[ "$mode" == "600" ]] || die "GIT_TOKEN_FILE $file has mode $mode; run chmod 600 on it first"
  [[ "$repo_url" == https://* ]] || die "a token file works with an https:// GIT_REPO_URL, got $repo_url"
  token="$(tr -d '\r\n' <"$file")"
  [[ -n "$token" ]] || die "GIT_TOKEN_FILE $file is empty"
  require_hub
  # repo-<last path element>, as a Kubernetes name.
  name="$(basename "${repo_url%.git}" | tr 'A-Z_.' 'a-z--' | tr -cd 'a-z0-9-' | cut -c1-58)"
  name="repo-${name:-git}"
  # The token goes to kubectl on stdin, base64-encoded, never in argv or output. Server-side
  # apply keeps it out of the last-applied-configuration annotation.
  {
    printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: argocd\n' "$name"
    printf '  labels:\n    argocd.argoproj.io/secret-type: repository\ntype: Opaque\n'
    printf 'stringData:\n  type: git\n  url: %s\n  username: x-access-token\ndata:\n' "$repo_url"
    printf '  password: %s\n' "$(printf '%s' "$token" | base64 | tr -d '\n')"
  } | "$KUBECTL" apply --server-side --field-manager=encabulator-repo-access -f - >/dev/null
  log "Argo CD repository Secret argocd/$name: $repo_url with the token from $file"
}

cell_access() {
  local cell="${1:-}" file name region caller account partition role principal entry
  local policy tags="auto-delete=no,project=encabulator,talk=opn315"
  [[ "$cell" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || die "usage: make cell-access CELL=<cell>"
  file="$cells_dir/$cell.yaml"
  [[ -f "$file" ]] || die "no gitops/hub/cells/$cell.yaml"
  name="$(awk '/^metadata:/ { m = 1; next } /^[^ #]/ { m = 0 } m && /^  name:/ { print $2; exit }' "$file")"
  region="$(awk '/^spec:/ { s = 1; next } /^[^ #]/ { s = 0 } s && /^  region:/ { print $2; exit }' "$file")"
  [[ -n "$name" && -n "$region" ]] || die "could not read metadata.name and spec.region from $file"

  [[ "$("$AWS" eks describe-cluster --region "$region" --name "$name" --query cluster.status --output text)" == "ACTIVE" ]] \
    || die "cluster $name in $region is not ACTIVE yet"
  caller="$("$AWS" sts get-caller-identity --query Arn --output text)"
  if [[ -n "${PRINCIPAL_ARN:-}" ]]; then
    principal="$PRINCIPAL_ARN"
  elif [[ "$caller" =~ ^arn:(aws[a-z-]*):sts::([0-9]{12}):assumed-role/([^/]+)/ ]]; then
    partition="${BASH_REMATCH[1]}" account="${BASH_REMATCH[2]}" role="${BASH_REMATCH[3]}"
    # The role's own ARN, with its path (an Identity Center role has one).
    principal="$("$AWS" iam get-role --role-name "$role" --query Role.Arn --output text 2>/dev/null)" \
      || { principal="arn:$partition:iam::$account:role/$role"
           warn "could not read role $role; using $principal (set PRINCIPAL_ARN if the role has a path)"; }
  elif [[ "$caller" =~ ^arn:aws[a-z-]*:iam::[0-9]{12}:user/ ]]; then
    principal="$caller"
  else
    die "unexpected caller $caller; set PRINCIPAL_ARN to the IAM role or user ARN"
  fi
  partition="$(cut -d: -f2 <<<"$principal")"
  policy="arn:$partition:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"

  if entry="$("$AWS" eks describe-access-entry --region "$region" --cluster-name "$name" \
      --principal-arn "$principal" --query accessEntry.accessEntryArn --output text 2>/dev/null)"; then
    log "access entry for $principal exists on $name; making sure it carries the tags"
    "$AWS" eks tag-resource --region "$region" --resource-arn "$entry" --tags "$tags"
  else
    log "creating an access entry for $principal on $name ($region)"
    "$AWS" eks create-access-entry --region "$region" --cluster-name "$name" \
      --principal-arn "$principal" --type STANDARD --tags "$tags" >/dev/null
  fi
  "$AWS" eks associate-access-policy --region "$region" --cluster-name "$name" \
    --principal-arn "$principal" --policy-arn "$policy" --access-scope type=cluster >/dev/null
  log "$principal has AmazonEKSClusterAdminPolicy on $name. Use it with:"
  log "  aws eks update-kubeconfig --region $region --name $name --alias $name"
}

case "${1:-}" in
  render)      render "${2:-all}" ;;
  check)       check ;;
  bootstrap)   bootstrap ;;
  add-cell)    add_cell "${2:-${CELL:-}}" ;;
  repo-access) repo_access ;;
  cell-access) cell_access "${2:-${CELL:-}}" ;;
  *) sed -n '6,25p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
