#!/bin/bash

# Cloud Native Encabulator - Version-based Image Publishing
# Publishes all images with the current version from VERSION file

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

# Get current version
VERSION_FILE="VERSION"
if [ ! -f "$VERSION_FILE" ]; then
    log_warning "VERSION file not found, using 'latest'"
    VERSION="latest"
else
    VERSION=$(cat "$VERSION_FILE")
fi

# Options
TAG_LATEST="${1:-yes}"

log_section "=== Publishing Cloud Native Encabulator Images ==="
echo ""
log_info "Version: $VERSION"
log_info "Tag as latest: $TAG_LATEST"
echo ""

# Publish with version tag
log_info "Publishing images with tag: $VERSION"
make image-publish IMAGE_TAG="$VERSION"

# Also tag as latest if requested
if [ "$TAG_LATEST" = "yes" ]; then
    echo ""
    log_info "Also tagging as 'latest'"
    make image-publish IMAGE_TAG="latest"
fi

echo ""
log_section "=== Publishing Complete ==="
log_success "All images published successfully!"
log_info "Version: $VERSION"
echo ""
log_info "Verify images in ECR:"
echo "  aws ecr list-images --repository-name encabulator/hydrocoptic-datastore --region us-east-2"
echo ""
log_info "Next steps:"
echo "  1. Update Helm chart versions to $VERSION"
echo "  2. Deploy to cluster: ./scripts/deploy/deploy-helm.sh"
