#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Clone the dirplayer-rs fork pinned in dirplayer-rs.lock.
#
# Reads ./dirplayer-rs.lock and clones the fork at the pinned SHA.
# Idempotent: if the destination already exists, fetches and checks
# out the SHA.
#
# Usage:
#   ./setup.sh [DEST]
#
# DEST defaults to ../dirplayer-rs (sibling to this checkout). Override
# via DIRPLAYER_DEST env var or the first positional arg.
#
# The pinned fork is public. No special access needed.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
lockfile="$script_dir/dirplayer-rs.lock"

if [[ ! -f "$lockfile" ]]; then
    echo "ERROR: lockfile not found at $lockfile" >&2
    exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
    echo "ERROR: jq is required to parse $lockfile" >&2
    echo "  brew install jq  (macOS) | apt-get install jq  (Debian/Ubuntu)" >&2
    exit 1
fi

remote=$(jq -r '.remote' "$lockfile")
branch=$(jq -r '.branch' "$lockfile")
sha=$(jq -r '.sha' "$lockfile")

parent_dir="$(cd "$script_dir/.." && pwd)"
dest="${1:-${DIRPLAYER_DEST:-$parent_dir/dirplayer-rs}}"

echo "Toolchain pin: $remote @ $sha (branch $branch)"
echo "Destination:   $dest"
echo

if [[ -d "$dest/.git" ]]; then
    echo "Existing clone detected; fetching pinned SHA..."
    git -C "$dest" fetch "$remote" "$branch"
    git -C "$dest" checkout "$sha"
    echo "Checked out pinned SHA in $dest"
else
    echo "Cloning fresh..."
    mkdir -p "$(dirname "$dest")"
    git clone "$remote" "$dest"
    git -C "$dest" checkout "$sha"
    echo "Cloned to $dest at pinned SHA"
fi

echo
echo "Verify:"
echo "  cd $dest && git rev-parse HEAD"
echo "  Expected: $sha"
