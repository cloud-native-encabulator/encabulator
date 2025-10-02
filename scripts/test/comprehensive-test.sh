#!/bin/bash

# Cloud Native Encabulator - Comprehensive Testing Suite
# Tests all platform components: infrastructure, observability, and integration
# Updated for Helm-first architecture

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

# Test results tracking
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_WARNINGS=0

# Track test result
track_result() {
    case "$1" in
        "pass") TESTS_PASSED=$((TESTS_PASSED + 1)) ;;
        "fail") TESTS_FAILED=$((TESTS_FAILED + 1)) ;;
        "warn") TESTS_WARNINGS=$((TESTS_WARNINGS + 1)) ;;
    esac
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
    local cluster_type="unknown"
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists helm; then
        missing_tools+=("helm")
    fi
    
    # Check for container runtime
    local container_runtime=""
    if command_exists docker; then
        container_runtime="docker"
    elif command_exists finch; then
        container_runtime="finch"
    else
        missing_tools+=("docker or finch")
    fi
    
    # Determine cluster type
    if kubectl cluster-info 2>/dev/null | grep -q "kind"; then
        cluster_type="kind"
    elif kubectl cluster-info 2>/dev/null | grep -q "minikube"; then
        cluster_type="minikube"
    elif kubectl cluster-info 2>/dev/null | grep -q "k3d"; then
        cluster_type="k3d"
    elif kubectl cluster-info 2>/dev/null | grep -q "eks"; then
        cluster_type="eks"
    elif kubectl cluster-info >/dev/null 2>&1; then
        cluster_type="kubernetes"
    else
        missing_tools+=("kubernetes cluster access")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        track_result "fail"
        return 1
    fi
    
    log_success "✓ All prerequisites available"
    log_info "Cluster type: $cluster_type"
    if [ -n "$container_runtime" ]; then
        log_info "Container runtime: $container_runtime"
    fi
    track_result "pass"
    
    echo ""
}

# Test Phase 1: Helm Infrastructure Foundation
test_helm_infrastructure() {
    log_section "=== Phase 1: Helm Infrastructure Foundation ==="
    
    log_info "Testing Helm repositories and charts..."
    
    # Add required Helm repositories
    add_helm_repo "argo" "https://argoproj.github.io/argo-helm"
    add_helm_repo "prometheus-community" "https://prometheus-community.github.io/helm-charts"
    add_helm_repo "grafana" "https://grafana.github.io/helm-charts"
    add_helm_repo "postgresql" "https://charts.postgresql.org/"
    add_helm_repo "redis" "https://charts.redis.io/"
    
    # Test Helm repository access
    if helm search repo prometheus-community/kube-prometheus-stack >/dev/null 2>&1; then
        log_success "✓ Prometheus community charts accessible"
        track_result "pass"
    else
        log_error "✗ Cannot access Prometheus community charts"
        track_result "fail"
        return 1
    fi
    
    # Test ACK OCI chart access
    if helm show chart oci://public.ecr.aws/aws-controllers-k8s/rds-chart --version v0.0.25 >/dev/null 2>&1; then
        log_success "✓ ACK OCI charts accessible"
        track_result "pass"
    else
        log_warning "⚠ ACK OCI charts not accessible (expected in local testing)"
        track_result "warn"
    fi
    
    echo ""
}

# Test Phase 2: ArgoCD Installation
test_argocd_installation() {
    log_section "=== Phase 2: ArgoCD Installation ==="
    
    log_info "Testing ArgoCD installation via Helm..."
    
    # Check if ArgoCD namespace exists
    if kubectl get namespace argocd >/dev/null 2>&1; then
        log_success "✓ ArgoCD namespace exists"
        track_result "pass"
    else
        log_info "Installing ArgoCD via Helm..."
        
        # Install ArgoCD via Helm
        helm upgrade --install argocd argo/argo-cd \
            --namespace argocd \
            --create-namespace \
            --values helm/values/argocd/values-development.yaml \
            --wait --timeout=300s
        
        if [ $? -eq 0 ]; then
            log_success "✓ ArgoCD installed successfully"
            track_result "pass"
        else
            log_error "✗ ArgoCD installation failed"
            track_result "fail"
            return 1
        fi
    fi
    
    # Check ArgoCD deployment status
    if kubectl get deployment argocd-server -n argocd >/dev/null 2>&1; then
        local argocd_status=$(kubectl get deployment argocd-server -n argocd -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null || echo "0/0")
        if [[ "$argocd_status" == *"/"* ]] && [[ "${argocd_status%/*}" -gt 0 ]]; then
            log_success "✓ ArgoCD Server: $argocd_status ready"
            track_result "pass"
        else
            log_warning "⚠ ArgoCD Server: $argocd_status ready"
            track_result "warn"
        fi
    fi
    
    echo ""
}

