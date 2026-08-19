# syntax=docker/dockerfile:1
#
# UsArr container image: a single CGO-free static binary with the SvelteKit SPA
# embedded via //go:embed, on a distroless base, non-root from PID 1.
# Authoritative deployment requirements: docs/ARCHITECTURE.md §15.
#
# HONESTY NOTE. This file has NOT been built in the environment it was written
# in: the agent container carries no Docker daemon (docs/DEVELOPMENT.md §8), so
# `make docker` cannot run here. It is written against the Makefile's own
# `build` recipe and §15's requirements; the first real `make docker` on a host
# with a daemon is what proves it. Treat a green build as unverified until then.
#
# Built via the Makefile's `docker` target, which supplies BASE_IMAGE (digest
# pinned), VERSION and COMMIT as --build-arg values and runs `docker buildx
# build` with --provenance/--sbom for multi-arch (linux/amd64, linux/arm64).

# The final base is digest-pinned. The default mirrors the Makefile's BASE_IMAGE
# so a bare `docker build -f deploy/Dockerfile .` still resolves the same image;
# `make docker` refuses a BASE_IMAGE that is not digest-pinned.
ARG BASE_IMAGE=gcr.io/distroless/static-debian12:nonroot@sha256:1b7b9f0f0e0a1d2155f531db587cc48ec26aaf97ab64364225f5bf18a054e66a

# ── Go toolchain, pinned to go.mod's version, copied into the builder ─────────
# Kept as its own stage so the builder is a Node image (which carries make, git
# and corepack) with the exact Go toolchain grafted in, rather than a Go image
# into which Node must be installed. Both stages run on the build platform.
FROM --platform=$BUILDPLATFORM golang:1.25.13-bookworm AS gotoolchain

# ── Build stage: runs the repo's OWN `make build` ────────────────────────────
# `make build` runs the SvelteKit build, the sync-embed step that mirrors the
# SPA into internal/web/spa, and `go build -trimpath -ldflags …` with the
# version stamp. Invoking make (rather than re-inventing the go build flags)
# keeps the in-image binary's --version honest and drift-free.
FROM --platform=$BUILDPLATFORM node:22-bookworm AS build
COPY --from=gotoolchain /usr/local/go /usr/local/go
ENV GOPATH=/go
ENV PATH=/usr/local/go/bin:/go/bin:$PATH
RUN corepack enable
WORKDIR /src

# Dependency layers first, keyed on the manifests, so a source-only change does
# not re-download the module graph or the pnpm store. `make build` fetches both
# again; these layers just prime them.
COPY go.mod go.sum ./
RUN go mod download
COPY web/package.json web/pnpm-lock.yaml ./web/
RUN cd web && pnpm install --frozen-lockfile

COPY . .

# Cross-compile for the requested target. CGO is off, so this is a pure
# GOOS/GOARCH switch with no C cross-toolchain. They enter make's environment
# and make re-exports them to the `go build` recipe.
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH

# VERSION and COMMIT come from the Makefile's own git describe / rev-parse on the
# host and are passed through, so the binary's --version does not lie in a build
# context that carries no .git. BUILD_DATE the recipe computes with `date -u`.
ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
RUN make build VERSION="$VERSION" COMMIT="$COMMIT"

# ── Final stage: the binary is the whole image ───────────────────────────────
FROM ${BASE_IMAGE}

# distroless/static:nonroot is UID 65532; state USER explicitly so the image is
# non-root from PID 1 regardless of the base's default (§15). There is no shell,
# no package manager and no entrypoint script: /config is a mounted volume the
# operator chowns to 65532:65532 once — the PUID/PGID chown-on-start dance is
# refused, because a shelled base starting as root to chown is the escape vector
# §15 rejects.
COPY --from=build /src/usarr /usr/local/bin/usarr

VOLUME ["/config"]
EXPOSE 8484
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/usarr"]

# HEALTHCHECK is deliberately absent. §15 asks for
# `HEALTHCHECK --start-period=60s` on /api/health/ready, but distroless/static
# has no shell and carries no wget/curl, and the binary has no `healthcheck`
# subcommand (it takes flags only and exits 1 on any positional argument), so a
# HEALTHCHECK here could not run without adding a probe binary — which would make
# the image more than the binary and a mounted volume. README's Quickstart
# records the same open question; until a static probe binary or a `usarr
# healthcheck` subcommand lands, the operator's Compose-side healthcheck (an HTTP
# GET on /api/health/live or /api/health/ready) is the probe.
