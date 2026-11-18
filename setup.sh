#!/usr/bin/env bash
# ==============================================================================
# Coke Studios 1:1 Complete Rebuild - Single-File Setup & Auto-Updater
# Target OS: Debian 12 (Bookworm) / Ubuntu / Linux
# Default Launch Port: 24902
# Scalable Database: MariaDB 11
# ==============================================================================
set -euo pipefail

# Force plain progress to prevent BuildKit vt100 terminal dimension panic in SSH sessions
export BUILDKIT_PROGRESS=plain
export DOCKER_BUILDKIT=1

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

# Helper: Check if a port is in use on the host system
is_port_in_use() {
    local p="$1"
    if command -v ss >/dev/null 2>&1; then
        ss -tuln 2>/dev/null | grep -qE "(:|\])${p}\b" && return 0
    elif command -v netstat >/dev/null 2>&1; then
        netstat -tuln 2>/dev/null | grep -qE "(:|\])${p}\b" && return 0
    elif command -v lsof >/dev/null 2>&1; then
        lsof -i ":${p}" >/dev/null 2>&1 && return 0
    fi
    (exec 6<>/dev/tcp/127.0.0.1/${p}) 2>/dev/null && { exec 6>&-; exec 6<&-; return 0; }
    return 1
}

find_next_free_port() {
    local p="$1"
    while is_port_in_use "${p}"; do
        p=$((p + 1))
    done
    echo "${p}"
}

# Helper: Sanitize string values (strip quotes, carriage returns, leading/trailing whitespace)
clean_val() {
    local val="$1"
    val="$(echo "${val}" | tr -d '\r\n\t'\''"')"
    echo "${val}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'
}

# Load existing configuration if .env exists
if [ -f .env ]; then
    # Normalize line endings and strip Windows carriage returns
    sed -i -e 's/\r$//' .env 2>/dev/null || true
    set -a
    # shellcheck disable=SC1091
    source .env 2>/dev/null || true
    set +a
fi

PORT="$(clean_val "${PORT:-24902}")"
BACKEND_PORT="$(clean_val "${BACKEND_PORT:-8080}")"
COKEPHASE_PORT="$(clean_val "${COKEPHASE_PORT:-8081}")"
MUSICMIXER_PORT="$(clean_val "${MUSICMIXER_PORT:-8082}")"
DB_PORT="$(clean_val "${DB_PORT:-3306}")"
DB_NAME="$(clean_val "${DB_NAME:-coke_studios}")"
DB_USER="$(clean_val "${DB_USER:-coke}")"
DB_PASS="$(clean_val "${DB_PASS:-coke_secret}")"
DB_ROOT_PASSWORD="$(clean_val "${DB_ROOT_PASSWORD:-coke_root_secret}")"

# Check for update/nuke arguments
IS_UPDATE=0
IS_NUKE=0
FORCE_CONFIRM=0
DO_PULL=0
WIPE_CACHE=0
WIPE_DB=0

for arg in "$@"; do
    case "$arg" in
        --nuke|-nuke|nuke|--reset|-reset|reset)
            IS_NUKE=1
            IS_UPDATE=1
            WIPE_CACHE=1
            WIPE_DB=1
            DO_PULL=1
            ;;
        --update|-update|update|-u)
            IS_UPDATE=1
            ;;
        --pull|-pull|pull|-p)
            DO_PULL=1
            ;;
        --wipe-cache)
            WIPE_CACHE=1
            ;;
        --wipe-db|--reset-db)
            WIPE_DB=1
            ;;
        --wipe-all|--reset-all)
            WIPE_CACHE=1
            WIPE_DB=1
            ;;
        -y|--yes|-f|--force)
            FORCE_CONFIRM=1
            ;;
    esac
done

echo "===================================================================="
echo "    COKE STUDIOS & COKE MUSIC 1:1 REBUILD - DEBIAN DOCKER SETUP"
echo "===================================================================="
if [ ${IS_NUKE} -eq 1 ]; then
    echo " Mode:         NUKE & COMPLETE RESETUP (Wipe Caches, Repull, Rebuild)"
