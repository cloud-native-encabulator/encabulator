#!/bin/bash

# Cloud Native Encabulator Microservices Demo - EKS Setup
# This script sets up the development environment assuming an EKS cluster context

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

# Check prerequisites for EKS development
check_prerequisites() {
    log_info "Checking EKS development prerequisites..."
    
    local missing_tools=()
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists aws; then
        missing_tools+=("aws-cli")
    fi
    
    if ! command_exists helm; then
        missing_tools+=("helm")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install the missing tools:"
        log_info "  kubectl: https://kubernetes.io/docs/tasks/tools/"
        log_info "  aws-cli: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"
        log_info "  helm: https://helm.sh/docs/intro/install/"
        exit 1
    fi
    
    log_success "All prerequisites are installed"
}

# Verify EKS cluster access
verify_eks_cluster() {
    log_info "Verifying EKS cluster access..."
    
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot access Kubernetes cluster"
        log_info "Please ensure:"
        log_info "1. Your EKS cluster is running"
        log_info "2. AWS CLI is configured: aws configure"
        log_info "3. Kubectl context is set: aws eks update-kubeconfig --region <region> --name <cluster-name>"
        exit 1
    fi
    
    local cluster_info=$(kubectl cluster-info | head -1)
    log_success "Connected to: $cluster_info"
    
    # Check if it's an EKS cluster
    if kubectl get nodes -o wide | grep -q "eks"; then
        log_success "Confirmed EKS cluster"
    else
        log_warning "This doesn't appear to be an EKS cluster, but continuing anyway"
    fi
    
    # Show current context
    local current_context=$(kubectl config current-context)
    log_info "Current context: $current_context"
}

# Check cluster permissions
check_permissions() {
    log_info "Checking cluster permissions..."
    
    # Test basic permissions
    if ! kubectl auth can-i create namespaces >/dev/null 2>&1; then
        log_error "Insufficient permissions to create namespaces"
        log_info "Please ensure your AWS user/role has appropriate EKS permissions"
        exit 1
    fi
    
    if ! kubectl auth can-i create deployments >/dev/null 2>&1; then
        log_error "Insufficient permissions to create deployments"
        exit 1
    fi
    
    log_success "Cluster permissions verified"
}

# Install/verify kustomize
setup_kustomize() {
    if command_exists kustomize; then
        log_info "Kustomize is already installed"
        return
    fi
    
    log_info "Installing kustomize..."
    if ! curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash; then
        log_warning "Failed to install kustomize, will use kubectl kustomize instead"
    else
        if [ -f ./kustomize ]; then
            sudo mv kustomize /usr/local/bin/ || log_warning "Failed to move kustomize to /usr/local/bin"
        fi
    fi
}

# Create environment configuration
create_eks_config() {
    log_info "Creating EKS environment configuration..."
    
    # Get cluster info
    local cluster_name=$(kubectl config current-context | cut -d'/' -f2 2>/dev/null || echo "unknown")
    local region=$(aws configure get region 2>/dev/null || echo "us-west-2")
    
    # Create .env file for EKS development
    cat > .env.eks << EOF
# EKS Development Environment Variables
ENVIRONMENT=eks-dev
LOG_LEVEL=info

# EKS Cluster Information
EKS_CLUSTER_NAME=$cluster_name
AWS_REGION=$region

# Database Configuration (will be deployed in cluster)
POSTGRES_HOST=postgres.encabulator-data.svc.cluster.local
POSTGRES_PORT=5432
POSTGRES_DB=encabulator
POSTGRES_USER=encabulator_user

# Redis Configuration (will be deployed in cluster)
REDIS_HOST=redis.encabulator-data.svc.cluster.local
REDIS_PORT=6379

# Message Broker Configuration (will be deployed in cluster)
NATS_URL=nats://nats.encabulator-data.svc.cluster.local:4222

# API Gateway Configuration
GATEWAY_PORT=80

# Monitoring Configuration
PROMETHEUS_URL=http://prometheus.encabulator-monitoring.svc.cluster.local:9090
GRAFANA_URL=http://grafana.encabulator-monitoring.svc.cluster.local:3000
JAEGER_URL=http://jaeger.encabulator-monitoring.svc.cluster.local:16686
EOF
    
    log_success "Created .env.eks file"
}

# Show next steps
show_next_steps() {
    log_success "EKS development environment setup completed!"
    log_info ""
    log_info "🎉 Your EKS development environment is ready!"
    log_info ""
    log_info "Current cluster: $(kubectl config current-context)"
    log_info "AWS Region: $(aws configure get region 2>/dev/null || echo 'not configured')"
    log_info ""
    log_info "Next steps:"
    log_info "1. Deploy the infrastructure: ./scripts/deploy/deploy-eks.sh"
    log_info "2. Check deployment status: ./scripts/deploy/status.sh"
    log_info "3. Access services via port-forward or ingress"
    log_info ""
    log_info "🔧 Useful commands:"
    log_info "  kubectl get all --all-namespaces -l app.kubernetes.io/name=cloud-native-encabulator"
    log_info "  kubectl port-forward -n encabulator-monitoring svc/grafana 3000:3000"
    log_info "  kubectl port-forward -n encabulator-monitoring svc/prometheus 9090:9090"
    log_info ""
    log_info "📁 Configuration saved to .env.eks"
}

# Main setup function
main() {
    log_info "Starting Cloud Native Encabulator EKS development setup..."
    echo ""
    
    check_prerequisites
    verify_eks_cluster
    check_permissions
    setup_kustomize
    create_eks_config
    show_next_steps
}

# Handle script arguments
case "${1:-}" in
    --verify-only)
        log_info "Running verification only..."
        check_prerequisites
        verify_eks_cluster
        check_permissions
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --verify-only    Only verify cluster access and permissions"
        echo "  --help, -h       Show this help message"
        echo ""
        echo "Default: Complete EKS development environment setup"
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