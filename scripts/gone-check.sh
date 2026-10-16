#!/usr/bin/env bash
# Read-only: is anything of the project left in this AWS account? (make gone-check)
#
#   scripts/gone-check.sh                     one PASS, FAIL or WARN line per check
#   GONE_REGIONS="eu-west-1 ap-south-1" ...   look in more regions as well
#
# Looks in the hub region, in every region a file in gitops/hub/cells/ names, and in
# GONE_REGIONS, service by service: EKS, VPCs and their parts, instances, volumes, ENIs, EBS
# snapshots, launch templates, AMIs, RDS, ElastiCache, ECR, log groups, load balancers, KMS,
# Secrets Manager, SSM, CloudFormation, key pairs, then IAM and S3, then the tagging index of
# every other enabled region. Rules learned from the teardown:
#   - the EC2 instance, volume and network interface calls pass --include-managed-resources:
#     without it EC2 leaves out what EKS Auto Mode manages, and nodes that still run read clean;
#   - an AWS error (expired credentials, access denied, throttling) is a FAIL, never "gone";
#   - the tagging index lags by hours, so each entry it lists is asked for in its own service,
#     and one that service no longer knows is counted as stale, not as left;
#   - the EC2 instant-fleet records EKS Auto Mode leaves are a WARN: no cost, nothing to delete.
# Exit 0 only when no check FAILs. Account ids are printed as <account-id>.
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
: "${HUB_REGION:=us-west-2}"
: "${GONE_REGIONS:=}"
: "${GONE_ALL_REGIONS:=yes}"

account=""
# The account id of the credentials, and any other account id in an ARN, as <account-id>.
mask() {
  local p="$account"
  [[ -n "$p" ]] || p='[0-9]{12}'
  sed -E -e "s/$p/<account-id>/g" -e 's/:[0-9]{12}:/:<account-id>:/g'
}
fails=0 passes=0 warns=0
pass() { passes=$((passes + 1)); echo "PASS   $1"; }
fail() { fails=$((fails + 1)); echo "FAIL   $1"; }
warn() { warns=$((warns + 1)); echo "WARN   $1"; }

# result LABEL STATUS OUTPUT: PASS when the call worked and printed nothing.
result() {
  local out
  out="$(printf '%s\n' "$3" | sed '/^None$/d;/^$/d;/^\[\]$/d')"
  if [[ "$2" -ne 0 ]]; then fail "$1: AWS error: $(printf '%s' "$3" | tail -n 1 | mask)"
  elif [[ -n "$out" ]]; then fail "$1: $(printf '%s' "$out" | tr '\n\t' '  ' | mask)"
  else pass "$1"; fi
}
# c LABEL COMMAND...: run one read-only call and report it.
c() { local label="$1" out rc; shift; out="$("$@" 2>&1)"; rc=$?; result "$label" "$rc" "$out"; }

