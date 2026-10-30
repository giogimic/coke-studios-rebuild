#!/usr/bin/env bash
# Shell Build Script for Coke Studios Go Client and Server (Debian 12 Bookworm / Linux First)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
GOCOKE_DIR="${REPO_ROOT}/sources/gocoke"
WEB_DIR="${GOCOKE_DIR}/web"
BIN_DIR="${REPO_ROOT}/tools/bin"

mkdir -p "${BIN_DIR}"

echo "==> 1. Copying wasm_exec.js from Go toolchain..."
GOROOT="$(go env GOROOT)"
if [ -f "${GOROOT}/lib/wasm/wasm_exec.js" ]; then
    cp -f "${GOROOT}/lib/wasm/wasm_exec.js" "${WEB_DIR}/wasm_exec.js"
    echo "    Copied from ${GOROOT}/lib/wasm/wasm_exec.js"
elif [ -f "${GOROOT}/misc/wasm/wasm_exec.js" ]; then
    cp -f "${GOROOT}/misc/wasm/wasm_exec.js" "${WEB_DIR}/wasm_exec.js"
    echo "    Copied from ${GOROOT}/misc/wasm/wasm_exec.js"
else
    echo "    Notice: Keeping existing web/wasm_exec.js"
fi

echo "==> 2. Building Go WebAssembly client (main.wasm)..."
(
    cd "${GOCOKE_DIR}"
    GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o "${WEB_DIR}/main.wasm" ./cmd/client
    WASM_SIZE=$(stat -c%s "${WEB_DIR}/main.wasm" 2>/dev/null || stat -f%z "${WEB_DIR}/main.wasm" 2>/dev/null || echo 0)
    if [ "$WASM_SIZE" -ge 104857600 ]; then
        echo "    Error: main.wasm ($WASM_SIZE bytes) exceeds the distribution limit; new binary retained, no old binary substituted." >&2
        exit 1
    else
        echo "    Successfully built ${WEB_DIR}/main.wasm ($WASM_SIZE bytes)"
    fi
)

echo "==> 3. Building Authoritative Server for Debian Linux (coke-server-linux)..."
(
    cd "${GOCOKE_DIR}"
    GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "${BIN_DIR}/coke-server-linux" ./cmd/server
    echo "    Successfully built ${BIN_DIR}/coke-server-linux (Debian/Linux x64)"
)

echo "==> 4. Building Authoritative Server for Windows (coke-server.exe)..."
(
    cd "${GOCOKE_DIR}"
    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "${BIN_DIR}/coke-server.exe" ./cmd/server
    echo "    Successfully built ${BIN_DIR}/coke-server.exe (Windows x64)"
)

echo "==> Build complete! Both Go WASM client and server binaries are up to date."
