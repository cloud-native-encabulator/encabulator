# Makefile Versioning Integration Guide

This guide explains how to integrate automatic version bumping into service Makefiles.

## Overview

The `image-publish` target now automatically:
1. Bumps the service version (patch/minor/major)
2. Builds multi-arch images
3. Tags with both the new version and `latest`
4. Pushes to registry

## Changes Required

### 1. Add Version Variables

Add these variables after the `CONTAINER_RUNTIME` line:

```makefile
BUMP ?= patch

# Version management
VERSION_FILE := VERSION
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")
```

### 2. Update image-publish Target

Replace the existing `image-publish` target with:

```makefile
image-publish: ## Build and publish multi-platform container image to ECR (auto-bumps version)
	@echo "Current version: $(CURRENT_VERSION)"
	@NEW_VERSION=$$(../../../scripts/version/bump-and-get-version.sh . $(BUMP)); \
	echo "Bumping version ($(BUMP)): $(CURRENT_VERSION) -> $$NEW_VERSION"; \
	echo "Building and publishing multi-platform $(SERVICE_NAME) to ECR..."; \
	echo "  Building for linux/amd64,linux/arm64: $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION"; \
	$(CONTAINER_RUNTIME) build --platform linux/amd64,linux/arm64 \
		-t $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION \
		-t $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest \
		.; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	echo "✓ Successfully published multi-platform $(SERVICE_NAME):$$NEW_VERSION"
```

### 3. Optional: Add version Target

Add a helper target to show the current version:

```makefile
version: ## Show current service version
	@echo "$(CURRENT_VERSION)"
```

## Usage

### Publish with Patch Bump (default)
```bash
make -C services/data-plane/hydrocoptic-datastore image-publish
# 0.0.1 -> 0.0.2
```

### Publish with Minor Bump
```bash
make -C services/data-plane/hydrocoptic-datastore image-publish BUMP=minor
# 0.0.2 -> 0.1.0
```

### Publish with Major Bump
```bash
make -C services/data-plane/hydrocoptic-datastore image-publish BUMP=major
# 0.1.0 -> 1.0.0
```

### Check Current Version
```bash
make -C services/data-plane/hydrocoptic-datastore version
# 0.0.1
```

## Reference Implementation

See `services/data-plane/hydrocoptic-datastore/Makefile` for a complete reference implementation.

## Services to Update

- [x] services/data-plane/hydrocoptic-datastore
- [ ] services/data-plane/prefabulated-cache
- [ ] services/data-plane/sinusoidal-database
- [ ] services/processing-plane/dingle-arm-api-gateway
- [ ] services/processing-plane/logarithmic-casing-service
- [ ] services/processing-plane/panametric-fam-processor
- [ ] services/support-plane/differential-girdle
- [ ] services/support-plane/side-fumbling-collector
- [ ] services/support-plane/unilateral-phase-detractor
- [ ] services/control-plane/cardinal-grammeter
- [ ] services/control-plane/demo-scenario-orchestrator
- [ ] services/control-plane/encabulation-control-console
- [ ] services/control-plane/malaise-injection-engine

## Testing

After updating a Makefile:

1. **Check version display:**
   ```bash
   make -C services/<plane>/<service> version
   ```

2. **Test version bump (dry run):**
   ```bash
   # Just bump version without publishing
   ../../../scripts/version/bump-and-get-version.sh services/<plane>/<service> patch
   ```

3. **Test full publish:**
   ```bash
   make -C services/<plane>/<service> image-publish
   ```

## Troubleshooting

### Version not bumping
- Check that VERSION file exists in service directory
- Verify script path is correct: `../../../scripts/version/bump-and-get-version.sh`

### Image not tagged correctly
- Ensure both version and latest tags are in the build command
- Check that NEW_VERSION variable is being captured correctly

### Permission denied
- Make sure bump-and-get-version.sh is executable:
  ```bash
  chmod +x scripts/version/bump-and-get-version.sh
  ```

