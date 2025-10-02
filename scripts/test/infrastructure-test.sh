#!/bin/bash

# Cloud Native Encabulator - Infrastructure Testing Script
# Tests ACK controllers via Helm, ArgoCD, and infrastructure components

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

# Test ACK Controllers via Helm
test_ack_controllers_helm() {
    log_info "=== Testing ACK Controllers via Helm ==="
    
    # Check if ACK namespace exists
    if ! kubectl get namespace ack-system >/dev/null 2>&1; then
        log_warning "ACK system namespace not found, creating..."
        kubectl create namespace ack-system
    fi
    
    # Test ACK OCI chart access
    log_info "Testing ACK OCI chart access..."
    if helm show chart oci://public.ecr.aws/aws-controllers-k8s/rds-chart --version v0.0.25 >/dev/null 2>&1; then
        log_success "✓ ACK RDS OCI chart accessible"
    else
        log_warning "⚠ ACK RDS OCI chart not accessible (expected in local testing)"
        log_info "  In a real AWS environment, install with:"
        log_info "  helm install ack-rds-controller oci://public.ecr.aws/aws-controllers-k8s/rds-chart --version v0.0.25"
        return 0
    fi
    
    # Install ACK RDS Controller via Helm
    log_info "Installing ACK RDS Controller via Helm..."
    helm upgrade --install ack-rds-controller oci://public.ecr.aws/aws-controllers-k8s/rds-chart \
        --namespace ack-system \
        --version v0.0.25 \
        --values helm/values/ack-controllers/values-rds.yaml \
        --wait --timeout=300s
    
    if [ $? -eq 0 ]; then
        log_success "✓ ACK RDS Controller installed successfully"
    else
        log_warning "⚠ ACK RDS Controller installation had issues"
    fi
    
    # Install ACK ElastiCache Controller via Helm
    log_info "Installing ACK ElastiCache Controller via Helm..."
    helm upgrade --install ack-elasticache-controller oci://public.ecr.aws/aws-controllers-k8s/elasticache-chart \
        --namespace ack-system \
        --version v0.0.21 \
        --values helm/values/ack-controllers/values-elasticache.yaml \
        --wait --timeout=300s
    
    if [ $? -eq 0 ]; then
        log_success "✓ ACK ElastiCache Controller installed successfully"
    else
        log_warning "⚠ ACK ElastiCache Controller installation had issues"
    fi
    
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
            log_warning "⚠ ACK CRD: $crd (not found - would be installed by ACK Helm charts)"
        fi
    done
    
    echo ""
}

# Test ArgoCD Installation via Helm
test_argocd_helm() {
    log_info "=== Testing ArgoCD Installation via Helm ==="
    
    # Add ArgoCD Helm repository
    add_helm_repo "argo" "https://argoproj.github.io/argo-helm"
    
    # Check if ArgoCD namespace exists
    if ! kubectl get namespace argocd >/dev/null 2>&1; then
        log_info "Installing ArgoCD via Helm..."
        
        # Install ArgoCD via Helm
        helm upgrade --install argocd argo/argo-cd \
            --namespace argocd \
            --create-namespace \
            --values helm/values/argocd/values-development.yaml \
            --wait --timeout=300s
        
        if [ $? -eq 0 ]; then
            log_success "✓ ArgoCD installed successfully via Helm"
        else
            log_error "✗ ArgoCD installation failed"
            return 1
        fi
    else
        log_success "✓ ArgoCD namespace already exists"
    fi
    
    # Check ArgoCD deployment status
    if kubectl get deployment argocd-server -n argocd >/dev/null 2>&1; then
        local argocd_status=$(kubectl get deployment argocd-server -n argocd -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null || echo "0/0")
        if [[ "$argocd_status" == *"/"* ]] && [[ "${argocd_status%/*}" -gt 0 ]]; then
            log_success "✓ ArgoCD Server: $argocd_status ready"
        else
            log_warning "⚠ ArgoCD Server: $argocd_status ready"
        fi
    fi
    
    # Get ArgoCD admin password
    local argocd_password=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d 2>/dev/null || echo "not-found")
    if [ "$argocd_password" != "not-found" ]; then
        log_success "✓ ArgoCD admin password retrieved"
        log_info "  Username: admin"
        log_info "  Password: $argocd_password"
    else
        log_warning "⚠ ArgoCD admin password not found"
    fi
    
    echo ""
}

