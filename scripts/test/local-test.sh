#!/bin/bash

# Cloud Native Encabulator - Local Testing Script
# Tests KRO ResourceGroups and observability stack on local Kubernetes

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

# Check prerequisites for local testing
check_prerequisites() {
    log_info "Checking local testing prerequisites..."
    
    local missing_tools=()
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists docker; then
        missing_tools+=("docker")
    fi
    
    # Check for local cluster tools
    local has_local_cluster=false
    if command_exists kind; then
        has_local_cluster=true
        LOCAL_CLUSTER_TYPE="kind"
    elif command_exists minikube; then
        has_local_cluster=true
        LOCAL_CLUSTER_TYPE="minikube"
    elif command_exists k3d; then
        has_local_cluster=true
        LOCAL_CLUSTER_TYPE="k3d"
    fi
    
    if [ "$has_local_cluster" = false ]; then
        missing_tools+=("kind or minikube or k3d")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install the missing tools:"
        log_info "  kubectl: https://kubernetes.io/docs/tasks/tools/"
        log_info "  docker: https://docs.docker.com/get-docker/"
        log_info "  kind: https://kind.sigs.k8s.io/docs/user/quick-start/"
        log_info "  minikube: https://minikube.sigs.k8s.io/docs/start/"
        exit 1
    fi
    
    log_success "All prerequisites are installed (using $LOCAL_CLUSTER_TYPE)"
}

# Create or verify local cluster
setup_local_cluster() {
    log_info "Setting up local Kubernetes cluster..."
    
    case $LOCAL_CLUSTER_TYPE in
        "kind")
            setup_kind_cluster
            ;;
        "minikube")
            setup_minikube_cluster
            ;;
        "k3d")
            setup_k3d_cluster
            ;;
    esac
}

setup_kind_cluster() {
    local cluster_name="encabulator-test"
    
    if kind get clusters | grep -q "$cluster_name"; then
        log_info "Kind cluster '$cluster_name' already exists"
    else
        log_info "Creating kind cluster '$cluster_name'..."
        
        # Create kind config with extra port mappings
        cat > /tmp/kind-config.yaml << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
    protocol: TCP
  - containerPort: 443
    hostPort: 8443
    protocol: TCP
  - containerPort: 30000
    hostPort: 30000
    protocol: TCP
  - containerPort: 30001
    hostPort: 30001
    protocol: TCP
  - containerPort: 30002
    hostPort: 30002
    protocol: TCP
EOF
        
        kind create cluster --name "$cluster_name" --config /tmp/kind-config.yaml
        rm /tmp/kind-config.yaml
    fi
    
    kubectl config use-context "kind-$cluster_name"
    log_success "Kind cluster ready"
}

setup_minikube_cluster() {
    if minikube status | grep -q "Running"; then
        log_info "Minikube cluster already running"
    else
        log_info "Starting minikube cluster..."
        minikube start --memory=4096 --cpus=2
    fi
    
    kubectl config use-context minikube
    log_success "Minikube cluster ready"
}

setup_k3d_cluster() {
    local cluster_name="encabulator-test"
    
    if k3d cluster list | grep -q "$cluster_name"; then
        log_info "K3d cluster '$cluster_name' already exists"
    else
        log_info "Creating k3d cluster '$cluster_name'..."
        k3d cluster create "$cluster_name" --port "8080:80@loadbalancer" --port "8443:443@loadbalancer"
    fi
    
    kubectl config use-context "k3d-$cluster_name"
    log_success "K3d cluster ready"
}

# Install KRO (simplified for local testing)
install_kro_local() {
    log_info "Installing KRO for local testing..."
    
    # Create KRO namespace
    kubectl create namespace kro-system --dry-run=client -o yaml | kubectl apply -f -
    
    # Apply our KRO installation manifests
    log_info "Applying KRO controller..."
    kubectl apply -f k8s/infrastructure/kro/kro-installation/
    
    # Wait for KRO to be ready
    log_info "Waiting for KRO controller to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/kro-controller -n kro-system
    
    log_success "KRO controller is ready"
}

# Test KRO ResourceGroups
test_kro_resource_groups() {
    log_info "Testing KRO ResourceGroups..."
    
    # Apply observability ResourceGroups
    log_info "Applying observability ResourceGroups..."
    kubectl apply -k k8s/platform/observability/resource-groups/
    
    # Wait for ResourceGroups to be ready
    log_info "Waiting for ResourceGroups to be established..."
    sleep 10
    
    # Check if custom resources are available
    local custom_resources=(
        "observabilitymetrics.observability.encabulator.io"
        "observabilitytracing.observability.encabulator.io"
        "observabilitylogs.observability.encabulator.io"
        "observabilitystack.observability.encabulator.io"
    )
    
    for resource in "${custom_resources[@]}"; do
        if kubectl get crd "$resource" >/dev/null 2>&1; then
            log_success "✓ Custom resource $resource is available"
        else
            log_error "✗ Custom resource $resource is not available"
            return 1
        fi
    done
    
    log_success "All KRO ResourceGroups are ready"
}

