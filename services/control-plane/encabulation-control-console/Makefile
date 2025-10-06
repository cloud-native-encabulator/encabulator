# EncabulationControlConsole Makefile (React + Node.js)

SERVICE_NAME := encabulation-control-console

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: deps build-frontend build-backend ## Build frontend and backend
	@echo "✓ Full build complete"

.PHONY: build-frontend
build-frontend: ## Build React frontend
	@echo "Building frontend..."
	@npm run build
	@echo "✓ Frontend build complete"

.PHONY: build-backend
build-backend: ## Build Node.js backend
	@echo "Building backend..."
	@cd server && npm install && npm run build
	@echo "✓ Backend build complete"

.PHONY: test
test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@npm test
	@echo "✓ Tests complete"

.PHONY: run
run: ## Run frontend and backend in development mode
	@echo "Run 'make run-frontend' and 'make run-backend' in separate terminals"

.PHONY: run-frontend
run-frontend: ## Run frontend dev server
	@npm run dev

.PHONY: run-backend
run-backend: ## Run backend dev server
	@cd server && npm run dev

.PHONY: docker-build
docker-build: ## Build Docker image
	@docker build -t $(SERVICE_NAME):latest .
	@echo "✓ Docker image built: $(SERVICE_NAME):latest"

.PHONY: clean
clean: ## Clean build artifacts
	@rm -rf dist node_modules server/dist server/node_modules
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
	@cd server && npm install
	@echo "✓ Dependencies installed"
