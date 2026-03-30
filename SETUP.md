# Cloud Native Encabulator Setup Guide

This guide walks you through setting up the complete Cloud Native Encabulator Microservices Demo using GitOps best practices.

## Prerequisites

- Kubernetes cluster (EKS recommended)
- `kubectl` configured to access your cluster
- `helm` (optional, for direct installation)
- Git repository (GitHub, GitLab, etc.)

## Quick Start

```bash
# 1. Configure your repository
./scripts/setup/configure-repository.sh

# 2. Bootstrap GitOps
./scripts/platform/bootstrap-gitops.sh

# 3. Access ArgoCD
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

## Detailed Setup

### Step 1: Repository Configuration

The system uses GitOps, so you need to configure your Git repository URL:

```bash
./scripts/setup/configure-repository.sh
```

This script will:
- Prompt for your GitHub organization/username
- Update all ArgoCD configurations with your repository URL
- Prepare the system for GitOps deployment

### Step 2: GitOps Bootstrap

Bootstrap the complete system using ArgoCD:

```bash
./scripts/platform/bootstrap-gitops.sh
```

This will:
1. Install ArgoCD
2. Configure projects and repositories
3. Deploy root applications
4. Set up the complete GitOps workflow

### Step 3: Access ArgoCD

Get ArgoCD credentials and access the UI:

```bash
# Get admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Port forward to ArgoCD
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Visit https://localhost:8080
# Username: admin
# Password: (from above command)
```

### Step 4: Monitor Deployment

In the ArgoCD UI, you'll see the deployment progress:

1. **Infrastructure Applications**: ACK controllers, AWS resources
2. **Platform Applications**: Monitoring, networking, namespaces
3. **Application Services**: Microservices (deployed after infrastructure)

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        ArgoCD GitOps                        │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │ Infrastructure  │  │    Platform     │  │ Applications │ │
│  │     Root        │  │      Root       │  │     Root     │ │
│  └─────────────────┘  └─────────────────┘  └──────────────┘ │
│           │                     │                   │       │
│           ▼                     ▼                   ▼       │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │ • ACK Controllers│  │ • Monitoring    │  │ • Data Plane │ │
│  │ • AWS Resources │  │ • Networking    │  │ • Proc Plane │ │
│  │ • KRO           │  │ • Namespaces    │  │ • Supp Plane │ │
│  └─────────────────┘  └─────────────────┘  │ • Ctrl Plane │ │
│                                            └──────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Deployment Order

The system deploys in the following order:

1. **ArgoCD Platform** (Foundation)
2. **Infrastructure** (ACK Controllers, AWS Resources)
3. **Platform Services** (Monitoring, Networking)
4. **Application Services** (Microservices)

This ensures proper dependencies and eliminates bootstrap issues.

## Alternative Installation Methods

### For Development/Testing

If you prefer not to use GitOps for development:

```bash
# Install infrastructure directly
./scripts/infrastructure/install-ack-complete.sh

# Install monitoring (when implemented)
./scripts/monitoring/install-monitoring.sh
```

### For Custom Setups

For custom installations or specific requirements:

```bash
# Install individual components
./scripts/infrastructure/install-ack-crds.sh
```

## Configuration

### AWS Configuration

Before deploying AWS resources, update:

1. **IAM Role ARNs**: Update service account annotations
2. **Security Groups**: Replace placeholder security group IDs
3. **Subnet Groups**: Configure RDS and ElastiCache subnet groups
4. **Account ID**: Update AWS account ID in configurations

### Environment Configuration

The system supports multiple environments:

- **Development**: Single-node, minimal resources
- **Staging**: Multi-AZ, production-like
- **Production**: High availability, encryption, monitoring

Use Kustomize overlays for environment-specific configurations.

## Monitoring and Troubleshooting

### Check System Status

```bash
# ArgoCD applications
kubectl get applications -n argocd

# Infrastructure components
kubectl get pods -n ack-system
kubectl get pods -n encabulator-data

# Application services
kubectl get pods -n encabulator-system
```

### Common Issues

1. **Repository Access**: Ensure ArgoCD can access your Git repository
2. **AWS Permissions**: Verify IAM roles have proper permissions
3. **Resource Limits**: Check cluster has sufficient resources
4. **Network Policies**: Ensure proper network connectivity

### Logs and Debugging

```bash
# ArgoCD logs
kubectl logs -n argocd deployment/argocd-application-controller

# ACK controller logs
kubectl logs -n ack-system deployment/ack-rds-controller

# Application logs
kubectl logs -n encabulator-system deployment/panametric-fam-processor
```

## Security Considerations

### Production Deployment

For production environments:

1. **Enable TLS**: Configure proper certificates for ArgoCD
2. **RBAC**: Implement proper role-based access control
3. **Network Policies**: Restrict network access between components
4. **Secrets Management**: Use external secret management (AWS Secrets Manager, Vault)
5. **Image Security**: Scan container images for vulnerabilities

### Access Control

- Use OIDC/SSO for ArgoCD authentication
- Implement least-privilege IAM roles
- Enable audit logging for all components
- Use private Git repositories with SSH keys

## Next Steps

After successful deployment:

1. **Explore ArgoCD UI**: Monitor application health and sync status
2. **Access Applications**: Use port-forwarding or ingress to access services
3. **Run Demo Scenarios**: Execute built-in demonstration workflows
4. **Customize Configuration**: Adapt to your specific requirements
5. **Scale and Monitor**: Use built-in scaling and monitoring capabilities

## Support and Documentation

- **Architecture**: See `docs/architecture/`
- **API Documentation**: See `docs/api/`
- **Troubleshooting**: See `docs/troubleshooting/`
- **Contributing**: See `CONTRIBUTING.md`

For issues and questions, please check the troubleshooting guide or open an issue in the repository.