# Test Phase 3: Observability Stack via Helm
test_observability_helm() {
    log_section "=== Phase 3: Observability Stack via Helm ==="
    
    log_info "Testing observability stack deployment via Helm..."
    
    # Create observability namespace
    kubectl create namespace encabulator-monitoring --dry-run=client -o yaml | kubectl apply -f -
    
    # Install kube-prometheus-stack
    log_info "Installing kube-prometheus-stack..."
    helm upgrade --install prometheus-stack prometheus-community/kube-prometheus-stack \
        --namespace encabulator-monitoring \
        --values helm/values/observability/values-prometheus.yaml \
        --wait --timeout=900s
    
    if [ $? -eq 0 ]; then
        log_success "✓ Prometheus stack installed successfully"
        track_result "pass"
    else
        log_error "✗ Prometheus stack installation failed"
        track_result "fail"
        return 1
    fi
    
    # Check Prometheus deployment
    if kubectl get deployment prometheus-stack-kube-prom-operator -n encabulator-monitoring >/dev/null 2>&1; then
        log_success "✓ Prometheus operator deployed"
        track_result "pass"
    else
        log_warning "⚠ Prometheus operator not found"
        track_result "warn"
    fi
    
    # Check Grafana deployment
    if kubectl get deployment prometheus-stack-grafana -n encabulator-monitoring >/dev/null 2>&1; then
        log_success "✓ Grafana deployed"
        track_result "pass"
    else
        log_warning "⚠ Grafana not found"
        track_result "warn"
    fi
    
    echo ""
}

# Test Phase 4: Data Plane Services via Helm
test_data_plane_helm() {
    log_section "=== Phase 4: Data Plane Services via Helm ==="
    
    log_info "Testing data plane services deployment via Helm..."
    
    # Create data namespace
    kubectl create namespace encabulator-data --dry-run=client -o yaml | kubectl apply -f -
    
    # Install Redis (Prefabulated Cache)
    log_info "Installing Redis for Prefabulated Cache..."
    helm upgrade --install prefabulated-cache redis/redis \
        --namespace encabulator-data \
        --set auth.password=prefabulated-cache-password \
        --set replica.replicaCount=0 \
        --set master.resources.requests.cpu=100m \
        --set master.resources.requests.memory=128Mi \
        --wait --timeout=300s
    
    if [ $? -eq 0 ]; then
        log_success "✓ Prefabulated Cache (Redis) installed successfully"
        track_result "pass"
    else
        log_warning "⚠ Prefabulated Cache installation had issues"
        track_result "warn"
    fi
    
    # Install Prometheus for Sinusoidal Database
    log_info "Installing Prometheus for Sinusoidal Database..."
    helm upgrade --install sinusoidal-database prometheus-community/prometheus \
        --namespace encabulator-data \
        --set server.name=sinusoidal-database \
        --set server.resources.requests.cpu=100m \
        --set server.resources.requests.memory=256Mi \
        --set alertmanager.enabled=false \
        --set kubeStateMetrics.enabled=false \
        --set nodeExporter.enabled=false \
        --set pushgateway.enabled=false \
        --wait --timeout=300s
    
    if [ $? -eq 0 ]; then
        log_success "✓ Sinusoidal Database (Prometheus TSDB) installed successfully"
        track_result "pass"
    else
        log_warning "⚠ Sinusoidal Database installation had issues"
        track_result "warn"
    fi
    
    echo ""
}

