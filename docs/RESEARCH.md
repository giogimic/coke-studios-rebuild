> Status note (2026-10-24): This is an earlier acquisition/research summary. Its completion, historical-architecture and fidelity claims have not all been substantiated. Use [BIBLE.md](BIBLE.md), [PROGRESS-REVIEW.md](PROGRESS-REVIEW.md), and [PARITY.md](PARITY.md) for current direction and verified status. Source collection is evidence gathering, not game completion.

# Source and Release Findings: Coke Music / Coke Studios

Updated: October 2026. Complete collection and verification of publicly discoverable rebuild, emulation, and preservation work.

---

## 1. Primary Rebuilds & Playable Clients

### 1. **GoCoke** (`sources/gocoke`)
- **Repository:** https://github.com/bolognesandwiches/GoCoke (Commit: `227be2c`)
- **Status:** Complete Git clone, native Linux/Windows binaries compiled in `tools/bin/`, and WASM client verified.
- **Evaluation:** **The single strongest released codebase.** Written in Go and compiled to WebAssembly using Ebitengine. Contains an embedded isometric rendering engine, multi-layer depth sorting, A* pathfinding, full avatar customizer (head, hair, shirt, pants, shoes, colors, animations, carry/drink/emote states), chat engine with speech bubbles, navigator, catalogue parsing, backpack inventory, and interactive music mixer sequencer with 94 extracted original audio loops.
- **Server Communication:** Interacts via Flash Remoting AMF0 (`/sf/gateway`) and WebSocket XMLSocket bridge (`/ws/realtime`).

### 2. **CokePhase** (`sources/cokephase`)
- **Repository:** https://github.com/michaelowens/cokemusic-cokephase (Commit: `94539b8`)
- **Fork of:** https://github.com/NewyorkDev/cokemusic (also saved in `sources/cokephase-upstream`)
- **Status:** Complete Git clone.
- **Evaluation:** HTML5 Canvas client running on the Isogenic Engine. Renders 7 public and private rooms with furniture and catalogue fixtures. Serves via Node adapter without needing original PHP/Apache/Grunt environment.

### 3. **CSMusicMixer** (`sources/music-mixer`)
- **Repository:** https://github.com/markhughes/CSMusicMixer (Commit: `c285b73`)
- **Status:** Complete Git clone, fully built with Webpack into `dist/`.
- **Evaluation:** Standalone modern web recreation of the MyCoke Music Mixer. Built with TypeScript and Howler.js audio engine. Includes 56 bundled high-fidelity WAV sound samples across 6 classic genres (Country, Dance, Hip-Hop, Latin, Pop, Rock), timeline scrubber, track import/export, and playback controls.

### 4. **Uncover The Music** (`sources/uncover-the-music`)
- **Repository:** https://github.com/markhughes/uncover-the-music (Commit: `affc581`)
- **Status:** Complete Git clone (7,225 files).
- **Evaluation:** Minigame rebuild combining a Phaser frontend client with a Node.js / Colyseus authoritative multiplayer game server in `server/`.

---

## 2. Sulake Decompiled Client Sources & Preservation

### 1. **Coke-Studios-Source** (`sources/original-client-source`)
- **Repository:** https://github.com/jtieri/Coke-Studios-Source (Commit: `4d29246`)
- **Status:** Complete Git clone (2,994 files).
- **Contents:** Full decompiled Macromedia Director / Shockwave Lingo cast scripts extracted from three historical client eras:
  - March 14, 2006 Client
  - May 3, 2006 Client
  - October 8, 2007 Client
  - Recycler & Moderator Tool source scripts
  - All room scripts (Atlantis, Casting Call, Central Park, London, New Orleans, Red Room, San Francisco, Seattle, Sydney)

### 2. **Legacy Clint Complete Pack Archive** (`archives/`)
- **Source:** RaGEZONE release mirror (`h4bbo.net/archive/HabboStuff/`)
- **Archive:** `archives/CokeStudios-The-Complete-Pack.rar` (84,308,458 bytes, SHA-256 verified)
- **Extracted:** `archives/extracted-complete-pack/`
- **Contents:**
  - ElectroServer 3 Java server (`ElectroServer3.jar`, `configuration.xml`)
  - ActionScript 2 Flash client source (`Client.fla`, `Client.swf`)
  - Navigator and Inventory Flash sources (`Navigator.fla`, `Inventory.fla`)
  - Complete sound mixer audio library (Country, Dance, Hip-Hop, Latin, Pop, Rock WAV dumps)
  - Public room cast dumps and Shockwave minigames (Coaster, Sumo, Config V-ego, Messenger)

---

## 3. Reverse Engineering & Extraction Toolchain

- **`sources/projector-rays`**: Macromedia Director decompiler (`ProjectorRays/ProjectorRays`).
- **`sources/shockrays`**: Decompilation management wrapper (`jtieri/shockrays`).
- **`sources/cokemusic-extractor`**: Modern extraction pipeline for JSON/PNG/WAV assets (`reowens/cokemusic-extractor`).
- **`sources/dirplayer-rs`**: Cast extractor backend in Rust (`reowens/dirplayer-rs`, pinned to `cfd7329`).
- **`sources/cache-finder`**: C utility to recover cached Shockwave/Director assets from old disk partitions (`Lost-Media-Hunt/Coke-Studios`).
- **`sources/cokeitems`**: Furniture item catalog schema and metadata (`bolognesandwiches/cokeitems`).

---

## 4. UI & Component Libraries (Mark Hughes Suite)

- **`sources/avatar-assets`**: 1,316 avatar body, clothing, and accessory sprites.
- **`sources/avatar-engine`**: Standalone HTML5 canvas avatar rendering library.
- **`sources/cs-ui`**: Reusable Coke Studios UI dialog and widget library (AGPL-3.0).
- **`sources/cs-chat`**: Coke Studios isometric speech bubble renderer.
- **`sources/catalogue-parser`**: Parser for `Catalogue_English.txt`.
- **`sources/cs-website`**: Early pre-MyCoke project website recreation.
- **`sources/screensavers`**: Extracted Coke Studios screensavers.
- **`sources/yeomans-client`**: Early community JavaScript client attempt.

---

## 5. Live Architecture & Hosting

- **Port 8000:** Master Recovery Portal (`tools/portal/index.html`).
- **Port 8080:** GoCoke Ebitengine/WASM Client.
- **Port 8081:** CokePhase HTML5 Engine.
- **Port 8082:** CSMusicMixer Standalone Application.
- **Debian / Linux First:** Native shell scripts (`setup.sh`, `start.sh`, `stop.sh`), `Dockerfile`, `docker-compose.yml`, and systemd unit (`coke-studios.service`).
