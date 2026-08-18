# UsArr Makefile
#
# ─── HONESTY NOTICE ──────────────────────────────────────────────────────────
# UsArr is PRE-ALPHA. The build is real now: ./cmd/usarr, ./internal/..., ./web/
# and ./internal/db/migrations/ all exist, so build, test and check work. What is
# still missing is ./deploy/Dockerfile, so `make docker` WILL fail. Targets that
# reference a path that does not exist yet remain the build contract the commits
# that create it are written against, not a description of a working build.
#
# ./docs/design/check.mjs used to be on that list. It is not any more: the design
# thread introduced it in f015655, merged to main by e0d4b26, and `make design`
# ran green — observed on main at 2026-08-16 16:41 UTC, after a `git fetch`, and
# the file re-confirmed present at 17:34 UTC. That is an observation with a
# timestamp, not a standing guarantee; the target's own guards report the truth
# on the machine you are sitting at.
#
# Four rules this file must keep obeying as code lands:
#   1. `make check` is the pre-commit gate. It must pass with NO Docker daemon
#      and NO live services. It makes exactly TWO network calls, both to
#      vulnerability databases: govulncheck to vuln.go.dev, and `pnpm audit` to
#      the npm registry. Both live in the `vuln` target, so `make check-offline`
#      drops both and is the target to use on a plane. Everything else is
#      hermetic. This said ONE for the life of the project while the `vuln`
#      target ran two commands — count the calls, do not count the sentences.
#      `make spec-drift` (upstream OpenAPI drift) also needs the network, which is
#      exactly why it is a separate opt-in target and NEVER part of `check`: a
#      GitHub outage must not redden a commit that touched no spec. ADR-0047.
#   2. `make docker` is the ONLY target allowed to require a Docker daemon, and
#      it is never part of `check`.
#   3. `make design` is the ONLY target allowed to require a browser, and like
#      `make docker` it is never part of `check`. It guards docs/design/, which
#      ships nothing — the reasoning is in full above the target itself.
#   4. `make bench` holds every wall-clock performance measurement. Wall-clock
#      numbers are a release gate on named hardware, never a merge gate — see
#      docs/DEVELOPMENT.md §5. What the gate does enforce is `EXPLAIN QUERY PLAN`
#      and row-count assertions, which are deterministic and live in `make test`.
#      There is NO CI in this repo — `make check` is the whole gate and a person
#      or an agent has to type it. See docs/DEVELOPMENT.md §8.
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

# THIS REPO'S OWN .go FILES — not every .go file under the working directory,
# which is what `gofumpt -l .` means and what it used to be handed.
#
# A NESTED CHECKOUT INSIDE THE REPO SILENTLY CHANGES WHAT THE FORMATTER IS
# POINTED AT. Reproduced: a concurrent agent's `git worktree` at
# .claude/worktrees/… made fmt-check sweep 273 .go files instead of 135 and fail
# on somebody else's scratch files, so `make check` could not be run in the
# primary checkout at all (docs/REVIEW-LOG.md, "On the gate for M5-01…M5-11").
# `.claude/` is untracked, so nothing in the repo excluded it.
#
# `git ls-files --cached --others --exclude-standard` is the list that means
# "this repo's files": tracked files PLUS untracked ones .gitignore does not
# exclude — so a brand-new .go file is still format-checked before it is
# staged, which a bare `git ls-files` would lose, while .claude/ and any other
# ignored path is not. Outside a git work tree (a release tarball) it falls back
# to find, with the old prunes plus ./.claude.
GO_SRC_LIST := if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
	  git ls-files --cached --others --exclude-standard -- '*.go' | sort -u; \
	else \
	  find . -path ./.git -prune -o -path ./.claude -prune -o \
	    -path ./$(WEB_DIR)/node_modules -prune -o -name '*.go' -print; \
	fi

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

# Never blocks an unattended build on a TTY prompt. When pnpm is provided through
# corepack — `corepack enable` is the documented install route, so this is the
# common case even though it is not how this repo's container happens to be built
# — corepack asks for interactive confirmation before downloading a package
# manager it does not already have cached. There is no TTY in CI or in an agent
# container, so that prompt is not a question, it is a hang. Setting it to 0
# answers "yes, download it" up front; the version being downloaded is the one
# web/package.json pins, so this is not a floating-version hole.
export COREPACK_ENABLE_DOWNLOAD_PROMPT := 0

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

