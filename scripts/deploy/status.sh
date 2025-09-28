#!/bin/bash

# Cloud Native Encabulator - Deployment Status Checker
# Comprehensive status checking for the platform deployment

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

# Check cluster connectivity
check_cluster() {
    log_info "=== Cluster Connectivity ==="
    
    if kubectl cluster-info >/dev/null 2>&1; then
        log_success "✓ Cluster is accessible"
        kubectl cluster-info | head -1
        echo "   Context: $(kubectl config current-context)"
    else
        log_error "✗ Cannot access cluster"
        return 1
    fi
    echo ""
}

# Check core infrastructure
check_infrastructure() {
    log_info "=== Core Infrastructure ==="
    
    # Check ArgoCD
    if kubectl get namespace argocd >/dev/null 2>&1; then
        local argocd_status=$(kubectl get deployment argocd-server -n argocd -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null || echo "0/0")
        if [[ "$argocd_status" == *"/"* ]] && [[ "${argocd_status%/*}" -gt 0 ]]; then
            log_success "✓ ArgoCD: $argocd_status ready"
        else
            log_warning "⚠ ArgoCD: $argocd_status ready"
        fi
    else
        log_error "✗ ArgoCD namespace not found"
    fi
    
    # Check ACK Controllers
    if kubectl get namespace ack-system >/dev/null 2>&1; then
        local ack_controllers=$(kubectl get deployments -n ack-system --no-headers 2>/dev/null | wc -l)
        if [ "$ack_controllers" -gt 0 ]; then
            log_success "✓ ACK Controllers: $ack_controllers deployed"
        else
            log_warning "⚠ ACK Controllers: none found"
        fi
    else
        log_warning "⚠ ACK system namespace not found"
    fi
    
    # Check KRO
    if kubectl get namespace kro-system >/dev/null 2>&1; then
        local kro_status=$(kubectl get deployment kro-controller -n kro-system -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null || echo "0/0")
        if [[ "$kro_status" == *"/"* ]] && [[ "${kro_status%/*}" -gt 0 ]]; then
            log_success "✓ KRO Controller: $kro_status ready"
        else
            log_warning "⚠ KRO Controller: $kro_status ready"
        fi
    else
        log_error "✗ KRO system namespace not found"
    fi
    
    echo ""
}

# Check custom resources
check_custom_resources() {
    log_info "=== Custom Resource Definitions ==="
    
    # Infrastructure CRDs
    local infrastructure_crds=(
        "encabulatordatabase.infrastructure.encabulator.io"
        "encabulatorcache.infrastructure.encabulator.io"
        "encabulatorcluster.infrastructure.encabulator.io"
    )
    
    log_info "Infrastructure CRDs:"
    for crd in "${infrastructure_crds[@]}"; do
        if kubectl get crd "$crd" >/dev/null 2>&1; then
            log_success "  ✓ $crd"
        else
            log_warning "  ⚠ $crd (not found)"
        fi
    done
    
    # Observability CRDs
    local observability_crds=(
        "observabilitymetrics.observability.encabulator.io"
        "observabilitytracing.observability.encabulator.io"
        "observabilitylogs.observability.encabulator.io"
        "observabilitystack.observability.encabulator.io"
    )
    
    log_info "Observability CRDs:"
    for crd in "${observability_crds[@]}"; do
        if kubectl get crd "$crd" >/dev/null 2>&1; then
            log_success "  ✓ $crd"
        else
            log_warning "  ⚠ $crd (not found)"
        fi
    done
    
    echo ""
}

