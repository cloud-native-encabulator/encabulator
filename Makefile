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

# ----------------------------------------------------------------------------- .env
# The AWS path (`make hub`) reads the few values that are specific to an account from a
# gitignored .env (start from .env.example). Precedence: the make command line, then the shell
# environment, then .env, then the defaults below. Without a .env, or with a key left empty,
# the defaults apply, so every target works without one.
DOTENV_FILE ?= .env
DOTENV_KEYS := IDC_INSTANCE_ARN IDC_REGION ARGOCD_ADMIN_GROUP_ID \
               GIT_REPO_URL GIT_REVISION GIT_TOKEN_FILE REGISTRY CONTAINER_RUNTIME

# A key that is not set on the command line or in the environment takes its .env value, when
# that is not empty, and is exported to the recipes. scripts/dotenv.sh does the parsing
# (quotes, `=` in values, comments) and prints only the one value asked for.
define dotenv_load
_dotenv_value := $$(shell ./scripts/dotenv.sh get '$$(DOTENV_FILE)' $(1))
ifeq ($$(origin $(1)),undefined)
ifneq ($$(_dotenv_value),)
$(1) := $$(_dotenv_value)
export $(1)
endif
endif
endef
ifneq ($(wildcard $(DOTENV_FILE)),)
$(foreach key,$(DOTENV_KEYS),$(eval $(call dotenv_load,$(key))))
endif

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

# Brownfield to GitOps. TERRAFORM can be a wrapper, e.g. TERRAFORM="mise exec terraform@1.14.9 -- terraform".
TERRAFORM         ?= terraform
TF_DIR            := terraform/brownfield
KIND_KUBECONFIG   ?=
REGISTER_ARGS     ?=
TAG_CHECK_ARGS    ?=
DECK_PORT         ?= 8000

export CONTAINER_RUNTIME REGISTRY VERSION PLATFORMS PUSH

# The hub: names and region are fixed by terraform/brownfield and the GitOps files
# (encabulator-usw2 in us-west-2), so they are not settings.
HUB_REGION        := us-west-2
HUB_CLUSTER       := encabulator-usw2
# Seconds `make wait-capabilities` waits for the Argo CD, ACK and kro capabilities.
CAPABILITY_TIMEOUT ?= 1800
# `make hub` asks before it creates anything; CONFIRM=yes answers for you.
CONFIRM           ?=
# Extra arguments for terraform apply; `make hub` passes -auto-approve after you confirm.
TF_APPLY_ARGS     ?=
HUB_ENV = HUB_REGION=$(HUB_REGION) HUB_CLUSTER=$(HUB_CLUSTER) DOTENV_FILE='$(DOTENV_FILE)' \
          TERRAFORM='$(TERRAFORM)' TF_DIR=$(TF_DIR) KUBECTL='$(KUBECTL)' NAMESPACE=$(NAMESPACE)

# Terraform gets the Identity Center values as TF_VAR_ variables on the plan and apply
# command lines, so a terraform/brownfield/*.auto.tfvars file, when there is one, still wins.
sh_quote = '$(subst ','\'',$(1))'
TF_ENV = $(strip $(if $(IDC_INSTANCE_ARN),TF_VAR_idc_instance_arn=$(call sh_quote,$(IDC_INSTANCE_ARN))) \
                 $(if $(IDC_REGION),TF_VAR_idc_region=$(call sh_quote,$(IDC_REGION))) \
                 $(if $(ARGOCD_ADMIN_GROUP_ID),TF_VAR_argocd_admin_group_id=$(call sh_quote,$(ARGOCD_ADMIN_GROUP_ID))))
TF_RUN = $(if $(TF_ENV),$(TF_ENV) )$(TERRAFORM)

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

## On AWS, from one .env (cp .env.example .env)
.PHONY: hub prereqs kubeconfig wait-capabilities mirror-images
hub: ## Build the hub from .env: checks, Terraform, kubeconfig, images, platform, app, GitOps (asks first; billable, about 25 minutes)
	+@$(call hub_step,prereqs)
	@CONFIRM=$(call sh_quote,$(CONFIRM)) ./scripts/confirm.sh
	+@$(call hub_step,brownfield-init)
	+@$(call hub_step,brownfield-apply,TF_APPLY_ARGS=-auto-approve)
	+@$(call hub_step,kubeconfig)
	+@$(call hub_step,wait-capabilities)
	+@$(call hub_step,mirror-images)
	+@$(call hub_step,platform,,$(hub_reread))
	+@$(call hub_step,brownfield-deploy,,$(hub_reread))
	+@$(call hub_step,gitops-register,AWS_REGION=$(HUB_REGION),$(hub_reread))
	+@$(call hub_step,gitops-repo-access,AWS_REGION=$(HUB_REGION),$(hub_reread))
	+@$(call hub_step,gitops-bootstrap,AWS_REGION=$(HUB_REGION),$(hub_reread))
	@printf '\n$(BOLD)==> The hub is up.$(OFF) Next:\n'
	@printf '  make credentials          the console and Grafana passwords\n'
	@printf '  make console              open the console on http://localhost:$(CONSOLE_PORT)\n'
	@printf '  make add-cell CELL=use2   optional: the hub builds a cell in us-east-2 (billable)\n'
	@printf '  make tag-check            read-only: every AWS resource carries its tags (wait 10 minutes)\n'

