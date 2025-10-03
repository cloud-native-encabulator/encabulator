#!/bin/bash

# Cloud Native Encabulator - Helm-First Deployment
# Deploys the complete platform using Helm charts with ArgoCD GitOps

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
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

log_section() {
    echo -e "${PURPLE}[SECTION]${NC} $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Add Helm repository
add_helm_repo() {
    local repo_name="$1"
    local repo_url="$2"
    
    if ! helm repo list | grep -q "^$repo_name"; then
        log_info "Adding Helm repository: $repo_name"
        helm repo add "$repo_name" "$repo_url"
        helm repo update
    fi
}

# Check prerequisites
check_prerequisites() {
    log_section "=== Checking Prerequisites ==="
    
    local missing_tools=()
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists helm; then
        missing_tools+=("helm")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        exit 1
    fi
    
    # Verify cluster access
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot access Kubernetes cluster"
        exit 1
    fi
    
    log_success "Prerequisites verified"
    log_info "Cluster: $(kubectl config current-context)"
    echo ""
}

# Setup Helm repositories
setup_helm_repos() {
    log_section "=== Setting up Helm Repositories ==="
    
    add_helm_repo "argo" "https://argoproj.github.io/argo-helm"
    add_helm_repo "prometheus-community" "https://prometheus-community.github.io/helm-charts"
    add_helm_repo "grafana" "https://grafana.github.io/helm-charts"
    add_helm_repo "postgresql" "https://charts.postgresql.org/"
    add_helm_repo "redis" "https://charts.redis.io/"
    add_helm_repo "influxdata" "https://helm.influxdata.com/"
    
    log_success "Helm repositories configured"
    echo ""
}

# Deploy ArgoCD
deploy_argocd() {
    log_section "=== Deploying ArgoCD ==="
    
    if kubectl get namespace argocd >/dev/null 2>&1; then
        log_info "ArgoCD namespace already exists"
    else
        log_info "Installing ArgoCD via Helm..."
        
        helm upgrade --install argocd argo/argo-cd \
            --namespace argocd \
            --create-namespace \
            --values helm/values/argocd/values-development.yaml \
            --wait --timeout=600s
        
        log_success "ArgoCD installed successfully"
    fi
    
    # Wait for ArgoCD to be ready
    log_info "Waiting for ArgoCD to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd
    
    log_success "ArgoCD is ready"
    echo ""
}

# Deploy observability stack
deploy_observability() {
    log_section "=== Deploying Observability Stack ==="
    
    # Create monitoring namespace
    kubectl create namespace encabulator-monitoring --dry-run=client -o yaml | kubectl apply -f -
    
    log_info "Installing kube-prometheus-stack..."
    helm upgrade --install prometheus-stack prometheus-community/kube-prometheus-stack \
        --namespace encabulator-monitoring \
        --values helm/values/observability/values-prometheus.yaml \
        --wait --timeout=900s
    
    if [ $? -eq 0 ]; then
        log_success "Observability stack deployed successfully"
    else
        log_error "Observability stack deployment failed"
        return 1
    fi
    
    echo ""
}

# Deploy data plane services
deploy_data_plane() {
    log_section "=== Deploying Data Plane Services ==="
    
    # Create data namespace
    kubectl create namespace encabulator-data --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy PostgreSQL (Hydrocoptic Data Store backend)
    log_info "Installing PostgreSQL (Hydrocoptic Data Store backend)..."
    helm upgrade --install hydrocoptic-postgresql postgresql/postgresql \
        --namespace encabulator-data \
        --values helm/values/data-services/values-postgresql.yaml \
        --wait --timeout=600s
    
    # Deploy Redis (Prefabulated Cache)
    log_info "Installing Prefabulated Cache (Redis)..."
    helm upgrade --install prefabulated-cache redis/redis \
        --namespace encabulator-data \
        --values helm/values/data-services/values-redis.yaml \
        --wait --timeout=300s
    
    # Deploy InfluxDB (Sinusoidal Database)
    log_info "Installing Sinusoidal Database (InfluxDB)..."
    helm upgrade --install sinusoidal-database influxdata/influxdb2 \
        --namespace encabulator-data \
        --values helm/values/data-services/values-influxdb.yaml \
        --wait --timeout=300s
    
    log_success "Data plane services deployed"
    echo ""
}

# Validate deployment
validate_deployment() {
    log_section "=== Validating Deployment ==="
    
    # Check ArgoCD
    if kubectl get deployment argocd-server -n argocd >/dev/null 2>&1; then
        log_success "✓ ArgoCD Server deployed"
    else
        log_warning "⚠ ArgoCD Server not found"
    fi
    
    # Check Prometheus
    if kubectl get deployment prometheus-stack-kube-prom-operator -n encabulator-monitoring >/dev/null 2>&1; then
        log_success "✓ Prometheus Operator deployed"
    else
        log_warning "⚠ Prometheus Operator not found"
    fi
    
    # Check Grafana
    if kubectl get deployment prometheus-stack-grafana -n encabulator-monitoring >/dev/null 2>&1; then
        log_success "✓ Grafana deployed"
    else
        log_warning "⚠ Grafana not found"
    fi
    
    # Check data services
    if kubectl get statefulset hydrocoptic-postgresql -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ PostgreSQL (Hydrocoptic backend) deployed"
    else
        log_warning "⚠ PostgreSQL (Hydrocoptic backend) not found"
    fi
    
    if kubectl get deployment prefabulated-cache-master -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ Prefabulated Cache deployed"
    else
        log_warning "⚠ Prefabulated Cache not found"
    fi
    
    if kubectl get statefulset sinusoidal-database-influxdb2 -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ Sinusoidal Database (InfluxDB) deployed"
    else
        log_warning "⚠ Sinusoidal Database (InfluxDB) not found"
    fi
    
    echo ""
}

# Show access information
show_access_info() {
    log_success "Helm-first deployment completed!"
    log_info ""
    log_info "🎉 Your Cloud Native Encabulator platform is ready!"
    log_info ""
    log_info "🌐 Access services:"
    log_info "  ArgoCD:      kubectl port-forward -n argocd svc/argocd-server 8080:443"
    log_info "  Prometheus:  kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090"
    log_info "  Grafana:     kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80"
    log_info ""
    log_info "🔑 Default credentials:"
    local argocd_password=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d 2>/dev/null || echo "not-found")
    if [ "$argocd_password" != "not-found" ]; then
        log_info "  ArgoCD:  admin / $argocd_password"
    else
        log_info "  ArgoCD:  admin / (get password: kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath=\"{.data.password}\" | base64 -d)"
    fi
    log_info "  Grafana: admin / encabulator-dev-2024"
    log_info ""
    log_info "🔍 Monitor deployment:"
    log_info "  helm list -A"
    log_info "  kubectl get all -n encabulator-monitoring"
    log_info "  kubectl get all -n encabulator-data"
    log_info ""
    log_info "📊 Next steps:"
    log_info "  1. Access Grafana and verify dashboards"
    log_info "  2. Test data services: ./scripts/test/test-data-services.sh"
    log_info "  3. Deploy service planes: Task 4.1"
    log_info "  4. Run comprehensive tests: ./scripts/test/comprehensive-test.sh"
}

# Main deployment function
main() {
    log_info "Starting Cloud Native Encabulator Helm-first deployment..."
    echo ""
    
    check_prerequisites
    setup_helm_repos
    deploy_argocd
    deploy_observability
    deploy_data_plane
    validate_deployment
    show_access_info
}

# Handle script arguments
case "${1:-}" in
    --argocd-only)
        log_info "Deploying ArgoCD only..."
        check_prerequisites
        setup_helm_repos
        deploy_argocd
        ;;
    --observability-only)
        log_info "Deploying observability stack only..."
        check_prerequisites
        setup_helm_repos
        deploy_observability
        validate_deployment
        ;;
    --data-plane-only)
        log_info "Deploying data plane services only..."
        check_prerequisites
        setup_helm_repos
        deploy_data_plane
        validate_deployment
        ;;
    --validate)
        log_info "Validating deployment only..."
        validate_deployment
        show_access_info
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --argocd-only        Deploy only ArgoCD"
        echo "  --observability-only Deploy only observability stack"
        echo "  --data-plane-only    Deploy only data plane services"
        echo "  --validate           Validate existing deployment"
        echo "  --help, -h           Show this help message"
        echo ""
        echo "Default: Complete Helm-first platform deployment"
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