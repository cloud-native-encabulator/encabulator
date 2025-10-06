# Processing Plane Makefile

SERVICES := logarithmic-casing-service dingle-arm-api-gateway panametric-fam-processor

# Declare all phony targets
.PHONY: help targets build test image images docker-build clean fmt

.PHONY: help
help: ## Show this help message

targets: help ## Alias for help
	@echo "Processing Plane - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all Processing Plane services
	@echo "Building all Processing Plane services..."
	@for service in $(SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) -C $$service build || exit 1; \
	done
	@echo "✓ All Processing Plane services built"

test: ## Test all Processing Plane services
	@echo "Testing all Processing Plane services..."
	@for service in $(SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) -C $$service test || exit 1; \
	done
	@echo "✓ All Processing Plane tests complete"

image: ## Build container images for all services
	@echo "Building container images..."
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service image || exit 1; \
	done
	@echo "✓ All container images built"

images: image ## Alias for image

docker-build: image ## Build Docker images (deprecated, use 'image')
clean: ## Clean all Processing Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service clean; \
	done
	@echo "✓ All Processing Plane services cleaned"

fmt: ## Format all Processing Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service fmt; \
	done
	@echo "✓ All Processing Plane services formatted"
