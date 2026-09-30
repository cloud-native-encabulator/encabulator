#!/usr/bin/env bash
# Create one private ECR repository per service (idempotent) and log the
# container runtime in, so `make push REGISTRY=<account>.dkr.ecr.<region>.amazonaws.com/encabulator`
# can publish images an EKS cluster in the same account pulls without secrets.
#
#   scripts/ecr-repos.sh [region]          # default: us-east-2
set -euo pipefail

region="${1:-${AWS_REGION:-us-east-2}}"
runtime="${CONTAINER_RUNTIME:-finch}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
account="$(aws sts get-caller-identity --query Account --output text)"
registry="$account.dkr.ecr.$region.amazonaws.com"

while read -r name _; do
  if aws ecr describe-repositories --region "$region" --repository-names "encabulator/$name" >/dev/null 2>&1; then
    echo "exists   encabulator/$name"
  else
    aws ecr create-repository --region "$region" --repository-name "encabulator/$name" \
      --image-scanning-configuration scanOnPush=true \
      --encryption-configuration encryptionType=AES256 \
      --tags Key=project,Value=cloud-native-encabulator >/dev/null
    echo "created  encabulator/$name"
  fi
done < <("$repo_root/scripts/services.sh" pairs)

aws ecr get-login-password --region "$region" | "$runtime" login --username AWS --password-stdin "$registry"
echo
echo "REGISTRY=$registry/encabulator"
