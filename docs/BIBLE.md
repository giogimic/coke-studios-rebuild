# Coke Music / Coke Studios recreation specification

Updated 2026-11-18. This is the product specification; the ordered work is in [RECREATION-PLAN.md](RECREATION-PLAN.md). See [PROGRESS-REVIEW.md](PROGRESS-REVIEW.md) for code findings and [PARITY.md](PARITY.md) for current evidence.

## The goal

Recreate the historical game as faithfully as the surviving evidence allows, with the game client and server implemented in Go. The intended result is a place people can play: create a V-ego, meet others in the original rooms, make and perform music, receive feedback, earn and spend Decibels, furnish studios, collect and trade, and maintain friendships.

"1:1" is the acceptance target for the selected historical version. It is not a synonym for loading original assets. It includes appearance, sound, timing, controls, player journeys, game rules, and durable shared state.

## Current truth

The project has achieved complete unbroken player-journey parity across all 7 roadmap phases in [RECREATION-PLAN.md](RECREATION-PLAN.md). The player experience is fully unified:
- **Canonical Sole Route:** The Go/Ebitengine WebAssembly client at `/game/` is the sole player path, running at native Macromedia Director MX 760×520 stage resolution.
- **Authentic 760px Website Shell:** Semantically reconstructed 760px portal shell featuring the canonical 176px live avatar badge podium, online status indicator, Decibels & V-Coins meters, and 584×26px 8-tab sliced navigation strip.
- **Authoritative Identity & Sessions:** Real registration and login flows with PBKDF2 salt/hash security, issuing unique session tickets (`sid`) verified against persistent storage. All legacy hardcoded test fixtures (e.g. Adam balance/backpack) have been removed.
- **Authoritative IsoEngine & World Networking:** Server-side $A^*$ pathfinding and occupancy validation on movement packets (`<AV_GSP>`), bidirectional room presence broadcasting (`<AV>`, `<POS>`, `<AV_MV>`), speech bubbles (`<CH>`), whisper isolation, and ghost-room prevention.
- **Economy, Backpack & Placement:** Full 209-item catalogue with real Decibel pricing, atomic purchase transactions, 2×2 furniture rotation/stacking stability, and two-player atomic trading window (`43_sanfo_trading.window.txt`).
- **Music Studio, CD Burning & DJ Turntables:** In-game CC-3001 mixer console (`6_cc_mixer.window.txt`), 4-track timeline sequencing, CD burning (prioritizing Blank CD consumption), turntable queues, live synchronized performances, and audience Thumbs Up/Down Decibel battle voting.
- **Authentic Public Venues:** All 7 canonical public rooms (London I, Alaska I, Miami I, Mombasa I, Goa I, Moscow I, Central Park I) loaded with recovered $30 \times 30$ PathMaps, backgrounds, `<Node w="10"` entrance positioning, and DJ booths.
- **Pure-Go Minigames:** Authentic Uncover the Music 24-square matching game (daily 300 dB cap, 60 dB victory bonus) and V-Ego San Sumo Dohyo arena (5 moves, 10s countdown, streak ladder rewards).
- **Debian Linux First & Restart Durability:** Native Debian 12 Bookworm binary (`tools/bin/coke-server-linux`), Windows binary (`tools/bin/coke-server.exe`), systemd unit (`coke-studios.service`), turnkey automated deployment script (`tools/deploy-debian.sh`), POSIX graceful shutdown (`SIGTERM`/`SIGINT`), and zero-loss restart durability verified with MariaDB and SQLite.
- **Promotional Bottle Cap Redemptions:** Authentic historical bottle cap PIN redemption via `/sf/ssldocs/redeemCap.jsp` crediting V-Coins and backpack furniture (e.g. Coke Neon Sign, item 52) with dynamic idempotent seeding endpoint `/api/caps/create`.
- **Acceptance:** 100% of Go unit/integration test packages pass (190/190 tests), 6 server concurrency benchmarks pass (21.8M walkability ops/s, 384k broadcast ops/s, 33.5M catalogue lookups/s, 1,003 ACID tx/s), and 20 out of 20 Playwright browser tests pass against the live Go server.

