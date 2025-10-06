# Cloud Native Encabulator Microservices - Makefile
# Provides unified build, test, and deployment targets for all services

# Container runtime (docker or finch)
CONTAINER_RUNTIME ?= finch

.PHONY: help
help: ## Show this help message
	@echo "Cloud Native Encabulator - Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

#############################################################################
# Build Targets
#############################################################################

.PHONY: build
build: build-data-plane build-processing-plane build-support-plane build-control-plane ## Build all services

.PHONY: build-data-plane
build-data-plane: ## Build all Data Plane services
	@$(MAKE) -C services/data-plane build

.PHONY: build-processing-plane
build-processing-plane: ## Build all Processing Plane services
	@$(MAKE) -C services/processing-plane build

.PHONY: build-support-plane
build-support-plane: ## Build all Support Plane services
	@$(MAKE) -C services/support-plane build

.PHONY: build-control-plane
build-control-plane: ## Build all Control Plane services
	@$(MAKE) -C services/control-plane build

#############################################################################
# Test Targets
#############################################################################

.PHONY: test
test: test-data-plane test-processing-plane test-support-plane test-control-plane ## Run all tests

.PHONY: test-data-plane
test-data-plane: ## Test all Data Plane services
	@$(MAKE) -C services/data-plane test

.PHONY: test-processing-plane
test-processing-plane: ## Test all Processing Plane services
	@$(MAKE) -C services/processing-plane test

.PHONY: test-support-plane
test-support-plane: ## Test all Support Plane services
	@$(MAKE) -C services/support-plane test

.PHONY: test-control-plane
test-control-plane: ## Test all Control Plane services
	@$(MAKE) -C services/control-plane test

#############################################################################
# Individual Service Targets
#############################################################################

# Data Plane
.PHONY: build-hydrocoptic-datastore
build-hydrocoptic-datastore: ## Build HydrocopticDataStore
	@cd services/data-plane/hydrocoptic-datastore && go build -o hydrocoptic-datastore .

.PHONY: build-prefabulated-cache
build-prefabulated-cache: ## Build PrefabulatedCache
	@cd services/data-plane/prefabulated-cache && go build -o prefabulated-cache .

.PHONY: build-sinusoidal-database
build-sinusoidal-database: ## Build SinusoidalDatabase
	@cd services/data-plane/sinusoidal-database && go build -o sinusoidal-database .

# Processing Plane
.PHONY: build-logarithmic-casing-service
build-logarithmic-casing-service: ## Build LogarithmicCasingService
	@cd services/processing-plane/logarithmic-casing-service && go build -o logarithmic-casing-service .

.PHONY: build-dingle-arm-api-gateway
build-dingle-arm-api-gateway: ## Build DingleArmAPIGateway
	@cd services/processing-plane/dingle-arm-api-gateway && go build -o dingle-arm-api-gateway .

.PHONY: build-panametric-fam-processor
build-panametric-fam-processor: ## Build PanametricFamProcessor
	@cd services/processing-plane/panametric-fam-processor && npm install && npm run build

# Support Plane
.PHONY: build-differential-girdle
build-differential-girdle: ## Build DifferentialGirdle
	@cd services/support-plane/differential-girdle && go build -o differential-girdle .

.PHONY: build-cardinal-grammeter
build-cardinal-grammeter: ## Build CardinalGrammeter
	@cd services/support-plane/cardinal-grammeter && python3 -m py_compile app/*.py

.PHONY: build-side-fumbling-collector
build-side-fumbling-collector: ## Build SideFumblingCollector
	@cd services/support-plane/side-fumbling-collector && python3 -m py_compile app/*.py

# Control Plane
.PHONY: build-malaise-injection-engine
build-malaise-injection-engine: ## Build MalaiseInjectionEngine
	@cd services/control-plane/malaise-injection-engine && go build -o malaise-injection-engine .

.PHONY: build-unilateral-phase-detractor
build-unilateral-phase-detractor: ## Build UnilateralPhaseDetractor
	@cd services/control-plane/unilateral-phase-detractor && npm install && npm run build

.PHONY: build-encabulation-control-console
build-encabulation-control-console: ## Build EncabulationControlConsole
	@cd services/control-plane/encabulation-control-console && npm install && npm run build
	@cd services/control-plane/encabulation-control-console/server && npm install && npm run build

.PHONY: build-demo-scenario-orchestrator
build-demo-scenario-orchestrator: ## Build DemoScenarioOrchestrator
	@cd services/control-plane/demo-scenario-orchestrator && python3 -m py_compile main.py

#############################################################################
# Container Image Targets
#############################################################################

.PHONY: image
image: image-data-plane image-processing-plane image-support-plane image-control-plane ## Build all container images

.PHONY: image-data-plane
image-data-plane: ## Build container images for Data Plane services
	@$(MAKE) -C services/data-plane image

.PHONY: image-processing-plane
image-processing-plane: ## Build container images for Processing Plane services
	@$(MAKE) -C services/processing-plane image

.PHONY: image-support-plane
image-support-plane: ## Build container images for Support Plane services
	@$(MAKE) -C services/support-plane image

.PHONY: image-control-plane
image-control-plane: ## Build container images for Control Plane services
	@$(MAKE) -C services/control-plane image

.PHONY: docker-build
docker-build: image ## Build all Docker images (deprecated, use 'image')



#############################################################################
# Helm Targets
#############################################################################

.PHONY: helm-lint
helm-lint: ## Lint all Helm charts
	@echo "Linting Helm charts..."
	@helm lint helm/charts/*/
	@echo "✓ Helm lint complete"

