# ArgoCD GitOps Platform

This directory contains the ArgoCD installation and configuration for the Cloud Native Encabulator Microservices Demo. ArgoCD serves as the GitOps engine that manages the entire system deployment and lifecycle.

## Architecture

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

## Components

### Core Files

- `namespace.yaml` - ArgoCD namespace definition
- `argocd-install.yaml` - References official ArgoCD installation
- `argocd-config.yaml` - ArgoCD server and repository configuration
- `argocd-projects.yaml` - Project definitions for RBAC and organization
- `argocd-applications.yaml` - Root applications that manage the entire system

### Projects

1. **infrastructure** - Platform and infrastructure components

   - ACK Controllers
   - AWS Resources
   - KRO (Kube Resource Orchestrator)
   - Monitoring stack

2. **cloud-native-encabulator** - Application components
   - Data Plane services
   - Processing Plane services
   - Support Plane services
   - Control Plane services

### Root Applications

1. **infrastructure-root** - Manages all infrastructure components
2. **platform-root** - Manages platform services (monitoring, networking)
3. **applications-root** - Manages Cloud Native Encabulator applications

## Installation

### Quick Start

```bash
# Bootstrap complete GitOps setup
./scripts/platform/bootstrap-gitops.sh
```

### Manual Installation

```bash
# 1. Install ArgoCD
./scripts/platform/install-argocd.sh

# 2. Deploy root applications
kubectl apply -f k8s/platform/argocd/argocd-applications.yaml

# 3. Access ArgoCD
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

### Access ArgoCD

1. **Get admin password**:

   ```bash
   kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
   ```

2. **Port forward**:

   ```bash
   kubectl port-forward svc/argocd-server -n argocd 8080:443
   ```

3. **Login**:
   - URL: https://localhost:8080
   - Username: `admin`
   - Password: (from step 1)

## Configuration

### Repository Configuration

**Important**: Before deploying, configure your repository URL:

```bash
./scripts/setup/configure-repository.sh
```

This updates all ArgoCD configurations with your actual repository URL, replacing `REPLACE_WITH_YOUR_ORG` placeholders.

### OCI Registry Support

ArgoCD is pre-configured to support OCI Helm charts from AWS ECR Public for ACK controllers:

```yaml
repositories: |
  - type: helm
    url: oci://public.ecr.aws/aws-controllers-k8s
    name: aws-controllers-k8s
    enableOCI: "true"
```

This enables direct deployment of ACK controllers using official OCI Helm charts.

### RBAC Configuration

Two main roles are configured:

- **admin** - Full access to all applications and repositories
- **developer** - Read and sync access to applications

## GitOps Workflow

### Application Hierarchy

```
Root Applications
├── infrastructure-root
│   ├── ACK Controllers (RDS, ElastiCache, IAM)
│   ├── AWS Resources (PostgreSQL, Redis, IAM Roles)
│   └── KRO (Resource Orchestration)
├── platform-root
│   ├── Monitoring (Prometheus, Grafana, Jaeger)
│   ├── Networking (Ingress, Service Mesh)
│   └── Namespaces
└── applications-root
    ├── Data Plane (Hydrocoptic Data Store, etc.)
    ├── Processing Plane (Panametric Fam Processor, etc.)
    ├── Support Plane (Cardinal Grammeter, etc.)
    └── Control Plane (Encabulation Control Console, etc.)
```

### Sync Policies

- **Automated Sync**: All applications sync automatically from Git
- **Self-Heal**: Applications automatically correct drift
- **Prune**: Unused resources are automatically removed
- **Retry Logic**: Failed syncs are retried with exponential backoff

## Monitoring and Troubleshooting

### Check Application Status

```bash
# List all applications
kubectl get applications -n argocd

# Get application details
kubectl describe application <app-name> -n argocd

# Check sync status
argocd app list  # (requires argocd CLI)
```

### Common Issues

1. **Application not syncing**:

   ```bash
   kubectl logs -n argocd deployment/argocd-application-controller
   ```

2. **Repository access issues**:

   ```bash
   kubectl logs -n argocd deployment/argocd-repo-server
   ```

3. **Server issues**:
   ```bash
   kubectl logs -n argocd deployment/argocd-server
   ```

### Health Checks

```bash
# Check ArgoCD components
kubectl get pods -n argocd

# Check application health
kubectl get applications -n argocd -o wide
```

## Security Considerations

### Development vs Production

Current configuration is set for development:

- `server.insecure: "true"` - Disables TLS
- Self-signed certificates accepted

For production:

1. Enable TLS with proper certificates
2. Configure OIDC/SSO integration
3. Implement proper RBAC policies
4. Use private Git repositories with SSH keys
5. Enable audit logging

### RBAC

Projects provide isolation and access control:

- Infrastructure project for platform components
- Application project for business applications
- Role-based access for different user types

## Backup and Recovery

### Backup ArgoCD Configuration

```bash
# Export applications
kubectl get applications -n argocd -o yaml > argocd-applications-backup.yaml

# Export projects
kubectl get appprojects -n argocd -o yaml > argocd-projects-backup.yaml

# Export configuration
kubectl get configmaps -n argocd -o yaml > argocd-config-backup.yaml
```

### Disaster Recovery

1. Reinstall ArgoCD using installation scripts
2. Restore configuration from backups
3. Applications will automatically sync from Git
4. Verify all applications are healthy

## Integration with Cloud Native Encabulator

ArgoCD manages the entire Cloud Native Encabulator system:

1. **Infrastructure First**: ACK controllers and AWS resources
2. **Platform Services**: Monitoring and observability
3. **Application Services**: Microservices in proper dependency order
4. **Configuration Management**: Environment-specific overlays
5. **Continuous Deployment**: Automatic updates from Git changes

This GitOps approach ensures:

- Declarative infrastructure and application management
- Version-controlled system state
- Automated deployment and rollback capabilities
- Audit trail of all changes
- Consistent environments across dev/staging/production
