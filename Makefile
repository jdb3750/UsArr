# UsArr Makefile
#
# ─── HONESTY NOTICE ──────────────────────────────────────────────────────────
# UsArr is PRE-ALPHA. The build is real now: ./cmd/usarr, ./internal/..., ./web/
# and ./internal/db/migrations/ all exist, so build, test and check work. What is
# still missing is ./deploy/Dockerfile, so `make docker` WILL fail. Targets that
# reference a path that does not exist yet remain the build contract the commits
# that create it are written against, not a description of a working build.
#
# Three rules this file must keep obeying as code lands:
#   1. `make check` is the pre-commit gate. It must pass with NO Docker daemon
#      and NO live services. It makes exactly ONE network call — govulncheck's
#      query to vuln.go.dev. `make check-offline` drops that one step and is the
#      target to use on a plane. Everything else is hermetic.
#   2. `make docker` is the ONLY target allowed to require a Docker daemon, and
#      it is never part of `check`.
#   3. `make bench` holds every wall-clock performance measurement. Wall-clock
#      numbers are a release gate on named hardware, never a merge gate — see
#      docs/DEVELOPMENT.md §5. What CI does enforce is `EXPLAIN QUERY PLAN` and
#      row-count assertions, which are deterministic and live in `make test`.
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
# Where web/scripts/sync-embed.mjs mirrors the SPA so //go:embed can reach it.
EMBED_DIR   ?= internal/web/spa
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

# Static binary. No CGO — SQLite is pure Go (ncruces/go-sqlite3 ships a
# wasm2go-translated SQLite; there is no Wasm runtime in the dependency graph).
# If this ever needs a C toolchain, something has gone wrong. DEVELOPMENT.md §1.
#
# This is the DEFAULT, so anything that produces a shipping artifact inherits it
# and a new build target added later cannot silently pick up the ambient value.
# The test recipes override it to 1 with a target-scoped variable, because the
# race detector requires cgo on every supported platform — see the note above
# GOTESTFLAGS. Overriding on the test side rather than removing the default here
# keeps the "no CGO in anything we ship" invariant fail-safe.
export CGO_ENABLED := 0

# Dependency integrity: never let a build silently mutate go.mod/go.sum.
export GOFLAGS := -mod=readonly

GO          ?= go

# -race requires cgo. Every recipe that consumes GOTESTFLAGS therefore carries a
# target-scoped `export CGO_ENABLED := 1`; without it `go test` refuses to start
# and the whole gate dies before running a single test. Tests are not a shipping
# artifact, so building them against the cgo resolvers is a non-issue — the
# binary in `make build` is still CGO_ENABLED=0.
GOTESTFLAGS ?= -race -shuffle=on
PNPM        ?= pnpm

# Dev database used by `make migrate`. Kept out of /config so `make clean` is safe.
DEV_CONFIG_DIR ?= ./.dev/config
DEV_DB         ?= $(DEV_CONFIG_DIR)/usarr.db

# ─── Pinned tool versions ────────────────────────────────────────────────────
# `@latest` is FORBIDDEN in this file. `make tools` runs `go install`, which
# executes the installed module's build on a developer machine that may hold a
# master key and *Arr admin credentials in .env. A floating version there is a
# supply-chain hole, not just a flaky gate. Bump these deliberately, in a commit
# that says why. Resolved from proxy.golang.org on 2026-08-16.
GOFUMPT_VERSION       ?= v0.11.0
GOLANGCI_VERSION      ?= v2.12.2
GOOSE_VERSION         ?= v3.27.3
GOVULNCHECK_VERSION   ?= v1.7.0
GITLEAKS_VERSION      ?= v8.30.1

# ─── Container base image ────────────────────────────────────────────────────
# Digest-pinned, always. A floating tag means the image you ship is not the
# image you tested. This is `gcr.io/distroless/static-debian12:nonroot` as
# resolved on 2026-08-16 — no shell, no package manager, a fixed non-root
# 65532:65532, and tzdata for TZ. It cannot run a chowning PUID/PGID entrypoint,
# which is deliberate: see docs/CONFIGURATION.md §2.4.
BASE_IMAGE ?= gcr.io/distroless/static-debian12:nonroot@sha256:1b7b9f0f0e0a1d2155f531db587cc48ec26aaf97ab64364225f5bf18a054e66a

# ─── Help ────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help
	@echo "UsArr — pre-alpha. Build, test and check work; targets whose paths do not exist yet fail."
	@echo ""
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ─── Development ─────────────────────────────────────────────────────────────

