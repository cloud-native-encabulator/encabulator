# Cloud Native Encabulator - Helm-First Architecture

This directory contains Helm charts, values files, and configurations for deploying all infrastructure and application components using a Helm-first approach with community-maintained and custom charts.

## Architecture

```
helm/
├── charts/                # Custom Helm charts
│   └── hydrocoptic-datastore/  # Custom service chart with templates
├── values/                # Helm values files
│   ├── argocd/           # ArgoCD Helm values
│   ├── ack-controllers/  # ACK Controllers Helm values
│   └── observability/    # Observability stack Helm values
└── README.md             # This file
```

## Chart Categories

### Custom Charts (Application Services)
- **hydrocoptic-datastore**: Custom Encabulator service with PostgreSQL dependency
- **sinusoidal-database**: Time-series service (uses Prometheus chart)
- **prefabulated-cache**: Cache service (uses Redis chart)

### Community Charts (Infrastructure)

#### ArgoCD
- **Chart**: `argo/argo-cd`
- **Repository**: https://argoproj.github.io/argo-helm
- **Benefits**: Official chart, well-maintained, production-ready

#### ACK Controllers (Official AWS Charts)
- **RDS**: `oci://public.ecr.aws/aws-controllers-k8s/rds-chart`
- **ElastiCache**: `oci://public.ecr.aws/aws-controllers-k8s/elasticache-chart`
- **IAM**: `oci://public.ecr.aws/aws-controllers-k8s/iam-chart`
- **Benefits**: Official AWS OCI charts, per-service installation

#### Observability Stack
- **Prometheus Stack**: `prometheus-community/kube-prometheus-stack`
- **Grafana**: `grafana/grafana` (standalone option)
- **Redis**: `bitnami/redis`
- **PostgreSQL**: `bitnami/postgresql`

#### Future Enhancements
- **KRO**: `oci://ghcr.io/kro-run/kro/kro` (for advanced resource composition)
- **Jaeger**: `jaegertracing/jaeger`
- **Loki**: `grafana/loki-stack`

## Implementation Status

1. **Phase 1**: ArgoCD Helm chart integration ✅
2. **Phase 2**: ACK controllers via official OCI Helm charts ✅
3. **Phase 3**: Observability stack via kube-prometheus-stack ✅
4. **Phase 4**: Custom service charts with community dependencies ✅
5. **Phase 5**: ArgoCD applications using Helm sources ✅
6. **Future**: Optional KRO adoption for advanced resource composition

## ArgoCD Helm Integration

ArgoCD Applications can reference Helm charts:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: prometheus-stack
spec:
  source:
    repoURL: https://prometheus-community.github.io/helm-charts
    chart: kube-prometheus-stack
    targetRevision: 45.7.1
    helm:
      valueFiles:
        - values.yaml
  # ... rest of application spec
```

## Benefits of Helm-First Approach

### For Infrastructure
- **Official Charts**: Use AWS-maintained ACK charts and community-maintained observability charts
- **Battle-Tested**: Leverage thousands of hours of community testing and hardening
- **Security**: Built-in security best practices and regular security updates
- **Maintenance**: Reduced maintenance overhead with community-maintained charts

### For Applications
- **Standardization**: Consistent templating patterns across all services
- **Reusability**: Custom charts can be reused across environments
- **Dependencies**: Automatic management of service dependencies (PostgreSQL, Redis)
- **Configuration**: Environment-specific values without template duplication

### For Operations
- **Version Management**: Easy upgrades and rollbacks with semantic versioning
- **GitOps Integration**: ArgoCD manages Helm charts natively with multiple sources
- **Observability**: Built-in ServiceMonitor and dashboard configurations
- **Scaling**: HPA and resource management built into templates

## Custom Chart Development

### Hydrocoptic Data Store Chart Structure
```
helm/charts/hydrocoptic-datastore/
├── Chart.yaml              # Chart metadata and dependencies
├── values.yaml             # Default configuration values
└── templates/
    ├── _helpers.tpl        # Template helpers and labels
    ├── deployment.yaml     # Service deployment
    ├── service.yaml        # Kubernetes service
    ├── configmap.yaml      # Configuration management
    ├── serviceaccount.yaml # RBAC service account
    ├── hpa.yaml           # Horizontal Pod Autoscaler
    └── servicemonitor.yaml # Prometheus monitoring
```

### Key Features
- **PostgreSQL Dependency**: Automatic PostgreSQL deployment or external database configuration
- **Monitoring Integration**: Built-in ServiceMonitor for Prometheus scraping
- **Security**: Non-root containers, read-only filesystems, dropped capabilities
- **Scaling**: HPA configuration with CPU and memory targets
- **Health Checks**: Liveness and readiness probes