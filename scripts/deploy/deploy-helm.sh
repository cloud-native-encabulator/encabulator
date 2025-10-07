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
    add_helm_repo "cowboysysop" "https://cowboysysop.github.io/charts/"
    add_helm_repo "groundhog2k" "https://groundhog2k.github.io/helm-charts/"
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

# Deploy data plane backends
deploy_data_plane() {
    log_section "=== Deploying Data Plane Backends ==="
    
    # Create data namespace
    kubectl create namespace encabulator-data --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy PostgreSQL (Hydrocoptic Data Store backend)
    log_info "Installing PostgreSQL (Hydrocoptic Data Store backend)..."
    helm upgrade --install hydrocoptic-postgresql groundhog2k/postgres \
        --namespace encabulator-data \
        --values helm/values/data-services/values-postgresql.yaml \
        --wait --timeout=600s
    
    # Deploy Redis (Prefabulated Cache backend)
    log_info "Installing Redis (Prefabulated Cache backend)..."
    helm upgrade --install prefabulated-cache-redis groundhog2k/redis \
        --namespace encabulator-data \
        --values helm/values/data-services/values-redis.yaml \
        --wait --timeout=300s
    
    # Deploy InfluxDB (Sinusoidal Database backend)
    log_info "Installing InfluxDB (Sinusoidal Database backend)..."
    helm upgrade --install sinusoidal-database-influxdb influxdata/influxdb2 \
        --namespace encabulator-data \
        --values helm/values/data-services/values-influxdb.yaml \
        --wait --timeout=300s
    
    log_success "Data plane backends deployed"
    echo ""
}

# Deploy Data Plane wrapper services
deploy_data_plane_services() {
    log_section "=== Deploying Data Plane Services ==="
    
    # Create data namespace if it doesn't exist
    kubectl create namespace encabulator-data --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy Hydrocoptic DataStore wrapper
    log_info "Deploying Hydrocoptic DataStore..."
    helm upgrade --install hydrocoptic-datastore ./helm/charts/hydrocoptic-datastore \
        --namespace encabulator-data \
        --wait --timeout=300s
    
    # Deploy Prefabulated Cache wrapper
    log_info "Deploying Prefabulated Cache..."
    helm upgrade --install prefabulated-cache ./helm/charts/prefabulated-cache \
        --namespace encabulator-data \
        --wait --timeout=300s
    
    # Deploy Sinusoidal Database wrapper
    log_info "Deploying Sinusoidal Database..."
    helm upgrade --install sinusoidal-database ./helm/charts/sinusoidal-database \
        --namespace encabulator-data \
        --wait --timeout=300s
    
    log_success "Data Plane services deployed (3/3)"
    echo ""
}

# Deploy Processing Plane services
deploy_processing_plane() {
    log_section "=== Deploying Processing Plane Services ==="
    
    # Create processing namespace
    kubectl create namespace encabulator-processing --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy Panametric Fam Processor
    log_info "Deploying Panametric Fam Processor..."
    helm upgrade --install panametric-fam-processor ./helm/charts/panametric-fam-processor \
        --namespace encabulator-processing \
        --wait --timeout=300s
    
    # Deploy Logarithmic Casing Service
    log_info "Deploying Logarithmic Casing Service..."
    helm upgrade --install logarithmic-casing-service ./helm/charts/logarithmic-casing-service \
        --namespace encabulator-processing \
        --wait --timeout=300s
    
    # Deploy Dingle Arm API Gateway
    log_info "Deploying Dingle Arm API Gateway..."
    helm upgrade --install dingle-arm-api-gateway ./helm/charts/dingle-arm-api-gateway \
        --namespace encabulator-processing \
        --wait --timeout=300s
    
    log_success "Processing Plane services deployed (3/3)"
    echo ""
}

