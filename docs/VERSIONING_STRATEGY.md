# Cloud Native Encabulator - Versioning Strategy

## Overview

The Cloud Native Encabulator uses a **hybrid versioning approach** that combines platform-level and service-level semantic versioning to support independent service evolution while maintaining platform compatibility.

## Versioning Model

### Platform Version
**Location:** `VERSION` file at repository root  
**Format:** `MAJOR.MINOR.PATCH`  
**Purpose:** Tracks overall platform compatibility and release versions

**Initial Development Phase (0.x):**
- Platform starts at `0.0.1`
- Major version remains `0` during initial implementation
- Move to `1.0.0` when end-to-end implementation is complete and stable
- Version `0.x` indicates "pre-production" development phase

### Service Versions
**Location:** Each service directory (e.g., `services/data-plane/hydrocoptic-datastore/VERSION`)  
**Format:** `MAJOR.MINOR.PATCH`  
**Purpose:** Tracks individual service changes and evolution

**Initial Development Phase:**
- Services start at `0.0.1`
- Can evolve independently during development
- Service major version can reach `1.0.0` before platform does

## Versioning Rules

### Service Version Bumps

| Change Type | Service Version | Platform Version | Example |
|-------------|----------------|------------------|---------|
| **Bug Fix** | `PATCH++` | `PATCH++` | Fix memory leak in cache service |
| **New Feature** | `MINOR++` | `MINOR++` | Add new API endpoint |
| **Breaking Change** | `MAJOR++` | `MAJOR++` | Change API contract |
| **Dependency Update** | `PATCH++` | No change | Update library version |
| **Documentation** | No change | No change | Update README |

### Platform Version Bumps

Platform version follows the same semantic versioning rules:

- **PATCH:** Bug fixes, minor improvements, dependency updates
- **MINOR:** New features, backwards-compatible changes
- **MAJOR:** Breaking changes, major architectural shifts

Platform version bumps are **independent** of individual service bumps. A platform release may include multiple service changes, but the platform version bump reflects the overall platform change level, not the sum of service changes.

## Version File Structure

### Platform Version
```
VERSION                    # 0.1.0
```

### Service Versions
```
services/
  data-plane/
    hydrocoptic-datastore/
      VERSION              # 0.2.1
    prefabulated-cache/
      VERSION              # 0.1.5
  processing-plane/
    panametric-fam-processor/
      VERSION              # 1.0.0
```

## Automation

### Automatic Version Bumping on Publish

The `image-publish` Make target automatically handles version bumping:

```bash
# Publish with automatic patch bump (default)
make -C services/data-plane/hydrocoptic-datastore image-publish

# Publish with minor bump
make -C services/data-plane/hydrocoptic-datastore image-publish BUMP=minor

# Publish with major bump
make -C services/data-plane/hydrocoptic-datastore image-publish BUMP=major
```

**How it works:**
1. Reads current version from `services/<plane>/<service>/VERSION`
2. Bumps version according to BUMP parameter (default: patch)
3. Updates VERSION file
4. Updates Helm chart appVersion
5. Builds multi-arch images
6. Tags with new service version
7. Pushes to registry

### Manual Version Bumping (Optional)

You can also manually bump versions without publishing:

```bash
# Bump a specific service version
./scripts/version/bump-service-version.sh <service-path> [major|minor|patch]

# Example
./scripts/version/bump-service-version.sh services/data-plane/hydrocoptic-datastore patch
```

### Platform Version Management

Platform versions are always managed manually:

```bash
# Bump platform version
./scripts/version/bump-platform-version.sh [major|minor|patch]

# Tag all images with platform version
./scripts/version/tag-platform-release.sh
```

## Image Tagging

### Service Images
```
<registry>/encabulator/<service-name>:<service-version>
<registry>/encabulator/<service-name>:<platform-version>
<registry>/encabulator/<service-name>:latest
```

Example:
```
510431938379.dkr.ecr.us-east-2.amazonaws.com/encabulator/hydrocoptic-datastore:0.2.1
510431938379.dkr.ecr.us-east-2.amazonaws.com/encabulator/hydrocoptic-datastore:0.1.0
510431938379.dkr.ecr.us-east-2.amazonaws.com/encabulator/hydrocoptic-datastore:latest
```

### Tagging Strategy
- **Service Version Tag:** Specific service version (e.g., `0.2.1`)
- **Platform Version Tag:** Platform release version (e.g., `0.1.0`)
- **Latest Tag:** Most recent build

## Helm Chart Versioning

Helm charts follow the **platform version**:

