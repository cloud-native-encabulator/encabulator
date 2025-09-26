#!/bin/bash

# Bootstrap GitOps for Cloud Native Encabulator
# This script sets up the complete GitOps workflow with ArgoCD

set -e

echo "🎯 Bootstrapping GitOps for Cloud Native Encabulator..."

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

# Step 1: Install ArgoCD
echo ""
print_info "Step 1: Installing ArgoCD..."

if [ -f "./scripts/platform/install-argocd.sh" ]; then
    ./scripts/platform/install-argocd.sh
    print_status 0 "ArgoCD installation completed"
else
    print_status 1 "ArgoCD installation script not found"
    exit 1
fi

# Step 2: Wait for ArgoCD to be fully ready
echo ""
print_info "Step 2: Ensuring ArgoCD is fully operational..."

print_info "Waiting for all ArgoCD components..."
kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd
kubectl wait --for=condition=available --timeout=300s deployment/argocd-application-controller -n argocd
kubectl wait --for=condition=available --timeout=300s deployment/argocd-repo-server -n argocd

print_status 0 "All ArgoCD components are ready"

# Step 3: Deploy root applications
echo ""
print_info "Step 3: Deploying root ArgoCD applications..."

if kubectl apply -f k8s/platform/argocd/argocd-applications.yaml > /dev/null 2>&1; then
    print_status 0 "Root applications deployed"
else
    print_status 1 "Failed to deploy root applications"
fi

# Step 4: Verify applications are syncing
echo ""
print_info "Step 4: Verifying application deployment..."

sleep 10

# Check if applications are created
apps=$(kubectl get applications -n argocd --no-headers 2>/dev/null | wc -l)
if [ "$apps" -gt 0 ]; then
    print_status 0 "ArgoCD applications created ($apps applications)"
else
    print_warning "No ArgoCD applications found yet"
fi

# Step 5: Display status
echo ""
print_info "Step 5: Displaying system status..."

echo ""
echo "📊 ArgoCD Applications:"
kubectl get applications -n argocd 2>/dev/null || echo "No applications found yet"

echo ""
echo "📊 ArgoCD Projects:"
kubectl get appprojects -n argocd 2>/dev/null || echo "No projects found yet"

# Step 6: Get access information
echo ""
print_info "Step 6: Retrieving access information..."

if kubectl get secret argocd-initial-admin-secret -n argocd > /dev/null 2>&1; then
    ADMIN_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
else
    ADMIN_PASSWORD="<password not available>"
fi

echo ""
print_info "GitOps bootstrap complete!"

echo ""
echo "🎉 Bootstrap Summary:"
echo "✅ ArgoCD installed and configured"
echo "✅ Projects and root applications deployed"
echo "✅ GitOps workflow established"
echo ""
echo "🔐 ArgoCD Access:"
echo "Username: admin"
echo "Password: $ADMIN_PASSWORD"
echo ""
echo "🌐 Access ArgoCD:"
echo "1. Port forward: kubectl port-forward svc/argocd-server -n argocd 8080:443"
echo "2. Visit: https://localhost:8080"
echo "3. Accept self-signed certificate"
echo "4. Login with admin credentials"
echo ""
echo "🚀 Next Steps:"
echo "1. Access ArgoCD UI to monitor deployments"
echo "2. Applications will automatically sync from Git"
echo "3. Infrastructure components (ACK, monitoring) will be deployed via GitOps"
echo "4. Monitor application health and sync status"
echo ""
echo "📖 Troubleshooting:"
echo "• Check application status: kubectl get applications -n argocd"
echo "• View application details: kubectl describe application <app-name> -n argocd"
echo "• Check ArgoCD logs: kubectl logs -n argocd deployment/argocd-application-controller"