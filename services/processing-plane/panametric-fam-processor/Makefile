# PanametricFamProcessor Makefile (Node.js/TypeScript)

SERVICE_NAME := panametric-fam-processor

# ECR Configuration (can be overridden from parent Makefile)
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch
BUMP ?= patch

# Version management
VERSION_FILE := VERSION
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: deps ## Build the service
	@echo "Building $(SERVICE_NAME)..."
	@npm run build
	@echo "✓ Build complete"

test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@npm test
	@echo "✓ Tests complete"

test-coverage: ## Run tests with coverage
	@npm test -- --coverage
	@echo "✓ Coverage report generated"

run: ## Run the service in development mode
	@npm run dev

start: build ## Build and start the service
	@npm start

image: ## Build container image for local platform
	@echo "Building container image for $(SERVICE_NAME)..."
	@$(CONTAINER_RUNTIME) build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built: $(SERVICE_NAME):latest"

image-local-publish: image ## Tag and publish local platform image to ECR
	@echo "Publishing local $(SERVICE_NAME) to ECR..."
	@echo "  Tagging: $(SERVICE_NAME):latest -> $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)"
	@$(CONTAINER_RUNTIME) tag $(SERVICE_NAME):latest $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)
	@echo "  Pushing: $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)"
	@$(CONTAINER_RUNTIME) push $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)
	@echo "✓ Successfully published local $(SERVICE_NAME):$(IMAGE_TAG)"

docker-build: image ## Build Docker image (deprecated, use 'image')

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

clean: ## Clean build artifacts
	@rm -rf dist node_modules
	@echo "✓ Cleaned"

fmt: ## Format code
	@npm run lint --fix 2>/dev/null || echo "Linter not configured"
	@echo "✓ Code formatted"

lint: ## Run linter
	@npm run lint

deps: ## Install dependencies
	@npm install
	@echo "✓ Dependencies installed"
