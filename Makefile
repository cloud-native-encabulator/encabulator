# Cloud Native Encabulator Microservices - Makefile
# Provides unified build, test, and deployment targets for all services

# Container runtime (docker or finch)
CONTAINER_RUNTIME ?= finch

# ECR Configuration
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest

# Declare all phony targets
.PHONY: help targets \
        build build-data-plane build-processing-plane build-support-plane build-control-plane \
        test test-data-plane test-processing-plane test-support-plane test-control-plane \
        image images image-data-plane image-processing-plane image-support-plane image-control-plane docker-build \
        image-publish image-publish-data-plane image-publish-processing-plane image-publish-support-plane image-publish-control-plane \
        version version-bump-major version-bump-minor version-bump-patch version-publish \
        helm-lint helm-template \
        clean clean-go clean-node clean-python \
        dev-deps fmt fmt-go fmt-node fmt-python \
        ci verify
help: ## Show this help message
	@echo "Cloud Native Encabulator - Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}'

targets: help ## Alias for help

.DEFAULT_GOAL := help

#############################################################################
# Build Targets
#############################################################################

build: build-data-plane build-processing-plane build-support-plane build-control-plane ## Build all services

build-data-plane: ## Build all Data Plane services
	@$(MAKE) -C services/data-plane build

build-processing-plane: ## Build all Processing Plane services
	@$(MAKE) -C services/processing-plane build

build-support-plane: ## Build all Support Plane services
	@$(MAKE) -C services/support-plane build

build-control-plane: ## Build all Control Plane services
	@$(MAKE) -C services/control-plane build

#############################################################################
# Test Targets
#############################################################################

test: test-data-plane test-processing-plane test-support-plane test-control-plane ## Run all tests

test-data-plane: ## Test all Data Plane services
	@$(MAKE) -C services/data-plane test

test-processing-plane: ## Test all Processing Plane services
	@$(MAKE) -C services/processing-plane test

test-support-plane: ## Test all Support Plane services
	@$(MAKE) -C services/support-plane test

test-control-plane: ## Test all Control Plane services
	@$(MAKE) -C services/control-plane test

#############################################################################
# Container Image Targets
#############################################################################

image: image-data-plane image-processing-plane image-support-plane image-control-plane ## Build all container images

images: image ## Alias for image
image-data-plane: ## Build container images for Data Plane services
	@$(MAKE) -C services/data-plane image

image-processing-plane: ## Build container images for Processing Plane services
	@$(MAKE) -C services/processing-plane image

image-support-plane: ## Build container images for Support Plane services
	@$(MAKE) -C services/support-plane image

image-control-plane: ## Build container images for Control Plane services
	@$(MAKE) -C services/control-plane image

docker-build: image ## Build all Docker images (deprecated, use 'image')

image-publish: image-publish-data-plane image-publish-processing-plane image-publish-support-plane image-publish-control-plane ## Publish all container images to ECR

image-publish-data-plane: ## Publish Data Plane container images to ECR
	@$(MAKE) -C services/data-plane image-publish ECR_REGISTRY=$(ECR_REGISTRY) ECR_NAMESPACE=$(ECR_NAMESPACE) IMAGE_TAG=$(IMAGE_TAG)

image-publish-processing-plane: ## Publish Processing Plane container images to ECR
	@$(MAKE) -C services/processing-plane image-publish ECR_REGISTRY=$(ECR_REGISTRY) ECR_NAMESPACE=$(ECR_NAMESPACE) IMAGE_TAG=$(IMAGE_TAG)

image-publish-support-plane: ## Publish Support Plane container images to ECR
	@$(MAKE) -C services/support-plane image-publish ECR_REGISTRY=$(ECR_REGISTRY) ECR_NAMESPACE=$(ECR_NAMESPACE) IMAGE_TAG=$(IMAGE_TAG)

image-publish-control-plane: ## Publish Control Plane container images to ECR
	@$(MAKE) -C services/control-plane image-publish ECR_REGISTRY=$(ECR_REGISTRY) ECR_NAMESPACE=$(ECR_NAMESPACE) IMAGE_TAG=$(IMAGE_TAG)

#############################################################################
# Version Management Targets
#############################################################################

version: ## Show current version
	@echo "Current version: $$(cat VERSION)"

