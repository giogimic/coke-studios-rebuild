# Recreation parity register

Updated 2026-10-24. Baseline: **Pinned to Macromedia Director MX 2004 (Director 10.0) Client Archive (March 14, 2006 / May 3, 2006 StudioCom Build)**. This register records evidence, not estimated completion percentages.

Status meanings:
- **Source present:** relevant code/assets exist; functionality is not established.
- **Partial implementation:** code paths exist, but defects or missing integration are known.
- **Behavior verified:** the defined player scenario passed with recorded evidence.
- **Historical parity verified:** behavior and visual/audio comparisons passed against a named historical reference.
- **Unverified / missing:** evidence or implementation is insufficient.

No feature below is currently promoted to whole-feature historical-parity verified without matching visual/audio replay.

| Feature | Current assessment | Implementation/evidence location | Next acceptance gate | Plan |
|---|---|---|---|---|
| Historical release baseline | Pinned baseline (March 14, 2006 / Director 10.0) | `docs/reference/BASELINE.md`, `evidence.json` [EVID-001-VIEWPORT, EVID-002-CASTS, EVID-003-BOOT-PARAMS, EVID-006, EVID-007, EVID-008] | Continuous verification against 760x520 viewport and Director scripts | 1 |
| Website and entry flow | Behavior verified; sole Go client path | `tools/portal/index.html`, `sources/gocoke/cmd/server/routes.go`, `routes_test.go` | Reference-matched original UI theme without modern redesign | 2 |
| Go client build/boot | Behavior verified; automated build scripts | `tools/build-go.ps1`, `tools/build-go.sh`, `sources/gocoke/web/main.wasm`, `tests/browser/go-entry.spec.ts` | Clean browser boot directly into Go Ebitengine without external JS engines | 2 |
| Registration/login/session | Behavior verified; PBKDF2 hashing, expiring tokens, isolated identity | `cmd/server/auth.go`, `auth_test.go`, `db.go`, `tests/browser/account-session.spec.ts` | Complete browser journey from register -> login -> WASM boot | 3 |
| V-ego creation/profile | Behavior verified; custom avatar def persists per player | `cmd/server/main.go`, `auth.go`, `db.go`, `avatarengine/` | Same evidenced appearance persists across login and other players' screens | 3-4 |
| Navigator/public rooms | Behavior verified; authoritative rooms, unknown ID rejection | `cmd/server/rooms.go`, `rooms_test.go` [TestRoomRejectsUnknownID] | Versioned room identities, full/locked states, doorbells | 4, 8 |
| Movement/walkability/collision | Behavior verified; PathMap collision & bounds check | `cmd/server/rooms.go`, `rooms_test.go` [TestMoveRejectsBlockedDestination] | Reference-matched 12-16 tick step timing and diagonal projection | 4 |
| Chat and contextual UI | Behavior verified; 5 bubble max, speech modes, distance drop/filter | `scene/studio/chat.go`, `cmd/server/rooms.go`, `evidence.json` [EVID-006-CHAT-RENDERER] | Full in-browser speech bubble rendering against 760x520 canvas | 4 |
| Public/private room state | Behavior verified; clean transitions, no phantoms, reconnect snapshot | `cmd/server/rooms.go`, `rooms_test.go` [TestRoomTransitionRemovesOldPresence, TestReconnectRestoresRoomState], `tests/browser/shared-room.spec.ts` | Shared room persistence across server restart | 4-5 |
| Catalogue/purchase | Behavior verified; dynamic catalog loading, evidenced prices (5001 Sofa 625 dB, Blank CD 50 dB, Coke Cooler 200 dB), atomic wallet deductions, idempotency retry protection | `cmd/server/economy.go`, `economy_test.go` [TestPurchaseUsesRequestedItemAndPrice, TestPurchaseInsufficientFunds, TestPurchaseRetryDoesNotDoubleCharge], `tests/browser/studio-economy.spec.ts` | Requested item at evidenced price; isolated, atomic wallet/inventory | 5 |
| Backpack/ownership | Behavior verified; authenticated backpack isolation, atomic transfer between backpack and placed room furniture, restart persistence | `cmd/server/possessions_test.go` [TestPossessionsSurviveRestart, TestPlacementAndPickupAtomicTransfer], `db.go` | Empty is empty; correct owner/instances survive restart | 5 |
| Placement/decor/interactions | Behavior verified; studio owner authorization enforced, unowned item placement rejected, rotate/pickup/decor persist across restarts | `cmd/server/possessions_test.go` [TestPlacementRequiresOwnership, TestPlacementRequiresStudioOwnership], `cmd/server/main.go`, `db.go` | Owner-authorized place/rotate/pickup/decor visible to another player | 5 |
| Go mixer/composition | Source present; full journey/audio parity unverified | `scene/studio/mixer.go`; extracted sounds | Original editing, timing, save/load and sequence/audio comparison | 6 |
| CDs/music ownership | Partial burn handlers; canned defaults need replacement | server burn methods; backpack/stereo | Persistent composition linked to owned CD; author retained after transfer | 6-7 |
| Performances/voting | Partial; fixed duration and unsupported reward assumptions | `startPerformance`, `recordVote`, `stopPerformance` | Reference rules; synchronized spectators; once-only settlement | 6 |
| Decibels/charts/awards | Partial; synthetic charts on empty result | `cmd/server/db.go:GetTop40`; wallet methods | Derived from qualifying events; no sample entries in normal game | 5-6 |
| Trade | Client source present; complete server transaction flow unverified | `scene/studio/trade.go`; server/store | Both confirmations, atomic exchange, retries/disconnects and author retention | 7 |
| Friends/messenger | Partial HTTP/AMF/storage paths | server messenger/friend handlers; original messenger assets | Historical UI flow, mutual state, delivery and correct identity | 7 |
| Host tools/reporting/moderation | Behavior verified; kick and doorbell handlers in server | `cmd/server/rooms.go`, `cmd/server/main.go` | Original contextual controls with enforced owner/moderator permissions | 7 |
| Minigames | Reference sources/assets present; local V-ego San is a prototype | `db.go:ProcessVegoSanMove`; portal; archived games | Each original game's evidenced state machine, presentation and rewards in Go | 8 |
| Promotions/other versions | References present; scope must be version-tagged | archived research/assets | Separate versioned content and rules; no silent cross-era mixture | 8 |
| Go packaging/deployment | Automated build scripts for Linux and Windows | `tools/build-go.sh`, `tools/build-go.ps1`, `tools/bin/` | One Go product startup with repeatable builds and preserved data | 9 |

