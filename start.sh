#!/usr/bin/env bash
# Coke Music / Coke Studios Recovery - Launcher Script
# Target: Debian 12 / Linux
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

mkdir -p .runtime

PID_FILE=".runtime/server.pid"
LOG_FILE=".runtime/server.log"

if [ -f "$PID_FILE" ]; then
    PID="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
        echo "[!] Server is already running with PID $PID."
        echo "    Open http://127.0.0.1:8000/ in your browser."
        echo "    To stop: ./stop.sh"
        exit 0
    fi
fi

echo "=========================================================="
echo " Starting Coke Music / Coke Studios Local Servers"
echo "=========================================================="

if ! command -v node >/dev/null 2>&1; then
    echo "[ERROR] Node.js is required. Install via: sudo apt-get install -y nodejs"
    exit 1
fi

FOREGROUND=0
if [[ "${1:-}" == "-f" || "${1:-}" == "--foreground" ]]; then
    FOREGROUND=1
fi

PORT="${PORT:-24902}"

if [ "$FOREGROUND" -eq 1 ]; then
    echo "[*] Running in foreground on port $PORT. Press Ctrl+C to stop."
    PORT="$PORT" exec node tools/serve-local.cjs
else
    PORT="$PORT" nohup node tools/serve-local.cjs > "$LOG_FILE" 2>&1 &
    NEW_PID=$!
    echo "$NEW_PID" > "$PID_FILE"
    sleep 1

    if ! kill -0 "$NEW_PID" 2>/dev/null; then
        echo "[ERROR] Failed to start server. Check $LOG_FILE for details."
        exit 1
    fi

    echo "[✓] Servers started in background (PID: $NEW_PID)."
    echo ""
    echo "  → Master Portal & Launcher: http://127.0.0.1:$PORT/"
    echo "  → GoCoke Game Client:       http://127.0.0.1:$PORT/game/ (or http://127.0.0.1:8080/)"
    echo "  → CokePhase HTML5:          http://127.0.0.1:8081/"
    echo "  → Music Mixer:              http://127.0.0.1:8082/"
    echo ""
    echo "  Logs: tail -f $LOG_FILE"
    echo "  Stop: ./stop.sh"
    echo "=========================================================="
fi
