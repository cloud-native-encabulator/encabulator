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

.PHONY: docker-build
docker-build: ## Build Docker images for all Support Plane services
	@echo "Building Docker images for all Support Plane services..."
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service docker-build || exit 1; \
	done
	@echo "✓ All Support Plane Docker images built"

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
