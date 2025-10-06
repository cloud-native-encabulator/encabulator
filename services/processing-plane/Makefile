# Processing Plane Makefile

SERVICES := logarithmic-casing-service dingle-arm-api-gateway panametric-fam-processor

.PHONY: help
help: ## Show this help message
	@echo "Processing Plane - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build all Processing Plane services
	@echo "Building all Processing Plane services..."
	@for service in $(SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) -C $$service build || exit 1; \
	done
	@echo "✓ All Processing Plane services built"

.PHONY: test
test: ## Test all Processing Plane services
	@echo "Testing all Processing Plane services..."
	@for service in $(SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) -C $$service test || exit 1; \
	done
	@echo "✓ All Processing Plane tests complete"

.PHONY: docker-build
docker-build: ## Build Docker images for all Processing Plane services
	@echo "Building Docker images for all Processing Plane services..."
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service docker-build || exit 1; \
	done
	@echo "✓ All Processing Plane Docker images built"

.PHONY: clean
clean: ## Clean all Processing Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service clean; \
	done
	@echo "✓ All Processing Plane services cleaned"

.PHONY: fmt
fmt: ## Format all Processing Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service fmt; \
	done
	@echo "✓ All Processing Plane services formatted"
