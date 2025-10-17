# Cloud Native Encabulator - Changelog

## [Unreleased]

## [0.2.0] - 2025-10-15

### Added
- **Real-Time Traffic Visualization**: Complete data flow visualization from traffic generation through all data services
  - Traffic particles flow from Cardinal Grammeter → API Gateway → Processor → Data Services
  - PostgreSQL (hydrocoptic-datastore): 1x TPS for encabulation writes
  - Redis (prefabulated-cache): 1.5x TPS for frequent cache checks
  - InfluxDB (sinusoidal-database): 0.8x TPS for metrics collection
- **Logarithmic Traffic Animation Scaling**: Dynamic particle spawning based on actual TPS
  - Low traffic (1-50 TPS): 1-3 particles per second
  - Medium traffic (50-500 TPS): 3-8 particles per second
  - High traffic (500+ TPS): 8-15 particles per second
  - Increased max particle cap from 100 to 200
- **Pod Indicator Scaling**: Pod indicators now scale proportionally with service node size
  - Pods remain visible and well-distributed when services scale from 1 to 5+ replicas
- **Testing Guide**: Comprehensive testing reference at `.kiro/TESTING_GUIDE.md`

### Changed
- **Responsiveness Optimization**: Significantly faster UI updates
  - Backend polling reduced from 5s to 2s (2.5x faster)
  - Prometheus rate window optimized from 1m to 45s (optimal for 15s scrape interval)
  - Response time improved from 5-10s to 2-4s for traffic start
- **Resource Optimization**: 70% reduction in idle resource usage
  - Control plane: CPU request increased from 100m to 200m (prevents unnecessary HPA scaling)
  - Processing plane: minReplicas reduced from 3/3/2 to 1/1/1
  - Data plane: minReplicas reduced from 2/2/2 to 1/1/1
  - Support plane: replicaCount reduced from 2 to 1
  - Idle usage: 4.75 cores → 1.5 cores (8 replicas total)
  - Services still scale up to 10-20 replicas under load
- **Improved Particle Spawn Resilience**: Better handling of long-running sessions

### Fixed
- Pod indicators disappearing when services scaled up
- Traffic particles not showing for data service connections
- Control console unnecessary scaling at idle due to low CPU request

### Performance
- **Response Time**: 2.5x faster (5-10s → 2-4s)
- **Metrics Update**: 2.5x faster (5s → 2s polling)
- **Resource Usage**: 70% reduction at idle (4.75 → 1.5 cores)
- **Visual Accuracy**: Traffic intensity now reflects actual system load

### Documentation
- Updated README with Key Features section
- Added Control Console access instructions
- Created comprehensive TESTING_GUIDE.md
- Updated tasks.md with 5 new completed subtasks (6.14-6.18)
- Cleaned up outdated TODO files

## [0.1.0] - 2024-12-XX

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
