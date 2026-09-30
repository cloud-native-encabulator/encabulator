SHELL := /bin/bash
SERVICE := $(notdir $(CURDIR))
REGISTRY ?= ghcr.io/jlbutler/cloud-native-encabulator
VERSION ?= $(shell cat ../../../VERSION 2>/dev/null || echo dev)
KIT := ../../../libs/ts/kit
REPO_ROOT := ../../..
CONTAINER_CLI ?= docker

.PHONY: all deps kit build test test-unit lint format typecheck run image image-multiarch clean

all: build

kit:
	$(MAKE) -C $(KIT) build

deps: kit node_modules

node_modules: package.json package-lock.json
	npm ci
	@touch node_modules

build: deps
	npm run build

typecheck: deps
	npm run typecheck

# Integration tests need a container runtime; they skip cleanly without DOCKER_HOST.
test: deps
	npm test

test-unit: deps
	npm run test:unit

lint: deps
	npm run lint

format: deps
	npm run format

run: build
	npm start

# CONTAINER_CLI=finch works too; Finch exposes a Docker-compatible API.
image:
	$(CONTAINER_CLI) build -f Dockerfile -t $(REGISTRY)/$(SERVICE):$(VERSION) $(REPO_ROOT)

image-multiarch:
	$(CONTAINER_CLI) build --platform linux/amd64,linux/arm64 \
		-f Dockerfile -t $(REGISTRY)/$(SERVICE):$(VERSION) $(REPO_ROOT)

clean:
	rm -rf dist node_modules
