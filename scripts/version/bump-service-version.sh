#!/bin/bash

# Cloud Native Encabulator - Service Version Bump Script
# Bumps version for a specific service

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check arguments
if [ $# -lt 1 ]; then
    log_error "Usage: $0 <service-path> [major|minor|patch]"
    echo "Example: $0 services/data-plane/hydrocoptic-datastore patch"
    exit 1
fi

SERVICE_PATH="$1"
BUMP_TYPE="${2:-patch}"

# Validate service path
if [ ! -d "$SERVICE_PATH" ]; then
    log_error "Service directory not found: $SERVICE_PATH"
    exit 1
fi

SERVICE_NAME=$(basename "$SERVICE_PATH")
VERSION_FILE="$SERVICE_PATH/VERSION"

# Create VERSION file if it doesn't exist
if [ ! -f "$VERSION_FILE" ]; then
    log_warning "VERSION file not found, creating with version 0.1.0"
    echo "0.1.0" > "$VERSION_FILE"
fi

# Get current version
CURRENT_VERSION=$(cat "$VERSION_FILE")
log_info "Service: $SERVICE_NAME"
log_info "Current version: $CURRENT_VERSION"

# Parse version
IFS='.' read -r -a VERSION_PARTS <<< "$CURRENT_VERSION"
MAJOR="${VERSION_PARTS[0]}"
MINOR="${VERSION_PARTS[1]}"
PATCH="${VERSION_PARTS[2]}"

# Bump version
case "$BUMP_TYPE" in
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        ;;
    patch)
        PATCH=$((PATCH + 1))
        ;;
    *)
        log_error "Invalid bump type: $BUMP_TYPE"
        echo "Valid types: major, minor, patch"
        exit 1
        ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"

log_info "New version: $NEW_VERSION"
echo ""

# Update VERSION file
echo "$NEW_VERSION" > "$VERSION_FILE"
log_success "Updated $VERSION_FILE to $NEW_VERSION"

# Update Helm chart appVersion if exists
HELM_CHART="helm/charts/$SERVICE_NAME/Chart.yaml"
if [ -f "$HELM_CHART" ]; then
    # Update appVersion in Chart.yaml
    if grep -q "^appVersion:" "$HELM_CHART"; then
        sed -i.bak "s/^appVersion:.*/appVersion: \"$NEW_VERSION\"/" "$HELM_CHART"
        rm "${HELM_CHART}.bak"
        log_success "Updated Helm chart appVersion to $NEW_VERSION"
    fi
fi

echo ""
log_success "Service version bumped successfully!"
log_info "Service: $SERVICE_NAME"
log_info "Version: $CURRENT_VERSION -> $NEW_VERSION"
echo ""
log_info "Next steps:"
echo "  1. Build and test: make -C $SERVICE_PATH build test"
echo "  2. Publish image: make -C $SERVICE_PATH image-publish IMAGE_TAG=$NEW_VERSION"
echo "  3. Bump platform version: ./scripts/version/bump-platform-version.sh $BUMP_TYPE"