# One step of `make hub`, run as its own make (`+` runs it under `make -n` too, which then
# only prints). $(1) is the target, $(2) extra make arguments, $(3) a command prefix. On a
# failure it names the step and how to go on.
hub_step = $(3)$(MAKE) --no-print-directory $(1)$(if $(2), $(2)) || { \
  printf '\n$(BOLD)make hub stopped at step "%s".$(OFF)\n' '$(1)'; \
  printf 'Fix what is reported above, then run make hub again: every step is safe to re-run.\n'; \
  printf 'To run just this step: make %s\n' '$(1)'; exit 1; }
# The steps after mirror-images read REGISTRY from .env again (mirror-images may have just
# written it there), unless REGISTRY came from the shell environment.
hub_reread = $(if $(findstring environment,$(origin REGISTRY)),,env -u REGISTRY )

prereqs: ## Read-only: check the tools, AWS credentials, .env and Identity Center; lists every problem with its fix
	@$(HUB_ENV) ./scripts/prereqs.sh

kubeconfig: ## Point kubectl at the hub (context encabulator-usw2, namespace encabulator) and check it
	@$(HUB_ENV) ./scripts/kubeconfig.sh

wait-capabilities: ## Wait until the hub's Argo CD, ACK and kro capabilities are ACTIVE (CAPABILITY_TIMEOUT seconds)
	@$(HUB_ENV) CAPABILITY_TIMEOUT=$(CAPABILITY_TIMEOUT) ./scripts/wait-capabilities.sh

mirror-images: ## Copy the VERSION images into Amazon ECR when GHCR lacks them, and record REGISTRY in .env
	@$(HUB_ENV) ./scripts/mirror-images.sh

## Brownfield to GitOps (Terraform, ACK, Argo CD, kro)
.PHONY: brownfield-init brownfield-test brownfield-plan brownfield-apply brownfield-deploy gitops-register gitops-bootstrap gitops-validate tag-check talk
brownfield-init: ## terraform init for terraform/brownfield, the hub's starting point
	$(TERRAFORM) -chdir=$(TF_DIR) init

brownfield-test: ## Offline contract checks for the Terraform stack (no AWS calls)
	$(TERRAFORM) -chdir=$(TF_DIR) init -backend=false
	$(TERRAFORM) -chdir=$(TF_DIR) test

brownfield-plan: ## terraform plan (needs AWS credentials and terraform/brownfield/encabulator.auto.tfvars)
	$(TF_RUN) -chdir=$(TF_DIR) plan

brownfield-apply: ## terraform apply: VPC, EKS Auto Mode, RDS, ElastiCache and the Argo CD, ACK and kro capabilities (about 25 minutes, costs money)
	$(TF_RUN) -chdir=$(TF_DIR) apply$(if $(TF_APPLY_ARGS), $(TF_APPLY_ARGS))

brownfield-deploy: ## Deploy the app onto the hub's RDS and ElastiCache (run `make platform` first)
	@REGISTRY=$(call sh_quote,$(REGISTRY)) ./$(TF_DIR)/deploy-app.sh

gitops-register: ## Register the hub with its own Argo CD (REGISTER_ARGS=--dry-run shows the Secret first)
	@./gitops/hub/bootstrap/register-hub.sh $(REGISTER_ARGS)

gitops-bootstrap: ## Apply the Argo CD projects, the hub ApplicationSet and the hub Applications
	$(KUBECTL) apply -f gitops/hub/bootstrap/

gitops-validate: ## Validate gitops/app on a local kind cluster, no AWS (KIND_KUBECONFIG=path)
	@test -n "$(KIND_KUBECONFIG)" || { echo "set KIND_KUBECONFIG=<kubeconfig of a kind cluster>"; exit 2; }
	@./gitops/app/hack/validate-kind.sh --kubeconfig "$(KIND_KUBECONFIG)"

tag-check: ## Read-only: fail on any AWS resource of the stacks that lacks the auto-delete=no tag
	@python3 scripts/tag-check.py $(TAG_CHECK_ARGS)

talk: ## Serve the OPN315 talk slides on http://localhost:DECK_PORT (press P for the presenter view)
	@printf '$(BOLD)==> http://localhost:$(DECK_PORT)$(OFF)  (ctrl-c to stop)\n'
	@cd docs/opn315 && python3 -m http.server $(DECK_PORT) --bind 127.0.0.1

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