version-bump-major: ## Bump major version (X.0.0)
	@./scripts/version/bump-version.sh major

version-bump-minor: ## Bump minor version (x.X.0)
	@./scripts/version/bump-version.sh minor

version-bump-patch: ## Bump patch version (x.x.X)
	@./scripts/version/bump-version.sh patch

version-publish: ## Publish images with current version from VERSION file
	@./scripts/version/publish-version.sh

#############################################################################
# Helm Targets
#############################################################################

helm-lint: ## Lint all Helm charts
	@echo "Linting Helm charts..."
	@helm lint helm/charts/*/
	@echo "✓ Helm lint complete"

helm-template: ## Test Helm template rendering for all charts
	@echo "Testing Helm template rendering..."
	@for chart in helm/charts/*/; do \
		echo "Testing $$chart..."; \
		helm template test $$chart --dry-run > /dev/null; \
	done
	@echo "✓ Helm template test complete"

#############################################################################
# Clean Targets
#############################################################################

clean: clean-go clean-node clean-python ## Clean all build artifacts

clean-go: ## Clean Go build artifacts
	@echo "Cleaning Go artifacts..."
	@find services -name "*.test" -delete
	@find services -name "*.out" -delete
	@rm -f services/data-plane/hydrocoptic-datastore/hydrocoptic-datastore
	@rm -f services/data-plane/prefabulated-cache/prefabulated-cache
	@rm -f services/data-plane/sinusoidal-database/sinusoidal-database
	@rm -f services/processing-plane/logarithmic-casing-service/logarithmic-casing-service
	@rm -f services/processing-plane/dingle-arm-api-gateway/dingle-arm-api-gateway
	@rm -f services/support-plane/differential-girdle/differential-girdle
	@rm -f services/control-plane/malaise-injection-engine/malaise-injection-engine
	@echo "✓ Go artifacts cleaned"

clean-node: ## Clean Node.js build artifacts
	@echo "Cleaning Node.js artifacts..."
	@find services -name "node_modules" -type d -exec rm -rf {} + 2>/dev/null || true
	@find services -name "dist" -type d -exec rm -rf {} + 2>/dev/null || true
	@echo "✓ Node.js artifacts cleaned"

clean-python: ## Clean Python build artifacts
	@echo "Cleaning Python artifacts..."
	@find services -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
	@find services -name "*.pyc" -delete
	@find services -name "*.pyo" -delete
	@find services -name ".pytest_cache" -type d -exec rm -rf {} + 2>/dev/null || true
	@echo "✓ Python artifacts cleaned"

#############################################################################
# Development Targets
#############################################################################

dev-deps: ## Install development dependencies
	@echo "Installing development dependencies..."
	@command -v go >/dev/null 2>&1 || { echo "Go is not installed"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Node.js is not installed"; exit 1; }
	@command -v python3 >/dev/null 2>&1 || { echo "Python 3 is not installed"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "Docker is not installed"; exit 1; }
	@command -v helm >/dev/null 2>&1 || { echo "Helm is not installed"; exit 1; }
	@echo "✓ All required tools are installed"

fmt: fmt-go fmt-node fmt-python ## Format all code

fmt-go: ## Format Go code
	@echo "Formatting Go code..."
	@find services -name "*.go" -exec gofmt -w {} \;
	@echo "✓ Go code formatted"

fmt-node: ## Format Node.js/TypeScript code
	@echo "Formatting Node.js/TypeScript code..."
	@cd services/processing-plane/panametric-fam-processor && npm run lint --fix 2>/dev/null || true
	@cd services/control-plane/cardinal-grammeter && npm run lint --fix 2>/dev/null || true
	@cd services/control-plane/encabulation-control-console && npm run lint --fix 2>/dev/null || true
	@echo "✓ Node.js/TypeScript code formatted"

fmt-python: ## Format Python code
	@echo "Formatting Python code..."
	@command -v black >/dev/null 2>&1 && find services -name "*.py" -exec black {} \; || echo "black not installed, skipping"
	@echo "✓ Python code formatted"

#############################################################################
# CI/CD Targets
#############################################################################

ci: dev-deps build test helm-lint ## Run CI pipeline (build, test, lint)
	@echo "✓ CI pipeline complete"

verify: build test helm-lint helm-template ## Verify all builds, tests, and Helm charts
	@echo "✓ Verification complete"
