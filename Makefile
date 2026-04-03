# ============================================================
# github-app Makefile
# ============================================================

MODULE      := github.com/bwalsh/github-app
BINARY_NAME := github-app
CMD_PATH    := ./cmd/$(BINARY_NAME)
BIN_DIR     := bin
DIST_DIR    := dist

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE)

GO      := go
GOFLAGS := -trimpath
HELM    := helm
KUBECTL := kubectl
KIND    := kind

KIND_CLUSTER_NAME ?= github-app
K8S_NAMESPACE     ?= github-app
HELM_RELEASE      ?= github-app
HELM_CHART_PATH   ?= charts/github-app
INGRESS_NGINX_VERSION ?= controller-v1.13.3
KIND_INGRESS_WAIT_TIMEOUT ?= 300s
KIND_CERT_MANAGER_WAIT_TIMEOUT ?= 300s
HOST              ?= github-app.localdev.me
TLS_SECRET        ?= github-app-tls
IMG_REPO          ?= github-app
IMG_TAG           ?= dev
FULL_IMAGE        ?= $(IMG_REPO):$(IMG_TAG)
FULLNAME_OVERRIDE ?=
APP_SERVICE_NAME  ?= $(if $(FULLNAME_OVERRIDE),$(FULLNAME_OVERRIDE),$(HELM_RELEASE)-github-app)
LOCAL_PORT        ?= 8080
SERVICE_PORT      ?= 80

# Platforms for release builds
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.DEFAULT_GOAL := help

# ── Targets ──────────────────────────────────────────────────

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile the binary into bin/
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

.PHONY: run
run: build ## Build and run the server
	./$(BIN_DIR)/$(BINARY_NAME)

.PHONY: test
test: ## Run all tests with race detection
	$(GO) test -race -count=1 ./...

.PHONY: coverage
coverage: ## Run tests and generate an HTML coverage report
	@mkdir -p $(BIN_DIR)
	$(GO) test -race -coverprofile=$(BIN_DIR)/coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=$(BIN_DIR)/coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

.PHONY: lint
lint: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format source code
	$(GO) fmt ./...

.PHONY: tidy
tidy: ## Tidy and verify go.mod / go.sum
	$(GO) mod tidy
	$(GO) mod verify

.PHONY: release
release: ## Build release binaries for all platforms into dist/
	@mkdir -p $(DIST_DIR)
	@$(foreach PLATFORM, $(PLATFORMS), \
		$(eval OS   := $(word 1, $(subst /, ,$(PLATFORM)))) \
		$(eval ARCH := $(word 2, $(subst /, ,$(PLATFORM)))) \
		$(eval EXT  := $(if $(filter windows,$(OS)),.exe,)) \
		$(eval OUT  := $(DIST_DIR)/$(BINARY_NAME)-$(OS)-$(ARCH)$(EXT)) \
		echo "Building $(OUT) ..."; \
		GOOS=$(OS) GOARCH=$(ARCH) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
			-o $(OUT) $(CMD_PATH); \
	)
	@echo "Release binaries written to $(DIST_DIR)/"

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf $(BIN_DIR) $(DIST_DIR) coverage.html

.PHONY: helm-validate
helm-validate: ## Lint and template the Helm chart
	$(HELM) lint $(HELM_CHART_PATH)
	$(HELM) template $(HELM_RELEASE) $(HELM_CHART_PATH) \
		--set image.repository=$(IMG_REPO) \
		--set image.tag=$(IMG_TAG) >/dev/null

.PHONY: k8s-namespace
k8s-namespace: ## Create namespace if it does not already exist
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -

.PHONY: k8s-create-secrets
k8s-create-secrets: ## Create/update app secrets in Kubernetes from environment variables
	@set -eu; \
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -; \
	tmp_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp_dir"' EXIT; \
	printf '%s' "$${GITHUB_WEBHOOK_SECRET:?GITHUB_WEBHOOK_SECRET is required}" > "$$tmp_dir/github-webhook-secret"; \
	secret_args="--from-file=github-webhook-secret=$$tmp_dir/github-webhook-secret"; \
	if [ -n "$${GITHUB_APP_ID:-}" ] && [ -n "$${GITHUB_APP_INSTALLATION_ID:-}" ]; then \
		printf '%s' "$${GITHUB_APP_ID}" > "$$tmp_dir/github-app-id"; \
		printf '%s' "$${GITHUB_APP_INSTALLATION_ID}" > "$$tmp_dir/github-app-installation-id"; \
		secret_args="$$secret_args --from-file=github-app-id=$$tmp_dir/github-app-id --from-file=github-app-installation-id=$$tmp_dir/github-app-installation-id"; \
	elif [ -n "$${GITHUB_APP_ID:-}" ] || [ -n "$${GITHUB_APP_INSTALLATION_ID:-}" ]; then \
		echo "warning: GITHUB_APP_ID and GITHUB_APP_INSTALLATION_ID are optional, but must be provided together; skipping both." >&2; \
	fi; \
	if [ -n "$${GITHUB_APP_PRIVATE_KEY_FILE:-}" ]; then \
		cp "$$GITHUB_APP_PRIVATE_KEY_FILE" "$$tmp_dir/github-app-private-key"; \
		secret_args="$$secret_args --from-file=github-app-private-key=$$tmp_dir/github-app-private-key"; \
	elif [ -n "$${GITHUB_APP_PRIVATE_KEY:-}" ]; then \
		printf '%s' "$${GITHUB_APP_PRIVATE_KEY}" > "$$tmp_dir/github-app-private-key"; \
		secret_args="$$secret_args --from-file=github-app-private-key=$$tmp_dir/github-app-private-key"; \
	fi; \
	$(KUBECTL) -n $(K8S_NAMESPACE) create secret generic github-app-secrets \
		$$secret_args \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -

