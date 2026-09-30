SHELL := /bin/bash

.PHONY: all deps build test test-unit lint format typecheck clean

all: build

deps: node_modules

node_modules: package.json package-lock.json
	npm ci
	@touch node_modules

build: deps
	npm run build
	@touch READY

typecheck: deps
	npm run typecheck

# Integration tests need a container runtime; they skip cleanly without DOCKER_HOST.
test: deps
	npm test

# Unit tests only, no containers.
test-unit: deps
	npx vitest run --exclude 'test/**'

lint: deps
	npm run lint

format: deps
	npm run format

clean:
	rm -rf dist READY