# gitleaks is the one pinned tool whose identity is asserted from its module
# path rather than its `--version` output, so the module path is a variable:
# `tools` installs it and `secrets` asserts it, and the two cannot drift apart.
GITLEAKS_MODULE       ?= github.com/zricethezav/gitleaks/v8

# ─── Resolving the pinned tools ──────────────────────────────────────────────
# PINNING A VERSION IS NOT THE SAME AS RUNNING IT. `make tools` installs into
# $GOBIN, which is very often NOT on $PATH — it is not on $PATH in this repo's
# own agent container. Every recipe below used to invoke the BARE tool name, so
# the gate ran whatever $PATH happened to resolve first and the pins above
# decorated a file that never consulted them. On the container that found this,
# $PATH resolved a system-wide golangci-lint v2.5.0 while this file pinned
# v2.12.2, and the other four pinned tools were not on $PATH at all. A gate that
# silently degrades to a weaker gate is worse than no gate: it produces a green
# result nobody re-examines. docs/reference/security.md §7.
#
# So: every pinned tool is invoked by ABSOLUTE PATH under $(GOBIN_DIR), never by
# name, and — where the tool can report its own version — that version is
# asserted against the pin before it runs. Both, not one:
#   * path alone silently runs a STALE binary left by an older pin, which is the
#     same class of bug as the one this block exists to close;
#   * assertion alone has already resolved and executed the wrong binary in
#     order to ask it what it is.
#
# gitleaks cannot answer the `--version` form. Measured on 2026-08-16 against
# the binary `make tools` installs:
#
#     $ /root/go/bin/gitleaks --version
#     gitleaks version version is set by build process
#
# That string is gitleaks' own default: version/version.go declares
# `var Version = "version is set by build process"`, and upstream overwrites it
# with `-ldflags -X` when it cuts a release. `go install` passes no ldflags, so
# the default survives. For most of this project's life that left gitleaks — the
# SECRETS scanner, the one binary you would least want to be an unknown build —
# as the only gate tool running with its identity unasserted.
#
# It is asserted now, from the Go build info the TOOLCHAIN stamps into every
# `go install`ed binary: `go version -m <bin>` reports the module path, the
# module version and the go.sum hash of what was actually fetched and built.
# That is the real answer to "which gitleaks is this", and it is not one this
# Makefile supplied. Two alternatives were tried and rejected:
#
#   * re-stamping the version ourselves at install time
#     (`go install -ldflags '-X …/version.Version=$(GITLEAKS_VERSION)' …`).
#     It works — verified, the binary then prints `gitleaks version v8.30.1` —
#     but the assertion becomes the Makefile asking the binary to repeat a
#     string the Makefile just handed it, and it makes `make tools` produce a
#     binary unlike every other install route, so a correct gitleaks installed
#     any other way would be rejected with "wrong version" while its version was
#     right. A guard that lies in the false direction is still a guard that lies.
#   * pinning the binary's content hash in-repo. sha256 of a Go binary is not
#     reproducible across toolchain patch versions, GOOS/GOARCH or build flags,
#     so the pin would have to be re-recorded per machine and would be updated
#     reflexively — which is the same as not having it.
#
# `go version -m` reads the binary on disk. No network, so `check-offline` keeps
# its contract.
#
# Override the directory if your layout differs: make check GOBIN_DIR=/some/bin
ifeq ($(origin GOBIN_DIR),undefined)
GOBIN_DIR := $(shell $(GO) env GOBIN)
ifeq ($(strip $(GOBIN_DIR)),)
GOBIN_DIR := $(shell $(GO) env GOPATH)/bin
endif
endif

GOFUMPT       := $(GOBIN_DIR)/gofumpt
GOLANGCI_LINT := $(GOBIN_DIR)/golangci-lint
GOOSE         := $(GOBIN_DIR)/goose
GOVULNCHECK   := $(GOBIN_DIR)/govulncheck
GITLEAKS      := $(GOBIN_DIR)/gitleaks

# golangci-lint prints "golangci-lint has version 2.12.2 ..." with no leading
# `v`, so the pin is stripped of it for the match. The others print theirs with
# the `v`. govulncheck puts it on the SECOND line ("Scanner: govulncheck@v1.7.0"),
# which is why the check flattens the whole output instead of taking head -1.
GOFUMPT_WANT     := $(GOFUMPT_VERSION)
GOLANGCI_WANT    := $(GOLANGCI_VERSION:v%=%)
GOOSE_WANT       := $(GOOSE_VERSION)
GOVULNCHECK_WANT := $(GOVULNCHECK_VERSION)

