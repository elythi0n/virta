# Builds a single self-contained virtad image: the web UI is built in a Node stage, staged
# into the Go source tree, then compiled into the binary via go:embed so the final image
# serves the full app at http://host:8344/ with no separate frontend process.
#
# Three build stages:
#   ui      — Node 22, builds the Vite SPA (app + overlay pages)
#   go-deps — Go 1.26, caches module downloads separately for layer reuse
#   build   — combines the UI artefacts with the Go source and produces the binary
#   runtime — minimal distroless image, ~20 MB final size

# ── Stage 1: build the web UI ────────────────────────────────────────────────
FROM node:22-alpine AS ui
WORKDIR /src/frontends

# Install workspace dependencies — copy only the package manifests first so
# this expensive layer is cached as long as dependencies don't change.
COPY frontends/package.json frontends/package-lock.json ./
COPY frontends/ui-kit/package.json ./ui-kit/
COPY frontends/feed-core/package.json ./feed-core/
COPY frontends/web/package.json ./web/
RUN npm ci --workspace ui-kit --workspace feed-core --workspace web

# Copy source and build. `npm run build` inside web produces dist/ with index.html,
# overlay.html, and all assets; the Go embed picks these up in the next stage.
COPY frontends/ ./
# build:docker skips tsc --noEmit (type-checking runs in CI, not in the image build).
RUN cd web && npm run build:docker

# ── Stage 2: cache Go module downloads (layer reuse) ─────────────────────────
FROM golang:1.26 AS go-deps
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# ── Stage 3: build the virtad binary with the UI embedded ─────────────────────
FROM golang:1.26 AS build
WORKDIR /src
# Bring in the cached module layer.
COPY --from=go-deps /go/pkg/mod /go/pkg/mod
COPY go.mod go.sum ./
# Copy the full Go source.
COPY . .
# Stage the built UI into the embed directory so go:embed picks it up.
COPY --from=ui /src/frontends/web/dist ./internal/webui/dist/
# Build the daemon binary (CGO off → static binary, works in distroless).
RUN CGO_ENABLED=0 go build \
      -ldflags "-s -w -X github.com/elythi0n/virta/internal/buildinfo.Version=$(git describe --tags --always 2>/dev/null || echo dev)" \
      -o /out/virtad ./cmd/virtad
# Pre-create the data tree owned by the distroless "nonroot" uid (65532).
RUN mkdir -p /data/cache /data/run && chown -R 65532:65532 /data

# ── Stage 4: minimal runtime image ───────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/virtad /usr/local/bin/virtad
COPY --from=build --chown=65532:65532 /data /data

# Defaults for container deployments. Override in docker-compose.yml or with -e flags.
ENV VIRTA_ADDR=0.0.0.0:8344 \
    VIRTA_DATA_DIR=/data \
    VIRTA_CACHE_DIR=/data/cache \
    VIRTA_RUNTIME_DIR=/data/run

VOLUME ["/data"]
EXPOSE 8344
USER nonroot
HEALTHCHECK --interval=15s --timeout=5s --retries=3 \
  CMD ["/usr/local/bin/virtad", "version"]
ENTRYPOINT ["/usr/local/bin/virtad"]
