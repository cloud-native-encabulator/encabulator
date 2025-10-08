# Versioning Strategy

This document describes the versioning strategy for the Cloud Native Encabulator platform.

## Overview

The Cloud Native Encabulator uses **unified semantic versioning** across all 13 microservices. This means all services share the same version number and are released together as a cohesive platform.

## Why Unified Versioning?

1. **Simplicity** - One version number to track
2. **Consistency** - All services tested and deployed together
3. **Clear Releases** - Easy to understand what changed
4. **Demo-Friendly** - Simpler for users to deploy and understand
5. **Tight Coupling** - Services are designed to work together

## Semantic Versioning

We follow [Semantic Versioning 2.0.0](https://semver.org/):

```
MAJOR.MINOR.PATCH
```

- **MAJOR** - Incompatible API changes or major architectural changes
- **MINOR** - New features, backward-compatible
- **PATCH** - Bug fixes, backward-compatible

### Current Version

The current version is stored in the `VERSION` file at the repository root:

```bash
cat VERSION
# 0.0.1
```

## Version Management

### Bumping Versions

Use the version bump script:

```bash
# Bump patch version (0.0.1 -> 0.0.2)
./scripts/version/bump-version.sh patch

# Bump minor version (0.0.1 -> 0.1.0)
./scripts/version/bump-version.sh minor

# Bump major version (0.0.1 -> 1.0.0)
./scripts/version/bump-version.sh major
```

The script will:
1. Update the `VERSION` file
2. Create a git commit
3. Create a git tag (e.g., `v0.0.2`)
4. Provide instructions for pushing

### Publishing Images

After bumping the version, publish images:

```bash
# Publish with current version from VERSION file
./scripts/version/publish-version.sh

# This publishes images with both version tag and 'latest'
```

Or manually:

```bash
VERSION=$(cat VERSION)
make image-publish IMAGE_TAG=$VERSION
make image-publish IMAGE_TAG=latest
```

## Release Workflow

### 1. Development Phase

During development, use `latest` tag:

```bash
make image-publish IMAGE_TAG=latest
```

### 2. Preparing a Release

When ready to release:

```bash
# 1. Ensure all changes are committed
git status

# 2. Bump version (choose appropriate type)
./scripts/version/bump-version.sh minor

# 3. Publish images with new version
./scripts/version/publish-version.sh

# 4. Push changes and tags
git push origin main
git push origin v0.1.0

# 5. Update Helm charts (see below)

# 6. Create GitHub release (optional)
```

### 3. Updating Helm Charts

After publishing images, update Helm chart versions:

```bash
VERSION=$(cat VERSION)

# Update chart versions
# Edit helm/charts/*/Chart.yaml
# Set version: $VERSION
# Set appVersion: $VERSION

# Update image tags in values.yaml
# Edit helm/charts/*/values.yaml
# Set image.tag: $VERSION
```

### 4. Deploying

Deploy the new version:

```bash
./scripts/deploy/deploy-helm.sh
```

## Version Tagging Strategy

Each release creates multiple tags:

1. **Version Tag** - Specific version (e.g., `v0.1.0`)
2. **Latest Tag** - Always points to most recent release

Example:
```bash
# After releasing v0.1.0, images are tagged as:
510431938379.dkr.ecr.us-east-2.amazonaws.com/encabulator/hydrocoptic-datastore:0.1.0
510431938379.dkr.ecr.us-east-2.amazonaws.com/encabulator/hydrocoptic-datastore:latest
```

## Pre-release Versions

For pre-release versions, use suffixes:

```bash
# Alpha release
make image-publish IMAGE_TAG=0.2.0-alpha.1

# Beta release
make image-publish IMAGE_TAG=0.2.0-beta.1

# Release candidate
make image-publish IMAGE_TAG=0.2.0-rc.1
```

## Hotfix Workflow

For urgent fixes:

```bash
# 1. Create hotfix branch from release tag
git checkout -b hotfix/0.1.1 v0.1.0

# 2. Make fixes and commit
git commit -m "fix: critical bug"

# 3. Bump patch version
./scripts/version/bump-version.sh patch

# 4. Publish images
./scripts/version/publish-version.sh

# 5. Merge back to main
git checkout main
git merge hotfix/0.1.1

# 6. Push changes
git push origin main
git push origin v0.1.1
```

## Version History

Track version history in `CHANGELOG.md`:

```markdown
## [0.1.0] - 2024-01-15

### Added
- New feature X
- New service Y

### Changed
- Updated service Z

### Fixed
- Bug in service A
```

## CI/CD Integration

### Automated Versioning

For CI/CD pipelines:

```bash
#!/bin/bash
# Get version from VERSION file
VERSION=$(cat VERSION)

# Or use git tags
VERSION=$(git describe --tags --always)

# Publish with version
make image-publish IMAGE_TAG=$VERSION
```

### Branch-based Versioning

Different strategies for different branches:

```bash
# Main branch - use VERSION file
if [ "$BRANCH" = "main" ]; then
    VERSION=$(cat VERSION)
    make image-publish IMAGE_TAG=$VERSION
    make image-publish IMAGE_TAG=latest
fi

# Development branch - use commit SHA
if [ "$BRANCH" = "develop" ]; then
    VERSION="dev-$(git rev-parse --short HEAD)"
    make image-publish IMAGE_TAG=$VERSION
fi

# Feature branches - use branch name
if [[ "$BRANCH" == feature/* ]]; then
    VERSION="$(echo $BRANCH | sed 's/\//-/g')-$(git rev-parse --short HEAD)"
    make image-publish IMAGE_TAG=$VERSION
fi
```

## Alternative: Per-Service Versioning

If you later need independent service versioning:

### Structure
```
services/
  data-plane/
    hydrocoptic-datastore/
      VERSION  # 1.2.3
    prefabulated-cache/
      VERSION  # 1.1.0
```

### Publishing
```bash
# Publish with service-specific version
cd services/data-plane/hydrocoptic-datastore
VERSION=$(cat VERSION)
make image-publish IMAGE_TAG=$VERSION
```

### When to Use
- Services evolve independently
- Different release cycles needed
- Services used by external teams

## Best Practices

1. **Always tag releases** - Use git tags for traceability
2. **Update CHANGELOG** - Document changes in each release
3. **Test before releasing** - Run full test suite
4. **Coordinate deployments** - Deploy all services together
5. **Communicate changes** - Notify team of breaking changes
6. **Keep VERSION file updated** - Single source of truth

## Version Queries

### Check Current Version
```bash
cat VERSION
```

### List All Versions
```bash
git tag -l "v*"
```

### Check Image Versions in ECR
```bash
aws ecr list-images \
  --repository-name encabulator/hydrocoptic-datastore \
  --region us-east-2 \
  --query 'imageIds[*].imageTag' \
  --output table
```

### Find Version of Running Service
```bash
kubectl get deployment hydrocoptic-datastore \
  -n encabulator \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
```

## Rollback Strategy

To rollback to a previous version:

```bash
# 1. Identify previous version
git tag -l "v*"

# 2. Deploy previous version
VERSION=0.0.1
helm upgrade encabulator-data-plane \
  helm/charts/encabulator-data-plane \
  --set image.tag=$VERSION \
  --namespace encabulator
```

## Version Compatibility Matrix

Track compatibility between versions:

| Platform Version | Kubernetes | Helm | Go | Node.js | Python |
|-----------------|------------|------|-----|---------|--------|
| 0.1.0           | 1.28+      | 3.12+| 1.21| 18+     | 3.11+  |
| 0.0.1           | 1.28+      | 3.12+| 1.21| 18+     | 3.11+  |

## Resources

- [Semantic Versioning](https://semver.org/)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [Keep a Changelog](https://keepachangelog.com/)
