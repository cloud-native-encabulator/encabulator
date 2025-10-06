# Malaise Injection Engine

Chaos engineering service for the Cloud Native Encabulator system that injects controlled failures for resilience testing.

## Overview

The Malaise Injection Engine is a Kubernetes-native chaos engineering tool that can inject various types of failures into the system to test resilience and recovery capabilities.

## Features

### Failure Types

1. **Pod Failure**: Terminates pods to simulate crashes
2. **Network Latency**: Introduces network delays (planned)
3. **Resource Stress**: Simulates resource exhaustion (planned)

### Capabilities

- Kubernetes-native failure injection
- Prometheus metrics for observability
- RESTful API for programmatic control
- Integration with Encabulation Control Console

## API Endpoints

### Inject Failure
```bash
POST /inject
Content-Type: application/json

{
  "type": "pod_failure",
  "target": "panametric-fam-processor",
  "duration": 60
}
```

**Failure Types**:
- `pod_failure`: Terminates a pod
- `network_latency`: Introduces network delays
- `resource_stress`: Simulates resource exhaustion

### Health Check
```bash
GET /health
```

### Readiness Check
```bash
GET /ready
```

### Metrics
```bash
GET /metrics
```

## Configuration

### Environment Variables

- `PORT`: Server port (default: `8080`)
- `NAMESPACE`: Target Kubernetes namespace (default: `encabulator-system`)
- `KUBECONFIG`: Path to kubeconfig file (optional, uses in-cluster config if not set)

## Development

### Prerequisites
- Go 1.21+
- Access to a Kubernetes cluster

### Build
```bash
go build -o malaise-injection-engine
```

### Run Locally
```bash
export KUBECONFIG=~/.kube/config
export NAMESPACE=encabulator-system
./malaise-injection-engine
```

### Test
```bash
go test ./...
```

## Docker

Build the image:
```bash
docker build -t malaise-injection-engine:latest .
```

Run the container:
```bash
docker run -p 8080:8080 \
  -v ~/.kube/config:/root/.kube/config \
  -e KUBECONFIG=/root/.kube/config \
  malaise-injection-engine:latest
```

## Kubernetes Deployment

The service is deployed via Helm chart with appropriate RBAC permissions. See `helm/charts/malaise-injection-engine/` for configuration.

### Required RBAC Permissions

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: malaise-injection-engine
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "delete"]
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list"]
```

## Safety Considerations

1. **Target Validation**: Always validates targets before injection
2. **Namespace Isolation**: Operates within configured namespace
3. **Audit Logging**: All actions are logged for audit trail
4. **Metrics**: Prometheus metrics track all injections

## Integration

### Encabulation Control Console
The console provides a UI for triggering failure scenarios through this service.

### Demo Scenario Orchestrator
Automated demo scenarios can trigger failures through this service's API.

## Metrics

- `malaise_injections_total`: Counter of total failure injections by type and target
- `malaise_active_failures`: Gauge of currently active failures

## License

Apache 2.0
