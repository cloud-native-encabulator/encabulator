# The Cloud Native Encabulator.
#
# Run `make` (or `make help`) for the list of targets. Everything here is a thin wrapper:
# the per-service Makefiles own how each language builds, scripts/ owns the multi-step
# plumbing, and the service list comes from scripts/services.txt and nowhere else.
#
# Kept to one-line recipes on purpose: macOS still ships GNU Make 3.81, which has no
# .ONESHELL, so anything that needs a loop lives in scripts/.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ----------------------------------------------------------------------------- settings
VERSION           ?= $(shell tr -d '[:space:]' < VERSION)
REGISTRY          ?= ghcr.io/jlbutler/cloud-native-encabulator
# Finch is Docker-API-compatible; `source ./dev-env.sh` exports the DOCKER_HOST it needs,
# which is also what the Testcontainers integration tests pick up.
CONTAINER_RUNTIME ?= finch
# Set PLATFORMS to build multi-arch, e.g. PLATFORMS=linux/amd64,linux/arm64. Empty means
# the host architecture only, which is what a local kind cluster wants.
PLATFORMS         ?=
PUSH              ?= false

NAMESPACE         ?= encabulator
RELEASE           ?= encabulator
CHART             := charts/encabulator
HELM              ?= helm
KUBECTL           ?= kubectl
HELM_TIMEOUT      ?= 10m
# Passed straight through to helm upgrade, e.g. HELM_ARGS="-f my-values.yaml"
HELM_ARGS         ?=
KEDA_ENABLED      ?= false

CONSOLE_PORT      ?= 8080
GRAFANA_PORT      ?= 3000
JAEGER_PORT       ?= 16686
PROMETHEUS_PORT   ?= 9090
OBS_NAMESPACE     ?= encabulator-observability

export CONTAINER_RUNTIME REGISTRY VERSION PLATFORMS PUSH

SERVICE_COUNT     := $(shell ./scripts/services.sh count)