# gone REGION ARN: 0 when the resource's own service says it no longer exists, 1 when it does,
# or when the type is not one this check knows (then it is reported, never hidden).
gone() {
  local r="$1" a="$2" svc res typ id s
  svc="$(cut -d: -f3 <<<"$a")"; res="$(cut -d: -f6- <<<"$a")"
  typ="${res%%[/:]*}"; id="${res#*[/:]}"
  case "$svc:$typ" in
    ec2:instance)
      s="$(aws ec2 describe-instances --region "$r" --include-managed-resources --instance-ids "$id" \
        --query 'Reservations[].Instances[].State.Name' --output text 2>&1)"
      # An instance terminated long ago answers with no reservation at all: gone.
      [[ -n "$s" ]] || s=terminated ;;
    ec2:natgateway) s="$(aws ec2 describe-nat-gateways --region "$r" --nat-gateway-ids "$id" --query 'NatGateways[].State' --output text 2>&1)" ;;
    ec2:network-interface) s="$(aws ec2 describe-network-interfaces --region "$r" --include-managed-resources --network-interface-ids "$id" 2>&1)" ;;
    ec2:volume) s="$(aws ec2 describe-volumes --region "$r" --include-managed-resources --volume-ids "$id" 2>&1)" ;;
    ec2:security-group) s="$(aws ec2 describe-security-groups --region "$r" --group-ids "$id" 2>&1)" ;;
    ec2:security-group-rule) s="$(aws ec2 describe-security-group-rules --region "$r" --security-group-rule-ids "$id" 2>&1)" ;;
    ec2:vpc-endpoint) s="$(aws ec2 describe-vpc-endpoints --region "$r" --vpc-endpoint-ids "$id" --query 'VpcEndpoints[].State' --output text 2>&1)" ;;
    ec2:launch-template) s="$(aws ec2 describe-launch-templates --region "$r" --launch-template-ids "$id" 2>&1)" ;;
    ec2:vpc) s="$(aws ec2 describe-vpcs --region "$r" --vpc-ids "$id" 2>&1)" ;;
    ec2:subnet) s="$(aws ec2 describe-subnets --region "$r" --subnet-ids "$id" 2>&1)" ;;
    ec2:route-table) s="$(aws ec2 describe-route-tables --region "$r" --route-table-ids "$id" 2>&1)" ;;
    ec2:internet-gateway) s="$(aws ec2 describe-internet-gateways --region "$r" --internet-gateway-ids "$id" 2>&1)" ;;
    ec2:network-acl) s="$(aws ec2 describe-network-acls --region "$r" --network-acl-ids "$id" 2>&1)" ;;
    ec2:elastic-ip) s="$(aws ec2 describe-addresses --region "$r" --allocation-ids "$id" 2>&1)" ;;
    ec2:snapshot) s="$(aws ec2 describe-snapshots --region "$r" --snapshot-ids "$id" 2>&1)" ;;
    elasticache:serverlesscache) s="$(aws elasticache describe-serverless-caches --region "$r" --serverless-cache-name "$id" --query 'length(ServerlessCaches)' --output text 2>&1)" ;;
    elasticache:serverlesscachesnapshot) s="$(aws elasticache describe-serverless-cache-snapshots --region "$r" --serverless-cache-snapshot-name "$id" --query 'length(ServerlessCacheSnapshots)' --output text 2>&1)" ;;
    rds:db) s="$(aws rds describe-db-instances --region "$r" --db-instance-identifier "$id" 2>&1)" ;;
    rds:snapshot) s="$(aws rds describe-db-snapshots --region "$r" --db-snapshot-identifier "$id" 2>&1)" ;;
    rds:subgrp) s="$(aws rds describe-db-subnet-groups --region "$r" --db-subnet-group-name "$id" 2>&1)" ;;
    eks:cluster|eks:access-entry|eks:capability|eks:podidentityassociation|eks:nodegroup)
      # These go with their cluster.
      s="$(aws eks describe-cluster --region "$r" --name "${id%%/*}" 2>&1)" ;;
    ecr:repository) s="$(aws ecr describe-repositories --region "$r" --repository-names "$id" 2>&1)" ;;
    # IAM is global; the index lists it in us-east-1.
    iam:role) s="$(aws iam get-role --role-name "${id##*/}" 2>&1)" ;;
    iam:instance-profile) s="$(aws iam get-instance-profile --instance-profile-name "${id##*/}" 2>&1)" ;;
    *) return 1 ;;
  esac
  case "$svc:$s" in
    elasticache:0) return 0 ;;
    *:terminated|*:deleted|*NotFound*|*"not found"*|*NoSuchEntity*|*"does not exist"*) return 0 ;;
    *) return 1 ;;
  esac
}

