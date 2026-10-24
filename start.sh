#!/usr/bin/env bash
# Coke Music / Coke Studios - Authoritative Go Server Launcher (Debian 12 First)
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
        echo "    Open http://127.0.0.1:24902/ in your browser."
        echo "    To stop: ./stop.sh"
        exit 0
    fi
fi

echo "=========================================================="
echo " Starting Coke Music / Coke Studios Authoritative Go Server"
echo "=========================================================="

PORT="${PORT:-24902}"
SERVER_BIN="./tools/bin/coke-server-linux"

if [ ! -f "$SERVER_BIN" ]; then
    echo "[*] Building coke-server-linux..."
    ./tools/build-go.sh
fi

FOREGROUND=0
if [[ "${1:-}" == "-f" || "${1:-}" == "--foreground" ]]; then
    FOREGROUND=1
fi

if [ "$FOREGROUND" -eq 1 ]; then
    echo "[*] Running in foreground on port $PORT. Press Ctrl+C to stop."
    exec "$SERVER_BIN" -addr "0.0.0.0:$PORT" -root "./sources/gocoke/web"
else
    nohup "$SERVER_BIN" -addr "0.0.0.0:$PORT" -root "./sources/gocoke/web" > "$LOG_FILE" 2>&1 &
    NEW_PID=$!
    echo "$NEW_PID" > "$PID_FILE"
    sleep 1

    if ! kill -0 "$NEW_PID" 2>/dev/null; then
        echo "[ERROR] Failed to start server. Check $LOG_FILE for details."
        exit 1
    fi

    echo "[✓] Go Server started in background (PID: $NEW_PID)."
    echo ""
    echo "  → Portal & Entry:       http://127.0.0.1:$PORT/"
    echo "  → Go WebAssembly Game:  http://127.0.0.1:$PORT/game/"
    echo "  → Health Status:        http://127.0.0.1:$PORT/health"
    echo ""
    echo "  Logs: tail -f $LOG_FILE"
    echo "  Stop: ./stop.sh"
    echo "=========================================================="
fi
