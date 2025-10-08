# Quick Start: Version Management

Quick guide to managing versions for the Cloud Native Encabulator platform.

## TL;DR

```bash
# Check current version
make version

# Bump version (patch: 0.0.1 -> 0.0.2)
make version-bump-patch

# Publish images with new version
make version-publish

# Push to git
git push origin main
git push origin v0.0.2
```

## Current Version

Check the current version:

```bash
make version
# Output: Current version: 0.0.1
```

Or directly:

```bash
cat VERSION
```

## Bumping Versions

### Patch Release (Bug Fixes)

For bug fixes and minor changes:

```bash
make version-bump-patch
# 0.0.1 -> 0.0.2
```

### Minor Release (New Features)

For new features (backward-compatible):

```bash
make version-bump-minor
# 0.0.1 -> 0.1.0
```

### Major Release (Breaking Changes)

For breaking changes or major updates:

```bash
make version-bump-major
# 0.0.1 -> 1.0.0
```

## Publishing Images

After bumping the version, publish images:

```bash
# Publish with current version from VERSION file
make version-publish
```

This will:
1. Read version from `VERSION` file
2. Build and publish all images with that version tag
3. Also tag images as `latest`

## Complete Release Workflow

### Step-by-Step

```bash
# 1. Make sure all changes are committed
git status

# 2. Bump version (choose appropriate type)
make version-bump-patch

# 3. Publish images
make version-publish

# 4. Push changes and tags to git
git push origin main
git push origin v0.0.2

# 5. Update Helm charts (if needed)
# Edit helm/charts/*/values.yaml
# Set image.tag to new version

# 6. Deploy
./scripts/deploy/deploy-helm.sh
```

## Manual Version Management

If you prefer manual control:

```bash
# 1. Edit VERSION file
echo "0.0.2" > VERSION

# 2. Commit changes
git add VERSION
git commit -m "chore: bump version to 0.0.2"

# 3. Create tag
git tag -a v0.0.2 -m "Release version 0.0.2"

# 4. Publish images
VERSION=$(cat VERSION)
make image-publish IMAGE_TAG=$VERSION
make image-publish IMAGE_TAG=latest

# 5. Push
git push origin main
git push origin v0.0.2
```

## Publishing Specific Versions

### Publish with Custom Tag

```bash
make image-publish IMAGE_TAG=0.0.3-beta.1
```

### Publish Only Version (No Latest)

```bash
VERSION=$(cat VERSION)
make image-publish IMAGE_TAG=$VERSION
# Skip the 'latest' tag
```

## Checking Published Versions

### In ECR

```bash
# List all versions for a service
aws ecr list-images \
  --repository-name encabulator/hydrocoptic-datastore \
  --region us-east-2 \
  --query 'imageIds[*].imageTag' \
  --output table
```

### In Git

```bash
# List all version tags
git tag -l "v*"

# Show latest tag
git describe --tags --abbrev=0
```

### In Kubernetes

```bash
# Check deployed version
kubectl get deployment hydrocoptic-datastore \
  -n encabulator \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
```

## Common Scenarios

### Scenario 1: Regular Release

```bash
# Bug fixes and improvements
make version-bump-patch
make version-publish
git push origin main
git push origin $(git describe --tags --abbrev=0)
```

### Scenario 2: Feature Release

```bash
# New features added
make version-bump-minor
make version-publish
git push origin main
git push origin $(git describe --tags --abbrev=0)
```

### Scenario 3: Development Build

```bash
# Quick test build (no version bump)
make image-publish IMAGE_TAG=dev-$(git rev-parse --short HEAD)
```

### Scenario 4: Hotfix

```bash
# Critical bug fix
git checkout -b hotfix/0.0.2 v0.0.1
# Make fixes
git commit -m "fix: critical bug"
make version-bump-patch
make version-publish
git checkout main
git merge hotfix/0.0.2
git push origin main
git push origin v0.0.2
```

## Version Naming Conventions

### Release Versions
- `0.0.1` - Patch release
- `0.1.0` - Minor release
- `1.0.0` - Major release

### Pre-release Versions
- `0.2.0-alpha.1` - Alpha release
- `0.2.0-beta.1` - Beta release
- `0.2.0-rc.1` - Release candidate

### Development Versions
- `dev-abc123` - Development build
- `feature-xyz-abc123` - Feature branch build

## Troubleshooting

### "VERSION file not found"

Create it:
```bash
echo "0.0.1" > VERSION
git add VERSION
git commit -m "chore: add VERSION file"
```

### "Tag already exists"

Delete and recreate:
```bash
git tag -d v0.0.1
git tag -a v0.0.1 -m "Release version 0.0.1"
git push origin :refs/tags/v0.0.1
git push origin v0.0.1
```

### Wrong version published

Republish with correct version:
```bash
make image-publish IMAGE_TAG=0.0.2
```

## Best Practices

1. ✅ **Always bump version before releasing**
2. ✅ **Use semantic versioning correctly**
3. ✅ **Tag releases in git**
4. ✅ **Update CHANGELOG.md**
5. ✅ **Test before publishing**
6. ✅ **Coordinate with team**

## Next Steps

After publishing a new version:

1. **Update Helm Charts** - Set image tags to new version
2. **Update Documentation** - Document changes in CHANGELOG.md
3. **Deploy to Cluster** - Roll out new version
4. **Monitor** - Watch for issues
5. **Communicate** - Notify team of release

## Resources

- [Full Versioning Guide](./VERSIONING.md)
- [Semantic Versioning](https://semver.org/)
- [Image Publishing Guide](./IMAGE_PUBLISHING.md)
