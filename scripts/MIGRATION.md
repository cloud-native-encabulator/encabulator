# Scripts Migration Guide

## Overview

The scripts directory has been updated to reflect the current Makefile-first, Helm-based architecture with 13 microservices across 4 service planes.

## What Changed

### ✅ New Scripts

#### Build Scripts
- **`build-all.sh`** - Wrapper for `make build` to build all services
- **`build-images.sh`** - Wrapper for `make image` to build all container images

#### Test Scripts
- **`test-all.sh`** - Wrapper for `make test` to run all unit tests

### 🗑️ Removed Scripts

The following old scripts have been removed in favor of Makefile targets:

#### Build Scripts
- **`build-data-plane-services.sh`** → Use `make build-data-plane` instead
- **`build-data-plane-images.sh`** → Use `make image-data-plane` instead

#### Deploy Scripts
- **`deploy-data-plane-services.sh`** → Use `./scripts/deploy/deploy-helm.sh --data-plane-only` instead

### 📝 Updated Documentation

- **`README.md`** - Completely rewritten to reflect:
  - Makefile-first approach
  - Helm-based deployment
  - 13 microservices architecture
  - Current service planes (Data, Processing, Support, Control)
  - Deprecated scripts with migration paths
  - Comprehensive workflow examples

## Migration Path

### If you were using old build scripts:

```bash
# Old way
./scripts/build/build-data-plane-services.sh
./scripts/build/build-data-plane-images.sh

# New way
make build-data-plane
make image-data-plane

# Or build everything
make build
make image
```

### If you were using old deployment scripts:

```bash
# Old way
./scripts/deploy/deploy-data-plane-services.sh

# New way
./scripts/deploy/deploy-helm.sh --data-plane-only

# Or deploy everything
./scripts/deploy/deploy-helm.sh
```

### If you were using individual service builds:

```bash
# Old way
cd services/data-plane/hydrocoptic-datastore
go build ./cmd/server

# New way
make build-data-plane
# Or for specific plane
cd services/data-plane && make build
```

## Current Architecture

### Service Planes

The platform now consists of 13 microservices across 4 planes:

1. **Data Plane** (3 services - Go)
   - hydrocoptic-datastore
   - prefabulated-cache
   - sinusoidal-database

2. **Processing Plane** (3 services - 2 Go, 1 TypeScript)
   - logarithmic-casing-service
   - dingle-arm-api-gateway
   - panametric-fam-processor

3. **Support Plane** (3 services - 1 Go, 2 Python)
   - differential-girdle
   - cardinal-grammeter
   - side-fumbling-collector

4. **Control Plane** (4 services - 1 Go, 2 TypeScript, 1 Python)
   - malaise-injection-engine
   - unilateral-phase-detractor
   - encabulation-control-console
   - demo-scenario-orchestrator

### Build System

- **Primary**: Unified Makefile at repository root
- **Secondary**: Per-plane Makefiles in each service plane directory
- **Scripts**: Convenience wrappers and deployment automation

### Deployment System

- **Primary**: Helm-based deployment via `deploy-helm.sh`
- **Components**:
  - ArgoCD (GitOps)
  - Observability stack (Prometheus, Grafana)
  - Data backends (PostgreSQL, Redis, InfluxDB)
  - Service planes (deployed separately or via ArgoCD)

## Recommended Workflows

### Development Workflow

```bash
# 1. Build all services
make build

# 2. Run all tests
make test

# 3. Build container images
make image

# 4. Deploy to cluster
./scripts/deploy/deploy-helm.sh

# 5. Validate deployment
./scripts/deploy/status.sh
```

### CI/CD Workflow

```bash
# Single command for CI
make ci

# Or comprehensive verification
make verify
```

### Testing Workflow

```bash
# Unit tests
make test

# Integration tests
./scripts/test/comprehensive-test.sh

# Data services tests
./scripts/test/test-data-services.sh
```

## Key Improvements

1. **Unified Build System**: Single Makefile for all services
2. **Consistent Testing**: Unified test targets across all planes
3. **Helm-First Deployment**: Modern Helm-based deployment approach
4. **Better Documentation**: Comprehensive README with examples
5. **Deprecation Warnings**: Old scripts warn users about new methods
6. **Backwards Compatibility**: Old scripts still work but delegate to new system

## Breaking Changes

Old individual build and deploy scripts have been removed. Use Makefile targets and unified deployment scripts instead.

## Next Steps

1. Update your CI/CD pipelines to use Makefile targets
2. Update documentation to reference new scripts
3. Train team on new workflows
4. Consider removing deprecated scripts in future major version

## Questions?

See the main README.md for detailed documentation on all scripts and workflows.
