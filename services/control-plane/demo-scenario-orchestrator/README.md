# Demo Scenario Orchestrator

Automated demonstration scenario management and execution for the Cloud Native Encabulator system.

## Overview

The Demo Scenario Orchestrator provides pre-built demonstration scenarios that showcase various capabilities of the Cloud Native Encabulator system. It coordinates with other Control Plane services to execute complex, multi-step demonstrations.

## Features

### Predefined Scenarios

1. **System Startup**: Demonstrates the system startup sequence and health checks
2. **Data Flow Demo**: Shows data flowing through the entire system
3. **Failure Recovery**: Demonstrates resilience and recovery from failures
4. **Auto-scaling Demo**: Shows horizontal pod autoscaling in action

### Capabilities

- Multi-step scenario execution
- Integration with traffic generation and chaos engineering
- Real-time status tracking
- Kubernetes-native operations
- Prometheus metrics

## API Endpoints

### List Scenarios
```bash
GET /scenarios
```

Response:
```json
{
  "scenarios": [
    {
      "name": "System Startup",
      "description": "Demonstrates the system startup sequence",
      "steps": 3
    }
  ]
}
```

### Get Scenario Details
```bash
GET /scenarios/{scenario_name}
```

### Execute Scenario
```bash
POST /scenarios/{scenario_name}/execute
```

Response:
```json
{
  "name": "System Startup",
  "status": "running",
  "current_step": 2,
  "total_steps": 3,
  "message": "Executing step 2: check_services"
}
```

### Get Scenario Status
```bash
GET /scenarios/status
```

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

## Scenario Structure

Each scenario consists of multiple steps:

```python
{
  "name": "Scenario Name",
  "description": "Scenario description",
  "steps": [
    {
      "action": "log",
      "target": "console",
      "parameters": {"message": "Starting..."},
      "delay": 0
    },
    {
      "action": "generate_traffic",
      "target": "dingle-arm-api-gateway",
      "parameters": {"pattern": "steady", "duration": 30},
      "delay": 5
    }
  ]
}
```

### Available Actions

- `log`: Log a message
- `check_services`: Verify service health
- `generate_traffic`: Start traffic generation
- `inject_failure`: Inject a failure
- `wait`: Wait for a specified duration
- `monitor_scaling`: Monitor deployment scaling

## Configuration

### Environment Variables

- `PORT`: Server port (default: `8080`)
- `NAMESPACE`: Target Kubernetes namespace (default: `encabulator-system`)
- `TRAFFIC_ENGINE_URL`: Unilateral Phase Detractor Engine URL (default: `http://unilateral-phase-detractor:8080`)
- `MALAISE_ENGINE_URL`: Malaise Injection Engine URL (default: `http://malaise-injection-engine:8080`)
- `KUBECONFIG`: Path to kubeconfig file (optional, uses in-cluster config if not set)

## Development

### Prerequisites
- Python 3.12+
- Access to a Kubernetes cluster

### Install Dependencies
```bash
pip install -r requirements.txt
```

### Run Development Server
```bash
python main.py
```

### Run with Uvicorn
```bash
uvicorn main:app --reload --port 8080
```

## Docker

Build the image:
```bash
docker build -t demo-scenario-orchestrator:latest .
```

Run the container:
```bash
docker run -p 8080:8080 \
  -v ~/.kube/config:/root/.kube/config \
  -e KUBECONFIG=/root/.kube/config \
  demo-scenario-orchestrator:latest
```

## Kubernetes Deployment

The service is deployed via Helm chart with appropriate RBAC permissions. See `helm/charts/demo-scenario-orchestrator/` for configuration.

### Required RBAC Permissions

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: demo-scenario-orchestrator
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
```

## Integration

### Encabulation Control Console
The console provides a UI for selecting and executing scenarios.

### Unilateral Phase Detractor Engine
Traffic generation is controlled through this service during scenarios.

### Malaise Injection Engine
Failure injection is triggered through this service during resilience scenarios.

## Metrics

- `scenario_executions_total`: Counter of scenario executions by scenario and status
- `scenario_active_scenarios`: Gauge of currently active scenarios

## Educational Value

The Demo Scenario Orchestrator demonstrates:
- Automated testing and demonstration workflows
- Service orchestration and coordination
- Integration testing patterns
- Chaos engineering practices
- System observability

## License

Apache 2.0
