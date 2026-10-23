#!/usr/bin/env bash
# Coke Music / Coke Studios Recovery Setup Script
# Target: Debian 12 (Bookworm) / Debian-based Linux Systems
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=========================================================="
echo " Coke Music / Coke Studios Recovery - Debian Setup"
echo "=========================================================="

# Check for root/sudo if packages are needed
check_command() {
    command -v "$1" >/dev/null 2>&1
}

MISSING_PKGS=()
if ! check_command node; then
    MISSING_PKGS+=("nodejs")
fi
if ! check_command git; then
    MISSING_PKGS+=("git")
fi
if ! check_command curl; then
    MISSING_PKGS+=("curl")
fi
if ! check_command tar; then
    MISSING_PKGS+=("tar")
fi

if [ ${#MISSING_PKGS[@]} -gt 0 ]; then
    echo "[!] Missing required packages: ${MISSING_PKGS[*]}"
    echo "    On Debian/Ubuntu, install them with:"
    echo "    sudo apt-get update && sudo apt-get install -y ${MISSING_PKGS[*]}"
    if check_command sudo; then
        read -r -p "    Attempt automated installation via sudo apt-get? [y/N]: " RESP || RESP="n"
        if [[ "$RESP" =~ ^[Yy]$ ]]; then
            sudo apt-get update && sudo apt-get install -y "${MISSING_PKGS[@]}"
        fi
    fi
else
    echo "[✓] Core requirements found (Node.js $(node -v), Git $(git --version | head -n1))"
fi

# Make binaries and scripts executable
echo "[*] Setting executable permissions on Linux binaries and scripts..."
chmod +x ./tools/bin/gocoke-serve-linux 2>/dev/null || true
chmod +x ./tools/bin/gocoke-static-linux 2>/dev/null || true
chmod +x ./start.sh 2>/dev/null || true
chmod +x ./stop.sh 2>/dev/null || true

# Verify runtime directories
mkdir -p .runtime archives docs sources tools/bin

# Run adapter checks
echo "[*] Running verification test suite..."
if check_command node; then
    node tools/test-local.cjs
    node tools/check-browser-assets.cjs
    node tools/verify-sources.cjs
    echo "[✓] All verification tests passed successfully."
else
    echo "[!] Node.js not detected; skipping local test run."
fi

echo ""
echo "=========================================================="
echo " Setup complete! To launch the local servers on Debian:"
echo "   ./start.sh"
echo " To stop the local servers:"
echo "   ./stop.sh"
echo " Or run containerized via Docker:"
echo "   docker compose up -d"
echo "=========================================================="