# gitleaks is asserted on the build-info `mod` line instead — `module@version`,
# exactly as `go version -m` prints the two fields.
GITLEAKS_WANT    := $(GITLEAKS_MODULE)@$(GITLEAKS_VERSION)

# ─── Did the pin hold, or was it moved? ──────────────────────────────────────
# A BANNER THAT READS THE SAME WHETHER THE PIN HELD OR WAS OVERRIDDEN IS A GUARD
# THAT MISDESCRIBES ITS OWN INVOCATION. `require_tool` compares the binary
# against $(*_WANT) and prints "asserted against the pin" — but every pin here is
# `?=`, and $(*_WANT) is a plain `:=` off it, so `make lint GOLANGCI_VERSION=2.5.0`
# moves the target the assertion aims at. The assertion still passes, truthfully:
# it did assert, against the version it was given. The banner just neglected to
# mention that the version it was given was not the one this file ships, which is
# the difference between a gate and a gate's echo.
#
# `$(origin V)` is the answer, and it is exact: `file` when the value is the one
# assigned here, `command line` or `environment` when somebody moved it. The
# *_PINVARS lists below name every variable a tool's pin is computed from —
# including the derived `*_WANT`, which is overridable on its own — so moving any
# one of them shows up. The BINARY's provenance needs no such help: the banner has
# printed the absolute path since the `$PATH` incident above, so an overridden
# `GOBIN_DIR` is already legible in the line itself.
#
# Overriding stays ALLOWED. Someone bisecting a version problem has a real reason
# to, and a guard that answered this by refusing would have traded a banner that
# lies for a gate that obstructs. The override is reported, not rejected.
GOFUMPT_PINVARS     := GOFUMPT_VERSION GOFUMPT_WANT
GOLANGCI_PINVARS    := GOLANGCI_VERSION GOLANGCI_WANT
GOOSE_PINVARS       := GOOSE_VERSION GOOSE_WANT
GOVULNCHECK_PINVARS := GOVULNCHECK_VERSION GOVULNCHECK_WANT
GITLEAKS_PINVARS    := GITLEAKS_MODULE GITLEAKS_VERSION GITLEAKS_WANT

# usage: $(call moved_pins,<names of the pin variables>) -> "" when every one of
# them came from this file, else one "NAME=value from the <origin>" per mover.
moved_pins = $(strip $(foreach v,$(1),\
    $(if $(filter-out file,$(origin $(v))),$(v)=$($(v)) from the $(origin $(v)))))

# usage: $(call pin_note,<names of the pin variables>) -> the clause require_tool
# puts after the version it measured. Says which pin was moved and from where.
pin_note = $(if $(call moved_pins,$(1)),asserted against an OVERRIDDEN pin ($(call moved_pins,$(1))) — NOT the version this Makefile ships,asserted against the pin)

# ─── Report what was measured, not just the verdict ──────────────────────────
# A CHECK THAT REPORTS ONLY PASS/FAIL CANNOT DISTINGUISH "PASSED" FROM "DID NOT
# RUN." For this project's whole life so far it was the second one: `check: OK`
# printed while a stale linter resolved off $PATH scanned the tree with an older
# ruleset. Any number at all in the output would have exposed that the first
# time somebody glanced at it.
#
# So each pinned step prints ONE line naming the binary it resolved, the version
# it asserted, and whatever count the tool gives cheaply. And where a floor is
# cheap to assert it IS asserted — a step that scans zero files or zero packages
# now fails instead of passing, because "nothing to check" and "everything
# checks out" are the two states a bare exit code cannot tell apart.
#
# One line per step, deliberately. This is a gate, not a build log; output
# nobody reads is how the gate got here.

