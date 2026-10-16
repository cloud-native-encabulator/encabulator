#!/usr/bin/env bash
# make prereqs: check, read-only, everything `make hub` needs, and list every problem at once,
# each with its fix. Exit 0 when nothing is missing, 1 otherwise.
#
# Checks: bash, GNU make, git, AWS CLI v2 and working credentials, Terraform 1.7 to 1.x,
# kubectl, Helm 4, python3, curl; the .env file and its two required values; that the
# Identity Center instance and the Argo CD admin group exist (read-only sso-admin and
# identitystore calls). A container runtime is only needed when images have to be mirrored,
# so a missing one is a warning.
#
# Reads the values make resolved (command line, environment, .env): IDC_INSTANCE_ARN,
# IDC_REGION, ARGOCD_ADMIN_GROUP_ID, REGISTRY, CONTAINER_RUNTIME, plus TERRAFORM, TF_DIR,
# HUB_REGION and DOTENV_FILE.
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root" || exit 1

: "${TERRAFORM:=terraform}"
: "${TF_DIR:=terraform/brownfield}"
: "${HUB_REGION:=us-west-2}"
: "${DOTENV_FILE:=.env}"
: "${CONTAINER_RUNTIME:=finch}"
: "${REGISTRY:=ghcr.io/jlbutler/cloud-native-encabulator}"
: "${IDC_INSTANCE_ARN:=}"
: "${IDC_REGION:=}"
: "${ARGOCD_ADMIN_GROUP_ID:=}"
default_registry="ghcr.io/jlbutler/cloud-native-encabulator"
idc_region="${IDC_REGION:-$HUB_REGION}"

# The placeholders .env.example ships.
placeholder_arn="arn:aws:sso:::instance/ssoins-0123456789abcdef"
placeholder_group="00000000-0000-0000-0000-000000000000"

bold=$'\033[1m'; red=$'\033[31m'; yellow=$'\033[33m'; green=$'\033[32m'; off=$'\033[0m'
problems=()
warnings=()
ok()      { printf '  %sok%s    %s\n' "$green" "$off" "$1"; }
problem() { printf '  %sFAIL%s  %s\n' "$red" "$off" "$1"; problems+=("$1"$'\n'"        fix: $2"); }
warn()    { printf '  %swarn%s  %s\n' "$yellow" "$off" "$1"; warnings+=("$1"$'\n'"        $2"); }
have()    { command -v "$1" >/dev/null 2>&1; }
# first_line COMMAND...: the first line of COMMAND's output (read whole, so no SIGPIPE).
first_line() { local o; o="$("$@" 2>/dev/null)" || return 1; printf '%s' "${o%%$'\n'*}"; }
# shellcheck source=terraform-error.sh
source "$repo_root/scripts/terraform-error.sh"

printf '%s==> make prereqs%s\n' "$bold" "$off"

# ------------------------------------------------------------------------ tools
ok "bash ${BASH_VERSION}"

if have make && make_version="$(first_line make --version)" && [[ "$make_version" == "GNU Make"* ]]; then
  ok "$make_version"
else
  problem "GNU make not found" "install GNU make (on macOS it comes with the Xcode command line tools: xcode-select --install)"
fi

if have git; then ok "$(git --version)"; else problem "git not found" "install git"; fi

aws_ok=false
if have aws; then
  aws_version="$(aws --version 2>&1)"
  if [[ "$aws_version" == aws-cli/2.* ]]; then
    ok "${aws_version%% *}"
    aws_ok=true
  else
    problem "AWS CLI v2 needed, found: ${aws_version%% *}" "install AWS CLI v2: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"
  fi
else
  problem "AWS CLI not found" "install AWS CLI v2: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"
fi

creds_ok=false
if $aws_ok; then
  if principal="$(aws sts get-caller-identity --query Arn --output text 2>/dev/null)" && [[ -n "$principal" ]]; then
    ok "AWS credentials: $principal"
    creds_ok=true
  else
    problem "no working AWS credentials (aws sts get-caller-identity failed)" "sign in (for example aws sso login), or export AWS_PROFILE=<profile> in this shell"
  fi
fi

read -r -a terraform <<<"$TERRAFORM"
# Terraform's error output is kept, so a version manager's shim with no version set is named.
tf_rc=1 tf_version="" tf_errors=""
if have "${terraform[0]}"; then
  tf_errors="$(mktemp)"
  trap 'rm -f "$tf_errors"' EXIT
  tf_out="$("${terraform[@]}" version 2>"$tf_errors")" && tf_rc=0
  tf_version="${tf_out%%$'\n'*}"
