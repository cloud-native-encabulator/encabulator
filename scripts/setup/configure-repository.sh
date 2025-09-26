#!/bin/bash

# Configure Repository URLs for Cloud Native Encabulator
# This script helps set up the correct Git repository URLs in ArgoCD configurations

set -e

echo "🔧 Configuring Repository URLs for Cloud Native Encabulator..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2${NC}"
    else
        echo -e "${RED}❌ $2${NC}"
        return 1
    fi
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

# Get repository URL from user
echo ""
print_info "Current placeholder: REPLACE_WITH_YOUR_ORG"
echo ""
read -p "Enter your GitHub organization/username: " GITHUB_ORG

if [ -z "$GITHUB_ORG" ]; then
    echo -e "${RED}❌ GitHub organization cannot be empty${NC}"
    exit 1
fi

REPO_URL="https://github.com/${GITHUB_ORG}/cloud-native-encabulator-microservices.git"

echo ""
print_info "Will configure repository URL as: $REPO_URL"
echo ""
read -p "Is this correct? (y/N): " CONFIRM

if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo "Configuration cancelled."
    exit 0
fi

# Files to update
files=(
    "k8s/platform/argocd/argocd-config.yaml"
    "k8s/platform/argocd/argocd-applications.yaml"
    "argocd/infrastructure/aws-resources.yaml"
)

echo ""
print_info "Updating repository URLs in configuration files..."

for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        if sed -i.bak "s|REPLACE_WITH_YOUR_ORG|${GITHUB_ORG}|g" "$file"; then
            print_status 0 "Updated $file"
            rm "${file}.bak" 2>/dev/null || true
        else
            print_status 1 "Failed to update $file"
        fi
    else
        print_warning "$file not found"
    fi
done

echo ""
print_info "Repository configuration complete!"

echo ""
echo "📋 Updated Files:"
for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        echo "✅ $file"
    fi
done

echo ""
echo "🔍 Verification:"
echo "Repository URL: $REPO_URL"
echo ""
echo "🚀 Next Steps:"
echo "1. Commit and push these changes to your repository"
echo "2. Run: ./scripts/platform/bootstrap-gitops.sh"
echo "3. ArgoCD will automatically sync from your repository"
echo ""
echo "⚠️  Important: Make sure your repository is accessible to ArgoCD"
echo "   • For public repos: No additional configuration needed"
echo "   • For private repos: Configure SSH keys or access tokens in ArgoCD"