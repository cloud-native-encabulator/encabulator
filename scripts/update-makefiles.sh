#!/bin/bash
# Script to update all service Makefiles with standardized pattern

set -e

# Function to generate Makefile for Go services
generate_go_makefile() {
    local service_name=$1
    local plane=$2
    local k8s_namespace=$3
    local makefile_path=$4
    
    cat > "$makefile_path" << 'EOF'
# SERVICE_NAME Service Makefile

SERVICE_NAME := SERVICE_NAME_VALUE
BINARY := $(SERVICE_NAME)

# Project paths
PROJECT_ROOT := ../../..
HELM_CHART := $(PROJECT_ROOT)/helm/charts/$(SERVICE_NAME)
VERSION_SCRIPT := $(PROJECT_ROOT)/scripts/version/bump-and-get-version.sh

# ECR Configuration
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch
BUMP ?= patch
K8S_NAMESPACE := K8S_NAMESPACE_VALUE

# Version management
VERSION_FILE := VERSION
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the service
	@echo "Building $(SERVICE_NAME)..."
	@go build -o $(BINARY) ./cmd/server
	@echo "✓ Build complete: $(BINARY)"

test: ## Run tests
	@echo "Testing $(SERVICE_NAME)..."
	@go test ./... -v
	@echo "✓ Tests complete"

image: ## Build container image for local platform
	@echo "Building container image for $(SERVICE_NAME)..."
	@$(CONTAINER_RUNTIME) build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built: $(SERVICE_NAME):latest"

image-build: ## Build multi-platform container image (no push)
	@echo "Building multi-platform $(SERVICE_NAME)..."
	@$(CONTAINER_RUNTIME) build --platform linux/amd64,linux/arm64 \
		-t $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG) .
	@echo "✓ Successfully built $(SERVICE_NAME):$(IMAGE_TAG)"

image-publish: ## Build and publish (auto-bumps version)
	@NEW_VERSION=$$($(VERSION_SCRIPT) . $(BUMP)); \
	$(MAKE) IMAGE_TAG=$$NEW_VERSION image-build; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION; \
	$(CONTAINER_RUNTIME) tag $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	echo "✓ Published $(SERVICE_NAME):$$NEW_VERSION"

dev-deploy: ## Build with 'dev' tag and deploy (no version bump)
	@$(MAKE) IMAGE_TAG=dev image-build
	@$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):dev
	@helm upgrade $(SERVICE_NAME) $(HELM_CHART) -n $(K8S_NAMESPACE) --set image.tag=dev --set image.pullPolicy=Always
	@echo "✓ Deployed $(SERVICE_NAME):dev"

clean: ## Clean build artifacts
	@rm -f $(BINARY) *.test *.out coverage.html
	@echo "✓ Cleaned"

fmt: ## Format code
	@gofmt -w .

lint: ## Run linter
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed"

deps: ## Download dependencies
	@go mod download && go mod tidy
EOF

    # Replace placeholders
    sed -i.bak "s/SERVICE_NAME_VALUE/$service_name/g" "$makefile_path"
    sed -i.bak "s/K8S_NAMESPACE_VALUE/$k8s_namespace/g" "$makefile_path"
    rm "${makefile_path}.bak"
}

# Function to generate Makefile for Node.js services  
generate_node_makefile() {
    local service_name=$1
    local plane=$2
    local k8s_namespace=$3
    local makefile_path=$4
    
    cat > "$makefile_path" << 'EOF'
# SERVICE_NAME Service Makefile

SERVICE_NAME := SERVICE_NAME_VALUE

# Project paths
PROJECT_ROOT := ../../..
HELM_CHART := $(PROJECT_ROOT)/helm/charts/$(SERVICE_NAME)
VERSION_SCRIPT := $(PROJECT_ROOT)/scripts/version/bump-and-get-version.sh

# ECR Configuration
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch
BUMP ?= patch
K8S_NAMESPACE := K8S_NAMESPACE_VALUE

# Version management
VERSION_FILE := VERSION
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: deps ## Build the service
	@npm run build
	@echo "✓ Build complete"

test: ## Run tests
	@npm test

image: ## Build container image for local platform
	@$(CONTAINER_RUNTIME) build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built"

image-build: ## Build multi-platform container image (no push)
	@$(CONTAINER_RUNTIME) build --platform linux/amd64,linux/arm64 \
		-t $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG) .
	@echo "✓ Successfully built $(SERVICE_NAME):$(IMAGE_TAG)"

image-publish: ## Build and publish (auto-bumps version)
	@NEW_VERSION=$$($(VERSION_SCRIPT) . $(BUMP)); \
	$(MAKE) IMAGE_TAG=$$NEW_VERSION image-build; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION; \
	$(CONTAINER_RUNTIME) tag $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	echo "✓ Published $(SERVICE_NAME):$$NEW_VERSION"

dev-deploy: ## Build with 'dev' tag and deploy (no version bump)
	@$(MAKE) IMAGE_TAG=dev image-build
	@$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):dev
	@helm upgrade $(SERVICE_NAME) $(HELM_CHART) -n $(K8S_NAMESPACE) --set image.tag=dev --set image.pullPolicy=Always
	@echo "✓ Deployed $(SERVICE_NAME):dev"

clean: ## Clean build artifacts
	@rm -rf dist node_modules
	@echo "✓ Cleaned"

deps: ## Install dependencies
	@npm install
