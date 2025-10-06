# DifferentialGirdle Makefile

SERVICE_NAME := differential-girdle
BINARY := $(SERVICE_NAME)

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

	
build: ## Build the service
	@echo "Building $(SERVICE_NAME)..."
	@go build -o $(BINARY) .
	@echo "✓ Build complete: $(BINARY)"

test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@go test ./... -v
	@echo "✓ Tests complete"

test-coverage: ## Run tests with coverage
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

run: build ## Build and run the service
	@./$(BINARY)

image: ## Build container image (alias for docker-build)
	@echo "Building container image for $(SERVICE_NAME)..."
	@finch build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built: $(SERVICE_NAME):latest"

docker-build: image ## Build Docker image (deprecated, use 'image')

clean: ## Clean build artifacts
	@rm -f $(BINARY)
	@rm -f *.test *.out coverage.html
	@echo "✓ Cleaned"

fmt: ## Format code
	@gofmt -w .
	@echo "✓ Code formatted"

lint: ## Run linter (requires golangci-lint)
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed"

deps: ## Download dependencies
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies updated"