# usage: $(call require_tool,<absolute path>,<--version substring or empty>,<module@version or empty>,<names of the pin variables>)
# Fails loudly and tells you to run `make tools` — the guard style `secrets`
# already used for gitleaks, now applied to every pinned tool.
#
# Exactly one of arg 2 and arg 3 is used, arg 2 first. Passing NEITHER is a hard
# error, not an existence-only fallback: an unasserted pinned binary is the hole
# this macro exists to close, so a tool added without an identity pin fails the
# gate instead of quietly opting out of it.
#
# Arg 4 is mandatory for the same reason one level up. Omitting it would leave a
# tool whose banner reads "asserted against the pin" no matter where its pin came
# from — the exact defect the *_PINVARS lists exist to close — and it would fail
# silently and in the reassuring direction, so it fails loudly here instead.
define require_tool
	@test -x $(1) || { \
		echo "missing pinned tool: $(1)"; \
		echo "run: make tools"; \
		exit 1; }
	@test -n "$(4)" || { \
		echo "unattributed pin for: $(1)"; \
		echo "  require_tool needs the names of the variables this tool's pin"; \
		echo "  is computed from (arg 4), so the banner can say whether the pin"; \
		echo "  held or was overridden on the command line."; \
		exit 1; }
	@if [ -n "$(2)" ]; then \
		got=$$($(1) --version 2>&1 | tr '\n' ' '); \
		case "$$got" in \
			*$(2)*) : ;; \
			*) echo "wrong version of pinned tool: $(1)"; \
			   echo "  want: $(2)"; \
			   echo "  got:  $$got"; \
			   echo "run: make tools"; \
			   exit 1 ;; \
		esac; \
		echo "tool: $(1) — version $(2), $(call pin_note,$(4))"; \
	elif [ -n "$(3)" ]; then \
		got=$$($(GO) version -m $(1) 2>/dev/null \
			| awk '$$1=="mod"{print $$2"@"$$3; exit}' || true); \
		if [ -z "$$got" ]; then \
			echo "no Go build info in pinned tool: $(1)"; \
			echo "  want: $(3)"; \
			echo "  go version -m said: $$($(GO) version -m $(1) 2>&1 | head -1 || true)"; \
			echo "run: make tools"; \
			exit 1; \
		fi; \
		if [ "$$got" != "$(3)" ]; then \
			echo "wrong version of pinned tool: $(1)"; \
			echo "  want: $(3)"; \
			echo "  got:  $$got"; \
			echo "run: make tools"; \
			exit 1; \
		fi; \
		echo "tool: $(1) — build-info module $(3), $(call pin_note,$(4)) (--version is unstamped)"; \
	else \
		echo "unasserted pinned tool: $(1)"; \
		echo "  require_tool needs a --version substring (arg 2) or a module@version (arg 3)."; \
		exit 1; \
	fi
endef

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
	cd $(WEB_DIR) && $(PNPM) install --frozen-lockfile

# `cd $(WEB_DIR) && pnpm …`, NOT `pnpm -C $(WEB_DIR) …`. The two run the same
# install and resolve the pnpm BINARY differently, which is the whole point.
#
# `-C` changes the directory pnpm operates on; it does not change the process's
# working directory when the launcher decides which pnpm to be. A corepack shim
# resolves `packageManager` from the CWD's nearest package.json, and the repo
# root has no package.json at all — this is a Go repo with a web/ subdirectory,
# not a JS workspace — so corepack finds no pin, falls back to its own default,
# and then the pnpm that actually starts reads web/package.json's
# `packageManager: pnpm@10.33.0` and hard-fails on the mismatch. Running with
# web/ as the CWD means the pin is found by whichever mechanism is looking, so
# the class of failure stops existing rather than being worked around.
#
# The recovery for the `-C` form was `corepack prepare` first, which is exactly
# the kind of undocumented prerequisite a fresh clone should not need.
#
# Each recipe line gets its own shell, so the `cd` cannot leak into a later line.

# Wrapper so web targets skip cleanly while web/ does not exist.
# usage: $(call pnpm_if_web,<script>)
define pnpm_if_web
	@if [ -f $(WEB_DIR)/package.json ]; then \
		cd $(WEB_DIR) && $(PNPM) $(1); \
	else \
		echo "SKIP: no $(WEB_DIR)/ yet — skipping 'pnpm $(1)'"; \
	fi
endef

# `go build ./cmd/usarr` on its own produces a binary that 404s on / because
# web/build was never generated. `build` depends on `web-build` to prevent that.