fi
if [[ $tf_rc -eq 0 ]]; then
  if [[ "$tf_version" =~ ^Terraform\ v([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
    major="${BASH_REMATCH[1]}" minor="${BASH_REMATCH[2]}"
    if [[ "$major" -eq 1 && "$minor" -ge 7 ]]; then
      ok "$tf_version (1.14.9 is the version tested)"
    else
      problem "Terraform 1.7 or newer 1.x needed, found ${tf_version#Terraform }" "install Terraform 1.14.9, or $(terraform_hint)"
    fi
  else
    problem "could not read the Terraform version from: $tf_version" "check that '$TERRAFORM version' works"
  fi
elif have "${terraform[0]}"; then
  problem "'$TERRAFORM version' fails: $(terraform_error "$tf_errors")" "$(terraform_hint) and run make prereqs again; otherwise check the Terraform install"
else
  problem "Terraform not found ($TERRAFORM)" "install Terraform 1.14.9 (https://developer.hashicorp.com/terraform/install), or $(terraform_hint)"
fi

if have kubectl; then
  kubectl_version="$(kubectl version --client 2>/dev/null)"
  kubectl_version="${kubectl_version#*Client Version: }"
  ok "kubectl ${kubectl_version%%$'\n'*}"
else
  problem "kubectl not found" "install kubectl: https://kubernetes.io/docs/tasks/tools/"
fi

if have helm; then
  helm_version="$(helm version --short 2>/dev/null)"
  if [[ "$helm_version" == v4.* ]]; then
    ok "helm $helm_version"
  else
    problem "Helm 4 needed, found ${helm_version:-an unknown version}" "install Helm 4: https://helm.sh/docs/intro/install/"
  fi
else
  problem "helm not found" "install Helm 4: https://helm.sh/docs/intro/install/"
fi

if have python3; then ok "$(python3 --version 2>&1)"; else problem "python3 not found" "install Python 3 (make tag-check runs it)"; fi
if have curl; then ok "curl"; else problem "curl not found" "install curl (make mirror-images checks GHCR with it)"; fi

# ------------------------------------------------------------------------- .env
if [[ -f "$DOTENV_FILE" ]]; then
  ok "$DOTENV_FILE found"
else
  problem "$DOTENV_FILE not found" "cp .env.example $DOTENV_FILE, then fill in IDC_INSTANCE_ARN and ARGOCD_ADMIN_GROUP_ID"
fi

arn_pattern='^arn:aws[a-z-]*:sso:::instance/ssoins-[0-9a-f]+$'
arn_ok=false
if [[ -z "$IDC_INSTANCE_ARN" ]]; then
  problem "IDC_INSTANCE_ARN is not set" "set it in $DOTENV_FILE: aws sso-admin list-instances --region <identity-center-region> --no-paginate --query 'Instances[0].InstanceArn' --output text"
elif [[ "$IDC_INSTANCE_ARN" == "$placeholder_arn" ]]; then
  problem "IDC_INSTANCE_ARN is still the placeholder from .env.example" "replace it in $DOTENV_FILE with your instance: aws sso-admin list-instances --region <identity-center-region> --no-paginate --query 'Instances[0].InstanceArn' --output text"
elif [[ ! "$IDC_INSTANCE_ARN" =~ $arn_pattern ]]; then
  problem "IDC_INSTANCE_ARN does not look like arn:aws:sso:::instance/ssoins-..." "copy the InstanceArn from aws sso-admin list-instances into $DOTENV_FILE"
else
  ok "IDC_INSTANCE_ARN is set"
  arn_ok=true
fi

group_pattern='^([0-9a-f]{10}-)?[A-Fa-f0-9]{8}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{12}$'
group_ok=false
if [[ -z "$ARGOCD_ADMIN_GROUP_ID" ]]; then
  problem "ARGOCD_ADMIN_GROUP_ID is not set" "set it in $DOTENV_FILE to the GroupId of an Identity Center group, not a user (.env.example shows how to list the groups or create one)"
elif [[ "$ARGOCD_ADMIN_GROUP_ID" == "$placeholder_group" ]]; then
  problem "ARGOCD_ADMIN_GROUP_ID is still the placeholder from .env.example" "replace it in $DOTENV_FILE with the GroupId of an Identity Center group, not a user (.env.example shows how to list the groups or create one)"
elif [[ ! "$ARGOCD_ADMIN_GROUP_ID" =~ $group_pattern ]]; then
  problem "ARGOCD_ADMIN_GROUP_ID does not look like an Identity Center group id" "copy the GroupId of an Identity Center group into $DOTENV_FILE (.env.example shows how to list the groups or create one)"
else
  ok "ARGOCD_ADMIN_GROUP_ID is set"
  group_ok=true
fi

# ------------------------------------------------------- Identity Center (read-only)
if $creds_ok && $arn_ok; then
  if instances="$(aws sso-admin list-instances --region "$idc_region" \
      --query 'Instances[].[InstanceArn,IdentityStoreId]' --output text 2>&1)"; then
    store="$(awk -v arn="$IDC_INSTANCE_ARN" '$1 == arn { print $2 }' <<<"$instances")"
    if [[ -n "$store" ]]; then
      ok "Identity Center instance found in $idc_region"
      if $group_ok; then
        if group_name="$(aws identitystore describe-group --region "$idc_region" \
            --identity-store-id "$store" --group-id "$ARGOCD_ADMIN_GROUP_ID" \
            --query DisplayName --output text 2>&1)"; then
          ok "Argo CD admin group: $group_name"
        elif [[ "$group_name" == *ResourceNotFoundException* || "$group_name" == *ValidationException* ]]; then
          problem "group $ARGOCD_ADMIN_GROUP_ID not found in that Identity Center" "it must be a group id, not a user id; copy one from: aws identitystore list-groups --region $idc_region --no-paginate --identity-store-id $store"
        else
          warn "could not check the group (identitystore:DescribeGroup)" "make sure $ARGOCD_ADMIN_GROUP_ID is a group in that Identity Center"
        fi
      elif groups="$(aws identitystore list-groups --region "$idc_region" --no-paginate \
          --identity-store-id "$store" --query 'length(Groups)' --output text 2>/dev/null)" \
          && [[ "$groups" == "0" ]]; then
        # No usable group id yet, and nothing to copy one from: say what to do about it.
        problem "that Identity Center has no groups yet" "create a group, add yourself to it and put its GroupId in $DOTENV_FILE (the ARGOCD_ADMIN_GROUP_ID comment in .env.example has the commands)"
      fi
    else
      problem "no Identity Center instance $IDC_INSTANCE_ARN in $idc_region" "check IDC_INSTANCE_ARN, or set IDC_REGION in $DOTENV_FILE to the region Identity Center runs in"
    fi
  else
    warn "could not list Identity Center instances in $idc_region (sso-admin:ListInstances)" "make sure IDC_INSTANCE_ARN is right and Identity Center is enabled"
  fi
fi

# ------------------------------------------------------------- container runtime
if have "$CONTAINER_RUNTIME"; then
  ok "container runtime $CONTAINER_RUNTIME"
elif [[ "$REGISTRY" != "$default_registry" ]]; then
  warn "$CONTAINER_RUNTIME not found" "needed only to build images into REGISTRY (make mirror-images); set CONTAINER_RUNTIME=docker or finch"
elif have curl && "$repo_root/scripts/mirror-images.sh" --check >/dev/null 2>&1; then
  ok "no container runtime needed: every image is on GHCR"
else
  warn "$CONTAINER_RUNTIME not found, and GHCR does not have every image at $(tr -d '[:space:]' <VERSION)" "make mirror-images will build them: install Docker or Finch and set CONTAINER_RUNTIME in $DOTENV_FILE"
fi

# ------------------------------------------------------------------- tfvars file
if [[ -f "$TF_DIR/encabulator.auto.tfvars" ]]; then
  warn "$TF_DIR/encabulator.auto.tfvars exists" "Terraform takes idc_instance_arn and the Argo CD mappings from it before $DOTENV_FILE; remove it to use $DOTENV_FILE alone"
fi

# ---------------------------------------------------------------------- summary
echo
if [[ ${#warnings[@]} -gt 0 ]]; then
  printf '%s%d warning(s):%s\n' "$yellow" "${#warnings[@]}" "$off"
  for w in "${warnings[@]}"; do printf '  - %s\n' "$w"; done
  echo
fi
if [[ ${#problems[@]} -gt 0 ]]; then
  printf '%s%d problem(s) to fix before make hub:%s\n' "$red" "${#problems[@]}" "$off"
  for p in "${problems[@]}"; do printf '  - %s\n' "$p"; done
  exit 1
fi
printf '%sEverything make hub needs is in place.%s\n' "$green" "$off"
