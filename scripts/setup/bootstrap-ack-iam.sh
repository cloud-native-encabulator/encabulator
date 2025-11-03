#!/bin/bash
set -euo pipefail

# Bootstrap ACK IAM Controller
# This script creates the admin IAM role needed for the ACK IAM controller
# to manage other IAM roles via Kubernetes CRDs.
#
# This is a ONE-TIME manual step. After this, all IAM roles are managed via GitOps.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CLUSTER_NAME="${CLUSTER_NAME:-}"
AWS_REGION="${AWS_REGION:-us-east-2}"
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-}"
NAMESPACE="ack-system"
SERVICE_ACCOUNT="ack-iam-controller"
ROLE_NAME="ack-iam-controller-admin-role"

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check AWS CLI
    if ! command -v aws &> /dev/null; then
        log_error "AWS CLI not found. Please install it first."
        exit 1
    fi
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install it first."
        exit 1
    fi
    
    # Check cluster access
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot access Kubernetes cluster. Please configure kubectl."
        exit 1
    fi
    
    log_info "Prerequisites check passed"
}

get_cluster_info() {
    log_info "Getting cluster information..."
    
    # Get cluster name from current context if not provided
    if [ -z "$CLUSTER_NAME" ]; then
        # Extract cluster name from eksctl context format: user@cluster.region.eksctl.io
        CLUSTER_NAME=$(kubectl config current-context | sed 's/.*@\([^.]*\).*/\1/')
        log_info "Detected cluster name: $CLUSTER_NAME"
    fi
    
    # Get AWS account ID if not provided
    if [ -z "$AWS_ACCOUNT_ID" ]; then
        AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
        log_info "Detected AWS account ID: $AWS_ACCOUNT_ID"
    fi
    
    # Verify cluster exists
    if ! aws eks describe-cluster --name "$CLUSTER_NAME" --region "$AWS_REGION" &> /dev/null; then
        log_error "EKS cluster '$CLUSTER_NAME' not found in region '$AWS_REGION'"
        exit 1
    fi
    
    log_info "Cluster info retrieved successfully"
}

create_namespace() {
    log_info "Creating namespace '$NAMESPACE'..."
    
    if kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_warn "Namespace '$NAMESPACE' already exists"
    else
        kubectl create namespace "$NAMESPACE"
        log_info "Namespace created"
    fi
}

create_iam_policy() {
    log_info "Creating IAM policy for ACK IAM controller..."
    
    local policy_name="ACKIAMControllerPolicy"
    local policy_arn="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/${policy_name}"
    
    # Check if policy already exists
    if aws iam get-policy --policy-arn "$policy_arn" &> /dev/null; then
        log_warn "IAM policy '$policy_name' already exists"
        echo "$policy_arn"
        return 0
    fi
    
    # Create policy document
    local policy_doc=$(cat <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "iam:CreateRole",
                "iam:DeleteRole",
                "iam:GetRole",
                "iam:UpdateRole",
                "iam:ListRoles",
                "iam:TagRole",
                "iam:UntagRole",
                "iam:AttachRolePolicy",
                "iam:DetachRolePolicy",
                "iam:ListAttachedRolePolicies",
                "iam:ListRolePolicies",
                "iam:PutRolePolicy",
                "iam:GetRolePolicy",
                "iam:DeleteRolePolicy",
                "iam:CreatePolicy",
                "iam:DeletePolicy",
                "iam:GetPolicy",
                "iam:GetPolicyVersion",
                "iam:ListPolicyVersions",
                "iam:CreatePolicyVersion",
                "iam:DeletePolicyVersion",
                "iam:SetDefaultPolicyVersion",
                "iam:TagPolicy",
                "iam:UntagPolicy"
            ],
            "Resource": "*"
        }
    ]
}
EOF
)
    
    # Create policy
    policy_arn=$(aws iam create-policy \
        --policy-name "$policy_name" \
        --policy-document "$policy_doc" \
        --description "Policy for ACK IAM controller to manage IAM resources" \
        --query 'Policy.Arn' \
        --output text)
    
    log_info "IAM policy created: $policy_arn"
    echo "$policy_arn"
}

