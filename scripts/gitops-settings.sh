# shellcheck shell=bash
# The deploy settings the GitOps scripts share, read from the environment (the Makefile exports
# them from .env). Sourced by scripts/gitops.sh and gitops/hub/bootstrap/register-hub.sh.
#
#   GIT_REPO_URL   the repository Argo CD syncs    default: this repository on GitHub
#   GIT_REVISION   the branch, tag or commit       default: carrlos
#   REGISTRY       an image registry for the cells default: none (the images' own registry)
#
# After sourcing: repo_url and revision always hold a value (the default when unset);
# repo_url_set, revision_set and registry hold only what was set to something other than the
# default, which is what register-hub.sh records on the hub. Invalid values stop the script.

default_repo_url="https://github.com/jlbutler/cloud-native-encabulator"
default_revision="carrlos"
# The Makefile's own REGISTRY default. The same value set by hand means "no mirror".
default_registry="ghcr.io/jlbutler/cloud-native-encabulator"

settings_die() { printf 'gitops: %s\n' "$*" >&2; exit 2; }

repo_url="${GIT_REPO_URL:-$default_repo_url}"
revision="${GIT_REVISION:-$default_revision}"
registry="${REGISTRY:-}"

# Strict shapes: these values land in YAML and in Argo CD, so nothing that needs quoting.
[[ "$repo_url" =~ ^(https://|ssh://|git@)[A-Za-z0-9._~:/@%+=-]+$ ]] \
  || settings_die "GIT_REPO_URL must be an https://, ssh:// or git@ URL without spaces or quotes, got '$repo_url'"
[[ "$revision" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$ && "$revision" != *..* ]] \
  || settings_die "GIT_REVISION must be a branch, tag or commit name, got '$revision'"
if [[ -n "$registry" ]]; then
  [[ "$registry" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:[0-9]+)?(/[a-z0-9]([a-z0-9._-]*[a-z0-9])?)*$ ]] \
    || settings_die "REGISTRY must look like <host>[/<path>], for example 111122223333.dkr.ecr.us-west-2.amazonaws.com/encabulator, got '$registry'"
fi

repo_url_set=""
revision_set=""
[[ "$repo_url" == "$default_repo_url" ]] || repo_url_set="$repo_url"
[[ "$revision" == "$default_revision" ]] || revision_set="$revision"
[[ "$registry" == "$default_registry" ]] && registry=""
# Read by the scripts that source this file.
export repo_url revision repo_url_set revision_set registry