.PHONY: helm-template
helm-template: ## Test Helm template rendering for all charts
	@echo "Testing Helm template rendering..."
	@for chart in helm/charts/*/; do \
		echo "Testing $$chart..."; \
		helm template test $$chart --dry-run > /dev/null; \
	done
	@echo "✓ Helm template test complete"

#############################################################################
# Clean Targets
#############################################################################

.PHONY: clean
clean: clean-go clean-node clean-python ## Clean all build artifacts

.PHONY: clean-go
clean-go: ## Clean Go build artifacts
	@echo "Cleaning Go artifacts..."
	@find services -name "*.test" -delete
	@find services -name "*.out" -delete
	@rm -f services/data-plane/hydrocoptic-datastore/hydrocoptic-datastore
	@rm -f services/data-plane/prefabulated-cache/prefabulated-cache
	@rm -f services/data-plane/sinusoidal-database/sinusoidal-database
	@rm -f services/processing-plane/logarithmic-casing-service/logarithmic-casing-service
	@rm -f services/processing-plane/dingle-arm-api-gateway/dingle-arm-api-gateway
	@rm -f services/support-plane/differential-girdle/differential-girdle
	@rm -f services/control-plane/malaise-injection-engine/malaise-injection-engine
	@echo "✓ Go artifacts cleaned"

.PHONY: clean-node
clean-node: ## Clean Node.js build artifacts
	@echo "Cleaning Node.js artifacts..."
	@find services -name "node_modules" -type d -exec rm -rf {} + 2>/dev/null || true
	@find services -name "dist" -type d -exec rm -rf {} + 2>/dev/null || true
	@echo "✓ Node.js artifacts cleaned"

.PHONY: clean-python
clean-python: ## Clean Python build artifacts
	@echo "Cleaning Python artifacts..."
	@find services -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
	@find services -name "*.pyc" -delete
	@find services -name "*.pyo" -delete
	@find services -name ".pytest_cache" -type d -exec rm -rf {} + 2>/dev/null || true
	@echo "✓ Python artifacts cleaned"

#############################################################################
# Development Targets
#############################################################################

.PHONY: dev-deps
dev-deps: ## Install development dependencies
	@echo "Installing development dependencies..."
	@command -v go >/dev/null 2>&1 || { echo "Go is not installed"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Node.js is not installed"; exit 1; }
	@command -v python3 >/dev/null 2>&1 || { echo "Python 3 is not installed"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "Docker is not installed"; exit 1; }
	@command -v helm >/dev/null 2>&1 || { echo "Helm is not installed"; exit 1; }
	@echo "✓ All required tools are installed"

.PHONY: fmt
fmt: fmt-go fmt-node fmt-python ## Format all code

.PHONY: fmt-go
fmt-go: ## Format Go code
	@echo "Formatting Go code..."
	@find services -name "*.go" -exec gofmt -w {} \;
	@echo "✓ Go code formatted"

.PHONY: fmt-node
fmt-node: ## Format Node.js/TypeScript code
	@echo "Formatting Node.js/TypeScript code..."
	@cd services/processing-plane/panametric-fam-processor && npm run lint --fix 2>/dev/null || true
	@cd services/control-plane/unilateral-phase-detractor && npm run lint --fix 2>/dev/null || true
	@cd services/control-plane/encabulation-control-console && npm run lint --fix 2>/dev/null || true
	@echo "✓ Node.js/TypeScript code formatted"

.PHONY: fmt-python
fmt-python: ## Format Python code
	@echo "Formatting Python code..."
	@command -v black >/dev/null 2>&1 && find services -name "*.py" -exec black {} \; || echo "black not installed, skipping"
	@echo "✓ Python code formatted"

#############################################################################
# CI/CD Targets
#############################################################################

.PHONY: ci
ci: dev-deps build test helm-lint ## Run CI pipeline (build, test, lint)
	@echo "✓ CI pipeline complete"

.PHONY: verify
verify: build test helm-lint helm-template ## Verify all builds, tests, and Helm charts
	@echo "✓ Verification complete"