.PHONY: k8s-create-tls-secret
k8s-create-tls-secret: ## Create/update TLS secret (requires TLS_CERT_FILE and TLS_KEY_FILE)
	@set -eu; \
	: "$${TLS_CERT_FILE:?TLS_CERT_FILE is required}"; \
	: "$${TLS_KEY_FILE:?TLS_KEY_FILE is required}"; \
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -; \
	$(KUBECTL) -n $(K8S_NAMESPACE) create secret tls $(TLS_SECRET) \
		--cert="$$TLS_CERT_FILE" \
		--key="$$TLS_KEY_FILE" \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -

.PHONY: helm-deploy-staging
helm-deploy-staging: ## Deploy chart with cert-manager staging ClusterIssuer
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART_PATH) \
		--namespace $(K8S_NAMESPACE) \
		--set image.repository=$(IMG_REPO) \
		--set image.tag=$(IMG_TAG) \
		--set ingress.host=$(HOST) \
		--set ingress.tls.secretName=$(TLS_SECRET) \
		--set certManager.localFallbackIssuer.enabled=false \
		--set certManager.clusterIssuer.name=letsencrypt-staging

.PHONY: helm-deploy-production
helm-deploy-production: ## Deploy chart with cert-manager production ClusterIssuer
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART_PATH) \
		--namespace $(K8S_NAMESPACE) \
		--set image.repository=$(IMG_REPO) \
		--set image.tag=$(IMG_TAG) \
		--set ingress.host=$(HOST) \
		--set ingress.tls.secretName=$(TLS_SECRET) \
		--set certManager.localFallbackIssuer.enabled=false \
		--set certManager.clusterIssuer.name=letsencrypt-production

.PHONY: helm-deploy-local-tls
helm-deploy-local-tls: ## Deploy chart with pre-created TLS secret and cert-manager disabled
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART_PATH) \
		--namespace $(K8S_NAMESPACE) \
		--set image.repository=$(IMG_REPO) \
		--set image.tag=$(IMG_TAG) \
		--set ingress.host=$(HOST) \
		--set ingress.tls.secretName=$(TLS_SECRET) \
		--set certManager.enabled=false

.PHONY: helm-deploy-internal-test
helm-deploy-internal-test: ## Deploy internal-only test release, wait for deployment availability, and port-forward service
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART_PATH) \
		--namespace $(K8S_NAMESPACE) \
		--set image.repository=$(IMG_REPO) \
		--set image.tag=$(IMG_TAG) \
		--set ingress.enabled=false \
		$(if $(FULLNAME_OVERRIDE),--set fullnameOverride=$(FULLNAME_OVERRIDE),)
	$(KUBECTL) -n $(K8S_NAMESPACE) wait \
		--for=condition=Available deployment/$(APP_SERVICE_NAME) \
		--timeout=180s
	$(KUBECTL) -n $(K8S_NAMESPACE) get pods
	$(KUBECTL) -n $(K8S_NAMESPACE) port-forward svc/$(APP_SERVICE_NAME) $(LOCAL_PORT):$(SERVICE_PORT)

.PHONY: helm-local-checks
helm-local-checks: ## Run local checks against port-forwarded service (/healthz + empty webhook POST)
	curl -i --fail http://127.0.0.1:$(LOCAL_PORT)/healthz
	@status="$$(curl -s -o /tmp/github-app-webhook-check.out -w '%{http_code}' -X POST http://127.0.0.1:$(LOCAL_PORT)/webhook \
		-H 'content-type: application/json' \
		-d '{}')"; \
	if [ "$$status" != "400" ]; then \
		echo "expected POST /webhook to return HTTP 400, got $$status" >&2; \
		cat /tmp/github-app-webhook-check.out >&2; \
		exit 1; \
	fi; \
	echo "POST /webhook returned expected HTTP 400"