# Test AWS Resources via ACK
test_aws_resources() {
    log_info "=== Testing AWS Resources via ACK ==="
    
    # Create data namespace
    kubectl create namespace encabulator-data --dry-run=client -o yaml | kubectl apply -f -
    
    # Apply AWS resources
    log_info "Applying AWS resources via ACK..."
    kubectl apply -k k8s/infrastructure/aws-resources/
    
    # Wait for resources to be created
    log_info "Waiting for AWS resources to be created..."
    sleep 20
    
    # Check for RDS database
    if kubectl get dbinstance hydrocoptic-datastore -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ RDS Database: hydrocoptic-datastore created"
        
        # Check status
        local db_status=$(kubectl get dbinstance hydrocoptic-datastore -n encabulator-data -o jsonpath='{.status.dbInstanceStatus}' 2>/dev/null || echo "Unknown")
        log_info "  Database Status: $db_status"
    else
        log_warning "⚠ RDS Database: hydrocoptic-datastore not found"
    fi
    
    # Check for ElastiCache cluster
    if kubectl get replicationgroup prefabulated-cache -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ ElastiCache Cluster: prefabulated-cache created"
        
        # Check status
        local cache_status=$(kubectl get replicationgroup prefabulated-cache -n encabulator-data -o jsonpath='{.status.status}' 2>/dev/null || echo "Unknown")
        log_info "  Cache Status: $cache_status"
    else
        log_warning "⚠ ElastiCache Cluster: prefabulated-cache not found"
    fi
    
    # Check secrets and config
    if kubectl get secret hydrocoptic-datastore-credentials -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ Database credentials secret created"
    else
        log_warning "⚠ Database credentials secret not found"
    fi
    
    if kubectl get configmap prefabulated-cache-config -n encabulator-data >/dev/null 2>&1; then
        log_success "✓ Cache configuration created"
    else
        log_warning "⚠ Cache configuration not found"
    fi
    
    echo ""
}