# ─── Deploy ──────────────────────────────────────────────────────────────────
# THIN WRAPPERS ONLY. The logic lives in deploy/*.sh and must stay there: a
# deployment host runs these over ssh, sometimes from a shell with no make
# target in mind, and the sequence is only auditable while it reads as one file
# instead of as backslash-continued recipe lines.
#
# CURDIR, not the scripts' /opt/UsArr default: invoked through make, the
# checkout being updated is by definition the one this Makefile is in, and
# silently updating a DIFFERENT checkout is the worst thing either script could
# do. An explicit USARR_CHECKOUT still wins. See docs/DEVELOPMENT.md §12.1.

.PHONY: deploy
deploy: ## Update this host: ff-only pull, make build, install, restart, verify the running commit
	@USARR_CHECKOUT="$${USARR_CHECKOUT:-$(CURDIR)}" ./deploy/update.sh

.PHONY: deploy-status
deploy-status: ## Report whether checkout, installed binary and running process are the same commit
	@USARR_CHECKOUT="$${USARR_CHECKOUT:-$(CURDIR)}" ./deploy/status.sh

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

# ─── Spec drift ──────────────────────────────────────────────────────────────
# The concrete form of the "scheduled job, not the PR gate" that DEVELOPMENT.md
# §7.2 and api/specs/SOURCES.md have both described since the specs were vendored.
#
# WHY IT IS NOT IN `check`, decided rather than assumed. Rule 1 at the top of this
# file says the gate makes exactly TWO network calls, both to vulnerability
# databases, and `check-offline` is defined as `check` minus them. This target
# talks to github.com, so putting it in the gate would break that contract AND
# make an upstream outage, a rate limit or an aeroplane fail somebody's unrelated
# commit. Upstream regenerating an OpenAPI document is news; it is not a reason to
# fail a commit that has nothing to do with it.
#
# The offline half of the same guard IS in the gate:
# TestVendoredSpecIsThePinnedBlob hashes the vendored bytes against a pinned git
# blob name, which is deterministic and needs nothing. See ADR-0047.
.PHONY: spec-drift
spec-drift: export CGO_ENABLED := 1
spec-drift: ## Tests behind the `upstream` tag: are the vendored specs still what upstream serves? NEEDS NETWORK. Never in CI.
	@test "$${USARR_SPEC_DRIFT:-}" = "1" || { \
		echo "refusing: set USARR_SPEC_DRIFT=1 — this target talks to github.com"; \
		echo "see docs/DEVELOPMENT.md §7.2"; exit 1; }
	$(GO) test -tags=upstream $(GOTESTFLAGS) -run Upstream ./internal/...
	@echo ""
	@echo "a failure here is NEWS, not a broken build: it means upstream moved. Read the message,"
	@echo "re-vendor deliberately, and revisit the ADR whose premise just changed."

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
# (Navidrome, a cgo driver), and reference/sync.md §6 marked cache_size and
# mmap_size "pending measurement". This target is the measurement: this driver, a
# 500k-row fixture, WAL, the §7.7 pragmas, idle and peak process RSS.
#
# It settled both (ADR-0001): cache_size is per-connection, so it multiplies by
# the read pool, and mmap_size did nothing at all because this driver compiles
# mmap out. mmap_size is no longer in the pragma list and is no longer swept by
# default — pass -mmap=... to sweep it if the driver ever gains mmap.
#
# It is behind the `bench` build tag like every other wall-clock measurement, and
# it is deliberately NOT part of `make bench`: that target runs Go benchmarks
# (`-run '^$$' -bench .`), and this is a long-running measurement tool whose
# output is a table for an ADR. Neither one is ever in `check`.
#
# A result belongs to the machine that produced it — architecture, core count and
# page size all move the numbers, and the report states all three.

.PHONY: bench-rss
bench-rss: ## Measure idle/peak RSS over a 500k-row DB, sweeping cache_size (ADR-0001)
	@echo "bench-rss: building a 500k-row fixture, then one child process per pragma cell."
	@echo "           first run takes a few minutes; the fixture is reused afterwards."
	$(GO) run -tags=bench ./internal/db/spike $(SPIKE_FLAGS)
	@echo ""
	@echo "record .dev/rss-spike.md in docs/DECISIONS.md ADR-0001, with the hardware named."
	@echo "the numbers are ONLY valid for the machine that produced them: architecture, core"
	@echo "count and page size all move them. See docs/DEVELOPMENT.md §5."

