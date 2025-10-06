# Control Plane Makefile

SERVICES := malaise-injection-engine unilateral-phase-detractor encabulation-control-console demo-scenario-orchestrator

.PHONY: help
help: ## Show this help message
	@echo "Control Plane - Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build all Control Plane services
	@echo "Building all Control Plane services..."
	@for service in $(SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) -C $$service build || exit 1; \
	done
	@echo "✓ All Control Plane services built"

.PHONY: test
test: ## Test all Control Plane services
	@echo "Testing all Control Plane services..."
	@for service in $(SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) -C $$service test || exit 1; \
	done
	@echo "✓ All Control Plane tests complete"

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
clean: ## Clean all Control Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service clean; \
	done
	@echo "✓ All Control Plane services cleaned"

.PHONY: fmt
fmt: ## Format all Control Plane services
	@for service in $(SERVICES); do \
		$(MAKE) -C $$service fmt; \
	done
	@echo "✓ All Control Plane services formatted"
