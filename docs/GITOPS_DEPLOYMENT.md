# GitOps Deployment Guide

**Status:** ✅ Active (as of v0.2.0)  
**Deployment Method:** ArgoCD GitOps  
**Previous Method:** Helm-first (preserved in `helm-first` branch)

## Overview

The Cloud Native Encabulator platform uses **ArgoCD** for GitOps-based continuous deployment. All infrastructure and application changes are managed through Git commits, providing:

- **Declarative deployments** - Infrastructure as code
- **Automated sync** - Changes deploy automatically
- **Audit trail** - Full Git history of all changes
- **Easy rollback** - Git revert to previous state
- **Self-healing** - Cluster drift automatically corrected

## Prerequisites

- EKS cluster with kubectl access
- ArgoCD installed (see [ArgoCD Setup](#argocd-setup))
- Git repository access
- GitHub personal access token (for private repos)

## Quick Start

### 1. Access ArgoCD UI

```bash
# Start port-forward
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Access UI at https://localhost:8080
# Username: admin
# Password: (see .kiro/ARGOCD_ACCESS.md)
```

### 2. Deploy All Services

```bash
# Deploy root application (manages all child apps)
kubectl apply -f argocd/root-app.yaml

# ArgoCD will automatically:
# - Create 14 child applications
# - Sync all services from Git
# - Deploy in proper order (sync waves)
```

### 3. Verify Deployment

```bash
# List all applications
argocd app list

# Check specific application
argocd app get encabulation-control-console

# View sync status
argocd app sync-status encabulation-control-console
```

## GitOps Workflow

### Making Changes

```bash
# 1. Make changes to Helm values or charts
vim helm/charts/encabulation-control-console/values.yaml

# 2. Commit and push
git add helm/charts/encabulation-control-console/values.yaml
git commit -m "Scale control console to 3 replicas"
git push origin main

# 3. ArgoCD automatically detects and syncs (within 3 minutes)
# Or manually trigger sync:
argocd app sync encabulation-control-console
```

### Rollback

```bash
# Option 1: Git revert (recommended)
git revert <commit-hash>
git push origin main
# ArgoCD will automatically sync to previous state

# Option 2: ArgoCD rollback
argocd app rollback encabulation-control-console <history-id>
```

## Application Structure

### App-of-Apps Pattern

```
root-app (parent)
├── Control Plane (sync wave 3)
│   ├── encabulation-control-console
│   ├── cardinal-grammeter
│   ├── malaise-injection-engine
│   └── demo-scenario-orchestrator
├── Processing Plane (sync wave 2)
│   ├── dingle-arm-api-gateway
│   ├── panametric-fam-processor
│   └── logarithmic-casing-service
├── Data Plane (sync wave 0)
│   ├── hydrocoptic-datastore
│   ├── prefabulated-cache
│   └── sinusoidal-database
└── Support Plane (sync wave 1)
    ├── unilateral-phase-detractor
    ├── differential-girdle
    └── side-fumbling-collector
```

### Sync Waves

Services deploy in order based on sync wave annotations:

- **Wave 0** - Data plane (databases first)
- **Wave 1** - Support plane (auth, config)
- **Wave 2** - Processing plane (business logic)
- **Wave 3** - Control plane (UI, control)
- **Wave 4** - Monitoring (observability)

## Sync Policies

All applications use automated sync with:

- **Auto-sync**: Changes in Git automatically deploy
- **Prune**: Resources not in Git are deleted
- **Self-heal**: Manual cluster changes are reverted

```yaml
syncPolicy:
  automated:
    prune: true
    selfHeal: true
  syncOptions:
    - CreateNamespace=true
```

## ArgoCD Setup

### Install ArgoCD

```bash
# Create namespace
kubectl create namespace argocd

# Install ArgoCD
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s
```

### Configure Repository Access

```bash
# For private repositories, add credentials
argocd repo add https://github.com/jlbutler/cloud-native-encabulator.git \
  --username <github-username> \
  --password <github-token> \
  --name cloud-native-encabulator
```

### Get Admin Password

```bash
# Retrieve initial admin password
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" | base64 -d && echo
```

## Common Operations

### View Application Status

```bash
# List all applications
argocd app list

# Get detailed application info
argocd app get <app-name>

# View application logs
argocd app logs <app-name>

# View sync history
argocd app history <app-name>
```

### Manual Sync

```bash
# Sync single application
argocd app sync <app-name>

# Sync with prune
argocd app sync <app-name> --prune

# Dry run (preview changes)
argocd app sync <app-name> --dry-run
```

### Troubleshooting

```bash
# View sync errors
argocd app get <app-name>

# View application events
kubectl get events -n <namespace>

# View ArgoCD controller logs
kubectl logs -n argocd statefulset/argocd-application-controller

# Refresh application (force Git fetch)
argocd app get <app-name> --refresh

# Hard refresh (clear cache)
argocd app get <app-name> --hard-refresh
```

## Monitoring

### Application Health

ArgoCD monitors application health using:

- **Kubernetes health checks** - Pod readiness/liveness
- **Resource status** - Deployment, Service, etc.
- **Custom health checks** - For CRDs

Health statuses:
- **Healthy** - All resources healthy
- **Progressing** - Resources being created/updated
- **Degraded** - Some resources unhealthy
- **Missing** - Resources not found

### Sync Status

Sync statuses:
- **Synced** - Git matches cluster
- **OutOfSync** - Git differs from cluster
- **Unknown** - Unable to determine status

## Migration from Helm-First

The previous Helm-first deployment approach is preserved in the `helm-first` branch for reference.

### Key Differences

| Aspect | Helm-First | GitOps (ArgoCD) |
|--------|-----------|-----------------|
| Deployment | Manual `helm upgrade` | Git commit + auto-sync |
| State | Helm releases | Git repository |
| Rollback | `helm rollback` | Git revert |
| Audit | Helm history | Git history |
| Access | Cluster access required | Git access sufficient |

### Migration Process

1. ✅ Preserved Helm-first in `helm-first` branch
2. ✅ Installed ArgoCD
3. ✅ Created Application manifests
4. ✅ Deployed root application
5. ✅ ArgoCD adopted existing resources (zero downtime)
6. ✅ Enabled automated sync policies

## Best Practices

1. **Always commit before deploying** - Git is source of truth
2. **Use meaningful commit messages** - They appear in ArgoCD UI
3. **Test in dev first** - Validate changes before production
4. **Use sync waves** - Ensure proper deployment order
5. **Monitor sync status** - Set up alerts for failures
6. **Review diffs before sync** - Use `argocd app diff`
7. **Use Git tags for releases** - Easy rollback points

## Security

### Repository Access

- Use GitHub personal access tokens (not passwords)
- Rotate tokens regularly
- Use read-only tokens when possible
- Store tokens in Kubernetes secrets

### RBAC

ArgoCD supports role-based access control:

```bash
# View current user permissions
argocd account get-user-info

# List available roles
argocd account list
```

## References

- **ArgoCD Docs**: https://argo-cd.readthedocs.io/
- **App-of-Apps Pattern**: https://argo-cd.readthedocs.io/en/stable/operator-manual/cluster-bootstrapping/
- **Sync Waves**: https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/
- **Health Checks**: https://argo-cd.readthedocs.io/en/stable/operator-manual/health/
- **ArgoCD Access Info**: `.kiro/ARGOCD_ACCESS.md`
- **Helm-First Branch**: `helm-first` branch (preserved for reference)

## Support

For issues or questions:

1. Check ArgoCD UI for sync errors
2. Review application logs: `argocd app logs <app-name>`
3. Check ArgoCD controller logs
4. Consult ArgoCD documentation
5. Review Git commit history for recent changes
