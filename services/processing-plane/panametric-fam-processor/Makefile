# SERVICE_NAME Service Makefile

SERVICE_NAME := panametric-fam-processor

# Project paths
PROJECT_ROOT := ../../..
HELM_CHART := $(PROJECT_ROOT)/helm/charts/$(SERVICE_NAME)
VERSION_SCRIPT := $(PROJECT_ROOT)/scripts/version/bump-and-get-version.sh

# ECR Configuration
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch
BUMP ?= patch
K8S_NAMESPACE := encabulator-processing

# Version management
VERSION_FILE := VERSION
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: deps ## Build the service
	@npm run build
	@echo "✓ Build complete"

test: ## Run tests
	@npm test

image: ## Build container image for local platform
	@$(CONTAINER_RUNTIME) build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built"

image-build: ## Build multi-platform container image (no push)
	@$(CONTAINER_RUNTIME) build --platform linux/amd64,linux/arm64 \
		-t $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG) .
	@echo "✓ Successfully built $(SERVICE_NAME):$(IMAGE_TAG)"

image-publish: ## Build and publish (auto-bumps version)
	@NEW_VERSION=$$($(VERSION_SCRIPT) . $(BUMP)); \
	$(MAKE) IMAGE_TAG=$$NEW_VERSION image-build; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION; \
	$(CONTAINER_RUNTIME) tag $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	echo "✓ Published $(SERVICE_NAME):$$NEW_VERSION"

dev-deploy: ## Build with 'dev' tag and deploy (no version bump)
	@$(MAKE) IMAGE_TAG=dev image-build
	@$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):dev
	@helm upgrade $(SERVICE_NAME) $(HELM_CHART) -n $(K8S_NAMESPACE) --set image.tag=dev --set image.pullPolicy=Always
	@echo "✓ Deployed $(SERVICE_NAME):dev"

clean: ## Clean build artifacts
	@rm -rf dist node_modules
	@echo "✓ Cleaned"

deps: ## Install dependencies
	@npm install