EOF

    # Replace placeholders
    sed -i.bak "s/SERVICE_NAME_VALUE/$service_name/g" "$makefile_path"
    sed -i.bak "s/K8S_NAMESPACE_VALUE/$k8s_namespace/g" "$makefile_path"
    rm "${makefile_path}.bak"
}

# Function to generate Makefile for Python services
generate_python_makefile() {
    local service_name=$1
    local plane=$2
    local k8s_namespace=$3
    local makefile_path=$4
    
    cat > "$makefile_path" << 'EOF'
# SERVICE_NAME Service Makefile

SERVICE_NAME := SERVICE_NAME_VALUE

# Project paths
PROJECT_ROOT := ../../..
HELM_CHART := $(PROJECT_ROOT)/helm/charts/$(SERVICE_NAME)
VERSION_SCRIPT := $(PROJECT_ROOT)/scripts/version/bump-and-get-version.sh

# ECR Configuration
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest
CONTAINER_RUNTIME ?= finch
BUMP ?= patch
K8S_NAMESPACE := K8S_NAMESPACE_VALUE

# Version management
VERSION_FILE := VERSION
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "0.0.1")

.PHONY: help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

test: ## Run tests
	@pytest

image: ## Build container image for local platform
	@$(CONTAINER_RUNTIME) build -t $(SERVICE_NAME):latest .
	@echo "✓ Container image built"

image-build: ## Build multi-platform container image (no push)
	@$(CONTAINER_RUNTIME) build --platform linux/amd64,linux/arm64 \
		-t $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$(IMAGE_TAG) .
	@echo "✓ Successfully built $(SERVICE_NAME):$(IMAGE_TAG)"

image-publish: ## Build and publish (auto-bumps version)
	@NEW_VERSION=$$($(VERSION_SCRIPT) . $(BUMP)); \
	$(MAKE) IMAGE_TAG=$$NEW_VERSION image-build; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION; \
	$(CONTAINER_RUNTIME) tag $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):$$NEW_VERSION $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):latest; \
	echo "✓ Published $(SERVICE_NAME):$$NEW_VERSION"

dev-deploy: ## Build with 'dev' tag and deploy (no version bump)
	@$(MAKE) IMAGE_TAG=dev image-build
	@$(CONTAINER_RUNTIME) push --platform linux/amd64,linux/arm64 $(ECR_REGISTRY)/$(ECR_NAMESPACE)/$(SERVICE_NAME):dev
	@helm upgrade $(SERVICE_NAME) $(HELM_CHART) -n $(K8S_NAMESPACE) --set image.tag=dev --set image.pullPolicy=Always
	@echo "✓ Deployed $(SERVICE_NAME):dev"

clean: ## Clean build artifacts
	@find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
	@find . -type f -name '*.pyc' -delete
	@echo "✓ Cleaned"

deps: ## Install dependencies
	@pip install -r requirements.txt
EOF

    # Replace placeholders
    sed -i.bak "s/SERVICE_NAME_VALUE/$service_name/g" "$makefile_path"
    sed -i.bak "s/K8S_NAMESPACE_VALUE/$k8s_namespace/g" "$makefile_path"
    rm "${makefile_path}.bak"
}

echo "Updating service Makefiles..."

# Control Plane (Go and Node.js services)
generate_node_makefile "cardinal-grammeter" "control-plane" "encabulator-control" "services/control-plane/cardinal-grammeter/Makefile"
generate_go_makefile "demo-scenario-orchestrator" "control-plane" "encabulator-control" "services/control-plane/demo-scenario-orchestrator/Makefile"
# encabulation-control-console already done
generate_go_makefile "malaise-injection-engine" "control-plane" "encabulator-control" "services/control-plane/malaise-injection-engine/Makefile"

# Data Plane (Go services)
generate_go_makefile "hydrocoptic-datastore" "data-plane" "encabulator-data" "services/data-plane/hydrocoptic-datastore/Makefile"
generate_go_makefile "prefabulated-cache" "data-plane" "encabulator-data" "services/data-plane/prefabulated-cache/Makefile"
generate_go_makefile "sinusoidal-database" "data-plane" "encabulator-data" "services/data-plane/sinusoidal-database/Makefile"

# Processing Plane (Go and Node.js services)
generate_go_makefile "dingle-arm-api-gateway" "processing-plane" "encabulator-processing" "services/processing-plane/dingle-arm-api-gateway/Makefile"
generate_go_makefile "logarithmic-casing-service" "processing-plane" "encabulator-processing" "services/processing-plane/logarithmic-casing-service/Makefile"
generate_node_makefile "panametric-fam-processor" "processing-plane" "encabulator-processing" "services/processing-plane/panametric-fam-processor/Makefile"

# Support Plane (Go and Python services)
generate_go_makefile "differential-girdle" "support-plane" "encabulator-support" "services/support-plane/differential-girdle/Makefile"
generate_python_makefile "side-fumbling-collector" "support-plane" "encabulator-support" "services/support-plane/side-fumbling-collector/Makefile"
generate_python_makefile "unilateral-phase-detractor" "support-plane" "encabulator-support" "services/support-plane/unilateral-phase-detractor/Makefile"

echo "✓ All Makefiles updated successfully!"
echo ""
echo "New targets available:"
echo "  make image-build    - Build multi-platform image (no push)"
echo "  make image-publish  - Build, bump version, and publish"
echo "  make dev-deploy     - Build with 'dev' tag and deploy to cluster"
