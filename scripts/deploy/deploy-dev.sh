#!/bin/bash

# Cloud Native Encabulator - Development Cluster Deployment
# Deploys the complete platform to a development EKS cluster

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

# Verify prerequisites
check_prerequisites() {
    log_info "Checking deployment prerequisites..."
    
    local missing_tools=()
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists aws; then
        missing_tools+=("aws-cli")
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
        log_info "Please run: aws eks update-kubeconfig --region <region> --name <cluster-name>"
        exit 1
    fi
    
    log_success "Prerequisites verified"
}

# Install ArgoCD
install_argocd() {
    log_info "Installing ArgoCD..."
    
    # Create ArgoCD namespace
    kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
    
    # Install ArgoCD
    if ! kubectl get deployment argocd-server -n argocd >/dev/null 2>&1; then
        log_info "Installing ArgoCD from manifests..."
        kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
        
        # Wait for ArgoCD to be ready
        log_info "Waiting for ArgoCD to be ready..."
        kubectl wait --for=condition=available --timeout=600s deployment/argocd-server -n argocd
        kubectl wait --for=condition=available --timeout=600s deployment/argocd-application-controller -n argocd
        kubectl wait --for=condition=available --timeout=600s deployment/argocd-repo-server -n argocd
    else
        log_info "ArgoCD already installed"
    fi
    
    log_success "ArgoCD is ready"
}

# Deploy data services
deploy_data_services() {
    log_info "Deploying in-cluster data services..."
    
    # Create data namespace
    kubectl create namespace encabulator-data --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy PostgreSQL
    log_info "Installing PostgreSQL..."
    helm repo add groundhog2k https://groundhog2k.github.io/helm-charts/ || true
    helm repo update
    helm upgrade --install hydrocoptic-postgresql groundhog2k/postgres \
        --namespace encabulator-data \
        --values helm/values/data-services/values-postgresql.yaml \
        --wait --timeout=300s
    
    # Deploy Redis
    log_info "Installing Redis..."
    helm upgrade --install prefabulated-cache groundhog2k/redis \
        --namespace encabulator-data \
        --values helm/values/data-services/values-redis.yaml \
        --wait --timeout=300s
    
    # Deploy InfluxDB
    log_info "Installing InfluxDB..."
    helm repo add influxdata https://helm.influxdata.com/ || true
    helm repo update
    helm upgrade --install sinusoidal-database influxdata/influxdb2 \
        --namespace encabulator-data \
        --values helm/values/data-services/values-influxdb.yaml \
        --wait --timeout=300s
    
    log_success "Data services ready"
}







# Deploy ArgoCD applications
deploy_argocd_applications() {
    log_info "Deploying ArgoCD applications..."
    
    # Create ArgoCD projects
    log_info "Creating ArgoCD projects..."
    kubectl apply -f argocd/projects/ 2>/dev/null || log_warning "ArgoCD projects directory not found, skipping"
    
    # Deploy infrastructure applications
    log_info "Deploying infrastructure applications..."
    kubectl apply -f argocd/infrastructure/
    
    # Deploy platform applications
    log_info "Deploying platform applications..."
    kubectl apply -f argocd/platform/
    
    log_success "ArgoCD applications deployed"
}

# Deploy development instances
deploy_development_instances() {
    log_info "Deploying development instances..."
    
    # Create necessary namespaces
    local namespaces=(
        "encabulator-monitoring"
        "encabulator-tracing" 
        "encabulator-logging"
        "encabulator-data"
        "encabulator-system"
    )
    
    for namespace in "${namespaces[@]}"; do
        kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -
    done
    
    # Deploy observability stack instance
    log_info "Deploying observability stack..."
    kubectl apply -k k8s/platform/observability/instances/
    
    log_success "Development instances deployed"
}