# Test observability stack deployment
test_observability_deployment() {
    log_info "Testing observability stack deployment..."
    
    # Create monitoring namespace
    kubectl create namespace encabulator-monitoring --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy development observability stack
    log_info "Deploying development observability stack..."
    kubectl apply -k k8s/platform/observability/instances/
    
    # Wait for deployment
    log_info "Waiting for observability components to be ready..."
    
    # Check for ObservabilityStack resource
    local max_wait=300
    local wait_time=0
    while [ $wait_time -lt $max_wait ]; do
        if kubectl get observabilitystack encabulator-observability -n encabulator-monitoring >/dev/null 2>&1; then
            log_success "ObservabilityStack resource created"
            break
        fi
        sleep 5
        wait_time=$((wait_time + 5))
    done
    
    if [ $wait_time -ge $max_wait ]; then
        log_error "ObservabilityStack resource not created within timeout"
        return 1
    fi
    
    # Wait for underlying resources to be created
    log_info "Waiting for underlying resources to be created by KRO..."
    sleep 30
    
    # Check for key components
    local components=(
        "deployment/encabulator-observability-metrics-prometheus"
        "deployment/encabulator-observability-tracing-jaeger"
        "deployment/encabulator-observability-logs-loki"
        "deployment/encabulator-observability-grafana"
    )
    
    for component in "${components[@]}"; do
        local namespace=""
        case $component in
            *metrics*) namespace="encabulator-monitoring" ;;
            *tracing*) namespace="encabulator-tracing" ;;
            *logs*) namespace="encabulator-logging" ;;
            *grafana*) namespace="encabulator-monitoring" ;;
        esac
        
        if kubectl get "$component" -n "$namespace" >/dev/null 2>&1; then
            log_success "✓ $component exists"
        else
            log_warning "⚠ $component not found (may still be creating)"
        fi
    done
}

# Test service connectivity
test_service_connectivity() {
    log_info "Testing service connectivity..."
    
    # Port forward to test services
    log_info "Setting up port forwards for testing..."
    
    # Test Prometheus
    if kubectl get service encabulator-observability-metrics-prometheus -n encabulator-monitoring >/dev/null 2>&1; then
        kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090 &
        local prom_pid=$!
        sleep 3
        
        if curl -s http://localhost:9090/-/healthy >/dev/null 2>&1; then
            log_success "✓ Prometheus is healthy"
        else
            log_warning "⚠ Prometheus health check failed"
        fi
        
        kill $prom_pid 2>/dev/null || true
    else
        log_warning "⚠ Prometheus service not found"
    fi
    
    # Test Grafana
    if kubectl get service encabulator-observability-grafana -n encabulator-monitoring >/dev/null 2>&1; then
        kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000 &
        local grafana_pid=$!
        sleep 3
        
        if curl -s http://localhost:3000/api/health >/dev/null 2>&1; then
            log_success "✓ Grafana is healthy"
        else
            log_warning "⚠ Grafana health check failed"
        fi
        
        kill $grafana_pid 2>/dev/null || true
    else
        log_warning "⚠ Grafana service not found"
    fi
}

# Show test results and access information
show_test_results() {
    log_success "Local testing completed!"
    log_info ""
    log_info "🎉 Your local Cloud Native Encabulator test environment is ready!"
    log_info ""
    log_info "Cluster: $(kubectl config current-context)"
    log_info ""
    log_info "🔍 Check deployment status:"
    log_info "  kubectl get all -n encabulator-monitoring"
    log_info "  kubectl get all -n encabulator-tracing"
    log_info "  kubectl get all -n encabulator-logging"
    log_info ""
    log_info "🌐 Access services (run in separate terminals):"
    log_info "  Prometheus:  kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090"
    log_info "  Grafana:     kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000"
    log_info "  Jaeger:      kubectl port-forward -n encabulator-tracing svc/encabulator-observability-tracing-jaeger 16686:16686"
    log_info ""
    log_info "📊 Default credentials:"
    log_info "  Grafana: admin / encabulator-dev-2024"
    log_info ""
    log_info "🧹 Cleanup:"
    case $LOCAL_CLUSTER_TYPE in
        "kind") log_info "  kind delete cluster --name encabulator-test" ;;
        "minikube") log_info "  minikube delete" ;;
        "k3d") log_info "  k3d cluster delete encabulator-test" ;;
    esac
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test environment..."
    
    # Kill any background processes
    jobs -p | xargs -r kill 2>/dev/null || true
    
    case "${1:-}" in
        --delete-cluster)
            case $LOCAL_CLUSTER_TYPE in
                "kind") 
                    kind delete cluster --name encabulator-test
                    log_success "Kind cluster deleted"
                    ;;
                "minikube") 
                    minikube delete
                    log_success "Minikube cluster deleted"
                    ;;
                "k3d") 
                    k3d cluster delete encabulator-test
                    log_success "K3d cluster deleted"
                    ;;
            esac
            ;;
        *)
            log_info "Cluster preserved. Use --delete-cluster to remove it."
            ;;
    esac
}

# Main test function
main() {
    log_info "Starting Cloud Native Encabulator local testing..."
    echo ""
    
    check_prerequisites
    setup_local_cluster
    install_kro_local
    test_kro_resource_groups
    test_observability_deployment
    test_service_connectivity
    show_test_results
}

# Handle script arguments and cleanup
trap 'cleanup' EXIT

case "${1:-}" in
    --cleanup)
        cleanup --delete-cluster
        exit 0
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --cleanup        Delete the test cluster and exit"
        echo "  --help, -h       Show this help message"
        echo ""
        echo "Default: Run complete local testing suite"
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