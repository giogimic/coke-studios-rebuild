# SETUP — cokemusic-extractor

How to set up the toolchain and run both extraction rails.

## Release status

Version 0.3.1 pins pushed hardened backend `cfd7329`. Authenticated Rail A
extraction has been verified. Release manifests are authenticated by signed Git
tags. Cast payloads and generated or proprietary asset bundles are outside the
release.

## Prereqs

- **Python ≥ 3.10** (for Rail A — text bodies)
- **Rust 1.95.0 + cargo** (for Rail B — bitmap + sound dumpers), selected by
  the pinned fork's `rust-toolchain.toml`.
- **For Rail A only:** the pinned `wasm32-unknown-unknown` target,
  **wasm-pack 0.14.0**, and Node/Electron. Run `npm run check:toolchain` and
  `npm run build-vm` in the dirplayer checkout rather than invoking ambient
  `wasm-pack` directly.
- **jq** (`brew install jq` on macOS) — used by `setup.sh`
- **Cast files** (`.cct`, `.dcr`) sourced from a community archive — this
  repo ships tools only, no binaries

## One-shot setup

```bash
./setup.sh
```

Reads `dirplayer-rs.lock`, clones the dirplayer-rs fork at the pinned
SHA into `../dirplayer-rs` (sibling to this checkout). Override the
destination with `DIRPLAYER_DEST=...` or pass it as the first arg.

Idempotent — re-run after a lockfile bump to fetch + checkout the new
SHA.

## Environment

Copy `.env.example` to `.env` and fill in the paths you use. The Rail B
dumpers (run from inside the fork checkout) read these from the
environment.

```bash
cp .env.example .env
$EDITOR .env
source .env
```

## Rail B — bitmap + sound dumpers (the routine path)

From the fork checkout root (after `setup.sh` clones the fork as a
sibling):

```bash
cd ../dirplayer-rs    # or "$DIRPLAYER_DEST"

# Backgrounds + per-room sprites for the 24 publicrooms
cargo test -p vm-rust --test dump_cct_bitmaps -- --nocapture

# Studio templates (Studio A-G + suites)
cargo test -p vm-rust --test dump_studio_bitmaps -- --nocapture

# Studio wall/floor pattern CLUT variants
cargo test -p vm-rust --test dump_studio_palette_variants -- --nocapture

# Engine cast libraries (cc_room, cc_furniture, chatengine, etc.)
cargo test -p vm-rust --test dump_engine_bitmaps -- --nocapture

# Every named furniture bitmap plus regPoint metadata
cargo test -p vm-rust --test dump_furniture_bitmaps -- --nocapture

# Avatar-engine + people cast bitmaps and optional UTM comparison
cargo test -p vm-rust --test dump_avatar_bitmaps -- --nocapture

# Recycler mini-game (FurniFactory2.dcr) — bitmaps + sounds
cargo test -p vm-rust --test dump_dcr_bitmaps -- --nocapture

# Recycler sprite-channel and behavior data
cargo test -p vm-rust --test dump_dcr_score -- --nocapture
```

Outputs land at:
- `<OUTPUT_ROOT>/rooms/<room_id>/...` — publicroom PNGs and
  `_members.json` sidecars
- `<OUTPUT_ROOT>/assets/rooms/<room_id>/...` — studio PNGs and sidecars
- `<OUTPUT_ROOT>/assets/rooms/_studio_palette_variants/` — pre-rendered
  studio CLUT variants and their manifest
- `<OUTPUT_ROOT>/ui/...` — engine cast library PNGs
- `<OUTPUT_ROOT>/furniture/data/...` and
  `<OUTPUT_ROOT>/furniture/_cc_furniture_members.json` — furniture PNGs
  and regPoint metadata
- `<OUTPUT_ROOT>/avatars/<cast>/...` — avatar PNGs, metadata, and the
  optional UTM comparison
- `<OUTPUT_ROOT>/games/recycler/` — Recycler mini-game PNGs +
  `_members.json`
- `<OUTPUT_ROOT>/games/recycler/sounds/` — Recycler PCM WAVs +
  `_sounds.json` (one entry per named sound member; channels /
  sampleRate / bitsPerSample / sampleCount / codec / wavByteLength).
  Cast member names are preserved verbatim in the filenames (case
  matters on case-sensitive filesystems).

Recycler outputs are also dual-written to
`/tmp/dirplayer_dumps/recycler/` (and `…/sounds/` for the WAVs) as
a fixed scratch path for debugging.

Output paths are also documented in each dumper's source-file header
inside the fork checkout. These mixed subdirectory conventions describe the
currently pinned hardened backend and must be revalidated for every lock bump.

## Rail A — text bodies + script bodies (rare, heavy)

Only needed if a `.cct` file changes upstream (essentially never for
CokeMusic — the casts are read-only archives) or if `extract_room.py`
itself changes. It drives a live dirplayer-rs Electron instance over MCP,
so it's a multi-step ritual, not a one-liner.

