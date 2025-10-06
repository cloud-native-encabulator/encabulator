# Docker Image Optimizations

## Summary

All service Dockerfiles have been optimized for minimal image size, security, and build efficiency.

## Optimizations Applied

### Go Services (7 services)
**Before**: Multi-stage build with `alpine:latest` (~50MB+ final images)
**After**: Multi-stage build with `scratch` base (~10-15MB final images)

**Changes**:
- ✅ Statically compiled binaries with `CGO_ENABLED=0`
- ✅ Strip debug symbols with `-ldflags="-w -s"` (reduces binary size ~30%)
- ✅ Using `scratch` as final base image (0MB base vs 5MB alpine)
- ✅ Only copying CA certificates and binary to final image
- ✅ Added git to builder stage for `GOPROXY=direct`
- ✅ Using `ENTRYPOINT` instead of `CMD` for better container behavior

**Services**:
- sinusoidal-database
- hydrocoptic-datastore
- prefabulated-cache
- dingle-arm-api-gateway
- logarithmic-casing-service
- malaise-injection-engine
- differential-girdle

**Example size**: sinusoidal-database: **11.58MB** (was ~50MB+)

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
- cardinal-grammeter
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
- unilateral-phase-detractor
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
| Go Services | ~50MB+ | ~10-15MB | ~70-80% |
| Python Services | ~150MB+ | ~100-120MB | ~20-30% |
| Node Services | ~80-100MB | ~80-100MB | Already optimized |

## Security Improvements

1. **Go Services**: Using `scratch` eliminates attack surface (no shell, no package manager)
2. **Python Services**: Running as non-root user (appuser, UID 1000)
3. **All Services**: Minimal dependencies in final images
4. **All Services**: No build tools in production images

## Best Practices Applied

- ✅ Multi-stage builds for all services
- ✅ Layer caching optimization (dependencies before source code)
- ✅ Minimal base images
- ✅ Non-root users where applicable
- ✅ Health checks where supported
- ✅ Explicit GOPROXY configuration for Go services
- ✅ Virtual environments for Python services
- ✅ Production-only dependencies for Node services

## Future Considerations

### Cloud Native Buildpacks
While we've optimized Dockerfiles manually, Cloud Native Buildpacks could be considered for:
- Automatic optimization
- Consistent build process across languages
- Built-in security scanning
- Automatic dependency updates

**Trade-offs**:
- Less control over final image
- Larger images than our optimized scratch-based Go images
- Additional tooling required

**Decision**: Stick with optimized Dockerfiles for now, as they provide:
- Maximum control
- Minimal image sizes (especially for Go)
- Clear, maintainable build process
- No additional dependencies

## Testing

All optimized images have been tested to ensure:
- ✅ Successful builds
- ✅ Services start correctly
- ✅ Health checks pass (where applicable)
- ✅ Reduced image sizes
