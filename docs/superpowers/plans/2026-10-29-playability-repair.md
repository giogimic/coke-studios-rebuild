# Playability repair implementation plan

> **For agentic workers:** Use `superpowers:executing-plans` task by task. Read `docs/verification/PLAYABILITY-HANDOFF.md` before work. Preserve prior uncommitted changes; checkboxes distinguish component progress from player acceptance.

**Goal:** Restore the selected-character -> shared room -> owned studio -> persistent decoration journey in the Go game.

**Architecture:** Repair existing Go/Ebitengine WASM and Go authoritative server. Keep one account/world; use existing storage and transport boundaries. Introduce focused shared helpers only where they remove a demonstrated disagreement.

**Tech stack:** Existing Go 1.26 module, Ebitengine 2.8.8, SQLite/MariaDB, AMF/realtime XML; minimal HTML/JS website/browser APIs.

**Spec:** `docs/BIBLE.md`, `docs/RECREATION-PLAN.md`; concrete defect evidence in `docs/verification/GAME-AUDIT-2026-10-29.md`.

## Constraints and decisions

- Go remains the only player engine. No substitute rooms, avatars, fake successes or seeded multiplayer.
- Preserve archives, licenses, attribution and `coke.db`; all tests use temporary stores.
- Preserve Windows/Linux compatibility; no dependency upgrades or hosting expansion.
- Historical edition remains a candidate, not authenticated parity. Known implementation bugs can be fixed without inventing historical rules.
- Work in the current working tree because it contains the user's unfinished repairs. Do not reset/stash/discard them or commit them indiscriminately. Record this turn's touched files in the ledger.
- User authorized audit, plan and beginning implementation in this turn; proceed without another approval gate. This is a repair of the existing design, not a redesign.
- Keep this plan and ledger in the repository so another agent can resume without this conversation.

## Review focus

1. Account profile with XML-special characters or stale socket appearance: persisted appearance wins, valid XML round-trips (Task 1).
2. Simultaneous joins, invalid destination, old socket close: no missing/phantom players (Tasks 1–2).
3. Slow or rejected login/room entry: never display an apparently connected empty world (Task 2).
4. Selected item followed by append/removal/network snapshot: manipulate only the original possession ID (Task 3).
5. Rejected placement, rotated 2x2 item, reconnect/restart: no visual-only success, missing possessions or anchor drift (Task 5).

## Task 1: Authoritative identity and room presence

**Files:** `sources/gocoke/cmd/server/{main.go,rooms.go,room_presence_test.go}`, focused shared XML decoder under `internal/client/net/realtime/` if needed.

**Interfaces:** Keep `handlePlayerMessage(*Player,string)`, `enterPlayerRoom(*Player,string,string) error`, `spawnAvatarInRoom(*Player,string)` compatible. `Player.ID` is a connection key; account identity remains `ScreenName` plus authenticated database lookup.

- [ ] Add regressions through real login/enter/spawn handlers with two temporary-store accounts; persisted distinct definitions override stale `def`; special characters survive XML decoding; stable connection key survives login.
- [ ] Add invalid room transition preserving existing presence and snapshot/admission ordering tests.
- [ ] Run focused tests and record actual pre-fix failures.
- [ ] Remove client appearance overrides, escape outgoing avatar attributes consistently, retain connection identity, validate destination before old-room removal; make snapshot and admission one serialized operation.
- [ ] Verify focused tests and entire server suite. Client XML decode must round-trip the same escaped representation exactly once.
- [ ] Record limits: transport-free tests do not establish rendered multiplayer.

## Task 2: Accepted connection and room entry, with recovery

**Files:** `internal/client/net/realtime/{client_wasm.go,client.go,room_inbox.go}`, `internal/client/session/session.go`, `scene/{boot/boot.go,entry/entry.go}`, `cmd/client/main.go` (relative to Go module; inspect native client filename).

**Interfaces:** Keep `SendLoginAndWait(context.Context,string,string) error`; add explicit room transition result/state shared by native and WASM clients. `SetRealtimeReady(true)` follows successful login only. Main installs the requested scene after matching room acceptance; `Room.NotFound`, `LoginFailed`, close and timeout clear pending navigation.

- [ ] Write a build-independent state-machine test for delayed login, denied login, accepted/rejected entry, switching during load, close/retry, and old-room packets. Each state transition must be consumed by actual WASM handlers, not an unused test helper.
- [ ] Observe failures, implement state handling and honest visible failure/retry in existing UI. Reuse/close connections deliberately; entering lobby must remove old room presence.
- [ ] Use `roomInbox` ordered buffering, with bounded lifetime and stale target cleanup; avoid relogging on every return to navigator.
- [ ] Build WASM and run focused + full suite. Add actual two-browser Go-canvas test; capture both views and leave blocked execution marked open.

