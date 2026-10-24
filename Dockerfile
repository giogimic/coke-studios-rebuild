FROM debian:bookworm-slim

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy application files
COPY . /app

# Ensure proper permissions
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
