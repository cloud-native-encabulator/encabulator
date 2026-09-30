# encabulation-control-console
#
# The root Makefile iterates over every service's build / test / lint / run.
# Every target here works from this directory.

SERVICE      := encabulation-control-console
REGISTRY     ?= ghcr.io/jlbutler/cloud-native-encabulator
VERSION      ?= $(shell node -p "require('./package.json').version")
IMAGE        ?= $(REGISTRY)/$(SERVICE):$(VERSION)
PLATFORMS    ?= linux/amd64,linux/arm64

# Build context is the repository root so the shared kit can be copied in.
REPO_ROOT    := $(abspath ../../..)
KIT_DIR      := $(REPO_ROOT)/libs/ts/kit
DOCKERFILE   := services/control-plane/$(SERVICE)/Dockerfile

# Finch and Docker share a CLI surface; use whichever is installed.
CONTAINER    ?= $(shell command -v docker 2>/dev/null || command -v finch 2>/dev/null)

.DEFAULT_GOAL := build
.PHONY: build test test-watch lint lint-fix typecheck run dev image image-multiarch \
        install clean help

## install: resolve dependencies (builds the shared kit first, as a file: dep must exist)
install: $(KIT_DIR)/dist
	npm ci

$(KIT_DIR)/dist:
	$(MAKE) -C $(KIT_DIR) build

## build: typecheck and compile the SPA (dist/web) and the BFF (dist/bff)
build: node_modules
	npm run build

## test: unit tests for the BFF and the SPA. No containers, no cluster, no network.
test: node_modules
	npm test

## test-watch: the same suites, re-run on change
test-watch: node_modules
	npm run test:watch

## lint: ESLint flat config, warnings are failures
lint: node_modules
	npm run lint

## lint-fix: apply what ESLint can fix
lint-fix: node_modules
	npm run lint:fix

## typecheck: all four TypeScript projects (SPA, BFF, tests, dev harness)
typecheck: node_modules
	npm run typecheck

## run: serve the built console on PORT (8080). Needs SESSION_SECRET and the upstream URLs.
run: build
	@test -n "$$SESSION_SECRET" || { \
	  echo "SESSION_SECRET is required. For a throwaway local run:"; \
	  echo "  SESSION_SECRET=local-development-secret-1234 make run"; \
	  exit 1; }
	npm start

## dev: fixtures + BFF + Vite together, with hot reload. http://127.0.0.1:5173
dev: node_modules
	npm run dev

## image: build the container image for this architecture
image:
	@test -n "$(CONTAINER)" || { echo "no docker or finch on PATH"; exit 1; }
	cd $(REPO_ROOT) && $(CONTAINER) build -f $(DOCKERFILE) -t $(IMAGE) .

## image-multiarch: build and push linux/amd64 + linux/arm64
image-multiarch:
	@test -n "$(CONTAINER)" || { echo "no docker or finch on PATH"; exit 1; }
	cd $(REPO_ROOT) && $(CONTAINER) buildx build --platform $(PLATFORMS) \
	  -f $(DOCKERFILE) -t $(IMAGE) --push .

node_modules: package.json package-lock.json
	$(MAKE) install
	@touch node_modules

## clean: remove build output and installed dependencies
clean:
	rm -rf dist node_modules coverage .vite

## help: list these targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
