#!/bin/bash

# Cloud Native Encabulator Microservices Demo - ArgoCD Bootstrap Script
# This script bootstraps ArgoCD and sets up GitOps-driven deployment

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

# Check if kubectl is available and cluster is accessible
check_kubernetes() {
    log_info "Checking Kubernetes cluster access..."
    
    if ! command -v kubectl >/dev/null 2>&1; then
        log_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot access Kubernetes cluster"
        log_info "Please ensure your EKS cluster is accessible:"
        log_info "  aws eks update-kubeconfig --region <region> --name <cluster-name>"
        exit 1
    fi
    
    local cluster_info=$(kubectl cluster-info | head -1)
    log_success "Connected to: $cluster_info"
    
    # Show current context
    local current_context=$(kubectl config current-context)
    log_info "Current context: $current_context"
}

# Install ArgoCD
install_argocd() {
    log_info "Installing ArgoCD..."
    
    # Create argocd namespace
    kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
    
    # Install ArgoCD
    log_info "Applying ArgoCD manifests..."
    kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
    
    # Wait for ArgoCD to be ready
    log_info "Waiting for ArgoCD to be ready..."
    kubectl wait --for=condition=available --timeout=600s deployment/argocd-server -n argocd
    kubectl wait --for=condition=available --timeout=600s deployment/argocd-repo-server -n argocd
    kubectl wait --for=condition=available --timeout=600s deployment/argocd-application-controller -n argocd
    
    log_success "ArgoCD installed successfully"
}

