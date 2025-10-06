# Processing Plane Services

The Processing Plane contains three microservices that handle core business logic, API gateway functionality, and message routing for the Cloud Native Encabulator system.

## Services

### 1. Panametric Fam Processor
Core business logic engine that processes encabulation requests with prefabulation support.

**Technology**: TypeScript + Node.js  
**Port**: 8080  
**Purpose**: Core encabulation processing with caching

[Service Documentation](./panametric-fam-processor/README.md)

### 2. Logarithmic Casing Service
Message broker and event bus for asynchronous communication between services.

**Technology**: Go 1.21  
**Port**: 8080  
**Purpose**: Message routing and pub/sub functionality

[Service Documentation](./logarithmic-casing-service/README.md)

### 3. Dingle Arm API Gateway
External API facade providing rate limiting, routing, and request validation.

**Technology**: Go 1.21  
**Port**: 8080  
**Purpose**: API gateway and ingress controller

[Service Documentation](./dingle-arm-api-gateway/README.md)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Processing Plane                          │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │   Dingle Arm API Gateway (Go)                        │  │
│  │   - Rate limiting                                     │  │
│  │   - Request routing                                   │  │
│  │   - Authentication                                    │  │
│  └──────────────────────────────────────────────────────┘  │
│                          │                                   │
│                          ▼                                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │   Panametric Fam Processor (TypeScript)              │  │
│  │   - Core business logic                               │  │
│  │   - Encabulation processing                           │  │
│  │   - Caching integration                               │  │
│  └──────────────────────────────────────────────────────┘  │
│                          │                                   │
│                          ▼                                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │   Logarithmic Casing Service (Go)                    │  │
│  │   - Message routing                                   │  │
│  │   - Event publishing                                  │  │
│  │   - Async communication                               │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                               │
└───────────────────────────────┬───────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │     Data Plane        │
                    │  (Storage Services)   │
                    └───────────────────────┘
```

## Service Directory Structure

### Go Services (Logarithmic Casing, Dingle Arm)
```
services/processing-plane/<service-name>/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/            # HTTP handlers
│   ├── broker|middleware|proxy/  # Core logic
│   └── models/         # Data models
├── pkg/health/         # Health check utilities
├── Dockerfile          # Container image definition
├── Makefile            # Build automation
├── go.mod              # Go module definition
└── README.md           # Service documentation
```

### TypeScript Service (Panametric Fam)
```
services/processing-plane/panametric-fam-processor/
├── src/
│   ├── index.ts        # Application entry point
│   ├── services/       # Business logic
│   ├── routes/         # API routes
│   └── types/          # TypeScript types
├── Dockerfile          # Container image definition
├── Makefile            # Build automation
├── package.json        # Node.js dependencies
├── tsconfig.json       # TypeScript configuration
└── README.md           # Service documentation
```

## Common Features

All Processing Plane services implement:

- ✅ REST API with standardized error responses
- ✅ Health (`/health`) and readiness (`/ready`) endpoints
- ✅ Prometheus metrics exposure (`/metrics`)
- ✅ Structured JSON logging
- ✅ Graceful shutdown handling
- ✅ Environment-based configuration
- ✅ Request tracing and correlation IDs
- ✅ Integration with Data Plane services

## Building

### Build All Services

```bash
# From repository root
make build-processing-plane

# Or from this directory
make build
```

### Build Individual Service

#### Go Services
```bash
cd services/processing-plane/<service-name>
make build
# Or manually:
go mod tidy
go build -o <service-name> ./cmd/server
```

#### TypeScript Service
```bash
cd services/processing-plane/panametric-fam-processor
make build
# Or manually:
npm install
npm run build
```

### Build Docker Images

```bash
# From repository root
make image-processing-plane

# Or from this directory
make image
```

## Testing

### Run All Tests

```bash
# From repository root
make test-processing-plane

# Or from this directory
make test
```

### Run Individual Service Tests

#### Go Services
```bash
cd services/processing-plane/<service-name>
go test ./...

# With coverage
go test -cover ./...
```

#### TypeScript Service
```bash
cd services/processing-plane/panametric-fam-processor
npm test

# With coverage
npm test -- --coverage
```

## Deployment

All Processing Plane services are deployed in the `encabulator-processing` namespace.

### Deploy via Helm

```bash
# Deploy all processing services
helm upgrade --install <service-name> \
    ./helm/charts/<service-name> \
    --namespace encabulator-processing \
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

