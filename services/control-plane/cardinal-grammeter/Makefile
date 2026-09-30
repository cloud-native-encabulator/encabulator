# cardinal-grammeter
SERVICE := cardinal-grammeter
GO ?= go
export GOTOOLCHAIN ?= auto

# Image coordinates. The build context is the repository root so that the Go kit
# can be copied in.
REGISTRY ?= ghcr.io/jlbutler/cloud-native-encabulator
VERSION ?= $(shell cat ../../../VERSION 2>/dev/null || echo dev)
IMAGE := $(REGISTRY)/$(SERVICE):$(VERSION)
CONTAINER_RUNTIME ?= finch
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: build test test-integration lint fmt tidy run image image-multiarch clean

build:
	$(GO) build -trimpath -ldflags "-s -w" -o bin/$(SERVICE) .

## test runs the unit tests. Every test here runs without a container runtime.
test:
	$(GO) test -short ./...

test-integration:
	$(GO) test -timeout 15m ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	$(GO) vet ./...

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

## run starts the grammeter against a port-forwarded gateway. It starts stopped;
## PUT /v1/traffic to set it going, or set AUTOSTART=true.
run:
	DINGLE_ARM_GATEWAY_URL=$${DINGLE_ARM_GATEWAY_URL:-http://localhost:8081} \
	CUSTOMER_PASSWORD_SEED=$${CUSTOMER_PASSWORD_SEED:-local-development-seed} \
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
