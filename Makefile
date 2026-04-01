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
