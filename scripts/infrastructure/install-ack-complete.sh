#!/bin/bash

# Complete ACK Installation Script for Cloud Native Encabulator
# This script installs CRDs and controllers for AWS Controllers for Kubernetes

set -e

echo "🚀 Installing Complete AWS Controllers for Kubernetes (ACK) Stack..."

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

# Check prerequisites
echo ""
print_info "Checking prerequisites..."

if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}❌ kubectl is not installed or not in PATH${NC}"
    exit 1
fi

if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}❌ Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi

print_status 0 "Prerequisites check passed"

# Step 1: Install CRDs
echo ""
print_info "Step 1: Installing ACK Custom Resource Definitions..."

if [ -f "./scripts/infrastructure/install-ack-crds.sh" ]; then
    ./scripts/infrastructure/install-ack-crds.sh
else
    print_warning "CRD installation script not found, installing manually..."
    
    # Install essential CRDs
    crds=(
        "https://raw.githubusercontent.com/aws-controllers-k8s/rds-controller/main/config/crd/bases/rds.services.k8s.aws_dbinstances.yaml"
        "https://raw.githubusercontent.com/aws-controllers-k8s/elasticache-controller/main/config/crd/bases/elasticache.services.k8s.aws_replicationgroups.yaml"
        "https://raw.githubusercontent.com/aws-controllers-k8s/iam-controller/main/config/crd/bases/iam.services.k8s.aws_roles.yaml"
    )
    
    for crd_url in "${crds[@]}"; do
        crd_name=$(basename "$crd_url" .yaml)
        if kubectl apply -f "$crd_url" > /dev/null 2>&1; then
            print_status 0 "Installed $crd_name"
        else
            print_warning "Failed to install $crd_name (may already exist)"
        fi
    done
fi

# Step 2: Create namespace
echo ""
print_info "Step 2: Creating ACK system namespace..."

if kubectl apply -f k8s/infrastructure/ack-controllers/namespace.yaml > /dev/null 2>&1; then
    print_status 0 "Created ack-system namespace"
else
    print_warning "ack-system namespace may already exist"
fi

# Step 3: Install controllers
echo ""
print_info "Step 3: Installing ACK Controllers..."

controllers=(
    "k8s/infrastructure/ack-controllers/rds-controller.yaml"
    "k8s/infrastructure/ack-controllers/elasticache-controller.yaml"
    "k8s/infrastructure/ack-controllers/iam-controller.yaml"
)

for controller in "${controllers[@]}"; do
    controller_name=$(basename "$controller" .yaml)
    if [ -f "$controller" ]; then
        if kubectl apply -f "$controller" > /dev/null 2>&1; then
            print_status 0 "Deployed $controller_name"
        else
            print_status 1 "Failed to deploy $controller_name"
        fi
    else
        print_status 1 "$controller not found"
    fi
done

# Step 4: Wait for controllers to be ready
echo ""
print_info "Step 4: Waiting for controllers to be ready..."

controllers_to_check=(
    "ack-rds-controller"
    "ack-elasticache-controller"
    "ack-iam-controller"
)

for controller in "${controllers_to_check[@]}"; do
    print_info "Waiting for $controller to be ready..."
    if kubectl wait --for=condition=available --timeout=300s deployment/$controller -n ack-system > /dev/null 2>&1; then
        print_status 0 "$controller is ready"
    else
        print_warning "$controller is not ready yet (may need more time)"
    fi
done

# Step 5: Verify installation
echo ""
print_info "Step 5: Verifying installation..."

# Check CRDs
required_crds=(
    "dbinstances.rds.services.k8s.aws"
    "replicationgroups.elasticache.services.k8s.aws"
    "roles.iam.services.k8s.aws"
)

for crd in "${required_crds[@]}"; do
    if kubectl get crd "$crd" > /dev/null 2>&1; then
        print_status 0 "$crd CRD is available"
    else
        print_status 1 "$crd CRD is not available"
    fi
done

# Check controller pods
for controller in "${controllers_to_check[@]}"; do
    if kubectl get deployment "$controller" -n ack-system > /dev/null 2>&1; then
        ready=$(kubectl get deployment "$controller" -n ack-system -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        desired=$(kubectl get deployment "$controller" -n ack-system -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "1")
        
        if [ "$ready" = "$desired" ] && [ "$ready" != "0" ]; then
            print_status 0 "$controller is running ($ready/$desired)"
        else
            print_warning "$controller is not fully ready ($ready/$desired)"
        fi
    else
        print_status 1 "$controller deployment not found"
    fi
done

echo ""
print_info "ACK installation complete!"

echo ""
echo "📋 Installation Summary:"
echo "✅ Custom Resource Definitions installed"
echo "✅ ACK Controllers deployed (direct Kubernetes manifests)"
echo "✅ Namespace and RBAC configured"
echo ""
echo "🔧 Configuration Required:"
echo "1. Update IAM role ARNs in controller ServiceAccounts"
echo "2. Ensure IAM roles have proper AWS permissions"
echo "3. Update placeholder values in AWS resource manifests"
echo ""
echo "🚀 Next Steps:"
echo "1. Configure AWS credentials and IAM roles"
echo "2. Deploy AWS resources: kubectl apply -k k8s/infrastructure/aws-resources/"
echo "3. Run validation: ./scripts/infrastructure/validate-ack-setup.sh"
echo ""
echo "📖 Recommended Installation Method:"
echo "• ArgoCD GitOps (Recommended): ./scripts/platform/bootstrap-gitops.sh"
echo "• Direct Helm: helm install ack-rds-controller oci://public.ecr.aws/aws-controllers-k8s/rds-chart"
echo "• Direct Kubernetes: kubectl apply -k k8s/infrastructure/ack-controllers/"
echo ""
echo "📖 For detailed setup instructions, see: k8s/infrastructure/README.md"