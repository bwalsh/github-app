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
HOST              ?= github-app.localdev.me
TLS_SECRET        ?= github-app-tls
IMG_REPO          ?= github-app
IMG_TAG           ?= dev
FULL_IMAGE        ?= $(IMG_REPO):$(IMG_TAG)

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
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
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

.PHONY: docker-build
docker-build: ## Build local container image for github-app
	docker build -t $(FULL_IMAGE) .

.PHONY: kind-load-image
kind-load-image: docker-build ## Load local container image into Kind nodes
	$(KIND) load docker-image $(FULL_IMAGE) --name $(KIND_CLUSTER_NAME)

.PHONY: kind-bootstrap
kind-bootstrap: ## Create Kind cluster and install ingress-nginx + cert-manager
	$(KIND) get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$" || $(KIND) create cluster --name $(KIND_CLUSTER_NAME)
	$(KUBECTL) apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
	$(KUBECTL) wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=180s
	$(KUBECTL) apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml
	$(KUBECTL) wait --for=condition=Available --timeout=180s deployment/cert-manager -n cert-manager
	$(KUBECTL) wait --for=condition=Available --timeout=180s deployment/cert-manager-webhook -n cert-manager
	$(KUBECTL) wait --for=condition=Available --timeout=180s deployment/cert-manager-cainjector -n cert-manager

.PHONY: kind-install-issuers
kind-install-issuers: ## Install ClusterIssuer manifests (staging, production, local fallback)
	$(KUBECTL) apply -f deploy/issuers/letsencrypt-staging.yaml
	$(KUBECTL) apply -f deploy/issuers/letsencrypt-production.yaml
	$(KUBECTL) apply -f deploy/issuers/selfsigned-local.yaml

.PHONY: kind-create-secrets
kind-create-secrets: ## Create/update app secrets in Kubernetes from environment variables
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) -n $(K8S_NAMESPACE) create secret generic github-app-secrets \
		--from-literal=github-webhook-secret="$(GITHUB_WEBHOOK_SECRET)" \
		--from-literal=github-app-id="$(GITHUB_APP_ID)" \
		--from-literal=github-app-installation-id="$(GITHUB_APP_INSTALLATION_ID)" \
		--from-literal=github-app-private-key="$(GITHUB_APP_PRIVATE_KEY)" \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -

.PHONY: kind-deploy-staging
kind-deploy-staging: kind-load-image ## Deploy chart using Let’s Encrypt staging issuer
	$(KUBECTL) create namespace $(K8S_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART_PATH) \
		--namespace $(K8S_NAMESPACE) \
		--set image.repository=$(IMG_REPO) \
		--set image.tag=$(IMG_TAG) \
		--set ingress.host=$(HOST) \
		--set ingress.tls.secretName=$(TLS_SECRET) \
		--set certManager.localFallbackIssuer.enabled=false \
		--set certManager.clusterIssuer.name=letsencrypt-staging

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

.PHONY: kind-clean
kind-clean: ## Uninstall Helm release and delete Kind cluster
	-$(HELM) uninstall $(HELM_RELEASE) -n $(K8S_NAMESPACE)
	-$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
