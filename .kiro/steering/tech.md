# Technology Stack & Build System

## Languages & Frameworks

### Go Services (7 services)
- **Version**: Go 1.21+
- **Services**: All Data Plane, 2 Processing Plane, 1 Support Plane, 1 Control Plane
- **Key Libraries**:
  - `gorilla/mux` - HTTP routing
  - `lib/pq` - PostgreSQL driver
  - `prometheus/client_golang` - Metrics
  - `rs/zerolog` - Structured logging
- **Structure**: Standard Go project layout with `cmd/`, `internal/`, `pkg/`

### TypeScript/Node.js Services (3 services)
- **Version**: Node.js 18+, TypeScript 5.8+
- **Services**: Panametric Fam Processor, Cardinal Grammeter, Encabulation Control Console
- **Key Libraries**:
  - `express` - HTTP server
  - `axios` - HTTP client
  - `prom-client` - Metrics
  - `redis` - Redis client
  - `vue` / `react` - Frontend frameworks
- **Testing**: Jest

### Python Services (3 services)
- **Version**: Python 3.12+
- **Services**: Demo Scenario Orchestrator, Unilateral Phase Detractor, Side Fumbling Collector
- **Key Libraries**:
  - `fastapi` - Web framework
  - `uvicorn` - ASGI server
  - `pydantic` - Data validation
  - `kubernetes` - K8s client
  - `prometheus-client` - Metrics
  - `pytest` - Testing

## Build System

### Make-based Build System
Primary build tool with hierarchical Makefiles:
- Root `Makefile` - Orchestrates all services
- Plane-level `Makefile` - Builds services by architectural plane
- Service-level `Makefile` - Individual service builds

### Common Commands

```bash
# Build
make build                    # Build all services
make build-data-plane         # Build by plane
make build-<service-name>     # Build individual service

# Test
make test                     # Run all tests
make test-data-plane          # Test by plane

# Container Images
make image                    # Build all images
make image-data-plane         # Build images by plane

# Publish to ECR
make image-publish            # Publish all images
make image-local-publish      # Publish local platform only

# Development
make dev-deploy               # Deploy with 'dev' tag
make clean                    # Clean build artifacts
make fmt                      # Format all code
make verify                   # Full verification (build + test + lint)

# Helm
make helm-lint                # Lint Helm charts
make helm-template            # Test template rendering

# Versioning
make version                  # Show current version
make version-bump-patch       # Bump patch version
make version-publish          # Publish versioned images
```

## Container Runtime

**Finch** (preferred) or Docker
- Images built with multi-stage Dockerfiles
- Each service has its own `Dockerfile`

## Deployment

### GitOps with ArgoCD (Current)
- All infrastructure and apps managed through Git
- Root application pattern (app-of-apps)
- Automatic synchronization from Git

### Helm Charts
- Custom charts for all 13 services in `helm/charts/`
- Community charts for backends (PostgreSQL, Redis, InfluxDB)
- `kube-prometheus-stack` for observability

## Infrastructure

- **Kubernetes**: EKS or any K8s cluster
- **Observability**: Prometheus, Grafana (via kube-prometheus-stack)
- **Databases**: PostgreSQL, Redis, InfluxDB (via community Helm charts)
- **Message Broker**: NATS (embedded in Logarithmic Casing Service)

## Development Tools

- **kubectl** - Kubernetes CLI
- **helm** - Package manager
- **argocd** - GitOps CLI
- **golangci-lint** - Go linting
- **black** - Python formatting
- **ESLint/Prettier** - JavaScript/TypeScript formatting

## Testing

- **Unit Tests**: Language-specific test frameworks
- **Integration Tests**: `./scripts/test/comprehensive-test.sh`
- **Local Testing**: Requires Docker/Finch + kind/minikube/k3d

## Configuration

- Environment-based configuration for services
- Helm values files for deployment configuration
- Secrets managed via Kubernetes Secrets
- Feature flags via Differential Girdle service
