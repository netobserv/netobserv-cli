# VERSION defines the project version.
# Update this value when you upgrade the version of your project.
# To re-generate a tar.gz for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make tar-commands VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= main

# Go architecture and targets images to build
GOARCH ?= $(shell go env GOARCH)
PLUGIN_GOOS ?= $(shell go env GOOS)
MULTIARCH_TARGETS ?= amd64

# In CI, to be replaced by `netobserv`
IMAGE_ORG ?= $(USER)

# Build output
NAME := network-observability-cli
DIST_DIR ?= build
FILES_OUTPUT_DIR ?= output
OUTPUT := $(DIST_DIR)/$(NAME)

# Available commands for development with args
COMMANDS = flows packets cleanup
COMMAND_ARGS ?=

# Plugin filename prefix; neither external CLI is needed to build or run it.
K8S_CLI_BIN ?= oc

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
OCI_BIN_PATH = $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
OCI_BIN ?= $(if $(OCI_BIN_PATH),$(notdir $(OCI_BIN_PATH)),podman)
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
YQ_VERSION = v4.45.1

# build a single arch target provided as argument
define build_target
	echo 'building image for arch $(1)'; \
	DOCKER_BUILDKIT=1 $(OCI_BIN) buildx build --load --build-arg LDFLAGS="${LDFLAGS}" --build-arg TARGETARCH=$(1) --build-arg VERSION="$(VERSION)" --build-arg COLLECTOR_IMAGE="$(IMAGE)" --build-arg AGENT_IMAGE="$(AGENT_IMAGE)" --build-arg PULL_POLICY="$(PULL_POLICY)" ${OCI_BUILD_OPTS} ${EXTRA_BUILD_FLAGS} -t ${IMAGE}-$(1) -f Dockerfile .;
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
	GOFLAGS="" go install github.com/mikefarah/yq/v4@${YQ_VERSION}

.PHONY: vendors
vendors: ## Refresh vendors directory.
	@echo "### Checking vendors"
	go mod tidy && go mod vendor

##@ Develop

.PHONY: compile
compile: ## Build the binary
	@echo "### Compiling project"
	GOARCH=${GOARCH} go build ${BUILD_FLAGS} -mod vendor -o $(OUTPUT)

.PHONY: test
test: ## Test code using go test
	@echo "### Testing code"
	GOOS=$(GOOS) go test -mod vendor -a ./... -coverpkg=./... -coverprofile cover.out

.PHONY: tests-e2e
tests-e2e: VERSION=test
tests-e2e: IMAGE=localhost/netobserv-cli:test
tests-e2e: PULL_POLICY=Never
tests-e2e: DIST_DIR=e2e/commands
tests-e2e: oc-commands ## Run e2e tests using kind cluster
	@rm -rf e2e/output
	@rm -f cli-e2e-img.tar
	go clean -testcache
	$(OCI_BIN) build --build-arg TARGETARCH=$(GOARCH) --build-arg VERSION="$(VERSION)" --build-arg COLLECTOR_IMAGE="$(IMAGE)" --build-arg AGENT_IMAGE="$(AGENT_IMAGE)" --build-arg PULL_POLICY="$(PULL_POLICY)" . -t ${IMAGE}
	$(OCI_BIN) save -o cli-e2e-img.tar ${IMAGE}
	GOOS=$(GOOS) go test -p 1 -timeout 30m -v -mod vendor -tags e2e ./e2e/...

.PHONY: tests-int
tests-int: oc-commands ## Build the Go plugin and run integration tests against KUBECONFIG
	PATH="$(abspath $(DIST_DIR)):$$PATH" GOOS= go test -p 1 -timeout 30m -v -mod vendor ./e2e/integration-tests/...

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
ifeq (, $(shell which shellcheck))
	@echo "### shellcheck could not be found, skipping shell lint"
else
	@echo "### Run shellcheck against bash scripts"
	find . -name '*.sh' -not -path "./vendor/*" | xargs shellcheck
endif

.PHONY: clean
clean: ## Clean up build directory
	@rm -rf $(DIST_DIR)
	@rm -rf $(FILES_OUTPUT_DIR)

.PHONY: commands
commands: ## Build the standalone Go plugin (also usable through oc/kubectl)
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=$(PLUGIN_GOOS) GOARCH=$(GOARCH) go build -mod=vendor \
		-ldflags "-X main.version=$(VERSION) -X github.com/netobserv/network-observability-cli/internal/pkg/plugin.CollectorImage=$(IMAGE) -X github.com/netobserv/network-observability-cli/internal/pkg/plugin.AgentImage=$(AGENT_IMAGE) -X github.com/netobserv/network-observability-cli/internal/pkg/plugin.PullPolicy=$(PULL_POLICY)" \
		-o $(DIST_DIR)/$(if $(filter true,$(KREW_PLUGIN)),,$(K8S_CLI_BIN)-)netobserv.tmp ./cmd/oc-netobserv
	mv $(DIST_DIR)/$(if $(filter true,$(KREW_PLUGIN)),,$(K8S_CLI_BIN)-)netobserv.tmp $(DIST_DIR)/$(if $(filter true,$(KREW_PLUGIN)),,$(K8S_CLI_BIN)-)netobserv

.PHONY: kubectl-commands
kubectl-commands: K8S_CLI_BIN=kubectl
kubectl-commands: commands ## Generate kubectl plugins and add them to build folder

.PHONY: oc-commands
oc-commands: K8S_CLI_BIN=oc
oc-commands: commands ## Generate oc plugins and add them to build folder

.PHONY: install-commands
install-commands: commands ## Generate plugins and add them to /usr/bin/
	sudo cp -a ./build/. /usr/bin/

.PHONY: docs
docs: oc-commands ## Generate asciidoc
	CLI_BIN="./$(DIST_DIR)/oc-netobserv" ./scripts/generate-doc.sh

.PHONY: update-config
update-config: ## Update config from operator repo
	./scripts/update-config.sh

.PHONY: release
release: ## Build an OS/architecture-specific Krew archive
	$(MAKE) KREW_PLUGIN=true kubectl-commands
	tar -czf netobserv-cli-$(PLUGIN_GOOS)-$(GOARCH).tar.gz LICENSE ./build/netobserv

.PHONY: release-all
release-all: ## Build supported Linux and macOS plugin archives
	@for arch in amd64 arm64 ppc64le s390x; do $(MAKE) release PLUGIN_GOOS=linux GOARCH=$$arch || exit $$?; done
	@for arch in amd64 arm64; do $(MAKE) release PLUGIN_GOOS=darwin GOARCH=$$arch || exit $$?; done

.PHONY: create-kind-cluster
create-kind-cluster: prereqs ## Create a kind cluster
	scripts/kind-cluster.sh

.PHONY: destroy-kind-cluster
destroy-kind-cluster: KUBECONFIG=./kubeconfig
destroy-kind-cluster: oc-commands ## Destroy the kind cluster.
	test -s ./kubeconfig || { echo "kubeconfig does not exist! Exiting..."; exit 1; }
	./$(DIST_DIR)/oc-netobserv cleanup --kubeconfig="$(KUBECONFIG)"
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
