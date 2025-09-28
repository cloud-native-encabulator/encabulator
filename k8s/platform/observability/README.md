# Cloud Native Encabulator - Observability Platform

This directory contains the KRO-based observability infrastructure for the Cloud Native Encabulator microservices platform.

## Architecture

Our observability stack uses **Kube Resource Orchestrator (KRO)** to create custom APIs that simplify the deployment and management of complex observability infrastructure.

### KRO ResourceGroups

#### Individual Components
- **`ObservabilityMetrics`** - Prometheus-based metrics collection with ServiceMonitors
- **`ObservabilityTracing`** - Jaeger + OpenTelemetry Collector for distributed tracing  
- **`ObservabilityLogs`** - Loki + Promtail for log aggregation and analysis

#### Composite Resource
- **`ObservabilityStack`** - Orchestrates all three components plus Grafana dashboards

## Directory Structure

```
k8s/platform/observability/
├── resource-groups/           # KRO ResourceGroup definitions
│   ├── observability-metrics.yaml
│   ├── observability-tracing.yaml
│   ├── observability-logs.yaml
│   ├── observability-stack.yaml
│   └── kustomization.yaml
├── instances/                 # Deployment instances
│   ├── development-stack.yaml
│   ├── production-stack.yaml
│   └── kustomization.yaml
└── README.md
```

## Usage

### Simple Deployment
Deploy a complete observability stack with a single resource:

```yaml
apiVersion: observability.encabulator.io/v1alpha1
kind: ObservabilityStack
metadata:
  name: my-observability
spec:
  environment: development
  # All components enabled by default with sensible defaults
```

### Customized Deployment
Fine-tune individual components:

```yaml
apiVersion: observability.encabulator.io/v1alpha1
kind: ObservabilityStack
metadata:
  name: custom-observability
spec:
  environment: production
  metrics:
    retention: 30d
    storage: 100Gi
    scrapeInterval: 15s
  tracing:
    sampling:
      strategy: adaptive
      param: 0.01
  logging:
    retention: 14d
    logLevel: warn
  grafana:
    adminPassword: secure-password
```

### Individual Components
Deploy only specific components:

```yaml
apiVersion: observability.encabulator.io/v1alpha1
kind: ObservabilityMetrics
metadata:
  name: metrics-only
spec:
  environment: development
  retention: 7d
```

## Features

### Prometheus Metrics (`ObservabilityMetrics`)
- **Automatic Service Discovery**: Pre-configured ServiceMonitors for:
  - ACK Controllers (AWS Controllers for Kubernetes)
  - KRO Controller
  - ArgoCD components
  - Kubernetes system components
  - Encabulator microservices (via annotations)
- **Alert Rules**: Built-in alerting for infrastructure health
- **Resource Management**: Configurable CPU/memory limits and requests
- **Persistent Storage**: Configurable retention and storage size

### Jaeger Tracing (`ObservabilityTracing`)
- **OpenTelemetry Collector**: Receives traces in multiple formats (OTLP, Jaeger, Zipkin)
- **Flexible Storage**: Memory (dev) or persistent storage (prod)
- **Sampling Strategies**: Probabilistic, constant, rate-limiting, or adaptive
- **Prometheus Integration**: Span metrics exported to Prometheus

### Loki Logging (`ObservabilityLogs`)
- **Promtail DaemonSet**: Collects logs from all cluster nodes
- **Structured Parsing**: JSON log parsing with trace correlation
- **Label Extraction**: Automatic Kubernetes metadata labeling
- **Retention Management**: Configurable retention with compaction
- **Encabulator-Aware**: Special parsing for Encabulator service logs

### Grafana Dashboards (`ObservabilityStack`)
- **Pre-configured Datasources**: Automatic connection to Prometheus, Jaeger, Loki
- **Encabulator Dashboard**: Custom dashboard for system overview
- **Multi-tenancy Ready**: Configurable admin credentials

## Deployment via ArgoCD

The observability stack is deployed through ArgoCD applications:

1. **`observability-resource-groups`** - Deploys KRO ResourceGroup definitions
2. **`observability-stack`** - Deploys the actual observability instances

## Monitoring Targets

The Prometheus configuration automatically discovers and monitors:

### Infrastructure Components
- **Kubernetes API Server** - Cluster health and performance
- **Kubernetes Nodes** - Node resource utilization
- **ACK Controllers** - AWS resource controller health
- **KRO Controller** - Resource orchestration health
- **ArgoCD** - GitOps deployment health

### Application Components
- **Encabulator Services** - Any pod with `prometheus.io/scrape: "true"` annotation
- **Custom Metrics** - Application-specific metrics via `/metrics` endpoints

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

### For Distributed Tracing
Configure OpenTelemetry to send traces to:
- **OTLP gRPC**: `encabulator-observability-tracing-otel-collector:4317`
- **OTLP HTTP**: `encabulator-observability-tracing-otel-collector:4318`
- **Jaeger**: `encabulator-observability-tracing-otel-collector:14268`

### For Structured Logging
Use JSON format with these fields for optimal parsing:
```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "service": "hydrocoptic-datastore",
  "message": "Processing encabulation request",
  "trace_id": "abc123...",
  "span_id": "def456..."
}
```

## Benefits of KRO Approach

1. **Simplified Operations**: Deploy complex observability stack with simple YAML
2. **Consistent Configuration**: Standardized settings across environments
3. **GitOps Ready**: All configuration in Git, managed by ArgoCD
4. **Composable**: Use individual components or full stack as needed
5. **Environment Aware**: Different configurations for dev/staging/prod
6. **Resource Efficient**: Optimized resource allocation per environment
7. **Kubernetes Native**: Leverages Kubernetes APIs and patterns

## Accessing Services

After deployment, access the observability tools:

- **Prometheus**: `kubectl port-forward svc/encabulator-observability-metrics-prometheus 9090:9090`
- **Grafana**: `kubectl port-forward svc/encabulator-observability-grafana 3000:3000`
- **Jaeger**: `kubectl port-forward svc/encabulator-observability-tracing-jaeger 16686:16686`

Default Grafana credentials: `admin` / `encabulator-dev-2024` (development)