# Test Phase 5: Service Connectivity
test_service_connectivity() {
    log_section "=== Phase 5: Service Connectivity ==="
    
    log_info "Testing service connectivity and health..."
    
    # Wait for services to be ready
    log_info "Waiting for services to be ready..."
    sleep 30
    
    # Test key services
    local services_to_test=(
        "encabulator-monitoring:prometheus-stack-kube-prom-prometheus:9090"
        "encabulator-monitoring:prometheus-stack-grafana:80"
        "encabulator-data:prefabulated-cache-master:6379"
        "encabulator-data:sinusoidal-database-server:80"
    )
    
    local connectivity_success=true
    for service_info in "${services_to_test[@]}"; do
        IFS=':' read -r namespace service port <<< "$service_info"
        
        if kubectl get service "$service" -n "$namespace" >/dev/null 2>&1; then
            log_success "✓ Service exists: $service in $namespace"
            
            # Test connectivity with port-forward (background process)
            kubectl port-forward -n "$namespace" "svc/$service" "$port:$port" &
            local pf_pid=$!
            sleep 3
            
            # Test health endpoint
            local health_check=false
            case "$service" in
                *prometheus*) 
                    if curl -s "http://localhost:$port/-/healthy" >/dev/null 2>&1; then
                        health_check=true
                    fi
                    ;;
                *grafana*) 
                    if curl -s "http://localhost:$port/api/health" >/dev/null 2>&1; then
                        health_check=true
                    fi
                    ;;
                *redis*) 
                    # Redis doesn't have HTTP health endpoint, just check if port is open
                    if nc -z localhost "$port" 2>/dev/null; then
                        health_check=true
                    fi
                    ;;
                *server*) 
                    if curl -s "http://localhost:$port/-/healthy" >/dev/null 2>&1; then
                        health_check=true
                    fi
                    ;;
            esac
            
            if [ "$health_check" = true ]; then
                log_success "✓ Health check passed: $service"
            else
                log_warning "⚠ Health check failed: $service"
                connectivity_success=false
            fi
            
            # Clean up port-forward
            kill $pf_pid 2>/dev/null || true
            sleep 1
        else
            log_warning "⚠ Service not found: $service in $namespace"
            connectivity_success=false
        fi
    done
    
    if [ "$connectivity_success" = true ]; then
        track_result "pass"
    else
        track_result "warn"
    fi
    
    echo ""
}

# Test Phase 6: Helm Integration Validation
test_helm_integration() {
    log_section "=== Phase 6: Helm Integration Validation ==="
    
    log_info "Testing Helm releases and integration..."
    
    # Check Helm releases
    local helm_releases=(
        "argocd:argocd"
        "prometheus-stack:encabulator-monitoring"
        "prefabulated-cache:encabulator-data"
        "sinusoidal-database:encabulator-data"
    )
    
    local integration_success=true
    for release_info in "${helm_releases[@]}"; do
        IFS=':' read -r release namespace <<< "$release_info"
        
        if helm list -n "$namespace" | grep -q "$release"; then
            local status=$(helm status "$release" -n "$namespace" -o json | jq -r '.info.status' 2>/dev/null || echo "unknown")
            if [ "$status" = "deployed" ]; then
                log_success "✓ Helm release deployed: $release in $namespace"
            else
                log_warning "⚠ Helm release status: $release ($status)"
                integration_success=false
            fi
        else
            log_warning "⚠ Helm release not found: $release in $namespace"
            integration_success=false
        fi
    done
    
    # Check ServiceMonitors for monitoring integration
    if kubectl get servicemonitor -A | grep -q prometheus; then
        log_success "✓ ServiceMonitor integration working"
    else
        log_warning "⚠ ServiceMonitor integration not found"
        integration_success=false
    fi
    
    if [ "$integration_success" = true ]; then
        track_result "pass"
    else
        track_result "warn"
    fi
    
    echo ""
}

