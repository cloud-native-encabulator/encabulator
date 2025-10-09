#!/bin/bash

# Cloud Native Encabulator - Initialize Service Versions
# Creates VERSION files for all services

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }

INITIAL_VERSION="0.0.1"

log_info "Initializing service versions to $INITIAL_VERSION"
echo ""

# Define service directories explicitly
PLANES=(
    "data-plane"
    "processing-plane"
    "support-plane"
    "control-plane"
)

COUNT=0
for PLANE in "${PLANES[@]}"; do
    PLANE_DIR="services/$PLANE"
    if [ ! -d "$PLANE_DIR" ]; then
        continue
    fi
    
    # Find service directories (direct children of plane directory)
    for SERVICE_DIR in "$PLANE_DIR"/*; do
        if [ ! -d "$SERVICE_DIR" ]; then
            continue
        fi
        
        SERVICE_NAME=$(basename "$SERVICE_DIR")
        VERSION_FILE="$SERVICE_DIR/VERSION"
        
        if [ -f "$VERSION_FILE" ]; then
            CURRENT=$(cat "$VERSION_FILE")
            log_info "$PLANE/$SERVICE_NAME: Already has version $CURRENT"
        else
            echo "$INITIAL_VERSION" > "$VERSION_FILE"
            log_success "$PLANE/$SERVICE_NAME: Created VERSION file with $INITIAL_VERSION"
            COUNT=$((COUNT + 1))
        fi
    done
done

echo ""
log_success "Initialized $COUNT service VERSION files"
echo ""
log_info "Next steps:"
echo "  1. Review VERSION files: find services -name VERSION -exec cat {} \;"
echo "  2. Commit changes: git add services/*/VERSION && git commit -m 'chore: initialize service versions'"
echo ""
log_info "Note: Platform version remains at 0.0.1 until initial implementation is complete"
log_info "      Major version 0 indicates pre-1.0 development phase"