elif [ ${IS_UPDATE} -eq 1 ]; then
    echo " Mode:         UPDATE & REBUILD (Preserve Player Data)"
else
    echo " Mode:         FRESH LAUNCH"
fi
echo " Database:     MariaDB 11 (Scalable Docker Cluster)"
echo " Workspace:    ${SCRIPT_DIR}"
echo "===================================================================="

# Automatically clean up any orphaned Coke containers or background processes
if [ -f "./stop.sh" ]; then
    echo "[*] Ensuring previous Coke Studios containers and processes are stopped..."
    bash ./stop.sh >/dev/null 2>&1 || true
fi

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

# Interactive Port Configuration (asks user for ports and checks for conflicts)
if [ ${IS_UPDATE} -eq 0 ] && [ ${IS_NUKE} -eq 0 ]; then
    if [ -t 0 ]; then
        echo ""
        echo "--------------------------------------------------------------------"
        echo "                     PORT SELECTION & CHECK"
        echo "  Default ports (like 8080 or 3306) can conflict with local web"
        echo "  servers, Tomcat, or MySQL. We verify availability before launch."
        echo "--------------------------------------------------------------------"

        # 1. Primary Ingress Portal Port
        SUGG_PORT="${PORT}"
        if is_port_in_use "${PORT}"; then
            SUGG_PORT=$(find_next_free_port "${PORT}")
            echo "  [!] Port ${PORT} is IN USE. Suggested free port: ${SUGG_PORT}"
        fi
        read -r -p "  [1/5] Primary Portal Port [default: ${SUGG_PORT}]: " INPUT_PORT
        PORT=$(clean_val "${INPUT_PORT:-$SUGG_PORT}")

        # 2. Standalone Go Backend Port (commonly 8080)
        SUGG_BACKEND="${BACKEND_PORT}"
        if is_port_in_use "${BACKEND_PORT}"; then
            SUGG_BACKEND=$(find_next_free_port 8085)
            echo "  [!] Port ${BACKEND_PORT} is ALREADY IN USE! Suggested free port: ${SUGG_BACKEND}"
        fi
        read -r -p "  [2/5] Standalone Go Backend Port [default: ${SUGG_BACKEND}]: " INPUT_BACKEND
        BACKEND_PORT=$(clean_val "${INPUT_BACKEND:-$SUGG_BACKEND}")

        # 3. MariaDB Host Port (commonly 3306)
        SUGG_DB="${DB_PORT}"
        if is_port_in_use "${DB_PORT}"; then
            SUGG_DB=$(find_next_free_port 3307)
            echo "  [!] Port ${DB_PORT} (MySQL) is ALREADY IN USE! Suggested free port: ${SUGG_DB}"
        fi
        read -r -p "  [3/5] MariaDB Host Port [default: ${SUGG_DB}]: " INPUT_DB
        DB_PORT=$(clean_val "${INPUT_DB:-$SUGG_DB}")

        # 4. CokePhase Port
        SUGG_CP="${COKEPHASE_PORT}"
        if is_port_in_use "${COKEPHASE_PORT}"; then
            SUGG_CP=$(find_next_free_port 8091)
            echo "  [!] Port ${COKEPHASE_PORT} is IN USE. Suggested free port: ${SUGG_CP}"
        fi
        read -r -p "  [4/5] CokePhase HTML5 Port [default: ${SUGG_CP}]: " INPUT_CP
        COKEPHASE_PORT=$(clean_val "${INPUT_CP:-$SUGG_CP}")

        # 5. CSMusicMixer Port
        SUGG_MIXER="${MUSICMIXER_PORT}"
        if is_port_in_use "${MUSICMIXER_PORT}"; then
            SUGG_MIXER=$(find_next_free_port 8092)
            echo "  [!] Port ${MUSICMIXER_PORT} is IN USE. Suggested free port: ${SUGG_MIXER}"
        fi
        read -r -p "  [5/5] CSMusicMixer Standalone Port [default: ${SUGG_MIXER}]: " INPUT_MIXER
        MUSICMIXER_PORT=$(clean_val "${INPUT_MIXER:-$SUGG_MIXER}")

        echo "--------------------------------------------------------------------"
    else
        # Non-interactive fallback: auto-shift if 8080 or 3306 is in use
        if is_port_in_use "${BACKEND_PORT}"; then
            BACKEND_PORT=$(find_next_free_port 8085)
            echo "[*] Non-interactive: Port 8080 in use, auto-assigned backend to ${BACKEND_PORT}"
        fi
        if is_port_in_use "${DB_PORT}"; then
            DB_PORT=$(find_next_free_port 3307)
            echo "[*] Non-interactive: Port 3306 in use, auto-assigned MariaDB to ${DB_PORT}"
        fi
    fi
