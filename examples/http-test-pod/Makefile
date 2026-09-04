# note: to build and push custom image tag use: IMAGE_ORG=netobserv VERSION=latest
VERSION ?= latest

GOARCH ?= amd64

IMAGE_ORG ?= $(USER)

NAME := http-test
NAMESPACE ?= http-test

K8S_CLI_BIN_PATH := $(shell which oc 2>/dev/null || which kubectl)
K8S_CLI_BIN ?= $(shell basename ${K8S_CLI_BIN_PATH})

IMAGE_REGISTRY ?= quay.io

IMAGE_TAG_BASE ?= $(IMAGE_REGISTRY)/$(IMAGE_ORG)/$(NAME)
IMAGE ?= $(IMAGE_TAG_BASE):$(VERSION)
PULL_POLICY ?= Always

OCI_BIN_PATH := $(shell which docker 2>/dev/null || which podman)
OCI_BIN ?= $(shell basename ${OCI_BIN_PATH})
OCI_BUILD_OPTS ?=

ifeq ("$(OCI_BIN)","docker")
EXTRA_BUILD_FLAGS ?= --provenance=false
endif

KIND_CLUSTER_NAME ?= netobserv-cli-cluster

.PHONY: image-build
image-build:
	DOCKER_BUILDKIT=1 $(OCI_BIN) build $(OCI_BUILD_OPTS) $(EXTRA_BUILD_FLAGS) -t $(IMAGE) .

.PHONY: image-push
image-push:
	DOCKER_BUILDKIT=1 $(OCI_BIN) push $(IMAGE)

.PHONY: images
images: image-build image-push

.PHONY: kind-load
kind-load:
	kind load docker-image $(IMAGE) --name $(KIND_CLUSTER_NAME)

.PHONY: deploy-local
deploy-local:
	$(MAKE) kind-load
	$(MAKE) deploy PULL_POLICY=IfNotPresent

.PHONY: deploy
deploy:
	sed -e 's|__HTTP_TEST_IMAGE__|$(IMAGE)|g' \
	    -e 's|__IMAGE_PULL_POLICY__|$(PULL_POLICY)|g' \
	    deployment.yaml | $(K8S_CLI_BIN) apply -f -

.PHONY: undeploy
undeploy:
	$(K8S_CLI_BIN) delete -f deployment.yaml --ignore-not-found
	$(K8S_CLI_BIN) delete -f traffic-job.yaml --ignore-not-found

.PHONY: traffic
traffic:
	$(K8S_CLI_BIN) apply -f traffic-job.yaml

.PHONY: wait
wait:
	$(K8S_CLI_BIN) wait -n $(NAMESPACE) deployment/http-test --for=condition=Available --timeout=120s

.PHONY: pod-ip
pod-ip:
	@$(K8S_CLI_BIN) get pod -n $(NAMESPACE) -l app=http-test -o jsonpath='{.items[0].status.podIP}{"\n"}'

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
