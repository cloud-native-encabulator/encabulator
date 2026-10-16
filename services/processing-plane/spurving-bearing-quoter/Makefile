# spurving-bearing-quoter
#
# The root Makefile iterates over every service's build, test, lint, and run targets
# (design appendix B).

SERVICE      := spurving-bearing-quoter
VERSION      ?= $(shell cat ../../../VERSION 2>/dev/null || echo 0.0.0)
REGISTRY     ?= ghcr.io/cloud-native-encabulator/encabulator
IMAGE        ?= $(REGISTRY)/$(SERVICE):$(VERSION)
PLATFORMS    ?= linux/amd64,linux/arm64
REPO_ROOT    := $(abspath ../../..)
CONTAINER    ?= $(shell command -v finch 2>/dev/null || command -v docker 2>/dev/null || echo docker)

# Local runs default to no cache, no flags, and no faults: the service works standalone.
RUN_ENV ?= PORT=8080 LOG_LEVEL=debug

.PHONY: all build test test-unit test-integration lint fmt fmt-check clippy run image image-load clean help

all: lint test build

## build: compile the release binary
build:
	cargo build --release --locked

## test: every test (the Valkey integration test skips without a container runtime)
test:
	cargo test --locked

## test-unit: unit tests and handler tests only, no container runtime involved
test-unit:
	cargo test --locked --lib --test handlers

## test-integration: the Valkey tests against a real valkey/valkey:9 container
test-integration:
	@if [ -z "$$DOCKER_HOST" ]; then \
	  echo "DOCKER_HOST is unset. Run: source $(REPO_ROOT)/dev-env.sh"; \
	  exit 1; \
	fi
	cargo test --locked --test valkey_cache -- --test-threads=3

## lint: clippy with warnings as errors, plus a formatting check
lint: clippy fmt-check

clippy:
	cargo clippy --locked --all-targets -- -D warnings

fmt:
	cargo fmt

fmt-check:
	cargo fmt --check

## run: run the service locally on :8080
run:
	$(RUN_ENV) cargo run --locked

## image: build the multi-arch image (build context is the repository root)
image:
	$(CONTAINER) buildx build \
	  --platform $(PLATFORMS) \
	  --build-arg VERSION=$(VERSION) \
	  -f $(REPO_ROOT)/services/processing-plane/$(SERVICE)/Dockerfile \
	  -t $(IMAGE) \
	  $(REPO_ROOT)

## image-load: build for this machine's architecture only and load it into the local runtime
image-load:
	$(CONTAINER) build \
	  --build-arg VERSION=$(VERSION) \
	  -f $(REPO_ROOT)/services/processing-plane/$(SERVICE)/Dockerfile \
	  -t $(IMAGE) \
	  $(REPO_ROOT)

clean:
	cargo clean

help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | awk -F': ' '{printf "  \033[1m%-18s\033[0m %s\n", $$1, $$2}'
