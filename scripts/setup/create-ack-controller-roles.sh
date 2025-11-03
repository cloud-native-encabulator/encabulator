#!/bin/bash
set -euo pipefail

# Create IAM roles for ACK RDS and ElastiCache controllers
# These roles allow the controllers to manage AWS resources

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Configuration
CLUSTER_NAME="${CLUSTER_NAME:-molly}"
AWS_REGION="${AWS_REGION:-us-east-2}"
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-}"
NAMESPACE="ack-system"

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Get AWS account ID
if [ -z "$AWS_ACCOUNT_ID" ]; then
    AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
fi

# Get OIDC provider
OIDC_PROVIDER=$(aws eks describe-cluster --name "$CLUSTER_NAME" --region "$AWS_REGION" --query "cluster.identity.oidc.issuer" --output text | sed 's|https://||')

log_info "Creating IAM roles for ACK controllers..."
log_info "Account: $AWS_ACCOUNT_ID"
log_info "Cluster: $CLUSTER_NAME"
log_info "OIDC: $OIDC_PROVIDER"
echo ""

# Create RDS Controller Role
log_info "Creating RDS controller IAM role..."

RDS_TRUST_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "${OIDC_PROVIDER}:sub": "system:serviceaccount:${NAMESPACE}:ack-rds-controller",
          "${OIDC_PROVIDER}:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
EOF
)

RDS_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "rds:CreateDBInstance",
        "rds:DeleteDBInstance",
        "rds:DescribeDBInstances",
        "rds:ModifyDBInstance",
        "rds:AddTagsToResource",
        "rds:RemoveTagsFromResource",
        "rds:ListTagsForResource",
        "rds:CreateDBSubnetGroup",
        "rds:DeleteDBSubnetGroup",
        "rds:DescribeDBSubnetGroups",
        "rds:ModifyDBSubnetGroup",
        "rds:CreateDBParameterGroup",
        "rds:DeleteDBParameterGroup",
        "rds:DescribeDBParameterGroups",
        "rds:ModifyDBParameterGroup",
        "rds:DescribeDBParameters",
        "rds:ResetDBParameterGroup",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSubnets",
        "ec2:DescribeVpcs",
        "ec2:DescribeAvailabilityZones"
      ],
      "Resource": "*"
    }
  ]
}
EOF
)

# Create or update RDS role
if aws iam get-role --role-name ack-rds-controller-role &> /dev/null; then
    log_warn "RDS controller role already exists, updating trust policy..."
    aws iam update-assume-role-policy \
        --role-name ack-rds-controller-role \
        --policy-document "$RDS_TRUST_POLICY"
else
    aws iam create-role \
        --role-name ack-rds-controller-role \
        --assume-role-policy-document "$RDS_TRUST_POLICY" \
        --description "IAM role for ACK RDS controller" \
        --tags Key=ManagedBy,Value=Script Key=Service,Value=rds-controller
    log_info "RDS controller role created"
fi

# Create or update RDS policy
RDS_POLICY_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/ACKRDSControllerPolicy"
if aws iam get-policy --policy-arn "$RDS_POLICY_ARN" &> /dev/null; then
    log_warn "RDS controller policy already exists"
else
    aws iam create-policy \
        --policy-name ACKRDSControllerPolicy \
        --policy-document "$RDS_POLICY" \
        --description "Policy for ACK RDS controller"
    log_info "RDS controller policy created"
fi

# Attach policy to role
aws iam attach-role-policy \
    --role-name ack-rds-controller-role \
    --policy-arn "$RDS_POLICY_ARN" 2>/dev/null || log_warn "Policy already attached"

log_info "RDS controller role ready: arn:aws:iam::${AWS_ACCOUNT_ID}:role/ack-rds-controller-role"
echo ""

# Create ElastiCache Controller Role
log_info "Creating ElastiCache controller IAM role..."

ELASTICACHE_TRUST_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "${OIDC_PROVIDER}:sub": "system:serviceaccount:${NAMESPACE}:ack-elasticache-controller",
          "${OIDC_PROVIDER}:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
EOF
)

ELASTICACHE_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "elasticache:CreateCacheCluster",
        "elasticache:DeleteCacheCluster",
        "elasticache:DescribeCacheClusters",
        "elasticache:ModifyCacheCluster",
        "elasticache:AddTagsToResource",
        "elasticache:RemoveTagsFromResource",
        "elasticache:ListTagsForResource",
        "elasticache:CreateCacheSubnetGroup",
        "elasticache:DeleteCacheSubnetGroup",
        "elasticache:DescribeCacheSubnetGroups",
        "elasticache:ModifyCacheSubnetGroup",
        "elasticache:CreateCacheParameterGroup",
        "elasticache:DeleteCacheParameterGroup",
        "elasticache:DescribeCacheParameterGroups",
        "elasticache:ModifyCacheParameterGroup",
        "elasticache:DescribeCacheParameters",
        "elasticache:ResetCacheParameterGroup",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSubnets",
        "ec2:DescribeVpcs",
        "ec2:DescribeAvailabilityZones"
      ],
      "Resource": "*"
    }
  ]
}
EOF
)

# Create or update ElastiCache role
if aws iam get-role --role-name ack-elasticache-controller-role &> /dev/null; then
    log_warn "ElastiCache controller role already exists, updating trust policy..."
    aws iam update-assume-role-policy \
        --role-name ack-elasticache-controller-role \
        --policy-document "$ELASTICACHE_TRUST_POLICY"
else
    aws iam create-role \
        --role-name ack-elasticache-controller-role \
        --assume-role-policy-document "$ELASTICACHE_TRUST_POLICY" \
        --description "IAM role for ACK ElastiCache controller" \
        --tags Key=ManagedBy,Value=Script Key=Service,Value=elasticache-controller
    log_info "ElastiCache controller role created"
fi

# Create or update ElastiCache policy
ELASTICACHE_POLICY_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/ACKElastiCacheControllerPolicy"
if aws iam get-policy --policy-arn "$ELASTICACHE_POLICY_ARN" &> /dev/null; then
    log_warn "ElastiCache controller policy already exists"
else
    aws iam create-policy \
        --policy-name ACKElastiCacheControllerPolicy \
        --policy-document "$ELASTICACHE_POLICY" \
        --description "Policy for ACK ElastiCache controller"
    log_info "ElastiCache controller policy created"
fi

# Attach policy to role
aws iam attach-role-policy \
    --role-name ack-elasticache-controller-role \
    --policy-arn "$ELASTICACHE_POLICY_ARN" 2>/dev/null || log_warn "Policy already attached"

log_info "ElastiCache controller role ready: arn:aws:iam::${AWS_ACCOUNT_ID}:role/ack-elasticache-controller-role"
echo ""

log_info "========================================="
log_info "ACK Controller Roles Created!"
log_info "========================================="
echo ""
log_info "Next steps:"
echo "  1. Install ACK RDS controller via ArgoCD"
echo "  2. Install ACK ElastiCache controller via ArgoCD"
echo "  3. Create RDS and ElastiCache resources via CRDs"
