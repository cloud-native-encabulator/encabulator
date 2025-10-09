#!/bin/bash

# Bump platform version

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }

VERSION_FILE="VERSION"
BUMP_TYPE="${1:-patch}"

if [ ! -f "$VERSION_FILE" ]; then
    echo "0.0.1" > "$VERSION_FILE"
fi

CURRENT_VERSION=$(cat "$VERSION_FILE")
log_info "Current platform version: $CURRENT_VERSION"

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
        echo "Error: Invalid bump type: $BUMP_TYPE"
        echo "Usage: $0 [major|minor|patch]"
        exit 1
        ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"

log_info "Bumping platform version (${BUMP_TYPE}): $CURRENT_VERSION -> $NEW_VERSION"
echo ""

# Update VERSION file
echo "$NEW_VERSION" > "$VERSION_FILE"
log_success "Updated platform VERSION to $NEW_VERSION"

echo ""
log_info "Next steps:"
echo "  1. Tag service images: ./scripts/version/tag-platform-release.sh"
echo "  2. Update Helm charts: ./scripts/version/update-helm-versions.sh"
echo "  3. Commit and tag: git add VERSION && git commit -m 'chore: release v$NEW_VERSION'"
echo "  4. Push: git push origin main --tags"

