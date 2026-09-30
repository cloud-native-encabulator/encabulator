# demo-scenario-orchestrator: rehearses the demo so the presenter does not have to.
SERVICE  := demo-scenario-orchestrator
REGISTRY ?= ghcr.io/jlbutler/cloud-native-encabulator
VERSION  ?= $(shell cat ../../../VERSION)
REPO_ROOT := ../../..

.PHONY: build test test-unit lint fmt run image scenarios clean

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

# Local run. Every upstream is optional: unset URLs make those steps logged no-ops.
run:
	SCENARIO_DIR=$${SCENARIO_DIR:-./scenarios} uv run python -m orchestrator

# Validate every built-in scenario without starting a server.
scenarios:
	uv run python -c "from orchestrator.loader import load_builtin; \
	  [print(f'{s.id:26s} {len(s.steps):3d} steps  {s.title}') for s in load_builtin()]"

image:
	docker build -f Dockerfile -t $(REGISTRY)/$(SERVICE):$(VERSION) $(REPO_ROOT)

clean:
	rm -rf .pytest_cache .ruff_cache dist
