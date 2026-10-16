#!/usr/bin/env bash
# Stand-in for aws, terraform, kubectl, helm, docker and curl in scripts/make-flow_test.sh.
# Installed under each of those names; picks its behavior from the name it was called by.
# Every call is appended to $STUB_LOG as "<name> <args>" (plus the environment that matters
# for that tool), and state that a real tool would keep remotely (ECR repositories and their
# images) lives in files under $STUB_STATE. A call it does not know fails with exit 99, so a
# test notices anything unexpected. Nothing here reaches a network or a cloud.
set -uo pipefail

tool="$(basename "$0")"
: "${STUB_LOG:?}" "${STUB_STATE:?}"
account=111122223333
endpoint="https://EXAMPLE.gr7.us-west-2.eks.amazonaws.com"

log() { printf '%s\n' "$*" >>"$STUB_LOG"; }
unknown() { echo "stub $tool: unexpected call: $*" >&2; log "UNEXPECTED $tool $*"; exit 99; }
not_found() { echo "An error occurred ($1) when calling the $2 operation: stub" >&2; exit 254; }

# arg NAME ARGS...: the value after --NAME in ARGS.
arg() {
  local want="--$1" prev=""; shift
  for a in "$@"; do
    [[ "$prev" == "$want" ]] && { printf '%s' "$a"; return 0; }
    prev="$a"
  done
  return 1
}