# Check observability stack
check_observability() {
    log_info "=== Observability Stack ==="
    
    # Check ObservabilityStack resources
    local observability_stacks=$(kubectl get observabilitystack -A --no-headers 2>/dev/null | wc -l)
    if [ "$observability_stacks" -gt 0 ]; then
        log_success "✓ ObservabilityStack resources: $observability_stacks found"
        kubectl get observabilitystack -A 2>/dev/null | while read -r line; do
            if [[ "$line" != "NAMESPACE"* ]]; then
                echo "    $line"
            fi
        done
    else
        log_warning "⚠ No ObservabilityStack resources found"
    fi
    
    # Check monitoring namespace
    if kubectl get namespace encabulator-monitoring >/dev/null 2>&1; then
        log_info "Monitoring components:"
        
        # Prometheus
        local prom_status=$(kubectl get deployment -n encabulator-monitoring -l app.kubernetes.io/component=prometheus --no-headers 2>/dev/null | wc -l)
        if [ "$prom_status" -gt 0 ]; then
            local prom_ready=$(kubectl get deployment -n encabulator-monitoring -l app.kubernetes.io/component=prometheus -o jsonpath='{.items[0].status.readyReplicas}' 2>/dev/null || echo "0")
            log_success "  ✓ Prometheus: $prom_ready ready"
        else
            log_warning "  ⚠ Prometheus: not found"
        fi
        
        # Grafana
        local grafana_status=$(kubectl get deployment -n encabulator-monitoring -l app.kubernetes.io/component=grafana --no-headers 2>/dev/null | wc -l)
        if [ "$grafana_status" -gt 0 ]; then
            local grafana_ready=$(kubectl get deployment -n encabulator-monitoring -l app.kubernetes.io/component=grafana -o jsonpath='{.items[0].status.readyReplicas}' 2>/dev/null || echo "0")
            log_success "  ✓ Grafana: $grafana_ready ready"
        else
            log_warning "  ⚠ Grafana: not found"
        fi
    else
        log_warning "⚠ Monitoring namespace not found"
    fi
    
    # Check tracing namespace
    if kubectl get namespace encabulator-tracing >/dev/null 2>&1; then
        log_info "Tracing components:"
        
        # Jaeger
        local jaeger_status=$(kubectl get deployment -n encabulator-tracing -l app.kubernetes.io/component=jaeger --no-headers 2>/dev/null | wc -l)
        if [ "$jaeger_status" -gt 0 ]; then
            local jaeger_ready=$(kubectl get deployment -n encabulator-tracing -l app.kubernetes.io/component=jaeger -o jsonpath='{.items[0].status.readyReplicas}' 2>/dev/null || echo "0")
            log_success "  ✓ Jaeger: $jaeger_ready ready"
        else
            log_warning "  ⚠ Jaeger: not found"
        fi
        
        # OpenTelemetry Collector
        local otel_status=$(kubectl get deployment -n encabulator-tracing -l app.kubernetes.io/component=otel-collector --no-headers 2>/dev/null | wc -l)
        if [ "$otel_status" -gt 0 ]; then
            local otel_ready=$(kubectl get deployment -n encabulator-tracing -l app.kubernetes.io/component=otel-collector -o jsonpath='{.items[0].status.readyReplicas}' 2>/dev/null || echo "0")
            log_success "  ✓ OpenTelemetry Collector: $otel_ready ready"
        else
            log_warning "  ⚠ OpenTelemetry Collector: not found"
        fi
    else
        log_warning "⚠ Tracing namespace not found"
    fi
    
    # Check logging namespace
    if kubectl get namespace encabulator-logging >/dev/null 2>&1; then
        log_info "Logging components:"
        
        # Loki
        local loki_status=$(kubectl get deployment -n encabulator-logging -l app.kubernetes.io/component=loki --no-headers 2>/dev/null | wc -l)
        if [ "$loki_status" -gt 0 ]; then
            local loki_ready=$(kubectl get deployment -n encabulator-logging -l app.kubernetes.io/component=loki -o jsonpath='{.items[0].status.readyReplicas}' 2>/dev/null || echo "0")
            log_success "  ✓ Loki: $loki_ready ready"
        else
            log_warning "  ⚠ Loki: not found"
        fi
        
        # Promtail
        local promtail_status=$(kubectl get daemonset -n encabulator-logging -l app.kubernetes.io/component=promtail --no-headers 2>/dev/null | wc -l)
        if [ "$promtail_status" -gt 0 ]; then
            local promtail_ready=$(kubectl get daemonset -n encabulator-logging -l app.kubernetes.io/component=promtail -o jsonpath='{.items[0].status.numberReady}' 2>/dev/null || echo "0")
            local promtail_desired=$(kubectl get daemonset -n encabulator-logging -l app.kubernetes.io/component=promtail -o jsonpath='{.items[0].status.desiredNumberScheduled}' 2>/dev/null || echo "0")
            log_success "  ✓ Promtail: $promtail_ready/$promtail_desired ready"
        else
            log_warning "  ⚠ Promtail: not found"
        fi
    else
        log_warning "⚠ Logging namespace not found"
    fi
    
    echo ""
}

