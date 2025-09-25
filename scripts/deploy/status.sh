#!/bin/bash

# Cloud Native Encabulator Microservices Demo - Status Check Script
# This script checks the status of the deployed services

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
check_cluster() {
    if ! command -v kubectl >/dev/null 2>&1; then
        log_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot access Kubernetes cluster"
        exit 1
    fi
    
    local cluster_info=$(kubectl cluster-info | head -1)
    log_success "Connected to: $cluster_info"
}

# Check namespace status
check_namespaces() {
    log_info "Checking namespaces..."
    echo ""
    kubectl get namespaces -l app.kubernetes.io/name=cloud-native-encabulator -o wide
    echo ""
}

# Check pod status
check_pods() {
    log_info "Checking pod status..."
    echo ""
    
    local namespaces=("encabulator-system" "encabulator-data" "encabulator-monitoring")
    
    for namespace in "${namespaces[@]}"; do
        echo "=== $namespace ==="
        kubectl get pods -n $namespace -o wide 2>/dev/null || echo "No pods in $namespace"
        echo ""
    done
}

# Check service status
check_services() {
    log_info "Checking services..."
    echo ""
    
    local namespaces=("encabulator-system" "encabulator-data" "encabulator-monitoring")
    
    for namespace in "${namespaces[@]}"; do
        echo "=== $namespace ==="
        kubectl get services -n $namespace -o wide 2>/dev/null || echo "No services in $namespace"
        echo ""
    done
}

# Check ingress status
check_ingress() {
    log_info "Checking ingress..."
    echo ""
    kubectl get ingress --all-namespaces -o wide
    echo ""
}

# Check persistent volumes
check_storage() {
    log_info "Checking storage..."
    echo ""
    echo "=== Persistent Volumes ==="
    kubectl get pv -o wide
    echo ""
    echo "=== Persistent Volume Claims ==="
    kubectl get pvc --all-namespaces -o wide
    echo ""
}

# Show resource usage
show_resource_usage() {
    log_info "Resource usage..."
    echo ""
    
    if kubectl top nodes >/dev/null 2>&1; then
        echo "=== Node Resource Usage ==="
        kubectl top nodes
        echo ""
        
        echo "=== Pod Resource Usage ==="
        kubectl top pods --all-namespaces --sort-by=cpu
        echo ""
    else
        log_warning "Metrics server not available - cannot show resource usage"
    fi
}

# Show access URLs
show_access_info() {
    log_info "Access Information..."
    echo ""
    
    # Check if ingress controller is running
    if kubectl get pods -n ingress-nginx -l app.kubernetes.io/component=controller --no-headers 2>/dev/null | grep -q Running; then
        log_success "Ingress controller is running"
        echo ""
        echo "🌐 Access URLs:"
        echo "  - Grafana:    http://localhost:8080/grafana (admin/encabulator_admin)"
        echo "  - Prometheus: http://localhost:8080/prometheus"
        echo "  - Jaeger:     http://localhost:8080/jaeger"
        echo ""
    else
        log_warning "Ingress controller not running - services not accessible via HTTP"
        echo ""
        echo "🔧 Port forward commands:"
        echo "  - Grafana:    kubectl port-forward -n encabulator-monitoring svc/grafana 3000:3000"
        echo "  - Prometheus: kubectl port-forward -n encabulator-monitoring svc/prometheus 9090:9090"
        echo "  - Jaeger:     kubectl port-forward -n encabulator-monitoring svc/jaeger 16686:16686"
        echo ""
    fi
}

# Show logs for failed pods
show_failed_pods() {
    log_info "Checking for failed pods..."
    echo ""
    
    local failed_pods=$(kubectl get pods --all-namespaces --field-selector=status.phase!=Running,status.phase!=Succeeded --no-headers 2>/dev/null)
    
    if [ -n "$failed_pods" ]; then
        log_warning "Found failed pods:"
        echo "$failed_pods"
        echo ""
        log_info "Use 'kubectl describe pod <pod-name> -n <namespace>' for more details"
        log_info "Use 'kubectl logs <pod-name> -n <namespace>' to view logs"
    else
        log_success "All pods are running successfully"
    fi
    echo ""
}

# Main status check function
main() {
    echo "🔍 Cloud Native Encabulator Microservices Demo - Status Check"
    echo "=================================================="
    echo ""
    
    check_cluster
    echo ""
    
    check_namespaces
    check_pods
    check_services
    check_ingress
    check_storage
    show_resource_usage
    show_failed_pods
    show_access_info
    
    log_success "Status check completed!"
}

# Handle script arguments
case "${1:-}" in
    --pods)
        check_cluster
        check_pods
        ;;
    --services)
        check_cluster
        check_services
        ;;
    --ingress)
        check_cluster
        check_ingress
        ;;
    --storage)
        check_cluster
        check_storage
        ;;
    --resources)
        check_cluster
        show_resource_usage
        ;;
    --failed)
        check_cluster
        show_failed_pods
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --pods       Show only pod status"
        echo "  --services   Show only service status"
        echo "  --ingress    Show only ingress status"
        echo "  --storage    Show only storage status"
        echo "  --resources  Show only resource usage"
        echo "  --failed     Show only failed pods"
        echo "  --help, -h   Show this help message"
        echo ""
        echo "Default: Show complete status overview"
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