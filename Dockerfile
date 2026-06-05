# Builds a tiny, static virtad image for running Virta on a server. The build is CGO-free,
# so the final stage can be distroless with no shared libraries.
#
# Server runs configure the daemon through the environment (see docker-compose.yml):
#   VIRTA_ADDR    bind address (e.g. 0.0.0.0:8344 to accept connections, not just loopback)
#   VIRTA_TOKEN   fixed bearer token clients authenticate with
#   VIRTA_STORAGE storage backend (sqlite default)
#   VIRTA_DATA_DIR / VIRTA_CACHE_DIR  persisted under a mounted volume

FROM golang:1.26 AS build
WORKDIR /src
# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/virtad ./cmd/virtad
# Pre-create the data tree so it's owned by the nonroot user in the final image (distroless
# has no shell to mkdir/chown at runtime). 65532 is the distroless "nonroot" uid/gid.
RUN mkdir -p /data/cache /data/run

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/virtad /usr/local/bin/virtad
COPY --from=build --chown=65532:65532 /data /data
# Defaults suited to a container: listen on all interfaces, keep data on a volume. There is
# no OS keychain in a container, so secrets fall back to the encrypted file vault under the
# data dir automatically.
ENV VIRTA_ADDR=0.0.0.0:8344 \
    VIRTA_DATA_DIR=/data \
    VIRTA_CACHE_DIR=/data/cache \
    VIRTA_RUNTIME_DIR=/data/run
VOLUME ["/data"]
EXPOSE 8344
USER nonroot
ENTRYPOINT ["/usr/local/bin/virtad"]