# Knobs, for a quick check or a different sweep. Examples:
#   make bench-rss SPIKE_FLAGS='-rows=50000'                    # fast smoke run
#   make bench-rss SPIKE_FLAGS='-rebuild'                       # remeasure the import peak
#   make bench-rss SPIKE_FLAGS='-cache=-2000,-32000'            # narrower sweep
#   make bench-rss SPIKE_FLAGS='-mmap=134217728'                # re-check mmap (see above)
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
	$(call require_tool,$(GOLANGCI_LINT),$(GOLANGCI_WANT),,$(GOLANGCI_PINVARS))
	@n=$$($(GO) list ./... | wc -l); \
	test "$$n" -gt 0 || { \
		echo "lint-go: 0 packages — the linter would scan nothing and exit 0."; exit 1; }; \
	echo "lint-go: linting $$n Go packages"
	$(GOLANGCI_LINT) run

.PHONY: lint-web
lint-web: web-deps ## eslint + svelte-check
	$(call pnpm_if_web,lint)
	$(call pnpm_if_web,check)

# Both fmt targets shell out to prettier, and prettier is a node_modules
# dependency exactly like eslint is — so both declare `web-deps`, for the same
# reason `lint-web` does. Without it, `fmt-check` is the FIRST target `check`
# runs and the only one reaching pnpm before anything has installed, so a fresh
# clone's very first `make check` died in prettier's plugin resolver:
#
#   [error] Cannot find package 'prettier-plugin-svelte' imported from …/web/noop.js
#   WARN   Local package.json exists, but node_modules missing, did you mean to install?
#
# Every green run of the gate before this had happened to run some other web
# target first, which is why the hole survived so long: `check` was only ever
# exercised on a tree that was already installed. Reproduced from a fresh
# `git clone` on 2026-08-16, and fixed here. See docs/REVIEW-LOG.md FI-02.
.PHONY: fmt
fmt: web-deps ## Format everything IN PLACE (gofumpt + prettier)
	$(call require_tool,$(GOFUMPT),$(GOFUMPT_WANT),,$(GOFUMPT_PINVARS))
	@$(GO_SRC_LIST) | xargs -r $(GOFUMPT) -l -w
	$(call pnpm_if_web,format)

.PHONY: fmt-check
fmt-check: web-deps ## Verify formatting without modifying files (used by `make check`)
	$(call require_tool,$(GOFUMPT),$(GOFUMPT_WANT),,$(GOFUMPT_PINVARS))
	@n=$$($(GO_SRC_LIST) | wc -l); \
	test "$$n" -gt 0 || { \
		echo "fmt-check: 0 .go files — gofumpt would scan nothing and exit 0."; exit 1; }; \
	echo "fmt-check: checking $$n .go files with gofumpt"; \
	out=$$($(GO_SRC_LIST) | xargs -r $(GOFUMPT) -l); \
	if [ -n "$$out" ]; then echo "not gofumpt-formatted:"; echo "$$out"; exit 1; fi
	$(call pnpm_if_web,format:check)

# ─── Build-tagged packages, which `go build ./...` does not see ──────────────
#
# internal/db/spike is behind `//go:build bench`, so `go list ./...` returns 11
# packages and never mentions it: a deliberate type error in that package passed
# the ENTIRE gate — fmt-check, lint, test, vuln, all green. Measured, then
# fixed here. gofumpt does see the files (it parses without resolving build
# tags), which is exactly why the hole was invisible: the formatter reported the
# package as checked while no compiler had ever looked at it.
#
# `go build` rather than `go vet` or `go test`: the spike has no tests, and a
# build with the tag on is the smallest thing that makes a type error fail. It
# writes no binary — `go build` only emits an executable when it is given a
# SINGLE main package, and `./...` is many.
#
# The `upstream` tag needs a SECOND command, and the difference is the whole
# reason this is spelled out. Everything behind `upstream` is a _test.go file,
# and `go build` does not compile test files — so `go build -tags=upstream ./...`
# would report success over a file no compiler had opened, which is the exact
# hole the paragraph above describes, one layer along. `go vet` does type-check
# test files, so it is what closes it. The gate must be able to compile
# `make spec-drift`'s tests without being allowed to RUN them (they need the
# network); vet gives precisely that.
.PHONY: build-tagged
build-tagged: ## Compile packages hidden behind build tags (`bench`: internal/db/spike; `upstream`: the spec-drift tests)
	@n=$$($(GO) list -tags=bench ./... | wc -l); \
	echo "build-tagged: compiling $$n Go packages with -tags=bench"
	$(GO) build -tags=bench ./...
	@echo "build-tagged: type-checking the -tags=upstream test files (go vet; go build cannot see them)"
	$(GO) vet -tags=upstream ./...

