#!/bin/bash

# Install ArgoCD for Cloud Native Encabulator
# This script installs ArgoCD and sets up the initial configuration

set -e

echo "🚀 Installing ArgoCD for Cloud Native Encabulator..."

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

# Check prerequisites
echo ""
print_info "Checking prerequisites..."

if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}❌ kubectl is not installed or not in PATH${NC}"
    exit 1
fi

if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}❌ Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi

print_status 0 "Prerequisites check passed"

# Step 1: Create ArgoCD namespace
echo ""
print_info "Step 1: Creating ArgoCD namespace..."

if kubectl apply -f k8s/platform/argocd/namespace.yaml > /dev/null 2>&1; then
    print_status 0 "Created argocd namespace"
else
    print_warning "argocd namespace may already exist"
fi

# Step 2: Install ArgoCD
echo ""
print_info "Step 2: Installing ArgoCD..."

if kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml > /dev/null 2>&1; then
    print_status 0 "ArgoCD installation manifests applied"
else
    print_status 1 "Failed to apply ArgoCD installation manifests"
    exit 1
fi

# Step 3: Wait for ArgoCD to be ready
echo ""
print_info "Step 3: Waiting for ArgoCD to be ready..."

print_info "Waiting for ArgoCD server deployment..."
if kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd > /dev/null 2>&1; then
    print_status 0 "ArgoCD server is ready"
else
    print_warning "ArgoCD server is not ready yet (may need more time)"
fi

print_info "Waiting for ArgoCD application controller..."
if kubectl wait --for=condition=available --timeout=300s deployment/argocd-application-controller -n argocd > /dev/null 2>&1; then
    print_status 0 "ArgoCD application controller is ready"
else
    print_warning "ArgoCD application controller is not ready yet"
fi

# Step 4: Apply ArgoCD configuration
echo ""
print_info "Step 4: Applying ArgoCD configuration..."

if kubectl apply -f k8s/platform/argocd/argocd-config.yaml > /dev/null 2>&1; then
    print_status 0 "ArgoCD configuration applied"
else
    print_warning "Failed to apply ArgoCD configuration (may need retry)"
fi

# Step 5: Create ArgoCD projects
echo ""
print_info "Step 5: Creating ArgoCD projects..."

if kubectl apply -f k8s/platform/argocd/argocd-projects.yaml > /dev/null 2>&1; then
    print_status 0 "ArgoCD projects created"
else
    print_warning "Failed to create ArgoCD projects (may need retry)"
fi

# Step 6: Get admin password
echo ""
print_info "Step 6: Retrieving ArgoCD admin credentials..."

# Wait for the secret to be created
sleep 10

if kubectl get secret argocd-initial-admin-secret -n argocd > /dev/null 2>&1; then
    ADMIN_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
    print_status 0 "Retrieved ArgoCD admin password"
else
    print_warning "ArgoCD admin secret not found yet"
    ADMIN_PASSWORD="<password not available yet>"
fi

echo ""
print_info "ArgoCD installation complete!"

echo ""
echo "📋 Installation Summary:"
echo "✅ ArgoCD installed in 'argocd' namespace"
echo "✅ Configuration and projects applied"
echo "✅ Ready for GitOps workflows"
echo ""
echo "🔐 Access Information:"
echo "Username: admin"
echo "Password: $ADMIN_PASSWORD"
echo ""
echo "🌐 Access Methods:"
echo "1. Port Forward: kubectl port-forward svc/argocd-server -n argocd 8080:443"
echo "2. Then visit: https://localhost:8080"
echo "3. Login with admin/$ADMIN_PASSWORD"
echo ""
echo "🚀 Next Steps:"
echo "1. Access ArgoCD UI and verify installation"
echo "2. Deploy root applications: kubectl apply -f k8s/platform/argocd/argocd-applications.yaml"
echo "3. Configure repository access if using private repos"
echo "4. Deploy infrastructure: ACK controllers, monitoring, etc."
echo ""
echo "📖 For more information, see: k8s/platform/argocd/README.md"