# Cloud Native Encabulator - Observability Platform

This directory contains the Prometheus-based observability infrastructure for the Cloud Native Encabulator microservices platform.

## Architecture

Our observability stack uses **standard Kubernetes manifests** with **Prometheus** for metrics collection and monitoring. The stack is designed to be simple, maintainable, and GitOps-friendly.

### Core Components

- **Prometheus** - Metrics collection with Kubernetes service discovery
- **ServiceMonitors** - Automatic discovery of ACK controllers and services
- **AlertManager** - Alert routing and management (planned)
- **Grafana** - Visualization and dashboards (planned)

## Directory Structure

```
k8s/platform/observability/
├── prometheus/               # Prometheus monitoring stack
│   ├── prometheus-config.yaml
│   ├── prometheus-deployment.yaml
│   └── kustomization.yaml
├── namespace.yaml           # Observability namespace
└── README.md
```

## Current Implementation

### Prometheus Monitoring

The current implementation provides:

- **Kubernetes Service Discovery**: Automatically discovers services and pods
- **ACK Controller Monitoring**: Pre-configured to monitor ACK controllers
- **Pod Annotation Support**: Monitors any pod with `prometheus.io/scrape: "true"`
- **Cluster Metrics**: Kubernetes API server and node metrics
- **Resource Management**: Appropriate CPU/memory limits and requests

### Configuration

Prometheus is configured to scrape:

1. **Kubernetes API Server** - Cluster health and performance
2. **Kubernetes Pods** - Any pod with prometheus.io annotations
3. **ACK Controllers** - AWS resource controller health (when deployed)

## Deployment via ArgoCD

The observability stack is deployed through ArgoCD applications:

1. **`observability-namespace`** - Creates the monitoring namespace
2. **`prometheus-monitoring`** - Deploys Prometheus with service discovery

## Service Integration

### For Encabulator Services

Add these annotations to enable monitoring:

```yaml
metadata:
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/path: "/metrics"
```

### Example Service Configuration

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
spec:
  selector:
    app: my-service
  ports:
  - port: 8080
    targetPort: 8080
```

## Monitoring Targets

The Prometheus configuration automatically discovers and monitors:

### Infrastructure Components

- **Kubernetes API Server** - Cluster health and performance
- **Kubernetes Nodes** - Node resource utilization  
- **ACK Controllers** - AWS resource controller health (when deployed)
- **ArgoCD** - GitOps deployment health (when deployed)

### Application Components

- **Encabulator Services** - Any pod with `prometheus.io/scrape: "true"` annotation
- **Custom Metrics** - Application-specific metrics via `/metrics` endpoints

## Accessing Prometheus

After deployment, access Prometheus:

```bash
kubectl port-forward -n encabulator-monitoring svc/prometheus 9090:9090
```

Then open http://localhost:9090 in your browser.

## Future Enhancements

The following components are planned for future implementation:

### Grafana Dashboards
- Pre-configured datasources for Prometheus
- Encabulator-specific dashboards
- Multi-environment support

### Jaeger Tracing
- OpenTelemetry Collector for trace ingestion
- Distributed tracing for service interactions
- Trace correlation with metrics

### Loki Logging
- Centralized log aggregation
- Structured log parsing
- Log correlation with traces and metrics

### KRO Integration (Advanced)
- Custom ObservabilityStack API for simplified deployment
- Composite resource management
- Advanced resource composition patterns

## Benefits of Current Approach

1. **Simplicity**: Standard Kubernetes manifests are easy to understand
2. **Maintainability**: Clear, readable YAML without custom abstractions
3. **GitOps Ready**: All configuration in Git, managed by ArgoCD
4. **Kubernetes Native**: Uses standard Kubernetes service discovery
5. **Resource Efficient**: Lightweight Prometheus deployment
6. **Extensible**: Easy to add new monitoring targets

## Testing

Test the observability stack:

```bash
# Test observability only
./scripts/test/comprehensive-test.sh --observability-only

# Check Prometheus health
kubectl get pods -n encabulator-monitoring
kubectl logs -n encabulator-monitoring deployment/prometheus
```

## Configuration Management

Prometheus configuration is managed through:

- **ConfigMap**: `prometheus-config` contains the main prometheus.yml
- **Kustomize**: Environment-specific overlays (planned)
- **ArgoCD**: GitOps deployment and synchronization