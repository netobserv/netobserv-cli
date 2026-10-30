# VERSION defines the project version.
# Update this value when you upgrade the version of your project.
# To re-generate a tar.gz for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make tar-commands VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= main

# Go architecture and targets images to build
GOARCH ?= amd64
MULTIARCH_TARGETS ?= amd64
KREW_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

# In CI, to be replaced by `netobserv`
IMAGE_ORG ?= $(USER)

# Build output
NAME := network-observability-cli
DIST_DIR ?= build
FILES_OUTPUT_DIR ?= output
RELEASE_ASSETS_DIR ?= release-assets
COLLECTOR_OUTPUT := $(DIST_DIR)/$(NAME)
UNIT_PACKAGES := $(shell go list -mod=vendor ./... | grep -v '/e2e/integration-tests$$')

# Available commands for development with args
COMMANDS = flows packets cleanup
COMMAND_ARGS ?=

# Get either oc (favorite) or kubectl paths
K8S_CLI_BIN_PATH = $(shell which oc 2>/dev/null || which kubectl)
K8S_CLI_BIN ?= $(shell basename ${K8S_CLI_BIN_PATH})

# Image registry such as quay or docker
IMAGE_REGISTRY ?= quay.io

# IMAGE_TAG_BASE defines the namespace and part of the image name for remote images.
IMAGE_TAG_BASE ?= $(IMAGE_REGISTRY)/$(IMAGE_ORG)/$(NAME)

# Image URL to use all building/pushing image targets
IMAGE ?= $(IMAGE_TAG_BASE):$(VERSION)
PULL_POLICY ?=Always

# Agent image URL to deploy
AGENT_IMAGE ?= $(IMAGE_REGISTRY)/netobserv/netobserv-ebpf-agent:main

# Image building tool (docker / podman) - docker is preferred in CI
OCI_BIN_PATH := $(shell which docker 2>/dev/null || which podman)
OCI_BIN ?= $(shell basename ${OCI_BIN_PATH})
OCI_BUILD_OPTS ?=
KREW_PLUGIN ?=false

ifeq ("$(OCI_BIN)","docker")
# https://stackoverflow.com/questions/75521775/buildx-docker-image-claims-to-be-a-manifest-list
EXTRA_BUILD_FLAGS ?= --provenance=false
endif

ifneq ($(CLEAN_BUILD),)
	BUILD_DATE := $(shell date +%Y-%m-%d\ %H:%M)
	BUILD_SHA := $(shell git rev-parse --short=8 HEAD)
	LDFLAGS ?= -X 'main.buildVersion=${VERSION}-${BUILD_SHA}' -X 'main.buildDate=${BUILD_DATE}'
endif

GOLANGCI_LINT_VERSION = v2.12.2

# Baked into the client plugin binary (cmd/plugin) at build time, the same way LDFLAGS bakes
# main.buildVersion/main.buildDate into the in-pod binary.
CLIENT_LDFLAGS = -X 'main.version=$(VERSION)' \
	-X 'github.com/netobserv/network-observability-cli/internal/plugin/commands.defaultCollectorImage=$(IMAGE)' \
	-X 'github.com/netobserv/network-observability-cli/internal/plugin/commands.defaultAgentImage=$(AGENT_IMAGE)' \
	-X 'github.com/netobserv/network-observability-cli/internal/plugin/commands.defaultPullPolicy=$(PULL_POLICY)'

# build a single arch target provided as argument
define build_target
	echo 'building image for arch $(1)'; \
	DOCKER_BUILDKIT=1 $(OCI_BIN) buildx build --load --build-arg LDFLAGS="${LDFLAGS}" --build-arg TARGETARCH=$(1) ${OCI_BUILD_OPTS} ${EXTRA_BUILD_FLAGS} -t ${IMAGE}-$(1) -f Dockerfile .;
endef

# push a single arch target image
define push_target
	echo 'pushing image ${IMAGE}-$(1)'; \
	DOCKER_BUILDKIT=1 $(OCI_BIN) push ${IMAGE}-$(1);
endef

# manifest create a single arch target provided as argument
define manifest_add_target
	echo 'manifest add target $(1)'; \
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest add ${IMAGE} ${IMAGE}-$(1);
endef

