# Support Plane Makefile

SERVICES := differential-girdle cardinal-grammeter side-fumbling-collector

.PHONY: help
help: ## Show this help message
	@echo "Support Plane - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build all Support Plane services
	@echo "Building all Support Plane services..."
	@for service in $(SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) -C $$service build || exit 1; \
	done
	@echo "✓ All Support Plane services built"

.PHONY: test
test: ## Test all Support Plane services
	@echo "Testing all Support Plane services..."
	@for service in $(SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) -C $$service test || exit 1; \
	done
	@echo "✓ All Support Plane tests complete"

.PHONY: image
image: ## Build container images for all services
@echo "Building container images..."
@for service in $(SERVICES); do \
$(MAKE) -C $$service image || exit 1; \
done
@echo "✓ All container images built"


.PHONY: docker-build
docker-build: image ## Build Docker images (deprecated, use 'image')
.PHONY: clean
clean: ## Clean all Support Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service clean; \
	done
	@echo "✓ All Support Plane services cleaned"

.PHONY: fmt
fmt: ## Format all Support Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service fmt; \
	done
	@echo "✓ All Support Plane services formatted"
