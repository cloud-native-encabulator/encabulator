#!/usr/bin/env bash
# Register the hub cluster with its own Argo CD (EKS Argo CD capability) as `encabulator-usw2`.
#
# The capability identifies clusters by EKS cluster ARN, and the ARN carries the account id, so
# this Secret is created from live AWS values instead of being committed to Git. It also carries
# the environment ids that the hub-brownfield ApplicationSet patches into the adopted ACK CRs:
#   encabulator.io/subnet-ids          subnets of the DB subnet group `encabulator`
#   encabulator.io/security-group-id   security group `encabulator-db`
# The hub gets label encabulator.io/hub=true and NOT the cell label encabulator.io/cell.
#
# Usage: ./register-hub.sh [--dry-run]
#   Needs AWS credentials for the account and kubectl pointed at the hub cluster.
#   --dry-run   validate with the API server and print the Secret; change nothing.
# Environment overrides: CLUSTER_NAME (encabulator-usw2), AWS_REGION (us-west-2).
set -euo pipefail

cluster_name="${CLUSTER_NAME:-encabulator-usw2}"
region="${AWS_REGION:-us-west-2}"
db_subnet_group="encabulator"
security_group_name="encabulator-db"
cache_name="encabulator-valkey"

dry_run=()
case "${1:-}" in
  "") ;;
  --dry-run) dry_run=(--dry-run=server -o yaml) ;;
  *) echo "usage: $0 [--dry-run]" >&2; exit 2 ;;
esac

die() { echo "register-hub: $*" >&2; exit 1; }

# Sorted, comma-separated list from whitespace-separated AWS CLI text output.
join_sorted() { tr -s '[:space:]' '\n' | sed '/^$/d' | sort | paste -sd, -; }

cluster_arn="$(aws eks describe-cluster --region "$region" --name "$cluster_name" \
  --query cluster.arn --output text)"
[[ "$cluster_arn" =~ ^arn:aws[a-z-]*:eks:[a-z0-9-]+:[0-9]{12}:cluster/.+$ ]] \
  || die "unexpected cluster ARN: $cluster_arn"

cluster_endpoint="$(aws eks describe-cluster --region "$region" --name "$cluster_name" \
  --query cluster.endpoint --output text)"
vpc_id="$(aws eks describe-cluster --region "$region" --name "$cluster_name" \
  --query cluster.resourcesVpcConfig.vpcId --output text)"
[[ "$vpc_id" =~ ^vpc-[0-9a-f]+$ ]] || die "unexpected VPC id: $vpc_id"

# Refuse to write the Secret into some other cluster.
current_server="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
[[ "$current_server" == "$cluster_endpoint" ]] \
  || die "kubectl points at $current_server, not $cluster_name ($cluster_endpoint)"

subnet_ids="$(aws rds describe-db-subnet-groups --region "$region" \
  --db-subnet-group-name "$db_subnet_group" \
  --query 'DBSubnetGroups[0].Subnets[].SubnetIdentifier' --output text | join_sorted)"
[[ "$subnet_ids" =~ ^subnet-[0-9a-f]+(,subnet-[0-9a-f]+)+$ ]] \
  || die "DB subnet group $db_subnet_group: expected two or more subnets, got '$subnet_ids'"

security_group_id="$(aws ec2 describe-security-groups --region "$region" \
  --filters "Name=group-name,Values=$security_group_name" "Name=vpc-id,Values=$vpc_id" \
  --query 'SecurityGroups[0].GroupId' --output text)"
[[ "$security_group_id" =~ ^sg-[0-9a-f]+$ ]] \
  || die "security group $security_group_name not found in $vpc_id"

# The cache must sit on the same subnets and the one security group, because the ApplicationSet
# writes the same values into the ServerlessCache CR. A mismatch would make ACK modify the cache.
cache_subnets="$(aws elasticache describe-serverless-caches --region "$region" \
  --serverless-cache-name "$cache_name" \
  --query 'ServerlessCaches[0].SubnetIds' --output text | join_sorted)"
cache_security_groups="$(aws elasticache describe-serverless-caches --region "$region" \
  --serverless-cache-name "$cache_name" \
  --query 'ServerlessCaches[0].SecurityGroupIds' --output text | join_sorted)"
[[ "$cache_subnets" == "$subnet_ids" ]] \
  || die "$cache_name subnets ($cache_subnets) differ from $db_subnet_group ($subnet_ids)"
[[ "$cache_security_groups" == "$security_group_id" ]] \
  || die "$cache_name security groups ($cache_security_groups) differ from $security_group_id"

kubectl apply ${dry_run[@]+"${dry_run[@]}"} -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${cluster_name}
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: cluster
    encabulator.io/hub: "true"
  annotations:
    encabulator.io/region: ${region}
    encabulator.io/subnet-ids: ${subnet_ids}
    encabulator.io/security-group-id: ${security_group_id}
type: Opaque
stringData:
  name: ${cluster_name}
  server: ${cluster_arn}
EOF

if [[ ${#dry_run[@]} -eq 0 ]]; then
  echo "Registered $cluster_name with Argo CD (subnets $subnet_ids, security group $security_group_id)."
fi