# Deploy Support Plane services
deploy_support_plane() {
    log_section "=== Deploying Support Plane Services ==="
    
    # Create support namespace
    kubectl create namespace encabulator-support --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy Unilateral Phase Detractor (auth)
    log_info "Deploying Unilateral Phase Detractor (Auth)..."
    helm upgrade --install unilateral-phase-detractor ./helm/charts/unilateral-phase-detractor \
        --namespace encabulator-support \
        --wait --timeout=300s
    
    # Deploy Differential Girdle (feature flags)
    log_info "Deploying Differential Girdle (Feature Flags)..."
    helm upgrade --install differential-girdle ./helm/charts/differential-girdle \
        --namespace encabulator-support \
        --wait --timeout=300s
    
    # Deploy Side Fumbling Collector (telemetry)
    log_info "Deploying Side Fumbling Collector (Telemetry)..."
    helm upgrade --install side-fumbling-collector ./helm/charts/side-fumbling-collector \
        --namespace encabulator-support \
        --wait --timeout=300s
    
    log_success "Support Plane services deployed (3/3)"
    echo ""
}

# Deploy Control Plane services
deploy_control_plane() {
    log_section "=== Deploying Control Plane Services ==="
    
    # Create control namespace
    kubectl create namespace encabulator-control --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy Encabulation Control Console
    log_info "Deploying Encabulation Control Console..."
    helm upgrade --install encabulation-control-console ./helm/charts/encabulation-control-console \
        --namespace encabulator-control \
        --wait --timeout=300s
    
    # Deploy Malaise Injection Engine
    log_info "Deploying Malaise Injection Engine..."
    helm upgrade --install malaise-injection-engine ./helm/charts/malaise-injection-engine \
        --namespace encabulator-control \
        --wait --timeout=300s
    
    # Deploy Cardinal Grammeter (traffic gen)
    log_info "Deploying Cardinal Grammeter (Traffic Generator)..."
    helm upgrade --install cardinal-grammeter ./helm/charts/cardinal-grammeter \
        --namespace encabulator-control \
        --wait --timeout=300s
    
    # Deploy Demo Scenario Orchestrator
    log_info "Deploying Demo Scenario Orchestrator..."
    helm upgrade --install demo-scenario-orchestrator ./helm/charts/demo-scenario-orchestrator \
        --namespace encabulator-control \
        --wait --timeout=300s
    
    log_success "Control Plane services deployed (4/4)"
    echo ""
}