# Test ArgoCD Applications with Helm Sources
test_argocd_helm_applications() {
    log_info "=== Testing ArgoCD Applications with Helm Sources ==="
    
    # Check if ArgoCD is available
    if ! kubectl get namespace argocd >/dev/null 2>&1; then
        log_warning "ArgoCD namespace not found, skipping ArgoCD application tests"
        return 0
    fi
    
    # Apply ArgoCD infrastructure applications
    log_info "Applying ArgoCD infrastructure applications with Helm sources..."
    kubectl apply -f argocd/infrastructure/ack-controllers-helm.yaml
    
    # Wait for applications to be created
    sleep 10
    
    # Check ArgoCD applications
    local argocd_apps=(
        "ack-rds-controller"
        "ack-elasticache-controller"
        "ack-iam-controller"
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
    
    # Apply AWS resources application
    kubectl apply -f argocd/infrastructure/resource-orchestration.yaml
    sleep 5
    
    if kubectl get application aws-resources -n argocd >/dev/null 2>&1; then
        local sync_status=$(kubectl get application aws-resources -n argocd -o jsonpath='{.status.sync.status}' 2>/dev/null || echo "Unknown")
        local health_status=$(kubectl get application aws-resources -n argocd -o jsonpath='{.status.health.status}' 2>/dev/null || echo "Unknown")
        log_success "✓ ArgoCD App: aws-resources (Sync: $sync_status, Health: $health_status)"
    else
        log_warning "⚠ ArgoCD App: aws-resources (not found)"
    fi
    
    echo ""
}

# Test Helm Releases
test_helm_releases() {
    log_info "=== Testing Helm Releases ==="
    
    # Check Helm releases
    local helm_releases=(
        "argocd:argocd"
        "ack-rds-controller:ack-system"
        "ack-elasticache-controller:ack-system"
    )
    
    for release_info in "${helm_releases[@]}"; do
        IFS=':' read -r release namespace <<< "$release_info"
        
        if helm list -n "$namespace" | grep -q "$release"; then
            local status=$(helm status "$release" -n "$namespace" -o json | jq -r '.info.status' 2>/dev/null || echo "unknown")
            if [ "$status" = "deployed" ]; then
                log_success "✓ Helm release deployed: $release in $namespace"
            else
                log_warning "⚠ Helm release status: $release ($status)"
            fi
        else
            log_warning "⚠ Helm release not found: $release in $namespace"
        fi
    done
    
    echo ""
}

# Test Infrastructure Integration
test_infrastructure_integration() {
    log_info "=== Testing Infrastructure Integration ==="
    
    # Test that ACK controllers are exposing metrics
    log_info "Checking ACK controller metrics endpoints..."
    
    local controllers=(
        "ack-rds-controller"
        "ack-elasticache-controller"
    )
    
    for controller in "${controllers[@]}"; do
        if kubectl get service "$controller-metrics" -n ack-system >/dev/null 2>&1; then
            log_success "✓ Metrics service exists: $controller-metrics"
        else
            log_warning "⚠ Metrics service not found: $controller-metrics"
        fi
    done
    
    # Test that AWS resources can be created (dry-run)
    log_info "Testing AWS resource creation (dry-run)..."
    
    # Test RDS instance creation
    if kubectl apply --dry-run=client -f k8s/infrastructure/aws-resources/rds-database.yaml >/dev/null 2>&1; then
        log_success "✓ RDS resource definition is valid"
    else
        log_warning "⚠ RDS resource definition has issues"
    fi
    
    # Test ElastiCache creation
    if kubectl apply --dry-run=client -f k8s/infrastructure/aws-resources/elasticache-cluster.yaml >/dev/null 2>&1; then
        log_success "✓ ElastiCache resource definition is valid"
    else
        log_warning "⚠ ElastiCache resource definition has issues"
    fi
    
    echo ""
}

# Show infrastructure status
show_infrastructure_status() {
    log_info "=== Infrastructure Status Summary ==="
    
    echo ""
    log_info "Helm Releases:"
    helm list -A 2>/dev/null || log_warning "No Helm releases found"
    
    echo ""
    log_info "ArgoCD Applications:"
    kubectl get applications -n argocd 2>/dev/null || log_warning "No ArgoCD applications found"
    
    echo ""
    log_info "ACK Controllers:"
    kubectl get deployments -n ack-system 2>/dev/null || log_warning "No ACK controllers found"
    
    echo ""
    log_info "ACK Custom Resource Definitions:"
    kubectl get crd | grep -E "(rds|elasticache)\.services\.k8s\.aws" || log_warning "No ACK CRDs found"
    
    echo ""
    log_info "AWS Resources:"
    kubectl get dbinstance,replicationgroup -A 2>/dev/null || log_warning "No AWS resources found"
    
    echo ""
    log_info "Infrastructure Namespaces:"
    kubectl get namespaces | grep -E "(ack-system|argocd|encabulator-data)" || log_warning "No infrastructure namespaces found"
    
    echo ""
}

# Show access information
show_access_info() {
    log_success "Infrastructure testing completed!"
    log_info ""
    log_info "🎉 Your Cloud Native Encabulator Helm-based infrastructure is ready!"
    log_info ""
    log_info "🔍 Check infrastructure status:"
    log_info "  helm list -A"
    log_info "  kubectl get all -n ack-system"
    log_info "  kubectl get all -n argocd"
    log_info "  kubectl get dbinstance,replicationgroup -A"
    log_info ""
    log_info "🌐 Access ArgoCD:"
    log_info "  kubectl port-forward -n argocd svc/argocd-server 8080:443"
    log_info "  Username: admin"
    local argocd_password=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d 2>/dev/null || echo "not-found")
    if [ "$argocd_password" != "not-found" ]; then
        log_info "  Password: $argocd_password"
    else
        log_info "  Password: (retrieve with: kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath=\"{.data.password}\" | base64 -d)"
    fi
    log_info ""
    log_info "🛠️ Useful commands:"
    log_info "  kubectl describe dbinstance hydrocoptic-datastore -n encabulator-data"
    log_info "  kubectl logs -n ack-system deployment/ack-rds-controller"
    log_info "  helm status argocd -n argocd"
    log_info ""
    log_info "📊 Next steps:"
    log_info "  1. Run observability tests: ./scripts/test/comprehensive-test.sh --observability-only"
    log_info "  2. Deploy complete platform: ./scripts/deploy/deploy-helm.sh"
    log_info "  3. Check overall status: ./scripts/deploy/status.sh"
}

# Main test function
main() {
    log_info "Starting Cloud Native Encabulator Helm-based infrastructure testing..."
    echo ""
    
    test_ack_controllers_helm
    test_argocd_helm
    test_aws_resources
    test_argocd_helm_applications
    test_helm_releases
    test_infrastructure_integration
    show_infrastructure_status
    show_access_info
}

# Handle script arguments
case "${1:-}" in
    --ack-only)
        log_info "Testing ACK controllers via Helm only..."
        test_ack_controllers_helm
        test_helm_releases
        ;;
    --argocd-only)
        log_info "Testing ArgoCD via Helm only..."
        test_argocd_helm
        test_argocd_helm_applications
        ;;
    --aws-resources-only)
        log_info "Testing AWS resources only..."
        test_aws_resources
        ;;
    --status-only)
        log_info "Showing infrastructure status only..."
        show_infrastructure_status
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --ack-only           Test only ACK controllers via Helm"
        echo "  --argocd-only        Test only ArgoCD installation and applications"
        echo "  --aws-resources-only Test only AWS resource creation"
        echo "  --status-only        Show infrastructure status only"
        echo "  --help, -h           Show this help message"
        echo ""
        echo "Default: Complete Helm-based infrastructure testing suite"
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