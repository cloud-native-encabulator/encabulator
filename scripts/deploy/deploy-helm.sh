#!/bin/bash

# Cloud Native Encabulator - Helm-Based Deployment Script
# Deploys the complete platform using Helm charts and ArgoCD

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2${NC}"
    else
        echo -e "${RED}❌ $2${NC}"
        return 1
    fi
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_header() {
    echo -e "${BLUE}"
    echo "=============================================="
    echo "$1"
    echo "=============================================="
    echo -e "${NC}"
}

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        print_status 1 "kubectl is required but not installed"
        exit 1
    fi
    print_status 0 "kubectl found"
    
    # Check helm
    if ! command -v helm &> /dev/null; then
        print_status 1 "helm is required but not installed"
        exit 1
    fi
    print_status 0 "helm found"
    
    # Check cluster access
    if ! kubectl cluster-info &> /dev/null; then
        print_status 1 "Cannot access Kubernetes cluster"
        exit 1
    fi
    print_status 0 "Kubernetes cluster access verified"
    
    # Check ArgoCD
    if ! kubectl get namespace argocd &> /dev/null; then
        print_warning "ArgoCD namespace not found - will install ArgoCD first"
        INSTALL_ARGOCD=true
    else
        print_status 0 "ArgoCD namespace found"
        INSTALL_ARGOCD=false
    fi
}

# Install ArgoCD if needed
install_argocd() {
    if [ "$INSTALL_ARGOCD" = true ]; then
        print_header "Installing ArgoCD via Helm"
        
        # Add ArgoCD Helm repository
        helm repo add argo https://argoproj.github.io/argo-helm
        helm repo update
        
        # Install ArgoCD
        helm upgrade --install argocd argo/argo-cd \
            --namespace argocd \
            --create-namespace \
            --values helm/values/argocd/values-development.yaml \
            --wait --timeout=600s
        
        print_status $? "ArgoCD installed via Helm"
        
        # Wait for ArgoCD to be ready
        print_info "Waiting for ArgoCD to be ready..."
        kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd
        
        # Get admin password
        ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
        print_info "ArgoCD admin password: $ARGOCD_PASSWORD"
    fi
}

# Deploy ACK Controllers via ArgoCD + Helm
deploy_ack_controllers() {
    print_header "Deploying ACK Controllers via ArgoCD + Helm"
    
    # Apply ACK controller applications
    kubectl apply -f argocd/infrastructure/ack-controllers-helm.yaml
    print_status $? "ACK controller ArgoCD applications created"
    
    # Wait for applications to sync
    print_info "Waiting for ACK controllers to sync..."
    sleep 30
    
    # Check application status
    for app in ack-rds-controller ack-elasticache-controller ack-iam-controller; do
        kubectl wait --for=condition=Synced --timeout=300s application/$app -n argocd
        print_status $? "ACK application $app synced"
    done
}

# Deploy Observability Stack via ArgoCD + Helm
deploy_observability() {
    print_header "Deploying Observability Stack via ArgoCD + Helm"
    
    # Apply observability applications
    kubectl apply -f argocd/platform/monitoring-helm.yaml
    print_status $? "Observability ArgoCD applications created"
    
    # Wait for applications to sync
    print_info "Waiting for observability stack to sync..."
    sleep 30
    
    # Check application status
    kubectl wait --for=condition=Synced --timeout=600s application/prometheus-stack -n argocd
    print_status $? "Prometheus stack application synced"
}

# Deploy AWS Resources
deploy_aws_resources() {
    print_header "Deploying AWS Resources via ACK"
    
    # Apply AWS resources
    kubectl apply -f argocd/infrastructure/resource-orchestration.yaml
    print_status $? "AWS resources ArgoCD application created"
    
    # Wait for application to sync
    print_info "Waiting for AWS resources to sync..."
    sleep 30
    
    kubectl wait --for=condition=Synced --timeout=300s application/aws-resources -n argocd
    print_status $? "AWS resources application synced"
}

# Deploy Data Plane Services via ArgoCD + Helm
deploy_data_plane() {
    print_header "Deploying Data Plane Services via ArgoCD + Helm"
    
    # Apply data plane applications
    kubectl apply -f argocd/applications/data-plane-helm.yaml
    print_status $? "Data plane ArgoCD applications created"
    
    # Wait for applications to sync
    print_info "Waiting for data plane services to sync..."
    sleep 30
    
    # Check application status
    for app in hydrocoptic-datastore sinusoidal-database prefabulated-cache; do
        kubectl wait --for=condition=Synced --timeout=300s application/$app -n argocd
        print_status $? "Data plane application $app synced"
    done
}

# Validate deployment
validate_deployment() {
    print_header "Validating Deployment"
    
    # Check ArgoCD applications
    print_info "Checking ArgoCD applications..."
    kubectl get applications -n argocd
    
    # Check ACK controllers
    print_info "Checking ACK controllers..."
    kubectl get pods -n ack-system
    
    # Check observability stack
    print_info "Checking observability stack..."
    kubectl get pods -n encabulator-monitoring
    
    # Check data plane services
    print_info "Checking data plane services..."
    kubectl get pods -n encabulator-data
    
    # Check AWS resources
    print_info "Checking AWS resources..."
    kubectl get dbinstance,replicationgroup -n encabulator-data
}

# Provide access information
provide_access_info() {
    print_header "Access Information"
    
    print_info "ArgoCD UI:"
    print_info "  kubectl port-forward svc/argocd-server -n argocd 8080:443"
    print_info "  Username: admin"
    print_info "  Password: $ARGOCD_PASSWORD"
    
    print_info "Prometheus UI:"
    print_info "  kubectl port-forward svc/prometheus-stack-kube-prom-prometheus -n encabulator-monitoring 9090:9090"
    
    print_info "Grafana UI:"
    print_info "  kubectl port-forward svc/prometheus-stack-grafana -n encabulator-monitoring 3000:80"
    print_info "  Username: admin"
    print_info "  Password: encabulator-dev-2024"
}

# Main execution
main() {
    print_header "Cloud Native Encabulator - Helm Deployment"
    
    check_prerequisites
    install_argocd
    deploy_ack_controllers
    deploy_observability
    deploy_aws_resources
    deploy_data_plane
    validate_deployment
    provide_access_info
    
    print_header "Deployment Complete!"
    print_info "All components have been deployed via Helm charts and ArgoCD"
    print_info "Monitor the deployment progress in the ArgoCD UI"
}

# Handle script arguments
case "${1:-}" in
    --ack-only)
        print_info "Deploying ACK controllers only..."
        check_prerequisites
        deploy_ack_controllers
        ;;
    --observability-only)
        print_info "Deploying observability stack only..."
        check_prerequisites
        deploy_observability
        ;;
    --data-plane-only)
        print_info "Deploying data plane services only..."
        check_prerequisites
        deploy_data_plane
        ;;
    --validate)
        print_info "Validating existing deployment..."
        validate_deployment
        ;;
    --help)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --ack-only           Deploy only ACK controllers"
        echo "  --observability-only Deploy only observability stack"
        echo "  --data-plane-only    Deploy only data plane services"
        echo "  --validate           Validate existing deployment"
        echo "  --help               Show this help message"
        echo ""
        echo "Default: Deploy complete platform"
        ;;
    *)
        main
        ;;
esac