# Build one native Krew plugin archive. The plugin does not use cgo, so disabling cgo makes
# cross-compilation reproducible and produces portable binaries for Krew's supported platforms.
define build_krew_target
	echo 'building Krew plugin for $(1)/$(2)'; \
	mkdir -p $(RELEASE_ASSETS_DIR)/$(1)-$(2); \
	CGO_ENABLED=0 GOOS=$(1) GOARCH=$(2) go build -ldflags "$(CLIENT_LDFLAGS)" -mod vendor -o $(RELEASE_ASSETS_DIR)/$(1)-$(2)/netobserv ./cmd/plugin; \
	cp LICENSE $(RELEASE_ASSETS_DIR)/$(1)-$(2)/LICENSE; \
	tar -czf $(RELEASE_ASSETS_DIR)/netobserv-cli-$(VERSION)-$(1)-$(2).tar.gz -C $(RELEASE_ASSETS_DIR)/$(1)-$(2) netobserv LICENSE;
endef

##@ General

.PHONY: prepare
prepare:
	@mkdir -p $(DIST_DIR)
	mkdir -p tmp

.PHONY: prereqs
prereqs: ## Install dependencies
	@echo "### Installing dependencies"
	test -f ./bin/golangci-lint-${GOLANGCI_LINT_VERSION} || ( \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s ${GOLANGCI_LINT_VERSION} \
		&& mv ./bin/golangci-lint ./bin/golangci-lint-${GOLANGCI_LINT_VERSION})

.PHONY: vendors
vendors: ## Refresh vendors directory.
	@echo "### Checking vendors"
	go mod tidy && go mod vendor

##@ Develop

.PHONY: build-collector
build-collector: ## Build the collector binary into DIST_DIR
	@echo "### Building collector"
	GOARCH=${GOARCH} go build ${BUILD_FLAGS} -mod vendor -o $(COLLECTOR_OUTPUT) ./cmd/collector

.PHONY: test
test: ## Run unit tests and generate cover.out (cluster integration tests excluded)
	@echo "### Testing code"
	GOOS=$(GOOS) go test -mod vendor $(UNIT_PACKAGES) -coverprofile cover.out

.PHONY: tests-e2e
tests-e2e: VERSION=test
tests-e2e: IMAGE=localhost/netobserv-cli:test
tests-e2e: PULL_POLICY=Never
tests-e2e: DIST_DIR=e2e/commands
tests-e2e: oc-commands ## Run e2e tests using kind cluster
	@rm -rf e2e/output
	@rm -f cli-e2e-img.tar
	go clean -testcache
	$(OCI_BIN) build . -t ${IMAGE}
	$(OCI_BIN) save -o cli-e2e-img.tar ${IMAGE}
	GOOS=$(GOOS) go test -p 1 -timeout 30m -v -mod vendor -tags e2e ./e2e/...

.PHONY: tests-int
tests-int: ## Run cluster integration tests. Requires KUBECONFIG and oc-netobserv in PATH
	GOOS= go test -p 1 -timeout 30m -v -mod vendor -tags integration ./e2e/integration-tests/...

.PHONY: coverage-report
coverage-report: ## Generate coverage report
	@echo "### Generating coverage report"
	go tool cover --func=./cover.out

.PHONY: coverage-report-html
coverage-report-html: ## Generate HTML coverage report
	@echo "### Generating HTML coverage report"
	go tool cover --html=./cover.out

.PHONY: fmt
fmt: ## Run go fmt against code.
	@echo "### Formatting code"
	go fmt ./...

.PHONY: lint
lint: prereqs ## Lint code
	@echo "### Linting code"
	./bin/golangci-lint-${GOLANGCI_LINT_VERSION} run ./... --timeout=3m

.PHONY: clean
clean: ## Clean up build directory
	@rm -rf $(DIST_DIR)
	@rm -rf $(FILES_OUTPUT_DIR)
	@rm -rf $(RELEASE_ASSETS_DIR)

