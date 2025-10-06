# Data Plane Makefile

SERVICES := hydrocoptic-datastore prefabulated-cache sinusoidal-database

.PHONY: help
help: ## Show this help message
	@echo "Data Plane - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build all Data Plane services
	@echo "Building all Data Plane services..."
	@for service in $(SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) -C $$service build || exit 1; \
	done
	@echo "✓ All Data Plane services built"

.PHONY: test
test: ## Test all Data Plane services
	@echo "Testing all Data Plane services..."
	@for service in $(SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) -C $$service test || exit 1; \
	done
	@echo "✓ All Data Plane tests complete"

.PHONY: image
image: ## Build container images for all Data Plane services
	@echo "Building container images for all Data Plane services..."
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service image || exit 1; \
	done
	@echo "✓ All Data Plane container images built"

.PHONY: docker-build
docker-build: image ## Build Docker images (deprecated, use 'image')

.PHONY: clean
clean: ## Clean all Data Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service clean; \
	done
	@echo "✓ All Data Plane services cleaned"

.PHONY: fmt
fmt: ## Format all Data Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service fmt; \
	done
	@echo "✓ All Data Plane services formatted"
