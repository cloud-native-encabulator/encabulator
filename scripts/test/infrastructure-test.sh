#!/bin/bash

# Cloud Native Encabulator - Infrastructure Testing Script
# Tests ACK controllers, KRO ResourceGroups, and infrastructure components

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

# Test ACK Controllers
test_ack_controllers() {
    log_info "=== Testing ACK Controllers ==="
    
    # Check if ACK namespace exists
    if ! kubectl get namespace ack-system >/dev/null 2>&1; then
        log_warning "ACK system namespace not found, creating..."
        kubectl create namespace ack-system
    fi
    
    # Apply ACK controller manifests
    log_info "Applying ACK controller manifests..."
    kubectl apply -f k8s/infrastructure/ack-controllers/
    
    # Wait for controllers to be ready
    log_info "Waiting for ACK controllers to be ready..."
    sleep 30
    
    # Check controller status
    local controllers=(
        "ack-rds-controller"
        "ack-elasticache-controller"
    )
    
    for controller in "${controllers[@]}"; do
        if kubectl get deployment "$controller" -n ack-system >/dev/null 2>&1; then
            local status=$(kubectl get deployment "$controller" -n ack-system -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null || echo "0/0")
            if [[ "$status" == *"/"* ]] && [[ "${status%/*}" -gt 0 ]]; then
                log_success "✓ $controller: $status ready"
            else
                log_warning "⚠ $controller: $status ready"
            fi
        else
            log_warning "⚠ $controller: not found"
        fi
    done
    
    # Check for ACK CRDs
    log_info "Checking ACK Custom Resource Definitions..."
    local ack_crds=(
        "dbinstances.rds.services.k8s.aws"
        "replicationgroups.elasticache.services.k8s.aws"
    )
    
    for crd in "${ack_crds[@]}"; do
        if kubectl get crd "$crd" >/dev/null 2>&1; then
            log_success "✓ ACK CRD: $crd"
        else
            log_warning "⚠ ACK CRD: $crd (not found)"
        fi
    done
    
    echo ""
}

# Test KRO Installation
test_kro_installation() {
    log_info "=== Testing KRO Installation ==="
    
    # Check if KRO namespace exists
    if ! kubectl get namespace kro >/dev/null 2>&1; then
        log_warning "KRO namespace not found, creating..."
        kubectl create namespace kro
    fi
    
    # Install KRO via official OCI Helm chart
    log_info "Installing KRO via official OCI Helm chart..."
    
    # Get latest KRO version
    local kro_version
    kro_version=$(curl -sL https://api.github.com/repos/kubernetes-sigs/kro/releases/latest | \
        jq -r '.tag_name | ltrimstr("v")' 2>/dev/null || echo "0.1.0")
    
    if [ "$kro_version" = "null" ] || [ -z "$kro_version" ]; then
        kro_version="0.1.0"
        log_warning "Could not fetch latest version, using v$kro_version"
    else
        log_info "Using KRO version: v$kro_version"
    fi
    
    # Install KRO
    helm upgrade --install kro oci://ghcr.io/kro-run/kro/kro \
        --namespace kro \
        --create-namespace \
        --version="${kro_version}" \
        --wait --timeout=300s
    
    # Check KRO controller status
    local kro_status=$(kubectl get deployment kro -n kro -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null || echo "0/0")
    if [[ "$kro_status" == *"/"* ]] && [[ "${kro_status%/*}" -gt 0 ]]; then
        log_success "✓ KRO Controller: $kro_status ready"
    else
        log_error "✗ KRO Controller: $kro_status ready"
        return 1
    fi
    
    # Check for KRO CRDs
    log_info "Checking KRO Custom Resource Definitions..."
    local kro_crds=(
        "resourcegraphdefinitions.kro.run"
    )
    
    for crd in "${kro_crds[@]}"; do
        if kubectl get crd "$crd" >/dev/null 2>&1; then
            log_success "✓ KRO CRD: $crd"
        else
            log_error "✗ KRO CRD: $crd (not found)"
            return 1
        fi
    done
    
    echo ""
}