`extract_room.py` dumps, per room: cast members, `text`/`field` bodies
(MapXml, SceneXml, EntryXml, `<room>.room`, …), and every script's
handlers **with their decompiled Lingo source** (`handler_sources` —
e.g. an animator's `init`/`exitFrame`, not just the script name). See
[`SCHEMA.md`](SCHEMA.md) for the output shape.

This assumes a CokeMusic cast tree you supply (this repo ships tools
only) laid out like the original client: the engine/UI casts
(`cc_*.cct`, `isoengine.cct`, …) plus `SF_Client.dcr` at the served
root, per-room casts under `publicrooms/`, and a tiny `Empty.cct` stub.

### Steps

```bash
# 0. Build the WASM VM in the fork checkout (see ../dirplayer-rs).
#    wasm-pack REQUIRES a rustup-managed toolchain that has the
#    wasm32-unknown-unknown std (Gotcha A below).

# 1. Stage a CORS-served copy of the cast tree with cc_messenger[1].cct
#    replaced by the Empty.cct stub (Gotcha C):
STAGED=/tmp/staged-casts
rm -rf "$STAGED" && cp -R /path/to/your/client2 "$STAGED"
cp "$STAGED/Empty.cct" "$STAGED/cc_messenger[1].cct"

# 2. Serve it (background): resolves /SF_Client.dcr, /Empty.cct,
#    /publicrooms/<room>.cct
python3 extract/cors_server.py 8765 "$STAGED" &

# 3. Generate a run-scoped token, then boot dirplayer-rs with MCP forced on.
#    Keep the token exported for curl and the extractor processes.
export DIRPLAYER_MCP_TOKEN="$(node -p 'require("crypto").randomBytes(32).toString("base64url")')"
( cd ../dirplayer-rs && REACT_APP_MCP_FORCE_ENABLED=true npm run electron-dev & )
# Wait for "MCP server listening on http://127.0.0.1:9847".

# 4. Load the base movie so castLib(19) (the "Studio" slot extract_room.py
#    hot-swaps) exists — SF_Client.dcr's preload brings up all 19 engine
#    cast libs. extract_room.py does NOT load it itself (Gotcha D).
#    autoplay:false skips the SmartFox network init; we only need static data.
curl -fsS localhost:9847 \
  -H "Authorization: Bearer $DIRPLAYER_MCP_TOKEN" \
  -H 'Content-Type: application/json' -d '{
  "jsonrpc":"2.0","id":1,"method":"tools/call","params":{
    "name":"load_movie",
    "arguments":{"url":"http://127.0.0.1:8765/SF_Client.dcr","autoplay":false}}}'

# 5. Drive extraction (repeat per room).
python3 extract/extract_room.py <room> <out.json>
```

`extract_room.py` clears + hot-swaps the cct into cast lib 19 inside the
running dirplayer instance, then dumps members + text bodies + scripts
(with `handler_sources`). It writes through a WIP-clobber guard
(`_safe_write.py`) and sorts scripts by `(name, cast_member)` so
re-extractions are idempotent and diff cleanly.

### Gotchas

- **A — rustup toolchain.** `wasm-pack` shells out to `cargo`/`rustc` from
  `PATH`. If those resolve to a non-rustup install (e.g. a Homebrew rust)
  the build fails with *"wasm32-unknown-unknown target not found in
  sysroot"* even when `rustup` has the target. Make sure a rustup
  toolchain with `wasm32-unknown-unknown` wins on `PATH` — e.g.
  `PATH="$HOME/.rustup/toolchains/<toolchain>/bin:$PATH" wasm-pack build
  --target web`. (`~/.cargo/bin` shims can be stale/dangling and silently
  fall through to the wrong rust.)
- **B — enable and authenticate MCP.** The server only starts when
  `localStorage['mcp:enabled']==='true'` AND `isElectron()`.
  `REACT_APP_MCP_FORCE_ENABLED=true` forces it on for headless extraction.
  The server and every extraction client must share the run-scoped
  `DIRPLAYER_MCP_TOKEN`; unauthenticated requests are rejected.
- **C — messenger stub.** Serve a staged cast tree with
  `cc_messenger[1].cct` replaced by the `Empty.cct` stub, or
  `SF_Client.dcr`'s preload panics in `vm-rust/src/director/file.rs`
  (`'CASt'` vs `'CAS\0'` chunk tag).
- **D — base movie first.** `extract_room.py` assumes `castLib(19)` (the
  Studio slot) already exists; load `SF_Client.dcr` (step 4) before
  extracting.

## Lockfile bumps

`dirplayer-rs.lock` pins the fork's URL + branch + SHA. To bump:

1. Land the change on `reowens/dirplayer-rs#cokemusic`.
2. Note the new SHA: `git -C $DIRPLAYER_DEST rev-parse HEAD`.
3. Update `dirplayer-rs.lock`'s `sha` field.
4. Re-run the dumpers; ship the resulting PNG / `_members.json`
   deltas alongside the bump.

Before release, perform those steps from a clean checkout, verify every
documented output path and deterministic manifest twice, then create a signed
annotated release tag. Never point this lock at an unpushed commit.
