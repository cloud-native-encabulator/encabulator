#!/bin/bash

# Install ACK Custom Resource Definitions
# This script installs the required CRDs for ACK controllers

set -e

echo "🔧 Installing AWS Controllers for Kubernetes (ACK) Custom Resource Definitions..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2${NC}"
    else
        echo -e "${RED}❌ $2${NC}"
        return 1
    fi
}

print_info() {
    echo -e "ℹ️  $1"
}

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}❌ kubectl is not installed or not in PATH${NC}"
    exit 1
fi

# Check if we can connect to the cluster
if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}❌ Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi

print_status 0 "Connected to Kubernetes cluster"

# Install RDS CRDs
echo ""
print_info "Installing RDS Controller CRDs..."

rds_crds=(
    "https://raw.githubusercontent.com/aws-controllers-k8s/rds-controller/main/config/crd/bases/rds.services.k8s.aws_dbinstances.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/rds-controller/main/config/crd/bases/rds.services.k8s.aws_dbsubnetgroups.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/rds-controller/main/config/crd/bases/rds.services.k8s.aws_dbclusters.yaml"
)

for crd_url in "${rds_crds[@]}"; do
    crd_name=$(basename "$crd_url" .yaml)
    if kubectl apply -f "$crd_url" > /dev/null 2>&1; then
        print_status 0 "Installed $crd_name"
    else
        echo -e "${YELLOW}⚠️  Failed to install $crd_name (may already exist)${NC}"
    fi
done

# Install ElastiCache CRDs
echo ""
print_info "Installing ElastiCache Controller CRDs..."

elasticache_crds=(
    "https://raw.githubusercontent.com/aws-controllers-k8s/elasticache-controller/main/config/crd/bases/elasticache.services.k8s.aws_replicationgroups.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/elasticache-controller/main/config/crd/bases/elasticache.services.k8s.aws_cacheclusters.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/elasticache-controller/main/config/crd/bases/elasticache.services.k8s.aws_cachesubnetgroups.yaml"
)

for crd_url in "${elasticache_crds[@]}"; do
    crd_name=$(basename "$crd_url" .yaml)
    if kubectl apply -f "$crd_url" > /dev/null 2>&1; then
        print_status 0 "Installed $crd_name"
    else
        echo -e "${YELLOW}⚠️  Failed to install $crd_name (may already exist)${NC}"
    fi
done

# Install IAM CRDs
echo ""
print_info "Installing IAM Controller CRDs..."

iam_crds=(
    "https://raw.githubusercontent.com/aws-controllers-k8s/iam-controller/main/config/crd/bases/iam.services.k8s.aws_roles.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/iam-controller/main/config/crd/bases/iam.services.k8s.aws_policies.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/iam-controller/main/config/crd/bases/iam.services.k8s.aws_users.yaml"
)

for crd_url in "${iam_crds[@]}"; do
    crd_name=$(basename "$crd_url" .yaml)
    if kubectl apply -f "$crd_url" > /dev/null 2>&1; then
        print_status 0 "Installed $crd_name"
    else
        echo -e "${YELLOW}⚠️  Failed to install $crd_name (may already exist)${NC}"
    fi
done

echo ""
print_info "CRD installation complete!"

# Verify CRDs are available
echo ""
print_info "Verifying CRD installation..."

required_crds=(
    "dbinstances.rds.services.k8s.aws"
    "replicationgroups.elasticache.services.k8s.aws"
    "roles.iam.services.k8s.aws"
)

for crd in "${required_crds[@]}"; do
    if kubectl get crd "$crd" > /dev/null 2>&1; then
        print_status 0 "$crd is available"
    else
        print_status 1 "$crd is not available"
    fi
done

echo ""
echo "📋 Next Steps:"
echo "1. Install ACK controllers using: kubectl apply -k k8s/infrastructure/ack-controllers/"
echo "2. Wait for controllers to be ready"
echo "3. Deploy AWS resources using: kubectl apply -k k8s/infrastructure/aws-resources/"
echo "4. Run validation: ./scripts/infrastructure/validate-ack-setup.sh"