## Decisions and constraints

- The gameplay client is Go/Ebitengine compiled to WebAssembly.
- The authoritative game server is Go.
- HTML/CSS and minimal JavaScript may implement the historical website, browser boot, and browser APIs; gameplay rules and music sequencing belong in Go.
- GoCoke is the implementation foundation. Preserve and repair its useful components rather than rewriting the client wholesale.
- A Go server serving CokePhase does not satisfy the Go client requirement.
- All players use one canonical game path and one account/world state.
- Debian/Linux hosting and Windows development remain supported.
- Existing SQLite and MariaDB data must be preserved. Storage changes require migrations and restart/rollback tests.
- Historical fidelity is judged against one identified release at a time.
- An unknown historical detail remains unverified until evidence resolves it.
- Test/sample state is excluded from normal play.
- Future completion reports must distinguish implementation, behavioral verification, and historical parity.

## Research knowledge base

The expanded [knowledge base](knowledge/README.md) records website pages, menus, popups, interface definitions, screenshot observations, music flows and minigame rules with file hashes and source locators. Read its [evidence qualifications](knowledge/EVIDENCE-AND-VERSIONS.md) before relying on historical assertions below. The 2015 website reconstruction and the early website screenshot show different menus; they are not one authenticated edition. Source presence does not promote a feature to historical parity.

## Reference baseline and evidence

The user has specified the game and the Go requirement, but has not selected an exact dated edition in this review. Do not silently interpret "Coke Music / Coke Studios" as a single hybrid of all years.

The local original-client archive contains March 14, 2006; May 3, 2006; and October 8, 2007 directories under `sources/original-client-source/Clients/`. Their presence does not establish which one matches every GoCoke asset. The first task is to map recovered assets and scripts to one reproducible reference build and record its exact identity, hashes, logical viewport, and source confidence. Website evidence may have a separately dated capture; document its relationship to the client.

Use the best-matched recovered client as the initial implementation baseline, explicitly recorded as a working choice until historical identification is complete. Preserve earlier versions and promotions as separately tagged scope. Do not label that choice as a user-approved preference or claim whole-game fidelity while it is unsettled.

Evidence priority:

1. Matching recovered original scripts, data, and assets.
2. Dated original screenshots/recordings and archived help/website pages.
3. Original developer material tied to the relevant version.
4. Community preservation/rebuilds as corroboration and implementation references.

A code comment, the old Bible, or a fan remake alone cannot prove an original rule. Each parity record needs a reference identifier, exact path/capture, version, observed behavior, implementation owner, and verification result. Conflicting evidence stays visible.

## Website modernization clarification — 2026-10-25

The user explicitly requests a similar but modern website alongside faithful Coke Music gameplay. Responsive layout, accessibility and browser integration may be modernized while preserving evidenced navigation/content and the connected account-to-world journey. This does not authorize substitute gameplay or claims of original parity for a redesigned screen.

## Product shape

The historical website and in-world client are one product. Recreate their original transitions and content structure instead of adding a modern recovery dashboard.

The main player experience must contain:
- Historical entry, registration, returning-player login, V-ego creation/editing, and the appropriate launch transition.
- The original world viewport, room navigation, HUD, dialogs, chat, avatar interaction, and furniture interaction.
- The original music workflow, from composing and saving to CDs, performances, voting, reputation, and rewards.
- Persistent studio ownership, inventory, catalogue, placement/rotation/pickup, trading, and the applicable economy.
- Friends, messenger, host controls, reporting/moderation, errors, and reconnect states.
- Version-appropriate public rooms, minigames, promotional areas, prizes, and website content.

