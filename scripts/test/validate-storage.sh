#!/bin/bash

# Cloud Native Encabulator - Storage Validation Script
# Validates storage class configuration and PVC readiness

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# Check storage classes
check_storage_classes() {
    log_info "=== Checking Storage Classes ==="
    
    if ! kubectl get storageclass >/dev/null 2>&1; then
        log_error "Cannot access storage classes"
        return 1
    fi
    
    local default_sc=$(kubectl get storageclass -o jsonpath='{.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}')
    
    if [ -n "$default_sc" ]; then
        log_success "Default storage class found: $default_sc"
    else
        log_warning "No default storage class found"
        log_info "Available storage classes:"
        kubectl get storageclass
    fi
    
    echo ""
}

# Check existing PVCs
check_existing_pvcs() {
    log_info "=== Checking Existing PVCs ==="
    
    local namespaces=("encabulator-monitoring" "encabulator-data")
    
    for namespace in "${namespaces[@]}"; do
        if kubectl get namespace "$namespace" >/dev/null 2>&1; then
            log_info "Checking PVCs in namespace: $namespace"
            
            local pvcs=$(kubectl get pvc -n "$namespace" --no-headers 2>/dev/null | wc -l)
            if [ "$pvcs" -gt 0 ]; then
                kubectl get pvc -n "$namespace"
                
                # Check for pending PVCs
                local pending=$(kubectl get pvc -n "$namespace" --no-headers 2>/dev/null | grep -c "Pending" || echo "0")
                if [ "$pending" -gt 0 ]; then
                    log_warning "$pending PVC(s) in Pending state in $namespace"
                    
                    # Show events for pending PVCs
                    kubectl get events -n "$namespace" --field-selector reason=ProvisioningFailed,reason=ProvisioningSucceeded --sort-by='.lastTimestamp' | tail -5
                else
                    log_success "All PVCs in $namespace are bound"
                fi
            else
                log_info "No PVCs found in $namespace"
            fi
        else
            log_info "Namespace $namespace does not exist"
        fi
        echo ""
    done
}

# Test PVC creation
test_pvc_creation() {
    log_info "=== Testing PVC Creation ==="
    
    local test_pvc_name="test-pvc-$(date +%s)"
    
    # Create test PVC
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: $test_pvc_name
  namespace: default
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF

    log_info "Created test PVC: $test_pvc_name"
    
    # Wait for PVC to be bound
    local max_wait=60
    local wait_time=0
    
    while [ $wait_time -lt $max_wait ]; do
        local status=$(kubectl get pvc "$test_pvc_name" -n default -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        
        if [ "$status" = "Bound" ]; then
            log_success "Test PVC bound successfully in ${wait_time}s"
            break
        elif [ "$status" = "Pending" ]; then
            log_info "Test PVC still pending... (${wait_time}s/${max_wait}s)"
        else
            log_warning "Test PVC status: $status"
        fi
        
        sleep 5
        wait_time=$((wait_time + 5))
    done
    
    if [ $wait_time -ge $max_wait ]; then
        log_error "Test PVC failed to bind within ${max_wait}s"
        
        # Show events
        log_info "Recent events:"
        kubectl describe pvc "$test_pvc_name" -n default | tail -10
    fi
    
    # Cleanup test PVC
    kubectl delete pvc "$test_pvc_name" -n default >/dev/null 2>&1 || true
    log_info "Cleaned up test PVC"
    
    echo ""
}

# Show recommendations
show_recommendations() {
    log_info "=== Storage Configuration Recommendations ==="
    
    local default_sc=$(kubectl get storageclass -o jsonpath='{.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}')
    
    if [ -n "$default_sc" ]; then
        log_success "✓ Default storage class available: $default_sc"
        log_info "Recommendation: Use default storage class (no explicit storageClassName)"
        log_info "Current Helm values configuration is optimal for your cluster"
    else
        log_warning "⚠ No default storage class found"
        log_info "Recommendation: Either set a default storage class or explicitly specify one"
        log_info "To set default storage class:"
        log_info "  kubectl patch storageclass <your-storage-class> -p '{\"metadata\": {\"annotations\":{\"storageclass.kubernetes.io/is-default-class\":\"true\"}}}'"
    fi
    
    echo ""
    log_info "Current Helm values configuration:"
    log_info "  - Grafana: persistence enabled, uses default storage class"
    log_info "  - Prometheus: persistence enabled, uses default storage class"
    log_info "  - AlertManager: persistence enabled, uses default storage class"
    
    echo ""
    log_info "If PVC issues persist, you can disable persistence by editing:"
    log_info "  helm/values/observability/values-prometheus.yaml"
}

# Main function
main() {
    log_info "Cloud Native Encabulator - Storage Validation"
    echo ""
    
    check_storage_classes
    check_existing_pvcs
    test_pvc_creation
    show_recommendations
}

# Handle arguments
case "${1:-}" in
    --storage-classes-only)
        check_storage_classes
        ;;
    --pvcs-only)
        check_existing_pvcs
        ;;
    --test-only)
        test_pvc_creation
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --storage-classes-only  Check only storage classes"
        echo "  --pvcs-only            Check only existing PVCs"
        echo "  --test-only            Test PVC creation only"
        echo "  --help, -h             Show this help message"
        echo ""
        echo "Default: Complete storage validation"
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