#!/bin/bash

# Cloud Native Encabulator - Build All Container Images
# Builds container images for all services using the unified Makefile

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_section() {
    echo -e "${PURPLE}[SECTION]${NC} $1"
}

# Check container runtime
check_container_runtime() {
    if command -v finch >/dev/null 2>&1; then
        log_info "Using Finch as container runtime"
        export CONTAINER_RUNTIME=finch
    elif command -v docker >/dev/null 2>&1; then
        log_info "Using Docker as container runtime"
        export CONTAINER_RUNTIME=docker
    else
        log_warning "No container runtime found (docker or finch)"
        log_warning "Please install Docker or Finch to build images"
        exit 1
    fi
}

log_section "=== Building Cloud Native Encabulator Container Images ==="
echo ""

check_container_runtime
echo ""

log_info "Building all container images using unified Makefile..."
make image

echo ""
log_success "All container images built successfully!"
echo ""

log_info "📦 Image Summary:"
log_info "  ✓ Data Plane: 3 images"
log_info "  ✓ Processing Plane: 3 images"
log_info "  ✓ Support Plane: 3 images"
log_info "  ✓ Control Plane: 4 images"
echo ""

log_info "🔍 View images:"
log_info "  ${CONTAINER_RUNTIME} images | grep encabulator"
echo ""

log_info "🚀 Next steps:"
log_info "  Push to registry: ${CONTAINER_RUNTIME} push <image>"
log_info "  Deploy platform:  ./scripts/deploy/deploy-helm.sh"
