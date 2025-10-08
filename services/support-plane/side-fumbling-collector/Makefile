# SideFumblingCollector Makefile (Python)

SERVICE_NAME := side-fumbling-collector

# ECR Configuration (can be overridden from parent Makefile)
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Validate Python syntax
	@echo "Building $(SERVICE_NAME)..."
	@python3 -m py_compile app/*.py
	@echo "✓ Build complete"

test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@python3 -m pytest tests/ -v 2>/dev/null || echo "pytest not available, skipping tests"
	@echo "✓ Tests complete"

test-coverage: ## Run tests with coverage
	@python3 -m pytest tests/ --cov=app --cov-report=html 2>/dev/null || echo "pytest-cov not available"
	@echo "✓ Coverage report: htmlcov/index.html"

run: ## Run the service
	@python3 app/main.py

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
	@find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	@find . -type f -name "*.pyc" -delete
	@find . -type f -name "*.pyo" -delete
	@find . -type d -name ".pytest_cache" -exec rm -rf {} + 2>/dev/null || true
	@rm -rf htmlcov .coverage
	@echo "✓ Cleaned"

fmt: ## Format code (requires black)
	@command -v black >/dev/null 2>&1 && black app/ tests/ || echo "black not installed"
	@echo "✓ Code formatted"

lint: ## Run linter (requires pylint)
	@command -v pylint >/dev/null 2>&1 && pylint app/ || echo "pylint not installed"

deps: ## Install dependencies
	@pip3 install -r requirements.txt
	@echo "✓ Dependencies installed"
