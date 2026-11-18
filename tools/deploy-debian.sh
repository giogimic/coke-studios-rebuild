#!/usr/bin/env bash
# ==============================================================================
# Coke Studios - Production Debian 12 (Bookworm) Deployment & Update Automation
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

INSTALL_DIR="/opt/coke-studios"
SERVICE_NAME="coke-studios.service"
DB_NAME="${DB_NAME:-coke_studios}"
DB_USER="${DB_USER:-coke}"
DB_PASS="${DB_PASS:-coke_secret}"
PORT="${PORT:-24902}"

IS_UPDATE=0
DO_PULL=0
FORCE_BUILD=0
IS_NUKE=0
FORCE_CONFIRM=0

# Parse CLI arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --nuke|-nuke|nuke|--reset|-reset|reset)
            IS_NUKE=1
            IS_UPDATE=1
            FORCE_BUILD=1
            DO_PULL=1
            shift
            ;;
        --update|-update|update|-u)
            IS_UPDATE=1
            shift
            ;;
        --pull|-pull|pull|-p)
            IS_UPDATE=1
            DO_PULL=1
            shift
            ;;
        --build|-build|build|-b)
            FORCE_BUILD=1
            shift
            ;;
        -y|--yes|-f|--force)
            FORCE_CONFIRM=1
            shift
            ;;
        --port)
            PORT="$2"
            shift 2
            ;;
        --help|-h)
            echo "===================================================================="
            echo " Coke Studios - Debian 12 Host Deployment & Update Automation"
            echo "===================================================================="
            echo "Usage: sudo ./tools/deploy-debian.sh [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --update, -u       Update existing server with zero data loss"
            echo "  --nuke             Wipe all caches, databases, repull git, and re-setup"
            echo "  --pull, -p         Git pull latest changes from origin master before updating"
            echo "  --build, -b        Force recompile of Go binaries (coke-server-linux & main.wasm)"
            echo "  -y, --yes          Bypass interactive confirmation prompts"
            echo "  --port <PORT>      Specify listening port (default: 24902)"
            echo "  --help, -h         Show this help message"
            echo ""
            exit 0
            ;;
        *)
            echo "[WARN] Unknown argument: $1"
            shift
            ;;
    esac
done

# Auto-detect existing installation
if [ -f "${INSTALL_DIR}/tools/bin/coke-server-linux" ] || systemctl list-unit-files "${SERVICE_NAME}" >/dev/null 2>&1; then
    IS_UPDATE=1
fi

echo "===================================================================="
if [ "${IS_NUKE}" -eq 1 ]; then
    echo "    COKE STUDIOS - DEBIAN 12 HOST NUKE & RESETUP"
elif [ "${IS_UPDATE}" -eq 1 ]; then
    echo "    COKE STUDIOS - DEBIAN 12 HOST UPDATE & RECOMPILE"
else
    echo "    COKE STUDIOS - DEBIAN 12 (BOOKWORM) HOST DEPLOYMENT SETUP"
fi
echo "===================================================================="
if [ "${IS_NUKE}" -eq 1 ]; then
    echo " Mode:             NUKE & RESETUP (Wipe Caches & Database)"
elif [ "${IS_UPDATE}" -eq 1 ]; then
    echo " Mode:             UPDATE (Zero Data Loss)"
else
    echo " Mode:             FRESH INSTALLATION"
fi
echo " Target Directory: ${INSTALL_DIR}"
echo " Service Name:     ${SERVICE_NAME}"
echo " Ingress Port:     ${PORT}"
echo " Database Name:    ${DB_NAME}"
echo "===================================================================="

if [ "$(id -u)" -ne 0 ]; then
    echo "[ERROR] This deployment script must be run as root (or with sudo)." >&2
    exit 1
fi

# Confirmation prompt if NUKE is requested
if [ "${IS_NUKE}" -eq 1 ] && [ "${FORCE_CONFIRM}" -eq 0 ]; then
    if [ -t 0 ] || [ -e /dev/tty ]; then
        TTY_IN="/dev/tty"
        [ -e /dev/tty ] || TTY_IN="/dev/stdin"
        echo ""
        echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
        echo "               WARNING: DEBIAN HOST NUKE SELECTED"
        echo "  This will:"
        echo "   1. Stop active ${SERVICE_NAME}"
        echo "   2. DROP MariaDB database '${DB_NAME}' (ALL PLAYER DATA WILL BE ERASED)"
        echo "   3. Clean ${INSTALL_DIR} binaries, web assets, and runtime caches"
        echo "   4. Wipe Go build caches"
        echo "   5. Discard local git modifications and repull origin master"
        echo "   6. Recompile Go server and WASM client fresh from source"
        echo "   7. Re-initialize MariaDB database and restart ${SERVICE_NAME}"
        echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
        read -r -p "  Are you sure you want to completely NUKE the host install? [y/N]: " REPLY_NUKE < "${TTY_IN}" || REPLY_NUKE="n"
        case "${REPLY_NUKE}" in
            [yY]|[yY][eE][sS])
                echo "[*] Nuke confirmed. Proceeding..."
                ;;
            *)
                echo "[*] Nuke aborted by user. Exiting."
                exit 0
                ;;
        esac
    fi
