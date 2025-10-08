#!/bin/bash

# Cloud Native Encabulator - Update Helm Chart Image Repositories
# Updates all Helm chart values.yaml files to use ECR images

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Configuration
ECR_REGISTRY="${ECR_REGISTRY:-510431938379.dkr.ecr.us-east-2.amazonaws.com}"
ECR_NAMESPACE="${ECR_NAMESPACE:-encabulator}"
IMAGE_TAG="${IMAGE_TAG:-0.0.1}"

log_info "Updating Helm charts to use ECR images"
log_info "Registry: $ECR_REGISTRY"
log_info "Namespace: $ECR_NAMESPACE"
log_info "Tag: $IMAGE_TAG"
echo ""

# All services
SERVICES=(
    "hydrocoptic-datastore"
    "prefabulated-cache"
    "sinusoidal-database"
    "panametric-fam-processor"
    "logarithmic-casing-service"
    "dingle-arm-api-gateway"
    "unilateral-phase-detractor"
    "differential-girdle"
    "side-fumbling-collector"
    "encabulation-control-console"
    "malaise-injection-engine"
    "cardinal-grammeter"
    "demo-scenario-orchestrator"
)

for service in "${SERVICES[@]}"; do
    VALUES_FILE="helm/charts/$service/values.yaml"
    
    if [ ! -f "$VALUES_FILE" ]; then
        log_warning "Skipping $service - values.yaml not found"
        continue
    fi
    
    log_info "Updating $service..."
    
    # Update repository and tag using sed
    # This preserves the YAML structure
    sed -i.bak \
        -e "s|repository:.*|repository: $ECR_REGISTRY/$ECR_NAMESPACE/$service|" \
        -e "s|tag:.*|tag: \"$IMAGE_TAG\"|" \
        "$VALUES_FILE"
    
    # Remove backup file
    rm -f "${VALUES_FILE}.bak"
    
    log_success "  ✓ Updated $service"
done

echo ""
log_success "All Helm charts updated!"
echo ""
log_info "Verify changes:"
echo "  git diff helm/charts/*/values.yaml"
echo ""
log_info "Next steps:"
echo "  1. Review changes: git diff"
echo "  2. Deploy services: ./scripts/deploy/deploy-helm.sh --data-plane-services"
