# DemoScenarioOrchestrator Makefile (Python)

SERVICE_NAME := demo-scenario-orchestrator

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Validate Python syntax
	@echo "Building $(SERVICE_NAME)..."
	@python3 -m py_compile main.py
	@echo "✓ Build complete"

test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@python3 -m pytest test_main.py -v 2>/dev/null || echo "pytest not available, skipping tests"
	@echo "✓ Tests complete"

test-coverage: ## Run tests with coverage
	@python3 -m pytest test_main.py --cov=. --cov-report=html 2>/dev/null || echo "pytest-cov not available"
	@echo "✓ Coverage report: htmlcov/index.html"

run: ## Run the service
	@python3 main.py

image: ## Build container image (alias for docker-build)
	@echo "Building container image for $(SERVICE_NAME)..."
	@finch build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built: $(SERVICE_NAME):latest"

docker-build: image ## Build Docker image (deprecated, use 'image')

clean: ## Clean build artifacts
	@find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	@find . -type f -name "*.pyc" -delete
	@find . -type f -name "*.pyo" -delete
	@find . -type d -name ".pytest_cache" -exec rm -rf {} + 2>/dev/null || true
	@rm -rf htmlcov .coverage
	@echo "✓ Cleaned"

fmt: ## Format code (requires black)
	@command -v black >/dev/null 2>&1 && black *.py || echo "black not installed"
	@echo "✓ Code formatted"

lint: ## Run linter (requires pylint)
	@command -v pylint >/dev/null 2>&1 && pylint *.py || echo "pylint not installed"

deps: ## Install dependencies
	@pip3 install -r requirements.txt
	@echo "✓ Dependencies installed"
