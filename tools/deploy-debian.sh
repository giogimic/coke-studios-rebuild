#!/usr/bin/env bash
# ==============================================================================
# Coke Studios - Production Debian 12 (Bookworm) Deployment Automation
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "===================================================================="
echo "    COKE STUDIOS - DEBIAN 12 (BOOKWORM) HOST DEPLOYMENT SETUP"
echo "===================================================================="

if [ "$(id -u)" -ne 0 ]; then
    echo "[ERROR] This deployment script must be run as root (or with sudo)." >&2
    exit 1
fi

INSTALL_DIR="/opt/coke-studios"
SERVICE_NAME="coke-studios.service"
DB_NAME="${DB_NAME:-coke_studios}"
DB_USER="${DB_USER:-coke}"
DB_PASS="${DB_PASS:-coke_secret}"
PORT="${PORT:-24902}"

echo "[1/6] Installing necessary Debian packages..."
apt-get update -qq
apt-get install -y -qq mariadb-server curl ca-certificates rsync

echo "[2/6] Configuring MariaDB database and credentials..."
systemctl start mariadb || systemctl start mysql || true
mariadb -e "CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" 2>/dev/null || mysql -e "CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
mariadb -e "CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';" 2>/dev/null || mysql -e "CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';"
mariadb -e "GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'localhost';" 2>/dev/null || mysql -e "GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'localhost';"
mariadb -e "FLUSH PRIVILEGES;" 2>/dev/null || mysql -e "FLUSH PRIVILEGES;"
echo "      MariaDB database '${DB_NAME}' and user '${DB_USER}' configured."

echo "[3/6] Setting up system service user 'coke' and target directory..."
if ! id -u coke >/dev/null 2>&1; then
    useradd -r -s /bin/false -d "${INSTALL_DIR}" coke 2>/dev/null || true
    echo "      Created system user 'coke'."
fi

mkdir -p "${INSTALL_DIR}/tools/bin"
mkdir -p "${INSTALL_DIR}/sources/gocoke"
mkdir -p "${INSTALL_DIR}/tools/portal"
mkdir -p "${INSTALL_DIR}/.runtime"

echo "[4/6] Synchronizing production binaries and web assets to ${INSTALL_DIR}..."
# Ensure Linux binary exists, build if missing
if [ ! -f "${REPO_ROOT}/tools/bin/coke-server-linux" ]; then
    echo "      Compiling static coke-server-linux..."
    (cd "${REPO_ROOT}" && chmod +x tools/build-go.sh && ./tools/build-go.sh)
fi

cp -f "${REPO_ROOT}/tools/bin/coke-server-linux" "${INSTALL_DIR}/tools/bin/coke-server-linux"
chmod +x "${INSTALL_DIR}/tools/bin/coke-server-linux"

rsync -a --delete "${REPO_ROOT}/sources/gocoke/web/" "${INSTALL_DIR}/sources/gocoke/web/"
rsync -a --delete "${REPO_ROOT}/tools/portal/" "${INSTALL_DIR}/tools/portal/"

chown -R coke:coke "${INSTALL_DIR}" 2>/dev/null || true

echo "[5/6] Installing and configuring systemd service..."
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

echo "[6/6] Verifying service readiness on port ${PORT}..."
READY=0
for i in {1..20}; do
    if curl -fs "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 0.5
done

if [ "$READY" -eq 1 ]; then
    HEALTH=$(curl -s "http://127.0.0.1:${PORT}/health")
    echo "===================================================================="
    echo "  DEPLOYMENT SUCCESSFUL! Coke Studios is running on Debian 12."
    echo "===================================================================="
    echo "  URL:           http://127.0.0.1:${PORT}/"
    echo "  Status:        ${HEALTH}"
    echo "  Systemd Unit:  systemctl status ${SERVICE_NAME}"
    echo "  Live Logs:     journalctl -u ${SERVICE_NAME} -f"
    echo "===================================================================="
else
    echo "[WARNING] Service started, but healthcheck timed out. Check: journalctl -u ${SERVICE_NAME} -e"
fi
