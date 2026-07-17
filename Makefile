# Core Makefile for the Pletka application.
#
# Platform/customer operations such as Airtable export, import waves, ontology
# manifest loading, deployment, and full reloads live in pletka-platform.

BINARY_NAME ?= pletka
BIN_DIR ?= bin

# Per-worktree overrides: .env (gitignored) sets PLETKA_PORT, PLETKA_DB_NAME,
# WEAVE_DB, etc. so sibling worktrees do not collide on port or database.
# See .env.example for the template.
ifneq (,$(wildcard ./.env))
include .env
export
endif

PORT ?= $(if $(PLETKA_PORT),$(PLETKA_PORT),$(if $(ZELLIJ_PORT),$(ZELLIJ_PORT),3333))
WEAVE_DB ?= $(if $(PLETKA_DB_NAME),$(PLETKA_DB_NAME),pletka_weave)
TIMESTAMP := $(shell date +%Y%m%d_%H%M%S)

RED = \033[0;31m
GREEN = \033[0;32m
YELLOW = \033[1;33m
NC = \033[0m

.PHONY: help
help:
	@echo "$(GREEN)Pletka Core Build System$(NC)"
	@echo "========================="
	@echo ""
	@echo "$(GREEN)Application:$(NC)"
	@echo "  make build                Build the pletka binary into bin/"
	@echo "  make build-all            Alias for make build"
	@echo "  make run [PORT=3333]      Build and run the server"
	@echo "  make prod-run [PORT=3333] Build and run in production mode"
	@echo "  make dev [PORT=3333]      Run with hot reload"
	@echo "  make prod-dev [PORT=3333] Run hot reload with embedded assets"
	@echo ""
	@echo "$(GREEN)Database:$(NC)"
	@echo "  make db-status            Show core table counts"
	@echo "  make db-reset             Reset the current database with confirmation"
	@echo "  make db-clone             Clone SRC_DB into WEAVE_DB"
	@echo "  make db-drop              Drop WEAVE_DB when it is not the default DB"
	@echo "  make db-backup            Create a local compressed database backup"
	@echo "  make worktree-init        Create per-worktree DB and update .env"
	@echo ""
	@echo "$(GREEN)Frontend:$(NC)"
	@echo "  make frontend-install     Install frontend dependencies"
	@echo "  make frontend-build       Build Svelte islands"
	@echo "  make frontend-dev         Watch and rebuild frontend assets"
	@echo "  make typegen              Generate TypeScript types"
	@echo ""
	@echo "$(GREEN)Utility:$(NC)"
	@echo "  make test                 Run Go tests"
	@echo "  make vet                  go vet ./..."
	@echo "  make lint                 golangci-lint (full — staticcheck + suite)"
	@echo "  make lint-new             golangci-lint on changes vs origin/main (the gate)"
	@echo "  make lint-fix             golangci-lint --fix (auto-fixable issues)"
	@echo "  make vuln                 govulncheck vulnerability scan"
	@echo "  make check                build + vet + lint-new + test (pre-push gate)"
	@echo "  make clean                Remove generated local artifacts"

BUILDINFO_PKG := github.com/pletka-io/pletka/pkg/buildinfo
GIT_COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_BRANCH    := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
GIT_DIRTY     := $(shell git diff --quiet 2>/dev/null || echo dirty)
BUILD_TIME    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION       ?= dev

BUILDINFO_LDFLAGS := -X $(BUILDINFO_PKG).Version=$(VERSION) \
                     -X $(BUILDINFO_PKG).GitCommit=$(GIT_COMMIT) \
                     -X $(BUILDINFO_PKG).GitBranch=$(GIT_BRANCH) \
                     -X $(BUILDINFO_PKG).GitDirty=$(GIT_DIRTY) \
                     -X $(BUILDINFO_PKG).BuildTime=$(BUILD_TIME)

.PHONY: build
build: frontend-build-if-needed
	@mkdir -p $(BIN_DIR)
	@echo "$(GREEN)Building $(BINARY_NAME) ($(VERSION) $(GIT_COMMIT)$(if $(GIT_DIRTY),.dirty))...$(NC)"
	@CGO_ENABLED=0 go build -ldflags "-w -s $(BUILDINFO_LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) .
	@echo "$(GREEN)Build complete: $(BIN_DIR)/$(BINARY_NAME)$(NC)"

.PHONY: build-all
build-all: build

.PHONY: run
run: build
	@echo "Starting $(BINARY_NAME) on port $(PORT)..."
	./$(BIN_DIR)/$(BINARY_NAME) serve --port $(PORT)

.PHONY: prod-run
prod-run: build
	@echo "$(GREEN)Starting $(BINARY_NAME) in production mode...$(NC)"
	@if lsof -ti:$(PORT) >/dev/null 2>&1; then \
		echo "$(YELLOW)Port $(PORT) is in use. Killing process...$(NC)"; \
		lsof -ti:$(PORT) | xargs kill -9 2>/dev/null || true; \
		sleep 1; \
	fi
	@PLETKA_ENV=production PLETKA_LOG_LEVEL=info ./$(BIN_DIR)/$(BINARY_NAME) serve --port $(PORT)

.PHONY: dev air
dev: air
air: frontend-build-if-needed
	@echo "$(GREEN)Starting $(BINARY_NAME) with hot reload on http://localhost:$(PORT)$(NC)"
	@if lsof -ti:$(PORT) >/dev/null 2>&1; then \
		echo "$(YELLOW)Port $(PORT) is in use. Killing process...$(NC)"; \
		lsof -ti:$(PORT) | xargs kill -9 2>/dev/null || true; \
		sleep 1; \
	fi
	@PLETKA_BIN_DIR=$(BIN_DIR) PLETKA_PORT=$(PORT) PLETKA_LOG_LEVEL=debug go tool air 2>&1 | tee server.log

.PHONY: prod-dev
prod-dev:
	@echo "$(GREEN)Starting $(BINARY_NAME) with production hot reload on http://localhost:$(PORT)$(NC)"
	@if lsof -ti:$(PORT) >/dev/null 2>&1; then \
		echo "$(YELLOW)Port $(PORT) is in use. Killing process...$(NC)"; \
		lsof -ti:$(PORT) | xargs kill -9 2>/dev/null || true; \
		sleep 1; \
	fi
	@PLETKA_BIN_DIR=$(BIN_DIR) PLETKA_PORT=$(PORT) PLETKA_LOG_LEVEL=info go tool air -c .air.prod.toml 2>&1 | tee server-prod.log

WEAVE_DSN = PGPASSWORD=pw123 psql -h localhost -p 5433 -U postgres -d $(WEAVE_DB)

.PHONY: db-status
db-status:
	@echo "$(YELLOW)PostgreSQL Status:$(NC)"
	@$(WEAVE_DSN) -c "\
		SELECT 'Projects' as type, COUNT(*) as count FROM weave_projects \
		UNION ALL SELECT 'Fields', COUNT(*) FROM weave_fields \
		UNION ALL SELECT 'Models', COUNT(*) FROM weave_models \
		UNION ALL SELECT 'Collections', COUNT(*) FROM weave_collections \
		UNION ALL SELECT 'Categories', COUNT(*) FROM weave_categories;" 2>/dev/null || echo "Could not connect to database"

.PHONY: db-backfill-path-elements
db-backfill-path-elements: build
	@echo "$(YELLOW)Backfilling path_elements for all fields...$(NC)"
	./$(BIN_DIR)/$(BINARY_NAME) db backfill-path-elements

.PHONY: db-backfill-path-elements-dry
db-backfill-path-elements-dry: build
	@echo "$(YELLOW)Checking fields needing path_elements backfill...$(NC)"
	./$(BIN_DIR)/$(BINARY_NAME) db backfill-path-elements --dry-run

.PHONY: db-cleanup-orphan-concept-lists
db-cleanup-orphan-concept-lists:
	@echo "$(GREEN)Cleaning orphan weave_concept_lists rows from $(WEAVE_DB)...$(NC)"
	@$(WEAVE_DSN) -f scripts/sql/cleanup_orphan_concept_lists.sql

.PHONY: db-backfill-institution-visibility
db-backfill-institution-visibility:
	@echo "$(GREEN)Backfilling institution visibility in $(WEAVE_DB)...$(NC)"
	@$(WEAVE_DSN) -f scripts/sql/backfill_institution_visibility.sql

.PHONY: db-reset
db-reset:
	@echo "$(RED)WARNING: This will delete all data in the PostgreSQL database!$(NC)"
	@echo "Press Ctrl+C to cancel, or Enter to continue..."
	@read confirm
	@$(MAKE) db-reset-no-prompt

DB_NAME ?= $(WEAVE_DB)

.PHONY: db-reset-no-prompt
db-reset-no-prompt:
	@echo "$(YELLOW)Resetting database $(DB_NAME)...$(NC)"
	@PGPASSWORD=pw123 psql -h localhost -p 5433 -U postgres -d $(DB_NAME) -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	@echo "$(GREEN)Database reset complete$(NC)"

SRC_DB ?= pletka_weave

.PHONY: db-clone
db-clone:
	@if [ "$(WEAVE_DB)" = "$(SRC_DB)" ]; then \
		echo "$(RED)ERROR: WEAVE_DB ($(WEAVE_DB)) equals SRC_DB ($(SRC_DB)). Set PLETKA_DB_NAME in .env first.$(NC)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Cloning $(SRC_DB) to $(WEAVE_DB)...$(NC)"
	@PGPASSWORD=pw123 psql -h localhost -p 5433 -U postgres -d postgres -c "DROP DATABASE IF EXISTS $(WEAVE_DB);"
	@PGPASSWORD=pw123 psql -h localhost -p 5433 -U postgres -d postgres -c "CREATE DATABASE $(WEAVE_DB) WITH TEMPLATE $(SRC_DB) OWNER postgres;"
	@echo "$(GREEN)Cloned to $(WEAVE_DB)$(NC)"

.PHONY: worktree-init
worktree-init:
	@WT_NAME=$${NAME:-$$(basename $$(pwd) | tr '-' '_')}; \
	NEW_DB="pletka_weave_$$WT_NAME"; \
	if [ "$$NEW_DB" = "pletka_weave_main" ]; then \
		echo "$(YELLOW)main worktree uses canonical pletka_weave. Skipping clone.$(NC)"; \
		exit 0; \
	fi; \
	echo "$(YELLOW)Cloning pletka_weave to $$NEW_DB...$(NC)" && \
	PGPASSWORD=pw123 psql -h localhost -p 5433 -U postgres -d postgres -c "DROP DATABASE IF EXISTS $$NEW_DB;" >/dev/null && \
	PGPASSWORD=pw123 psql -h localhost -p 5433 -U postgres -d postgres -c "CREATE DATABASE $$NEW_DB WITH TEMPLATE pletka_weave OWNER postgres;" >/dev/null && \
	touch .env && \
	sed -i '/^PLETKA_DB_NAME=/d' .env && \
	echo "PLETKA_DB_NAME=$$NEW_DB" >> .env && \
	echo "$(GREEN)Cloned pletka_weave to $$NEW_DB; .env updated$(NC)" && \
	echo "  PLETKA_DB_NAME=$$NEW_DB"

.PHONY: db-drop
db-drop:
	@if [ "$(WEAVE_DB)" = "pletka_weave" ]; then \
		echo "$(RED)ERROR: refusing to drop default DB pletka_weave. Override WEAVE_DB.$(NC)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Dropping $(WEAVE_DB)...$(NC)"
	@PGPASSWORD=pw123 psql -h localhost -p 5433 -U postgres -d postgres -c "DROP DATABASE IF EXISTS $(WEAVE_DB);"
	@echo "$(GREEN)Dropped $(WEAVE_DB)$(NC)"

LOCAL_DB_PASSWORD ?= pw123
LOCAL_DB_HOST ?= localhost
LOCAL_DB_PORT ?= 5433
LOCAL_DB_USER ?= postgres
LOCAL_DB_NAME ?= $(WEAVE_DB)

.PHONY: db-backup
db-backup:
	@echo "$(YELLOW)Creating local database backup...$(NC)"
	@mkdir -p backups
	@BACKUP_FILE="backups/$(LOCAL_DB_NAME)_backup_$(TIMESTAMP).sql.zst"; \
	PGPASSWORD=$(LOCAL_DB_PASSWORD) pg_dump -h $(LOCAL_DB_HOST) -p $(LOCAL_DB_PORT) -U $(LOCAL_DB_USER) -d $(LOCAL_DB_NAME) \
		--clean --if-exists --no-owner --no-acl | zstd > $$BACKUP_FILE && \
	echo "$(GREEN)Backup saved to $$BACKUP_FILE$(NC)"

.PHONY: frontend-install
frontend-install:
	@echo "$(GREEN)Installing frontend dependencies...$(NC)"
	@cd frontend && npm ci
	@echo "$(GREEN)Frontend dependencies installed$(NC)"

.PHONY: frontend-build
frontend-build:
	@echo "$(GREEN)Building frontend assets...$(NC)"
	@cd frontend && npm ci && npx vite build
	@echo "$(GREEN)Frontend build complete$(NC)"

.PHONY: frontend-build-if-needed
frontend-build-if-needed:
	@MANIFEST=pkg/assets/static/dist/.vite/manifest.json; \
	if [ ! -f $$MANIFEST ] || [ ! -d frontend/node_modules ]; then \
		echo "$(YELLOW)Frontend build missing; building...$(NC)"; \
		$(MAKE) frontend-build; \
	elif [ -n "$$(find frontend/src -newer $$MANIFEST -type f -print -quit 2>/dev/null)" ] \
	    || [ -n "$$(find frontend -maxdepth 2 \( -name 'package*.json' -o -name 'vite.config.*' -o -name 'tsconfig*.json' -o -name 'svelte.config.*' -o -name 'postcss.config.*' -o -name 'tailwind.config.*' \) -newer $$MANIFEST -print -quit 2>/dev/null)" ]; then \
		echo "$(YELLOW)Frontend sources changed; rebuilding...$(NC)"; \
		$(MAKE) frontend-build; \
	fi

.PHONY: frontend-dev
frontend-dev:
	@echo "$(GREEN)Watching frontend assets for changes...$(NC)"
	@cd frontend && npx vite build --watch

.PHONY: typegen
typegen:
	@echo "$(GREEN)Generating TypeScript types from Go models...$(NC)"
	@go tool tygo generate
	@echo "$(GREEN)TypeScript types generated$(NC)"

.PHONY: clean
clean:
	@echo "$(YELLOW)Cleaning...$(NC)"
	@rm -rf $(BIN_DIR)
	@rm -rf tmp/
	@rm -f *.log
	@echo "$(GREEN)Clean complete$(NC)"

.PHONY: test
test:
	@echo "$(YELLOW)Running tests...$(NC)"
	@go test ./...

# ---------------------------------------------------------------------------
# Static analysis
#
# golangci-lint v2 aggregates staticcheck + ~18 linters (see .golangci.yml)
# in one fast parallel pass (~9s cold, ~1-2s warm). It MUST be built with the
# same Go toolchain as the code (the config targets go 1.26); an older binary
# refuses to run. Install/upgrade with:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
# ---------------------------------------------------------------------------
GOLANGCI := golangci-lint

.PHONY: vet
vet:
	@echo "$(YELLOW)go vet...$(NC)"
	@go vet ./...

.PHONY: lint
lint:
	@command -v $(GOLANGCI) >/dev/null 2>&1 || { echo "$(RED)golangci-lint not found — go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest$(NC)"; exit 1; }
	@echo "$(YELLOW)golangci-lint (full)...$(NC)"
	@$(GOLANGCI) run ./...

# lint-new is the realistic gate on a legacy codebase: only reports issues
# introduced by changes vs origin/main, so the ~2.5k-issue backlog doesn't
# block new work. Clean the backlog gradually via `make lint`.
.PHONY: lint-new
lint-new:
	@command -v $(GOLANGCI) >/dev/null 2>&1 || { echo "$(RED)golangci-lint not found — go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest$(NC)"; exit 1; }
	@echo "$(YELLOW)golangci-lint (changes vs origin/main)...$(NC)"
	@$(GOLANGCI) run --new-from-rev=origin/main ./...

.PHONY: lint-fix
lint-fix:
	@$(GOLANGCI) run --fix ./...

.PHONY: vuln
vuln:
	@echo "$(YELLOW)govulncheck...$(NC)"
	@go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Fast pre-push gate: compile everything, vet, lint only the diff, run tests.
# govulncheck is intentionally separate (downloads the advisory DB — slower).
.PHONY: check
check:
	@echo "$(YELLOW)build...$(NC)" && go build ./...
	@$(MAKE) --no-print-directory vet
	@$(MAKE) --no-print-directory lint-new
	@$(MAKE) --no-print-directory test
	@echo "$(GREEN)check passed$(NC)"

.DEFAULT_GOAL := help