.PHONY: build-plugin
build-plugin: ## Build the client plugin binary (oc-netobserv/kubectl-netobserv, or 'netobserv' for krew) into DIST_DIR
	@echo "### Building $(K8S_CLI_BIN) client plugin"
	@mkdir -p $(DIST_DIR)
	GOARCH=${GOARCH} go build -ldflags "$(CLIENT_LDFLAGS)" -mod vendor -o $(DIST_DIR)/$(if $(filter true,$(KREW_PLUGIN)),netobserv,$(K8S_CLI_BIN)-netobserv) ./cmd/plugin

.PHONY: commands
commands: build-plugin ## Compatibility alias for building the client plugin

.PHONY: kubectl-commands
kubectl-commands: K8S_CLI_BIN=kubectl
kubectl-commands: commands ## Build the kubectl client plugin binary into DIST_DIR

.PHONY: oc-commands
oc-commands: K8S_CLI_BIN=oc
oc-commands: commands ## Build the oc client plugin binary into DIST_DIR

.PHONY: install-commands
install-commands: commands
	sudo install -m 0755 \
		./build/$(K8S_CLI_BIN)-netobserv \
		/usr/local/bin/$(K8S_CLI_BIN)-netobserv

.PHONY: docs
docs: oc-commands ## Generate asciidoc
	go run -mod vendor ./cmd/devtools/gendocs

.PHONY: update-config
update-config: ## Update config from operator repo
	go run -mod vendor ./cmd/devtools/updateconfig

.PHONY: release
release: clean ## Generate tar.gz containing krew plugin
	@mkdir -p $(RELEASE_ASSETS_DIR)
	$(foreach target,$(KREW_PLATFORMS),$(call build_krew_target,$(word 1,$(subst /, ,$(target))),$(word 2,$(subst /, ,$(target)))))

.PHONY: create-kind-cluster
create-kind-cluster: prereqs ## Create a kind cluster
	go run -mod vendor ./cmd/devtools/kindcluster

.PHONY: destroy-kind-cluster
destroy-kind-cluster: KUBECONFIG=./kubeconfig
destroy-kind-cluster: ## Destroy the kind cluster.
	test -s ./kubeconfig || { echo "kubeconfig does not exist! Exiting..."; exit 1; }
	$(K8S_CLI_BIN) delete namespace netobserv-cli --ignore-not-found
	kind delete cluster --name netobserv-cli-cluster
	rm ./kubeconfig

.PHONY: $(COMMANDS)
$(COMMANDS): commands ## Run command using custom image
	@echo "### Running ${K8S_CLI_BIN}-netobserv $@ using $(IMAGE)"
	./$(DIST_DIR)/${K8S_CLI_BIN}-netobserv $@ $(COMMAND_ARGS)

##@ Images

# note: to build and push custom image tag use: IMAGE_ORG=myuser VERSION=dev
.PHONY: image-build
image-build: ## Build MULTIARCH_TARGETS images
	trap 'exit' INT; \
	$(foreach target,$(MULTIARCH_TARGETS),$(call build_target,$(target)))

.PHONY: image-push
image-push: ## Push MULTIARCH_TARGETS images
	trap 'exit' INT; \
	$(foreach target,$(MULTIARCH_TARGETS),$(call push_target,$(target)))

.PHONY: manifest-build
manifest-build: ## Build MULTIARCH_TARGETS manifest
	echo 'building manifest $(IMAGE)'
	DOCKER_BUILDKIT=1 $(OCI_BIN) rmi ${IMAGE} -f
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest create ${IMAGE} $(foreach target,$(MULTIARCH_TARGETS), --amend ${IMAGE}-$(target));

.PHONY: manifest-push
manifest-push: ## Push MULTIARCH_TARGETS manifest
	@echo 'publish manifest $(IMAGE)'
ifeq (${OCI_BIN}, docker)
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest push ${IMAGE};
else
	DOCKER_BUILDKIT=1 $(OCI_BIN) manifest push ${IMAGE} docker://${IMAGE};
endif

.PHONY: tar-image
tar-image: MULTIARCH_TARGETS=amd64
tar-image: image-build ## Build single arch (amd64) and save as a tar
	$(OCI_BIN) tag $(IMAGE)-amd64 $(IMAGE)
	mkdir -p ./out
	$(OCI_BIN) save -o out/image.tar $(IMAGE)
	echo $(IMAGE) > ./out/name

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

include .mk/dev.mk
include .mk/shortcuts.mk