.PHONY: dev
dev: ## Run the backend (loads .env through the binary's own parser, if present)
	@test -f .env || echo "note: no .env — using defaults; a master key is generated on first run"
	$(GO) run $(MAIN_PKG) --env-file .env

# `.env` is DATA, not shell. It is deliberately not sourced with `. ./.env`:
# bash performs expansion and command substitution, Docker Compose's env_file
# parser does neither, so the same file would mean two different things — on a
# file that can hold the master key. The binary's own loader is the one parser.

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

# web/package.json is committed (since a279517), so the "no frontend yet" guard
# that used to wrap this line was dead code on every checkout. A missing
# package.json is now a hard failure, which is the right answer: it means the
# tree is broken, not that the frontend has not been written yet.
.PHONY: web-deps
web-deps:
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
test-go: export CGO_ENABLED := 1
test-go: web-build ## Go tests with the race detector. Includes the EXPLAIN QUERY PLAN gates.
	@if [ -f $(WEB_DIR)/package.json ]; then \
		USARR_REQUIRE_WEB_BUILD=1 $(GO) test $(GOTESTFLAGS) ./...; \
	else \
		$(GO) test $(GOTESTFLAGS) ./...; \
	fi

# Why `test-go` depends on `web-build`, and why USARR_REQUIRE_WEB_BUILD is set:
# internal/web's content assertions — above all TestEmbeddedFSCarriesAppDir, the
# only thing that catches a lost `all:` prefix — skip when nothing is embedded.
# Skipping is correct for a bare `go test` in a fresh clone, but it meant the
# gate went green on a tree where the trap was live. The dependency produces the
# build; the env var turns the skip into a failure once a frontend exists, so the
# guard cannot silently disarm itself again.

.PHONY: test-web
test-web: web-deps ## Frontend unit tests (vitest)
	$(call pnpm_if_web,test)

.PHONY: test-integration
test-integration: export CGO_ENABLED := 1
test-integration: ## Tests behind the `integration` tag. Needs a live stack. NEVER run in CI.
	@test "$${USARR_INTEGRATION:-}" = "1" || { \
		echo "refusing: set USARR_INTEGRATION=1 and have a live stack up"; \
		echo "see docs/DEVELOPMENT.md §7.1"; exit 1; }
	$(GO) test -tags=integration $(GOTESTFLAGS) ./...

.PHONY: bench
bench: ## Wall-clock performance harness. A RELEASE gate on named hardware, never a merge gate.
	@echo "bench: generating the reference fixture (10k movies / 2k series / ~400k episodes)"
	$(GO) test -tags=bench -run '^$$' -bench . -benchmem -benchtime 10x ./...
	@echo ""
	@echo "record the numbers, the hardware and the commit in docs/BENCHMARKS.md."
	@echo "these are NOT enforced in CI: a p99 latency gate on shared runners is a"
	@echo "flake generator, and on emulated arm64 it measures nothing. See DEVELOPMENT.md §5."

# ─── The RSS measurement ─────────────────────────────────────────────────────
# ARCHITECTURE §13's idle-RSS budget rested on a citation that does not transfer
# (Navidrome, a cgo driver), and reference/sync.md §6 marks cache_size and
# mmap_size "pending measurement". This target is the measurement: this driver, a
# 500k-row fixture, WAL, the §7.7 pragmas, idle and peak process RSS.
#
# It is behind the `bench` build tag like every other wall-clock measurement, and
# it is deliberately NOT part of `make bench`: that target runs Go benchmarks
# (`-run '^$$' -bench .`), and this is a long-running measurement tool whose
# output is a table for an ADR. Neither one is ever in `check`.
#
# A result belongs to the machine that produced it — architecture, core count and
# page size all move the numbers, and the report states all three.

.PHONY: bench-rss
bench-rss: ## Measure idle/peak RSS over a 500k-row DB, sweeping cache_size × mmap_size (ADR-0001)
	@echo "bench-rss: building a 500k-row fixture, then one child process per pragma cell."
	@echo "           first run takes a few minutes; the fixture is reused afterwards."
	$(GO) run -tags=bench ./internal/db/spike $(SPIKE_FLAGS)
	@echo ""
	@echo "record .dev/rss-spike.md in docs/DECISIONS.md ADR-0001, with the hardware named."
	@echo "the numbers are ONLY valid for the machine that produced them: architecture, core"
	@echo "count and page size all move them. See docs/DEVELOPMENT.md §5."

