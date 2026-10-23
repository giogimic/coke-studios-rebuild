#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
"""Dump any cast lib loaded in the running dirplayer movie via MCP.

Usage:
    extract_castlib.py <lib_number> <output_path>
    extract_castlib.py --swap <url> <lib_number> <output_path>

`--swap <url>` hot-swaps `castLib(<lib_number>).fileName = <url>` first
(with the Empty.cct cycle to clear stale members).

Output JSON shape:
    {
        "cast_lib": <int>,
        "name": <lib name>,
        "swapped_to": <url or null>,
        "member_count": <int>,
        "script_count": <int>,
        "members": [...],
        "text_bodies": {<member name>: <text>},
        "scripts": [...]
    }
"""
import json
import os
import re
import sys
import urllib.request

MCP_URL = "http://localhost:9847/"
MCP_TOKEN_PATTERN = re.compile(r"[A-Za-z0-9_-]{43,128}")
EMPTY_CCT = "http://127.0.0.1:8765/Empty.cct"


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
    with urllib.request.urlopen(req, timeout=60) as resp:
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
    if value.startswith('"') and value.endswith('"'):
        return value[1:-1]
    return value


def dump_lib(lib_num: int, out_path: str, swap_url: str | None = None) -> None:
    if swap_url is not None:
        print(f"[0/4] Cycling castLib({lib_num}) via Empty.cct...", file=sys.stderr)
        r = eval_expr(f'castLib({lib_num}).fileName = "{EMPTY_CCT}"', req_id=0)
        assert r.get("success"), r
        print(f"[1/4] Hot-swapping castLib({lib_num}).fileName -> {swap_url}", file=sys.stderr)
        r = eval_expr(f'castLib({lib_num}).fileName = "{swap_url}"', req_id=1)
        assert r.get("success"), r

    # Lib metadata
    libs = tool("list_cast_libs", {}, req_id=2)
    me = next((l for l in libs if l["number"] == lib_num), None)
    if me is None:
        raise RuntimeError(f"cast lib {lib_num} not present")
    print(
        f"[2/4] Lib {lib_num} '{me['name']}': {me['member_count']} members, {me['script_count']} scripts",
        file=sys.stderr,
    )

    print(f"[3/4] Listing members + text bodies...", file=sys.stderr)
    members = tool("list_cast_members", {"cast_lib": lib_num}, req_id=3)
    text_payloads: dict[str, str] = {}
    for m in members:
        if m["member_type"] in ("text", "field"):
            num = m["cast_member"]
            try:
                r = eval_expr(
                    f"the text of member {num} of castLib {lib_num}",
                    req_id=1000 + num,
                )
                if r.get("success") and r.get("result_type") == "string":
                    text_payloads[m["name"]] = unquote_string(r["result_value"])
                else:
                    text_payloads[m["name"]] = f"<EVAL_FAILED: {r}>"
            except Exception as e:
                text_payloads[m["name"]] = f"<EXCEPTION: {e}>"

    print(f"[4/4] Listing scripts...", file=sys.stderr)
    scripts_resp = tool("list_scripts", {"cast_lib": lib_num}, req_id=4)
    scripts = scripts_resp.get("scripts", scripts_resp) if isinstance(scripts_resp, dict) else scripts_resp

    out = {
        "cast_lib": lib_num,
        "name": me["name"],
        "swapped_to": swap_url,
        "member_count": me["member_count"],
        "script_count": me["script_count"],
        "members": members,
        "text_bodies": text_payloads,
        "scripts": scripts,
    }
    with open(out_path, "w") as f:
        json.dump(out, f, indent=2)
    size = sum(1 for _ in open(out_path))
    print(f"Wrote {out_path} ({len(json.dumps(out))} bytes, {size} lines)", file=sys.stderr)


def main() -> int:
    args = sys.argv[1:]
    swap_url = None
    if args and args[0] == "--swap":
        if len(args) < 4:
            print(__doc__, file=sys.stderr)
            return 1
        swap_url = args[1]
        args = args[2:]
    if len(args) != 2:
        print(__doc__, file=sys.stderr)
        return 1
    lib_num = int(args[0])
    out_path = args[1]
    dump_lib(lib_num, out_path, swap_url=swap_url)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
