SHELL := /bin/sh

DOCKERFILE ?= build/package/Dockerfile
APPLICATION ?= mcd
BUILD_RFC3339 ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
DESCRIPTION ?= "moxfield collection downloader"
PACKAGE ?= $(shell git config --get remote.origin.url 2>/dev/null | sed -e 's/^git@github.com://' -e 's/^https:\/\/github.com\///' -e 's/\.git$$//')
REVISION ?= $(shell git rev-parse HEAD 2>/dev/null || echo local)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo local)

DOCKER_ORGANIZATION ?= $(word 1,$(subst /, ,$(PACKAGE)))
DOCKER_REPOSITORY ?= $(word 2,$(subst /, ,$(PACKAGE)))
IMAGE ?= $(if $(and $(DOCKER_ORGANIZATION),$(DOCKER_REPOSITORY)),$(DOCKER_ORGANIZATION)/$(DOCKER_REPOSITORY):$(VERSION),$(APPLICATION):$(VERSION))

ID ?=
URL ?=
TIMEOUT ?= 10
OUTPUT ?= ./cache/collection.json
FORCE ?= 0
BUILD_DIR ?= dist

.PHONY: help build test test docker-build run

help:
	@echo "Targets:"
	@echo "  make build          Build local Go binary"
	@echo "  make test           Run unit tests"
	@echo "  make docker-build   Build Docker image"
	@echo "  make run            Run in Docker (ID or URL required)"

build:
	@mkdir -p "$(BUILD_DIR)"
	go build -ldflags="-s -w -X main.version=$(VERSION) -X main.buildRFC3339=$(BUILD_RFC3339) -X main.revision=$(REVISION)" -o "$(BUILD_DIR)/$(APPLICATION)" ./cmd/$(APPLICATION)

test:
	go test ./...

docker-build:
	docker buildx build \
		--file $(DOCKERFILE) \
		--build-arg APPLICATION=$(APPLICATION) \
		--build-arg BUILD_RFC3339=$(BUILD_RFC3339) \
		--build-arg DESCRIPTION=$(DESCRIPTION) \
		--build-arg PACKAGE=$(PACKAGE) \
		--build-arg REVISION=$(REVISION) \
		--build-arg VERSION=$(VERSION) \
		--tag $(IMAGE) \
		.
	docker tag $(IMAGE) $(APPLICATION):dev

run:
	@if [ -z "$(strip $(ID))" ] && [ -z "$(strip $(URL))" ]; then \
		echo "Error: provide ID=<collectionId> or URL=<collectionUrl>"; \
		exit 1; \
	fi
	@mkdir -p "$$(dirname $(OUTPUT))"
	docker run --rm \
		-v "$(PWD)/$$(dirname $(OUTPUT)):/data" \
		-e MCD_ID="$(ID)" \
		-e MCD_URL="$(URL)" \
		-e MCD_TIMEOUT="$(TIMEOUT)" \
		-e MCD_FORCE="$(FORCE)" \
		-e MCD_OUTPUT="/data/$$(basename $(OUTPUT))" \
		$(APPLICATION):dev
