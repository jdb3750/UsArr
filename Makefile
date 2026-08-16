# UsArr Makefile
#
# ─── HONESTY NOTICE ──────────────────────────────────────────────────────────
# UsArr is PRE-ALPHA and currently has ZERO application code. Most targets below
# reference paths that do not exist yet: ./cmd/usarr, ./internal/..., ./web/,
# ./deploy/Dockerfile, ./internal/db/migrations/. They WILL fail until those are
# created. That is intentional — this file is the build contract the first
# commits are written against, not a description of a working build.
#
# Two rules this file must keep obeying as code lands:
#   1. `make check` (fmt-check + lint + test) must pass with NO Docker daemon,
#      NO live services and NO network. The CI/agent container has none of them.
#   2. `make docker` is the ONLY target allowed to require a Docker daemon, and
#      it is never part of `check`.
#
# Reference: docs/DEVELOPMENT.md
# ─────────────────────────────────────────────────────────────────────────────

SHELL       := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# ─── Variables ───────────────────────────────────────────────────────────────

BINARY      ?= usarr
MAIN_PKG    ?= ./cmd/usarr
WEB_DIR     ?= web
MIGRATIONS  ?= internal/db/migrations
DIST_DIR    ?= dist

# Build metadata stamped into the binary via -ldflags.
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE)

# Static binary. No CGO — SQLite is a pure-Go Wasm build (ncruces/go-sqlite3).
# If this ever needs a C toolchain, something has gone wrong. See DEVELOPMENT.md §1.
export CGO_ENABLED := 0

GO          ?= go
GOTESTFLAGS ?= -race -shuffle=on
PNPM        ?= pnpm

# Dev database used by `make migrate`. Kept out of /config so `make clean` is safe.
DEV_CONFIG_DIR ?= ./.dev/config
DEV_DB         ?= $(DEV_CONFIG_DIR)/usarr.db

# Pinned tool versions. Bump deliberately; a floating linter is a flaky gate.
GOFUMPT_VERSION       ?= latest
GOLANGCI_VERSION      ?= v2.12.2
GOOSE_VERSION         ?= latest
GOVULNCHECK_VERSION   ?= latest

# ─── Help ────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help
	@echo "UsArr — pre-alpha. Most targets fail until the code they reference exists."
	@echo ""
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ─── Development ─────────────────────────────────────────────────────────────

.PHONY: dev
dev: ## Run the backend against .env (SPA served separately by `make web-dev`)
	@test -f .env || { echo "no .env — run: cp .env.example .env && edit it"; exit 1; }
	set -a; . ./.env; set +a; $(GO) run $(MAIN_PKG)

.PHONY: web-dev
web-dev: web-deps ## Run the SvelteKit dev server with HMR (proxies /api to the backend)
	$(call pnpm_if_web,dev)

# ─── Build ───────────────────────────────────────────────────────────────────

.PHONY: build
build: web-build ## Build the static binary with the SPA embedded -> ./usarr
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) $(MAIN_PKG)
	@echo "built ./$(BINARY)  version=$(VERSION) commit=$(COMMIT)"

.PHONY: web-build
web-build: web-deps ## Build the SvelteKit SPA -> web/build (embedded by `make build`)
	$(call pnpm_if_web,build)

# Frontend dependency guard.
# While the project is pre-alpha there is no web/ yet, so every web target
# no-ops loudly instead of dying with a cryptic "No rule to make target".
# Delete the guard branch once web/package.json exists.
.PHONY: web-deps
web-deps:
	@if [ ! -f $(WEB_DIR)/package.json ]; then \
		echo "SKIP: $(WEB_DIR)/package.json not present yet (pre-alpha) — skipping frontend step"; \
		exit 0; \
	fi; \
	$(PNPM) -C $(WEB_DIR) install --frozen-lockfile

# Wrapper so web targets skip cleanly while web/ does not exist.
# usage: $(call pnpm_if_web,<script>)
define pnpm_if_web
	@if [ -f $(WEB_DIR)/package.json ]; then \
		$(PNPM) -C $(WEB_DIR) $(1); \
	else \
		echo "SKIP: no $(WEB_DIR)/ yet — skipping 'pnpm $(1)'"; \
	fi
endef

# `go build ./cmd/usarr` on its own produces a binary that 404s on / because
# web/build was never generated. `build` depends on `web-build` to prevent that.

# ─── Test ────────────────────────────────────────────────────────────────────