```yaml
# helm/charts/hydrocoptic-datastore/Chart.yaml
apiVersion: v2
name: hydrocoptic-datastore
version: 0.1.0              # Platform version
appVersion: "0.2.1"         # Service version
```

## Release Process

### Development Workflow

During development, you can build and test repeatedly without version changes:

```bash
# Make code changes to service
cd services/data-plane/hydrocoptic-datastore
# ... make changes ...

# Build and test locally (no version bump)
make build test

# Build local image for testing (no version bump)
make image
```

### Publishing a Service

When ready to publish, the `image-publish` target automatically bumps the service version:

```bash
# Publish service (automatically bumps patch version and publishes)
make -C services/data-plane/hydrocoptic-datastore image-publish

# Or specify bump type
make -C services/data-plane/hydrocoptic-datastore image-publish BUMP=minor
```

The `image-publish` target will:
1. Automatically bump the service version (default: patch)
2. Build multi-arch images
3. Tag with service version
4. Push to registry

### Platform Release

Platform releases are separate from service publishes and done manually when you want to create a versioned platform release:

```bash
# Bump platform version
./scripts/version/bump-platform-version.sh minor
# Output: 0.0.1 -> 0.1.0

# Tag all service images with platform version
./scripts/version/tag-platform-release.sh

# Update Helm chart versions
./scripts/version/update-helm-versions.sh

# Commit and tag
git add -A
git commit -m "chore: release platform version 0.1.0"
git tag -a v0.1.0 -m "Platform release version 0.1.0"
git push origin main --tags
```

### When to Create Platform Releases

Platform releases are typically created when:
- Multiple services have been updated and tested together
- You want to mark a stable, tested configuration
- Preparing for deployment to production
- Reaching a milestone (e.g., all services deployed, new major feature complete)

You can publish individual services frequently without creating platform releases.

## Version Compatibility Matrix

Platform releases maintain a compatibility matrix:

| Platform Version | Service Versions |
|-----------------|------------------|
| 0.1.0 | hydrocoptic-datastore: 0.2.1<br>prefabulated-cache: 0.1.5<br>... |
| 0.0.1 | hydrocoptic-datastore: 0.2.0<br>prefabulated-cache: 0.1.4<br>... |

This is automatically generated in `CHANGELOG.md` during releases.

## Best Practices

### DO:
- ✅ Bump service version for every service change
- ✅ Bump platform version for releases
- ✅ Tag images with both service and platform versions
- ✅ Update CHANGELOG.md with version changes
- ✅ Test compatibility before platform version bump

### DON'T:
- ❌ Skip service version bumps for "small" changes
- ❌ Bump platform version without service changes
- ❌ Use same version for incompatible changes
- ❌ Forget to update Helm chart appVersion

## Migration from Single Version

Current state: All services at `0.0.1`

Migration plan:
1. Create VERSION files in each service directory
2. Initialize all services at `0.1.0`
3. Set platform version to `0.1.0`
4. Update build scripts to read service versions
5. Update Helm charts with appVersion

## Examples

### Example 1: Bug Fix in Single Service
```bash
# Fix bug in hydrocoptic-datastore
# ... make code changes ...

# Build and test locally (no version bump)
make -C services/data-plane/hydrocoptic-datastore build test

# Publish (automatically bumps patch: 0.0.1 -> 0.0.2)
make -C services/data-plane/hydrocoptic-datastore image-publish
```

### Example 2: New Feature in Service
```bash
# Add feature to panametric-fam-processor
# ... make code changes ...

# Build and test locally
make -C services/processing-plane/panametric-fam-processor build test

# Publish with minor bump (0.0.1 -> 0.1.0)
make -C services/processing-plane/panametric-fam-processor image-publish BUMP=minor
```

### Example 3: Platform Release
```bash
# After publishing several services, create platform release
./scripts/version/bump-platform-version.sh minor
# Platform: 0.0.1 -> 0.1.0

# Tag all images with platform version
./scripts/version/tag-platform-release.sh

# Commit and tag
git add -A
git commit -m "chore: release platform version 0.1.0"
git tag -a v0.1.0 -m "Platform release 0.1.0"
git push origin main --tags
```

## Tooling

All versioning scripts are located in `scripts/version/`:
- `bump-service-version.sh` - Bump individual service version
- `bump-platform-version.sh` - Bump platform version
- `tag-platform-release.sh` - Tag all images with platform version
- `update-helm-versions.sh` - Update Helm chart versions
- `generate-version-matrix.sh` - Generate compatibility matrix