BOLD := \033[1m
DIM  := \033[2m
OFF  := \033[0m

.PHONY: help
help: ## Show this help
	@printf '$(BOLD)The Cloud Native Encabulator$(OFF)  v$(VERSION)\n'
	@printf '  $(DIM)make platform && make deploy && make console$(OFF)\n'
	@awk 'BEGIN { FS = ":.*##" } \
	      /^## / { printf "\n$(BOLD)%s$(OFF)\n", substr($$0, 4); next } \
	      /^[a-zA-Z0-9_.-]+:.*##/ { printf "  \033[36m%-19s$(OFF) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf '\n$(BOLD)Settings$(OFF) (override on the command line)\n'
	@printf '  %-18s %s\n' VERSION '$(VERSION)' REGISTRY '$(REGISTRY)' \
	        CONTAINER_RUNTIME '$(CONTAINER_RUNTIME)' PLATFORMS '$(if $(PLATFORMS),$(PLATFORMS),(host only))' \
	        NAMESPACE '$(NAMESPACE)' RELEASE '$(RELEASE)' KEDA_ENABLED '$(KEDA_ENABLED)'
	@printf '\n  $(SERVICE_COUNT) services in scripts/services.txt; `make services` lists them.\n\n'

## Code
.PHONY: build test lint fmt clean
build: ## Build every service and library that has a Makefile
	@./scripts/iterate.sh build

test: ## Run every service's and library's tests
	@./scripts/iterate.sh test

lint: ## Lint every service and library
	@./scripts/iterate.sh lint

fmt: ## Format every service and library that implements `fmt`
	@./scripts/iterate.sh fmt

clean: ## Remove build output
	@./scripts/iterate.sh clean
	@rm -rf bin dist

## Charts
.PHONY: lint-charts template
lint-charts: ## helm lint + helm template + kubeconform over several value combinations
	@./scripts/helm-validate.sh

template: ## Render charts/encabulator to stdout
	@KUBECONFIG=/dev/null $(HELM) template $(RELEASE) $(CHART) --namespace $(NAMESPACE) \
	  --set keda.enabled=$(KEDA_ENABLED) $(HELM_ARGS)

## Images
.PHONY: images push
images: ## Build every service image with the repository root as the build context
	@./scripts/images.sh

push: ## Build and push every service image
	@PUSH=true ./scripts/images.sh

## Cluster
.PHONY: platform platform-status platform-uninstall deploy undeploy status
platform: ## Install the observability platform and KEDA from pinned upstream charts
	@./scripts/platform.sh install

platform-status: ## Show what the platform has installed
	@./scripts/platform.sh status

platform-uninstall: ## Remove the platform and KEDA
	@./scripts/platform.sh uninstall

deploy: ## Install charts/encabulator into the target namespace
	@printf '$(BOLD)==> deploying %s v%s into %s$(OFF)\n' '$(RELEASE)' '$(VERSION)' '$(NAMESPACE)'
	$(HELM) upgrade --install $(RELEASE) $(CHART) \
	  --namespace $(NAMESPACE) --create-namespace \
	  --set global.registry=$(REGISTRY) \
	  --set global.imageTag=$(VERSION) \
	  --set keda.enabled=$(KEDA_ENABLED) \
	  --wait --timeout $(HELM_TIMEOUT) $(HELM_ARGS)

undeploy: ## Uninstall charts/encabulator (PersistentVolumeClaims are left behind)
	$(HELM) uninstall $(RELEASE) --namespace $(NAMESPACE)
	@printf '$(DIM)PVCs survive on purpose. Remove them with:$(OFF)\n'
	@printf '  kubectl -n $(NAMESPACE) delete pvc -l app.kubernetes.io/part-of=encabulator\n'

status: ## Pods, deployments, autoscalers and stores in the target namespace
	@printf '$(BOLD)==> workloads$(OFF)\n'
	-@$(KUBECTL) -n $(NAMESPACE) get deploy,sts -L encabulator.io/plane
	@printf '\n$(BOLD)==> pods$(OFF)\n'
	-@$(KUBECTL) -n $(NAMESPACE) get pods -o wide
	@printf '\n$(BOLD)==> autoscaling$(OFF)\n'
	-@$(KUBECTL) -n $(NAMESPACE) get hpa
	-@$(KUBECTL) -n $(NAMESPACE) get scaledobjects
	@printf '\n$(BOLD)==> storage$(OFF)\n'
	-@$(KUBECTL) -n $(NAMESPACE) get pvc

## Access
.PHONY: console grafana jaeger prometheus credentials
console: ## Port-forward the console (CONSOLE_PORT, default 8080)
	@printf '$(BOLD)==> http://localhost:$(CONSOLE_PORT)$(OFF)  (ctrl-c to stop)\n'
	$(KUBECTL) -n $(NAMESPACE) port-forward svc/encabulation-control-console $(CONSOLE_PORT):8080

grafana: ## Port-forward Grafana (GRAFANA_PORT, default 3000)
	@printf '$(BOLD)==> http://localhost:$(GRAFANA_PORT)$(OFF)\n'
	$(KUBECTL) -n $(OBS_NAMESPACE) port-forward svc/prometheus-stack-grafana $(GRAFANA_PORT):80

jaeger: ## Port-forward Jaeger (JAEGER_PORT, default 16686)
	@printf '$(BOLD)==> http://localhost:$(JAEGER_PORT)$(OFF)\n'
	$(KUBECTL) -n $(OBS_NAMESPACE) port-forward svc/jaeger $(JAEGER_PORT):16686

prometheus: ## Port-forward Prometheus (PROMETHEUS_PORT, default 9090)
	@printf '$(BOLD)==> http://localhost:$(PROMETHEUS_PORT)$(OFF)\n'
	$(KUBECTL) -n $(OBS_NAMESPACE) port-forward svc/prometheus-operated $(PROMETHEUS_PORT):9090

credentials: ## Print the bootstrap admin/operator and Grafana passwords
	@printf '$(BOLD)%-22s$(OFF) ' 'encabulator admin'
	@$(KUBECTL) -n $(NAMESPACE) get secret $(RELEASE)-app -o jsonpath='{.data.bootstrap-admin-password}' | base64 -d; echo
	@printf '$(BOLD)%-22s$(OFF) ' 'encabulator operator'
	@$(KUBECTL) -n $(NAMESPACE) get secret $(RELEASE)-app -o jsonpath='{.data.bootstrap-operator-password}' | base64 -d; echo
	@printf '$(BOLD)%-22s$(OFF) ' 'grafana admin'
	-@$(KUBECTL) -n $(OBS_NAMESPACE) get secret prometheus-stack-grafana -o jsonpath='{.data.admin-password}' | base64 -d; echo

## Local development
.PHONY: dev-up dev-down dev-logs kind-up kind-down kind-load
dev-up: ## Start Postgres, Valkey, NATS, InfluxDB and Jaeger for running services natively
	$(CONTAINER_RUNTIME) compose -f dev/compose.yaml up -d
	@printf '\n$(BOLD)==> stores up.$(OFF) Point a service at them with:\n'
	@printf '  set -a && source dev/env && set +a\n'
	@printf '  make -C services/<plane>-plane/<service> run\n'

dev-down: ## Stop the development stores (CLEAN=true also deletes their volumes)
	$(CONTAINER_RUNTIME) compose -f dev/compose.yaml down $(if $(filter true,$(CLEAN)),--volumes,)

dev-logs: ## Tail the development stores' logs
	$(CONTAINER_RUNTIME) compose -f dev/compose.yaml logs -f

kind-up: ## Create a local kind cluster (prints install instructions if kind is missing)
	@./scripts/kind.sh up

kind-down: ## Delete the local kind cluster
	@./scripts/kind.sh down

kind-load: ## Load locally built images into the kind cluster
	@./scripts/kind.sh load

## Meta
.PHONY: services version
services: ## List the fifteen services with their directories, languages and planes
	@printf '%-30s %-52s %-9s %s\n' NAME DIRECTORY LANGUAGE PLANE
	@awk -F'\t' '!/^[[:space:]]*#/ && NF>=4 { printf "%-30s %-52s %-9s %s\n", $$1, $$2, $$3, $$4 }' scripts/services.txt

version: ## Print the version every image is tagged with
	@echo $(VERSION)
