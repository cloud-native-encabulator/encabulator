#!/usr/bin/env bash
# Create one private ECR repository per service (idempotent) and log the container runtime
# in, so `make push REGISTRY=<account>.dkr.ecr.<region>.amazonaws.com/encabulator` can
# publish images an EKS cluster in the same account pulls without secrets.
# `make mirror-images` calls this for you.
#
#   scripts/ecr-repos.sh [region]          # default: us-west-2, the hub region
#
# Every repository carries auto-delete=no, project=encabulator and talk=opn315 from the
# moment it is created (docs/TAGGING.md; scripts/tag-check.py checks them). A repository
# that already exists without them gets them added; no other tag is touched.
set -euo pipefail

region="${1:-us-west-2}"
runtime="${CONTAINER_RUNTIME:-finch}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
account="$(aws sts get-caller-identity --query Account --output text)"
registry="$account.dkr.ecr.$region.amazonaws.com"
tags=("Key=auto-delete,Value=no" "Key=project,Value=encabulator" "Key=talk,Value=opn315")

while read -r name _; do
  repo="encabulator/$name"
  if arn="$(aws ecr describe-repositories --region "$region" --repository-names "$repo" \
      --query 'repositories[0].repositoryArn' --output text 2>/dev/null)"; then
    have="$(aws ecr list-tags-for-resource --region "$region" --resource-arn "$arn" \
      --query 'tags[].[Key,Value]' --output text | tr '\t' '=' | sort | paste -sd' ' -)"
    if [[ " $have " == *" auto-delete=no "* && " $have " == *" project=encabulator "* \
          && " $have " == *" talk=opn315 "* ]]; then
      echo "exists   $repo"
    else
      aws ecr tag-resource --region "$region" --resource-arn "$arn" --tags "${tags[@]}"
      echo "tagged   $repo"
    fi
  else
    aws ecr create-repository --region "$region" --repository-name "$repo" \
      --image-scanning-configuration scanOnPush=true \
      --encryption-configuration encryptionType=AES256 \
      --tags "${tags[@]}" >/dev/null
    echo "created  $repo"
  fi
done < <("$repo_root/scripts/services.sh" pairs)

aws ecr get-login-password --region "$region" | "$runtime" login --username AWS --password-stdin "$registry"
echo
echo "REGISTRY=$registry/encabulator"
