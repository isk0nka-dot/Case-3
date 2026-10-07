# =============================================================================
#  Event Collector Service — Multi-stage Dockerfile
#  Produces a minimal, secure container image (~15MB) with just the binary.
# =============================================================================

# ---------------------------------------------------------------------------
#  Stage 1: Build
#  Compile the Go binary in a full build environment with all dependencies.
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS builder

# Install build dependencies.
# - git: required for version embedding via `git describe`
# - ca-certificates: required for TLS connections to external registries
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy dependency manifests first for better layer caching.
# This layer is invalidated only when go.mod or go.sum change,
# not on every source code change.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the full source tree.
COPY . .

# Build the binary with all optimizations:
# - CGO_ENABLED=0: static binary, no libc dependency
# - -trimpath: reproducible builds, strips local filesystem paths
# - -s -w: strip debug symbols and DWARF tables for smaller binary
# - Version and build time are injected via ldflags
ARG VERSION=dev
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/event-collector \
    ./cmd/server

# ---------------------------------------------------------------------------
#  Stage 2: Runtime
#  Minimal Alpine image with only the compiled binary and CA certificates.
# ---------------------------------------------------------------------------
FROM alpine:3.20

# Install runtime dependencies:
# - ca-certificates: TLS connections to Kafka, ClickHouse, external services
# - tzdata: timezone support for correct timestamp handling
RUN apk add --no-cache ca-certificates tzdata

# Create a non-root user and group for running the service.
# Running as root inside containers is a security anti-pattern.
# The 'ping' group (GID 999) matches Docker socket ownership on most hosts.
RUN addgroup -S appgroup && adduser -S appuser -G appgroup \
    && addgroup -g 999 -S docker || true \
    && addgroup appuser docker || true

# Create directories for config and data with correct ownership.
RUN mkdir -p /app/config && chown -R appuser:appgroup /app

WORKDIR /app

# Copy the compiled binary from the build stage.
COPY --from=builder /build/bin/event-collector /app/event-collector

# Ensure the binary is executable.
RUN chmod +x /app/event-collector

# Switch to non-root user.
USER appuser

# Expose service ports:
# - 50051: gRPC (primary ingestion endpoint)
# - 8080:  HTTP  (health checks, metrics, readiness probes)
EXPOSE 50051 8080

# Health check: hit the HTTP health endpoint every 30 seconds.
# The service must respond within 5 seconds or be marked unhealthy.
# After 3 consecutive failures, the container is restarted.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Entrypoint is the compiled binary.
ENTRYPOINT ["/app/event-collector"]
