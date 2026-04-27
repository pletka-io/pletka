# Pletka root Makefile.
#
# All build / test / lint / release entry points go through this file so
# that local invocations and CI invocations stay aligned.

SHELL        := /usr/bin/env bash
.SHELLFLAGS  := -eu -o pipefail -c
.DEFAULT_GOAL := help

GO           ?= go
GOFLAGS      ?=
NPM          ?= npm
GORELEASER   ?= goreleaser
GITLEAKS     ?= gitleaks
GOLANGCILINT ?= golangci-lint

BIN_DIR      := bin
BINARY       := $(BIN_DIR)/pletka
RENDERER_DIR := renderer
DIST_DIR     := server/assets/dist

VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0-dev")
COMMIT       ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE         := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS      := -s -w \
                -X main.version=$(VERSION) \
                -X main.commit=$(COMMIT) \
                -X main.date=$(DATE)

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

.PHONY: build
build: renderer-build go-build ## Build renderer + Go binary into bin/pletka

.PHONY: renderer-build
renderer-build: ## Build the renderer bundle into server/assets/dist
	cd $(RENDERER_DIR) && $(NPM) ci
	cd $(RENDERER_DIR) && $(NPM) run build

.PHONY: go-build
go-build: ## Build the Go binary (assumes renderer bundle exists)
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/pletka

# ---------------------------------------------------------------------------
# Test / lint
# ---------------------------------------------------------------------------

.PHONY: test
test: go-test renderer-test ## Run Go + renderer tests

.PHONY: go-test
go-test: ## Run Go tests
	$(GO) test $(GOFLAGS) -race ./...

.PHONY: renderer-test
renderer-test: ## Run renderer type-check (and tests, when present)
	cd $(RENDERER_DIR) && $(NPM) run check
	cd $(RENDERER_DIR) && $(NPM) test --if-present

.PHONY: lint
lint: go-lint renderer-lint ## Lint Go + renderer

.PHONY: go-lint
go-lint: ## Run golangci-lint
	$(GOLANGCILINT) run --timeout=5m ./...

.PHONY: renderer-lint
renderer-lint: ## Run svelte-check
	cd $(RENDERER_DIR) && $(NPM) run check

.PHONY: tidy
tidy: ## Run go mod tidy and verify there is no diff
	$(GO) mod tidy
	@if ! git diff --quiet go.mod go.sum; then \
	  echo "go.mod or go.sum changed by 'go mod tidy' — commit the result"; \
	  exit 1; \
	fi

# ---------------------------------------------------------------------------
# Dev
# ---------------------------------------------------------------------------

.PHONY: dev
dev: ## Run renderer in watch mode + Go server (Ctrl-C stops both)
	@echo "Starting renderer watch and Go server..."
	@trap 'kill 0' INT TERM; \
	  ( cd $(RENDERER_DIR) && $(NPM) run dev ) & \
	  ( $(GO) run ./cmd/pletka ) & \
	  wait

# ---------------------------------------------------------------------------
# Security / release
# ---------------------------------------------------------------------------

.PHONY: gitleaks
gitleaks: ## Scan working tree for secrets
	$(GITLEAKS) detect --no-git --config=.gitleaks.toml --redact

.PHONY: release-snapshot
release-snapshot: build ## Build a goreleaser snapshot locally
	$(GORELEASER) release --snapshot --clean

.PHONY: release
release: ## Run a real goreleaser release (CI-only; expects a tag)
	$(GORELEASER) release --clean

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

.PHONY: clean
clean: ## Remove build artifacts (keeps dist/index.html placeholder)
	rm -rf $(BIN_DIR) dist/
	find $(DIST_DIR) -mindepth 1 ! -name index.html -delete
	cd $(RENDERER_DIR) && rm -rf dist node_modules/.cache .svelte-kit

.PHONY: distclean
distclean: clean ## Also remove node_modules and Go test cache
	cd $(RENDERER_DIR) && rm -rf node_modules
	$(GO) clean -testcache
