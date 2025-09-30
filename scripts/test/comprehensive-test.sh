#!/bin/bash

# Cloud Native Encabulator - Comprehensive Testing Suite
# Tests all platform components: infrastructure, observability, and integration

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

# Test Phase 1: Infrastructure Foundation
test_infrastructure_foundation() {
    log_section "=== Phase 1: Infrastructure Foundation ==="
    
    log_info "Running infrastructure tests..."
    if ./scripts/test/infrastructure-test.sh --kro-only; then
        log_success "✓ Infrastructure foundation tests passed"
        track_result "pass"
    else
        log_error "✗ Infrastructure foundation tests failed"
        track_result "fail"
        return 1
    fi
    
    echo ""
}

# Test Phase 2: Resource Orchestration
test_resource_orchestration() {
    log_section "=== Phase 2: Resource Orchestration ==="
    
    log_info "Testing KRO ResourceGroups and custom resources..."
    if ./scripts/test/infrastructure-test.sh --instances-only; then
        log_success "✓ Resource orchestration tests passed"
        track_result "pass"
    else
        log_warning "⚠ Resource orchestration tests had issues"
        track_result "warn"
    fi
    
    echo ""
}

# Test Phase 3: Observability Platform
test_observability_platform() {
    log_section "=== Phase 3: Observability Platform ==="
    
    log_info "Testing observability stack deployment..."
    
    # Create observability namespaces
    local namespaces=("encabulator-monitoring" "encabulator-tracing" "encabulator-logging")
    for namespace in "${namespaces[@]}"; do
        kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -
    done
    
    # Apply observability ResourceGroups
    log_info "Applying observability ResourceGroups..."
    kubectl apply -k k8s/platform/observability/resource-graph-definitions/
    
    # Wait for ResourceGroups to be established
    sleep 20
    
    # Check observability CRDs
    local observability_crds=(
        "observabilitymetrics.observability.encabulator.io"
        "observabilitytracing.observability.encabulator.io"
        "observabilitylogs.observability.encabulator.io"
        "observabilitystack.observability.encabulator.io"
    )
    
    local crd_success=true
    for crd in "${observability_crds[@]}"; do
        if kubectl get crd "$crd" >/dev/null 2>&1; then
            log_success "✓ Observability CRD: $crd"
        else
            log_warning "⚠ Observability CRD: $crd (not found)"
            crd_success=false
        fi
    done
    
    if [ "$crd_success" = true ]; then
        track_result "pass"
    else
        track_result "warn"
    fi
    
    # Deploy observability stack instance
    log_info "Deploying observability stack instance..."
    kubectl apply -k k8s/platform/observability/instances/
    
    # Wait for ObservabilityStack resource
    local max_wait=120
    local wait_time=0
    while [ $wait_time -lt $max_wait ]; do
        if kubectl get observabilitystack encabulator-observability -n encabulator-monitoring >/dev/null 2>&1; then
            log_success "✓ ObservabilityStack resource created"
            track_result "pass"
            break
        fi
        sleep 5
        wait_time=$((wait_time + 5))
    done
    
    if [ $wait_time -ge $max_wait ]; then
        log_warning "⚠ ObservabilityStack resource not created within timeout"
        track_result "warn"
    fi
    
    echo ""
}

# Test Phase 4: Service Connectivity
test_service_connectivity() {
    log_section "=== Phase 4: Service Connectivity ==="
    
    log_info "Testing service connectivity and health..."
    
    # Wait for services to be ready
    sleep 60
    
    # Test key services
    local services_to_test=(
        "encabulator-monitoring:encabulator-observability-metrics-prometheus:9090"
        "encabulator-monitoring:encabulator-observability-grafana:3000"
        "encabulator-tracing:encabulator-observability-tracing-jaeger:16686"
        "encabulator-logging:encabulator-observability-logs-loki:3100"
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
                *jaeger*) 
                    if curl -s "http://localhost:$port/" >/dev/null 2>&1; then
                        health_check=true
                    fi
                    ;;
                *loki*) 
                    if curl -s "http://localhost:$port/ready" >/dev/null 2>&1; then
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

