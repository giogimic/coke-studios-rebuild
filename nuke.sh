#!/usr/bin/env bash
# ==============================================================================
# Coke Studios 1:1 Recreation - Total Nuke & Resetup (Debian / Linux First)
# Wipes all Docker/Go caches, databases, repulls origin master, and rebuilds fresh.
# ==============================================================================
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${SCRIPT_DIR}/setup.sh" --nuke "$@"
