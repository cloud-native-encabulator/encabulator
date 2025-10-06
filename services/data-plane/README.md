# Data Plane Services

The Data Plane contains three microservices that provide REST API wrappers around the core data storage backends for the Cloud Native Encabulator system.

## Services

### 1. Hydrocoptic Data Store
PostgreSQL wrapper service providing domain-specific CRUD operations for encabulation records.

**Technology**: Go 1.21  
**Backend**: PostgreSQL  
**Port**: 8080

[Service Documentation](./hydrocoptic-datastore/README.md)

### 2. Prefabulated Cache
Redis wrapper service providing caching operations with TTL support.

**Technology**: Go 1.21  
**Backend**: Redis  
**Port**: 8080

[Service Documentation](./prefabulated-cache/README.md)

### 3. Sinusoidal Database
InfluxDB wrapper service providing time-series data operations.

**Technology**: Go 1.21  
**Backend**: InfluxDB 2.x  
**Port**: 8080

[Service Documentation](./sinusoidal-database/README.md)

## Architecture

All Data Plane services follow a consistent architecture:

```
services/data-plane/<service-name>/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/            # HTTP handlers
│   ├── db|cache|tsdb/  # Backend client wrappers
│   └── models/         # Data models
├── pkg/health/         # Health check utilities
├── Dockerfile          # Container image definition
├── go.mod              # Go module definition
└── README.md           # Service documentation
```

## Common Features

All services implement:

- ✅ REST API with standardized error responses
- ✅ Health (`/health`) and readiness (`/ready`) endpoints
- ✅ Prometheus metrics exposure (`/metrics`)
- ✅ Structured JSON logging with zerolog
- ✅ Graceful shutdown handling
- ✅ Connection pooling and retry logic
- ✅ Environment-based configuration

## Building

### Build All Services

```bash
# From repository root
make build-data-plane

# Or from this directory
cd services/data-plane
make build
```

### Build Individual Service

```bash
cd services/data-plane/<service-name>
make build

# Or manually
go mod tidy
go build -o <service-name> ./cmd/server
```

### Build Docker Images

```bash
# From repository root
make image-data-plane

# Or from this directory
cd services/data-plane
make image
```

## Testing

### Run All Tests

```bash
# From repository root
make test-data-plane

# Or from this directory
cd services/data-plane
make test
```

### Run Individual Service Tests

```bash
cd services/data-plane/<service-name>
go test ./...

# With coverage
go test -cover ./...
```

## Deployment

Data Plane services are deployed as part of the complete platform via Helm.

### Deploy All Data Services

```bash
# Deploy complete platform (includes data backends)
./scripts/deploy/deploy-helm.sh

# Or deploy only data plane backends
./scripts/deploy/deploy-helm.sh --data-plane-only
```

### Deploy Individual Service

```bash
helm upgrade --install <service-name> \
    ./helm/charts/<service-name> \
    --namespace encabulator-data \
    --create-namespace
```

**Note**: Data Plane services require their backend services (PostgreSQL, Redis, InfluxDB) to be running. These are deployed automatically by `deploy-helm.sh`.

## Configuration

Each service is configured via Helm values. See individual chart values files:

- `helm/charts/hydrocoptic-datastore/values.yaml`
- `helm/charts/prefabulated-cache/values.yaml`
- `helm/charts/sinusoidal-database/values.yaml`

### Common Configuration Options

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

## Monitoring

All services expose Prometheus metrics at `/metrics`:

- HTTP request duration and count
- Backend connection pool stats
- Error rates
- Custom business metrics

ServiceMonitor resources are automatically created when deployed with Helm.

## Health Checks

### Liveness Probe
- Endpoint: `GET /health`
- Purpose: Checks if the service process is running
- Always returns 200 OK if the service is alive

### Readiness Probe
- Endpoint: `GET /ready`
- Purpose: Checks if the service can handle requests
- Validates backend connectivity (PostgreSQL/Redis/InfluxDB)
- Returns 503 if backend is unavailable

## Development

### Prerequisites

- Go 1.21 or later
- Docker (for building images)
- kubectl (for deployment)
- Helm 3.x (for deployment)
- Access to Kubernetes cluster

### Local Development

1. Start the backend service (PostgreSQL/Redis/InfluxDB)
2. Set environment variables
3. Run the service

Example for Hydrocoptic Data Store:

```bash
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=postgres
export DB_NAME=hydrocoptic
cd services/data-plane/hydrocoptic-datastore
go run ./cmd/server
```

### Adding New Endpoints

1. Define models in `internal/models/types.go`
2. Implement backend operations in `internal/db|cache|tsdb/`
3. Create HTTP handlers in `internal/api/handlers.go`
4. Register routes in `cmd/server/main.go`
5. Add tests
6. Update README

## Troubleshooting

### Service Won't Start

Check logs:
```bash
kubectl logs -n encabulator-data <pod-name>
```

Common issues:
- Backend not accessible (check readiness probe)
- Invalid credentials (check secrets)
- Port already in use (check service configuration)

### Backend Connection Issues

Verify backend is running:
```bash
kubectl get pods -n encabulator-data
```

Check service endpoints:
```bash
kubectl get endpoints -n encabulator-data
```

### High Memory Usage

Adjust resource limits in Helm values:
```yaml
resources:
  limits:
    memory: 1Gi  # Increase as needed
```

## API Examples

See individual service READMEs for detailed API examples:

- [Hydrocoptic Data Store API](./hydrocoptic-datastore/README.md#example-requests)
- [Prefabulated Cache API](./prefabulated-cache/README.md#example-requests)
- [Sinusoidal Database API](./sinusoidal-database/README.md#example-requests)

## Contributing

When adding new Data Plane services:

1. Follow the established directory structure
2. Implement all common features (health, metrics, logging)
3. Create Helm chart with consistent naming
4. Add service to build and deploy scripts
5. Update this README
6. Add comprehensive tests

## License

Apache 2.0 - See LICENSE file for details
