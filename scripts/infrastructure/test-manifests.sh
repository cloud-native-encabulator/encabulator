#!/bin/bash

# Test Cloud Native Encabulator Kubernetes Manifests
# This script validates manifest syntax and structure without creating AWS resources

set -e

echo "🧪 Testing Cloud Native Encabulator Kubernetes Manifests..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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
    echo -e "ℹ️  $1"
}

# Test 1: Validate Kustomize builds
echo ""
print_info "Testing Kustomize builds..."

if kustomize build k8s/infrastructure/ack-controllers/ > /dev/null 2>&1; then
    print_status 0 "ACK Controllers kustomization builds successfully"
else
    print_status 1 "ACK Controllers kustomization failed to build"
fi

if kustomize build k8s/infrastructure/aws-resources/ > /dev/null 2>&1; then
    print_status 0 "AWS Resources kustomization builds successfully"
else
    print_status 1 "AWS Resources kustomization failed to build"
fi

# Test 2: Validate YAML syntax
echo ""
print_info "Testing YAML syntax..."

yaml_files=(
    "k8s/infrastructure/ack-controllers/namespace.yaml"
    "k8s/infrastructure/aws-resources/hydrocoptic-datastore.yaml"
    "k8s/infrastructure/aws-resources/prefabulated-cache.yaml"
    "k8s/infrastructure/aws-resources/iam-roles.yaml"
    "argocd/infrastructure/ack-controllers.yaml"
    "argocd/infrastructure/aws-resources.yaml"
)

for file in "${yaml_files[@]}"; do
    if [ -f "$file" ]; then
        # Skip ArgoCD Application validation if ArgoCD CRDs are not installed
        if [[ "$file" == *"argocd/"* ]]; then
            if kubectl get crd applications.argoproj.io > /dev/null 2>&1; then
                if kubectl apply --dry-run=client --validate=true -f "$file" > /dev/null 2>&1; then
                    print_status 0 "$(basename "$file") has valid YAML syntax"
                else
                    print_status 1 "$(basename "$file") has invalid YAML syntax"
                fi
            else
                echo -e "${YELLOW}⚠️  $(basename "$file") skipped (ArgoCD not installed)${NC}"
            fi
        else
            if kubectl apply --dry-run=client --validate=true -f "$file" > /dev/null 2>&1; then
                print_status 0 "$(basename "$file") has valid YAML syntax"
            else
                print_status 1 "$(basename "$file") has invalid YAML syntax"
            fi
        fi
    else
        print_status 1 "$(basename "$file") not found"
    fi
done

# Test 3: Check for required CRDs (if available)
echo ""
print_info "Checking for ACK Custom Resource Definitions..."

crds=(
    "dbinstances.rds.services.k8s.aws"
    "replicationgroups.elasticache.services.k8s.aws"
    "roles.iam.services.k8s.aws"
)

for crd in "${crds[@]}"; do
    if kubectl get crd "$crd" > /dev/null 2>&1; then
        print_status 0 "$crd CRD is available"
    else
        echo -e "${YELLOW}⚠️  $crd CRD not found (install ACK controllers first)${NC}"
    fi
done

# Test 4: Validate resource references
echo ""
print_info "Testing resource references..."

# Check if secrets are properly referenced
if grep -q "hydrocoptic-datastore-credentials" k8s/infrastructure/aws-resources/hydrocoptic-datastore.yaml; then
    print_status 0 "Database credentials secret is properly referenced"
else
    print_status 1 "Database credentials secret reference missing"
fi

if grep -q "prefabulated-cache-credentials" k8s/infrastructure/aws-resources/prefabulated-cache.yaml; then
    print_status 0 "Cache credentials secret is properly referenced"
else
    print_status 1 "Cache credentials secret reference missing"
fi

# Test 5: Check ArgoCD Application structure
echo ""
print_info "Testing ArgoCD Application structure..."

if grep -q "kind: Application" argocd/infrastructure/ack-controllers.yaml; then
    print_status 0 "ACK Controllers ArgoCD Application has correct kind"
else
    print_status 1 "ACK Controllers ArgoCD Application missing or incorrect kind"
fi

if grep -q "oci://public.ecr.aws/aws-controllers-k8s" argocd/infrastructure/ack-controllers.yaml; then
    print_status 0 "ACK Controllers ArgoCD Application uses correct OCI registry"
else
    print_status 1 "ACK Controllers ArgoCD Application OCI registry reference incorrect"
fi

# Test 6: Validate namespace consistency
echo ""
print_info "Testing namespace consistency..."

if grep -q "name: ack-system" k8s/infrastructure/ack-controllers/namespace.yaml; then
    print_status 0 "ACK namespace is consistently defined"
else
    print_status 1 "ACK namespace definition inconsistent"
fi

if grep -q "namespace: encabulator-data" k8s/infrastructure/aws-resources/hydrocoptic-datastore.yaml; then
    print_status 0 "Data plane namespace is consistently used"
else
    print_status 1 "Data plane namespace usage inconsistent"
fi

echo ""
print_info "Manifest testing complete!"

echo ""
echo "📋 Summary:"
echo "- All manifests should have valid YAML syntax"
echo "- Kustomize builds should work without errors"
echo "- Resource references should be consistent"
echo "- ArgoCD Applications should point to correct paths"
echo ""
echo "🚀 Next steps:"
echo "1. Install ACK controllers in your cluster"
echo "2. Update placeholder values (security groups, subnet groups, account IDs)"
echo "3. Deploy via ArgoCD or kubectl apply"
echo "4. Run validation script: ./scripts/infrastructure/validate-ack-setup.sh"