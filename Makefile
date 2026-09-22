-include .env
export

CONFIG_PODMAN_BIN := $(shell awk '/^[[:space:]]*bin:/ {print $$2}' config.yaml 2>/dev/null | tr -d '"' | tr -d "'")
PODMAN_BIN ?= $(if $(CONFIG_PODMAN_BIN),$(CONFIG_PODMAN_BIN),/opt/podman/bin/podman)
PODMAN_SOCKET ?= $(shell $(PODMAN_BIN) machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null)

export DOCKER_HOST ?= unix://$(PODMAN_SOCKET)
export TESTCONTAINERS_RYUK_DISABLED ?= true

# Local Repository Paths & Container Image Tags (configurable via .env)
SORT_MISTAKE_DIR ?= ../sort-mistake
SORT_SERVICE_DIR ?= ../sort-service
SORT_MISTAKE_IMAGE ?= sort-mistake:latest
SORT_SERVICE_IMAGE ?= sort-service:latest

.PHONY: test test-sort-mistake test-intra-node test-report proto-gen clean clean-containers prune-containers prune-images pull-images build-image build-images build-sort-mistake build-sort-service help

help:
	@echo "Multi-Repo Integration Test Platform Commands:"
	@echo "  make pull-images       Pre-pull public backing infrastructure images defined in config.yaml"
	@echo "  make build-image       Build local application images (overridable: SERVICE=...)"
	@echo "                         Examples:"
	@echo "                           make build-image                     (builds all services)"
	@echo "                           make build-image SERVICE=sort-mistake"
	@echo "                           make build-image SERVICE=sort-service"
	@echo "  make build-images      Alias for 'make build-image'"
	@echo "  make build-sort-mistake Shortcut for 'make build-image SERVICE=sort-mistake'"
	@echo "  make build-sort-service Shortcut for 'make build-image SERVICE=sort-service'"
	@echo "  make test              Run integration test suites (overridable: SUITE=..., RUN=..., TIMEOUT=...)"
	@echo "                         Examples:"
	@echo "                           make test"
	@echo "                           make test SUITE=sort-mistake"
	@echo "                           make test SUITE=sort-mistake/intra_node"
	@echo "                           make test RUN=TestSortTaskPipeline"
	@echo "                           make test SUITE=sort-mistake/intra_node TIMEOUT=5m"
	@echo "  make test-sort-mistake Shortcut for 'make test SUITE=sort-mistake'"
	@echo "  make test-intra-node   Shortcut for 'make test SUITE=sort-mistake/intra_node'"
	@echo "  make proto-gen         Compile .proto files in proto/ into Go code (dev-time)"
	@echo "  make test-report       Run tests and generate standard JUnit XML report in reports/"
	@echo "  make prune-containers  Safely prune test containers by label (images untouched)"
	@echo "  make prune-images      Safely prune dangling/intermediate build images"
	@echo "  make clean             Clean reports, prune test containers, and prune dangling images"

# Reusable macro to build a local application container image:
#   $(call build_service_image,service_name,repo_dir,image_tag,prebuild_command)
define build_service_image
	@if [ ! -d "$(2)" ]; then \
		echo "ERROR: Directory '$(2)' not found."; \
		echo "Please point $(shell echo $(1) | tr '[:lower:]-' '[:upper:]_')_DIR in .env to your local $(1) checkout."; \
		exit 1; \
	fi
	@echo "Building $(3) from $(2)..."
	$(if $(4),cd $(2) && $(4))
	$(PODMAN_BIN) build --force-rm -t $(3) -f docker/$(1).Containerfile $(2)
	@$(PODMAN_BIN) image prune -f >/dev/null 2>&1 || true
	@echo "$(3) successfully built."
endef

build-sort-mistake:
	$(call build_service_image,sort-mistake,$(SORT_MISTAKE_DIR),$(SORT_MISTAKE_IMAGE),go mod vendor)

build-sort-service:
	$(call build_service_image,sort-service,$(SORT_SERVICE_DIR),$(SORT_SERVICE_IMAGE),(command -v mise >/dev/null 2>&1 && mise exec -- sbt stage || sbt stage))

# Local service image build parameters (overridable via command-line arguments)
SERVICE ?= all

build-image:
ifeq ($(SERVICE),all)
	@$(MAKE) build-sort-mistake
	@$(MAKE) build-sort-service
	@echo "All local application images built successfully."
else
	@$(MAKE) build-$(subst _,-,$(SERVICE))
endif

build-images: build-image

proto-gen:
	@echo "Compiling protobuf definitions in proto/sortmistake..."
	@PATH="$(HOME)/go/bin:$$PATH" which protoc-gen-go >/dev/null 2>&1 || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	PATH="$(HOME)/go/bin:$$PATH" protoc --proto_path=proto/sortmistake --go_out=proto/sortmistake --go_opt=paths=source_relative proto/sortmistake/sort_node.proto
	@echo "Proto generation complete."


# Test execution parameters (overridable via command-line arguments)
SUITE   ?= ...
RUN     ?=
TIMEOUT ?= 15m

# Normalize SUITE path: accepts '...', 'sort-mistake', 'sort-mistake/intra_node', or './suites/...'
ifeq ($(SUITE),...)
  TEST_TARGET := ./suites/...
else ifneq ($(findstring suites/,$(SUITE)),)
  TEST_TARGET := $(if $(filter %/...,$(SUITE)),./$(subst ./,,$(SUITE)),./$(subst ./,,$(SUITE))/...)
else
  TEST_TARGET := $(if $(filter %/...,$(SUITE)),./suites/$(SUITE),./suites/$(SUITE)/...)
endif

test:
	@echo "Running integration tests [target: $(TEST_TARGET)] [timeout: $(TIMEOUT)]$(if $(RUN), [filter: $(RUN)])..."
	go test -p 1 -v -timeout $(TIMEOUT) $(if $(RUN),-run '$(RUN)') $(TEST_TARGET)

test-sort-mistake:
	@$(MAKE) test SUITE=sort-mistake TIMEOUT=10m

test-intra-node:
	@$(MAKE) test SUITE=sort-mistake/intra_node TIMEOUT=5m

test-report:
	@mkdir -p reports
	@which gotestsum >/dev/null 2>&1 || (echo "Installing gotestsum..." && go install gotest.tools/gotestsum@latest)
	@echo "Running tests with gotestsum [target: $(TEST_TARGET)]..."
	gotestsum --junitfile reports/junit.xml --format pkgname -- -p 1 -timeout $(TIMEOUT) $(if $(RUN),-run '$(RUN)') $(TEST_TARGET)
	@echo "Report generated at reports/junit.xml"

prune-images:
	@echo "Checking for dangling/intermediate build images..."
	@$(PODMAN_BIN) image prune -f

prune-containers:
	@echo "Checking for managed test containers (label=harness.managed=true)..."
	@CONTAINERS=$$($(PODMAN_BIN) ps -aq --filter "label=harness.managed=true" 2>/dev/null); \
	if [ -n "$$CONTAINERS" ]; then \
		echo "Pruning test containers (images will NOT be touched): $$CONTAINERS"; \
		$(PODMAN_BIN) rm -f $$CONTAINERS; \
		echo "Managed containers successfully pruned."; \
	else \
		echo "No dangling managed test containers found."; \
	fi

clean-containers: prune-containers

pull-images:
	@echo "Pre-pulling all required images defined in config.yaml..."
	@PODMAN_BIN="$(PODMAN_BIN)" go run ./cmd/pull-images

clean: prune-containers prune-images
	rm -rf reports/