fi

# If nuke or update, stop active service first to drain connections
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    echo "      Stopping active ${SERVICE_NAME} to drain connections..."
    systemctl stop "${SERVICE_NAME}" || true
fi

# Wipe existing database and installation directories if NUKE
if [ "${IS_NUKE}" -eq 1 ]; then
    echo "      [!] NUKING MariaDB database '${DB_NAME}'..."
    mariadb -e "DROP DATABASE IF EXISTS \`${DB_NAME}\`;" 2>/dev/null || mysql -e "DROP DATABASE IF EXISTS \`${DB_NAME}\`;" || true

    echo "      [!] Wiping existing files in ${INSTALL_DIR}..."
    rm -rf "${INSTALL_DIR}/tools/bin"/* "${INSTALL_DIR}/sources/gocoke/web"/* "${INSTALL_DIR}/tools/portal"/* "${INSTALL_DIR}/.runtime"/* 2>/dev/null || true

    if command -v go >/dev/null 2>&1; then
        echo "      [!] Wiping Go build and test caches..."
        go clean -cache -testcache 2>/dev/null || true
    fi
fi

# Step 0: Git pull or reset if requested and available
if [ "${IS_NUKE}" -eq 1 ]; then
    echo "[*] Cleaning git working tree and repulling latest code from Git..."
    if [ -d "${REPO_ROOT}/.git" ]; then
        cd "${REPO_ROOT}"
        echo "    Current commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
        git fetch origin master || git fetch || echo "[WARN] Git fetch skipped."
        git reset --hard origin/master 2>/dev/null || git reset --hard HEAD || echo "[WARN] Git reset skipped."
        git clean -fd -e .env 2>/dev/null || true
        echo "    Reset and synced to: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
    else
        echo "[WARN] No .git directory found at ${REPO_ROOT}; skipping git pull."
    fi
elif [ "${DO_PULL}" -eq 1 ]; then
    echo "[*] Pulling latest repository changes from Git..."
    if [ -d "${REPO_ROOT}/.git" ]; then
        cd "${REPO_ROOT}"
        echo "    Current commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
        git pull origin master || git pull || echo "[WARN] Git pull encountered warnings or was skipped."
        echo "    Updated commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
    else
        echo "[WARN] No .git directory found at ${REPO_ROOT}; skipping git pull."
    fi
fi

echo "[1/6] Verifying necessary Debian packages..."
apt-get update -qq
apt-get install -y -qq mariadb-server curl ca-certificates rsync

# Step 2: MariaDB verification (preserves existing data)
echo "[2/6] Configuring MariaDB database and user credentials..."
systemctl start mariadb || systemctl start mysql || true
mariadb -e "CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" 2>/dev/null || mysql -e "CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
mariadb -e "CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';" 2>/dev/null || mysql -e "CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';"
mariadb -e "GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'localhost';" 2>/dev/null || mysql -e "GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'localhost';"
mariadb -e "FLUSH PRIVILEGES;" 2>/dev/null || mysql -e "FLUSH PRIVILEGES;"
echo "      MariaDB database '${DB_NAME}' and user '${DB_USER}' verified (data preserved)."

# Step 3: Setup service user and directories
echo "[3/6] Setting up system service user 'coke' and target directories..."
if ! id -u coke >/dev/null 2>&1; then
    useradd -r -s /bin/false -d "${INSTALL_DIR}" coke 2>/dev/null || true
    echo "      Created system user 'coke'."
fi

mkdir -p "${INSTALL_DIR}/tools/bin"
mkdir -p "${INSTALL_DIR}/sources/gocoke/web"
mkdir -p "${INSTALL_DIR}/tools/portal"
mkdir -p "${INSTALL_DIR}/.runtime"

# Step 4: Recompile and sync binaries and web assets
echo "[4/6] Compiling latest Go binaries and synchronizing assets..."

# If updating or forced, always recompile Go binaries to ensure updates take effect
if [ "${IS_UPDATE}" -eq 1 ] || [ "${FORCE_BUILD}" -eq 1 ] || [ ! -f "${REPO_ROOT}/tools/bin/coke-server-linux" ]; then
    if command -v go >/dev/null 2>&1; then
        echo "      Building fresh static coke-server-linux and main.wasm..."
        (cd "${REPO_ROOT}" && chmod +x tools/build-go.sh && ./tools/build-go.sh)
    else
        echo "      Go toolchain not found on host. Installing golang-go..."
        apt-get install -y -qq golang-go
        (cd "${REPO_ROOT}" && chmod +x tools/build-go.sh && ./tools/build-go.sh)
    fi
fi

# Gracefully stop active service before replacing binary to prevent ETXTBSY (Text file busy)
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    echo "      Gracefully stopping active ${SERVICE_NAME} to drain connections..."
    systemctl stop "${SERVICE_NAME}" || true
fi

# Backup previous binary if it exists
if [ -f "${INSTALL_DIR}/tools/bin/coke-server-linux" ]; then
    cp -f "${INSTALL_DIR}/tools/bin/coke-server-linux" "${INSTALL_DIR}/tools/bin/coke-server-linux.prev"
fi

# Install new binary using install -m 755 (unlinks inode cleanly)
install -m 755 "${REPO_ROOT}/tools/bin/coke-server-linux" "${INSTALL_DIR}/tools/bin/coke-server-linux"

# Synchronize web assets and portal
rsync -a --delete "${REPO_ROOT}/sources/gocoke/web/" "${INSTALL_DIR}/sources/gocoke/web/"
rsync -a --delete "${REPO_ROOT}/tools/portal/" "${INSTALL_DIR}/tools/portal/"

# Ensure file ownership
chown -R coke:coke "${INSTALL_DIR}" 2>/dev/null || true

# Step 5: Systemd service configuration
echo "[5/6] Installing and updating systemd service (${SERVICE_NAME})..."
cat > "/etc/systemd/system/${SERVICE_NAME}" <<EOF
[Unit]
Description=Coke Music / Coke Studios Authoritative Virtual World Server (Debian 12 First)
After=network.target mariadb.service
Wants=network-online.target

[Service]
Type=simple
User=coke
Group=coke
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/tools/bin/coke-server-linux -addr 0.0.0.0:${PORT} -root ${INSTALL_DIR}/sources/gocoke/web
Restart=always
RestartSec=5s
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal
SyslogIdentifier=coke-studios
KillSignal=SIGTERM
TimeoutStopSec=15s
Environment=PORT=${PORT}
Environment=DB_DRIVER=mysql
Environment=DATABASE_URL=${DB_USER}:${DB_PASS}@tcp(127.0.0.1:3306)/${DB_NAME}?parseTime=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl restart "${SERVICE_NAME}"

# Step 6: Readiness check
echo "[6/6] Verifying service readiness on port ${PORT}..."
READY=0
for i in {1..30}; do
    if curl -fs "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 0.5
    echo -n "."
done
echo ""

if [ "$READY" -eq 1 ]; then
    HEALTH=$(curl -s "http://127.0.0.1:${PORT}/health" || echo "{}")
    echo "===================================================================="
    if [ "${IS_NUKE}" -eq 1 ]; then
        echo "  NUKE & RESETUP SUCCESSFUL! Coke Studios is running from scratch."
    elif [ "${IS_UPDATE}" -eq 1 ]; then
        echo "  UPDATE SUCCESSFUL! Coke Studios is running the latest build."
    else
        echo "  DEPLOYMENT SUCCESSFUL! Coke Studios is running on Debian 12."
    fi
    echo "===================================================================="
    echo "  URL:           http://127.0.0.1:${PORT}/"
    echo "  Status:        ${HEALTH}"
    echo "  Systemd Unit:  systemctl status ${SERVICE_NAME}"
    echo "  Live Logs:     journalctl -u ${SERVICE_NAME} -f"
    echo "===================================================================="
    echo "  Useful Commands:"
    echo "  - Update server (zero data loss): sudo ./tools/deploy-debian.sh --update"
    echo "  - Git pull & update:              sudo ./tools/deploy-debian.sh --pull"
    echo "  - Total reset & nuke:             sudo ./tools/deploy-debian.sh --nuke"
    echo "===================================================================="
else
    echo "===================================================================="
    echo "[ERROR] Service started, but healthcheck timed out on port ${PORT}."
    echo "===================================================================="
    echo "Recent service logs:"
    journalctl -u "${SERVICE_NAME}" -n 25 --no-pager 2>/dev/null || true
    echo "===================================================================="
    if [ -f "${INSTALL_DIR}/tools/bin/coke-server-linux.prev" ]; then
        echo "[!] Rolling back to previous stable binary..."
        install -m 755 "${INSTALL_DIR}/tools/bin/coke-server-linux.prev" "${INSTALL_DIR}/tools/bin/coke-server-linux"
        systemctl restart "${SERVICE_NAME}"
        echo "[*] Previous binary restored. Check journalctl -u ${SERVICE_NAME} -e"
    fi
    exit 1
fi