.PHONY: helm-status
helm-status: ## Show release and Kubernetes resource status
	$(KUBECTL) -n $(K8S_NAMESPACE) get deploy,pods,svc,ingress
	@INGRESS_NAMES="$$($(KUBECTL) -n $(K8S_NAMESPACE) get ingress -l app.kubernetes.io/instance=$(HELM_RELEASE) -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')"; \
	if [ -n "$$INGRESS_NAMES" ]; then \
		echo "Describing ingress resources for Helm release $(HELM_RELEASE):"; \
		echo "$$INGRESS_NAMES" | xargs -n1 $(KUBECTL) -n $(K8S_NAMESPACE) describe ingress; \
	else \
		echo "No ingress resources found for app.kubernetes.io/instance=$(HELM_RELEASE) in namespace $(K8S_NAMESPACE)."; \
	fi
	@if [ -n "$(TLS_SECRET)" ]; then \
		if ! $(KUBECTL) -n $(K8S_NAMESPACE) get secret $(TLS_SECRET); then \
			echo "TLS secret '$(TLS_SECRET)' not found in namespace $(K8S_NAMESPACE) (this may be expected if TLS is not yet provisioned or disabled)."; \
		fi; \
	else \
		echo "TLS secret name (TLS_SECRET) is not set; skipping TLS secret status."; \
	fi
	$(HELM) -n $(K8S_NAMESPACE) status $(HELM_RELEASE)

.PHONY: docker-build
docker-build: ## Build local container image for github-app
	docker build -t $(FULL_IMAGE) .

.PHONY: kind-load-image
kind-load-image: docker-build ## Load local container image into Kind nodes
	$(KIND) load docker-image $(FULL_IMAGE) --name $(KIND_CLUSTER_NAME)

.PHONY: kind-bootstrap
kind-bootstrap: ## Create Kind cluster and install ingress-nginx + cert-manager
	@command -v $(KIND) >/dev/null 2>&1 || { echo "$(KIND) is required; install Kind before running kind-bootstrap" >&2; exit 1; }
	@command -v $(KUBECTL) >/dev/null 2>&1 || { echo "$(KUBECTL) is required; install kubectl before running kind-bootstrap" >&2; exit 1; }
	$(KIND) get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$" || $(KIND) create cluster --name $(KIND_CLUSTER_NAME)
	$(KUBECTL) apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/$(INGRESS_NGINX_VERSION)/deploy/static/provider/kind/deploy.yaml
	$(KUBECTL) -n ingress-nginx rollout status deployment/ingress-nginx-controller --timeout=$(KIND_INGRESS_WAIT_TIMEOUT)
	$(KUBECTL) apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml
	$(KUBECTL) wait --for=condition=Available --timeout=$(KIND_CERT_MANAGER_WAIT_TIMEOUT) deployment/cert-manager -n cert-manager
	$(KUBECTL) wait --for=condition=Available --timeout=$(KIND_CERT_MANAGER_WAIT_TIMEOUT) deployment/cert-manager-webhook -n cert-manager
	$(KUBECTL) wait --for=condition=Available --timeout=$(KIND_CERT_MANAGER_WAIT_TIMEOUT) deployment/cert-manager-cainjector -n cert-manager

.PHONY: kind-install-issuers
kind-install-issuers: ## Install ClusterIssuer manifests (staging, production, local fallback)
	$(KUBECTL) apply -f deploy/issuers/letsencrypt-staging.yaml
	$(KUBECTL) apply -f deploy/issuers/letsencrypt-production.yaml
	$(KUBECTL) apply -f deploy/issuers/selfsigned-local.yaml

.PHONY: kind-create-secrets
kind-create-secrets: k8s-create-secrets ## Create/update app secrets in Kubernetes from environment variables

.PHONY: kind-deploy-staging
kind-deploy-staging: kind-load-image helm-deploy-staging ## Deploy chart using Let’s Encrypt staging issuer

.PHONY: kind-deploy-local
kind-deploy-local: kind-load-image ## Deploy chart using local self-signed fallback issuer
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART_PATH) \
		--namespace $(K8S_NAMESPACE) \
		--set image.repository=$(IMG_REPO) \
		--set image.tag=$(IMG_TAG) \
		--set ingress.host=$(HOST) \
		--set ingress.tls.secretName=$(TLS_SECRET) \
		--set certManager.localFallbackIssuer.enabled=true \
		--set certManager.localFallbackIssuer.name=selfsigned-local

.PHONY: kind-deploy-verify
kind-deploy-verify: ## Bootstrap Kind, deploy app, and verify /healthz via the helper script
	./scripts/kind-deploy-verify.sh

.PHONY: kind-clean
kind-clean: ## Uninstall Helm release and delete Kind cluster
	-$(HELM) uninstall $(HELM_RELEASE) -n $(K8S_NAMESPACE)
	-$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
