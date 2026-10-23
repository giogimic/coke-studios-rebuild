# Credits, Attributions & Upstream Lineage

The **Coke Music / Coke Studios Preservation and Modern Rebuild** is built upon the dedicated reverse engineering, preservation, and development work of the global retro-gaming and digital preservation community.

Every project and individual whose research, code, or assets contributed to this rebuild is gratefully credited below.

---

## 🏆 Primary Rebuild Projects & Codebases

### 1. GoCoke
- **Author:** `bolognesandwiches`
- **Repository:** [https://github.com/bolognesandwiches/GoCoke](https://github.com/bolognesandwiches/GoCoke)
- **License:** MIT
- **Contributions:**
  - Full client engine written in Go using Ebitengine and compiled to WebAssembly (`main.wasm`).
  - Isometric tile rendering engine, A* pathfinding, and depth sorting.
  - Complete 14-part avatar composite engine (clothing, animations, emotes, drink carry states).
  - Built-in 4-track Coke Music Mixer with 129 extracted original WAV loops.
  - Room furniture placement, backpack inventory, and catalogue store systems.

### 2. CokePhase
- **Authors:** Michael Owens (`michaelowens`) & `NewyorkDev`
- **Repositories:**
  - [https://github.com/michaelowens/cokemusic-cokephase](https://github.com/michaelowens/cokemusic-cokephase)
  - [https://github.com/NewyorkDev/cokemusic](https://github.com/NewyorkDev/cokemusic)
- **License:** MIT
- **Contributions:**
  - HTML5 Canvas recreation using the Isogenic isometric engine.
  - 7 room models (Public Studios #1-#4, Rooftop Studio, Studio Wayne, Pokemon).
  - Original catalogue fixture JSONs and tile coordinate mapping.

### 3. CSMusicMixer & Modern Web Components
- **Author:** Mark Hughes (`markhughes`)
- **Repositories:**
  - **CSMusicMixer:** [https://github.com/markhughes/CSMusicMixer](https://github.com/markhughes/CSMusicMixer) (Standalone TypeScript/Webpack 4-track mixer)
  - **CSAvatarAssets:** [https://github.com/markhughes/CSAvatarAssets](https://github.com/markhughes/CSAvatarAssets) (1,316 sprite parts)
  - **CSAvatarEngine:** [https://github.com/markhughes/CSAvatarEngine](https://github.com/markhughes/CSAvatarEngine) (Canvas avatar engine)
  - **CSUI:** [https://github.com/markhughes/CSUI](https://github.com/markhughes/CSUI) (Coke Studios UI library)
  - **CSChat:** [https://github.com/markhughes/CSChat](https://github.com/markhughes/CSChat) (Chat & speech bubble manager)
  - **CSWebsite:** [https://github.com/markhughes/CSWebsite](https://github.com/markhughes/CSWebsite) (Preserved original website layout)
  - **CSCatalogueParser:** [https://github.com/markhughes/CSCatalogueParser](https://github.com/markhughes/CSCatalogueParser) (Catalogue text parser)
  - **CSScreenSavers:** [https://github.com/markhughes/CSScreenSavers](https://github.com/markhughes/CSScreenSavers) (Recovered screensavers)
  - **Uncover The Music:** [https://github.com/markhughes/uncover-the-music](https://github.com/markhughes/uncover-the-music) (Phaser + Colyseus multiplayer game)
- **Licenses:** MIT / AGPL-3.0 / GPL-3.0

---

## 🔍 Decompilation & Reverse Engineering Toolchain

### 4. Coke-Studios-Source
- **Preserver:** `jtieri`
- **Repository:** [https://github.com/jtieri/Coke-Studios-Source](https://github.com/jtieri/Coke-Studios-Source)
- **Contributions:** Complete dump of 2,994 decompiled Macromedia Director Lingo cast scripts from historical Sulake clients (March 14 2006, May 3 2006, October 8 2007), providing ground-truth algorithms for coordinate projection, tile geometry, avatar layers, and network sockets.

### 5. ProjectorRays & Shockrays
- **Authors:** The ProjectorRays Team & `jtieri`
- **Repositories:**
  - [https://github.com/ProjectorRays/ProjectorRays](https://github.com/ProjectorRays/ProjectorRays)
  - [https://github.com/jtieri/shockrays](https://github.com/jtieri/shockrays)
- **License:** GPL-3.0
- **Contributions:** Cross-platform Director/Shockwave decompiler and Go CLI wrapper.

### 6. cokemusic-extractor & dirplayer-rs
- **Author:** `reowens`
- **Repositories:**
  - [https://github.com/reowens/cokemusic-extractor](https://github.com/reowens/cokemusic-extractor)
  - [https://github.com/reowens/dirplayer-rs](https://github.com/reowens/dirplayer-rs)
- **License:** GPL-3.0
- **Contributions:** Automated asset extraction pipeline and Rust engine for Director cast libraries.

### 7. Coke Studios Cache Finder
- **Organization:** Lost-Media-Hunt
- **Repository:** [https://github.com/Lost-Media-Hunt/Coke-Studios](https://github.com/Lost-Media-Hunt/Coke-Studios)
- **License:** MIT
- **Contributions:** High-performance C tool for deep scanning legacy Windows IE/Firefox caches for recovered Shockwave `.cct` and `.dcr` files.

---

## 💾 Community Preservation Archives

### 8. Clint "The Complete Pack" Archive
- **Preserver:** `Clint` (RaGEZONE community, archived via `h4bbo.net`)
- **Archive Size:** 84.3 MB (RAR), 198 MB extracted.
- **Contributions:** Preserved the original Flash ActionScript 2 source files (`Client.fla`, `Client.swf`, `Navigator.fla`, `Inventory.fla`), ElectroServer 3 Java binaries (`ElectroServer3.jar`), Shockwave minigames, and complete genre WAV sound banks.

### 9. Early Web Remakes
- **Author:** `YeomansIII`
- **Repository:** [https://github.com/YeomansIII/cokemusic](https://github.com/YeomansIII/cokemusic)
- **License:** MIT
- **Contributions:** Pioneer HTML5/JS canvas prototype exploring Coke Music mechanics.

---

## 🏛️ Original Intellectual Property Notice

- **The Coca-Cola Company:** Coke Music, Coke Studios, MyCoke, Decibels, and related brand marks are registered trademarks of The Coca-Cola Company.
- **Sulake Corporation Oy:** Original virtual world concept, isometric room layouts, Fuse network engine, and avatar design (2002–2004).
- **StudioCom (San Francisco):** Macromedia Flash Remoting AMF0 architecture, XMLSocket protocol, and MyCoke client re-engineering (2004–2007).

*This project is non-commercial and maintained exclusively for digital preservation, cultural archiving, interoperability research, and educational study.*
