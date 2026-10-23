FROM debian:bookworm-slim

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    nodejs \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy application files
COPY . /app

# Ensure proper permissions
RUN chmod +x /app/*.sh /app/tools/bin/* 2>/dev/null || true

ENV PORT=24902

# Expose ports
# 24902: Master Recovery Portal & Gateway (Default Entry Point)
# 8080: Standalone GoCoke Backend (Direct WebSocket & AMF)
# 8081: CokePhase HTML5 Engine
# 8082: CSMusicMixer Standalone
EXPOSE 24902 8080 8081 8082

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://127.0.0.1:24902/health || exit 1

CMD ["node", "tools/serve-local.cjs"]
