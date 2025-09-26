#!/bin/bash

# Validate ACK Setup Script for Cloud Native Encabulator
# This script validates that ACK controllers are properly installed and configured

set -e

echo "🔍 Validating Cloud Native Encabulator ACK Setup..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2${NC}"
    else
        echo -e "${RED}❌ $2${NC}"
        return 1
    fi
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_info() {
    echo -e "ℹ️  $1"
}

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}❌ kubectl is not installed or not in PATH${NC}"
    exit 1
fi

# Check if we can connect to the cluster
if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}❌ Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi

print_status 0 "Connected to Kubernetes cluster"

# Check if ack-system namespace exists
if kubectl get namespace ack-system &> /dev/null; then
    print_status 0 "ack-system namespace exists"
else
    print_status 1 "ack-system namespace does not exist"
fi

# Check ACK Controllers
echo ""
print_info "Checking ACK Controllers..."

controllers=("ack-rds-controller" "ack-elasticache-controller" "ack-iam-controller")

for controller in "${controllers[@]}"; do
    if kubectl get deployment "$controller" -n ack-system &> /dev/null; then
        # Check if deployment is ready
        ready=$(kubectl get deployment "$controller" -n ack-system -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        desired=$(kubectl get deployment "$controller" -n ack-system -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "1")
        
        if [ "$ready" = "$desired" ] && [ "$ready" != "0" ]; then
            print_status 0 "$controller is running and ready"
        else
            print_status 1 "$controller exists but is not ready ($ready/$desired)"
        fi
    else
        print_status 1 "$controller deployment not found"
    fi
done

# Check CRDs
echo ""
print_info "Checking Custom Resource Definitions..."

crds=("dbinstances.rds.services.k8s.aws" "replicationgroups.elasticache.services.k8s.aws" "roles.iam.services.k8s.aws")

for crd in "${crds[@]}"; do
    if kubectl get crd "$crd" &> /dev/null; then
        print_status 0 "$crd CRD is installed"
    else
        print_status 1 "$crd CRD not found"
    fi
done

# Check AWS Resources
echo ""
print_info "Checking AWS Resources..."

# Check if encabulator-data namespace exists
if kubectl get namespace encabulator-data &> /dev/null; then
    print_status 0 "encabulator-data namespace exists"
    
    # Check DBInstance
    if kubectl get dbinstance hydrocoptic-datastore -n encabulator-data &> /dev/null; then
        status=$(kubectl get dbinstance hydrocoptic-datastore -n encabulator-data -o jsonpath='{.status.dbInstanceStatus}' 2>/dev/null || echo "unknown")
        if [ "$status" = "available" ]; then
            print_status 0 "Hydrocoptic Data Store (PostgreSQL) is available"
        else
            print_warning "Hydrocoptic Data Store status: $status (may still be creating)"
        fi
    else
        print_status 1 "Hydrocoptic Data Store (DBInstance) not found"
    fi
    
    # Check ReplicationGroup
    if kubectl get replicationgroup prefabulated-cache -n encabulator-data &> /dev/null; then
        status=$(kubectl get replicationgroup prefabulated-cache -n encabulator-data -o jsonpath='{.status.status}' 2>/dev/null || echo "unknown")
        if [ "$status" = "available" ]; then
            print_status 0 "Prefabulated Cache (Redis) is available"
        else
            print_warning "Prefabulated Cache status: $status (may still be creating)"
        fi
    else
        print_status 1 "Prefabulated Cache (ReplicationGroup) not found"
    fi
else
    print_status 1 "encabulator-data namespace does not exist"
fi

# Check IAM Roles
echo ""
print_info "Checking IAM Resources..."

if kubectl get role encabulator-data-plane-role -n ack-system &> /dev/null; then
    print_status 0 "Data Plane IAM Role exists"
else
    print_status 1 "Data Plane IAM Role not found"
fi

# Check Secrets
echo ""
print_info "Checking Secrets..."

if kubectl get secret hydrocoptic-datastore-credentials -n encabulator-data &> /dev/null; then
    print_status 0 "Database credentials secret exists"
else
    print_status 1 "Database credentials secret not found"
fi

if kubectl get secret prefabulated-cache-credentials -n encabulator-data &> /dev/null; then
    print_status 0 "Cache credentials secret exists"
else
    print_status 1 "Cache credentials secret not found"
fi

echo ""
print_info "Validation complete!"

# Provide next steps
echo ""
echo "📋 Next Steps:"
echo "1. If any controllers are not ready, check their logs:"
echo "   kubectl logs -n ack-system deployment/ack-rds-controller"
echo ""
echo "2. If AWS resources are still creating, monitor their status:"
echo "   kubectl get dbinstance -n encabulator-data -w"
echo "   kubectl get replicationgroup -n encabulator-data -w"
echo ""
echo "3. Once resources are available, you can proceed with Task 3 (Observability Infrastructure)"
echo ""
echo "4. To get connection details for applications:"
echo "   kubectl get dbinstance hydrocoptic-datastore -n encabulator-data -o yaml"
echo "   kubectl get replicationgroup prefabulated-cache -n encabulator-data -o yaml"