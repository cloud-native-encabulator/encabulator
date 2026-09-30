# The Go service kit. There is nothing to build into a binary here; `build`
# compiles the packages so that a broken kit fails fast.
GO ?= go
export GOTOOLCHAIN ?= auto

.PHONY: build test test-integration lint fmt tidy

build:
	$(GO) build ./...

## test runs the unit tests. Integration tests skip themselves without a runtime.
test:
	$(GO) test -short ./...

## test-integration runs everything, including the Testcontainers tests.
## Source ../../../dev-env.sh first so DOCKER_HOST points at the container runtime.
test-integration:
	$(GO) test -timeout 15m ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	$(GO) vet ./...

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy
