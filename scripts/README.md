# Cloud Native Encabulator Scripts Documentation

This directory contains automation scripts for building, testing, and deploying the Cloud Native Encabulator Microservices Demo. The platform uses a **Makefile-first** approach with scripts providing convenient wrappers and deployment automation.

## 📁 Directory Structure

```
scripts/
├── build/             # Build scripts (mostly deprecated - use Makefile)
├── test/              # Testing and validation scripts
├── deploy/            # Deployment scripts (Helm-first approach)
├── setup/             # Initial configuration scripts
└── README.md          # This documentation
```

## 🎯 Quick Start Guide

### For Local Development:
```bash
# 1. Build all services
make build

# 2. Run all tests
make test

# 3. Build container images
make image
```

### For Platform Deployment:
```bash
# 1. Deploy complete platform via Helm
./scripts/deploy/deploy-helm.sh

# 2. Validate deployment
./scripts/deploy/status.sh

# 3. Run comprehensive tests
./scripts/test/comprehensive-test.sh
```

### For Testing Individual Components:
```bash
# Test specific service plane
make test-data-plane
make test-processing-plane
make test-support-plane
make test-control-plane

# Test specific data service
./scripts/test/test-data-services.sh
```

## 📋 Script Reference

### Build Scripts (`build/`)

> **Note**: Most build scripts are deprecated in favor of the unified Makefile. Use `make` targets instead.

#### `build-all.sh` - **BUILD ALL SERVICES**
**Purpose**: Build all services across all planes  
**Recommendation**: Use `make build` directly  
**What it does**:
- Builds all Go services (Data, Processing, Support, Control planes)
- Builds all TypeScript services (Processing, Control planes)
- Builds all Python services (Support, Control planes)
- Provides build summary

**Usage**:
```bash
./scripts/build/build-all.sh
# Or better: make build
```

---

#### `build-images.sh` - **BUILD ALL CONTAINER IMAGES**
**Purpose**: Build container images for all services  
**Recommendation**: Use `make image` directly  
**What it does**:
- Detects container runtime (Docker or Finch)
- Builds images for all 13 services
- Provides image summary

**Usage**:
```bash
./scripts/build/build-images.sh
# Or better: make image
```

---

### Deployment Scripts (`deploy/`)

#### `deploy-helm.sh` - **PRIMARY DEPLOYMENT METHOD**
**Purpose**: Deploy complete platform using Helm charts  
**Use Case**: Production and development deployments  
**What it does**:
- Installs ArgoCD via Helm
- Deploys observability stack (Prometheus, Grafana)
- Deploys data plane backends (PostgreSQL, Redis, InfluxDB)
- Validates all deployments
- Provides access information

**Usage**:
```bash
# Complete deployment
./scripts/deploy/deploy-helm.sh

# Component-specific deployment
./scripts/deploy/deploy-helm.sh --argocd-only
./scripts/deploy/deploy-helm.sh --observability-only
./scripts/deploy/deploy-helm.sh --data-plane-only

# Validation only
./scripts/deploy/deploy-helm.sh --validate
```

**Prerequisites**: 
- Kubernetes cluster access
- helm and kubectl installed

**Output**: 
- Complete platform deployed via Helm
- Access credentials and port-forward commands

---

#### `deploy-dev.sh` - **DEVELOPMENT CLUSTER DEPLOYMENT**
**Purpose**: Deploy to development EKS cluster  
**Use Case**: EKS-specific development setup  
**What it does**:
- Deploys data services (PostgreSQL, Redis, InfluxDB)
- Waits for deployments to be ready
- Shows deployment status and access info

**Usage**:
```bash
# Full deployment
./scripts/deploy/deploy-dev.sh

# Status check only
./scripts/deploy/deploy-dev.sh --status-only
```

**Prerequisites**: 
- EKS cluster access
- AWS CLI configured
- kubectl and helm installed

---

#### `status.sh` - **DEPLOYMENT STATUS CHECKER**
**Purpose**: Comprehensive status check for platform deployment  
**Use Case**: Monitoring and troubleshooting  
**What it does**:
- Checks cluster connectivity
- Validates infrastructure components
- Checks observability stack status
- Shows ArgoCD applications
- Provides access commands

**Usage**:
```bash
# Complete status check
./scripts/deploy/status.sh

# Component-specific checks
./scripts/deploy/status.sh --observability-only
./scripts/deploy/status.sh --infrastructure-only
```

---

### Test Scripts (`test/`)

#### `test-all.sh` - **TEST ALL SERVICES**
**Purpose**: Run all unit tests across all service planes  
**Recommendation**: Use `make test` directly  
**What it does**:
- Runs tests for all Go services
- Runs tests for all TypeScript services
- Runs tests for all Python services (if pytest available)
- Provides test summary

**Usage**:
```bash
./scripts/test/test-all.sh
# Or better: make test
```

---

