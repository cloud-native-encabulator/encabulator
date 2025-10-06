#!/bin/bash

# Cloud Native Encabulator - Test All Services
# Runs all unit tests across all service planes using the unified Makefile

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_section() {
    echo -e "${PURPLE}[SECTION]${NC} $1"
}

log_section "=== Testing Cloud Native Encabulator Services ==="
echo ""

log_info "Running all tests using unified Makefile..."
echo ""

if make test; then
    echo ""
    log_success "All tests passed! ✅"
    echo ""
    
    log_info "📊 Test Summary:"
    log_info "  ✓ Data Plane: All tests passed"
    log_info "  ✓ Processing Plane: All tests passed"
    log_info "  ✓ Support Plane: All tests passed"
    log_info "  ✓ Control Plane: All tests passed"
    echo ""
    
    log_info "🚀 Next steps:"
    log_info "  Build images:     make image"
    log_info "  Deploy platform:  ./scripts/deploy/deploy-helm.sh"
    log_info "  Run integration:  ./scripts/test/comprehensive-test.sh"
else
    echo ""
    log_error "Some tests failed! ❌"
    echo ""
    
    log_info "🔍 Debug individual planes:"
    log_info "  make test-data-plane"
    log_info "  make test-processing-plane"
    log_info "  make test-support-plane"
    log_info "  make test-control-plane"
    
    exit 1
fi