## Task 3: Stable furniture selection and actions

**Files:** `internal/client/scene/studio/{infostand.go,infostand_furni.go,studio.go,placement.go,furniture_selection_test.go}`.

**Interfaces:** Selection uses `PosID`; `Scene` resolves a current `*PlacedFurni` by ID immediately before action. Stale/removed IDs result in no action and cleared selection. No equality comparisons against saved pointers into mutable slices.

- [ ] Test select A -> force append reallocation -> rotate/move A; select A -> remove preceding item -> act on A; remove A -> act -> no action on replacement B.
- [ ] Observe failures; implement stable lookup at the InfoStand callback boundary, retain existing renderer and placement logic.
- [ ] Run studio tests and WASM build. This task repairs intermittent action targeting, not all footprint/rotation rules.

## Task 4: Complete private studio creation

**Files:** `internal/client/ui/navigator/overlay.go`, `scene/entry/entry.go`, `net/gateway/gateway.go`, `cmd/server/{main.go,rooms.go,db.go,studio_creation_test.go}`, recovered navigator creation definitions.

**Interfaces:** Choose the recovered create method/payload only after tracing original navigator handlers and controls; reuse `Store.CreateStudio` transaction and authenticated account identity. Return a full studio record that existing My Studios parsing accepts; generate unique IDs server-side. Caller cannot choose owner.

- [ ] Trace original create controls, layout choices, title validation and response into a small evidence table (paths/hashes/locators). Keep unverified limits explicitly provisional.
- [ ] Add handler+real gateway tests for valid A–D creation, duplicate titles with distinct IDs, invalid layout/name, unauthenticated caller, spoofed owner, failed storage and reopened-store persistence.
- [ ] Implement creation handler/client request and recovered Go dialog with editable title/layout, disabled pending submit, actual errors and cancel. Wire both entry and in-room navigators through the same controller.
- [ ] Refresh My Studios from server and enter created room after acceptance. Verify another account sees correct owner/access. Run tests + Go UI browser journey.

## Task 5: Authoritative decoration and coordinate agreement

**Files:** `scene/studio/{placement.go,pos_furni.go,studio.go}`, `net/realtime/client_wasm.go`, `cmd/server/{main.go,rooms.go,db.go,possessions_test.go}`.

**Interfaces:** One documented grid/anchor conversion shared by placement, update and reload. Server uses persisted possession/catalogue dimensions, ownership and oriented footprint. Pending client actions settle on authoritative POS/POS_RM or explicit denial.

- [ ] Reproduce round-trip drift for 1x1, 2x1, 2x2 and wall items in all supported directions, using actual catalogue records; test four rotations return to same anchor and snapshot matches live update.
- [ ] Derive anchor/rotation and path-map weight meaning from matched original sources. Replace contradictory offsets only after fixture evidence.
- [ ] Test moved furniture clears old occupancy; overlapping/outside-room/other-owner requests reject; failed writes retain backpack/room state. UI must restore pending items on send error/denial/disconnect.
- [ ] Test restart and second-client agreement for buy -> place -> rotate -> move -> pickup. Update full footprint collision and ownership atomically, preserve stored data through migration if schema changes.

## Task 6: Appearance equivalence and final player gate

**Files:** `tools/portal/index.html`, `internal/avatar/`, `internal/client/avatarengine/`, `tests/browser/`, `docs/PARITY.md`.

- [ ] Inventory matching renderer-supported body/hair/clothing IDs and tint data; remove assumptions that independent website image IDs equal Go cast IDs. Keep legacy saved definitions readable.
- [ ] Use a Go-backed preview or shared verified compositing data; saved selection must reproduce exact gender/body/style/colors in both players' game views. No default-avatar substitution on error.
- [ ] Execute the audit's two-player acceptance gate with fresh compiled artifacts and isolated storage; capture 760x520 images, build hashes, logs without secrets, and restart results.
- [ ] Update parity separately for implemented, component-tested, player-verified and historically compared states. Record room-specific, Sumo, music and historical-evidence gaps still outside this repair.

## Verification commands

From `sources/gocoke`, PowerShell:

```powershell
$env:GOPROXY = 'off'
$env:GOCACHE = Join-Path (Resolve-Path ../..).Path '.runtime/go-cache'
go test ./cmd/server -count=1 -timeout=60s
go test ./... -count=1 -timeout=90s
```

Use `tools/build-go.ps1` only when ready to replace packaged artifacts; otherwise compile WASM/server to the task's `.runtime/` scratch directory. Never restore older binaries and call a build successful. Browser checks need an allowed local server; record permission/graphics barriers exactly, without claiming gameplay passed.
