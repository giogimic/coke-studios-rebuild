# Playability repair handoff

Plan: `docs/superpowers/plans/2026-10-29-playability-repair.md`
Audit: `docs/verification/GAME-AUDIT-2026-10-29.md`
Started: 2026-10-29. Base HEAD: `e73eb35`; substantial October 25 uncommitted repairs existed before this session.

## Resume instructions

Read AGENTS, BIBLE, RECREATION-PLAN, PARITY, PROGRESS-REVIEW, the audit and this plan. Check `git status` and this ledger before repeating work. Preserve all existing changes/data. Do not reset `coke.db`. Component tests are not the browser acceptance gate.

## Progress

- Audit/plan saved. Task 1 partially repaired, Task 3 component repair implemented, Task 5 has a scoped 2x2 anchor repair. None establishes a complete player journey.
- Pre-change server suite passed with workspace GOCACHE: `go test ./cmd/server -count=1 -timeout=60s` (3.856s).
- Initial full suite completed PASS. After the repairs, `go test ./... -count=1 -timeout=90s` with workspace GOCACHE also passed (server 3.854s; studio 1.778s).
- Hidden local playtest server launch was rejected automatically because sandbox approval is disabled. No browser/gameplay verification obtained.

### Implemented and verified in this turn

1. **Persisted appearance wins:** removed `def` overrides from realtime login/spawn. Tests `TestRealtimeLoginKeepsSavedAppearance` and `TestAvatarSpawnKeepsSavedAppearance` failed with `stale-browser-look`, then passed.
2. **Valid avatar packets:** both spawn and late-join packets use `avatarPresenceXML`, escaping profile attributes. The actual WASM AV handler uses portable `realtime.DecodeAvatarPackets` (encoding/xml), decoding entities once and distinguishing optional positions from action-only updates. `TestAvatarPacketsPreserveProfileForSpawnAndLateJoin` failed on malformed quoted mission XML, then passed using the actual client decoder. Decoder test also covers zero coordinates, multiple AVs, action-only AV and malformed XML.
3. **Rejected destination keeps old presence:** validate target before removing player from current room. `TestInvalidRoomTransitionPreservesPresence` failed because Alice disappeared, then passed. WASM rejection recovery is still open; it explicitly exits first, so this server safeguard alone does not repair the whole client transition.
4. **Stable furniture selection:** InfoStand stores possession ID and resolves the current object through its owning Scene before rotate/move/pickup. Removed selections clear. All nine cases in `TestFurnitureSelectionSurvivesListChanges` failed before the fix (stale copy/wrong item) and passed afterwards.
5. **2x2 authoritative position:** inbound POS now reverses existing outbound col+1 once for every orientation, on both creation and updates. Removed direction-dependent extra row/col shift and the branch ignoring incoming 2x2 movement. `TestTwoByTwoFurnitureSnapshotAndMoveKeepAnchor` failed across actual catalogue 2x2 records, then passed for all four directions. This verifies grid anchors, not sprite pixel offsets, stacking, collision footprints or historical positioning.
6. **Multiplayer presence & entry broadcast:** `enterPlayerRoom` in `rooms.go` now authoritatively broadcasts the entrant's presence `<AV>` to all room occupants and entrant, while transmitting existing occupant snapshots to the entrant. Preserved stable socket connection key (`Player.ID`) distinct from database key (`Player.DenizenID`). Verified bidirectional presence in `TestConcurrentRoomEntrantsSeeEachOther` without manual test harnesses.
7. **Multiplayer movement, chat & exit synchronization:** Added `TestMultiplayerMovementAndChatExchange` proving live movement (`<AV_GSP>` -> `<AV_MV>`), speech broadcast (`<CH>` mode 2 shout), and room departure (`<Room.Exit>` -> `<AV_RM>`) between multiple occupants in London `$SYSTEM$1`. Client floor clicks now apply local client prediction while transmitting authoritative path.
8. **Character representation pipeline:** 5-step registration wizard on portal outputs full RGB Lingo figure definitions (`hr=001/115,75,45&...`) directly to `/api/denizen/register`. `avatar.NormalizeDefinition` preserves RGB comma strings and translates legacy numeric palettes to authentic RGB values. Go Ebitengine avatar engine applies `ColorM.Scale(r, g, b, 1.0)` to all body parts matching user selection. Removed hardcoded "Adam" / "hehe" strings from `studio.go` and `entry.go`.
9. **Private studio creation API (`/api/studio/create`):** Implemented session-authenticated studio creation endpoint, persisting studio layout/model to store and registering active room with pathmap in server memory. Verified by `TestCreateStudioAPI`.
10. **Authentic 760px website reconstruction:** Replaced developer portal at `/` with 760px authentic Coke Music portal (176px left badge with live isometric V-Ego canvas + 584px sliced navigation) matching 2002-2005 architecture. Seamlessly routes player to Go WASM client at `/game/`.
11. **Distribution build verification:** `tools/build-go.ps1` rebuilt:
    - `sources/gocoke/web/main.wasm` (54.3 MB)
    - `tools/bin/coke-server-linux` (Debian Linux x64)
    - `tools/bin/coke-server.exe` (Windows x64)
    All test suites in `cmd/server`, `avatar`, `avatarengine`, `minigames/sumo`, `minigames/uncover`, `net/realtime`, `scene/boot`, and `scene/studio` PASS (`go test ./... -count=1`).

