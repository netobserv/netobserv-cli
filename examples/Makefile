# Orchestrate OpenSSL TLS and cleartext HTTP test workloads (NETOBSERV-2858).
VERSION ?= latest
IMAGE_ORG ?= $(USER)
PULL_POLICY ?= Always

K8S_CLI_BIN_PATH := $(shell which oc 2>/dev/null || which kubectl)
K8S_CLI_BIN ?= $(shell basename ${K8S_CLI_BIN_PATH})

TLS_STACKS := openssl-test-pod
HTTP_STACK := http-test-pod
ALL_STACKS := $(TLS_STACKS) $(HTTP_STACK)

.PHONY: help
help: ## Show targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

.PHONY: images-all
images-all: ## Build and push all test images (OpenSSL TLS + HTTP)
	@for d in $(ALL_STACKS); do $(MAKE) -C $$d images VERSION=$(VERSION) IMAGE_ORG=$(IMAGE_ORG); done

.PHONY: images-http
images-http: ## Build and push cleartext HTTP test image only
	@$(MAKE) -C $(HTTP_STACK) images VERSION=$(VERSION) IMAGE_ORG=$(IMAGE_ORG)

.PHONY: deploy-local-all
deploy-local-all: ## Kind: load images then deploy all stacks locally
	@for d in $(ALL_STACKS); do $(MAKE) -C $$d deploy-local VERSION=$(VERSION) IMAGE_ORG=$(IMAGE_ORG); done

.PHONY: deploy-local-http
deploy-local-http: ## Kind: load and deploy cleartext HTTP pod
	@$(MAKE) -C $(HTTP_STACK) deploy-local VERSION=$(VERSION) IMAGE_ORG=$(IMAGE_ORG)

##@ Cluster

.PHONY: deploy-all
deploy-all: ## Deploy all test pods (OpenSSL TLS + HTTP)
	@for d in $(ALL_STACKS); do $(MAKE) -C $$d deploy VERSION=$(VERSION) IMAGE_ORG=$(IMAGE_ORG) PULL_POLICY=$(PULL_POLICY); done

.PHONY: deploy-http
deploy-http: ## Deploy cleartext HTTP test pod
	@$(MAKE) -C $(HTTP_STACK) deploy VERSION=$(VERSION) IMAGE_ORG=$(IMAGE_ORG) PULL_POLICY=$(PULL_POLICY)

.PHONY: wait-all
wait-all: ## Wait until all deployments are Available
	@for d in $(ALL_STACKS); do $(MAKE) -C $$d wait; done

.PHONY: wait-http
wait-http: ## Wait for HTTP test deployment
	@$(MAKE) -C $(HTTP_STACK) wait

.PHONY: traffic-all
traffic-all: ## Start traffic Jobs for all stacks
	@for d in $(ALL_STACKS); do $(MAKE) -C $$d traffic; done

.PHONY: traffic-http
traffic-http: ## Start cleartext HTTP traffic Job
	@$(MAKE) -C $(HTTP_STACK) traffic

.PHONY: pod-ips
pod-ips: ## Print pod IPs for --peer_ip scoping
	@echo "OPENSSL_POD_IP=$$($(MAKE) -sC openssl-test-pod pod-ip)"
	@echo "HTTP_POD_IP=$$($(MAKE) -sC http-test-pod pod-ip)"

.PHONY: capture-hint
capture-hint: pod-ips ## Print example netobserv packets commands
	@eval "$$($(MAKE) -s pod-ips | sed 's/^/export /')"
	@echo ""
	@echo "# Cleartext HTTP (PCA only — no TLS flags):"
	@echo './build/oc-netobserv packets --port=8080 --peer_ip="$$HTTP_POD_IP" --background'
	@echo ""
	@echo "# OpenSSL TLS plaintext JSONL (use agent image from NETOBSERV-2857):"
	@echo 'NETOBSERV_AGENT_IMAGE=<your-agent> ./build/oc-netobserv packets --port=8443 --peer_ip="$$OPENSSL_POD_IP" --enable_openssl --privileged --background'
	@echo 'jq -r "select(.RecordType==\"plaintext\") | {TLSSource,PlaintextDisplay}" output/plaintext/*.jsonl | head'
	@echo ""
	@echo "# Live TUI + wire/pcap correlation: NETOBSERV-2859"

.PHONY: undeploy-all
undeploy-all: ## Remove all test namespaces
	@for d in $(ALL_STACKS); do $(MAKE) -C $$d undeploy; done

.PHONY: undeploy-http
undeploy-http: ## Remove HTTP test namespace
	@$(MAKE) -C $(HTTP_STACK) undeploy