#### `comprehensive-test.sh` - **COMPREHENSIVE PLATFORM TESTING**
**Purpose**: Complete platform testing including infrastructure and integration  
**Use Case**: Validation of complete deployment  
**What it does**:
- Tests Helm infrastructure
- Tests ArgoCD installation
- Tests observability stack
- Tests data plane services
- Tests service connectivity
- Validates Helm integration

**Usage**:
```bash
# Complete test suite
./scripts/test/comprehensive-test.sh

# Quick test (infrastructure + observability)
./scripts/test/comprehensive-test.sh --quick

# Component-specific tests
./scripts/test/comprehensive-test.sh --infrastructure-only
./scripts/test/comprehensive-test.sh --observability-only
./scripts/test/comprehensive-test.sh --data-plane-only

# Status only
./scripts/test/comprehensive-test.sh --status-only
```

**Prerequisites**: 
- Kubernetes cluster access
- Platform deployed via deploy-helm.sh

---

#### `test-data-services.sh` - **DATA SERVICES CONNECTIVITY TEST**
**Purpose**: Test data service backends connectivity and credentials  
**Use Case**: Validate data plane backend deployment  
**What it does**:
- Tests PostgreSQL connectivity
- Tests Redis connectivity
- Tests InfluxDB connectivity
- Validates database backends
- Checks credentials and secrets

**Usage**:
```bash
# Test all data services
./scripts/test/test-data-services.sh

# Test specific service
./scripts/test/test-data-services.sh --postgresql
./scripts/test/test-data-services.sh --redis
./scripts/test/test-data-services.sh --influxdb
./scripts/test/test-data-services.sh --backends
./scripts/test/test-data-services.sh --credentials
```

**Prerequisites**: 
- Data services deployed in encabulator-data namespace

---

#### `local-test.sh` - **LOCAL TESTING**
**Purpose**: Test platform components locally  
**Use Case**: Development and local validation  

---

#### `infrastructure-test.sh` - **INFRASTRUCTURE TESTING**
**Purpose**: Test infrastructure components  
**Use Case**: Validate infrastructure deployment  

---

#### `cleanup-test.sh` - **TEST CLEANUP**
**Purpose**: Clean up test resources  
**Use Case**: Remove test deployments  

---

#### `validate-storage.sh` - **STORAGE VALIDATION**
**Purpose**: Validate storage configurations  
**Use Case**: Check persistent volume claims  

---

### Setup Scripts (`setup/`)

#### `configure-repository.sh` - **GITOPS CONFIGURATION**
**Purpose**: Configure Git repository URLs for GitOps  
**Use Case**: First-time setup, changing repositories  
**What it does**:
- Prompts for GitHub organization/username
- Updates ArgoCD configuration files
- Updates root application manifests
- Replaces placeholder URLs

**Usage**:
```bash
./scripts/setup/configure-repository.sh
```

**Prerequisites**: 
- Write access to configuration files
- Git repository created

---

#### `eks-setup.sh` - **EKS ENVIRONMENT SETUP**
**Purpose**: Set up EKS development environment  
**Use Case**: Initial EKS cluster preparation  
**What it does**:
- Configures EKS-specific settings
- Sets up development environment variables
- Validates EKS cluster readiness

**Usage**:
```bash
./scripts/setup/eks-setup.sh
```

**Prerequisites**: 
- EKS cluster created
- AWS CLI configured
- kubectl configured for EKS

---

## 🎯 Common Workflows

### Complete Development Workflow
```bash
# 1. Build all services
make build

# 2. Run all tests
make test

# 3. Build container images
make image

# 4. Deploy to cluster
./scripts/deploy/deploy-helm.sh

# 5. Validate deployment
./scripts/deploy/status.sh

# 6. Run comprehensive tests
./scripts/test/comprehensive-test.sh
```

### Quick Build and Test
```bash
# Build and test in one command
make ci

# Or verify everything
make verify
```

### Service Plane Development
```bash
# Work on specific plane
make build-data-plane
make test-data-plane
make image-data-plane

# Or use other planes
make build-processing-plane
make build-support-plane
make build-control-plane
```

### Deployment Validation
```bash
# 1. Deploy platform
./scripts/deploy/deploy-helm.sh

# 2. Check status
./scripts/deploy/status.sh

# 3. Test data services
./scripts/test/test-data-services.sh

# 4. Run comprehensive tests
./scripts/test/comprehensive-test.sh
```

### Troubleshooting Workflow
```bash
# 1. Check deployment status
./scripts/deploy/status.sh

# 2. Test specific components
./scripts/test/test-data-services.sh --postgresql
./scripts/test/test-data-services.sh --redis

# 3. Check Helm releases
helm list -A

# 4. Check pod status
kubectl get pods -A | grep encabulator

# 5. View logs
kubectl logs -n encabulator-data <pod-name>
```

## 🔧 Makefile Targets

The unified Makefile provides the primary interface for building and testing:

### Build Targets
- `make build` - Build all services
- `make build-data-plane` - Build Data Plane services
- `make build-processing-plane` - Build Processing Plane services
- `make build-support-plane` - Build Support Plane services
- `make build-control-plane` - Build Control Plane services

