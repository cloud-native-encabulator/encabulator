#!/bin/bash

# Cloud Native Encabulator - Build All Services
# Builds all services across all planes using the unified Makefile

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_section() {
    echo -e "${PURPLE}[SECTION]${NC} $1"
}

log_section "=== Building Cloud Native Encabulator Services ==="
echo ""

log_info "Building all services using unified Makefile..."
make build

echo ""
log_success "All services built successfully!"
echo ""

log_info "📊 Build Summary:"
log_info "  ✓ Data Plane: 3 services (Go)"
log_info "  ✓ Processing Plane: 3 services (2 Go, 1 TypeScript)"
log_info "  ✓ Support Plane: 3 services (1 Go, 2 Python)"
log_info "  ✓ Control Plane: 4 services (1 Go, 2 TypeScript, 1 Python)"
echo ""

log_info "🚀 Next steps:"
log_info "  Run tests:        make test"
log_info "  Build images:     make image"
log_info "  Deploy platform:  ./scripts/deploy/deploy-helm.sh"