# Configure ArgoCD
configure_argocd() {
    log_info "Configuring ArgoCD..."
    
    # Get ArgoCD admin password
    local admin_password=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
    
    # Create ArgoCD configuration
    cat > argocd-config.yaml << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
  labels:
    app.kubernetes.io/name: argocd-cm
    app.kubernetes.io/part-of: argocd
data:
  # Enable insecure mode for easier access (development only)
  server.insecure: "true"
  
  # Repository configuration
  repositories: |
    - type: git
      url: https://github.com/your-org/cloud-native-encabulator-microservices
      name: cloud-native-encabulator-repo
  
  # Application configuration
  application.instanceLabelKey: argocd.argoproj.io/instance
EOF
    
    kubectl apply -f argocd-config.yaml
    
    # Create RBAC configuration
    cat > argocd-rbac.yaml << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-rbac-cm
  namespace: argocd
  labels:
    app.kubernetes.io/name: argocd-rbac-cm
    app.kubernetes.io/part-of: argocd
data:
  policy.default: role:readonly
  policy.csv: |
    p, role:admin, applications, *, */*, allow
    p, role:admin, clusters, *, *, allow
    p, role:admin, repositories, *, *, allow
    g, argocd-admins, role:admin
EOF
    
    kubectl apply -f argocd-rbac.yaml
    
    log_success "ArgoCD configured"
    log_info "ArgoCD admin password: $admin_password"
}

# Create ArgoCD Applications
create_argocd_applications() {
    log_info "Creating ArgoCD Applications..."
    
    # Create argocd applications directory
    mkdir -p argocd/applications
    
    # Infrastructure Application
    cat > argocd/applications/infrastructure.yaml << EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: encabulator-infrastructure
  namespace: argocd
  labels:
    app.kubernetes.io/name: cloud-native-encabulator
    app.kubernetes.io/component: infrastructure
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/cloud-native-encabulator-microservices
    targetRevision: main
    path: argocd/infrastructure
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
      - ServerSideApply=true
  ignoreDifferences:
  - group: apiextensions.k8s.io
    kind: CustomResourceDefinition
    jsonPointers:
    - /spec/conversion/webhook/clientConfig/caBundle
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: encabulator-platform
  namespace: argocd
  labels:
    app.kubernetes.io/name: cloud-native-encabulator
    app.kubernetes.io/component: platform
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/cloud-native-encabulator-microservices
    targetRevision: main
    path: argocd/platform
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: encabulator-applications
  namespace: argocd
  labels:
    app.kubernetes.io/name: cloud-native-encabulator
    app.kubernetes.io/component: applications
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/cloud-native-encabulator-microservices
    targetRevision: main
    path: argocd/applications
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
EOF
    
    # Apply ArgoCD Applications
    kubectl apply -f argocd/applications/infrastructure.yaml
    
    log_success "ArgoCD Applications created"
}

# Create ArgoCD directory structure
create_argocd_structure() {
    log_info "Creating ArgoCD directory structure..."
    
    # Create directory structure
    mkdir -p argocd/{infrastructure,platform,applications}
    
    # Infrastructure - ACK Controllers
    cat > argocd/infrastructure/ack-controllers.yaml << EOF
# ACK Controllers Installation
# This will be populated with ACK controller installations
apiVersion: v1
kind: Namespace
metadata:
  name: ack-system
  labels:
    app.kubernetes.io/name: ack-controllers
---
# Placeholder for ACK controller installations
# Will be populated in Task 2.1
EOF
    
    # Infrastructure - AWS Resources
    cat > argocd/infrastructure/aws-resources.yaml << EOF
# AWS Resources via ACK
# This will be populated with RDS, ElastiCache, IAM resources
# Placeholder - will be populated in Task 2.4
EOF
    
    # Platform - Base Kubernetes Resources
    cat > argocd/platform/namespaces.yaml << EOF
apiVersion: v1
kind: Namespace
metadata:
  name: encabulator-system
  labels:
    app.kubernetes.io/name: cloud-native-encabulator
    app.kubernetes.io/component: system
    cloud-native-encabulator.io/plane: system
---
apiVersion: v1
kind: Namespace
metadata:
  name: encabulator-data
  labels:
    app.kubernetes.io/name: cloud-native-encabulator
    app.kubernetes.io/component: data
    cloud-native-encabulator.io/plane: data
---
apiVersion: v1
kind: Namespace
metadata:
  name: encabulator-monitoring
  labels:
    app.kubernetes.io/name: cloud-native-encabulator
    app.kubernetes.io/component: monitoring
    cloud-native-encabulator.io/plane: observability
EOF
    
    # Platform - Monitoring Stack
    cat > argocd/platform/monitoring.yaml << EOF
# Monitoring Stack
# This will be populated with Prometheus, Grafana, Jaeger
# Using the existing infrastructure/kubernetes/base/monitoring-services.yaml
# Placeholder - will be populated in Task 5.1
EOF
    
    # Applications - Data Plane
    cat > argocd/applications/data-plane.yaml << EOF
# Data Plane Applications
# This will be populated with data services
# Placeholder - will be populated in Task 3
EOF
    
    # Applications - Processing Plane
    cat > argocd/applications/processing-plane.yaml << EOF
# Processing Plane Applications
# This will be populated with processing services
# Placeholder - will be populated in Task 4
EOF
    
    # Applications - Support Plane
    cat > argocd/applications/support-plane.yaml << EOF
# Support Plane Applications
# This will be populated with support services
# Placeholder - will be populated in Task 5
EOF
    
    log_success "ArgoCD directory structure created"
}

# Show access information
show_access_info() {
    log_success "ArgoCD Bootstrap completed!"
    log_info ""
    log_info "🎉 ArgoCD is now managing your Cloud Native Encabulator deployment!"
    log_info ""
    log_info "📊 Access ArgoCD UI:"
    log_info "  kubectl port-forward -n argocd svc/argocd-server 8080:443"
    log_info "  Then visit: https://localhost:8080"
    log_info "  Username: admin"
    log_info "  Password: $(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)"
    log_info ""
    log_info "🔧 ArgoCD Applications:"
    log_info "  - encabulator-infrastructure: ACK controllers and AWS resources"
    log_info "  - encabulator-platform: Base Kubernetes resources and monitoring"
    log_info "  - encabulator-applications: Microservices applications"
    log_info ""
    log_info "📁 GitOps Structure:"
    log_info "  argocd/infrastructure/  - ACK controllers and AWS resources"
    log_info "  argocd/platform/        - Base Kubernetes and monitoring"
    log_info "  argocd/applications/    - Microservices applications"
    log_info ""
    log_info "🚀 Next Steps:"
    log_info "  1. Update Git repository URL in ArgoCD applications"
    log_info "  2. Implement Task 2.1: Set up ACK controllers"
    log_info "  3. All future deployments will be managed by ArgoCD"
    log_info "  4. Monitor deployments in ArgoCD UI"
    log_info ""
    log_info "🧹 To clean up:"
    log_info "  kubectl delete namespace argocd"
    log_info "  rm -rf argocd/"
}

# Main bootstrap function
main() {
    log_info "Starting ArgoCD Bootstrap for Cloud Native Encabulator..."
    echo ""
    
    check_kubernetes
    install_argocd
    configure_argocd
    create_argocd_structure
    create_argocd_applications
    show_access_info
}

# Handle script arguments
case "${1:-}" in
    --clean)
        log_info "Cleaning up ArgoCD..."
        kubectl delete namespace argocd || true
        rm -rf argocd/ || true
        rm -f argocd-*.yaml || true
        log_success "ArgoCD cleanup completed"
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --clean      Clean up ArgoCD installation"
        echo "  --help, -h   Show this help message"
        echo ""
        echo "Default: Bootstrap ArgoCD for GitOps deployment"
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