# Wait for deployments to be ready
wait_for_deployments() {
    log_info "Waiting for deployments to be ready..."
    
    # Wait for observability components
    local max_wait=600
    local wait_time=0
    
    log_info "Waiting for ObservabilityStack to be created..."
    while [ $wait_time -lt $max_wait ]; do
        if kubectl get observabilitystack encabulator-observability -n encabulator-monitoring >/dev/null 2>&1; then
            log_success "ObservabilityStack resource created"
            break
        fi
        sleep 10
        wait_time=$((wait_time + 10))
        log_info "Still waiting... ($wait_time/${max_wait}s)"
    done
    
    if [ $wait_time -ge $max_wait ]; then
        log_warning "ObservabilityStack not created within timeout, but continuing..."
    fi
    
    # Wait a bit more for underlying resources
    log_info "Waiting for underlying resources to be created by KRO..."
    sleep 60
    
    log_success "Deployment wait completed"
}

# Show deployment status
show_deployment_status() {
    log_info "Checking deployment status..."
    
    echo ""
    log_info "=== Cluster Information ==="
    kubectl cluster-info
    
    echo ""
    log_info "=== Namespaces ==="
    kubectl get namespaces | grep encabulator || log_warning "No encabulator namespaces found"
    
    echo ""
    log_info "=== Custom Resources ==="
    kubectl get crd | grep encabulator || log_warning "No encabulator CRDs found"
    
    echo ""
    log_info "=== ObservabilityStack Status ==="
    kubectl get observabilitystack -A 2>/dev/null || log_warning "No ObservabilityStack resources found"
    
    echo ""
    log_info "=== Key Deployments ==="
    kubectl get deployments -n encabulator-monitoring 2>/dev/null || log_warning "No deployments in encabulator-monitoring"
    kubectl get deployments -n encabulator-tracing 2>/dev/null || log_warning "No deployments in encabulator-tracing"
    kubectl get deployments -n encabulator-logging 2>/dev/null || log_warning "No deployments in encabulator-logging"
    
    echo ""
    log_info "=== Services ==="
    kubectl get services -n encabulator-monitoring 2>/dev/null || log_warning "No services in encabulator-monitoring"
    
    echo ""
    log_info "=== ArgoCD Applications ==="
    kubectl get applications -n argocd 2>/dev/null || log_warning "No ArgoCD applications found"
}

# Show access information
show_access_info() {
    log_success "Development cluster deployment completed!"
    log_info ""
    log_info "🎉 Your Cloud Native Encabulator development cluster is ready!"
    log_info ""
    log_info "Cluster: $(kubectl config current-context)"
    log_info "Region: $(aws configure get region 2>/dev/null || echo 'not configured')"
    log_info ""
    log_info "🔍 Check status:"
    log_info "  kubectl get all -n encabulator-monitoring"
    log_info "  kubectl get observabilitystack -A"
    log_info "  kubectl get applications -n argocd"
    log_info ""
    log_info "🌐 Access services:"
    log_info "  ArgoCD UI:   kubectl port-forward -n argocd svc/argocd-server 8080:443"
    log_info "  Prometheus:  kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090"
    log_info "  Grafana:     kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000"
    log_info "  Jaeger:      kubectl port-forward -n encabulator-tracing svc/encabulator-observability-tracing-jaeger 16686:16686"
    log_info ""
    log_info "🔑 Default credentials:"
    log_info "  ArgoCD:  admin / $(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d 2>/dev/null || echo 'check manually')"
    log_info "  Grafana: admin / encabulator-dev-2024"
    log_info ""
    log_info "📊 Monitoring:"
    log_info "  ./scripts/deploy/status.sh - Check deployment status"
    log_info "  ./scripts/deploy/logs.sh - View component logs"
}

# Main deployment function
main() {
    log_info "Starting Cloud Native Encabulator development cluster deployment..."
    echo ""
    
    check_prerequisites
    deploy_data_services
    wait_for_deployments
    show_deployment_status
    show_access_info
}

# Handle script arguments
case "${1:-}" in
    --status-only)
        log_info "Checking deployment status only..."
        show_deployment_status
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --status-only    Only check deployment status"
        echo "  --help, -h       Show this help message"
        echo ""
        echo "Default: Complete development cluster deployment"
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