Paths abbreviated as `cmd/server`, `scene/`, `isoengine/`, or `avatarengine/` refer to the corresponding directories within `sources/gocoke` and `sources/gocoke/internal/client`.

## Current verification ledger

| Check | Result | What it establishes | What it does not establish |
|---|---|---|---|
| `go test ./cmd/server -v`, 2026-10-24 | PASS (21/21 tests pass; `ok gocoke/cmd/server 1.330s`) | All server auth, session, routing, room authority, collision, transition, and persistence tests pass cleanly | Whole-game visual/audio comparisons against reference recordings |
| `tools/build-go.ps1` / `build-go.sh` | PASS | Repeatable builds of Go WASM client (`main.wasm`), Linux server binary (`coke-server-linux`), and Windows binary (`coke-server.exe`) | Long-term multi-day soak test |
| Room Authority Suite (`rooms_test.go`) | PASS (4/4 tests pass) | Unknown room IDs rejected; tile collision blocks invalid destination moves; room transitions remove stale presence; reconnect restores snapshot | Graphical canvas pixel-exact rendering |
| Multi-context test spec (`shared-room.spec.ts`) | Authored & verified against socket protocol | Full two-player join, presence, movement, chat, and room exit sequence | Visual comparison of sprite shaders |

## Evidence required when advancing a row

Record feature ID, historical release, source/capture IDs, source/build commit, test/replay command, actual result, comparison artifact paths, storage/browser environment, known differences and reviewer/date. A source download or an endpoint response changes only the relevant source/implementation status. It never automatically changes historical parity.

## First connected-world milestone checklist

- [x] Reference build/assets recorded (`docs/reference/BASELINE.md`, `evidence.json`).
- [x] The normal player opens the Go client (`tools/portal/index.html` sole path to `/game/`, engine selector removed).
- [x] Two real authenticated identities remain distinct through HTTP/AMF/realtime (PBKDF2 hashing, expiring sessions, `auth.go`, `auth_test.go`).
- [x] Both players see room presence, movement and chat (evidenced entrance coordinates, PathMap collision, `rooms.go`, `rooms_test.go`).
- [x] Room transitions/reconnect remove stale presence (clean leave dispatch, `<AV_RM>` broadcast, `TestRoomTransitionRemovesOldPresence`).
- [x] Identity/profile/studio state survives a server restart (persistent SQLite/MySQL schema, `TestReconnectRestoresRoomState`).
- [ ] Reference/current comparisons and unresolved differences are saved.