create_iam_role() {
    local policy_arn=$1
    
    log_info "Creating IAM role '$ROLE_NAME'..."
    
    # Check if role already exists
    if aws iam get-role --role-name "$ROLE_NAME" &> /dev/null; then
        log_warn "IAM role '$ROLE_NAME' already exists"
        local role_arn=$(aws iam get-role --role-name "$ROLE_NAME" --query 'Role.Arn' --output text)
        echo "$role_arn"
        return 0
    fi
    
    # Get OIDC provider for the cluster
    local oidc_provider=$(aws eks describe-cluster \
        --name "$CLUSTER_NAME" \
        --region "$AWS_REGION" \
        --query "cluster.identity.oidc.issuer" \
        --output text | sed 's|https://||')
    
    if [ -z "$oidc_provider" ]; then
        log_error "Failed to get OIDC provider for cluster"
        exit 1
    fi
    
    log_info "OIDC provider: $oidc_provider"
    
    # Create trust policy
    local trust_policy=$(cat <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${oidc_provider}"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "${oidc_provider}:sub": "system:serviceaccount:${NAMESPACE}:${SERVICE_ACCOUNT}",
                    "${oidc_provider}:aud": "sts.amazonaws.com"
                }
            }
        }
    ]
}
EOF
)
    
    # Create role
    local role_arn=$(aws iam create-role \
        --role-name "$ROLE_NAME" \
        --assume-role-policy-document "$trust_policy" \
        --description "IAM role for ACK IAM controller" \
        --query 'Role.Arn' \
        --output text)
    
    log_info "IAM role created: $role_arn"
    
    # Attach policy to role
    aws iam attach-role-policy \
        --role-name "$ROLE_NAME" \
        --policy-arn "$policy_arn"
    
    log_info "Policy attached to role"
    
    echo "$role_arn"
}

create_service_account() {
    local role_arn=$1
    
    log_info "Creating service account '$SERVICE_ACCOUNT'..."
    
    # Check if service account already exists
    if kubectl get serviceaccount "$SERVICE_ACCOUNT" -n "$NAMESPACE" &> /dev/null; then
        log_warn "Service account '$SERVICE_ACCOUNT' already exists"
        
        # Update annotation
        kubectl annotate serviceaccount "$SERVICE_ACCOUNT" \
            -n "$NAMESPACE" \
            "eks.amazonaws.com/role-arn=$role_arn" \
            --overwrite
        
        log_info "Service account annotation updated"
        return 0
    fi
    
    # Create service account with IAM role annotation
    kubectl create serviceaccount "$SERVICE_ACCOUNT" -n "$NAMESPACE"
    kubectl annotate serviceaccount "$SERVICE_ACCOUNT" \
        -n "$NAMESPACE" \
        "eks.amazonaws.com/role-arn=$role_arn"
    
    log_info "Service account created with IAM role annotation"
}

print_summary() {
    local role_arn=$1
    
    echo ""
    log_info "========================================="
    log_info "ACK IAM Controller Bootstrap Complete!"
    log_info "========================================="
    echo ""
    log_info "Configuration:"
    echo "  Cluster:         $CLUSTER_NAME"
    echo "  Region:          $AWS_REGION"
    echo "  Namespace:       $NAMESPACE"
    echo "  Service Account: $SERVICE_ACCOUNT"
    echo "  IAM Role:        $ROLE_NAME"
    echo "  Role ARN:        $role_arn"
    echo ""
    log_info "Next steps:"
    echo "  1. Install ACK IAM controller via ArgoCD"
    echo "  2. Create IAM roles for RDS and ElastiCache controllers via CRDs"
    echo "  3. Install RDS and ElastiCache controllers"
    echo ""
    log_info "See docs/ACK_SETUP.md for detailed instructions"
}

# Main execution
main() {
    log_info "Starting ACK IAM Controller bootstrap..."
    echo ""
    
    check_prerequisites
    get_cluster_info
    create_namespace
    
    local policy_arn=$(create_iam_policy)
    local role_arn=$(create_iam_role "$policy_arn")
    create_service_account "$role_arn"
    
    print_summary "$role_arn"
}

# Run main function
main "$@"