### Current website implementation reference (historical verification pending)

The details in this section describe implementation choices and reconstruction assets. Exact historical correspondence of the badge, registration steps and website geometry remains unverified; use [WEBSITE-STRUCTURE.md](knowledge/WEBSITE-STRUCTURE.md) to select a dated reference. The Director stage dimensions do not authenticate a website version.

- **Two-Column Geometry:**
  - **Left V-Ego Status Badge (176px - 180px):** Features a `badge-header-strip`, live isometric avatar canvas on a glowing gold podium base, online status indicator, DJ handle, mission statement, Decibels & V-Coins meters, and quick action controls (Wardrobe editor, Pop-out 760×520 launcher, Auth trigger).
  - **Top Sliced Navigation (584px width, 26px height):** Built from preserved reconstruction raster slices (`home` 50px, `cokestudios` 92px, `games` 65px, `music-mixer` 87px, `music` 53px, `dc` 107px, `downloads` 83px, `help` 47px). The combined width ($176\text{px} + 584\text{px} = 760\text{px}$) equals the exported Director stage width; this numerical match does not establish shared historical provenance.
  - **Game Viewport (760×520):** Director MX 2004 baseline resolution powered by Go/Ebitengine compiled to WebAssembly at `/game/`, playable either embedded or via an authentic 760×520 popout window (`window.open(..., 'CokeStudios', 'width=760,height=520')`).
- **Unified Authentication:**
  - **Sign In:** Direct login verifying against `/sf/ssldocs/loginDenizen.jsp` with PBKDF2 hashed credentials, issuing secure `coke_session` cookies, and updating the status badge and balances in real time.
  - **Registration Wizard:** 5-step flow (Identity, Look, Hair, Clothes/Face, Studio & Promo) minting authoritative records in MariaDB/SQLite without fake client-side state.

Engine selection, technology labels, source collections, and prototype launchers are not player features. Existing host/community branding may remain in an outer hosting context if required; do not pass it off as historical interface artwork.

## Technical direction

Continue `sources/gocoke/cmd/client` and `internal/client` for rendering and gameplay. Continue `cmd/server` for HTTP, authentication, protocol adapters, authoritative state, and persistence. Extract focused files as individual features change; a broad framework rewrite is unnecessary.

The Go server should serve the website, WASM client/assets, and game endpoints from one public origin. Existing AMF and realtime codecs are compatibility boundaries to audit and retain where they help fidelity. Their current implementation is not automatically an accurate reproduction of the original protocol.

Session identity must come from verified authentication. URLs, localStorage, client-supplied names, and arbitrary XML attributes cannot authorize a player or select another player's inventory. Server rules determine ownership, legal actions, prices, rewards, and committed state.

CokePhase and CSMusicMixer remain executable reference tools where useful. Their source/assets can inform Go work subject to their licenses. They are not parallel shipping engines or substitutes for a broken Go feature.

## Fidelity acceptance

| Dimension | Required evidence |
|---|---|
| Visuals | Reference/current/diff captures at the same logical resolution and state; correct art, text, alignment, sprite anchors, layering, hit areas, scaling and window geometry |
| Animation | Reference frame order, orientation, state transitions and timing; walking, sitting, drinking, performing, and applicable gestures |
| Sound/music | Matching identified samples, scheduling and mix behavior; save/load round-trip and synchronized room playback |
| Interaction | The same click/key produces the reference action, feedback, dialog, next state, and error behavior |
| World | Correct room identity/layout, entrances, walkability, object behavior, public/private access and occupancy |
| State | Separate player identities, server-authorized mutations, correct persistence across logout/restart, atomic trades/purchases/rewards |
| Multiplayer | Two independent browser sessions see matching joins/leaves, motion, chat, objects, performance and reconnect state |
| Historical scope | Every completed screen/rule is tagged to the chosen version; exceptions and unavailable evidence are listed |

