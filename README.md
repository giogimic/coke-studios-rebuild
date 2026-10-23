# Coke Music / Coke Studios Recovery & Preservation Workspace

**Focus Architecture:** Debian 12 (Bookworm) / Linux First  
*(Windows compatibility options provided for dual-environment workflows)*  
**Collection Date:** October 2026  
**Engineering Bible:** [📘 docs/BIBLE.md](docs/BIBLE.md) *(Complete reverse-engineered architecture, formulas, and protocols)*

---

## 🚀 Quick Start (Debian / Linux First)

### 1. Automated Setup & Verification
Run the Debian setup script to inspect prerequisites, set binary permissions, and verify asset integrity:
```bash
chmod +x ./setup.sh
./setup.sh
```
*(On Debian/Ubuntu, prerequisites can also be installed manually via: `sudo apt-get update && sudo apt-get install -y nodejs git curl tar`)*

### 2. Launch Local Servers
```bash
chmod +x ./start.sh ./stop.sh
./start.sh
```
To run in the foreground instead: `./start.sh -f`  
To stop the background servers: `./stop.sh`

### 3. Containerized Deployment (Docker / Compose)
For a completely containerized setup running on Debian:
```bash
docker compose up -d
```

### 4. Windows Compatibility Options
If running directly on Windows:
- Double-click **`Start-Local.cmd`** (or execute `.\Start-Local.ps1` in PowerShell).
- Double-click **`Stop-Local.cmd`** (or execute `.\Stop-Local.ps1`) to shut down.

---

## 🌐 Live Service Endpoints

Once launched, the following endpoints are accessible:

