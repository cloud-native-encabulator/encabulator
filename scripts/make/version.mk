# Common versioning functionality for service Makefiles
# Include this file in service Makefiles with: include ../../../scripts/make/version.mk

# Version configuration
BUMP ?= patch
VERSION_FILE := VERSION
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")
VERSION_SCRIPT := ../../../scripts/version/bump-and-get-version.sh

# Helper target to bump version and return new version
.PHONY: bump-version
bump-version:
	@$(VERSION_SCRIPT) . $(BUMP)

# Helper target to show current version
.PHONY: version
version: ## Show current service version
	@echo "$(CURRENT_VERSION)"

