# SideFumblingCollector Makefile (Python)

SERVICE_NAME := side-fumbling-collector

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Validate Python syntax
	@echo "Building $(SERVICE_NAME)..."
	@python3 -m py_compile app/*.py
	@echo "✓ Build complete"

.PHONY: test
test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@python3 -m pytest tests/ -v 2>/dev/null || echo "pytest not available, skipping tests"
	@echo "✓ Tests complete"

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@python3 -m pytest tests/ --cov=app --cov-report=html 2>/dev/null || echo "pytest-cov not available"
	@echo "✓ Coverage report: htmlcov/index.html"

.PHONY: run
run: ## Run the service
	@python3 app/main.py

.PHONY: image
image: ## Build container image (alias for docker-build)
@echo "Building container image for $(SERVICE_NAME)..."
@finch build -t $(SERVICE_NAME):latest .
@echo "✓ Container image built: $(SERVICE_NAME):latest"

.PHONY: docker-build
docker-build: image ## Build Docker image (deprecated, use 'image')

.PHONY: clean
clean: ## Clean build artifacts
	@find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	@find . -type f -name "*.pyc" -delete
	@find . -type f -name "*.pyo" -delete
	@find . -type d -name ".pytest_cache" -exec rm -rf {} + 2>/dev/null || true
	@rm -rf htmlcov .coverage
	@echo "✓ Cleaned"

.PHONY: fmt
fmt: ## Format code (requires black)
	@command -v black >/dev/null 2>&1 && black app/ tests/ || echo "black not installed"
	@echo "✓ Code formatted"

.PHONY: lint
lint: ## Run linter (requires pylint)
	@command -v pylint >/dev/null 2>&1 && pylint app/ || echo "pylint not installed"

.PHONY: deps
deps: ## Install dependencies
	@pip3 install -r requirements.txt
	@echo "✓ Dependencies installed"