# ─── Design ──────────────────────────────────────────────────────────────────
# docs/design/check.mjs is the runnable form of DESIGN-DIRECTION.md §13: the ban
# sweep, token drift, contrast over every ground in both themes, overflow at five
# widths, the density row-height bands, accessible names, roving tabindex, the
# content-visibility guard and the webfont. One entry point, one exit code, and
# it prints what it CHECKED rather than only what failed.
#
# WHY IT IS NOT IN `check`, decided rather than assumed. It is genuinely
# hermetic — the mockups load over file:// and the woff2 subsets are inlined as
# data: URIs, so it makes ZERO network calls — and at ~40 s it is not slow.
# Neither of those is the reason it stays out. Three things are:
#
#   1. It needs a Playwright Chromium. The gate's whole contract is that it runs
#      on a machine carrying Go and pnpm and nothing else; a ~150 MB browser
#      download is a large new prerequisite for every developer and every runner.
#   2. The pin, and it is now DISCHARGED. This reason was wrong twice in two
#      directions, so both are recorded. It first said check.mjs imports
#      Playwright from an absolute path outside the repo, which stopped being
#      true when the file grew a resolution ladder — bare specifier first, then
#      web/node_modules, then the npm global root. It then said nothing declared
#      WHICH version, and that is closed: web/package.json pins `playwright-core`
#      at 1.56.1, exact, no caret — the version whose browsers.json declares
#      chromium revision 1194, which is the build in this container's cache and
#      the one `make design` was observed driving. It declares no install script
#      and downloads no browser, so a fresh install pays nothing for it.
#      The last gap was that the ladder asked only for the specifier
#      `playwright` while the pinned package is `playwright-core`, so the pin
#      resolved nowhere and was decorative. docs/design/check.mjs now probes both
#      specifiers at every rung, so the pin is what answers: before web/
#      node_modules was populated the checker printed `playwright resolved via
#      npm global root 'playwright'`, and after, `playwright resolved via
#      web/node_modules 'playwright-core'`. The pin is load-bearing, and this
#      reason no longer argues for anything.
#      Reason (1) is untouched by any of it: the ~150 MB of browser binaries is
#      still a prerequisite the gate's contract does not carry — this repo does
#      not vendor them and should not — and on its own it is still enough to
#      keep `design` out of `check`.
#   3. It guards docs/design/ — prose, tokens and mockups, none of which is a
#      shipping artifact. Token drift in a mockup cannot break the binary.
#
# So it is a target a person runs, like `make bench`. With (2) discharged, (1)
# and (3) are the whole of the case, and (1) carries it on its own.

# Overridable. The default is the browser cache this repo's agent container
# preinstalls; a workstation's is usually ~/.cache/ms-playwright.
PW_BROWSERS_PATH ?= /opt/pw-browsers
DESIGN_CHECK     ?= docs/design/check.mjs
NODE             ?= node

.PHONY: design
design: ## Run the design check (DESIGN-DIRECTION §13). Needs Chromium. NOT part of `check`.
	@test -f $(DESIGN_CHECK) || { \
		echo "no $(DESIGN_CHECK) — the design thread has not landed it on this branch yet."; \
		exit 1; }
	@command -v $(NODE) >/dev/null 2>&1 || { \
		echo "node not found — the design check needs Node 22+ with Playwright."; exit 1; }
	@test -d $(PW_BROWSERS_PATH) || { \
		echo "no Playwright browser cache at $(PW_BROWSERS_PATH)."; \
		echo "point at yours with: make design PW_BROWSERS_PATH=~/.cache/ms-playwright"; \
		exit 1; }
	@# The three guards above check preconditions check.mjs cannot check for
	@# itself. Whether the Playwright MODULE resolves is NOT one of them: the
	@# script resolves it through a fallback ladder and, on failure, prints the
	@# problem, the pinned install command and every attempt it made before
	@# exiting 1. A fourth guard here used to duplicate that, and it did so by
	@# grepping the import specifier out of check.mjs — asserting the SHAPE OF
	@# THE SCRIPT'S SOURCE rather than the behaviour it actually cared about.
	@# When the static import became a dynamic ladder the grep matched nothing,
	@# and under `.SHELLFLAGS := -eu -o pipefail` the assignment itself failed,
	@# killing the recipe before its own error message could print. Guard the
	@# behaviour you need or do not guard at all.
	PLAYWRIGHT_BROWSERS_PATH=$(PW_BROWSERS_PATH) $(NODE) $(DESIGN_CHECK)

