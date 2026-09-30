# unilateral-phase-detractor: identity for the Encabulator Works.
SERVICE  := unilateral-phase-detractor
REGISTRY ?= ghcr.io/jlbutler/cloud-native-encabulator
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

# Local run. Needs a Postgres; everything else degrades to defaults.
run:
	DATABASE_URL?=$${DATABASE_URL:-postgresql://detractor:detractor@localhost:5432/detractor} \
	BOOTSTRAP_ADMIN_PASSWORD=$${BOOTSTRAP_ADMIN_PASSWORD:-local-admin-password} \
	BOOTSTRAP_OPERATOR_PASSWORD=$${BOOTSTRAP_OPERATOR_PASSWORD:-local-operator-password} \
	uv run python -m detractor

# The build context is the repository root so the kit can be copied in.
image:
	docker build -f Dockerfile -t $(REGISTRY)/$(SERVICE):$(VERSION) $(REPO_ROOT)

clean:
	rm -rf .pytest_cache .ruff_cache dist
