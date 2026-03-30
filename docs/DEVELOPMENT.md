# Development Guide

## Prerequisites

- **Go** 1.21+
- **Node.js** 18+
- **Python** 3.12+
- **Docker** or **Finch** (for container builds)
- **Helm** 3+ (for chart validation)
- **Make**
- **kubectl** configured to access your cluster

Verify dependencies:

```bash
make dev-deps
```

## Building

```bash
make help                          # Show all targets
make build                         # Build everything
make build-data-plane              # Build by plane
make build-hydrocoptic-datastore   # Build single service
```

### Docker Images

```bash
make image                         # Build all images
make image-data-plane              # Build by plane
```

See [Image Publishing](./IMAGE_PUBLISHING.md) for ECR publishing.

## Testing

```bash
make test                          # Run all tests
make test-data-plane               # Test by plane
make verify                        # Build + test + helm lint
make ci                            # Full CI pipeline
```

### Platform Testing

```bash
# Comprehensive platform test (infrastructure + observability + integration)
./scripts/test/comprehensive-test.sh

# Quick test
./scripts/test/comprehensive-test.sh --quick

# Observability-focused test with local cluster
./scripts/test/local-test.sh
./scripts/test/local-test.sh --cleanup
```

### Access Test Services

```bash
# Prometheus
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090

# Grafana (admin/encabulator-dev-2024)
kubectl port-forward -n encabulator-monitoring svc/prometheus-stack-grafana 3000:80
```

## Setup & Deployment

### GitOps Setup (Recommended)

```bash
# 1. Configure repository
./scripts/setup/configure-repository.sh

# 2. Bootstrap GitOps
./scripts/platform/bootstrap-gitops.sh

# 3. Access ArgoCD
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

See [GitOps Deployment](./GITOPS_DEPLOYMENT.md) for full ArgoCD workflow.

### Quick Deploy (Helm Direct)

```bash
# Create namespaces
kubectl create namespace encabulator-control
kubectl create namespace encabulator-support

# Deploy all services
helm install differential-girdle ./helm/charts/differential-girdle -n encabulator-support && \
helm install side-fumbling-collector ./helm/charts/side-fumbling-collector -n encabulator-support && \
helm install unilateral-phase-detractor ./helm/charts/unilateral-phase-detractor -n encabulator-support && \
helm install cardinal-grammeter ./helm/charts/cardinal-grammeter -n encabulator-control && \
helm install malaise-injection-engine ./helm/charts/malaise-injection-engine -n encabulator-control && \
helm install demo-scenario-orchestrator ./helm/charts/demo-scenario-orchestrator -n encabulator-control && \
helm install encabulation-control-console ./helm/charts/encabulation-control-console -n encabulator-control
```

### Verify Deployment

```bash
kubectl get pods -A | grep encabulator
kubectl get svc -n encabulator-support
kubectl get svc -n encabulator-control
```

## Code Formatting

```bash
make fmt           # Format all code
make fmt-go        # Go only
make fmt-node      # Node.js/TypeScript only
make fmt-python    # Python only (requires black)
```

## Cleaning

```bash
make clean         # Clean all build artifacts
```

## Development Workflow

1. Make changes to a service
2. `make build-<service-name>` — build
3. `make test-<plane-name>` — test
4. `make image` — build Docker image
5. `make helm-lint` — validate Helm charts
6. `make verify` — full verification

## Service Build Details

| Language | Build | Test |
|----------|-------|------|
| Go | `go build` | `go test ./...` |
| Node.js/TypeScript | `npm install && npm run build` | `npm test` |
| Python | No compilation | `pytest` or `python3 -m py_compile` |

## Troubleshooting

```bash
# Go build fails
cd services/<plane>/<service> && go mod tidy && go build .

# Node.js build fails
cd services/<plane>/<service> && rm -rf node_modules package-lock.json && npm install

# Pod not starting
kubectl describe pod <pod-name> -n <namespace>
kubectl logs <pod-name> -n <namespace>
```
