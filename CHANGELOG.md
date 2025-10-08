# Cloud Native Encabulator - Changelog

## [Unreleased]

### Added
- **Versioning System**: Implemented unified semantic versioning across all services
  - `VERSION` file as single source of truth
  - Automated version bumping script (`scripts/version/bump-version.sh`)
  - Automated publishing script (`scripts/version/publish-version.sh`)
  - Makefile targets: `version`, `version-bump-*`, `version-publish`
  - Comprehensive versioning documentation
- **Image Publishing Targets**: Added `image-publish` Makefile targets at all levels (root, plane, and service)
  - Root level: `make image-publish` - publishes all images
  - Plane level: `make image-publish-{plane}` - publishes images for a specific plane
  - Service level: `make image-publish` - publishes a single service image
- **ECR Configuration**: Added configurable ECR settings via environment variables
  - `ECR_REGISTRY` - ECR registry URL
  - `ECR_NAMESPACE` - ECR namespace/prefix
  - `IMAGE_TAG` - Image tag (default: latest)
  - `CONTAINER_RUNTIME` - Container runtime (default: finch)
- **Documentation**: Added comprehensive image publishing guide at `docs/IMAGE_PUBLISHING.md`

### Changed
- Updated all service-level Makefiles to use `$(CONTAINER_RUNTIME)` variable instead of hardcoded `finch`
- Fixed Makefile syntax issues with for-loop variable escaping (`$$service`)

### Technical Details
- All 13 microservices now support automated ECR publishing
- Images are automatically built before publishing if not already present
- Proper error handling with exit codes for CI/CD integration
- Hierarchical Makefile structure maintains consistency across all services

## Services Updated

### Data Plane (3 services)
- hydrocoptic-datastore
- prefabulated-cache
- sinusoidal-database

### Processing Plane (3 services)
- panametric-fam-processor
- logarithmic-casing-service
- dingle-arm-api-gateway

### Support Plane (3 services)
- unilateral-phase-detractor
- differential-girdle
- side-fumbling-collector

### Control Plane (4 services)
- encabulation-control-console
- malaise-injection-engine
- cardinal-grammeter
- demo-scenario-orchestrator

## Usage Examples

```bash
# Publish all images
make image-publish

# Publish specific plane
make image-publish-data-plane

# Publish with custom tag
make image-publish IMAGE_TAG=v1.0.0

# Use Docker instead of Finch
make image-publish CONTAINER_RUNTIME=docker
```

## Next Steps

1. Update Helm chart values to reference ECR images
2. Configure Kubernetes image pull secrets
3. Deploy services to EKS cluster