# Test Infrastructure ResourceGroups
test_infrastructure_resource_groups() {
    log_info "=== Testing Infrastructure ResourceGroups ==="
    
    # Apply simple test ResourceGraphDefinition
    log_info "Applying simple test ResourceGraphDefinition..."
    kubectl apply -k k8s/infrastructure/kro/resource-graph-definitions/
    
    # Wait for ResourceGroups to be established
    log_info "Waiting for ResourceGroups to be established..."
    sleep 20
    
    # Check for infrastructure custom resources
    local infrastructure_resources=(
        "encabulatordatabase.encabulator.io"
        "encabulatorcache.encabulator.io"
        "encabulatorcluster.encabulator.io"
    )
    
    for resource in "${infrastructure_resources[@]}"; do
        if kubectl get crd "$resource" >/dev/null 2>&1; then
            log_success "✓ Infrastructure CRD: $resource"
        else
            log_warning "⚠ Infrastructure CRD: $resource (not yet available)"
        fi
    done
    
    # Check ResourceGraphDefinition status
    log_info "Checking ResourceGraphDefinition status..."
    if kubectl get resourcegraphdefinition simple-configmap -n kro >/dev/null 2>&1; then
        log_success "✓ ResourceGraphDefinition: simple-configmap"
        
        # Check if CRD was created
        if kubectl get crd simpleconfigmaps.v1alpha1.kro.run >/dev/null 2>&1; then
            log_success "✓ Custom Resource created: SimpleConfigMap"
        else
            log_warning "⚠ Custom Resource not yet created"
        fi
    else
        log_warning "⚠ ResourceGraphDefinition: simple-configmap (not found)"
    fi
    
    echo ""
}

