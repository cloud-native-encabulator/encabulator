# PanametricFamProcessor Makefile (Node.js/TypeScript)

SERVICE_NAME := panametric-fam-processor

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: deps ## Build the service
	@echo "Building $(SERVICE_NAME)..."
	@npm run build
	@echo "✓ Build complete"

.PHONY: test
test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@npm test
	@echo "✓ Tests complete"

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@npm test -- --coverage
	@echo "✓ Coverage report generated"

.PHONY: run
run: ## Run the service in development mode
	@npm run dev

.PHONY: start
start: build ## Build and start the service
	@npm start

.PHONY: image
image: ## Build container image (alias for docker-build)
@echo "Building container image for $(SERVICE_NAME)..."
@finch build -t $(SERVICE_NAME):latest .
@echo "✓ Container image built: $(SERVICE_NAME):latest"

.PHONY: docker-build
docker-build: image ## Build Docker image (deprecated, use 'image')

.PHONY: clean
clean: ## Clean build artifacts
	@rm -rf dist node_modules
	@echo "✓ Cleaned"

.PHONY: fmt
fmt: ## Format code
	@npm run lint --fix 2>/dev/null || echo "Linter not configured"
	@echo "✓ Code formatted"

.PHONY: lint
lint: ## Run linter
	@npm run lint

.PHONY: deps
deps: ## Install dependencies
	@npm install
	@echo "✓ Dependencies installed"
