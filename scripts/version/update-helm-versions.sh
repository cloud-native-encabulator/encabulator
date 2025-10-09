#!/bin/bash

# Update Helm chart versions with platform and service versions

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }

# Get platform version
PLATFORM_VERSION=$(cat VERSION 2>/dev/null || echo "0.0.1")
log_info "Platform version: $PLATFORM_VERSION"
echo ""

# Find all Helm charts
CHARTS=$(find helm/charts -maxdepth 1 -mindepth 1 -type d | sort)

COUNT=0
for CHART_DIR in $CHARTS; do
    CHART_NAME=$(basename "$CHART_DIR")
    CHART_FILE="$CHART_DIR/Chart.yaml"
    
    if [ ! -f "$CHART_FILE" ]; then
        continue
    fi
    
    # Find corresponding service
    SERVICE_DIR=$(find services -type d -name "$CHART_NAME" | head -1)
    
    if [ -z "$SERVICE_DIR" ]; then
        log_info "$CHART_NAME: No matching service found, skipping"
        continue
    fi
    
    SERVICE_VERSION=$(cat "$SERVICE_DIR/VERSION" 2>/dev/null || echo "0.0.1")
    
    log_info "$CHART_NAME: Updating chart version to $PLATFORM_VERSION, appVersion to $SERVICE_VERSION"
    
    # Update Chart.yaml
    # Update version (platform version)
    sed -i.bak "s/^version:.*/version: $PLATFORM_VERSION/" "$CHART_FILE"
    
    # Update appVersion (service version)
    if grep -q "^appVersion:" "$CHART_FILE"; then
        sed -i.bak "s/^appVersion:.*/appVersion: \"$SERVICE_VERSION\"/" "$CHART_FILE"
    else
        # Add appVersion if it doesn't exist
        echo "appVersion: \"$SERVICE_VERSION\"" >> "$CHART_FILE"
    fi
    
    # Remove backup file
    rm -f "${CHART_FILE}.bak"
    
    log_success "  Updated $CHART_NAME (chart: $PLATFORM_VERSION, app: $SERVICE_VERSION)"
    COUNT=$((COUNT + 1))
done

echo ""
log_success "Updated $COUNT Helm charts"

echo ""
log_info "Next steps:"
echo "  1. Review changes: git diff helm/charts/*/Chart.yaml"
echo "  2. Commit: git add helm/charts/*/Chart.yaml"
echo "  3. Include in release commit"

