#!/bin/bash

# Pre-installation script for ACK
# This script should be run before deploying ACK controllers via ArgoCD
# It installs the required CRDs that controllers need

set -e

echo "🔧 Pre-installing ACK Custom Resource Definitions..."

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed or not in PATH"
    exit 1
fi

# Check if we can connect to the cluster
if ! kubectl cluster-info &> /dev/null; then
    echo "❌ Cannot connect to Kubernetes cluster"
    exit 1
fi

print_info "Installing essential ACK CRDs..."

# Install only the essential CRDs needed for our resources
essential_crds=(
    "https://raw.githubusercontent.com/aws-controllers-k8s/rds-controller/main/config/crd/bases/rds.services.k8s.aws_dbinstances.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/elasticache-controller/main/config/crd/bases/elasticache.services.k8s.aws_replicationgroups.yaml"
    "https://raw.githubusercontent.com/aws-controllers-k8s/iam-controller/main/config/crd/bases/iam.services.k8s.aws_roles.yaml"
)

for crd_url in "${essential_crds[@]}"; do
    crd_name=$(basename "$crd_url" .yaml | cut -d'_' -f2-)
    if kubectl apply -f "$crd_url" > /dev/null 2>&1; then
        print_status "Installed $crd_name CRD"
    else
        print_status "$crd_name CRD already exists or failed to install"
    fi
done

print_info "CRD pre-installation complete!"
echo ""
echo "✅ Ready for ACK controller deployment via ArgoCD"
echo "🚀 Next: Deploy controllers using ArgoCD or kubectl apply -k k8s/infrastructure/ack-controllers/"