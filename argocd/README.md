# ArgoCD GitOps Configuration

This directory contains ArgoCD Application manifests for deploying the Cloud Native Encabulator platform using GitOps principles.

## Directory Structure

```
argocd/
├── README.md                    # This file
├── root-app.yaml               # App-of-Apps root application
└── apps/                       # Individual Application manifests
    ├── control-plane/          # Control plane services
    ├── processing-plane/       # Processing plane services
    ├── data-plane/            # Data plane services
    ├── support-plane/         # Support plane services
    └── monitoring/            # Monitoring stack
```

## Architecture: App-of-Apps Pattern

We use the **App-of-Apps pattern** where:

1. **Root Application** (`root-app.yaml`) - Parent application that manages all child applications
2. **Child Applications** (`apps/*/`) - Individual service applications that reference Helm charts

### Why App-of-Apps?

- **Explicit control** - Each service has its own Application manifest
- **Easy to understand** - Clear structure and relationships
- **Flexible sync policies** - Different policies per service
- **Educational value** - Great for learning GitOps concepts
- **Manageable scale** - Works well for 12+ microservices

## Deployment Flow

```
Git Commit → ArgoCD Detects Change → Sync Application → Deploy to Kubernetes
```

### Manual Deployment (Initial Setup)

```bash
# 1. Deploy root application
kubectl apply -f argocd/root-app.yaml

# 2. ArgoCD will automatically create all child applications
argocd app list

# 3. Sync applications (manual sync initially for safety)
argocd app sync root-app
argocd app sync <service-name>

# Or sync all at once
argocd app sync --all
```

### GitOps Workflow (After Setup)

```bash
# 1. Make changes to Helm values or charts
vim helm/charts/encabulation-control-console/values.yaml

# 2. Commit and push
git add helm/charts/encabulation-control-console/values.yaml
git commit -m "Update control console replicas"
git push

# 3. ArgoCD detects change and syncs automatically (if auto-sync enabled)
# Or manually sync via UI or CLI
argocd app sync encabulation-control-console
```

## Service Planes

### Control Plane (`apps/control-plane/`)

Services that provide control and orchestration:

- **encabulation-control-console** - Web UI with real-time visualization
- **cardinal-grammeter** - Traffic generation and load testing
- **malaise-injection-engine** - Chaos engineering operator
- **demo-scenario-orchestrator** - Scenario management and automation

### Processing Plane (`apps/processing-plane/`)

Core business logic and API services:

- **dingle-arm-api-gateway** - API gateway with rate limiting
- **panametric-fam-processor** - Core encabulation processing
- **logarithmic-casing-service** - Message broker wrapper

### Data Plane (`apps/data-plane/`)

Data storage and caching services:

- **hydrocoptic-datastore** - PostgreSQL wrapper with REST API
- **prefabulated-cache** - Redis wrapper with REST API
- **sinusoidal-database** - InfluxDB wrapper with REST API

### Support Plane (`apps/support-plane/`)

Supporting services for auth, config, and optimization:

- **unilateral-phase-detractor** - Authentication and authorization
- **differential-girdle** - Configuration management
- **side-fumbling-collector** - Resource optimization

### Monitoring (`apps/monitoring/`)

Observability stack:

- **prometheus-stack** - Prometheus, Grafana, AlertManager

## Application Manifest Structure

Each Application manifest follows this structure:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: <service-name>
  namespace: argocd
spec:
  project: default
  source:
    repoURL: <git-repo-url>
    targetRevision: HEAD
    path: helm/charts/<service-name>
    helm:
      valueFiles:
        - values.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: <target-namespace>
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

## Sync Policies

### Manual Sync (Initial Setup)

For safety during migration, we start with manual sync:

```yaml
syncPolicy:
  syncOptions:
    - CreateNamespace=true
```

### Automated Sync (Production)

After validation, enable auto-sync:

```yaml
syncPolicy:
  automated:
    prune: true      # Delete resources not in Git
    selfHeal: true   # Revert manual changes
  syncOptions:
    - CreateNamespace=true
```

## Sync Waves

We use sync waves to ensure proper deployment order:

1. **Wave 0** - Data plane (databases must be ready first)
2. **Wave 1** - Support plane (auth and config services)
3. **Wave 2** - Processing plane (business logic services)
4. **Wave 3** - Control plane (UI and control services)
5. **Wave 4** - Monitoring (observability stack)

Sync waves are defined using annotations:

```yaml
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "0"
```

## Health Checks

ArgoCD automatically monitors application health using:

- **Kubernetes health checks** - Pod readiness and liveness
- **Custom health checks** - For CRDs and operators
- **Resource status** - Deployment, StatefulSet, Service status

## Troubleshooting

### Application Out of Sync

```bash
# Check sync status
argocd app get <app-name>

# View differences
argocd app diff <app-name>

# Force sync
argocd app sync <app-name> --force
```

### Sync Failures

```bash
# View sync logs
argocd app logs <app-name>

# Check application events
kubectl get events -n <namespace>

# View ArgoCD controller logs
kubectl logs -n argocd statefulset/argocd-application-controller
```

### Application Stuck

```bash
# Delete and recreate application
argocd app delete <app-name>
kubectl apply -f argocd/apps/<plane>/<app-name>.yaml

# Or refresh application
argocd app get <app-name> --refresh
```

## Best Practices

1. **Start with manual sync** - Validate before enabling auto-sync
2. **Use sync waves** - Ensure proper deployment order
3. **Enable pruning carefully** - Understand what will be deleted
4. **Test in dev first** - Validate changes before production
5. **Use Git tags** - Tag releases for easy rollback
6. **Monitor sync status** - Set up alerts for sync failures
7. **Document changes** - Use meaningful commit messages

## Migration from Helm-First

The `helm-first` branch preserves the original Helm-only deployment approach. Key differences:

| Aspect | Helm-First | GitOps (ArgoCD) |
|--------|-----------|-----------------|
| Deployment | Manual `helm upgrade` | Git commit + auto-sync |
| State | Helm releases | Git repository |
| Rollback | `helm rollback` | Git revert |
| Audit | Helm history | Git history |
| Access | Cluster access required | Git access sufficient |

## References

- **ArgoCD Docs**: https://argo-cd.readthedocs.io/
- **App-of-Apps Pattern**: https://argo-cd.readthedocs.io/en/stable/operator-manual/cluster-bootstrapping/
- **Sync Waves**: https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/
- **Health Checks**: https://argo-cd.readthedocs.io/en/stable/operator-manual/health/

## Next Steps

1. ✅ Directory structure created
2. ⏭️ Create Application manifests for all services
3. ⏭️ Create root App-of-Apps application
4. ⏭️ Deploy and validate GitOps workflow
5. ⏭️ Enable auto-sync after validation
