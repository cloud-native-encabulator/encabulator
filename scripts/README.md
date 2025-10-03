# Cloud Native Encabulator Scripts Documentation

This directory contains all automation scripts for deploying and managing the Cloud Native Encabulator Microservices Demo. Each script serves a specific purpose in the deployment workflow.

## 📁 Directory Structure

```
scripts/
├── test/              # Testing and validation scripts
├── deploy/            # Deployment and status scripts  
├── setup/             # Initial configuration and setup scripts
└── README.md          # This documentation
```

## 🚀 Quick Start

### For Testing (Recommended First Step):
```bash
# 1. Test complete platform locally
./scripts/test/comprehensive-test.sh

# 2. Test specific components
./scripts/test/local-test.sh            # Observability only

# 3. Clean up when done
./scripts/test/local-test.sh --cleanup
```

### For Production/GitOps Deployment:
```bash
# 1. Configure your repository
./scripts/setup/configure-repository.sh

# 2. Bootstrap complete system
./scripts/platform/bootstrap-gitops.sh

# 3. Validate deployment
./scripts/deploy/status.sh
```

### For Helm-Based Deployment (Recommended):
```bash
# 1. Deploy complete platform via Helm
./scripts/deploy/deploy-helm.sh

# 2. Deploy specific components
./scripts/deploy/deploy-helm.sh --observability-only
./scripts/deploy/deploy-helm.sh --data-plane-only

# 3. Validate deployment
./scripts/deploy/deploy-helm.sh --validate
```

### For Development Cluster Deployment:
```bash
# 1. Set up EKS environment
./scripts/setup/eks-setup.sh

# 2. Deploy complete platform
./scripts/deploy/deploy-dev.sh

# 3. Check deployment status
./scripts/deploy/status.sh
```

## 📋 Script Reference

### Deployment Scripts (`deploy/`)

#### `deploy-helm.sh` - **HELM-FIRST DEPLOYMENT**
**Purpose**: Deploy complete platform using Helm charts and ArgoCD  
**Use Case**: Production deployment with Helm-first approach  
**What it does**:
- Installs ArgoCD via Helm if not present
- Deploys ACK controllers via official OCI Helm charts
- Deploys observability stack via kube-prometheus-stack
- Deploys data plane services via custom and community Helm charts
- Validates all deployments and provides access information

**Usage**:
```bash
# Complete deployment
./scripts/deploy/deploy-helm.sh

# Component-specific deployment
./scripts/deploy/deploy-helm.sh --ack-only
./scripts/deploy/deploy-helm.sh --observability-only
./scripts/deploy/deploy-helm.sh --data-plane-only

# Validation
./scripts/deploy/deploy-helm.sh --validate
```

**Prerequisites**: 
- Kubernetes cluster access
- helm and kubectl installed
- Configured repository URLs

**Output**: 
- Complete platform deployed via Helm charts
- ArgoCD managing all applications
- Access credentials and port-forward instructions

---

### Platform Scripts (`platform/`)

#### `bootstrap-gitops.sh` - **GITOPS DEPLOYMENT METHOD**
**Purpose**: Complete GitOps setup from scratch  
**Use Case**: First-time deployment, production setup  
**What it does**:
- Installs ArgoCD using official manifests
- Configures projects and repositories  
- Deploys root applications for infrastructure, platform, and applications
- Sets up complete GitOps workflow
- Provides access credentials and next steps

**Usage**:
```bash
./scripts/platform/bootstrap-gitops.sh
```

**Prerequisites**: 
- Kubernetes cluster access
- Repository URLs configured (run `configure-repository.sh` first)

**Output**: 
- ArgoCD installed and configured
- Root applications deployed
- Admin credentials displayed
- Port-forward instructions provided

---

#### `install-argocd.sh` - **ARGOCD FOUNDATION**
**Purpose**: Install and configure ArgoCD only  
**Use Case**: When you want ArgoCD setup without applications  
**What it does**:
- Installs ArgoCD using official stable manifests
- Applies custom configuration (repositories, projects)
- Creates ArgoCD projects for infrastructure and applications
- Retrieves and displays admin credentials

**Usage**:
```bash
./scripts/platform/install-argocd.sh
```

