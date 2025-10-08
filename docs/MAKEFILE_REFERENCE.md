# Makefile Reference Guide

Quick reference for all available Makefile targets in the Cloud Native Encabulator project.

## Container Image Management

### Building Images

```bash
# Build all images
make image

# Build by plane
make image-data-plane
make image-processing-plane
make image-support-plane
make image-control-plane
```

### Publishing Images to ECR

```bash
# Publish all images to ECR
make image-publish

# Publish by plane
make image-publish-data-plane
make image-publish-processing-plane
make image-publish-support-plane
make image-publish-control-plane

# Publish with custom configuration
make image-publish IMAGE_TAG=v1.0.0
make image-publish ECR_REGISTRY=custom-registry.com
make image-publish CONTAINER_RUNTIME=docker
```

## Building Services

```bash
# Build all services
make build

# Build by plane
make build-data-plane
make build-processing-plane
make build-support-plane
make build-control-plane
```

## Testing

```bash
# Run all tests
make test

# Test by plane
make test-data-plane
make test-processing-plane
make test-support-plane
make test-control-plane
```

## Code Quality

```bash
# Format all code
make fmt

# Format by language
make fmt-go
make fmt-node
make fmt-python
```

## Helm Charts

```bash
# Lint all Helm charts
make helm-lint

# Test Helm template rendering
make helm-template
```

## Cleanup

```bash
# Clean all build artifacts
make clean

# Clean by language
make clean-go
make clean-node
make clean-python
```

## CI/CD

```bash
# Run full CI pipeline (build, test, lint)
make ci

# Verify everything (build, test, helm)
make verify
```

## Development

```bash
# Check development dependencies
make dev-deps
```

## Configuration Variables

All targets support the following environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTAINER_RUNTIME` | `finch` | Container runtime (finch or docker) |
| `ECR_REGISTRY` | `510431938379.dkr.ecr.us-east-2.amazonaws.com` | ECR registry URL |
| `ECR_NAMESPACE` | `encabulator` | ECR namespace/prefix |
| `IMAGE_TAG` | `latest` | Container image tag |

## Service-Level Targets

Navigate to any service directory to run service-specific targets:

```bash
cd services/data-plane/hydrocoptic-datastore

# Available targets
make help
make build
make test
make image
make image-publish
make clean
make fmt
make lint
make deps
```

## Examples

### Publish All Images with Version Tag

```bash
make image-publish IMAGE_TAG=v1.2.3
```

### Build and Test Everything

```bash
make build test
```

### CI/CD Pipeline

```bash
# Full verification
make verify

# Build and publish
make image-publish IMAGE_TAG=$(git describe --tags)
```

### Development Workflow

```bash
# Format code
make fmt

# Build services
make build

# Run tests
make test

# Build images
make image

# Publish to ECR
make image-publish
```

## Plane Organization

### Data Plane Services
- hydrocoptic-datastore (Go)
- prefabulated-cache (Go)
- sinusoidal-database (Go)

### Processing Plane Services
- panametric-fam-processor (Node.js/TypeScript)
- logarithmic-casing-service (Go)
- dingle-arm-api-gateway (Go)

### Support Plane Services
- unilateral-phase-detractor (Python)
- differential-girdle (Go)
- side-fumbling-collector (Python)

### Control Plane Services
- encabulation-control-console (React + Node.js)
- malaise-injection-engine (Go)
- cardinal-grammeter (Node.js/TypeScript)
- demo-scenario-orchestrator (Python)

## Tips

1. **Parallel Builds**: Use `make -j4` for parallel execution
2. **Dry Run**: Use `make -n target` to see what would be executed
3. **Verbose Output**: Most targets include progress indicators
4. **Error Handling**: All targets properly propagate exit codes for CI/CD

## Getting Help

```bash
# Show all available targets
make help

# Show targets for a specific plane
make -C services/data-plane help

# Show targets for a specific service
make -C services/data-plane/hydrocoptic-datastore help
```