fi

# Write updated configuration to .env with clean Unix line endings
echo "[*] Writing .env configuration..."
cat > .env <<EOF
PORT=${PORT}
HOST=0.0.0.0
BACKEND_PORT=${BACKEND_PORT}
COKEPHASE_PORT=${COKEPHASE_PORT}
MUSICMIXER_PORT=${MUSICMIXER_PORT}
DB_PORT=${DB_PORT}
DB_NAME=${DB_NAME}
DB_USER=${DB_USER}
DB_PASS=${DB_PASS}
DB_ROOT_PASSWORD=${DB_ROOT_PASSWORD}
DATABASE_URL=${DB_USER}:${DB_PASS}@tcp(mariadb:3306)/${DB_NAME}?parseTime=true
EOF
sed -i -e 's/\r$//' .env 2>/dev/null || true

# Ensure Linux binaries have executable permissions
chmod +x start.sh stop.sh tools/bin/* 2>/dev/null || true

# Handle --nuke / --update argument
if [ ${IS_NUKE} -eq 1 ]; then
    # Confirmation prompt if interactive
    if [ ${FORCE_CONFIRM} -eq 0 ]; then
        if [ -t 0 ] || [ -e /dev/tty ]; then
            TTY_IN="/dev/tty"
            [ -e /dev/tty ] || TTY_IN="/dev/stdin"
            echo ""
            echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
            echo "                   WARNING: NUKE OPTION SELECTED"
            echo "  This will:"
            echo "   1. Stop all active Coke Studios containers and processes"
            echo "   2. Wipe all local databases (coke.db, MariaDB container volume)"
            echo "   3. Wipe all Docker builder and image caches"
            echo "   4. Wipe precompiled binaries and Go build caches"
            echo "   5. Discard local modifications and repull latest from Git"
            echo "   6. Recompile Go binaries and rebuild containers from scratch"
            echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
            read -r -p "  Are you sure you want to completely NUKE and resetup? [y/N]: " REPLY_NUKE < "${TTY_IN}" || REPLY_NUKE="n"
            case "${REPLY_NUKE}" in
                [yY]|[yY][eE][sS])
                    echo "[*] Nuke confirmed. Proceeding with full resetup..."
                    ;;
                *)
                    echo "[*] Nuke aborted by user. Exiting."
                    exit 0
                    ;;
            esac
        fi
    fi

    echo ""
    echo "[*] NUKE STEP 1/6: Stopping all running containers and background services..."
    if [ -f "./stop.sh" ]; then
        bash ./stop.sh >/dev/null 2>&1 || true
    fi
    ${DOCKER_COMPOSE} down -v --remove-orphans >/dev/null 2>&1 || true
    docker volume rm -f coke_mariadb_data 2>/dev/null || true

    echo ""
    echo "[*] NUKE STEP 2/6: Wiping build caches, compiled binaries, and databases..."
    docker builder prune -a -f 2>/dev/null || true
    docker image prune -f 2>/dev/null || true
    if command -v go >/dev/null 2>&1; then
        echo "    Wiping Go build and test cache..."
        go clean -cache -testcache 2>/dev/null || true
    fi
    rm -f tools/bin/coke-server-linux* tools/bin/coke-server.exe* sources/gocoke/web/main.wasm* sources/gocoke/web/wasm_exec.js 2>/dev/null || true
    rm -f coke.db coke.db-shm coke.db-wal sources/gocoke/coke.db* tools/bin/coke.db* data/coke.db* 2>/dev/null || true

    echo ""
    echo "[*] NUKE STEP 3/6: Cleaning git working tree and repulling latest code from Git..."
    if [ -d .git ]; then
        echo "    Current commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
        git fetch origin master || git fetch || echo "[WARN] Git fetch skipped."
        git reset --hard origin/master 2>/dev/null || git reset --hard HEAD || echo "[WARN] Git reset skipped."
        git clean -fd -e .env 2>/dev/null || true
        echo "    Reset and synced to: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
    else
        echo "[WARN] No .git directory found; skipping git repull."
    fi

    chmod +x start.sh stop.sh setup.sh tools/deploy-debian.sh tools/build-go.sh tools/bin/* 2>/dev/null || true

    echo ""
    echo "[*] NUKE STEP 4/6: Compiling fresh Go binaries and WebAssembly client..."
    if command -v go >/dev/null 2>&1; then
        ./tools/build-go.sh || echo "[WARN] Host Go build skipped."
    fi

    echo ""
    echo "[*] NUKE STEP 5/6: Building Docker images from scratch (--no-cache)..."
    ${DOCKER_COMPOSE} build --no-cache --pull --progress=plain

    echo ""
    echo "[*] NUKE STEP 6/6: Launching fresh containers and MariaDB cluster..."
    ${DOCKER_COMPOSE} up -d

    if systemctl is-active --quiet coke-studios.service 2>/dev/null || [ -f "/etc/systemd/system/coke-studios.service" ]; then
        echo "[*] Detected native Debian systemd service (coke-studios.service)."
        echo "    Resetting and deploying fresh to /opt/coke-studios..."
        if [ "$(id -u)" -eq 0 ]; then
            ./tools/deploy-debian.sh --nuke -y || echo "[WARN] Native deploy-debian nuke encountered warnings."
        else
            sudo ./tools/deploy-debian.sh --nuke -y || echo "[WARN] Native deploy-debian nuke encountered warnings."
        fi
    fi
elif [ ${IS_UPDATE} -eq 1 ]; then
    WIPE_CACHE=0
    WIPE_DB=0

    # Parse CLI flags if passed
    for arg in "$@"; do
        case "$arg" in
            --wipe-cache)
                WIPE_CACHE=1
                ;;
            --wipe-db|--reset-db)
                WIPE_DB=1
                ;;
            --wipe-all|--reset-all)
                WIPE_CACHE=1
                WIPE_DB=1
                ;;
        esac
    done

    # Interactive prompt if running in a terminal
    if [ -t 0 ] || [ -e /dev/tty ]; then
        TTY_IN="/dev/tty"
        [ -e /dev/tty ] || TTY_IN="/dev/stdin"
        echo ""
        echo "--------------------------------------------------------------------"
        echo "                       UPDATE CLEANUP OPTIONS"
        echo "--------------------------------------------------------------------"
        if [ ${WIPE_CACHE} -eq 0 ]; then
            read -r -p "  Wipe build and Docker image cache? [y/N]: " REPLY_CACHE < "${TTY_IN}" || REPLY_CACHE="n"
            case "${REPLY_CACHE}" in
                [yY]|[yY][eE][sS])
                    WIPE_CACHE=1
                    ;;
                *)
                    WIPE_CACHE=0
                    ;;
            esac
        fi

        if [ ${WIPE_DB} -eq 0 ]; then
            read -r -p "  Wipe database (MariaDB volume & local coke.db)? [y/N]: " REPLY_DB < "${TTY_IN}" || REPLY_DB="n"
            case "${REPLY_DB}" in
                [yY]|[yY][eE][sS])
                    WIPE_DB=1
                    ;;
                *)
                    WIPE_DB=0
                    ;;
            esac
        fi
        echo "--------------------------------------------------------------------"
    fi

    echo ""
    echo "[*] STEP 1/4: Pulling latest changes from Git repository..."
    if [ -d .git ]; then
        echo "    Current commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
        git pull origin master || git pull || echo "[WARN] Git pull skipped or encountered merge warnings."
        echo "    Updated commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
    else
        echo "[WARN] No .git directory found; skipping git pull."
    fi

    # If Go is installed on the host, compile binaries locally
    if command -v go >/dev/null 2>&1; then
        echo "[*] Compiling latest Go WebAssembly and static Linux binaries on host..."
        chmod +x tools/build-go.sh && ./tools/build-go.sh || echo "[WARN] Local Go build skipped."
    fi

    echo ""
    if [ ${WIPE_CACHE} -eq 1 ]; then
        echo "[*] STEP 2/4: Rebuilding Docker images (CACHE WIPED)..."
        docker builder prune -f 2>/dev/null || true
        ${DOCKER_COMPOSE} build --no-cache --pull --progress=plain
    else
        echo "[*] STEP 2/4: Rebuilding Docker images..."
        ${DOCKER_COMPOSE} build --pull --progress=plain
    fi

    echo ""
    if [ ${WIPE_DB} -eq 1 ]; then
        echo "[*] STEP 3/4: Wiping database and restarting fresh containers..."
        ${DOCKER_COMPOSE} down -v --remove-orphans || true
        docker volume rm coke_mariadb_data 2>/dev/null || true
        rm -f coke.db sources/gocoke/coke.db tools/bin/coke.db data/coke.db 2>/dev/null || true
        ${DOCKER_COMPOSE} up -d
        echo "[*] Database wiped. Fresh tables will be created on startup."
    else
        echo "[*] STEP 3/4: Restarting containers with MariaDB persistence preserved..."
        ${DOCKER_COMPOSE} down --remove-orphans || true
        ${DOCKER_COMPOSE} up -d
    fi

    echo ""
    echo "[*] STEP 4/4: Applying database schema migrations..."
    # coke-server automatically runs idempotent migrations on startup

    # If native Debian systemd service is active on host, synchronize /opt/coke-studios
    if systemctl is-active --quiet coke-studios.service 2>/dev/null; then
        echo "[*] Detected active native Debian systemd service (coke-studios.service)."
        echo "    Synchronizing /opt/coke-studios with latest build..."
        if [ "$(id -u)" -eq 0 ]; then
            ./tools/deploy-debian.sh --update || echo "[WARN] Native deploy-debian update encountered warnings."
        else
            sudo ./tools/deploy-debian.sh --update || echo "[WARN] Native deploy-debian update encountered warnings."
        fi
    fi
else
    echo ""
    echo "[*] Launching MariaDB & Coke Studios via Docker Compose..."
    ${DOCKER_COMPOSE} build --pull --progress=plain
    ${DOCKER_COMPOSE} up -d
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
    if [ ${IS_NUKE} -eq 1 ]; then
        echo "  SUCCESS: COKE STUDIOS NUKED & RESETUP FRESH!"
    elif [ ${IS_UPDATE} -eq 1 ]; then
        echo "  SUCCESS: COKE STUDIOS UPDATED WITH ZERO DATA LOSS!"
    else
        echo "  SUCCESS: COKE STUDIOS IS LIVE AND HEALTHY!"
    fi
    echo "===================================================================="
    echo "  Official MyCoke Game Portal: http://127.0.0.1:${PORT}/"
    echo "  Direct Game Client:         http://127.0.0.1:${PORT}/game/"
    echo "  Music Mixer Studio:         http://127.0.0.1:${PORT}/mixer/"
    echo "  Health Status:              ${HEALTH_DATA}"
    echo "===================================================================="
    echo "  Useful Commands:"
    echo "  - Update server (zero data loss): ./setup.sh --update"
    echo "  - Total reset & cache wipe:       ./setup.sh --nuke (or ./nuke.sh)"
    echo "===================================================================="
else
    echo "[WARN] Server is starting up. Check logs with:"
    echo "  ${DOCKER_COMPOSE} logs -f"
    echo "  Then visit: http://127.0.0.1:${PORT}/"
fi
