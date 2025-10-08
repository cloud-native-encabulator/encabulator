#!/bin/bash

# Cloud Native Encabulator - Version Bump Script
# Manages semantic versioning for the entire platform

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
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

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Get current version
VERSION_FILE="VERSION"
if [ ! -f "$VERSION_FILE" ]; then
    log_error "VERSION file not found!"
    exit 1
fi

CURRENT_VERSION=$(cat "$VERSION_FILE")
log_info "Current version: $CURRENT_VERSION"

# Parse version
IFS='.' read -r -a VERSION_PARTS <<< "$CURRENT_VERSION"
MAJOR="${VERSION_PARTS[0]}"
MINOR="${VERSION_PARTS[1]}"
PATCH="${VERSION_PARTS[2]}"

# Determine bump type
BUMP_TYPE="${1:-patch}"

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
        echo "Usage: $0 [major|minor|patch]"
        exit 1
        ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"

log_info "Bumping version: $CURRENT_VERSION -> $NEW_VERSION"
echo ""

# Confirm
read -p "Proceed with version bump? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_warning "Version bump cancelled"
    exit 0
fi

# Update VERSION file
echo "$NEW_VERSION" > "$VERSION_FILE"
log_success "Updated VERSION file to $NEW_VERSION"

# Create git tag (if in git repo)
if git rev-parse --git-dir > /dev/null 2>&1; then
    log_info "Creating git tag: v$NEW_VERSION"
    git add "$VERSION_FILE"
    git commit -m "chore: bump version to $NEW_VERSION"
    git tag -a "v$NEW_VERSION" -m "Release version $NEW_VERSION"
    log_success "Created git tag: v$NEW_VERSION"
    echo ""
    log_info "To push changes and tag:"
    echo "  git push origin main"
    echo "  git push origin v$NEW_VERSION"
else
    log_warning "Not in a git repository, skipping git operations"
fi

echo ""
log_success "Version bumped successfully!"
log_info "New version: $NEW_VERSION"
echo ""
log_info "Next steps:"
echo "  1. Build and publish images: make image-publish IMAGE_TAG=$NEW_VERSION"
echo "  2. Update Helm chart versions"
echo "  3. Deploy to cluster"
