#!/bin/bash

# Update all service Makefiles to include automatic version bumping

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }

log_info "Updating service Makefiles for automatic versioning..."
echo ""

# Find all service Makefiles
MAKEFILES=$(find services -maxdepth 3 -name Makefile -path "*/data-plane/*" -o \
                                                      -path "*/processing-plane/*" -o \
                                                      -path "*/support-plane/*" -o \
                                                      -path "*/control-plane/*" | \
            grep -E "(data-plane|processing-plane|support-plane|control-plane)/[^/]+/Makefile$" | \
            sort)

COUNT=0
for MAKEFILE in $MAKEFILES; do
    SERVICE_DIR=$(dirname "$MAKEFILE")
    SERVICE_NAME=$(basename "$SERVICE_DIR")
    
    # Skip if already has VERSION_FILE variable
    if grep -q "VERSION_FILE :=" "$MAKEFILE"; then
        log_info "$SERVICE_NAME: Already updated"
        continue
    fi
    
    # Backup
    cp "$MAKEFILE" "$MAKEFILE.bak"
    
    # Add version variables after CONTAINER_RUNTIME line
    sed -i '' '/^CONTAINER_RUNTIME ?=/a\
BUMP ?= patch\
\
# Version management\
VERSION_FILE := VERSION\
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")
' "$MAKEFILE"
    
    # Update image-publish target
    # This is complex, so we'll add a note for manual review
    
    log_success "$SERVICE_NAME: Added version variables"
    COUNT=$((COUNT + 1))
done

echo ""
log_success "Updated $COUNT Makefiles"
echo ""
log_info "Next steps:"
echo "  1. Review changes: git diff services/"
echo "  2. Manually update image-publish targets to use version bumping"
echo "  3. Test: make -C services/data-plane/hydrocoptic-datastore image-publish BUMP=patch"
echo ""
log_info "Note: Backup files created as Makefile.bak"