# ─── Supply chain ────────────────────────────────────────────────────────────

.PHONY: secrets
secrets: ## Scan the working tree for committed credentials. GATING, part of `check`.
	$(call require_tool,$(GITLEAKS),,$(GITLEAKS_WANT),$(GITLEAKS_PINVARS))
	@# No count is added here: gitleaks already ends with "scanned ~N bytes in Ns",
	@# which is exactly the number this step needs to prove it looked at something.
	$(GITLEAKS) dir . --redact=100 --no-banner --exit-code 1

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
vuln: ## govulncheck + pnpm audit. GATING, part of `check`. THE ONLY NETWORK STEP — two calls.
	$(call require_tool,$(GOVULNCHECK),$(GOVULNCHECK_WANT),,$(GOVULNCHECK_PINVARS))
	@n=$$($(GO) list ./... | wc -l); \
	test "$$n" -gt 0 || { \
		echo "vuln: 0 packages — govulncheck would scan nothing and exit 0."; exit 1; }; \
	echo "vuln: scanning $$n Go packages against vuln.go.dev"
	$(GOVULNCHECK) ./...
	@echo "vuln: auditing the pnpm dependency tree against the npm registry"
	$(call pnpm_if_web,audit)

# TWO network calls happen here, not one: govulncheck queries vuln.go.dev and
# `pnpm audit` queries the npm registry's advisory endpoint. They are the only
# two in the whole gate, which is why both sit in this one target and why
# `check-offline` is exactly `check` minus this target.

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
	$(call require_tool,$(GOOSE),$(GOOSE_WANT),,$(GOOSE_PINVARS))
	@mkdir -p $(DEV_CONFIG_DIR)
	$(GOOSE) -dir $(MIGRATIONS) sqlite3 $(DEV_DB) up

.PHONY: migrate-down
migrate-down: ## Roll back ONE migration on the dev database (local testing only)
	$(call require_tool,$(GOOSE),$(GOOSE_WANT),,$(GOOSE_PINVARS))
	$(GOOSE) -dir $(MIGRATIONS) sqlite3 $(DEV_DB) down

.PHONY: migrate-status
migrate-status: ## Show migration status of the dev database
	$(call require_tool,$(GOOSE),$(GOOSE_WANT),,$(GOOSE_PINVARS))
	$(GOOSE) -dir $(MIGRATIONS) sqlite3 $(DEV_DB) status

.PHONY: migrate-new
migrate-new: ## Scaffold a migration: make migrate-new name=add_tag_rules
	$(call require_tool,$(GOOSE),$(GOOSE_WANT),,$(GOOSE_PINVARS))
	@test -n "$(name)" || { echo "usage: make migrate-new name=add_tag_rules"; exit 1; }
	@mkdir -p $(MIGRATIONS)
	$(GOOSE) -dir $(MIGRATIONS) create $(name) sql

# ─── Docker ──────────────────────────────────────────────────────────────────

.PHONY: docker
docker: ## Build the container image. THE ONLY TARGET THAT NEEDS A DOCKER DAEMON.
	@docker info >/dev/null 2>&1 || { \
		echo "no Docker daemon reachable."; \
		echo "this is expected in the agent container — see docs/DEVELOPMENT.md §8."; \
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
tools: ## Install the pinned dev tools into $(GOBIN_DIR)
	@echo "installing the pinned tools into $(GOBIN_DIR)"
	$(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	$(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	$(GO) install $(GITLEAKS_MODULE)@$(GITLEAKS_VERSION)
	@echo ""
	@echo "the recipes invoke these by absolute path under $(GOBIN_DIR), so you do"
	@echo "NOT need them on \$$PATH — and a stray copy on \$$PATH can no longer shadow them."

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
check: check-offline vuln ## THE PRE-COMMIT GATE. No Docker. Two network calls (vuln.go.dev, npm).
	@echo "check: OK"

.PHONY: check-offline
check-offline: fmt-check lint build-tagged modverify secrets test ## Everything in `check` except the vuln scan.
	@echo "check-offline: OK"
