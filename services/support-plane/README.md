# Support Plane Services

The Support Plane contains three microservices that provide cross-cutting concerns including feature flags, metrics collection, and telemetry for the Cloud Native Encabulator system.

## Services

### 1. Differential Girdle
Feature flag and configuration management service providing dynamic feature toggles.

**Technology**: Go 1.21  
**Port**: 8080  
**Purpose**: Feature flags and configuration management

[Service Documentation](./differential-girdle/README.md)

### 2. Cardinal Grammeter
Metrics collection and aggregation service for custom business metrics.

**Technology**: Python + FastAPI  
**Port**: 8080  
**Purpose**: Custom metrics collection and aggregation

[Service Documentation](./cardinal-grammeter/README.md)

### 3. Side Fumbling Collector
Telemetry and observability data collection service.

**Technology**: Python + FastAPI  
**Port**: 8080  
**Purpose**: Telemetry collection and forwarding

[Service Documentation](./side-fumbling-collector/README.md)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Support Plane                             │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │   Differential Girdle (Go)                           │  │
│  │   - Feature flags                                     │  │
│  │   - Configuration management                          │  │
│  │   - Dynamic toggles                                   │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │   Cardinal Grammeter (Python)                        │  │
│  │   - Custom metrics collection                         │  │
│  │   - Business metrics aggregation                      │  │
│  │   - Metrics forwarding                                │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │   Side Fumbling Collector (Python)                   │  │
│  │   - Telemetry collection                              │  │
│  │   - Trace aggregation                                 │  │
│  │   - Log forwarding                                    │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                               │
└───────────────────────────────┬───────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Observability Stack  │
                    │ (Prometheus, Grafana) │
                    └───────────────────────┘
```

## Service Directory Structure

### Go Service (Differential Girdle)
```
services/support-plane/differential-girdle/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/            # HTTP handlers
│   ├── config/         # Configuration management
│   └── models/         # Data models
├── pkg/health/         # Health check utilities
├── Dockerfile          # Container image definition
├── Makefile            # Build automation
├── go.mod              # Go module definition
└── README.md           # Service documentation
```

### Python Services (Cardinal Grammeter, Side Fumbling Collector)
```
services/support-plane/<service-name>/
├── app/
│   ├── main.py         # Application entry point
│   ├── routes/         # API routes
│   ├── services/       # Business logic
│   └── models/         # Data models
├── Dockerfile          # Container image definition
├── Makefile            # Build automation
├── requirements.txt    # Python dependencies
└── README.md           # Service documentation
```

## Common Features

All Support Plane services implement:

- ✅ REST API with standardized error responses
- ✅ Health (`/health`) and readiness (`/ready`) endpoints
- ✅ Prometheus metrics exposure (`/metrics`)
- ✅ Structured JSON logging
- ✅ Graceful shutdown handling
- ✅ Environment-based configuration
- ✅ Integration with observability stack

## Building

### Build All Services

```bash
# From repository root
make build-support-plane

# Or from this directory
make build
```

### Build Individual Service

#### Go Service (Differential Girdle)
```bash
cd services/support-plane/differential-girdle
make build
# Or manually:
go mod tidy
go build -o differential-girdle ./cmd/server
```

#### Python Services
```bash
cd services/support-plane/<service-name>
make build
# Or manually:
pip install -r requirements.txt
```

### Build Docker Images

```bash
# From repository root
make image-support-plane

# Or from this directory
make image
```

## Testing

### Run All Tests

```bash
# From repository root
make test-support-plane

# Or from this directory
make test
```

### Run Individual Service Tests

#### Go Service
```bash
cd services/support-plane/differential-girdle
go test ./...

# With coverage
go test -cover ./...
```

#### Python Services
```bash
cd services/support-plane/<service-name>
pytest

# With coverage
pytest --cov=app
```

**Note**: Python tests require pytest to be installed. If pytest is not available, tests will be skipped.

## Deployment

All Support Plane services are deployed in the `encabulator-support` namespace.

### Deploy via Helm

```bash
# Deploy individual service
helm upgrade --install <service-name> \
    ./helm/charts/<service-name> \
    --namespace encabulator-support \
    --create-namespace
```

### Configuration

Each service is configured via Helm values. Common configuration options:

```yaml
replicaCount: 2              # Number of replicas
image:
  repository: <service-name>  # Image repository
  tag: latest                 # Image tag
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 250m
    memory: 256Mi
serviceMonitor:
  enabled: true               # Enable Prometheus monitoring
```

## Service Integration

### Differential Girdle Integration

Other services query Differential Girdle to check feature flags:

```bash
# Check if feature is enabled
curl http://differential-girdle:8080/flags/prefabulation-v2

# Response
{
  "enabled": true,
  "metadata": {
    "rollout_percentage": 50,
    "environments": ["dev", "staging"]
  }
}
```

### Cardinal Grammeter Integration

Services send custom metrics to Cardinal Grammeter:

```bash
# Submit custom metric
curl -X POST http://cardinal-grammeter:8080/metrics \
  -H "Content-Type: application/json" \
  -d '{
    "name": "encabulation_success_rate",
    "value": 0.95,
    "labels": {"service": "panametric-fam", "environment": "prod"}
  }'
