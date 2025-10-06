# Building and Testing the Cloud Native Encabulator

This document describes how to build, test, and work with the Cloud Native Encabulator microservices.

## Prerequisites

Ensure you have the following tools installed:

- **Go** 1.21+
- **Node.js** 18+
- **Python** 3.12+
- **Docker** (for container builds)
- **Helm** 3+ (for chart validation)
- **Make** (for build automation)

Verify all dependencies:
```bash
make dev-deps
```

## Quick Start

### View All Available Targets
```bash
make help
```

### Build Everything
```bash
make build
```

### Run All Tests
```bash
make test
```

### Full Verification (Build + Test + Helm Lint)
```bash
make verify
```

## Building by Plane

Build services organized by architectural plane:

```bash
make build-data-plane          # Build all Data Plane services
make build-processing-plane    # Build all Processing Plane services
make build-support-plane       # Build all Support Plane services
make build-control-plane       # Build all Control Plane services
```

## Testing by Plane

Run tests organized by architectural plane:

```bash
make test-data-plane           # Test all Data Plane services
make test-processing-plane     # Test all Processing Plane services
make test-support-plane        # Test all Support Plane services
make test-control-plane        # Test all Control Plane services
```

## Building Individual Services

Each service has its own build target:

### Data Plane
```bash
make build-hydrocoptic-datastore
make build-prefabulated-cache
make build-sinusoidal-database
```

### Processing Plane
```bash
make build-logarithmic-casing-service
make build-dingle-arm-api-gateway
make build-panametric-fam-processor
```

### Support Plane
```bash
make build-differential-girdle
make build-cardinal-grammeter
make build-side-fumbling-collector
```

### Control Plane
```bash
make build-malaise-injection-engine
make build-unilateral-phase-detractor
make build-encabulation-control-console
make build-demo-scenario-orchestrator
```

## Docker Images

Build Docker images for all services or by plane:

```bash
make docker-build                    # Build all Docker images
make docker-build-data-plane         # Build Data Plane images
make docker-build-processing-plane   # Build Processing Plane images
make docker-build-support-plane      # Build Support Plane images
make docker-build-control-plane      # Build Control Plane images
```

## Helm Charts

Validate and test Helm charts:

```bash
make helm-lint       # Lint all Helm charts
make helm-template   # Test template rendering
```

## Code Formatting

Format code across all languages:

```bash
make fmt           # Format all code
make fmt-go        # Format Go code only
make fmt-node      # Format Node.js/TypeScript code only
make fmt-python    # Format Python code only (requires black)
```

## Cleaning Build Artifacts

Remove build artifacts:

```bash
make clean           # Clean all artifacts
make clean-go        # Clean Go artifacts only
make clean-node      # Clean Node.js artifacts only
make clean-python    # Clean Python artifacts only
```

## CI/CD Integration

The Makefile includes targets designed for CI/CD pipelines:

```bash
make ci       # Run full CI pipeline (deps check, build, test, lint)
make verify   # Comprehensive verification (build, test, helm validation)
```

### Example GitHub Actions Workflow

```yaml
name: CI
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      - uses: actions/setup-python@v4
        with:
          python-version: '3.12'
      - name: Run CI
        run: make ci
```

## Development Workflow

Typical development workflow:

1. **Make changes** to a service
2. **Build the service**: `make build-<service-name>`
3. **Run tests**: `make test-<plane-name>`
4. **Build Docker image**: `make docker-build-<plane-name>`
5. **Validate Helm chart**: `make helm-lint`
6. **Full verification**: `make verify`

## Service-Specific Build Details

### Go Services
- Built with `go build`
- Tests run with `go test ./...`
- Binaries output to service directory

### Node.js/TypeScript Services
- Dependencies installed with `npm install`
- Built with `npm run build` (TypeScript compilation)
- Tests run with `npm test`

### Python Services
- Syntax validated with `python3 -m py_compile`
- Tests run with `pytest` (when available)
- No compilation step required

## Troubleshooting

### Build Failures

**Go build fails:**
```bash
cd services/<plane>/<service>
go mod tidy
go build .
```

**Node.js build fails:**
```bash
cd services/<plane>/<service>
rm -rf node_modules package-lock.json
npm install
npm run build
```

**Python syntax errors:**
```bash
cd services/<plane>/<service>
python3 -m py_compile *.py
```

### Clean and Rebuild

If you encounter persistent issues:
```bash
make clean
make build
```

## Performance Tips

- **Parallel builds**: Make supports parallel execution with `-j` flag:
  ```bash
  make -j4 build  # Build with 4 parallel jobs
  ```

- **Build only what changed**: Use individual service targets instead of plane-wide targets

- **Skip tests during development**: Use `make build` without `make test` for faster iteration

## Additional Resources

- [Service Documentation](services/)
- [Helm Charts](helm/charts/)
- [Architecture Design](.kiro/specs/cloud-native-encabulator-microservices/design.md)
- [Requirements](.kiro/specs/cloud-native-encabulator-microservices/requirements.md)