### Request Flow

1. **External Request** → Dingle Arm API Gateway
   - Rate limiting applied
   - Authentication checked
   - Request validated

2. **Dingle Arm** → Panametric Fam Processor
   - Business logic executed
   - Data Plane services called
   - Response cached if applicable

3. **Panametric Fam** → Logarithmic Casing Service
   - Events published
   - Async notifications sent
   - Audit logs created

4. **Logarithmic Casing** → Other Services
   - Messages routed to subscribers
   - Event-driven workflows triggered

## Monitoring

All services expose Prometheus metrics at `/metrics`:

### Common Metrics
- HTTP request duration and count
- Error rates by endpoint
- Active connections
- Request queue depth

### Service-Specific Metrics
- **Dingle Arm**: Rate limit hits, proxy latency
- **Panametric Fam**: Processing time, cache hit rate
- **Logarithmic Casing**: Message throughput, queue depth

ServiceMonitor resources are automatically created when deployed with Helm.

## Health Checks

### Liveness Probe
- Endpoint: `GET /health`
- Purpose: Checks if the service process is running
- Always returns 200 OK if the service is alive

### Readiness Probe
- Endpoint: `GET /ready`
- Purpose: Checks if the service can handle requests
- Validates downstream service connectivity
- Returns 503 if dependencies are unavailable

## Development

### Prerequisites

- **Go Services**: Go 1.21 or later
- **TypeScript Service**: Node.js 18+ and npm
- Docker or Finch (for building images)
- kubectl (for deployment)
- Helm 3.x (for deployment)
- Access to Kubernetes cluster

### Local Development

#### Dingle Arm API Gateway
```bash
export PORT=8080
export UPSTREAM_URL=http://localhost:8081
cd services/processing-plane/dingle-arm-api-gateway
go run ./cmd/server
```

#### Panametric Fam Processor
```bash
export PORT=8080
export CACHE_URL=http://localhost:6379
export DB_URL=http://localhost:5432
cd services/processing-plane/panametric-fam-processor
npm install
npm run dev
```

#### Logarithmic Casing Service
```bash
export PORT=8080
cd services/processing-plane/logarithmic-casing-service
go run ./cmd/server
```

## API Examples

### Dingle Arm API Gateway

```bash
# Health check
curl http://localhost:8080/health

# Proxy request
curl -X POST http://localhost:8080/api/encabulate \
  -H "Content-Type: application/json" \
  -d '{"grammeters": 10, "prefabulation": true}'
```

### Panametric Fam Processor

```bash
# Process encabulation
curl -X POST http://localhost:8080/encabulate \
  -H "Content-Type: application/json" \
  -d '{
    "grammeters": 10,
    "prefabulation": true,
    "sinusoidalMode": "STANDARD"
  }'

# Get metrics
curl http://localhost:8080/metrics
```

### Logarithmic Casing Service

```bash
# Publish message
curl -X POST http://localhost:8080/publish \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "encabulation.completed",
    "payload": {"id": "123", "status": "success"}
  }'

# Get stats
curl http://localhost:8080/stats
```

## Troubleshooting

### Service Won't Start

Check logs:
```bash
kubectl logs -n encabulator-processing <pod-name>
```

Common issues:
- Downstream services not accessible
- Invalid configuration
- Port already in use

### High Latency

Check metrics:
```bash
kubectl port-forward -n encabulator-processing svc/<service-name> 8080:8080
curl http://localhost:8080/metrics | grep http_request_duration
```

Possible causes:
- Data Plane services slow
- Resource constraints
- Network issues

### Rate Limiting Issues (Dingle Arm)

Check rate limit configuration in Helm values:
```yaml
rateLimiting:
  enabled: true
  requestsPerSecond: 100
  burst: 200
```

## Performance Tuning

### Dingle Arm API Gateway
- Adjust rate limits based on load
- Configure connection pooling
- Enable request caching

### Panametric Fam Processor
- Tune cache TTL values
- Adjust worker pool size
- Enable response compression

### Logarithmic Casing Service
- Configure message buffer sizes
- Adjust retry policies
- Enable message batching

## Contributing

When adding new Processing Plane services:

1. Follow the established directory structure
2. Implement all common features (health, metrics, logging)
3. Create Helm chart with consistent naming
4. Add service to Makefile targets
5. Update this README
6. Add comprehensive tests
7. Document API endpoints

## License

Apache 2.0 - See LICENSE file for details