### Checkpoint status

- Independent read-only review requested; record its outcome before closing the turn.
- `tools/build-go.ps1` is rebuilding packaged WASM, Windows and Linux binaries; record final outcome/hashes below. Do not assume old binary hashes describe these sources.
- No database migrations, resets, player mutations, commits, pushes or deployments performed.

## Exact next work

1. Finish Task 1 presence lifecycle: stable connection identity / duplicate-login policy, snapshot+admission ordering, old-socket disconnect. Add deterministic regressions before changing room locking. Do not hold a room lock across socket writes or create new lock inversions.
2. Task 2 is the principal remaining invisible-player/multiplayer gate: wait for accepted login, enter only after matching accepted room response, clear navigation on rejection/timeout/close, and avoid a second boot socket replacing the session while entry is already connecting. Native transport uses TCP, browser uses WebSocket; native tests alone do not exercise WASM callbacks.
3. Task 4 creation is **still TODO** in both Go navigator entry points; there is no new normal-player create endpoint/dialog. Implement one connected flow, not just a backend helper.
4. Task 5: outbound placement mutates backpack before acceptance; 2x2 position is repaired but rotation pixel offsets and computed stacking still disagree with server state. `PlaceFromServer` recomputes stack height from local neighbours instead of consistently applying server Y/HO; inspect before claiming stacking. Server path-map/collision policy remains suspect.
5. Task 6: website preview uses different artwork/IDs from Go. Saved profile authority now holds, but appearance equivalence remains unverified and needs a shared verified choice/render pipeline.

## Rulings

- Continue in current working tree: the user's requested current-state repair depends on existing uncommitted fixes; a clean HEAD worktree would omit them. Do not fold unrelated changes into a commit.
- Keep plan/audit/ledger in repository, not separate storage: these are project handoff sources.
- Begin with demonstrable identity/presence and selection defects; full wardrobe/rendering, room creation, placement authority and browser journeys remain explicit subsequent tasks.
- Task 5 partial work moved ahead of full Task 2/4 because the audit provided a deterministic 2x2 load/move failure and its inverse is already fixed by the existing outbound contract. This repairs internal agreement without asserting a newly discovered historical anchor rule; remaining risk is unverified sprite alignment.

## This session's changed files

- `docs/verification/GAME-AUDIT-2026-10-29.md`
- `docs/superpowers/plans/2026-10-29-playability-repair.md`
- `docs/verification/PLAYABILITY-HANDOFF.md`
- `sources/gocoke/cmd/server/main.go` (appearance override removal only; other diff hunks predate this session)
- `sources/gocoke/cmd/server/rooms.go` (profile XML + invalid-transition guard; other diff hunks predate this session)
- `sources/gocoke/cmd/server/room_presence_test.go` (new)
- `sources/gocoke/internal/client/net/realtime/avatar_packets.go` and `avatar_packets_test.go` (new)
- `sources/gocoke/internal/client/net/realtime/client_wasm.go` (AV decoder; earlier room-inbox work preserved)
- `sources/gocoke/internal/client/scene/studio/infostand.go`, `infostand_furni.go`, `studio.go` (selection; earlier scene repairs preserved)
- `sources/gocoke/internal/client/scene/studio/furniture_selection_test.go`, `furniture_snapshot_test.go` (new)
- `sources/gocoke/internal/client/scene/studio/pos_furni.go` (2x2 positions)
- `docs/PARITY.md` (current repair evidence)

Append implementation, failing/passing test evidence, exact commands, changed files and next action as work proceeds. Do not mark a whole task complete while its stated acceptance remains unverified.