case "$tool" in
  # ------------------------------------------------------------------------- aws
  aws)
    log "aws $* | AWS_REGION=${AWS_REGION:-}"
    [[ "${1:-}" == "--version" ]] && { echo "${STUB_AWS_VERSION:-aws-cli/2.31.0 Python/3.13.7 Linux/6.1 exe/x86_64}"; exit 0; }
    svc="${1:-}" op="${2:-}"
    case "$svc $op" in
      "sts get-caller-identity")
        [[ "${STUB_AWS_CREDS:-yes}" == yes ]] || { echo "Unable to locate credentials." >&2; exit 253; }
        case "$(arg query "$@")" in
          Arn) echo "arn:aws:sts::$account:assumed-role/Admin/tester" ;;
          Account) echo "$account" ;;
          *) unknown "$@" ;;
        esac
        ;;
      "sso-admin list-instances")
        [[ "$(arg region "$@")" == "${STUB_IDC_REGION:-us-west-2}" ]] || exit 0
        printf '%s\t%s\n' "${STUB_IDC_ARN:-arn:aws:sso:::instance/ssoins-1111111111111111}" d-1111111111
        ;;
      "identitystore describe-group")
        [[ "$(arg group-id "$@")" == "${STUB_GROUP_ID:-a1b2c3d4e5-11111111-2222-3333-4444-555555555555}" ]] \
          || not_found ResourceNotFoundException DescribeGroup
        echo "Encabulator admins"
        ;;
      "identitystore list-groups") echo "${STUB_GROUP_COUNT:-1}" ;;
      "eks describe-capability")
        name="$(arg capability-name "$@")"
        var="STUB_CAP_$(printf '%s' "$name" | tr '[:lower:]' '[:upper:]')"
        echo "${!var:-ACTIVE}"
        ;;
      "eks update-kubeconfig") echo "Added new context $(arg alias "$@") to kubeconfig" ;;
      "eks describe-cluster") echo "$endpoint" ;;
      "ecr describe-repositories")
        repo="$(arg repository-names "$@")"
        [[ -f "$STUB_STATE/ecr/$repo/tags" ]] || not_found RepositoryNotFoundException DescribeRepositories
        echo "arn:aws:ecr:us-west-2:$account:repository/$repo"
        ;;
      "ecr create-repository")
        repo="$(arg repository-name "$@")"
        mkdir -p "$STUB_STATE/ecr/$repo"
        # Keep the --tags values, one Key=...,Value=... per line.
        printf '%s\n' "$@" | grep '^Key=' >"$STUB_STATE/ecr/$repo/tags"
        echo '{}'
        ;;
      "ecr list-tags-for-resource")
        repo="$(arg resource-arn "$@")"; repo="${repo#*:repository/}"
        sed -e 's/^Key=//' -e 's/,Value=/\t/' "$STUB_STATE/ecr/$repo/tags"
        ;;
      "ecr tag-resource")
        repo="$(arg resource-arn "$@")"; repo="${repo#*:repository/}"
        printf '%s\n' "$@" | grep '^Key=' >"$STUB_STATE/ecr/$repo/tags"
        ;;
      "ecr describe-images")
        repo="$(arg repository-name "$@")" tag="$(arg image-ids "$@")"; tag="${tag#imageTag=}"
        [[ -f "$STUB_STATE/ecr/$repo/tags" ]] || not_found RepositoryNotFoundException DescribeImages
        [[ -f "$STUB_STATE/ecr/$repo/image-$tag" ]] || not_found ImageNotFoundException DescribeImages
        echo "$tag"
        ;;
      "ecr get-login-password") echo "stub-password" ;;
      *) unknown "$@" ;;
    esac
    ;;

  # ------------------------------------------------------------------- terraform
  terraform)
    log "terraform $* | $(env | grep '^TF_VAR_' | sort | paste -sd' ' -)"
    if [[ "${STUB_TF_NO_VERSION:-}" == 1 ]]; then
      # A version manager's shim with no version set (mise 2026.9): Terraform never runs.
      printf '%s\n' "mise ERROR No version is set for shim: terraform" \
        "Set a global default version with one of the following:" "mise use -g terraform@1.14.9" \
        "mise ERROR Version: 2026.9.18 linux-x64 (2026-09-30)" \
        "mise ERROR Run with --verbose or MISE_VERBOSE=1 for more information" >&2
      exit 1
    fi
    if [[ "${1:-}" == "version" ]]; then
      [[ "${STUB_TF_VERSION_FAILS:-}" == 1 ]] && { echo "No version is set for terraform" >&2; exit 126; }
      echo "Terraform ${STUB_TF_VERSION:-v1.14.9}"; echo "on linux_amd64"; exit 0
    fi
    [[ "${1:-}" == -chdir=* ]] || unknown "$@"
    case "${2:-}" in
      init|plan) ;;
      apply) [[ "${STUB_TF_APPLY_FAIL:-}" == 1 ]] && { echo "Error: stub apply failed" >&2; exit 1; } ;;
      output)
        [[ "${3:-} ${4:-}" == "-raw kubeconfig_command" ]] || unknown "$@"
        # STUB_TF_OUTPUT: none (no outputs in the state: a warning, exit 0), missing (the output
        # is not in the state: an error) or backend (another Terraform error), each framed and
        # colored as Terraform prints it.
        esc=$'\033'
        case "${STUB_TF_OUTPUT:-}" in
          none)
            printf '%s\n' "${esc}[33m╷${esc}[0m${esc}[0m" "${esc}[33m│${esc}[0m ${esc}[0m${esc}[1m${esc}[33mWarning: ${esc}[0m${esc}[0m${esc}[1mNo outputs found${esc}[0m" \
              "${esc}[33m│${esc}[0m ${esc}[0m" "${esc}[33m│${esc}[0m ${esc}[0m${esc}[0mThe state file either has no outputs defined, or all the defined outputs are empty.${esc}[0m" \
              "${esc}[33m╵${esc}[0m${esc}[0m" >&2
            exit 0
            ;;
          missing|backend)
            if [[ "$STUB_TF_OUTPUT" == missing ]]; then
              summary='Output "kubeconfig_command" not found'
              detail='The output variable requested could not be found in the state file.'
            else
              summary='Backend initialization required, please run "terraform init"'
              detail='Reason: Initial configuration of the requested backend "s3"'
            fi
            printf '%s\n' "${esc}[31m╷${esc}[0m${esc}[0m" "${esc}[31m│${esc}[0m ${esc}[0m${esc}[1m${esc}[31mError: ${esc}[0m${esc}[0m${esc}[1m$summary${esc}[0m" \
              "${esc}[31m│${esc}[0m ${esc}[0m" "${esc}[31m│${esc}[0m ${esc}[0m${esc}[0m$detail${esc}[0m" \
              "${esc}[31m╵${esc}[0m${esc}[0m" >&2
            exit 1
            ;;
        esac
        echo "aws eks update-kubeconfig --region us-west-2 --name encabulator-usw2"
        ;;
      *) unknown "$@" ;;
    esac
    ;;

  # --------------------------------------------------------------------- kubectl
  kubectl)
    log "kubectl $*"
    case "$*" in
      "version --client") echo "Client Version: v1.35.0" ;;
      "config use-context encabulator-usw2"|"config set-context --current --namespace encabulator") ;;
      "config current-context") echo "encabulator-usw2" ;;
      "config view --minify -o jsonpath={.clusters[0].cluster.server}") echo "$endpoint" ;;
      "--namespace encabulator get configmap encabulator-aws --output jsonpath={.data."*)
        case "$*" in
          *POSTGRES_HOST*) echo "encabulator-pg.example.us-west-2.rds.amazonaws.com" ;;
          *POSTGRES_PORT*) echo "5432" ;;
          *VALKEY_HOST*) echo "encabulator-valkey.example.serverless.usw2.cache.amazonaws.com" ;;
          *VALKEY_PORT*) echo "6379" ;;
        esac
        ;;
      *) unknown "$@" ;;
    esac
    ;;

  # ------------------------------------------------------------------------ helm
  helm)
    log "helm $*"
    case "${1:-}" in
      version) echo "${STUB_HELM_VERSION:-v4.2.4+g3900f43}" ;;
      upgrade) ;;
      *) unknown "$@" ;;
    esac
    ;;

  # ----------------------------------------------------------- docker and finch
  docker|finch)
    log "$tool $*"
    case "${1:-}" in
      login) cat >/dev/null; echo "Login Succeeded" ;;
      build)
        # Record every -t ref that ends in a repository under encabulator/ as pushed.
        push=false; refs=()
        prev=""
        for a in "$@"; do
          [[ "$prev" == "-t" ]] && refs+=("$a")
          [[ "$a" == "--push" ]] && push=true
          prev="$a"
        done
        if $push; then
          for ref in "${refs[@]}"; do
            path="${ref#*/}" tag="${ref##*:}"; repo="${path%:*}"
            mkdir -p "$STUB_STATE/ecr/$repo"
            : >"$STUB_STATE/ecr/$repo/image-$tag"
          done
        fi
        ;;
      *) unknown "$@" ;;
    esac
    ;;

  # ------------------------------------------------------------------------ curl
  curl)
    url="${*: -1}"
    log "curl $url"
    case "$url" in
      https://ghcr.io/token\?*) echo '{"token":"stub-token"}' ;;
      https://ghcr.io/v2/*/manifests/*)
        svc="${url%/manifests/*}"; svc="${svc##*/}"
        case " ${STUB_GHCR_HAVE:-all} " in
          *" all "*|*" $svc "*) printf '200' ;;
          *) printf '404' ;;
        esac
        ;;
      *) unknown "$@" ;;
    esac
    ;;

  *) unknown "$@" ;;
esac
exit 0