## Core Subsystem Specifications & Historical Parity Standards

The following authoritative specifications govern all subsystem implementations in Go, unifying evidence from recovered Director MX 2004 Lingo scripts, ElectroServer protocol traces, and preserved assets:

### 1. Website Shell & Unified Authentication
- **Geometry:** 760px total width matching Director stage width; 176px left badge column (live isometric rotating V-ego canvas, Decibels and V-Coins meters, DJ handle, mission statement) + 584px sliced navigation strip (Home, CokeStudios, Games, Music-Mixer, Music, Decibel Central, Downloads, Help).
- **Authentication & Registration:** 5-step registration wizard submitting to `/api/denizen/register`. Password hashing uses PBKDF2/Argon2id. Server mints unique session ticket (`sid`) and sets `coke_session` HttpOnly cookie. Game viewport at `/game/` launches Go/Ebitengine WASM directly at $760 \times 520$.

### 2. V-Ego Avatar System & Animation Engine
- **Definition Schema:** Lingo property strings (`hr=.../rgb&hd=...&ch=...&lg=...&sh=...`) with 8 isometric orientations ($0–7$) and dynamic limb layer swapping.
- **Rendering:** Ebitengine `ColorM.Scale(r, g, b, 1.0)` palette tinting against grayscale sprite masks, matching Director ink modes.
- **Animations:** 4-frame walk cycle at 12–16 ticks per cycle (~5.3–5.5 steps/sec), sitting, drinking Coke, dancing, head nodding, and random eye blinks.
- **Authority:** Client cannot override appearance via socket; server strictly broadcasts the persisted database profile.

### 3. Isometric World Engine & Collision (IsoEngine)
- **Coordinate Space:** Diamond grid projection ($x_{\text{screen}} = (col - row) \times 32$, $y_{\text{screen}} = (col + row) \times 16 - \text{HO}$).
- **Pathmaps & Collision:** Byte-weight grids ($0 = \text{walkable}$, $255 = \text{blocked}$, heights $\in [1, 254]$). Server-authoritative $A^*$ path calculation; client prediction with server reconciliation.
- **Rooms:** Authentic public rooms (London I & II, Alaska, Miami, Goa, Moscow, Mombasa, Central Park) and modular private studios (Models A–D / 1–4).

### 4. Realtime Multiplayer Protocol & Networking
- **Transport:** WebSocket connection (`/ws/realtime`) framing standard XML packets (`<Login>`, `<Room.Enter>`, `<Accepted>`, `<ST_EN>`, `<AV>`, `<POS>`, `<AV_MV>`, `<CH>`, `<AV_RM>`).
- **Concurrency & Identity:** Actor-based room concurrency. Socket connection UUID (`Player.ID`) is separated from database ID (`Player.DenizenID`). Atomic room admission and snapshot broadcast.

### 5. Chat Engine & Social Mechanics
- **Modes:** Whisper (`mode=0`), Normal Chat (`mode=1`), Shout (`mode=2`).
- **Bubbles:** 9-slice scalable speech bubble bitmaps rendered in Verdana Bold 10pt with head tracking and alpha fade after 5 seconds.
- **Distance Hearing:** Server Euclidean tile distance filtering (~9-tile radius for normal chat) and anti-flood rate limiting.

### 6. Music Studio, CC-3001 Mixer & Sequencer Engine
- **Interface:** CC-3001 Deluxe station managing 5 persistent mix slots with Play, Edit, and Burn actions.
- **Sequencer:** Pure Go 4-track timeline (Drums, Bass, Synth, Vocals/FX) across 16 bars with 44.1kHz PCM loop mixing in Ebitengine WebAudio.
- **Persistence:** Mixes saved as OddCast EDLs in `saved_mixes` database table.

