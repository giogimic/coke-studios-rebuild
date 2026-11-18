# Stage 1: Build & Packaging (Debian 12 Bookworm with Go toolchain)
FROM golang:1.24-bookworm AS builder

WORKDIR /src
COPY . .

# Compile static Linux binary if not already pre-compiled
RUN if [ ! -f /src/tools/bin/coke-server-linux ]; then \
      echo "Building coke-server-linux inside container..." && \
      cd /src/sources/gocoke && \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /src/tools/bin/coke-server-linux ./cmd/server; \
    fi

# Stage 2: Production Runtime (Debian 12 Bookworm Slim)
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /src /app
RUN chmod +x /app/*.sh /app/tools/bin/* 2>/dev/null || true

ENV PORT=24902
ENV HOST=0.0.0.0

# Expose ports
# 24902: Authoritative GoCoke Server & Portal (Canonical Entry Point)
# 8080: Standalone Direct Port
EXPOSE 24902 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://127.0.0.1:24902/health || exit 1

ENTRYPOINT ["/app/tools/bin/coke-server-linux"]
CMD ["-root", "/app/sources/gocoke/web"]
