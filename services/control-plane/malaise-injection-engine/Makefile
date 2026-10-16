# malaise-injection-engine
SERVICE := malaise-injection-engine
GO ?= go
export GOTOOLCHAIN ?= auto

# Image coordinates. The build context is the repository root so that the Go kit
# can be copied in.
REGISTRY ?= ghcr.io/cloud-native-encabulator/encabulator
VERSION ?= $(shell cat ../../../VERSION 2>/dev/null || echo dev)
IMAGE := $(REGISTRY)/$(SERVICE):$(VERSION)
CONTAINER_RUNTIME ?= finch
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: build test test-integration lint fmt tidy run image image-multiarch clean

build:
	$(GO) build -trimpath -ldflags "-s -w" -o bin/$(SERVICE) .

## test runs the unit tests: miniredis and the client-go fake clientset, no
## container runtime needed.
test:
	$(GO) test -short ./...

## test-integration adds the Testcontainers Valkey test. Source
## ../../../dev-env.sh first so DOCKER_HOST points at the container runtime.
test-integration:
	$(GO) test -timeout 15m ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	$(GO) vet ./...

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

## run starts the engine against a local Valkey. Outside a cluster the
## Kubernetes-level kinds are unavailable and the app-level kinds still work.
run:
	VALKEY_URL=$${VALKEY_URL:-redis://localhost:6379/0} \
	POD_NAMESPACE=$${POD_NAMESPACE:-encabulator} \
	PORT=$${PORT:-8080} LOG_LEVEL=$${LOG_LEVEL:-debug} \
	$(GO) run .

image:
	cd ../../.. && $(CONTAINER_RUNTIME) build -f services/control-plane/$(SERVICE)/Dockerfile \
		-t $(IMAGE) .

image-multiarch:
	cd ../../.. && $(CONTAINER_RUNTIME) build --platform $(PLATFORMS) \
		-f services/control-plane/$(SERVICE)/Dockerfile -t $(IMAGE) .

clean:
	rm -rf bin