.PHONY: test
test: test-go test-web ## Run all tests (offline; no Docker, no network)

.PHONY: test-go
test-go: ## Go tests with the race detector
	$(GO) test $(GOTESTFLAGS) ./...

.PHONY: test-web
test-web: web-deps ## Frontend unit tests (vitest)
	$(call pnpm_if_web,test)

.PHONY: test-integration
test-integration: ## Tests behind the `integration` tag. Needs a live stack. NEVER run in CI.
	@test "$${USARR_INTEGRATION:-}" = "1" || { \
		echo "refusing: set USARR_INTEGRATION=1 and have a live stack up"; \
		echo "see docs/DEVELOPMENT.md §7.1"; exit 1; }
	$(GO) test -tags=integration $(GOTESTFLAGS) ./...

.PHONY: cover
cover: ## Coverage report -> cover.html
	$(GO) test -coverprofile=cover.out -covermode=atomic ./...
	$(GO) tool cover -html=cover.out -o cover.html
	@echo "wrote cover.html"

# ─── Lint & format ───────────────────────────────────────────────────────────

.PHONY: lint
lint: lint-go lint-web ## Run all linters

.PHONY: lint-go
lint-go: ## golangci-lint (v2 config format: .golangci.yml must declare version: "2")
	golangci-lint run

.PHONY: lint-web
lint-web: web-deps ## eslint + svelte-check
	$(call pnpm_if_web,lint)
	$(call pnpm_if_web,check)

.PHONY: fmt
fmt: ## Format everything IN PLACE (gofumpt + prettier)
	gofumpt -l -w .
	$(call pnpm_if_web,format)

.PHONY: fmt-check
fmt-check: ## Verify formatting without modifying files (used by `make check`)
	@out=$$(gofumpt -l .); \
	if [ -n "$$out" ]; then echo "not gofumpt-formatted:"; echo "$$out"; exit 1; fi
	$(call pnpm_if_web,format:check)

.PHONY: vuln
vuln: ## govulncheck — advisory, not a gate
	govulncheck ./... || true

# ─── Migrations ──────────────────────────────────────────────────────────────
# Migrations are embedded (//go:embed migrations/*.sql) and applied at startup,
# so a binary can always bring its own DB forward. These targets are for
# authoring and for poking the dev database. Forward-only: write a `-- +goose Down`
# block for local testing, but downgrades are not a supported user path.

.PHONY: migrate
migrate: ## Apply pending migrations to the dev database
	@mkdir -p $(DEV_CONFIG_DIR)
	goose -dir $(MIGRATIONS) sqlite3 $(DEV_DB) up

.PHONY: migrate-down
migrate-down: ## Roll back ONE migration on the dev database (local testing only)
	goose -dir $(MIGRATIONS) sqlite3 $(DEV_DB) down

.PHONY: migrate-status
migrate-status: ## Show migration status of the dev database
	goose -dir $(MIGRATIONS) sqlite3 $(DEV_DB) status

.PHONY: migrate-new
migrate-new: ## Scaffold a migration: make migrate-new name=add_tag_rules
	@test -n "$(name)" || { echo "usage: make migrate-new name=add_tag_rules"; exit 1; }
	@mkdir -p $(MIGRATIONS)
	goose -dir $(MIGRATIONS) create $(name) sql

# ─── Docker ──────────────────────────────────────────────────────────────────

.PHONY: docker
docker: ## Build the container image. THE ONLY TARGET THAT NEEDS A DOCKER DAEMON.
	@docker info >/dev/null 2>&1 || { \
		echo "no Docker daemon reachable."; \
		echo "this is expected in the CI/agent container — see docs/DEVELOPMENT.md §8."; \
		exit 1; }
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-f deploy/Dockerfile \
		-t usarr:$(VERSION) -t usarr:latest .

# ─── Housekeeping ────────────────────────────────────────────────────────────

.PHONY: tools
tools: ## Install the pinned dev tools into $GOBIN
	$(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	$(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

.PHONY: clean
clean: ## Remove build artifacts and the dev database
	rm -f $(BINARY) cover.out cover.html
	rm -rf $(DIST_DIR) $(WEB_DIR)/build $(WEB_DIR)/.svelte-kit ./.dev

.PHONY: check
check: fmt-check lint test ## THE PRE-COMMIT GATE. Must pass offline, with no Docker.
	@echo "check: OK"
