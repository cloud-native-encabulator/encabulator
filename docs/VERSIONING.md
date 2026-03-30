# Versioning

The Cloud Native Encabulator uses **unified semantic versioning** — all services share the same version number stored in the `VERSION` file at the repository root.

## Quick Reference

```bash
make version                # Show current version
make version-bump-patch     # Bump patch (0.0.1 → 0.0.2)
make version-bump-minor     # Bump minor (0.0.1 → 0.1.0)
make version-bump-major     # Bump major (0.0.1 → 1.0.0)
make version-publish        # Publish all images with current version
```

## Semantic Versioning

We follow [SemVer 2.0.0](https://semver.org/):

- **MAJOR** — Incompatible API or architectural changes
- **MINOR** — New features, backward-compatible
- **PATCH** — Bug fixes, backward-compatible

## Release Workflow

```bash
# 1. Ensure all changes are committed
git status

# 2. Bump version
make version-bump-patch

# 3. Publish images
make version-publish

# 4. Push changes and tag
git push origin main
git push origin v$(cat VERSION)
```

## Version Scripts

Located in `scripts/version/`:

- `bump-version.sh` — Bumps the VERSION file, creates git commit and tag
- `publish-version.sh` — Publishes all images with the current version and `latest` tag

### Manual Usage

```bash
./scripts/version/bump-version.sh patch   # or minor, major
./scripts/version/publish-version.sh
```

## Makefile Integration

Each service Makefile supports automatic version bumping on publish:

```bash
# Publish single service with auto patch bump
make -C services/data-plane/hydrocoptic-datastore image-publish

# Publish with minor bump
make -C services/data-plane/hydrocoptic-datastore image-publish BUMP=minor
```

Service Makefiles read from a local `VERSION` file and use `scripts/version/bump-and-get-version.sh` to increment before publishing.

## Pre-release and Development Builds

```bash
# Pre-release
make image-publish IMAGE_TAG=0.2.0-beta.1

# Development build
make image-publish IMAGE_TAG=dev-$(git rev-parse --short HEAD)
```

## Hotfix Workflow

```bash
git checkout -b hotfix/0.0.2 v0.0.1
# Make fixes, commit
make version-bump-patch
make version-publish
git checkout main
git merge hotfix/0.0.2
git push origin main
git push origin v$(cat VERSION)
```

## CI/CD Integration

```yaml
name: Release
on:
  push:
    tags: ['v*']
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Get version
        run: echo "VERSION=$(cat VERSION)" >> $GITHUB_OUTPUT
        id: version
      - name: Publish images
        run: make version-publish
```

## Checking Versions

```bash
cat VERSION                          # Current version
git tag -l "v*"                      # All version tags
git describe --tags --abbrev=0       # Latest tag
```
