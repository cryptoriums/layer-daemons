# syntax=docker/dockerfile:1
# Dockerfile for the wallet-balancer tool. Runs next to the signer and signs with
# a LOCAL file key (it does not talk to the signer container), so no remote-signer
# api / vendor-api is needed. Build context is the layer-daemons repo root.

### Build stage
FROM golang:1.24-bookworm AS builder

WORKDIR /src
ENV GOTOOLCHAIN=auto CGO_ENABLED=0

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /tmp/wallet-balancer ./cmd/wallet-balancer

### Runtime stage
FROM debian:bookworm-slim

# ca-certificates: TLS to the Binance API and the layer node RPC.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /tmp/wallet-balancer /usr/local/bin/wallet-balancer

# Prometheus metrics port (matches metrics_addr default :9095).
EXPOSE 9095

# Mount the config and the keyring dir at runtime, e.g.:
#   docker run -v /opt/wb/config.yaml:/config/wallet-balancer.yaml:ro \
#              -v /opt/wb/keys:/keys:ro wallet-balancer:latest
# (keyring_dir + key_password_file in the config point at /keys)
ENTRYPOINT ["/usr/local/bin/wallet-balancer"]
CMD ["-config", "/config/wallet-balancer.yaml"]
