#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
"""
Reference consumer of the Rail A JSON schema.

What this script does
---------------------
Reads one `extracted/<room>.json` produced by ``extract/extract_room.py``
and emits a generic per-room summary as pretty-printed JSON on stdout.
The summary groups cast members by Director member type, previews the
three structured-XML text bodies (MapXml / SceneXml / EntryXml), and —
if ``--output-root`` is passed — resolves each bitmap member's name to
its expected PNG path under ``<output_root>/rooms/<room>/`` (the
convention Rail B's bitmap dumpers write to).

The output shape is intentionally generic. Anyone wiring CokeMusic
casts into their own runtime / engine / archive needs to know
*how to walk* the extracted JSON; this script demonstrates the
walk in stdlib Python, no third-party deps. Take it as
documentation-by-example, not as a production translator.

What this script is NOT
-----------------------
- It is **not a runtime**. It does not parse layouts into tile
  coordinates, walk grids, render anything, or load the actual
  bitmap bytes. It only reports what is *present* in the extraction.
- It is **not a tool-specific translator**. The shape it emits is
  generic, with no per-project field names. The extraction's
  text_bodies (MapXml / SceneXml / EntryXml) carry XML that downstream
  consumers parse however their runtime needs — see the comments at
  ``_summarize_xml_body`` for what's in each.
- It is **not exhaustive**. Director cast members may carry more
  metadata than ``extract_room.py`` extracts (palette refs, regPoints,
  bit depths) — that lives in Rail B's ``_members.json`` sidecars.
  See ``docs/SCHEMA.md`` for the canonical schema.

Usage
-----
    python3 examples/translate-example.py <extracted/room.json>
    python3 examples/translate-example.py <extracted/room.json> --output-root <abs/path>

The optional ``--output-root`` is exactly the ``OUTPUT_ROOT`` env var
passed to ``dump_cct_bitmaps``. When set, each bitmap member entry
gains a ``bitmap_path`` field that is either an existing absolute
path or ``null`` if the dumper hasn't written that member yet.

Exit codes:
    0  success
    1  extracted JSON file not found
    2  extracted JSON could not be parsed
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

# The three structured XML payloads Director embeds as text members
# in every CokeMusic publicroom cast. Each carries a different slice
# of the room's static state:
#   - MapXml    : per-cell walkability codes + initial occupants
#                 (chairs, queue points, performance stages, etc.)
#   - SceneXml  : scene-level config — bg cast member name, sound
#                 ambience, camera offset, palette refs.
#   - EntryXml  : avatar entry / spawn behavior (entry door cell,
#                 first-move cell, queue layout).
# The schema does not require these to be present (a non-publicroom
# cct may have only a subset), so the script reports each
# independently.
STRUCTURED_XML_BODIES = ("MapXml", "SceneXml", "EntryXml")

# Default output bytes for the XML preview. Long enough to confirm
# the shape (root element, a child or two, an attribute), short
# enough not to drown the JSON output.
XML_PREVIEW_CHARS = 200


def walk_extraction(extracted: dict, output_root: Path | None) -> dict:
    """Walk one extracted-JSON dict and emit the generic summary."""
    room = extracted.get("room")
    members = extracted.get("members", [])
    text_bodies = extracted.get("text_bodies", {}) or {}
    scripts = extracted.get("scripts", {}) or {}

    return {
        # Passthrough fields — exactly as Rail A wrote them.
        "room": room,
        "source_cct": extracted.get("source_cct"),
        "cast_lib_loaded_into": extracted.get("cast_lib_loaded_into"),
        "member_count": extracted.get("member_count"),

        # Members grouped by member_type, with each entry slimmed to
        # the fields a generic consumer needs to identify the member.
        "members_by_type": _classify_members(members, room, output_root),

        # Per-XML-body presence + size + short preview. Consumers
        # parse the actual XML themselves (xml.etree, lxml, ad-hoc
        # regex, whatever fits the project).
        "structured_xml": {
            name: _summarize_xml_body(text_bodies.get(name))
            for name in STRUCTURED_XML_BODIES
        },

        # Script count is the cheap sanity check; full source bodies
        # live in extracted["scripts"]["scripts"][i]["text"]. We don't
        # quote them here because they are typically multi-kilobyte.
        "scripts_count": scripts.get("total_count", len(scripts.get("scripts", []))),
    }


def _classify_members(
    members: list[dict],
    room: str | None,
    output_root: Path | None,
) -> dict[str, list[dict]]:
    """Group members by `member_type`. Bitmap members get an optional
    `bitmap_path` field resolved to where Rail B writes its output."""
    by_type: dict[str, list[dict]] = {}
    for member in members:
        # Director member_type is one of: bitmap, script, text, field,
        # sound, palette, filmLoop, shape, ... (the full Director
        # member-type set; in practice CokeMusic publicrooms only use a
        # handful). Unknown member_types still get a bucket so the
        # output is lossless.
        member_type = member.get("member_type", "unknown")
        entry = {
            "name": member.get("name"),
            "cast_lib": member.get("cast_lib"),
            "cast_member": member.get("cast_member"),
        }
        if member_type == "bitmap":
            entry["bitmap_path"] = _resolve_bitmap_path(
                member.get("name"), room, output_root
            )
        by_type.setdefault(member_type, []).append(entry)
    return by_type


def _resolve_bitmap_path(
    name: str | None,
    room: str | None,
    output_root: Path | None,
) -> str | None:
    """Return the absolute path Rail B's `dump_cct_bitmaps` would have
    written this member's PNG to, OR `None` if `--output-root` wasn't
    passed or the file isn't there yet.

    Rail B writes one PNG per bitmap member at:
        <output_root>/rooms/<room>/<member_name>.png
    plus a `_members.json` sidecar in the same directory.
    """
    if output_root is None or not name or not room:
        return None
    candidate = output_root / "rooms" / room / f"{name}.png"
    return str(candidate) if candidate.exists() else None


def _summarize_xml_body(body: str | None) -> dict:
    """Report whether one of the structured XML text bodies is
    present, how large it is, and a short preview. Reading the actual
    XML belongs to the downstream consumer."""
    if body is None:
        return {"present": False, "byte_length": 0, "preview": None}
    return {
        "present": True,
        # Director text_bodies are decoded as Python strings by
        # `extract_room.py`, so byte_length here is the UTF-8 size,
        # which matches what a downstream serialization will produce.
        "byte_length": len(body.encode("utf-8")),
        "preview": body[:XML_PREVIEW_CHARS],
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Reference consumer of the Rail A JSON schema "
            "(see docs/SCHEMA.md). Reads one extracted/<room>.json "
            "and prints a generic per-room summary."
        ),
    )
    parser.add_argument(
        "extracted_json",
        help="Path to a JSON file produced by extract/extract_room.py.",
    )
    parser.add_argument(
        "--output-root",
        default=None,
        help=(
            "Optional. Directory passed as OUTPUT_ROOT to Rail B's "
            "dump_cct_bitmaps (the parent of `rooms/`). When set, each "
            "bitmap member gets a resolved `bitmap_path` field if the "
            "PNG exists on disk."
        ),
    )
    args = parser.parse_args(argv)

    src = Path(args.extracted_json)
    if not src.is_file():
        print(f"error: file not found: {src}", file=sys.stderr)
        return 1
    try:
        extracted = json.loads(src.read_text(encoding="utf-8"))
    except json.JSONDecodeError as e:
        print(f"error: invalid JSON in {src}: {e}", file=sys.stderr)
        return 2

    output_root = Path(args.output_root) if args.output_root else None
    summary = walk_extraction(extracted, output_root)
    json.dump(summary, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
