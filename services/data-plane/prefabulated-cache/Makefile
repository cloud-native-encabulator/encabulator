# PrefabulatedCache Service Makefile

SERVICE_NAME := prefabulated-cache
BINARY := $(SERVICE_NAME)

# ECR Configuration (can be overridden from parent Makefile)
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

	
build: ## Build the service
	@echo "Building $(SERVICE_NAME)..."
	@go build -o $(BINARY) ./cmd/server
	@echo "✓ Build complete: $(BINARY)"

build-static: ## Build statically linked binary (for containers)
	@echo "Building static $(SERVICE_NAME)..."
	@CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o $(BINARY) ./cmd/server
	@echo "✓ Static build complete: $(BINARY)"

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