**Prerequisites**: 
- Kubernetes cluster access
- kubectl configured

**Output**: 
- ArgoCD installed in `argocd` namespace
- Projects created
- Admin password displayed

---





### Setup Scripts (`setup/`)

#### `configure-repository.sh` - **GITOPS CONFIGURATION**
**Purpose**: Configure Git repository URLs for GitOps  
**Use Case**: First-time setup, changing repositories  
**What it does**:
- Prompts for GitHub organization/username
- Updates ArgoCD configuration files with repository URL
- Updates root application manifests
- Replaces placeholder URLs throughout the system

**Usage**:
```bash
./scripts/setup/configure-repository.sh
```

**Prerequisites**: 
- Write access to configuration files
- Git repository created and accessible

**Output**: 
- All configuration files updated with repository URL
- Instructions for committing changes
- Guidance for private repository setup

---

#### `eks-setup.sh` - **EKS ENVIRONMENT SETUP**
**Purpose**: Set up EKS development environment  
**Use Case**: Initial EKS cluster preparation  
**What it does**:
- Configures EKS-specific settings and optimizations
- Sets up development environment variables
- Prepares cluster for Cloud Native Encabulator deployment
- Validates EKS cluster readiness

**Usage**:
```bash
./scripts/setup/eks-setup.sh
```

**Prerequisites**: 
- EKS cluster created and accessible
- AWS CLI configured
- kubectl configured for EKS

**Output**: 
- EKS cluster optimized for demo
- Environment variables set
- Readiness validation

---

## 🎯 Usage Patterns

### Complete GitOps Deployment (Recommended)
```bash
# 1. Initial setup
./scripts/setup/configure-repository.sh

# 2. Deploy everything via GitOps
./scripts/platform/bootstrap-gitops.sh



# 4. Access ArgoCD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

### Development/Testing Workflow
```bash
# 1. Test manifests first
./scripts/infrastructure/test-manifests.sh

# 2. Deploy services directly
./scripts/deploy/deploy-helm.sh

# 4. Deploy applications manually
kubectl apply -k k8s/applications/
```

### Custom/Hybrid Setup
```bash
# 1. Install ArgoCD foundation
./scripts/platform/install-argocd.sh

# 2. Prepare for ACK deployment
./scripts/infrastructure/pre-install-ack.sh

# 3. Deploy services via ArgoCD
kubectl apply -f argocd/applications/

# 4. Monitor in ArgoCD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

### CI/CD Integration
```bash
# Validation pipeline
./scripts/infrastructure/test-manifests.sh

# Deployment pipeline
./scripts/setup/configure-repository.sh
./scripts/platform/bootstrap-gitops.sh

# Health check pipeline
./scripts/deploy/deploy-helm.sh --validate
```

## 🔧 Troubleshooting

### Script Execution Issues

**Permission denied**:
```bash
chmod +x scripts/**/*.sh
```

**kubectl not found**:
```bash
# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

**Cluster connection issues**:
```bash
# Verify cluster access
kubectl cluster-info
kubectl get nodes
```

### Common Script Failures

**ArgoCD installation timeout**:
- Check cluster resources: `kubectl top nodes`
- Verify internet connectivity for image pulls
- Check for conflicting ArgoCD installations

**ACK controller deployment failures**:
- Verify IAM roles and permissions
- Check AWS credentials configuration
- Validate security groups and networking

**Repository configuration issues**:
- Ensure repository is accessible
- For private repos, configure SSH keys or tokens in ArgoCD
- Verify repository URL format

## 📖 Additional Resources

- **Main Setup Guide**: `../SETUP.md`
- **ArgoCD Documentation**: `../k8s/platform/argocd/README.md`
- **Infrastructure Guide**: `../k8s/infrastructure/README.md`
- **Architecture Overview**: `../docs/architecture/`

## 🤝 Contributing

When adding new scripts:

1. **Follow naming conventions**: `verb-noun.sh` (e.g., `install-monitoring.sh`)
2. **Add proper documentation**: Include purpose, usage, prerequisites
3. **Use consistent logging**: Follow the color-coded logging pattern
4. **Add error handling**: Use `set -e` and proper error messages
5. **Update this README**: Document new scripts in the appropriate section

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

# Script implementation here...
```