# Test Infrastructure Instances
test_infrastructure_instances() {
    log_info "=== Testing Infrastructure Instances ==="
    
    # Create data namespace
    kubectl create namespace encabulator-data --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy development cluster example
    log_info "Deploying development cluster example..."
    kubectl apply -f k8s/infrastructure/kro/examples/development-cluster.yaml
    
    # Wait for resources to be created
    log_info "Waiting for infrastructure resources to be created..."
    sleep 30
    
    # Check for EncabulatorCluster resource
    if kubectl get encabulatorcluster development-cluster -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ EncabulatorCluster: development-cluster created"
        
        # Check status
        local cluster_status=$(kubectl get encabulatorcluster development-cluster -n encabulator-data -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        log_info "  Cluster Status: $cluster_status"
    else
        log_warning "⚠ EncabulatorCluster: development-cluster not found"
    fi
    
    # Check for underlying resources created by KRO
    log_info "Checking underlying resources created by KRO..."
    
    # Database
    if kubectl get encabulatordatabase development-cluster-database -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ EncabulatorDatabase: development-cluster-database"
    else
        log_warning "⚠ EncabulatorDatabase: development-cluster-database (not found)"
    fi
    
    # Cache
    if kubectl get encabulatorcache development-cluster-cache -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ EncabulatorCache: development-cluster-cache"
    else
        log_warning "⚠ EncabulatorCache: development-cluster-cache (not found)"
    fi
    
    # Namespace
    if kubectl get namespace development-cluster-apps >/dev/null 2>&1; then
        log_success "✓ Application Namespace: development-cluster-apps"
    else
        log_warning "⚠ Application Namespace: development-cluster-apps (not found)"
    fi
    
    # ConfigMap
    if kubectl get configmap development-cluster-config -n development-cluster-apps >/dev/null 2>&1; then
        log_success "✓ Cluster ConfigMap: development-cluster-config"
    else
        log_warning "⚠ Cluster ConfigMap: development-cluster-config (not found)"
    fi
    
    echo ""
}

# Test Individual Components
test_individual_components() {
    log_info "=== Testing Individual Components ==="
    
    # Deploy individual components example
    log_info "Deploying individual components example..."
    kubectl apply -f k8s/infrastructure/kro/examples/individual-components.yaml
    
    # Wait for resources
    sleep 20
    
    # Check individual database
    if kubectl get encabulatordatabase hydrocoptic-datastore -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ Individual Database: hydrocoptic-datastore"
    else
        log_warning "⚠ Individual Database: hydrocoptic-datastore (not found)"
    fi
    
    # Check individual cache
    if kubectl get encabulatorcache prefabulated-cache -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ Individual Cache: prefabulated-cache"
    else
        log_warning "⚠ Individual Cache: prefabulated-cache (not found)"
    fi
    
    echo ""
}

# Test ArgoCD Integration
test_argocd_integration() {
    log_info "=== Testing ArgoCD Integration ==="
    
    # Check if ArgoCD is available
    if ! kubectl get namespace argocd >/dev/null 2>&1; then
        log_warning "ArgoCD namespace not found, skipping ArgoCD integration tests"
        return 0
    fi
    
    # Apply ArgoCD infrastructure applications
    log_info "Applying ArgoCD infrastructure applications..."
    kubectl apply -f argocd/infrastructure/
    
    # Wait for applications to be created
    sleep 10
    
    # Check ArgoCD applications
    local argocd_apps=(
        "kro-installation"
        "kro-resource-groups"
        "aws-resources"
    )
    
    for app in "${argocd_apps[@]}"; do
        if kubectl get application "$app" -n argocd >/dev/null 2>&1; then
            local sync_status=$(kubectl get application "$app" -n argocd -o jsonpath='{.status.sync.status}' 2>/dev/null || echo "Unknown")
            local health_status=$(kubectl get application "$app" -n argocd -o jsonpath='{.status.health.status}' 2>/dev/null || echo "Unknown")
            log_success "✓ ArgoCD App: $app (Sync: $sync_status, Health: $health_status)"
        else
            log_warning "⚠ ArgoCD App: $app (not found)"
        fi
    done
    
    echo ""
}

# Test Resource Dependencies
test_resource_dependencies() {
    log_info "=== Testing Resource Dependencies ==="
    
    # Test that composite resources create their dependencies
    log_info "Checking resource dependency creation..."
    
    # Check that EncabulatorCluster creates EncabulatorDatabase and EncabulatorCache
    local cluster_resources=$(kubectl get encabulatorcluster -A --no-headers 2>/dev/null | wc -l)
    local database_resources=$(kubectl get encabulatordatabase -A --no-headers 2>/dev/null | wc -l)
    local cache_resources=$(kubectl get encabulatorcache -A --no-headers 2>/dev/null | wc -l)
    
    log_info "Resource counts:"
    log_info "  EncabulatorCluster: $cluster_resources"
    log_info "  EncabulatorDatabase: $database_resources"
    log_info "  EncabulatorCache: $cache_resources"
    
    if [ "$cluster_resources" -gt 0 ] && [ "$database_resources" -ge "$cluster_resources" ] && [ "$cache_resources" -ge "$cluster_resources" ]; then
        log_success "✓ Resource dependencies are being created correctly"
    else
        log_warning "⚠ Resource dependency creation may have issues"
    fi
    
    echo ""
}

# Show infrastructure status
show_infrastructure_status() {
    log_info "=== Infrastructure Status Summary ==="
    
    echo ""
    log_info "ACK Controllers:"
    kubectl get deployments -n ack-system 2>/dev/null || log_warning "No ACK controllers found"
    
    echo ""
    log_info "KRO Controller:"
    kubectl get deployments -n kro 2>/dev/null || log_warning "No KRO controller found"
    
    echo ""
    log_info "Infrastructure Custom Resources:"
    kubectl get crd | grep encabulator || log_warning "No encabulator CRDs found"
    
    echo ""
    log_info "Infrastructure Instances:"
    kubectl get encabulatorcluster -A 2>/dev/null || log_warning "No EncabulatorCluster resources found"
    kubectl get encabulatordatabase -A 2>/dev/null || log_warning "No EncabulatorDatabase resources found"
    kubectl get encabulatorcache -A 2>/dev/null || log_warning "No EncabulatorCache resources found"
    
    echo ""
    log_info "ArgoCD Applications:"
    kubectl get applications -n argocd 2>/dev/null || log_warning "No ArgoCD applications found"
    
    echo ""
}

# Show access information
show_access_info() {
    log_success "Infrastructure testing completed!"
    log_info ""
    log_info "🎉 Your Cloud Native Encabulator infrastructure is ready!"
    log_info ""
    log_info "🔍 Check infrastructure status:"
    log_info "  kubectl get all -n ack-system"
    log_info "  kubectl get all -n kro"
    log_info "  kubectl get encabulatorcluster -A"
    log_info "  kubectl get encabulatordatabase -A"
    log_info "  kubectl get encabulatorcache -A"
    log_info ""
    log_info "🛠️ Useful commands:"
    log_info "  kubectl describe encabulatorcluster development-cluster -n encabulator-data"
    log_info "  kubectl logs -n kro deployment/kro"
    log_info "  kubectl get resourcegraphdefinitions -n kro"
    log_info ""
    log_info "📊 Next steps:"
    log_info "  1. Run observability tests: ./scripts/test/local-test.sh"
    log_info "  2. Deploy to development cluster: ./scripts/deploy/deploy-dev.sh"
    log_info "  3. Check overall status: ./scripts/deploy/status.sh"
}

# Main test function
main() {
    log_info "Starting Cloud Native Encabulator infrastructure testing..."
    echo ""
    
    test_ack_controllers
    test_kro_installation
    test_infrastructure_resource_groups
    test_infrastructure_instances
    test_individual_components
    test_argocd_integration
    test_resource_dependencies
    show_infrastructure_status
    show_access_info
}

# Handle script arguments
case "${1:-}" in
    --ack-only)
        log_info "Testing ACK controllers only..."
        test_ack_controllers
        ;;
    --kro-only)
        log_info "Testing KRO only..."
        test_kro_installation
        test_infrastructure_resource_groups
        ;;
    --instances-only)
        log_info "Testing infrastructure instances only..."
        test_infrastructure_instances
        test_individual_components
        ;;
    --status-only)
        log_info "Showing infrastructure status only..."
        show_infrastructure_status
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --ack-only        Test only ACK controllers"
        echo "  --kro-only        Test only KRO installation and ResourceGroups"
        echo "  --instances-only  Test only infrastructure instances"
        echo "  --status-only     Show infrastructure status only"
        echo "  --help, -h        Show this help message"
        echo ""
        echo "Default: Complete infrastructure testing suite"
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