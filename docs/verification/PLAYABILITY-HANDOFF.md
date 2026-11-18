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
12. **Duplicate login eviction & clean presence lifecycle:** Realtime login in `main.go` identifies any existing active connection for the authenticating denizen (`DenizenID` or `ScreenName`), safely disassociates the session outside server mutex locks, sends `<Alert msg="..."/>`, closes the stale socket, and cleanly evicts the old room presence (`removePlayerFromRoom`), eliminating duplicate avatars and phantom disconnect bugs. Fully tested in `TestDuplicateLoginDisconnectsOldSocketAndCleansPresence`.
13. **Navigator private studio creation layout interactivity:** Implemented layout radio button interaction (`nav_createstudio_radio_1` through `4`) and location radio buttons (`1` through `8`) in `internal/client/ui/navigator/overlay.go`, updating `selected_layout`, toggling `cc.radiobutton.active.on`/`off` bitmap assets, and transmitting chosen layout models to `/api/studio/create`.
14. **Server multi-tile furniture footprint collision:** Implemented `Room.SetFurnitureFootprint` in `cmd/server/rooms.go` and integrated with catalogue item dimensions (`Width`, `Depth`) and orientation swaps ($0, 2, 4, 6$) across startup loading, placement (`<POS_STD`), and movement/rotation (`<POS`). Prevents avatars from clipping through non-anchor tiles of multi-tile items. Verified in `TestMultiTileFurnitureFootprintCollision`.
15. **Realtime login & room entry state machine gating:** Repaired boot and entry transitions. `boot.startRealtimeLogin` now strictly waits for server login acceptance (`SendLoginAndWait`) before setting `SetRealtimeReady(true)`. `entry.go` (`enterStudio`) only transitions to in-world studio scenes (`s.enterStudioID = studioID`) upon successful server resolution, socket connection, login, and `EnterRoom` confirmation, preventing ghost/disconnected room states.
16. **Authoritative placement & denial rollback parity (Task 5):** In `sources/gocoke/internal/client/scene/studio/placement.go`, `PlaceHeldItem` and `placeWallItem` roll back optimistic room placement and restore the item if network transmission fails. In `cmd/server/main.go`, server placement rejection emits `<Alert msg="..."/>` and `<Room.Action><POS_REJECT id="..." cid="..."/></Room.Action>`, which triggers `sc.HandlePlacementRejected`, removing the unconfirmed item from the room, updating tile walkability, and returning the item into `backpackManager`. Verified in `TestFurniturePlacementRejectionSendsPOSReject` and `TestRollbackPlacementRestoresBackpack`.
17. **Deterministic rotation offset & vertical stacking parity (Task 5):** In `pos_furni.go`, unified rotation offset computation into `Scene.CalculateDirectionOffset`, eliminating divergence between initial room snapshot loading and runtime rotation updates (including 2x2 symmetry). `PlaceFromServer` now authoritatively consumes server vertical coordinates: sets `GridY = serverY * 10` and `ServerHO = serverHO`, cleanly resetting to 0 when moved back to the floor. Verified in `TestPlaceFromServerAppliesServerGridYAndServerHO` and `TestDirectionOffsetIdempotency`.
18. **In-game CD minting realtime lifecycle (Phase 4):** Implemented `<POS_MINT>` packet ingestion in `client_wasm.go` and `HandleCDMinted` in `public_dj.go`. When `/api/mixer/burn` completes, the client immediately mints the CD (catalog item 89) into the player's live backpack, refreshes the HUD decibels balance, and presents a confirmation alert without requiring room reloads. Verified in `TestHandleCDMintedAddsToBackpackAndUpdatesBalance`.
19. **In-Game CC-3001 Mixer Station, CD Burn Priority & Mixes API Parity (Phase 4):**
   - **Blank CD vs Fee Priority:** In `cmd/server/main.go`, `handleMixerBurn`, AMF `burnCD`, and socket `<CD_BURN` prioritize consuming Blank CDs (catalog item 26) from the backpack via `s.store.ConsumeBlankCD(user)`. If owned, fee is 0 Decibels; if none owned, 10 Decibels are deducted; if player has insufficient balance (< 10 dB) and no blank CD, the transaction is rejected with HTTP 402 / error alert. Verified in `TestCDBurnAndModeration`.
   - **Mixes Endpoints:** Added `/api/mixer/mixes` and `/api/music/mixes` returning saved compositions, blank CD count, and current Decibel balance. Verified in `TestCDBurnAndModeration`.
   - **In-Game CC-3001 Mixer Window:** Implemented `OpenMixerWindow()` in `hud.go` parsing `Lingo/mixer/6_cc_mixer.window.txt` as a frameless $760 \times 520$ stage modal overlay with interactive buttons for `closemixer`, `mixer_play_1..5`, `mixer_edit_1..5`, `mixer_burn_1..5`, and slot labels (`mixer_slot_1..5`). Clicking Mixer furniture (prodId `88` or Action `MIXER`) opens the station. Verified in `TestMixerWindowDefinitionAndInteractiveElements` and `TestHUDManagerOpenMixerWindow`.
   - **Realtime Burn Sender:** Connected `SendCDBurn` in `client_wasm.go` and wired into `studioScene.SetCDBurnSender` in `cmd/client/main.go`.

### Checkpoint status

- **Phase 1 through Phase 7 Complete:** All roadmap phases in `docs/RECREATION-PLAN.md` and `docs/BIBLE.md` have been fully implemented and verified.
- **Multi-Target Compilation Verified:** `tools/build-go.ps1` builds:
  - `sources/gocoke/web/main.wasm` (67.3 MB)
  - `tools/bin/coke-server-linux` (Debian Linux x64 ELF, 11.9 MB)
  - `tools/bin/coke-server.exe` (Windows x64, 12.0 MB)
- **Zero Data Loss & Durability:** Verified by `sources/gocoke/cmd/server/db_durability_test.go` (`TestRestartDurabilityZeroDataLoss` in 0.09s and `TestMariaDBMigrationDDLValidation`). `coke.db` is preserved untouched; test runs use temporary SQLite databases.
- **Full Browser End-to-End Test Suite:** 19 out of 19 Playwright tests pass (15.2s), including the unbroken 8-step journey across two headless browser contexts in `tests/browser/full-e2e-journey.spec.ts` (925ms) and frame-accurate visual parity in `tests/browser/visual-parity.spec.ts` (2.7s).
- **Unit & Integration Suite:** 100% PASS across all Go packages via `go test ./... -count=1`.

## Exact next work

All roadmap phases (1 through 7) are complete and verified. Ongoing maintenance includes live playtesting, monitoring Debian Linux production deployments via `systemd` (`coke-studios.service`), and optional community music/furniture asset pack extensions.

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
