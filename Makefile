IMAGE ?= mcd
HEADLESS_DIR ?= apps/headless
OUTPUT_DIR ?= $(HEADLESS_DIR)/output
DOCKERFILE ?= build/package/Dockerfile

ID ?=
URL ?=
TIMEOUT ?= 60
QUIET ?= 0

.PHONY: help headless-build headless-run

help:
	@echo "Targets:"
	@echo "  make headless-build                    Build the headless Docker image"
	@echo "  make headless-run ID=<collectionId>    Run with a collection ID"
	@echo "  make headless-run URL=<collectionUrl>  Run with a full collection URL"
	@echo ""
	@echo "Variables:"
	@echo "  IMAGE=$(IMAGE)"
	@echo "  DOCKERFILE=$(DOCKERFILE)"
	@echo "  OUTPUT_DIR=$(OUTPUT_DIR)"
	@echo "  TIMEOUT=$(TIMEOUT)"
	@echo "  QUIET=$(QUIET)"

headless-build:
	docker build -t $(IMAGE) -f $(DOCKERFILE) \
		--build-arg VERSION="$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
		--build-arg REVISION="$$(git rev-parse HEAD 2>/dev/null || echo unknown)" \
		--build-arg BUILD_RFC3339="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		.

headless-run:
	@mkdir -p "$(OUTPUT_DIR)"
	@if [ -z "$(strip $(ID))" ] && [ -z "$(strip $(URL))" ]; then \
		echo "Error: provide ID=<collectionId> or URL=<collectionUrl>"; \
		exit 1; \
	fi
	docker run --rm \
		-v "$(PWD)/$(OUTPUT_DIR):/app/output" \
		-e MCD_ID="$(ID)" \
		-e MCD_URL="$(URL)" \
		-e MCD_TIMEOUT="$(TIMEOUT)" \
		-e MCD_QUIET="$(QUIET)" \
		$(IMAGE)