# Validate deployment
validate_deployment() {
    log_section "=== Validating Deployment ==="
    
    local total_expected=0
    local total_found=0
    
    # Check Observability
    log_info "Observability Stack:"
    if kubectl get deployment prometheus-stack-kube-prom-operator -n encabulator-monitoring >/dev/null 2>&1; then
        log_success "  ✓ Prometheus Operator"
        ((total_found++))
    else
        log_warning "  ⚠ Prometheus Operator not found"
    fi
    ((total_expected++))
    
    if kubectl get deployment prometheus-stack-grafana -n encabulator-monitoring >/dev/null 2>&1; then
        log_success "  ✓ Grafana"
        ((total_found++))
    else
        log_warning "  ⚠ Grafana not found"
    fi
    ((total_expected++))
    
    # Check Data Plane Backends
    log_info "Data Plane Backends:"
    if kubectl get statefulset hydrocoptic-postgresql -n encabulator-data >/dev/null 2>&1; then
        log_success "  ✓ PostgreSQL"
        ((total_found++))
    else
        log_warning "  ⚠ PostgreSQL not found"
    fi
    ((total_expected++))
    
    if kubectl get deployment prefabulated-cache-redis-master -n encabulator-data >/dev/null 2>&1; then
        log_success "  ✓ Redis"
        ((total_found++))
    else
        log_warning "  ⚠ Redis not found"
    fi
    ((total_expected++))
    
    if kubectl get statefulset sinusoidal-database-influxdb-influxdb2 -n encabulator-data >/dev/null 2>&1; then
        log_success "  ✓ InfluxDB"
        ((total_found++))
    else
        log_warning "  ⚠ InfluxDB not found"
    fi
    ((total_expected++))
    
    # Check Data Plane Services
    if kubectl get namespace encabulator-data >/dev/null 2>&1 && kubectl get deployments -n encabulator-data | grep -q hydrocoptic-datastore; then
        log_info "Data Plane Services:"
        
        if kubectl get deployment hydrocoptic-datastore -n encabulator-data >/dev/null 2>&1; then
            log_success "  ✓ Hydrocoptic DataStore"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment prefabulated-cache -n encabulator-data >/dev/null 2>&1; then
            log_success "  ✓ Prefabulated Cache"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment sinusoidal-database -n encabulator-data >/dev/null 2>&1; then
            log_success "  ✓ Sinusoidal Database"
            ((total_found++))
        fi
        ((total_expected++))
    fi
    
    # Check Processing Plane
    if kubectl get namespace encabulator-processing >/dev/null 2>&1; then
        log_info "Processing Plane Services:"
        
        if kubectl get deployment panametric-fam-processor -n encabulator-processing >/dev/null 2>&1; then
            log_success "  ✓ Panametric Fam Processor"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment logarithmic-casing-service -n encabulator-processing >/dev/null 2>&1; then
            log_success "  ✓ Logarithmic Casing Service"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment dingle-arm-api-gateway -n encabulator-processing >/dev/null 2>&1; then
            log_success "  ✓ Dingle Arm API Gateway"
            ((total_found++))
        fi
        ((total_expected++))
    fi
    
    # Check Support Plane
    if kubectl get namespace encabulator-support >/dev/null 2>&1; then
        log_info "Support Plane Services:"
        
        if kubectl get deployment unilateral-phase-detractor -n encabulator-support >/dev/null 2>&1; then
            log_success "  ✓ Unilateral Phase Detractor"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment differential-girdle -n encabulator-support >/dev/null 2>&1; then
            log_success "  ✓ Differential Girdle"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment side-fumbling-collector -n encabulator-support >/dev/null 2>&1; then
            log_success "  ✓ Side Fumbling Collector"
            ((total_found++))
        fi
        ((total_expected++))
    fi
    
    # Check Control Plane
    if kubectl get namespace encabulator-control >/dev/null 2>&1; then
        log_info "Control Plane Services:"
        
        if kubectl get deployment encabulation-control-console -n encabulator-control >/dev/null 2>&1; then
            log_success "  ✓ Encabulation Control Console"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment malaise-injection-engine -n encabulator-control >/dev/null 2>&1; then
            log_success "  ✓ Malaise Injection Engine"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment cardinal-grammeter -n encabulator-control >/dev/null 2>&1; then
            log_success "  ✓ Cardinal Grammeter"
            ((total_found++))
        fi
        ((total_expected++))
        
        if kubectl get deployment demo-scenario-orchestrator -n encabulator-control >/dev/null 2>&1; then
            log_success "  ✓ Demo Scenario Orchestrator"
            ((total_found++))
        fi
        ((total_expected++))
    fi
    
    echo ""
    log_info "Deployment Status: $total_found/$total_expected components found"
    echo ""
}

