# Debian Deployment & Server Guide for Coke Studios (Go Recreation)

This document provides production hosting, service configuration, and deployment instructions specifically targeted for **Debian 12 (Bookworm)** and Linux systems.

---

## 1. System Requirements & Architecture

The authoritative Coke Studios server runs as a single, unified Go executable on Debian 12. It serves the 760px authentic website shell, hosts the Go/Ebitengine WebAssembly client (`main.wasm`), processes REST/AMF API endpoints, and terminates realtime WebSocket connections on port `24902`.

- **Operating System:** Debian 12 (Bookworm) x86_64
- **Primary Service Port:** `24902` (HTTP, WebAssembly, and WebSocket `/ws/realtime`)
- **Server Executable:** `tools/bin/coke-server-linux` (pure Go, CGO-free, headless Linux ELF)
- **Database Backend:** MariaDB 11 (production) or SQLite 3 (embedded fallback)
- **Systemd Unit:** `/etc/systemd/system/coke-studios.service`

---

## 2. Package Installation on Debian 12

Update package indices and install build tools and dependencies:
```bash
sudo apt-get update
sudo apt-get install -y \
    golang-go \
    mariadb-server \
    curl \
    git \
    ca-certificates
```

Verify the Go toolchain version:
```bash
go version   # Go 1.20+ recommended
```

---

## 3. Building from Source on Debian

To compile the latest WebAssembly game client and native Linux server binary:
```bash
cd /opt/coke-studios

# Run the Debian build script
chmod +x tools/build-go.sh
./tools/build-go.sh
```

This generates:
- `sources/gocoke/web/main.wasm` (Go/Ebitengine WebAssembly client)
- `tools/bin/coke-server-linux` (Debian Linux x86_64 executable)

---

## 4. Production Service Management (Systemd)

The project includes an authentic production systemd service definition in `coke-studios.service`:

```ini
[Unit]
Description=Coke Music / Coke Studios Authoritative Go Server (Debian 12 First)
After=network.target mariadb.service

[Service]
Type=simple
User=coke
WorkingDirectory=/opt/coke-studios
ExecStart=/opt/coke-studios/tools/bin/coke-server-linux -addr 0.0.0.0:24902 -root /opt/coke-studios/sources/gocoke/web
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
Environment=PORT=24902
Environment=DB_DRIVER=mysql
Environment=DATABASE_URL=coke:coke_secret@tcp(127.0.0.1:3306)/coke_studios?parseTime=true

[Install]
WantedBy=multi-user.target
```

### Automated Turnkey Deployment & Updates
You can run the automated deployment script to perform package installation, MariaDB user creation, binary and asset synchronization, and systemd service installation in a single step:

```bash
# Fresh installation
sudo chmod +x tools/deploy-debian.sh
sudo ./tools/deploy-debian.sh

# Update running server with latest code (zero data loss)
sudo ./tools/deploy-debian.sh --update

# Update running server and pull latest changes from git
sudo ./tools/deploy-debian.sh --pull
```

The updater gracefully stops the service (alerting connected players and draining active sockets), backs up the existing binary, recompiles the latest Go server and WebAssembly client, replaces files without `ETXTBSY` text file busy errors, restarts the service, and verifies health status. If a health check fails, it automatically rolls back to the previous stable binary.

### Manual Installation Steps
```bash
# 1. Create service user if needed
sudo useradd -r -s /bin/false coke || true
sudo chown -R coke:coke /opt/coke-studios

# 2. Install unit file
sudo cp coke-studios.service /etc/systemd/system/
sudo systemctl daemon-reload

# 3. Enable and start service
sudo systemctl enable --now coke-studios.service

# 4. Inspect status and logs
sudo systemctl status coke-studios.service
journalctl -u coke-studios.service -f
```

---

## 5. Standalone Shell Controls

For quick testing or maintenance without systemd:

```bash
# Start background daemon
./start.sh

# Start in foreground for debugging
./start.sh -f

# Stop server daemon
./stop.sh
```

---

## 6. MariaDB Database Configuration

Configure a local MariaDB instance for production durability:

```sql
CREATE DATABASE IF NOT EXISTS coke_studios CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'coke'@'localhost' IDENTIFIED BY 'coke_secret';
GRANT ALL PRIVILEGES ON coke_studios.* TO 'coke'@'localhost';
FLUSH PRIVILEGES;
```

The Go server automatically applies idempotent schema migrations (`s.migrate()`) on startup, establishing tables for:
- `denizens` (accounts, password hashes, mission, figure defs, Decibels, V-Coins)
- `studios` (rooms, layouts, wallpaper, floor, doorbell)
- `backpack_items` (furniture inventory, CD items, mint properties)
- `placed_furniture` (room coordinates $x, y, z$, rotation $0,2,4,6$, state, height $ho$)
- `messenger_friends` & `messenger_messages` (social relationships and offline mail)
- `user_songs` & `song_votes` (OddCast EDL compositions, performance records, Top 40 charts)
- `vegosan_streaks` & `daily_minigame_earnings` (minigame caps and prize ladders)

---

## 7. Nginx Reverse Proxy (Optional SSL / Port 80/443 Termination)

To terminate HTTPS and proxy to port `24902`:

```nginx
server {
    listen 80;
    server_name cokemusic.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name cokemusic.example.com;

    ssl_certificate /etc/letsencrypt/live/cokemusic.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/cokemusic.example.com/privkey.pem;

    # Gzip / Brotli compression for WebAssembly
    gzip on;
    gzip_types application/wasm text/plain text/css application/json application/javascript;

    location / {
        proxy_pass http://127.0.0.1:24902;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support for realtime game engine
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
}
```

---

## 8. UFW Firewall Configuration

If UFW is enabled on Debian:
```bash
sudo ufw allow 24902/tcp comment "Coke Studios Game & Web Ingress"
sudo ufw allow 80/tcp comment "HTTP Web"
sudo ufw allow 443/tcp comment "HTTPS Secure"
sudo ufw status
```
