# Debian Deployment & Server Guide for Coke Studios Recovery

This document outlines deployment, service configuration, and production hosting instructions specifically targeted for **Debian 12 (Bookworm)** and modern Linux distributions.

---

## 1. Debian Package Installation

Update your package index and install the required runtimes and tools:
```bash
sudo apt-get update
sudo apt-get install -y \
    nodejs \
    npm \
    golang-go \
    git \
    curl \
    tar \
    ca-certificates
```

Verify your installed versions:
```bash
node -v      # Recommended: v18+ or v20+ LTS
go version   # Recommended: go 1.20+
git --version
```

---

## 2. Directory Setup & Permissions

Navigate to the project root and run `setup.sh`:
```bash
cd /opt/coke-studios  # or your workspace directory
chmod +x setup.sh start.sh stop.sh tools/bin/*
./setup.sh
```

---

## 3. Running Services on Debian

### Option A: Background Daemon via Shell Scripts
```bash
./start.sh
```
Outputs:
```text
  → Master Portal:  http://127.0.0.1:8000/
  → GoCoke Client:  http://127.0.0.1:8080/
  → CokePhase:      http://127.0.0.1:8081/
  → Music Mixer:    http://127.0.0.1:8082/
```
To stop:
```bash
./stop.sh
```

### Option B: Systemd Service Unit (Recommended for Production)
Install the provided `coke-studios.service` unit file into systemd:
```bash
sudo cp coke-studios.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now coke-studios.service
```
Check service status:
```bash
sudo systemctl status coke-studios.service
journalctl -u coke-studios.service -f
```

### Option C: Docker & Docker Compose
A Debian-based container setup is included in `Dockerfile` and `docker-compose.yml`:
```bash
docker compose up -d
```
Inspect container logs:
```bash
docker compose logs -f
```

---

## 4. Compiling GoCoke on Debian

GoCoke includes two native Linux binaries compiled for `linux/amd64` in `tools/bin/`:
- `gocoke-serve-linux`
- `gocoke-static-linux`

To recompile them natively from source on Debian:
```bash
cd sources/gocoke

# Compile GoCoke static file server
go build -o ../../tools/bin/gocoke-static-linux ./cmd/static

# Compile GoCoke proxy server
go build -o ../../tools/bin/gocoke-serve-linux ./cmd/serve
```

---

## 5. Nginx Reverse Proxy Configuration (Debian)

To serve all components on a standard single port (e.g. port 80/443) behind Nginx:
```bash
sudo apt-get install -y nginx
```

Create `/etc/nginx/sites-available/coke-studios`:
```nginx
server {
    listen 80;
    server_name cokemusic.local;

    # Master Portal Dashboard
    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # GoCoke WebAssembly Client
    location /gocoke/ {
        proxy_pass http://127.0.0.1:8080/;
        proxy_set_header Host $host;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # CokePhase HTML5 Engine
    location /cokephase/ {
        proxy_pass http://127.0.0.1:8081/;
        proxy_set_header Host $host;
    }

    # Standalone Music Mixer
    location /mixer/ {
        proxy_pass http://127.0.0.1:8082/;
        proxy_set_header Host $host;
    }
}
```

Enable the site and reload Nginx:
```bash
sudo ln -s /etc/nginx/sites-available/coke-studios /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

---

## 6. UFW Firewall Configuration
If using UFW on Debian:
```bash
sudo ufw allow 8000/tcp comment "Coke Studios Portal"
sudo ufw allow 8080/tcp comment "GoCoke Client"
sudo ufw allow 8081/tcp comment "CokePhase"
sudo ufw allow 8082/tcp comment "Music Mixer"
sudo ufw status
```
