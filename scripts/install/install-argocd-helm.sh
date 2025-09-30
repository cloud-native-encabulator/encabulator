#!/bin/bash

# Cloud Native Encabulator - Install ArgoCD via Helm
# Installs ArgoCD using the official Helm chart with our custom values

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing_tools=()
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists helm; then
        missing_tools+=("helm")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install:"
        log_info "  kubectl: https://kubernetes.io/docs/tasks/tools/"
        log_info "  helm: https://helm.sh/docs/intro/install/"
        exit 1
    fi
    
    # Verify cluster access
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot access Kubernetes cluster"
        exit 1
    fi
    
    log_success "Prerequisites verified"
}

# Add ArgoCD Helm repository
add_argocd_repo() {
    log_info "Adding ArgoCD Helm repository..."
    
    if ! helm repo list | grep -q "^argo"; then
        helm repo add argo https://argoproj.github.io/argo-helm
        helm repo update
        log_success "ArgoCD Helm repository added"
    else
        log_info "ArgoCD Helm repository already exists"
        helm repo update argo
    fi
}

# Install ArgoCD via Helm
install_argocd() {
    log_info "Installing ArgoCD via Helm..."
    
    # Create namespace
    kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
    
    # Install or upgrade ArgoCD
    helm upgrade --install argocd argo/argo-cd \
        --namespace argocd \
        --values helm/values/argocd/values-development.yaml \
        --wait --timeout=600s
    
    log_success "ArgoCD installed successfully"
}

# Wait for ArgoCD to be ready
wait_for_argocd() {
    log_info "Waiting for ArgoCD to be ready..."
    
    # Wait for deployments
    kubectl wait --for=condition=available --timeout=600s deployment/argocd-server -n argocd
    kubectl wait --for=condition=available --timeout=600s deployment/argocd-application-controller -n argocd
    kubectl wait --for=condition=available --timeout=600s deployment/argocd-repo-server -n argocd
    
    log_success "ArgoCD is ready"
}

# Get ArgoCD admin password
get_admin_password() {
    log_info "Retrieving ArgoCD admin password..."
    
    local password
    password=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d 2>/dev/null || echo "")
    
    if [ -n "$password" ]; then
        log_success "ArgoCD admin password: $password"
    else
        log_warning "Could not retrieve admin password. Check manually with:"
        log_info "kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
    fi
}

# Show access information
show_access_info() {
    log_success "ArgoCD installation completed!"
    log_info ""
    log_info "🌐 Access ArgoCD:"
    log_info "  kubectl port-forward -n argocd svc/argocd-server 8080:443"
    log_info "  Then visit: https://localhost:8080"
    log_info ""
    log_info "🔑 Login credentials:"
    log_info "  Username: admin"
    log_info "  Password: (see above or run get-password command)"
    log_info ""
    log_info "🔍 Useful commands:"
    log_info "  kubectl get all -n argocd"
    log_info "  helm list -n argocd"
    log_info "  helm status argocd -n argocd"
}

# Main installation function
main() {
    log_info "Starting ArgoCD Helm installation..."
    echo ""
    
    check_prerequisites
    add_argocd_repo
    install_argocd
    wait_for_argocd
    get_admin_password
    show_access_info
}

# Handle script arguments
case "${1:-}" in
    --get-password)
        get_admin_password
        ;;
    --status)
        log_info "ArgoCD Status:"
        kubectl get all -n argocd
        echo ""
        helm status argocd -n argocd
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --get-password   Get ArgoCD admin password"
        echo "  --status         Show ArgoCD status"
        echo "  --help, -h       Show this help message"
        echo ""
        echo "Default: Install ArgoCD via Helm"
        ;;
    "")
        main
        ;;
    *)
        log_error "Unknown option: $1"
        echo "Use --help for usage information"
        exit 1
        ;;
esac