```

### Side Fumbling Collector Integration

Services send telemetry data to Side Fumbling Collector:

```bash
# Submit telemetry
curl -X POST http://side-fumbling-collector:8080/telemetry \
  -H "Content-Type: application/json" \
  -d '{
    "type": "trace",
    "trace_id": "abc123",
    "span_id": "def456",
    "duration_ms": 150
  }'
```

## Monitoring

All services expose Prometheus metrics at `/metrics`:

### Common Metrics
- HTTP request duration and count
- Error rates by endpoint
- Active connections
- Memory and CPU usage

### Service-Specific Metrics
- **Differential Girdle**: Flag evaluation count, configuration updates
- **Cardinal Grammeter**: Custom metrics received, aggregation latency
- **Side Fumbling Collector**: Telemetry events processed, forwarding errors

ServiceMonitor resources are automatically created when deployed with Helm.

## Health Checks

### Liveness Probe
- Endpoint: `GET /health`
- Purpose: Checks if the service process is running
- Always returns 200 OK if the service is alive

### Readiness Probe
- Endpoint: `GET /ready`
- Purpose: Checks if the service can handle requests
- Validates dependencies (storage, downstream services)
- Returns 503 if dependencies are unavailable

## Development

### Prerequisites

- **Go Service**: Go 1.21 or later
- **Python Services**: Python 3.12+ and pip
- Docker or Finch (for building images)
- kubectl (for deployment)
- Helm 3.x (for deployment)
- Access to Kubernetes cluster

### Local Development

#### Differential Girdle
```bash
export PORT=8080
cd services/support-plane/differential-girdle
go run ./cmd/server
```

#### Cardinal Grammeter
```bash
export PORT=8080
cd services/support-plane/cardinal-grammeter
pip install -r requirements.txt
python app/main.py
```

#### Side Fumbling Collector
```bash
export PORT=8080
cd services/support-plane/side-fumbling-collector
pip install -r requirements.txt
python app/main.py
```

## API Examples

### Differential Girdle

```bash
# Get all flags
curl http://localhost:8080/flags

# Get specific flag
curl http://localhost:8080/flags/prefabulation-v2

# Evaluate flag for user
curl -X POST http://localhost:8080/flags/prefabulation-v2/evaluate \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user123", "context": {"environment": "prod"}}'

# Update flag
curl -X PUT http://localhost:8080/flags/prefabulation-v2 \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "rollout_percentage": 75}'
```

### Cardinal Grammeter

```bash
# Submit metric
curl -X POST http://localhost:8080/metrics \
  -H "Content-Type: application/json" \
  -d '{
    "name": "encabulation_duration",
    "value": 125.5,
    "type": "histogram",
    "labels": {"service": "panametric-fam"}
  }'

# Get aggregated metrics
curl http://localhost:8080/metrics/aggregated?name=encabulation_duration

# Health check
curl http://localhost:8080/health
```

### Side Fumbling Collector

```bash
# Submit telemetry
curl -X POST http://localhost:8080/telemetry \
  -H "Content-Type: application/json" \
  -d '{
    "type": "trace",
    "service": "panametric-fam",
    "trace_id": "abc123",
    "data": {"duration_ms": 150, "status": "success"}
  }'

# Get telemetry stats
curl http://localhost:8080/stats

# Health check
curl http://localhost:8080/health
```

## Troubleshooting

### Service Won't Start

Check logs:
```bash
kubectl logs -n encabulator-support <pod-name>
```

Common issues:
- Missing dependencies (Python packages, Go modules)
- Invalid configuration
- Port already in use

### Python Services Not Running

Ensure Python dependencies are installed:
```bash
cd services/support-plane/<service-name>
pip install -r requirements.txt
```

Check Python version:
```bash
python --version  # Should be 3.12 or later
```

### Feature Flags Not Working (Differential Girdle)

Check flag configuration:
```bash
kubectl exec -n encabulator-support <pod-name> -- cat /config/flags.json
```

Verify flag evaluation logic in logs:
```bash
kubectl logs -n encabulator-support <pod-name> | grep "flag evaluation"
```

### Metrics Not Appearing (Cardinal Grammeter)

Verify Prometheus is scraping the service:
```bash
kubectl get servicemonitor -n encabulator-support
```

Check metrics endpoint:
```bash
kubectl port-forward -n encabulator-support svc/cardinal-grammeter 8080:8080
curl http://localhost:8080/metrics
```

## Performance Tuning

### Differential Girdle
- Cache flag evaluations
- Optimize configuration reload
- Use in-memory storage for hot flags

### Cardinal Grammeter
- Batch metric submissions
- Adjust aggregation windows
- Configure metric retention

### Side Fumbling Collector
- Buffer telemetry events
- Adjust forwarding batch size
- Configure sampling rates

## Testing Without pytest

If pytest is not available, Python services will skip tests during `make test`. To install pytest:

```bash
pip install pytest pytest-cov
```

Or use the service's requirements-dev.txt if available:

```bash
pip install -r requirements-dev.txt
```

## Contributing

When adding new Support Plane services:

1. Follow the established directory structure
2. Implement all common features (health, metrics, logging)
3. Create Helm chart with consistent naming
4. Add service to Makefile targets
5. Update this README
6. Add comprehensive tests
7. Document API endpoints
8. Consider integration with observability stack

## Educational Value

The Support Plane demonstrates:
- Feature flag patterns
- Custom metrics collection
- Telemetry aggregation
- Cross-cutting concerns in microservices
- Configuration management
- Observability integration

## License

Apache 2.0 - See LICENSE file for details
