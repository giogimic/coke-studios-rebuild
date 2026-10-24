#!/usr/bin/env bash
# ==============================================================================
# Coke Studios 1:1 Complete Rebuild - Single-File Setup & Auto-Updater
# Target OS: Debian 12 (Bookworm) / Ubuntu / Linux
# Default Launch Port: 24902
# Scalable Database: MariaDB 11
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

PORT="${PORT:-24902}"
DB_NAME="${DB_NAME:-coke_studios}"
DB_USER="${DB_USER:-coke}"
DB_PASS="${DB_PASS:-coke_secret}"
DB_ROOT_PASSWORD="${DB_ROOT_PASSWORD:-coke_root_secret}"

echo "===================================================================="
echo "    COKE STUDIOS & COKE MUSIC 1:1 REBUILD - DEBIAN DOCKER SETUP"
echo "===================================================================="
echo " Mode: $( [ "${1:-}" = "--update" ] || [ "${1:-}" = "update" ] && echo "UPDATE & REBUILD" || echo "FRESH LAUNCH" )"
echo " Ingress Port: ${PORT}"
echo " Database:     MariaDB 11 (Scalable Docker Cluster)"
echo " Workspace:    ${SCRIPT_DIR}"
echo "===================================================================="

# Helper: check for docker command
check_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        echo "[!] Docker is not installed on this system."
        if [ -f /etc/debian_version ]; then
            echo "[*] Detected Debian/Ubuntu. Installing Docker..."
            sudo apt-get update
            sudo apt-get install -y ca-certificates curl gnupg
            sudo install -m 0755 -d /etc/apt/keyrings
            curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg || true
            sudo chmod a+r /etc/apt/keyrings/docker.gpg || true
            echo "deb [arch=\"$(dpkg --print-architecture)\" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian $(. /etc/os-release && echo \"$VERSION_CODENAME\") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
            sudo apt-get update
            sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
            sudo systemctl enable --now docker
        else
            echo "[ERROR] Please install Docker and Docker Compose before running this script."
            exit 1
        fi
    fi

    # Determine docker compose syntax (plugin or standalone)
    if docker compose version >/dev/null 2>&1; then
        DOCKER_COMPOSE="docker compose"
    elif command -v docker-compose >/dev/null 2>&1; then
        DOCKER_COMPOSE="docker-compose"
    else
        echo "[ERROR] Neither 'docker compose' nor 'docker-compose' found. Please install docker-compose-plugin."
        exit 1
    fi
}

check_docker

# Generate .env if missing
if [ ! -f .env ]; then
    echo "[*] Generating .env configuration..."
    cat > .env <<EOF
PORT=${PORT}
DB_NAME=${DB_NAME}
DB_USER=${DB_USER}
DB_PASS=${DB_PASS}
DB_ROOT_PASSWORD=${DB_ROOT_PASSWORD}
DATABASE_URL=${DB_USER}:${DB_PASS}@tcp(mariadb:3306)/${DB_NAME}?parseTime=true
EOF
fi

# Ensure Linux binaries have executable permissions
chmod +x start.sh stop.sh tools/bin/* 2>/dev/null || true

# Handle --update / update argument
if [ "${1:-}" = "--update" ] || [ "${1:-}" = "update" ]; then
    echo ""
    echo "[*] STEP 1/4: Pulling latest changes from Git repository..."
    if [ -d .git ]; then
        git pull origin master || git pull || echo "[WARN] Git pull skipped or encountered merge warnings."
    else
        echo "[WARN] No .git directory found; skipping git pull."
    fi

    echo ""
    echo "[*] STEP 2/4: Rebuilding Docker images..."
    ${DOCKER_COMPOSE} build --pull

    echo ""
    echo "[*] STEP 3/4: Restarting containers with MariaDB persistence..."
    ${DOCKER_COMPOSE} down --remove-orphans || true
    ${DOCKER_COMPOSE} up -d

    echo ""
    echo "[*] STEP 4/4: Applying database schema migrations..."
    # coke-server automatically runs idempotent migrations on startup
else
    echo ""
    echo "[*] Launching MariaDB & Coke Studios via Docker Compose..."
    ${DOCKER_COMPOSE} up -d --build
fi

# Wait for service health check
echo ""
echo "[*] Waiting for Coke Studios on http://127.0.0.1:${PORT}/health..."
MAX_ATTEMPTS=30
ATTEMPT=0
READY=0

while [ ${ATTEMPT} -lt ${MAX_ATTEMPTS} ]; do
    ATTEMPT=$((ATTEMPT + 1))
    if curl -fs "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 2
    echo -n "."
done
echo ""

if [ ${READY} -eq 1 ]; then
    HEALTH_DATA=$(curl -s "http://127.0.0.1:${PORT}/health" || echo "{}")
    echo "===================================================================="
    echo "  SUCCESS: COKE STUDIOS IS LIVE AND HEALTHY!"
    echo "===================================================================="
    echo "  Official MyCoke Game Portal: http://127.0.0.1:${PORT}/"
    echo "  Direct Game Client:         http://127.0.0.1:${PORT}/game/?user=Adam"
    echo "  Music Mixer Studio:         http://127.0.0.1:${PORT}/mixer/"
    echo "  Health Status:              ${HEALTH_DATA}"
    echo "===================================================================="
    echo "  To update at any time with zero data loss, simply run:"
    echo "  ./setup.sh --update"
    echo "===================================================================="
else
    echo "[WARN] Server is starting up. Check logs with:"
    echo "  ${DOCKER_COMPOSE} logs -f"
    echo "  Then visit: http://127.0.0.1:${PORT}/"
fi
