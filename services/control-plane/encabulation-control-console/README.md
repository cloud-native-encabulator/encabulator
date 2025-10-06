# Encabulation Control Console

Interactive web interface for controlling and monitoring the Cloud Native Encabulator system.

## Overview

The Encabulation Control Console provides a real-time visual interface for:
- System topology visualization
- Service scaling and control
- Failure injection for chaos engineering
- Demo scenario execution
- Real-time metrics and monitoring

## Architecture

- **Frontend**: React with TypeScript, D3.js for visualizations
- **Backend**: Node.js with Express and Socket.io for real-time communication
- **Kubernetes Integration**: Official Kubernetes client for cluster operations

## Features

### System Topology
- Real-time visualization of all service planes (Data, Processing, Support, Control)
- Service health status indicators
- Replica count display

### Control Panel
- Service scaling (up/down)
- Failure injection for resilience testing
- Demo scenario execution

### Metrics Panel
- Real-time system metrics
- Request rates, latency, resource usage
- Error rates and performance indicators

## Development

### Prerequisites
- Node.js 18+
- npm or yarn
- Access to a Kubernetes cluster (for backend functionality)

### Frontend Development
```bash
npm install
npm run dev
```

### Backend Development
```bash
cd server
npm install
npm run dev
```

### Build
```bash
# Frontend
npm run build

# Backend
cd server
npm run build
```

## Configuration

### Environment Variables

**Frontend** (`.env`):
- `VITE_WS_URL`: WebSocket server URL (default: `ws://localhost:8080/ws`)

**Backend**:
- `PORT`: Server port (default: `8080`)
- `CORS_ORIGIN`: CORS origin (default: `*`)
- `KUBECONFIG`: Path to kubeconfig file (optional, uses in-cluster config if not set)

## Docker

Build the image:
```bash
docker build -t encabulation-control-console:latest .
```

Run the container:
```bash
docker run -p 8080:8080 \
  -v ~/.kube/config:/root/.kube/config \
  encabulation-control-console:latest
```

## Kubernetes Deployment

The service is deployed via Helm chart. See `helm/charts/encabulation-control-console/` for configuration.

## API Endpoints

### HTTP Endpoints
- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /metrics` - Prometheus metrics

### WebSocket Events

**Client → Server**:
```json
{
  "type": "scale_service",
  "payload": { "service": "service-name", "direction": "up|down" }
}
```

```json
{
  "type": "inject_failure",
  "payload": { "service": "service-name" }
}
```

```json
{
  "type": "run_scenario",
  "payload": { "scenario": "scenario-name" }
}
```

**Server → Client**:
```json
{
  "type": "system_status",
  "payload": {
    "services": { "service-name": { "status": "healthy", "replicas": 3 } },
    "metrics": { "request_rate": 100, "avg_latency": 50 }
  }
}
```

## Integration

### Malaise Injection Engine
The console integrates with the Malaise Injection Engine for chaos engineering operations.

### Demo Scenario Orchestrator
Pre-built demo scenarios are executed through the Demo Scenario Orchestrator service.

### Unilateral Phase Detractor Engine
Traffic simulation and load generation is controlled through this service.

## License

Apache 2.0
