# EncabulationControlConsole Makefile (React + Node.js)

SERVICE_NAME := encabulation-control-console

# ECR Configuration (can be overridden from parent Makefile)
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: deps build-frontend build-backend ## Build frontend and backend
	@echo "✓ Full build complete"

build-frontend: ## Build React frontend
	@echo "Building frontend..."
	@npm run build
	@echo "✓ Frontend build complete"

build-backend: ## Build Node.js backend
	@echo "Building backend..."
	@cd server && npm install && npm run build
	@echo "✓ Backend build complete"

test: test-frontend test-backend ## Run all tests

test-frontend: ## Run frontend tests
	@echo "Testing frontend..."
	@npm test
	@echo "✓ Frontend tests complete"

test-backend: ## Run backend tests
	@echo "Testing backend..."
	@cd server && npm test
	@echo "✓ Backend tests complete"

run: ## Run frontend and backend in development mode
	@echo "Run 'make run-frontend' and 'make run-backend' in separate terminals"

run-frontend: ## Run frontend dev server
	@npm run dev

run-backend: ## Run backend dev server
	@cd server && npm run dev

image: ## Build container image (alias for docker-build)
	@echo "Building container image for $(SERVICE_NAME)..."
	@$(CONTAINER_RUNTIME) build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built: $(SERVICE_NAME):latest"

docker-build: image ## Build Docker image (deprecated, use 'image')

image-publish: image ## Tag and publish container image to ECR
	@echo "Publishing $(SERVICE_NAME) to ECR..."
	@echo "  Tagging: $(SERVICE_NAME):latest -> $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)"
	@$(CONTAINER_RUNTIME) tag $(SERVICE_NAME):latest $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)
	@echo "  Pushing: $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)"
	@$(CONTAINER_RUNTIME) push $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG)
	@echo "✓ Successfully published $(SERVICE_NAME):$(IMAGE_TAG)"

clean: ## Clean build artifacts
	@rm -rf dist node_modules server/dist server/node_modules
	@echo "✓ Cleaned"

fmt: ## Format code
	@npm run lint --fix 2>/dev/null || echo "Linter not configured"
	@echo "✓ Code formatted"

lint: ## Run linter
	@npm run lint

deps: ## Install dependencies
	@npm install
	@cd server && npm install
	@echo "✓ Dependencies installed"
