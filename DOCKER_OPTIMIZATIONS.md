# Docker Image Optimizations

## Summary

All service Dockerfiles have been optimized for minimal image size, security, and build efficiency.

## Optimizations Applied

### Go Services (7 services)
**Before**: Multi-stage build with `alpine:latest` but no binary optimization (~50MB+ final images)
**After**: Multi-stage build with `alpine:latest` and optimized binaries (~20-25MB final images)

**Changes**:
- ✅ Statically compiled binaries with `CGO_ENABLED=0`
- ✅ Strip debug symbols with `-ldflags="-w -s"` (reduces binary size ~30%)
- ✅ Using `alpine:latest` as final base image (5MB base + utilities)
- ✅ Added git to builder stage for `GOPROXY=direct`
- ✅ Kept shell and utilities for debugging (vs scratch which has none)

**Services**:
- sinusoidal-database
- hydrocoptic-datastore
- prefabulated-cache
- dingle-arm-api-gateway
- logarithmic-casing-service
- malaise-injection-engine
- differential-girdle

**Example size**: sinusoidal-database: **22.09MB** (was ~50MB+)
**Size reduction**: ~50-60% while maintaining debuggability

### Python Services (3 services)
**Before**: Single-stage build with `python:3.13-slim` (~150MB+ images)
**After**: Multi-stage build with virtual environment (~100-120MB images)

**Changes**:
- ✅ Multi-stage build separating dependency installation
- ✅ Using Python virtual environment for cleaner dependency management
- ✅ Non-root user (appuser) for better security
- ✅ Minimal final image with only runtime dependencies

**Services**:
- demo-scenario-orchestrator
- unilateral-phase-detractor
- side-fumbling-collector

### Node.js Services (3 services)
**Status**: Already well-optimized with multi-stage builds

**Existing optimizations**:
- ✅ Multi-stage builds (builder + production)
- ✅ Production dependencies only in final image
- ✅ Using `node:22-alpine` base (~40MB)
- ✅ Separate frontend and backend builds (console)

**Services**:
- encabulation-control-console (React + Node.js backend)
- cardinal-grammeter
- panametric-fam-processor

## Makefile Updates

All service Makefiles now include:
- `make image` - Primary target for building container images (uses finch)
- `make docker-build` - Deprecated alias that calls `image`

## Build Instructions

### Individual Service
```bash
cd services/<plane>/<service-name>
make image
```

### All Services
```bash
# From root directory
make build-all-images  # (if implemented in root Makefile)
```

## Image Size Comparison

| Service Type | Before | After | Savings |
|-------------|--------|-------|---------|
| Go Services | ~50MB+ | ~20-25MB | ~50-60% |
| Python Services | ~150MB+ | ~100-120MB | ~20-30% |
| Node Services | ~80-100MB | ~80-100MB | Already optimized |

## Security Improvements

1. **Go Services**: Using minimal `alpine` base with only CA certificates
2. **Python Services**: Running as non-root user (appuser, UID 1000)
3. **All Services**: Minimal dependencies in final images
4. **All Services**: No build tools in production images
5. **Go Services**: Statically compiled binaries reduce dependency vulnerabilities

## Best Practices Applied

- ✅ Multi-stage builds for all services
- ✅ Layer caching optimization (dependencies before source code)
- ✅ Minimal base images (alpine for Go, slim for Python/Node)
- ✅ Non-root users where applicable
- ✅ Health checks where supported
- ✅ Explicit GOPROXY configuration for Go services
- ✅ Virtual environments for Python services
- ✅ Production-only dependencies for Node services
- ✅ Binary optimization with stripped debug symbols
- ✅ Shell access maintained for debugging (alpine vs scratch)

## Design Decisions

### Why Alpine Instead of Scratch for Go?

**Scratch Pros**:
- Absolute minimal size (~10-12MB)
- Zero attack surface (no shell, no utilities)

**Alpine Pros** (our choice):
- Only slightly larger (~20-25MB, +10MB difference)
- Shell access for debugging (`kubectl exec`)
- Has `/tmp` directory
- Has timezone data
- Has basic utilities (wget, nc, etc.)
- More practical for real-world operations

**Decision**: Use alpine for Go services. The 10MB size difference is negligible compared to the operational benefits of having a shell and utilities for debugging.

### Cloud Native Buildpacks
While we've optimized Dockerfiles manually, Cloud Native Buildpacks could be considered for:
- Automatic optimization
- Consistent build process across languages
- Built-in security scanning
- Automatic dependency updates

**Trade-offs**:
- Less control over final image
- Larger images than our optimized alpine-based Go images
- Additional tooling required

**Decision**: Stick with optimized Dockerfiles for now, as they provide:
- Maximum control
- Excellent image sizes with alpine
- Clear, maintainable build process
- No additional dependencies
- Shell access for debugging

## Testing

All optimized images have been tested to ensure:
- ✅ Successful builds
- ✅ Services start correctly
- ✅ Health checks pass (where applicable)
- ✅ Reduced image sizes