# Knobs, for a quick check or a different sweep. Examples:
#   make bench-rss SPIKE_FLAGS='-rows=50000'                  # fast smoke run
#   make bench-rss SPIKE_FLAGS='-rebuild'                     # remeasure the import peak
#   make bench-rss SPIKE_FLAGS='-cache=-2000,-32000 -mmap=0'  # narrower sweep
SPIKE_FLAGS ?=

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

# ─── Supply chain ────────────────────────────────────────────────────────────

.PHONY: secrets
secrets: ## Scan the working tree for committed credentials. GATING, part of `check`.
	@command -v gitleaks >/dev/null 2>&1 || { \
		echo "gitleaks not installed — run: make tools"; exit 1; }
	gitleaks dir . --redact=100 --no-banner --exit-code 1

# A gitignore is a request; this is enforcement. UsArr's repo carries *Arr admin
# keys in every developer's .env and fixture keys in testdata — the one thing
# that must never reach a commit is exactly the thing a human reviewer skims
# past. Waive a known-safe finding by id in .gitleaksignore, never by dropping
# the step. Fixture credentials must be structurally impossible to mistake for
# real ones (docs/DEVELOPMENT.md §7.1).

.PHONY: modverify
modverify: ## Verify module contents against go.sum, and that go.mod/go.sum are tidy
	$(GO) mod verify
	@# `go mod verify` only checks that the downloaded module CONTENT matches the
	@# recorded checksums. It cannot see that go.mod is untidy — that is how four
	@# directly-imported modules sat marked `// indirect` with go.sum entries
	@# missing and the gate stayed green. `-diff` exits non-zero and prints the
	@# patch instead of writing it, so the gate reports drift without mutating the
	@# tree under GOFLAGS=-mod=readonly. It reads the module cache, not the
	@# network, so it stays inside the `check-offline` contract.
	$(GO) mod tidy -diff

.PHONY: vuln
vuln: ## govulncheck + pnpm audit. GATING, part of `check`. THE ONE NETWORK STEP.
	govulncheck ./...
	$(call pnpm_if_web,audit)

# `|| true` used to live on the line above. It meant a known-vulnerable
# dependency in the crypto, HTTP or SQLite path shipped with a green build, in a
# project whose own security chapter opens "it is a credential vault for a dozen
# services". If a specific advisory genuinely must be waived, waive it by ID in
# a checked-in file so the waiver is reviewable — do not ignore the exit code.

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
	@case '$(BASE_IMAGE)' in \
		*@sha256:*) : ;; \
		*) echo "BASE_IMAGE is not digest-pinned: $(BASE_IMAGE)"; \
		   echo "resolve it with: docker buildx imagetools inspect <tag>"; exit 1 ;; \
	esac
	docker buildx build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		--provenance=true \
		--sbom=true \
		-f deploy/Dockerfile \
		-t usarr:$(VERSION) -t usarr:latest .

# `--provenance` and `--sbom` require buildx and are attached as OCI attestations
# to the pushed image, so "what is in this image, and what built it" is answerable
# without trusting the maintainer's memory. Signing is the missing third piece:
# a `sign` target (cosign keyless against the registry digest) must land before
# the first published tag, or ARCHITECTURE.md §14.7's "signed, reproducible
# container images" is a promise with no mechanism behind it.

# ─── Housekeeping ────────────────────────────────────────────────────────────

.PHONY: tools
tools: ## Install the pinned dev tools into $GOBIN
	$(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	$(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	$(GO) install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

.PHONY: clean
clean: ## Remove build artifacts and the dev database
	rm -f $(BINARY) cover.out cover.html
	rm -rf $(DIST_DIR) $(WEB_DIR)/build $(WEB_DIR)/.svelte-kit ./.dev
	@# The embed mirror too. //go:embed cannot reach outside internal/web, so the
	@# SPA is synced into $(EMBED_DIR) by web/scripts/sync-embed.mjs. Removing
	@# web/build without removing the mirror leaves a STALE build embedded in the
	@# next binary — `make clean && make build` would ship whatever was there
	@# before. .gitkeep survives: it is what keeps //go:embed compiling in a tree
	@# where the frontend has never been built.
	@find $(EMBED_DIR) -mindepth 1 -not -name .gitkeep -delete 2>/dev/null || true

.PHONY: check
check: check-offline vuln ## THE PRE-COMMIT GATE. No Docker. One network call (vuln.go.dev).
	@echo "check: OK"

.PHONY: check-offline
check-offline: fmt-check lint modverify secrets test ## Everything in `check` except the vuln scan.
	@echo "check-offline: OK"
