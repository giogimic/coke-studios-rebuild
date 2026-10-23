#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
"""Extract one publicroom .cct's data via dirplayer MCP into a single JSON file."""
import json
import os
import re
import sys
import urllib.request

from _safe_write import safe_write, WIPClobberError

MCP_URL = "http://localhost:9847/"
MCP_TOKEN_PATTERN = re.compile(r"[A-Za-z0-9_-]{43,128}")
CAST_LIB = 19  # Studio slot we hot-swap into
CAST_BASE = "http://127.0.0.1:8765/publicrooms"


def call(method: str, params: dict | None = None, req_id: int = 1) -> dict:
    token = os.environ.get("DIRPLAYER_MCP_TOKEN")
    if token is None or MCP_TOKEN_PATTERN.fullmatch(token) is None:
        raise RuntimeError(
            "DIRPLAYER_MCP_TOKEN must be a 43-128 character base64url token"
        )

    payload = {"jsonrpc": "2.0", "id": req_id, "method": method}
    if params is not None:
        payload["params"] = params
    req = urllib.request.Request(
        MCP_URL,
        data=json.dumps(payload).encode(),
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read())


def tool(name: str, arguments: dict, req_id: int = 1) -> dict:
    r = call("tools/call", {"name": name, "arguments": arguments}, req_id)
    if "error" in r:
        raise RuntimeError(f"MCP error: {r['error']}")
    text = r["result"]["content"][0]["text"]
    return json.loads(text)


def eval_expr(code: str, req_id: int = 1) -> dict:
    return tool("eval_lingo", {"code": code}, req_id)


def unquote_string(value: str) -> str:
    """Lingo eval returns string values wrapped in literal quotes; unwrap them."""
    if value.startswith('"') and value.endswith('"'):
        return value[1:-1]
    return value


def collect_script_bodies(scripts: dict, tool_fn=tool) -> dict:
    """Attach decompiled Lingo source to every script in a `list_scripts` result.

    `list_scripts` only yields each script's name + handler *names*; the actual
    behavior source comes from `decompile_handler` (per cast_lib/cast_member/
    handler_name → `{handler_name, arguments, source}`). For each handler we
    store its `source` under `script["handler_sources"][handler_name]`. This is
    what turns the bare script list into real per-room behavior bodies
    (e.g. whale_wash's ColorBlendAnimator, whose `.ls` exists only inside
    `secretroom.cct`).

    Decompile failures are recorded inline so one bad handler never aborts the
    whole extraction. Mutates and returns `scripts`."""
    captured = 0
    for s in scripts.get("scripts") or []:
        cl, cm = s.get("cast_lib"), s.get("cast_member")
        sources: dict[str, str] = {}
        for hname in s.get("handlers", []):
            try:
                d = tool_fn(
                    "decompile_handler",
                    {"cast_lib": cl, "cast_member": cm, "handler_name": hname},
                )
                sources[hname] = d.get("source", "")
            except Exception as e:  # noqa: BLE001 — never abort on one handler
                sources[hname] = f"<DECOMPILE_FAILED: {e}>"
            captured += 1
        s["handler_sources"] = sources
    scripts["handler_source_count"] = captured
    return scripts


def extract(room_name: str, out_path: str) -> None:
    cct_url = f"{CAST_BASE}/{room_name}.cct"

    print(f"[0/6] Clearing castLib({CAST_LIB}) via Empty.cct cycle...", file=sys.stderr)
    r = eval_expr(f'castLib({CAST_LIB}).fileName = "http://127.0.0.1:8765/Empty.cct"', req_id=0)
    assert r.get("success"), r

    print(f"[1/6] Hot-swapping castLib({CAST_LIB}).fileName -> {cct_url}", file=sys.stderr)
    r = eval_expr(f'castLib({CAST_LIB}).fileName = "{cct_url}"', req_id=1)
    assert r.get("success"), r

    print(f"[2/6] Listing cast members of lib {CAST_LIB}...", file=sys.stderr)
    members = tool("list_cast_members", {"cast_lib": CAST_LIB}, req_id=2)
    print(f"      {len(members)} members loaded", file=sys.stderr)

    text_payloads: dict[str, str] = {}
    print(f"[3/6] Reading text/field bodies via eval_lingo...", file=sys.stderr)
    for m in members:
        if m["member_type"] in ("text", "field"):
            num = m["cast_member"]
            try:
                r = eval_expr(f"the text of member {num} of castLib {CAST_LIB}", req_id=100 + num)
                if r.get("success") and r.get("result_type") == "string":
                    text_payloads[m["name"]] = unquote_string(r["result_value"])
                else:
                    text_payloads[m["name"]] = f"<EVAL_FAILED: {r}>"
            except Exception as e:
                text_payloads[m["name"]] = f"<EXCEPTION: {e}>"

    print(f"[4/6] Listing scripts in lib {CAST_LIB}...", file=sys.stderr)
    scripts = tool("list_scripts", {"cast_lib": CAST_LIB}, req_id=3)

    print(
        f"[5/6] Decompiling handler sources for {scripts.get('total_count', 0)} scripts...",
        file=sys.stderr,
    )
    collect_script_bodies(scripts)
    print(
        f"      {scripts.get('handler_source_count', 0)} handler bodies captured",
        file=sys.stderr,
    )

    # `list_scripts` returns scripts in a non-deterministic order (HashMap
    # iteration in the Rust MCP), so re-extractions produce noisy reorder-only
    # diffs. Sort by (name, cast_member) for stable, diff-friendly output.
    if isinstance(scripts.get("scripts"), list):
        scripts["scripts"].sort(key=lambda s: (s.get("name") or "", s.get("cast_member") or 0))

    out = {
        "room": room_name,
        "source_cct": cct_url,
        "cast_lib_loaded_into": CAST_LIB,
        "member_count": len(members),
        "members": members,
        "text_bodies": text_payloads,
        "scripts": scripts,
    }
    content = json.dumps(out, indent=2) + "\n"
    try:
        safe_write(out_path, content)
    except WIPClobberError as e:
        print(f"[6/6] SKIP {out_path}: {e}", file=sys.stderr)
        sys.exit(1)
    print(f"[6/6] Wrote {out_path} ({len(content)} bytes)", file=sys.stderr)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("usage: extract_room.py <room_name> <output_path>", file=sys.stderr)
        sys.exit(1)
    extract(sys.argv[1], sys.argv[2])