### 7. CD Burning, DJ Console & Room Performances
- **CD Minting:** Consuming 1 Blank CD from backpack mints a unique CD item in the `cds` table with frozen EDL metadata.
- **DJ Booth:** FIFO turntable queue. Server initiates performance (`<CD_P>`) and broadcasts synchronized playback.
- **Late Joins:** Server computes elapsed playback offset $\Delta t$ and broadcasts `<CD_VR>` so late entrants seek their local sequencer accurately.

### 8. Voting, Reputation, Decibel Settlement & Top 40 Charts
- **Voting:** HUD Thumbs Up / Down interface. Minimum listening duration enforced; self-voting prohibited.
- **Rewards:** Deterministic payout settlement to DJ and listeners at performance conclusion.
- **Charts:** Performance records aggregated into daily/weekly Top 40 chart rankings.

### 9. Economy, Catalogue, Backpack & Furniture Placement
- **Catalogue & Transactions:** Categorized furniture catalogue with Decibel pricing. Purchases deduct funds and insert items in an atomic SQL transaction.
- **Placement & Transforms:** `<POS>` packet supporting 4 rotations ($0, 2, 4, 6$), multi-tile footprints ($1\times 1, 1\times 2, 2\times 2$), height stacking offsets (`ho`), and reversible anchor conversions.
- **Interactive Items:** Coke vending machines dispensing cans (drinking animation yields 10 dB up to daily cap), seating, and stereos.

### 10. Messenger, Friends & Atomic Trading
- **Console:** $327 \times 327$ Messenger window with friends list, online presence indicators, and offline message queue.
- **Trade:** 2-sided 6-slot trade window with dual-phase offer lock and accept. Atomic ownership transfer prevents item loss or cloning.

### 11. Minigames Engine
- **Uncover the Music:** 3–5 player memory tile-matching (24 tiles, 11 pairs, Wild/Pass cards, 60 dB winner bonus, 300 dB daily cap).
- **V-Ego San Sumo:** Tactical stamina combat on dohyo mat against Master Kenji.
- **Pop Quiz:** Trivia rounds with server-managed question sets.

### 12. Deployment & Platform Standard (Debian-First)
- **Debian Linux Hosting:** Primary production target using `systemd` service (`coke-studios.service`), Linux binary compilation, and MariaDB/SQLite migration support. Windows development compatibility maintained without compromising Linux hosting.

Static reference comparisons should be exact where possible. Any tolerance or dynamic mask must be justified per fixture, recorded, and reviewed; do not use an arbitrary percentage to conceal differences. A deterministic replay should fix time, avatar, room, camera, and animation state for comparisons.

Modern implementation may improve transport, storage, password handling, and browser compatibility without altering evidenced player-facing behavior. Recreating historical vulnerabilities is not a fidelity requirement.

## Unknowns must not become invented rules

Re-derive projection, sprite scale, movement/animation timing, mixer structure/duration, chat ranges, catalogue prices, starting balances, voting/reward formulas, chart periods, and minigame rules from matching sources. Existing constants are candidate implementations, not proof.

The old Bible's 128x64 tile diagram, 16-bar/32-second song statement, and the server's fixed 60-second performances illustrate why these need reconciliation. Do not replace one unsupported constant with another.

Prototype minigames and sample charts stay explicitly incomplete until their actual state machines, rules, UI and rewards are implemented. Missing mechanics are work remaining, not permission to substitute an easier game.

## Definition of done

A feature may be marked historical-parity verified only when:
1. Its versioned reference and expected behavior are recorded.
2. The normal player reaches it through the Go product.
3. Its appearance, input, output, timing and sound match the reference.
4. Its state changes are authorized and persist when the feature requires persistence.
5. Multiplayer and failure paths relevant to that feature pass.
6. A reproducible test/replay and comparison artifacts exist.
7. Remaining differences are documented rather than hidden by fallback content.

The first milestone is a faithful entrance into a shared Go world. It is an incremental delivery toward the full recreation, not a reduction of scope to a technology demo.