# Show comprehensive status
show_comprehensive_status() {
    log_section "=== Comprehensive Platform Status ==="
    
    echo ""
    log_info "Helm Releases:"
    helm list -A 2>/dev/null || log_warning "No Helm releases found"
    
    echo ""
    log_info "ArgoCD Components:"
    kubectl get deployments -n argocd 2>/dev/null || log_warning "No ArgoCD components found"
    
    echo ""
    log_info "Observability Components:"
    kubectl get deployments -n encabulator-monitoring 2>/dev/null || log_warning "No observability components found"
    
    echo ""
    log_info "Data Plane Components:"
    kubectl get deployments,statefulsets -n encabulator-data 2>/dev/null || log_warning "No data plane components found"
    
    echo ""
    log_info "Key Services:"
    kubectl get services -n encabulator-monitoring 2>/dev/null || log_warning "No monitoring services"
    kubectl get services -n encabulator-data 2>/dev/null || log_warning "No data services"
    
    echo ""
    log_info "ServiceMonitors:"
    kubectl get servicemonitor -A 2>/dev/null || log_warning "No ServiceMonitors found"
    
    echo ""
}

# Show test results summary
show_test_results() {
    log_section "=== Test Results Summary ==="
    
    local total_tests=$((TESTS_PASSED + TESTS_FAILED + TESTS_WARNINGS))
    
    echo ""
    log_info "📊 Test Results:"
    log_success "  ✓ Passed: $TESTS_PASSED"
    log_warning "  ⚠ Warnings: $TESTS_WARNINGS"
    log_error "  ✗ Failed: $TESTS_FAILED"
    log_info "  📈 Total: $total_tests"
    
    echo ""
    if [ $TESTS_FAILED -eq 0 ]; then
        if [ $TESTS_WARNINGS -eq 0 ]; then
            log_success "🎉 All tests passed! Helm-based platform is fully operational."
        else
            log_warning "⚠️ Tests passed with warnings. Platform is mostly operational."
        fi
    else
        log_error "❌ Some tests failed. Platform may have issues."
        return 1
    fi
    
    echo ""
    log_info "🌐 Access your platform:"
    log_info "  ArgoCD:      kubectl port-forward -n argocd svc/argocd-server 8080:443"
    log_info "  Prometheus:  kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090"
    log_info "  Grafana:     kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80"
    
    echo ""
    log_info "🔍 Monitor your platform:"
    log_info "  ./scripts/deploy/status.sh"
    log_info "  helm list -A"
    log_info "  kubectl get all -A"
    
    echo ""
    log_info "🚀 Next steps:"
    log_info "  1. Deploy via ArgoCD: ./scripts/deploy/deploy-helm.sh"
    log_info "  2. Start building service planes: Task 4.1"
    log_info "  3. Create production configurations"
}

# Main comprehensive test function
main() {
    log_info "Starting Cloud Native Encabulator Helm-based comprehensive testing..."
    echo ""
    
    check_prerequisites || exit 1
    test_helm_infrastructure || exit 1
    test_argocd_installation
    test_observability_helm
    test_data_plane_helm
    test_service_connectivity
    test_helm_integration
    show_comprehensive_status
    show_test_results
}

# Handle script arguments
case "${1:-}" in
    --quick)
        log_info "Running quick Helm test suite..."
        check_prerequisites
        test_helm_infrastructure
        test_observability_helm
        show_test_results
        ;;
    --infrastructure-only)
        log_info "Testing Helm infrastructure only..."
        check_prerequisites
        test_helm_infrastructure
        test_argocd_installation
        show_test_results
        ;;
    --observability-only)
        log_info "Testing observability only..."
        check_prerequisites
        test_observability_helm
        test_service_connectivity
        show_test_results
        ;;
    --data-plane-only)
        log_info "Testing data plane services only..."
        check_prerequisites
        test_data_plane_helm
        test_service_connectivity
        show_test_results
        ;;
    --status-only)
        log_info "Showing comprehensive status only..."
        show_comprehensive_status
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --quick               Run quick test suite (infrastructure + observability)"
        echo "  --infrastructure-only Test only infrastructure components (Helm + ArgoCD)"
        echo "  --observability-only  Test only observability components (Prometheus stack)"
        echo "  --data-plane-only     Test only data plane services (Redis, Prometheus TSDB)"
        echo "  --status-only         Show comprehensive platform status"
        echo "  --help, -h            Show this help message"
        echo ""
        echo "Default: Complete comprehensive testing suite with Helm-first approach"
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