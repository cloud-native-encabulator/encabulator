#!/bin/bash

# Helper script to bump service version and return the new version
# Used by Makefiles during image-publish

set -e

SERVICE_DIR="${1:-.}"
BUMP_TYPE="${2:-patch}"

VERSION_FILE="$SERVICE_DIR/VERSION"

# Create VERSION file if it doesn't exist
if [ ! -f "$VERSION_FILE" ]; then
    echo "0.0.1" > "$VERSION_FILE"
fi

# Get current version
CURRENT_VERSION=$(cat "$VERSION_FILE")

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
        echo "Error: Invalid bump type: $BUMP_TYPE" >&2
        exit 1
        ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"

# Update VERSION file
echo "$NEW_VERSION" > "$VERSION_FILE"

# Output new version (for Makefile to capture)
echo "$NEW_VERSION"