| Service | Port | Description |
|---|---|---|
| **Master Recovery Portal & Launcher** | [http://127.0.0.1:24902/](http://127.0.0.1:24902/) | **Primary entry point**: Character creator, room selector, 129-loop sound player, and unified web gateway. |
| **GoCoke Game Client & Server** | [http://127.0.0.1:24902/game/](http://127.0.0.1:24902/game/) *(or [8080](http://127.0.0.1:8080/))* | **Authoritative Rebuild**: Ebitengine/WASM client with avatars, isometric rooms, mixer, items, AMF gateway, and realtime WebSocket. |
| **CokePhase Demo** | [http://127.0.0.1:8081/](http://127.0.0.1:8081/) | HTML5 Canvas / Isogenic engine remake with 7 public & private rooms. |
| **CSMusicMixer Standalone** | [http://127.0.0.1:8082/](http://127.0.0.1:8082/) | Standalone recreation of the Coke Studios Music Mixer with 56 genre samples. |

---

## 🏆 Project with the "Most Meat on the Bones"

### **GoCoke (`sources/gocoke`)**
**GoCoke** by bolognesandwiches is conclusively the most sophisticated and complete Coke Studios recreation produced to date:
* **Client Architecture:** Built in Go using Ebitengine and compiled to a self-contained ~53 MB WebAssembly binary (`web/main.wasm`). Runs in any modern browser without Flash or Shockwave plugins.
* **Rendering & Systems:** Full isometric tile rendering, multi-layer depth sorting, A* pathfinding, avatar engine (clothing, animations, carry/drink states, emotes), chat bubbles, room navigator, catalogue shopping, backpack inventory, and furniture placing/rotating/stacking.
* **Audio Engine:** Embedded Coke Music Mixer sequencer with 94 original audio loop WAV files (Country, Dance, Hip-Hop, Latin, Pop, Rock) in `web/Lingo/soundlibrary`.
* **Server Protocol:** Implements the legacy StudioCom San Francisco AMF0 remoting gateway and XMLSocket realtime protocol. The included standalone Go server provides authoritative realtime multiplayer, AMF RPC, and Denizen XML authentication.
* **Binaries Built:**
  - `tools/bin/coke-server-linux` & `tools/bin/coke-server.exe` (Standalone Authoritative Game Server)
  - `tools/bin/gocoke-serve-linux` & `tools/bin/gocoke-static-linux` (Linux x86-64 ELF)
  - `tools/bin/gocoke-serve.exe` & `tools/bin/gocoke-static.exe` (Windows x64 PE)

---

## 📦 What is Saved in this Directory

All 20 identified community repositories, the full 84.3 MB Clint preservation pack, and extraction tools have been collected and verified:

| Location | Contents | Upstream / Origin | State |
|---|---|---|---|
| `sources/gocoke` | GoCoke complete source & assets | `bolognesandwiches/GoCoke` | Complete Git clone & compiled binaries |
| `sources/cokephase` | CokePhase HTML5 client | `michaelowens/cokemusic-cokephase` | Complete Git clone & local demo adapter |
| `sources/music-mixer` | Modern Music Mixer | `markhughes/CSMusicMixer` | Complete Git clone & compiled `dist/` bundle |
| `sources/original-client-source` | Decompiled Sulake Lingo sources (2006 & 2007) | `jtieri/Coke-Studios-Source` | Complete Git clone (2,994 cast scripts) |
| `sources/avatar-assets` | Full Coke Studios avatar sprite set | `markhughes/CSAvatarAssets` | Complete Git clone (1,316 sprite assets) |
| `sources/avatar-engine` | Avatar canvas renderer library | `markhughes/CSAvatarEngine` | Complete Git clone |
| `sources/cs-ui` | Coke Studios UI library | `markhughes/CSUI` | Complete Git clone |
| `sources/cs-chat` | Coke Studios chat library | `markhughes/CSChat` | Complete Git clone |
| `sources/cs-website` | Legacy website rebuild | `markhughes/CSWebsite` | Complete Git clone |
| `sources/catalogue-parser` | Catalogue parser & data | `markhughes/CSCatalogueParser` | Complete Git clone |
| `sources/screensavers` | Recovered screensaver files | `markhughes/CSScreenSavers` | Complete Git clone |
| `sources/uncover-the-music` | Uncover The Music game & Colyseus server | `markhughes/uncover-the-music` | Complete Git clone (7,225 files) |
| `sources/yeomans-client` | Early JavaScript remake | `YeomansIII/cokemusic` | Complete Git clone |
| `sources/cokemusic-extractor` | Cast extraction toolchain | `reowens/cokemusic-extractor` | Complete Git clone |
| `sources/cokeitems` | Item analysis & definitions | `bolognesandwiches/cokeitems` | Complete Git clone |
| `sources/cache-finder` | Coke Studios cache finder utility (C source) | `Lost-Media-Hunt/Coke-Studios` | Complete Git clone |
| `sources/shockrays` | ProjectorRays CLI wrapper | `jtieri/shockrays` | Complete Git clone |
| `sources/cokephase-upstream` | Original CokePhase repository | `NewyorkDev/cokemusic` | Complete Git clone |
| `sources/projector-rays` | Director/Shockwave decompiler | `ProjectorRays/ProjectorRays` | Complete Git clone |
| `sources/dirplayer-rs` | Extraction backend | `reowens/dirplayer-rs` | Complete Git clone (pinned commit `cfd7329`) |
| `archives/CokeStudios-The-Complete-Pack.rar` | Legacy Clint Complete Pack (84.3 MB) | Clint / RaGEZONE / h4bbo.net | Verified SHA-256 archive |
| `archives/extracted-complete-pack` | Extracted Clint Complete Pack | Clint / RaGEZONE | Extracted files, sounds, Flash .FLAs, ElectroServer |
| `tools/portal` | Master Web Dashboard | Local | Complete single-page recovery hub |

---

## 🔍 Game Server Architecture Analysis

Investigation of surviving server sources, protocol dumps, and emulators reveals three distinct eras of Coke Music / Coke Studios server architecture:

1. **Sulake Fuse Era (2002–2004):**
   - Similar to early Habbo Hotel (v1–v14).
   - Raw TCP binary/text socket protocol (EVA / VL64 / Fuse protocol).
   - Decompiled Lingo cast scripts in `sources/original-client-source` detail the low-level socket packet builders and message dispatchers.

2. **StudioCom "San Francisco" Era (2004–2007, MyCoke):**
   - **HTTP / AMF0 Gateway:** Flash Remoting gateway (`/sf/gateway`) for RPC calls:
     - Authentication: `/sf/ssldocs/loginDenizen.jsp` returning session credentials.
     - Economy & Inventory: `FlashDenizenManager.getDenizenBalance`, `FlashPossessionManager.getBackpack`.
     - Room Directory: `FlashDenizenManager.getGameServerByStudioID`, `FlashStudioManager.getAllPrivateStudios`.
   - **XMLSocket Realtime Server:** Multi-client TCP socket server framed with NUL (`\x00`) bytes.
     - Flash cross-domain policy handshake: `<policy-file-request/>\x00` -> `<cross-domain-policy>...`.
     - Room lifecycle: `<Room.Enter msgID="n" roomID="..."/>\x00` -> `<Accepted msgID="n"/>\x00` -> `<ST_EN ... layout="n"/>\x00`.
     - Avatar coordination: `<Room.Action><AV id="..." x="..." y="..." z="..." dir="..."/></Room.Action>`.

3. **Clint Private Server Era (2009–2012):**
   - Packaged in `archives/extracted-complete-pack/`.
   - Utilizes **ElectroServer 3** (`ElectroServer3.jar`), a Java XMLSocket server designed for Flash ActionScript 2 clients (`Client.fla`, `Client.swf`).
   - Configured via `configuration.xml` with room definitions and admin user credentials.

---

## 🛠️ Verification & Test Suite

Run the full local test suite:
```bash
node tools/test-local.cjs
node tools/check-browser-assets.cjs
node tools/verify-sources.cjs
```
All tests verify document roots, jQuery/analytics cleaning, 7 room endpoints, catalogue parsing, WebAssembly signatures, MIME headers, and file integrity.

---

## 📜 Credits & Licensing
- **GoCoke:** bolognesandwiches (Ebitengine, WebAssembly client)
- **CokePhase:** Michael Owens & NewyorkDev
- **MusicMixer & Libraries:** Mark Hughes (MIT / AGPL-3.0)
- **Coke-Studios-Source & Shockrays:** jtieri & ProjectorRays team
- **Extraction Tools:** reowens
- **Legacy Preservation:** Clint (RaGEZONE / h4bbo.net)
- **Original IP:** The Coca-Cola Company & Sulake Corporation. Preserved under digital archive fair-use for historical research.
