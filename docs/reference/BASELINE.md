# Coke Studios Historical Baseline Specification

> Research qualification, 2026-10-24: this is a working reference candidate. March 14 and May 3 are separate archive labels; their file-level association to the GoCoke exports is not authenticated by Movies.csv. The 760x520 stage observation is supported. The Retina interpretation, FPS ranges and combined release identity below remain unverified. See [evidence and versions](../knowledge/EVIDENCE-AND-VERSIONS.md), [music](../knowledge/MUSIC.md), and [hashed observations](../knowledge/observations.json).

**Document Date:** 2026-10-24  
**Reference Release Target:** Macromedia Director MX 2004 (Director 10.0) Client Archive (March 14, 2006 / May 3, 2006 StudioCom Build)  
**Primary Evidence Corpus:**
- `sources/original-client-source/Clients/March 14, 2006/`
- `sources/original-client-source/Clients/May 3, 2006/`
- `sources/gocoke/web/Lingo/` (`Movies.csv`, `SF_Client/`, `SF_Gateway/`, `cc_room/`, `avatarengine/`, `isoengine/`)

---

## 1. Core Viewport & Display Dimensions

- **Director Stage Resolution:** 760 x 520 pixels.
- **Stage Rect (from `Movies.csv`):** `(89, 50) => (849, 570)`
  - Logical Width: $849 - 89 = 760$ px
  - Logical Height: $570 - 50 = 520$ px
- **Internal Double-Buffer `drawRect` (from `SF_Client/2_MAIN.ls`):** `rect(0, 0, 1520, 1040)` ($2\times$ retina / pixel multiplier for crisp scaling).
- **Target Frame Rate (FPS):**
  - Stage FPS: 24-30 FPS.
  - Avatar Walk/Animation FPS: 12-16 ticks per cycle (5.3 - 5.5 step transitions per second).

---

## 2. Cast Libraries & Asset Packages

From `SF_Client/Casts.csv` and `SF_Client/2_MAIN.ls`:
1. `Internal`
2. `Entry` (`entry.cst`) - Front door splash, news, loader
3. `Interface` (`cc_interface[1].cst`) - Buttons, dock widgets, dialog chrome
4. `Navigator` (`cc_navigator[1].cst`) - Public studio directory, search, favorites
5. `Gateway` (`SF_Gateway.cst`) - AMF remoting client and XML parsers
6. `ChatEngine` (`chatengine.cst`) - Word bubbles, filter, speech modes
7. `Room` (`cc_room.cst`) - Infostand, voting, DJ console UI, backpack
8. `FurnitureSmall` (`cc_furniture_small.cst`) - Catalog thumbnails, icons
9. `Catalogue` (`cc_catalogue[1].cst`) - Furni browser, purchase dialog
10. `Messenger` (`cc_messenger[1].cst`) - Friends list, instant messages, invites
11. `AvatarEngine` (`avatarengine.cst`) - Figure compositing, direction matrices
12. `Furniture` (`cc_furniture[1].cst`) - In-world furniture sprite definitions
13. `IsoEngine` (`isoengine.cst`) - Tile math, depth sorting, occlusion
14. `SoundLibrary` (`soundlibrary.cst`) - WAV audio loops, UI sound effects
15. `People` (`people.cst`) - Character sprite atlases (body, clothes, hair)
16. `Items` (`cc_items[1].cst`) - Collectibles, badges, trophies
17. `Sequencer` (`sequencer.cst`) - Sound scheduling and mix events
18. `Mixer` (`mixer.cst`) - 4-track timeline, volume controls, CD burner
19. `Studio` (`Empty.cst` / dynamic CST per room) - Studio model layouts

---

## 3. Client Boot & Authentication Parameters

From `SF_Client/28_LoginScreenName.ls`:
- **`sw1`:** Base64-encoded ScreenName (or empty if unauthenticated).
- **`sw2`:** Session secret / ticket (`sid`).
- **Initial Verification:**
  - Client sends HTTP GET `/sf/ssldocs/loginDenizen.jsp?sidempty=<sid>` (or AMF call `FlashDenizenManager.loginDenizen`).
  - Authoritative Go server verifies the session ticket in the persistent database.
  - On success, returns XML `<LoginResult>` with `ErrorCode=0`, `ScreenName`, `Gender`, `AvatarDefinition`, `MissionStatement`, `Secret`.
  - On failure or expired session, returns `<LoginResult><ErrorCode>1</ErrorCode></LoginResult>`, triggering the in-game login prompt.

---

## 4. Authoritative Protocol & Sequence

### A. Room Transition (`SF_Client/19_ENTER STUDIO.ls`):
1. Client requests room server via AMF `FlashStudioManager.getGameServerByStudioID(studioID)`.
2. Client opens realtime socket connection.
3. Client sends `<Login username="..." msgID="1"/>` with session ticket.
4. Client sends `<Room.Enter roomID="..." msgID="2"/>`.
5. Server verifies authorization and sends:
   - `<Accepted msgID="..."/>`
   - `<ST_EN layout="..." roomID="..." name="..." owner="..." wp="..." fl="..." db="..."/>` (room definition)
   - `<Room.Action><ST_DECOR wp="..." fl="..." db="..."/></Room.Action>` (room decor)
   - `<Room.Action><AV av="..." x="..." z="..." d="..." g="..." m="..." def="..." act="..."/></Room.Action>` (each present avatar)
   - `<Room.Action><POS id="..." cid="..." x="..." y="..." z="..." ho="..." d="..." st="..." iaot="..."/></Room.Action>` (each placed furniture item)
   - If performance active: `<Room.Action><CD_P .../><CD_VR .../></Room.Action>`.

### B. Movement & Action:
- Client sends `<AV_GSP roomID="..."><P x="..." z="..."/></AV_GSP>`.
- Server validates that destination is within bounds and not blocked.
- Server broadcasts `<AV_MV av="..." x="..." z="..."/>` to all room occupants.

### C. Chat:
- Client sends `<CH text="..." mode="..."/>` (0 = Whisper, 1 = Chat, 2 = Shout).
- Server sanitizes, validates range, and broadcasts `<CH av="..." m="<hex>"/>`.

---

## 5. Unresolved / Unknown Historical Items

1. **Daily Decibel Cap:** Historical evidence suggests 100 dB/day from drinking Coke (10 drinks @ 10 dB). Verified in recovered FAQ notes; marked provisional until cross-referenced with server logs.
2. **Top 40 Reward Payout Calculation:** Exact formula for weekly DJ trophy distribution is not preserved in client Lingo; preserved in database as qualified play events.
3. **Minigame Exact Rules:**
   - Pop Quiz questions and answers exist in recovered Text_English.txt.
   - V-ego San Sumo: Master Kenji's internal state machine logic was client/server hybrid. Kept isolated from core world room loop.

---

## 6. Deterministic Reference Test States

1. **State 1: Initial Public Room (Central Park Studio - VIS$0):**
   - Logical dimensions: 760 x 520
   - Layout ID: 2
   - Default entrance coordinate: (1, -1) -> (5, 5)
2. **State 2: Private Studio (Model B):**
   - 10x10 isometric grid with corner blockages
   - Entrance at (5, -1)
3. **State 3: Two-Player Synchronized Session:**
   - Player 1: "Alice", avatar `hr=001/1&hd=001/1&ch=001/1&lg=001/1&sh=001/1`
   - Player 2: "Bob", avatar `hr=003/1&hd=002/1&ch=004/1&lg=002/1&sh=002/1`
   - Both authenticated with separate valid credentials, in room `studio_model_b`.