# Show access information
show_access_info() {
    log_success "Deployment completed!"
    log_info ""
    log_info "🎉 Your Cloud Native Encabulator platform is ready!"
    log_info ""
    log_info "🌐 Access services:"
    log_info "  Prometheus:  kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090"
    log_info "  Grafana:     kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80"
    
    # Check if Control Console is deployed
    if kubectl get deployment encabulation-control-console -n encabulator-control >/dev/null 2>&1; then
        log_info "  Control UI:  kubectl port-forward -n encabulator-control svc/encabulation-control-console 8080:8080"
    fi
    
    log_info ""
    log_info "🔑 Default credentials:"
    log_info "  Grafana: admin / encabulator-dev-2024"
    log_info ""
    log_info "🔍 Monitor deployment:"
    log_info "  helm list -A"
    log_info "  kubectl get pods -A | grep encabulator"
    log_info "  ./scripts/deploy/deploy-helm.sh --validate"
    log_info ""
    
    # Show next steps based on what's deployed
    if kubectl get namespace encabulator-control >/dev/null 2>&1; then
        log_info "📊 Next steps:"
        log_info "  1. Access Grafana and verify dashboards"
        log_info "  2. Access Control Console for interactive demos"
        log_info "  3. Run end-to-end tests: ./scripts/test/comprehensive-test.sh"
    else
        log_info "📊 Next steps:"
        log_info "  1. Access Grafana and verify dashboards"
        log_info "  2. Test data services: ./scripts/test/test-data-services.sh"
        log_info "  3. Deploy service planes: ./scripts/deploy/deploy-helm.sh --all"
        log_info "  4. Run comprehensive tests: ./scripts/test/comprehensive-test.sh"
    fi
}

# Main deployment function (foundation only)
main() {
    log_info "Starting Cloud Native Encabulator foundation deployment..."
    echo ""
    
    check_prerequisites
    setup_helm_repos
    deploy_observability
    deploy_data_plane
    validate_deployment
    show_access_info
}

# Deploy all services (foundation + all service planes)
deploy_all() {
    log_info "Starting complete Cloud Native Encabulator deployment..."
    echo ""
    
    check_prerequisites
    setup_helm_repos
    deploy_observability
    deploy_data_plane
    deploy_data_plane_services
    deploy_processing_plane
    deploy_support_plane
    deploy_control_plane
    validate_deployment
    show_access_info
}

# Handle script arguments
case "${1:-}" in
    --all)
        deploy_all
        ;;
    --observability-only)
        log_info "Deploying observability stack only..."
        check_prerequisites
        setup_helm_repos
        deploy_observability
        validate_deployment
        ;;
    --data-plane-only)
        log_info "Deploying data plane backends only..."
        check_prerequisites
        setup_helm_repos
        deploy_data_plane
        validate_deployment
        ;;
    --data-plane-services)
        log_info "Deploying Data Plane services only..."
        check_prerequisites
        deploy_data_plane_services
        validate_deployment
        ;;
    --processing-plane)
        log_info "Deploying Processing Plane services only..."
        check_prerequisites
        deploy_processing_plane
        validate_deployment
        ;;
    --support-plane)
        log_info "Deploying Support Plane services only..."
        check_prerequisites
        deploy_support_plane
        validate_deployment
        ;;
    --control-plane)
        log_info "Deploying Control Plane services only..."
        check_prerequisites
        deploy_control_plane
        validate_deployment
        ;;
    --validate)
        log_info "Validating deployment only..."
        validate_deployment
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Foundation Deployment:"
        echo "  (no options)         Deploy foundation (observability + data backends)"
        echo "  --all                Deploy everything (foundation + all 13 services)"
        echo ""
        echo "Component Deployment:"
        echo "  --observability-only Deploy only observability stack (Prometheus, Grafana)"
        echo "  --data-plane-only    Deploy only data backends (PostgreSQL, Redis, InfluxDB)"
        echo ""
        echo "Service Plane Deployment:"
        echo "  --data-plane-services Deploy Data Plane services (3 services)"
        echo "  --processing-plane    Deploy Processing Plane services (3 services)"
        echo "  --support-plane       Deploy Support Plane services (3 services)"
        echo "  --control-plane       Deploy Control Plane services (4 services)"
        echo ""
        echo "Utilities:"
        echo "  --validate           Validate existing deployment"
        echo "  --help, -h           Show this help message"
        echo ""
        echo "Examples:"
        echo "  $0                   # Deploy foundation only"
        echo "  $0 --all             # Deploy complete platform"
        echo "  $0 --data-plane-services  # Deploy just Data Plane wrappers"
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