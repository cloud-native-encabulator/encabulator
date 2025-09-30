# Cloud Native Encabulator - Helm Charts Integration

This directory contains Helm values files and configurations for deploying infrastructure components using community-maintained Helm charts.

## Architecture

```
helm/
├── values/                 # Helm values files
│   ├── argocd/            # ArgoCD Helm values
│   ├── ack-controllers/   # ACK Controllers Helm values
│   └── observability/     # Observability stack Helm values
├── charts/                # Custom Helm charts (if needed)
└── README.md              # This file
```

## Available Community Charts

### ArgoCD
- **Chart**: `argo/argo-cd`
- **Repository**: https://argoproj.github.io/argo-helm
- **Benefits**: Official chart, well-maintained, production-ready

### KRO (Kube Resource Orchestrator) - Future Enhancement
- **Chart**: oci://ghcr.io/kro-run/kro/kro (for future adoption)
- **Repository**: https://github.com/kro-run/kro
- **Benefits**: TBD

### ACK Controllers
- **Chart**: `aws-controllers-k8s/*-chart`
- **Repository**: https://aws-controllers-k8s.github.io/ec2-chart
- **Benefits**: Official AWS charts, per-service installation

### Observability Stack
- **Prometheus**: `prometheus-community/kube-prometheus-stack`
- **Grafana**: Included in kube-prometheus-stack
- **Jaeger**: `jaegertracing/jaeger`
- **Loki**: `grafana/loki-stack`

## Migration Strategy

1. **Phase 1**: Convert ArgoCD to Helm chart ✅
2. **Phase 2**: Convert ACK controllers to Helm charts
3. **Phase 3**: Convert observability stack to Helm charts
4. **Phase 4**: Update ArgoCD applications to use Helm sources
5. **Phase 5**: Optional KRO adoption for advanced resource composition

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

## Benefits

- **Simplified Installation**: `helm install` vs complex YAML
- **Version Management**: Easy upgrades and rollbacks
- **Community Maintenance**: Leverage community expertise
- **Configuration Management**: Values files for different environments
- **GitOps Compatible**: ArgoCD manages Helm charts natively