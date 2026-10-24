#!/usr/bin/env bash
# Coke Music / Coke Studios Recovery - Stop Script
# Target: Debian 12 / Linux
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

PID_FILE=".runtime/server.pid"

STOPPED=0

if [ -f "$PID_FILE" ]; then
    PID="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
        echo "[*] Stopping server process (PID: $PID)..."
        kill "$PID" 2>/dev/null || true
        for i in {1..10}; do
            if ! kill -0 "$PID" 2>/dev/null; then
                break
            fi
            sleep 0.5
        done
        if kill -0 "$PID" 2>/dev/null; then
            echo "[!] Force killing process $PID..."
            kill -9 "$PID" 2>/dev/null || true
        fi
        STOPPED=1
    fi
    rm -f "$PID_FILE"
fi

# Also check for any rogue server processes
ROGUE_PIDS="$(pgrep -f "coke-server-linux" 2>/dev/null || true)"
if [ -n "$ROGUE_PIDS" ]; then
    echo "[*] Cleaning up remaining coke-server processes ($ROGUE_PIDS)..."
    kill $ROGUE_PIDS 2>/dev/null || true
    STOPPED=1
fi

if [ "$STOPPED" -eq 1 ]; then
    echo "[✓] Coke Music / Coke Studios local servers stopped."
else
    echo "[i] No active servers found."
fi