# index REGION: every entry of the tagging index is either stale (its service says gone) or a FAIL.
index() {
  local r="$1" idx a n_stale=0 n_fleet=0 live="" types=""
  if ! idx="$(aws resourcegroupstaggingapi get-resources --region "$r" --tag-filters Key=project,Values=encabulator \
      --query 'ResourceTagMappingList[].ResourceARN' --output text 2>&1)"; then
    fail "$r tagging index: AWS error: $(tail -n 1 <<<"$idx" | mask)"; return
  fi
  for a in $idx; do
    [[ "$a" == None ]] && continue
    case "$a" in *:fleet/*) n_fleet=$((n_fleet + 1)); continue ;; esac
    if gone "$r" "$a"; then
      n_stale=$((n_stale + 1)); types+=" $(cut -d: -f3 <<<"$a"):$(cut -d: -f6- <<<"$a" | sed 's#[/:].*##')"
    else live+=" $a"; fi
  done
  if [[ -z "$live" ]]; then
    pass "$r tagging index: nothing left; $n_stale stale entries, each confirmed gone in its own service"
    [[ -z "$types" ]] || echo "       stale by type: $(tr ' ' '\n' <<<"$types" | sed '/^$/d' | sort | uniq -c | awk '{ printf "%s %s, ", $1, $2 }' | sed 's/, $//')"
  else
    fail "$r tagging index lists what is still there:$(mask <<<"$live")"
  fi
  [[ $n_fleet -eq 0 ]] || warn "$r $n_fleet EC2 instant-fleet records from EKS Auto Mode (no cost, nothing to delete)"
}

region_checks() {
  local r="$1" q='Name=tag:project,Values=encabulator' up='pending,running,stopping,stopped,shutting-down'
  echo "== $r"
  c "$r EKS clusters encabulator*" aws eks list-clusters --region "$r" --query 'clusters[?starts_with(@, `encabulator`)]' --output text
  c "$r VPCs" aws ec2 describe-vpcs --region "$r" --filters "$q" --query 'Vpcs[].VpcId' --output text
  c "$r VPCs named encabulator*" aws ec2 describe-vpcs --region "$r" --filters 'Name=tag:Name,Values=encabulator*' --query 'Vpcs[].VpcId' --output text
  c "$r subnets" aws ec2 describe-subnets --region "$r" --filters "$q" --query 'Subnets[].SubnetId' --output text
  c "$r route tables" aws ec2 describe-route-tables --region "$r" --filters "$q" --query 'RouteTables[].RouteTableId' --output text
  c "$r route tables named encabulator*" aws ec2 describe-route-tables --region "$r" --filters 'Name=tag:Name,Values=encabulator*' --query 'RouteTables[].RouteTableId' --output text
  c "$r NAT gateways" aws ec2 describe-nat-gateways --region "$r" --filter "$q" --query 'NatGateways[?State!=`deleted`].NatGatewayId' --output text
  c "$r internet gateways" aws ec2 describe-internet-gateways --region "$r" --filters "$q" --query 'InternetGateways[].InternetGatewayId' --output text
  c "$r Elastic IPs" aws ec2 describe-addresses --region "$r" --filters "$q" --query 'Addresses[].AllocationId' --output text
  c "$r VPC endpoints" aws ec2 describe-vpc-endpoints --region "$r" --filters "$q" --query 'VpcEndpoints[?State!=`deleted`].VpcEndpointId' --output text
  c "$r security groups" aws ec2 describe-security-groups --region "$r" --filters "$q" --query 'SecurityGroups[].GroupId' --output text
  c "$r security groups encabulator-db, eks-cluster-sg-encabulator*" aws ec2 describe-security-groups --region "$r" \
    --filters 'Name=group-name,Values=encabulator-db,eks-cluster-sg-encabulator*' --query 'SecurityGroups[].GroupId' --output text
  c "$r network interfaces" aws ec2 describe-network-interfaces --region "$r" --include-managed-resources --filters "$q" \
    --query 'NetworkInterfaces[].NetworkInterfaceId' --output text
  c "$r instances" aws ec2 describe-instances --region "$r" --include-managed-resources \
    --filters "$q" "Name=instance-state-name,Values=$up" --query 'Reservations[].Instances[].InstanceId' --output text
  c "$r instances of an encabulator* cluster" aws ec2 describe-instances --region "$r" --include-managed-resources \
    --filters 'Name=tag:eks:eks-cluster-name,Values=encabulator*' "Name=instance-state-name,Values=$up" --query 'Reservations[].Instances[].InstanceId' --output text
  c "$r volumes" aws ec2 describe-volumes --region "$r" --include-managed-resources --filters "$q" --query 'Volumes[].VolumeId' --output text
  c "$r volumes of an encabulator* cluster" aws ec2 describe-volumes --region "$r" --include-managed-resources \
    --filters 'Name=tag:eks:eks-cluster-name,Values=encabulator*' --query 'Volumes[].VolumeId' --output text
  c "$r EBS snapshots" aws ec2 describe-snapshots --region "$r" --owner-ids self --filters "$q" --query 'Snapshots[].SnapshotId' --output text
  c "$r launch templates" aws ec2 describe-launch-templates --region "$r" --include-managed-resources \
    --filters 'Name=tag:eks:eks-cluster-name,Values=encabulator*' --query 'LaunchTemplates[].LaunchTemplateId' --output text
  c "$r AMIs" aws ec2 describe-images --region "$r" --owners self --filters "$q" --query 'Images[].ImageId' --output text
  c "$r key pairs encabulator*" aws ec2 describe-key-pairs --region "$r" --query 'KeyPairs[?contains(KeyName, `encabulator`)].KeyName' --output text
  c "$r RDS instances" aws rds describe-db-instances --region "$r" \
    --query 'DBInstances[?starts_with(DBInstanceIdentifier, `encabulator`)].DBInstanceIdentifier' --output text
  c "$r RDS subnet groups" aws rds describe-db-subnet-groups --region "$r" \
    --query 'DBSubnetGroups[?starts_with(DBSubnetGroupName, `encabulator`)].DBSubnetGroupName' --output text
  c "$r RDS snapshots" aws rds describe-db-snapshots --region "$r" \
    --query 'DBSnapshots[?starts_with(DBInstanceIdentifier, `encabulator`)].DBSnapshotIdentifier' --output text
  c "$r RDS automated backups" aws rds describe-db-instance-automated-backups --region "$r" \
    --query 'DBInstanceAutomatedBackups[?starts_with(DBInstanceIdentifier, `encabulator`)].DBInstanceIdentifier' --output text
  c "$r ElastiCache serverless caches" aws elasticache describe-serverless-caches --region "$r" \
    --query 'ServerlessCaches[?starts_with(ServerlessCacheName, `encabulator`)].ServerlessCacheName' --output text
  c "$r ElastiCache snapshots" aws elasticache describe-serverless-cache-snapshots --region "$r" \
    --query 'ServerlessCacheSnapshots[?contains(ServerlessCacheSnapshotName, `encabulator`)].ServerlessCacheSnapshotName' --output text
  c "$r ECR repositories" aws ecr describe-repositories --region "$r" \
    --query 'repositories[?starts_with(repositoryName, `encabulator`)].repositoryName' --output text
  c "$r log groups" aws logs describe-log-groups --region "$r" --query 'logGroups[?contains(logGroupName, `encabulator`)].logGroupName' --output text
  c "$r load balancers" aws elbv2 describe-load-balancers --region "$r" \
    --query 'LoadBalancers[?contains(LoadBalancerName, `encabulator`) || starts_with(LoadBalancerName, `k8s-encabula`)].LoadBalancerName' --output text
  c "$r target groups" aws elbv2 describe-target-groups --region "$r" \
    --query 'TargetGroups[?contains(TargetGroupName, `encabulator`) || starts_with(TargetGroupName, `k8s-encabula`)].TargetGroupName' --output text
  c "$r KMS aliases" aws kms list-aliases --region "$r" --query 'Aliases[?contains(AliasName, `encabulator`)].AliasName' --output text
  c "$r Secrets Manager" aws secretsmanager list-secrets --region "$r" --include-planned-deletion \
    --query 'SecretList[?contains(Name, `encabulator`)].Name' --output text
  c "$r SSM parameters" aws ssm describe-parameters --region "$r" --query 'Parameters[?contains(Name, `encabulator`)].Name' --output text
  c "$r CloudFormation stacks" aws cloudformation list-stacks --region "$r" \
    --query 'StackSummaries[?StackStatus!=`DELETE_COMPLETE` && contains(StackName, `encabulator`)].StackName' --output text
  index "$r"
}

# The regions: the hub's, every cell's, and GONE_REGIONS.
regions="$HUB_REGION $(awk '/^spec:/ { s = 1; next } /^[^ #]/ { s = 0 } s && /^  region:/ { print $2 }' "$repo_root"/gitops/hub/cells/*.yaml 2>/dev/null) ${GONE_REGIONS//,/ }"
regions="$(tr ' ' '\n' <<<"$regions" | sed '/^$/d' | awk '!seen[$0]++' | paste -sd' ' -)"

echo "gone-check $(date -u +%FT%TZ), regions: $regions"
if ! who="$(aws sts get-caller-identity --query Arn --output text 2>&1)"; then
  echo "FAIL   no working AWS credentials: $(tail -n 1 <<<"$who" | mask)"
  echo; echo "1 FAIL line(s): nothing was checked."
  exit 1
fi
account="$(cut -d: -f5 <<<"$who")"
[[ "$account" =~ ^[0-9]{12}$ ]] || account=""
echo "as $(mask <<<"$who")"
for r in $regions; do region_checks "$r"; done

echo "== global"
c "IAM roles encabulator*" aws iam list-roles --query 'Roles[?starts_with(RoleName, `encabulator`)].RoleName' --output text
c "IAM instance profiles" aws iam list-instance-profiles \
  --query 'InstanceProfiles[?contains(InstanceProfileName, `encabulator`) || contains(Path, `encabulator`)].InstanceProfileName' --output text
c "IAM customer policies" aws iam list-policies --scope Local --query 'Policies[?contains(PolicyName, `encabulator`)].PolicyName' --output text
c "S3 buckets" aws s3api list-buckets --query 'Buckets[?contains(Name, `encabulator`)].Name' --output text
oidc_left="" plist="$(aws iam list-open-id-connect-providers --query 'OpenIDConnectProviderList[].Arn' --output text 2>&1)"; prc=$?
if [[ $prc -eq 0 ]]; then
  for p in $plist; do
    [[ "$p" == None ]] && continue
    t="$(aws iam list-open-id-connect-provider-tags --open-id-connect-provider-arn "$p" \
      --query 'Tags[?Key==`project` && Value==`encabulator`] | length(@)' --output text 2>&1)" || { prc=1; plist="$t"; break; }
    [[ "$t" == 0 ]] || oidc_left+=" $p"
  done
fi
result "IAM OIDC providers tagged project=encabulator" "$prc" "$([[ $prc -eq 0 ]] && echo "$oidc_left" || echo "$plist")"

if [[ "$GONE_ALL_REGIONS" == yes ]]; then
  echo "== every other enabled region: the tagging index"
  if all="$(aws ec2 describe-regions --region "$HUB_REGION" --query 'Regions[].RegionName' --output text 2>&1)"; then
    for r in $all; do
      [[ " $regions " == *" $r "* ]] && continue
      n="$(aws resourcegroupstaggingapi get-resources --region "$r" --tag-filters Key=project,Values=encabulator \
        --query 'length(ResourceTagMappingList)' --output text 2>&1)" || { fail "$r tagging index: AWS error: $(tail -n 1 <<<"$n" | mask)"; continue; }
      [[ "$n" == 0 ]] && continue
      echo "-- $r has $n entries"; index "$r"
    done
    pass "the other enabled regions (a region with an entry is listed above)"
  else
    fail "the list of enabled regions: AWS error: $(tail -n 1 <<<"$all" | mask)"
  fi
fi

echo
if [[ $fails -eq 0 ]]; then
  echo "Nothing of the project is left: $passes checks passed, $warns warning(s)."
  exit 0
fi
echo "$fails check(s) FAIL: the lines marked FAIL above are still in the account ($passes passed, $warns warning(s))."
exit 1