# Check ArgoCD applications
check_argocd_applications() {
    log_info "=== ArgoCD Applications ==="
    
    if kubectl get namespace argocd >/dev/null 2>&1; then
        local app_count=$(kubectl get applications -n argocd --no-headers 2>/dev/null | wc -l)
        if [ "$app_count" -gt 0 ]; then
            log_success "✓ ArgoCD Applications: $app_count found"
            
            # Show application status
            kubectl get applications -n argocd -o custom-columns="NAME:.metadata.name,SYNC:.status.sync.status,HEALTH:.status.health.status" 2>/dev/null | while read -r line; do
                if [[ "$line" == "NAME"* ]]; then
                    echo "    $line"
                else
                    echo "    $line"
                fi
            done
        else
            log_warning "⚠ No ArgoCD applications found"
        fi
    else
        log_warning "⚠ ArgoCD namespace not found"
    fi
    
    echo ""
}

# Check services and endpoints
check_services() {
    log_info "=== Key Services ==="
    
    local namespaces=("encabulator-monitoring" "encabulator-tracing" "encabulator-logging")
    
    for namespace in "${namespaces[@]}"; do
        if kubectl get namespace "$namespace" >/dev/null 2>&1; then
            local service_count=$(kubectl get services -n "$namespace" --no-headers 2>/dev/null | wc -l)
            if [ "$service_count" -gt 0 ]; then
                log_success "✓ $namespace: $service_count services"
                kubectl get services -n "$namespace" --no-headers 2>/dev/null | while read -r line; do
                    echo "    $(echo "$line" | awk '{print $1, $2, $5}')"
                done
            else
                log_warning "⚠ $namespace: no services found"
            fi
        fi
    done
    
    echo ""
}

# Show access commands
show_access_commands() {
    log_info "=== Access Commands ==="
    
    echo "🌐 Port Forward Commands:"
    echo "  ArgoCD UI:   kubectl port-forward -n argocd svc/argocd-server 8080:443"
    echo "  Prometheus:  kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-metrics-prometheus 9090:9090"
    echo "  Grafana:     kubectl port-forward -n encabulator-monitoring svc/encabulator-observability-grafana 3000:3000"
    echo "  Jaeger:      kubectl port-forward -n encabulator-tracing svc/encabulator-observability-tracing-jaeger 16686:16686"
    echo ""
    
    echo "🔑 Default Credentials:"
    echo "  ArgoCD:  admin / (run: kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"
    echo "  Grafana: admin / encabulator-dev-2024"
    echo ""
    
    echo "🔍 Useful Commands:"
    echo "  kubectl get all -n encabulator-monitoring"
    echo "  kubectl get observabilitystack -A"
    echo "  kubectl get applications -n argocd"
    echo "  kubectl logs -n kro-system deployment/kro-controller"
    echo ""
}

# Main status check function
main() {
    log_info "Cloud Native Encabulator - Deployment Status Check"
    echo ""
    
    check_cluster
    check_infrastructure
    check_custom_resources
    check_observability
    check_argocd_applications
    check_services
    show_access_commands
    
    log_success "Status check completed!"
}

# Handle script arguments
case "${1:-}" in
    --observability-only)
        log_info "Checking observability stack only..."
        check_cluster
        check_observability
        ;;
    --infrastructure-only)
        log_info "Checking infrastructure only..."
        check_cluster
        check_infrastructure
        check_custom_resources
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --observability-only  Check only observability components"
        echo "  --infrastructure-only Check only infrastructure components"
        echo "  --help, -h           Show this help message"
        echo ""
        echo "Default: Complete status check"
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