### Test Targets
- `make test` - Run all tests
- `make test-data-plane` - Test Data Plane services
- `make test-processing-plane` - Test Processing Plane services
- `make test-support-plane` - Test Support Plane services
- `make test-control-plane` - Test Control Plane services

### Image Targets
- `make image` - Build all container images
- `make image-data-plane` - Build Data Plane images
- `make image-processing-plane` - Build Processing Plane images
- `make image-support-plane` - Build Support Plane images
- `make image-control-plane` - Build Control Plane images

### Helm Targets
- `make helm-lint` - Lint all Helm charts
- `make helm-template` - Test Helm template rendering

### Utility Targets
- `make clean` - Clean all build artifacts
- `make fmt` - Format all code
- `make ci` - Run CI pipeline (build, test, lint)
- `make verify` - Verify all builds, tests, and Helm charts
- `make help` - Show all available targets

## 📊 Service Architecture

The platform consists of 13 microservices across 4 planes:

### Data Plane (3 services - Go)
- `hydrocoptic-datastore` - PostgreSQL wrapper
- `prefabulated-cache` - Redis wrapper
- `sinusoidal-database` - InfluxDB wrapper

### Processing Plane (3 services - 2 Go, 1 TypeScript)
- `logarithmic-casing-service` - Message broker (Go)
- `dingle-arm-api-gateway` - API gateway (Go)
- `panametric-fam-processor` - Core processor (TypeScript)

### Support Plane (3 services - 1 Go, 2 Python)
- `differential-girdle` - Feature flags (Go)
- `cardinal-grammeter` - Metrics collector (Python)
- `side-fumbling-collector` - Telemetry (Python)

### Control Plane (4 services - 1 Go, 2 TypeScript, 1 Python)
- `malaise-injection-engine` - Chaos engineering (Go)
- `unilateral-phase-detractor` - Traffic generator (TypeScript)
- `encabulation-control-console` - Web UI (TypeScript/React)
- `demo-scenario-orchestrator` - Demo orchestration (Python)

## 🔍 Troubleshooting

### Script Execution Issues

**Permission denied**:
```bash
chmod +x scripts/**/*.sh
```

**kubectl not found**:
```bash
# macOS
brew install kubectl

# Linux
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

**helm not found**:
```bash
# macOS
brew install helm

# Linux
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

**Cluster connection issues**:
```bash
# Verify cluster access
kubectl cluster-info
kubectl get nodes

# For EKS
aws eks update-kubeconfig --region <region> --name <cluster-name>
```

### Common Deployment Failures

**ArgoCD installation timeout**:
- Check cluster resources: `kubectl top nodes`
- Verify internet connectivity for image pulls
- Increase timeout: `--timeout=900s`

**Helm chart deployment failures**:
- Verify Helm repositories: `helm repo list`
- Update repositories: `helm repo update`
- Check chart values: `helm template test <chart>`

**Data service connection issues**:
- Check pod status: `kubectl get pods -n encabulator-data`
- View logs: `kubectl logs -n encabulator-data <pod-name>`
- Test connectivity: `./scripts/test/test-data-services.sh`

**Image pull failures**:
- Verify images exist: `docker images | grep encabulator`
- Check image pull policy in Helm values
- For local development, use `imagePullPolicy: Never`

## 📖 Additional Resources

- **Main Setup Guide**: `../SETUP.md`
- **Build Guide**: `../BUILD.md`
- **Architecture Overview**: `../docs/architecture/`
- **Helm Charts**: `../helm/charts/`
- **Makefile**: `../Makefile`

## 🤝 Contributing

When adding new scripts:

1. **Follow naming conventions**: `verb-noun.sh` (e.g., `deploy-monitoring.sh`)
2. **Add proper documentation**: Include purpose, usage, prerequisites
3. **Use consistent logging**: Follow the color-coded logging pattern
4. **Add error handling**: Use `set -e` and proper error messages
5. **Update this README**: Document new scripts in the appropriate section
6. **Consider Makefile integration**: Can this be a Makefile target instead?

## 📝 Script Template

```bash
#!/bin/bash

# Cloud Native Encabulator - [Script Purpose]
# [Brief description of what this script does]

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

# Script implementation here...
```

## 🚀 Best Practices

### Use Makefile Targets

The unified Makefile provides the primary interface:

```bash
# Build
make build              # All services
make build-data-plane   # Specific plane

# Test
make test               # All tests
make test-data-plane    # Specific plane

# Images
make image              # All images
make image-data-plane   # Specific plane
```

### Use Deployment Scripts

For deployment, use the Helm-based scripts:

```bash
./scripts/deploy/deploy-helm.sh              # Complete deployment
./scripts/deploy/deploy-helm.sh --data-plane-only  # Specific component
./scripts/deploy/status.sh                   # Check status
```

## 📅 Version History

- **v2.0** (Current) - Makefile-first approach, Helm-based deployment, 13 microservices
- **v1.0** - Initial GitOps approach, individual build scripts
