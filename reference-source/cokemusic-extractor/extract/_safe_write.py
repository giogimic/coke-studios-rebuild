# SPDX-License-Identifier: GPL-3.0-only
"""WIP-clobber guard for generator scripts.

Generators (translate_all.py, translate_room.py, etc.) regenerate tracked
files in the working tree. If the user has uncommitted edits in those
files (in-flight WIP from a parallel session), a naive `path.write_text(...)`
silently destroys that work.

`safe_write` refuses to overwrite a tracked file that has working-tree
modifications vs HEAD, unless `force=True` is passed. New (untracked) files
write freely. Identical content is a no-op (avoids touching mtime).

Usage:

    from _safe_write import safe_write, WIPClobberError

    try:
        safe_write(dst, content, force=args.force)
    except WIPClobberError as e:
        print(f"SKIP  {e}", file=sys.stderr)
        # caller decides: skip, abort, prompt, etc.
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from typing import Union


class WIPClobberError(Exception):
    """Refusing to overwrite a file with uncommitted changes vs HEAD."""


def _is_dirty_vs_head(path: Path) -> bool:
    """True iff path is tracked by git AND its working-tree content differs
    from HEAD. False for untracked files, files matching HEAD, or any error
    (be conservative on error: don't claim dirty)."""
    try:
        # `git diff --quiet HEAD -- <path>` exits:
        #   0 if no changes (or untracked — git diff treats untracked as nothing)
        #   1 if working tree differs from HEAD
        # 128 if not a git repo or other error
        result = subprocess.run(
            ["git", "diff", "--quiet", "HEAD", "--", str(path)],
            cwd=path.parent if path.parent.exists() else Path.cwd(),
            capture_output=True,
        )
    except FileNotFoundError:
        return False  # no git in PATH — can't tell, assume clean
    if result.returncode == 1:
        # confirm the file is actually tracked (untracked files never differ
        # from HEAD per git diff, but be explicit just in case)
        check = subprocess.run(
            ["git", "ls-files", "--error-unmatch", "--", str(path)],
            cwd=path.parent if path.parent.exists() else Path.cwd(),
            capture_output=True,
        )
        return check.returncode == 0
    return False


def safe_write(
    path: Union[Path, str],
    content: Union[str, bytes],
    *,
    force: bool = False,
) -> bool:
    """Write `content` to `path`. Refuses to overwrite a tracked file that
    has uncommitted working-tree changes vs HEAD unless `force=True`.

    Returns True if the file was written (or no-op identical), False is
    impossible — raises instead. Raises `WIPClobberError` on refusal.

    No-op when the existing file content equals the new content (avoids
    bumping mtime which can trigger watchers / rebuilds for nothing)."""
    p = Path(path)

    # No-op short-circuit: if file exists and content already matches, skip.
    if p.exists():
        existing: Union[str, bytes]
        if isinstance(content, bytes):
            existing = p.read_bytes()
        else:
            existing = p.read_text()
        if existing == content:
            return True

    if not force and p.exists() and _is_dirty_vs_head(p):
        raise WIPClobberError(
            f"{p} has uncommitted changes vs HEAD. Refusing to overwrite. "
            f"Pass --force to override, or commit/stash the changes first."
        )

    if isinstance(content, bytes):
        p.write_bytes(content)
    else:
        p.write_text(content)
    return True
