# side-fumbling-detector: inspection.
SERVICE  := side-fumbling-detector
REGISTRY ?= ghcr.io/cloud-native-encabulator/encabulator
VERSION  ?= $(shell cat ../../../VERSION)
REPO_ROOT := ../../..

.PHONY: build test test-unit lint fmt run image clean

build:
	uv sync

test:
	uv run pytest -q

test-unit:
	uv run pytest -q -m "not integration"

lint:
	uv run ruff check .
	uv run ruff format --check .

fmt:
	uv run ruff format .
	uv run ruff check --fix .

# Local run. Needs a NATS with JetStream; the recorder and Valkey are optional.
run:
	NATS_URL=$${NATS_URL:-nats://localhost:4222} uv run python -m detector

image:
	docker build -f Dockerfile -t $(REGISTRY)/$(SERVICE):$(VERSION) $(REPO_ROOT)

clean:
	rm -rf .pytest_cache .ruff_cache dist
