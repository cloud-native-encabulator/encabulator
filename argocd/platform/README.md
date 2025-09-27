# Platform ArgoCD Applications

This directory contains ArgoCD Applications that manage the platform layer of the Cloud Native Encabulator system. Platform services provide cross-cutting concerns like monitoring, networking, and namespace management.

## Components

### `namespaces.yaml`
**Purpose**: Kubernetes namespace management  
**What it manages**:
- Application namespaces (`encabulator-system`, `encabulator-data`, etc.)
- Namespace labels and annotations
- Resource quotas and limits
- Network policies

**Source**: Git repository `k8s/platform/namespaces/`

### `monitoring.yaml`
**Purpose**: Observability and monitoring stack  
**What it manages**:
- Prometheus for metrics collection
- Grafana for visualization
- Jaeger for distributed tracing
- AlertManager for alerting

**Source**: Git repository `k8s/platform/monitoring/`

**Target Namespace**: `encabulator-monitoring`

### `networking.yaml`
**Purpose**: Network infrastructure and ingress  
**What it manages**:
- Ingress controllers
- Service mesh components (if applicable)
- Network policies
- Load balancers and external access

**Source**: Git repository `k8s/platform/networking/`

## Dependencies

Platform applications depend on infrastructure being deployed first:

1. **Infrastructure** (ACK controllers, AWS resources) ← Must be deployed first
2. **Platform** (namespaces, monitoring, networking) ← This layer
3. **Applications** (microservices) ← Depends on platform

## Deployment Order

Within the platform layer:

1. **Namespaces** (creates required namespaces)
2. **Monitoring** (observability foundation)
3. **Networking** (ingress and connectivity)

## Configuration

### Prerequisites

1. **Infrastructure deployed**: ACK controllers and AWS resources must be running
2. **Repository configured**: Run `./scripts/setup/configure-repository.sh`
3. **Cluster resources**: Ensure sufficient CPU/memory for monitoring stack

### Environment-Specific Configuration

Platform services support environment-specific overlays:

```
k8s/platform/
├── base/              # Base configurations
├── overlays/
│   ├── development/   # Dev-specific settings
│   ├── staging/       # Staging configuration
│   └── production/    # Production settings
```

## Deployment

### Via GitOps (Recommended)
```bash
# Deploy all platform services via root application
./scripts/platform/bootstrap-gitops.sh
```

### Manual Deployment
```bash
# Deploy all platform applications
kubectl apply -k argocd/platform/

# Monitor deployment
kubectl get applications -n argocd -l app.kubernetes.io/component=platform
```

### Individual Components
```bash
# Deploy namespaces first
kubectl apply -f argocd/platform/namespaces.yaml

# Deploy monitoring
kubectl apply -f argocd/platform/monitoring.yaml

# Deploy networking
kubectl apply -f argocd/platform/networking.yaml
```

## Monitoring

### Check Application Status
```bash
# List platform applications
kubectl get applications -n argocd -l app.kubernetes.io/component=platform

# Check sync status
kubectl get applications -n argocd -o wide
```

### Check Platform Resources
```bash
# Namespaces
kubectl get namespaces -l app.kubernetes.io/managed-by=argocd

# Monitoring stack
kubectl get pods -n encabulator-monitoring

# Networking components
kubectl get ingress --all-namespaces
kubectl get services --all-namespaces
```

## Access Platform Services

### Monitoring Stack
```bash
# Access Grafana
kubectl port-forward -n encabulator-monitoring svc/grafana 3000:80

# Access Prometheus
kubectl port-forward -n encabulator-monitoring svc/prometheus 9090:9090

# Access Jaeger
kubectl port-forward -n encabulator-monitoring svc/jaeger 16686:16686
```

### Get Service URLs
```bash
# List all services with external access
kubectl get ingress --all-namespaces
kubectl get services --all-namespaces --field-selector spec.type=LoadBalancer
```

## Observability

The monitoring stack provides comprehensive observability:

### Metrics (Prometheus)
- Infrastructure metrics (nodes, pods, containers)
- Application metrics (custom business metrics)
- ACK controller metrics
- AWS resource metrics

### Visualization (Grafana)
- Pre-built dashboards for infrastructure
- Application-specific dashboards
- Custom dashboards for Cloud Native Encabulator

### Tracing (Jaeger)
- Distributed tracing across microservices
- Request flow visualization
- Performance analysis

### Alerting (AlertManager)
- Infrastructure alerts (resource usage, failures)
- Application alerts (error rates, latency)
- Integration with notification channels

## Troubleshooting

### Common Issues

1. **Monitoring stack not starting**:
   - Check cluster resources: `kubectl top nodes`
   - Verify persistent volume availability
   - Check image pull issues

2. **Ingress not working**:
   - Verify ingress controller is running
   - Check DNS configuration
   - Validate TLS certificates

3. **Network policies blocking traffic**:
   - Review network policy configurations
   - Check pod-to-pod connectivity
   - Validate service discovery

### Debugging Commands
```bash
# Check platform application logs
kubectl logs -n argocd deployment/argocd-application-controller

# Check monitoring stack
kubectl get pods -n encabulator-monitoring
kubectl logs -n encabulator-monitoring deployment/prometheus

# Check networking
kubectl get networkpolicies --all-namespaces
kubectl describe ingress <ingress-name>
```

## Security Considerations

### Network Security
- Network policies restrict inter-pod communication
- Ingress controllers use TLS termination
- Service mesh provides mTLS (if enabled)

### Monitoring Security
- Grafana authentication and authorization
- Prometheus access controls
- Secure metric collection endpoints

### Secrets Management
- TLS certificates stored as Kubernetes Secrets
- Monitoring credentials managed securely
- Integration with external secret management

## Customization

### Adding Custom Dashboards
1. Create dashboard JSON in `k8s/platform/monitoring/dashboards/`
2. Update ConfigMap to include new dashboard
3. Commit changes for GitOps deployment

### Custom Metrics
1. Add ServiceMonitor resources for new applications
2. Configure Prometheus scraping rules
3. Create alerts for custom metrics

### Network Policies
1. Define policies in `k8s/platform/networking/policies/`
2. Test connectivity between services
3. Update policies as needed for new services

## Next Steps

After platform deployment:

1. **Verify all platform services are healthy**
2. **Access monitoring dashboards**
3. **Deploy applications** (`argocd/applications/`)
4. **Configure custom dashboards and alerts**
5. **Set up external access via ingress**