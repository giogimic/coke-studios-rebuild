#!/usr/bin/env bash
# ==============================================================================
# Coke Studios & Coke Music - Comprehensive Stop Script
# Target OS: Debian 12 (Bookworm) / Ubuntu / Linux
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "===================================================================="
echo "    COKE STUDIOS & COKE MUSIC - STOPPING ALL SERVICES (DEBIAN)"
echo "===================================================================="

STOPPED_ANY=0

# 1. Stop Docker Compose if docker is present
if command -v docker >/dev/null 2>&1; then
    if [ -f "docker-compose.yml" ]; then
        echo "[*] Stopping Docker Compose containers..."
        docker compose down --remove-orphans -t 3 2>/dev/null || true
        STOPPED_ANY=1
    fi

    # Halt any lingering Docker containers matching 'coke'
    COKE_CONTAINERS=$(docker ps -q --filter "name=coke" 2>/dev/null || true)
    if [ -n "${COKE_CONTAINERS}" ]; then
        echo "[*] Halting orphaned Coke Docker containers: ${COKE_CONTAINERS}..."
        docker stop ${COKE_CONTAINERS} 2>/dev/null || true
        STOPPED_ANY=1
    fi
fi

# 2. Stop systemd service if running
if command -v systemctl >/dev/null 2>&1; then
    if systemctl is-active --quiet coke-studios.service 2>/dev/null; then
        echo "[*] Stopping systemd service coke-studios.service..."
        sudo systemctl stop coke-studios.service 2>/dev/null || true
        STOPPED_ANY=1
    fi
fi

# 3. Stop native Go server process via PID file if present
PID_FILE=".runtime/server.pid"
if [ -f "$PID_FILE" ]; then
    PID="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
        echo "[*] Stopping server process (PID: $PID)..."
        kill "$PID" 2>/dev/null || true
        for _ in {1..10}; do
            if ! kill -0 "$PID" 2>/dev/null; then
                break
            fi
            sleep 0.2
        done
        if kill -0 "$PID" 2>/dev/null; then
            echo "[!] Force killing server PID $PID..."
            kill -9 "$PID" 2>/dev/null || true
        fi
        STOPPED_ANY=1
    fi
    rm -f "$PID_FILE"
fi

# 4. Kill native Go server binaries by name
echo "[*] Checking for background Go binaries..."
GO_PIDS=$(pgrep -f "coke-server" 2>/dev/null || true)
if [ -n "${GO_PIDS}" ]; then
    echo "[*] Terminating Go server processes: ${GO_PIDS}..."
    kill -9 ${GO_PIDS} 2>/dev/null || true
    STOPPED_ANY=1
fi

# 5. Kill reference Node / Mixer servers if running
NODE_PIDS=$(pgrep -f "node.*server.js|cokephase" 2>/dev/null || true)
if [ -n "${NODE_PIDS}" ]; then
    echo "[*] Terminating Node/Mixer processes: ${NODE_PIDS}..."
    kill -9 ${NODE_PIDS} 2>/dev/null || true
    STOPPED_ANY=1
fi

# 6. Sweep and free all Coke Studios ports directly
PORTS=(24902 8080 8085 3306 9999 8081 8082)
echo "[*] Verifying release of Coke Studios ports: ${PORTS[*]}..."
for PORT in "${PORTS[@]}"; do
    if command -v fuser >/dev/null 2>&1; then
        if fuser -n tcp "${PORT}" >/dev/null 2>&1; then
            echo "  [!] Freeing port ${PORT} via fuser..."
            fuser -k -n tcp "${PORT}" 2>/dev/null || true
            STOPPED_ANY=1
        fi
    elif command -v lsof >/dev/null 2>&1; then
        PORT_PIDS=$(lsof -ti :"${PORT}" 2>/dev/null || true)
        if [ -n "${PORT_PIDS}" ]; then
            echo "  [!] Freeing port ${PORT} (PIDs: ${PORT_PIDS})..."
            kill -9 ${PORT_PIDS} 2>/dev/null || true
            STOPPED_ANY=1
        fi
    fi
done

# 7. Clean runtime lock files
rm -f .runtime/*.pid *.pid .runtime/*.sock

echo "===================================================================="
if [ "${STOPPED_ANY}" -eq 1 ]; then
    echo " [OK] ALL COKE STUDIOS PROCESSES, CONTAINERS & PORTS STOPPED!"
else
    echo " [i] No active Coke Studios services or processes were running."
fi
echo "===================================================================="
