# Support Plane Makefile

SERVICES := differential-girdle unilateral-phase-detractor side-fumbling-collector

# ECR Configuration (can be overridden from parent Makefile)
ECR_REGISTRY ?= 510431938379.dkr.ecr.us-east-2.amazonaws.com
ECR_NAMESPACE ?= encabulator
IMAGE_TAG ?= latest

# Declare all phony targets
.PHONY: help targets build test image images docker-build image-publish clean fmt

.PHONY: help
help: ## Show this help message

targets: help ## Alias for help
	@echo "Support Plane - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all Support Plane services
	@echo "Building all Support Plane services..."
	@for service in $(SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) -C $$service build || exit 1; \
	done
	@echo "✓ All Support Plane services built"

test: ## Test all Support Plane services
	@echo "Testing all Support Plane services..."
	@for service in $(SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) -C $$service test || exit 1; \
	done
	@echo "✓ All Support Plane tests complete"

image: ## Build container images for all services
	@echo "Building container images..."
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service image || exit 1; \
	done
	@echo "✓ All container images built"

images: image ## Alias for image

docker-build: image ## Build Docker images (deprecated, use 'image')

image-publish: ## Publish container images to ECR for all Support Plane services
	@echo "Publishing container images for all Support Plane services..."
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service image-publish ECR_REGISTRY=$(ECR_REGISTRY) ECR_NAMESPACE=$(ECR_NAMESPACE) IMAGE_TAG=$(IMAGE_TAG) || exit 1; \
	done
	@echo "✓ All Support Plane container images published"

clean: ## Clean all Support Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service clean; \
	done
	@echo "✓ All Support Plane services cleaned"

fmt: ## Format all Support Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service fmt; \
	done
	@echo "✓ All Support Plane services formatted"
