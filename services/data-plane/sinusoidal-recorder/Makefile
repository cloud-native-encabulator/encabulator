# sinusoidal-recorder
SERVICE   := sinusoidal-recorder
REPO_ROOT := ../../..
REGISTRY  ?= ghcr.io/jlbutler/cloud-native-encabulator
VERSION   ?= $(shell cat $(REPO_ROOT)/VERSION 2>/dev/null || echo dev)
IMAGE     := $(REGISTRY)/$(SERVICE):$(VERSION)
PLATFORMS ?= linux/amd64,linux/arm64
GO        ?= go
export GOTOOLCHAIN ?= auto

.PHONY: build test test-integration lint fmt run image image-multiarch tidy clean

## build compiles the server binary into bin/.
build:
	$(GO) build -trimpath -ldflags="-X main.version=$(VERSION)" -o bin/$(SERVICE) ./cmd/server

## test runs the unit tests. Integration tests skip themselves without a runtime.
test:
	$(GO) test -short ./...

## test-integration also runs the Testcontainers tests.
## Source $(REPO_ROOT)/dev-env.sh first so DOCKER_HOST points at the container runtime.
test-integration:
	$(GO) test -timeout 15m ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	$(GO) vet ./...

fmt:
	gofmt -w .

## run starts the service locally. Export the variables in README.md first.
run:
	$(GO) run ./cmd/server

image:
	docker build --build-arg VERSION=$(VERSION) -f Dockerfile -t $(IMAGE) $(REPO_ROOT)

image-multiarch:
	docker buildx build --platform $(PLATFORMS) --build-arg VERSION=$(VERSION) \
		-f Dockerfile -t $(IMAGE) $(REPO_ROOT)

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin
