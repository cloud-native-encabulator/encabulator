# Project Structure & Organization

## Repository Layout

```
cloud-native-encabulator-microservices/
├── services/              # All microservices organized by plane
├── helm/                  # Helm charts and values
├── argocd/               # GitOps configuration
├── k8s/                  # Kubernetes manifests (legacy/platform)
├── scripts/              # Build, deploy, and test automation
├── docs/                 # Documentation
├── audits/               # Project audits and health checks
└── .kiro/                # Kiro AI assistant configuration
```

## Services Directory

Organized by **architectural plane**:

```
services/
├── data-plane/           # 3 Go services (PostgreSQL, Redis, InfluxDB wrappers)
├── processing-plane/     # 2 Go + 1 TypeScript (core logic, gateway, broker)
├── support-plane/        # 1 Go + 2 Python (feature flags, auth, telemetry)
└── control-plane/        # 1 Go + 2 TypeScript + 1 Python (UI, chaos, traffic)
```

### Service Structure Pattern

Each service follows a consistent structure:

**Go Services:**
```
<service-name>/
├── cmd/server/           # Application entry point (main.go)
├── internal/             # Private application code
│   ├── api/             # HTTP handlers
│   ├── db|cache|tsdb/   # Backend client wrappers
│   └── models/          # Data models
├── pkg/health/          # Reusable health check utilities
├── Dockerfile           # Multi-stage container build
├── Makefile             # Service-specific build targets
├── go.mod               # Go module definition
├── VERSION              # Semantic version
└── README.md            # Service documentation
```

**TypeScript/Node.js Services:**
```
<service-name>/
├── src/                 # Source code
│   ├── index.ts        # Entry point
│   ├── routes/         # API routes
│   └── services/       # Business logic
├── dist/               # Compiled output (gitignored)
├── Dockerfile          # Container build
├── package.json        # Dependencies and scripts
├── tsconfig.json       # TypeScript configuration
├── jest.config.js      # Test configuration
├── Makefile            # Build targets
├── VERSION             # Semantic version
└── README.md           # Documentation
```

**Python Services:**
```
<service-name>/
├── app/                # Application code
│   ├── main.py        # FastAPI entry point
│   ├── models.py      # Pydantic models
│   └── config.py      # Configuration
├── tests/             # Test files
├── Dockerfile         # Container build
├── requirements.txt   # Dependencies
├── pyproject.toml     # Project metadata
├── Makefile           # Build targets
├── VERSION            # Semantic version
└── README.md          # Documentation
```

## Helm Directory

```
helm/
├── charts/              # Custom charts for all 13 services
│   ├── <service-name>/
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/
│   │       ├── deployment.yaml
│   │       ├── service.yaml
│   │       ├── servicemonitor.yaml
│   │       └── namespace.yaml
└── values/             # Values for upstream charts
    ├── data-services/  # PostgreSQL, Redis, InfluxDB configs
    ├── observability/  # Prometheus, Grafana configs
    └── scaling/        # Metrics server, HPA configs
```

## ArgoCD Directory

GitOps configuration using app-of-apps pattern:

```
argocd/
├── root-app.yaml       # Root application (manages all apps)
└── apps/               # Application definitions by plane
    ├── control-plane/
    ├── data-plane/
    ├── processing-plane/
    ├── support-plane/
    └── monitoring/
```

## Scripts Directory

Automation organized by purpose:

```
scripts/
├── build/              # Build automation
├── deploy/             # Deployment scripts
│   ├── deploy-helm.sh
│   └── status.sh
├── test/               # Testing scripts
│   ├── comprehensive-test.sh
│   └── test-all.sh
├── version/            # Version management
│   ├── bump-version.sh
│   └── publish-version.sh
├── setup/              # Environment setup
│   └── eks-setup.sh
└── helm/               # Helm utilities
```

## Documentation Directory

```
docs/
├── architecture/       # Architecture documentation
├── GITOPS_DEPLOYMENT.md
├── IMAGE_PUBLISHING.md
├── VERSIONING.md
├── MAKEFILE_REFERENCE.md
└── troubleshooting-*.md
```

## Key Files

- **`Makefile`** - Root build orchestration
- **`VERSION`** - Platform version (semantic versioning)
- **`README.md`** - Main project documentation
- **`ARCHITECTURE_SUMMARY.md`** - High-level architecture
- **`BUILD.md`** - Build instructions
- **`TESTING.md`** - Testing guide
- **`CONTRIBUTING.md`** - Contribution guidelines

## Naming Conventions

### Services
- Lowercase with hyphens: `hydrocoptic-datastore`
- Whimsical Encabulator-themed names
- Descriptive of function despite humor

### Namespaces
- Prefix: `encabulator-`
- By plane: `encabulator-data`, `encabulator-system`, `encabulator-control`, `encabulator-monitoring`

### Container Images
- Format: `<registry>/<namespace>/<service-name>:<tag>`
- Example: `510431938379.dkr.ecr.us-east-2.amazonaws.com/encabulator/hydrocoptic-datastore:v0.3.0`

### Helm Charts
- Match service names exactly
- Located in `helm/charts/<service-name>/`

### Git Branches
- `main` - Current implementation (GitOps)
- `helm-first` - Preserved Helm-only implementation
- `argo-first` - Will preserve GitOps when KRO added
- Feature branches: `feature/<name>`

## Common Patterns

### Health Endpoints
All services implement:
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe (checks backend connectivity)
- `GET /metrics` - Prometheus metrics

### Configuration
- Environment variables for runtime config
- Helm values for deployment config
- Secrets via Kubernetes Secrets

### Logging
- Structured JSON logging
- Go: `zerolog`
- Python: `logging` with JSON formatter
- TypeScript: `winston` or similar

### Metrics
All services expose Prometheus metrics with ServiceMonitor resources