# Test Phase 5: Integration Validation
test_integration_validation() {
    log_section "=== Phase 5: Integration Validation ==="
    
    log_info "Testing end-to-end integration..."
    
    # Test that infrastructure and observability work together
    local integration_success=true
    
    # Check that infrastructure resources have monitoring
    if kubectl get servicemonitor -A | grep -q encabulator; then
        log_success "✓ Infrastructure monitoring integration"
    else
        log_warning "⚠ Infrastructure monitoring integration not found"
        integration_success=false
    fi
    
    # Check that observability stack is monitoring itself
    if kubectl get servicemonitor -n encabulator-monitoring | grep -q prometheus; then
        log_success "✓ Self-monitoring integration"
    else
        log_warning "⚠ Self-monitoring integration not found"
        integration_success=false
    fi
    
    # Test resource composition (EncabulatorCluster creates sub-resources)
    local cluster_count=$(kubectl get encabulatorcluster -A --no-headers 2>/dev/null | wc -l)
    local database_count=$(kubectl get encabulatordatabase -A --no-headers 2>/dev/null | wc -l)
    
    if [ "$cluster_count" -gt 0 ] && [ "$database_count" -ge "$cluster_count" ]; then
        log_success "✓ Resource composition working"
    else
        log_warning "⚠ Resource composition may have issues"
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
    log_info "Infrastructure Components:"
    kubectl get deployments -n kro-system 2>/dev/null || log_warning "No KRO components found"
    kubectl get deployments -n ack-system 2>/dev/null || log_warning "No ACK components found"
    
    echo ""
    log_info "Custom Resource Definitions:"
    kubectl get crd | grep -E "(encabulator|observability)" || log_warning "No custom CRDs found"
    
    echo ""
    log_info "Infrastructure Instances:"
    kubectl get encabulatorcluster -A 2>/dev/null || log_warning "No cluster instances"
    kubectl get encabulatordatabase -A 2>/dev/null || log_warning "No database instances"
    kubectl get encabulatorcache -A 2>/dev/null || log_warning "No cache instances"
    
    echo ""
    log_info "Observability Instances:"
    kubectl get observabilitystack -A 2>/dev/null || log_warning "No observability stacks"
    
    echo ""
    log_info "Key Services:"
    kubectl get services -n encabulator-monitoring 2>/dev/null || log_warning "No monitoring services"
    kubectl get services -n encabulator-tracing 2>/dev/null || log_warning "No tracing services"
    kubectl get services -n encabulator-logging 2>/dev/null || log_warning "No logging services"
    
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
            log_success "🎉 All tests passed! Platform is fully operational."
        else
            log_warning "⚠️ Tests passed with warnings. Platform is mostly operational."
        fi
    else
        log_error "❌ Some tests failed. Platform may have issues."
        return 1
    fi
    
    echo ""
    log_info "🌐 Access your platform:"
    log_info "  Prometheus:  kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090"
    log_info "  Grafana:     kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000"
    log_info "  Jaeger:      kubectl port-forward -n encabulator-tracing svc/encabulator-observability-tracing-jaeger 16686:16686"
    
    echo ""
    log_info "🔍 Monitor your platform:"
    log_info "  ./scripts/deploy/status.sh"
    log_info "  kubectl get all -A -l app.kubernetes.io/managed-by=kro"
    
    echo ""
    log_info "🚀 Next steps:"
    log_info "  1. Deploy to development cluster: ./scripts/deploy/deploy-dev.sh"
    log_info "  2. Start building service planes: Task 4.1"
    log_info "  3. Create production configurations"
}

# Main comprehensive test function
main() {
    log_info "Starting Cloud Native Encabulator comprehensive testing..."
    echo ""
    
    check_prerequisites || exit 1
    test_infrastructure_foundation || exit 1
    test_resource_orchestration
    test_observability_platform
    test_service_connectivity
    test_integration_validation
    show_comprehensive_status
    show_test_results
}

# Handle script arguments
case "${1:-}" in
    --quick)
        log_info "Running quick test suite..."
        check_prerequisites
        test_infrastructure_foundation
        test_observability_platform
        show_test_results
        ;;
    --infrastructure-only)
        log_info "Testing infrastructure only..."
        check_prerequisites
        test_infrastructure_foundation
        test_resource_orchestration
        show_test_results
        ;;
    --observability-only)
        log_info "Testing observability only..."
        check_prerequisites
        test_observability_platform
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
        echo "  --infrastructure-only Test only infrastructure components"
        echo "  --observability-only  Test only observability components"
        echo "  --status-only         Show comprehensive platform status"
        echo "  --help, -h            Show this help message"
        echo ""
        echo "Default: Complete comprehensive testing suite"
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