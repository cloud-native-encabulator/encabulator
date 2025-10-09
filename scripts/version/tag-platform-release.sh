#!/bin/bash

# Tag all service images with platform version

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }

# Configuration
ECR_REGISTRY="${ECR_REGISTRY:-510431938379.dkr.ecr.us-east-2.amazonaws.com}"
ECR_NAMESPACE="${ECR_NAMESPACE:-encabulator}"
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-finch}"

# Get platform version
PLATFORM_VERSION=$(cat VERSION 2>/dev/null || echo "0.0.1")
log_info "Platform version: $PLATFORM_VERSION"
echo ""

# Find all services
PLANES=(
    "data-plane"
    "processing-plane"
    "support-plane"
    "control-plane"
)

COUNT=0
FAILED=0

for PLANE in "${PLANES[@]}"; do
    PLANE_DIR="services/$PLANE"
    if [ ! -d "$PLANE_DIR" ]; then
        continue
    fi
    
    for SERVICE_DIR in "$PLANE_DIR"/*; do
        if [ ! -d "$SERVICE_DIR" ]; then
            continue
        fi
        
        SERVICE_NAME=$(basename "$SERVICE_DIR")
        SERVICE_VERSION=$(cat "$SERVICE_DIR/VERSION" 2>/dev/null || echo "0.0.1")
        
        log_info "Tagging $SERVICE_NAME (service: $SERVICE_VERSION, platform: $PLATFORM_VERSION)"
        
        # Pull service version image
        if ! $CONTAINER_RUNTIME pull "$ECR_REGISTRY/$ECR_NAMESPACE/$SERVICE_NAME:$SERVICE_VERSION" 2>/dev/null; then
            log_warning "  Could not pull $SERVICE_NAME:$SERVICE_VERSION (may not be published yet)"
            FAILED=$((FAILED + 1))
            continue
        fi
        
        # Tag with platform version
        $CONTAINER_RUNTIME tag \
            "$ECR_REGISTRY/$ECR_NAMESPACE/$SERVICE_NAME:$SERVICE_VERSION" \
            "$ECR_REGISTRY/$ECR_NAMESPACE/$SERVICE_NAME:$PLATFORM_VERSION"
        
        # Push platform version tag
        $CONTAINER_RUNTIME push "$ECR_REGISTRY/$ECR_NAMESPACE/$SERVICE_NAME:$PLATFORM_VERSION"
        
        log_success "  Tagged and pushed $SERVICE_NAME:$PLATFORM_VERSION"
        COUNT=$((COUNT + 1))
    done
done

echo ""
log_success "Tagged $COUNT services with platform version $PLATFORM_VERSION"
if [ $FAILED -gt 0 ]; then
    log_warning "Failed to tag $FAILED services (not published yet)"
fi

echo ""
log_info "Next steps:"
echo "  1. Update Helm charts: ./scripts/version/update-helm-versions.sh"
echo "  2. Commit and tag: git add -A && git commit -m 'chore: release v$PLATFORM_VERSION'"
echo "  3. Create git tag: git tag -a v$PLATFORM_VERSION -m 'Platform release $PLATFORM_VERSION'"
echo "  4. Push: git push origin main --tags"

