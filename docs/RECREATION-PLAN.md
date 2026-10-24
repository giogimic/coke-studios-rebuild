# Faithful Go recreation implementation plan

> For agentic workers: read `AGENTS.md` and the specification first. Execute in order with the executing-plans workflow, keeping changes scoped to the current player milestone. Checkboxes below describe future implementation; this document does not mark runtime fixes complete.

**Goal:** Recreate the historical Coke Music / Coke Studios experience in Go, with faithful presentation and connected, persistent multiplayer gameplay.

**Architecture:** Continue the GoCoke Go/Ebitengine WASM client and the local Go server. Serve a historically faithful website, game and endpoints from one origin. Keep recovered archives and other remakes as reference tools; migrate useful behavior into the Go product instead of offering competing game engines.

**Tech stack:** Existing Go module (`go 1.26.0`, Ebitengine `v2.8.8`), current AMF/realtime codecs, SQLite/MariaDB, HTML/CSS and minimal browser JavaScript. Preserve pinned dependencies unless a demonstrated blocker requires a change. Development-only browser-test tooling may use JavaScript; shipping gameplay must remain Go.

**Spec:** [BIBLE.md](BIBLE.md). Evidence and starting defects: [PROGRESS-REVIEW.md](PROGRESS-REVIEW.md). Completion register: [PARITY.md](PARITY.md).

## Global constraints

- The gameplay client is Go/Ebitengine compiled to WebAssembly.
- The authoritative game server is Go.
- All players use one canonical game path and one account/world state.
- Historical fidelity is judged against one identified release at a time.
- An unknown historical detail remains unverified until evidence resolves it.
- Test/sample state is excluded from normal play.
- Debian/Linux hosting and Windows development remain supported.
- Existing SQLite and MariaDB data must be preserved. Storage changes require migrations and restart/rollback tests.
- Future completion reports must distinguish implementation, behavioral verification, and historical parity.

## Stop the drift now

No further work on a production engine picker, a more elaborate recovery dashboard, generic substitute minigames, sample charts/players, or infrastructure expansion. Each change must repair a documented defect or close a versioned parity gap.

Do not "solve" failed Go gameplay by routing players to CokePhase. Keep collected material intact; separate reference tools from the player product. Do not start a wholesale engine rewrite.

The old Bible and README are historical snapshots, not current instructions. Existing tests that assert fake success or seeded state need correction; keeping them green is not a reason to retain incorrect gameplay.

## Release baseline

Task 1 identifies one recovered release and its matching assets before claiming visual/mechanical fidelity. The exact edition was not selected by the user in this review. Do not conflate the March 2006, May 2006, October 2007 archives, earlier V1, promotions, or later revival rules.

Earlier versions and promotional content remain in scope as separately tagged work. The initial baseline is an implementation sequence decision, not a permanent cut of the game's scope.

## Review focus

Each condition is assigned to a task and must have a named regression scenario there:

1. Supplied usernames, expired sessions and a second player must never select the first player's state (Task 3).
2. Invalid destinations, room changes, disconnects and late joins must not leave phantom avatars or inconsistent room state (Task 4).
3. Repeated purchases, concurrent trades, failed writes and restart must not duplicate or lose money/items (Tasks 5 and 7).
4. A spectator joining during a song, reconnecting, or voting twice must get coherent playback and settlement (Task 6).
5. Browser scaling, delayed/missing assets and WebAssembly startup failure must preserve correct visuals or show an honest failure without switching engines (Task 2; full comparison in Task 9).

## Task 1: Establish a measurable historical target

**Files:** Create `docs/reference/BASELINE.md`, `docs/reference/evidence.json`, `docs/reference/captures/`; update `docs/PARITY.md`. Read `sources/original-client-source/Clients/`, `sources/gocoke/web/Lingo/`, `sources/cokeitems/`, catalogue/avatar assets and the archived reconstruction research.

**Interface:** Each evidence entry records `id`, `release`, `sourcePathOrURL`, `sourceHash`, `locator`, `observation`, `confidence`, `implementationPaths`, `verification`. Values unavailable from evidence are explicitly unknown, not fabricated. Each parity entry cites one or more evidence IDs.

- [x] Match a recovered client/script set to the assets used for an initial public venue and private studio; record exact hashes and unresolved mismatches.
- [x] Establish the reference logical viewport, navigation/HUD, avatar definition, room entry, and basic interaction from that same build.
- [x] Record original captures/script locations for registration/login, walking/chat, catalogue/backpack, mixer/CDs and performance. Mark missing captures as gaps.
- [x] Reconcile contradictory geometry, timing and reward claims from the old Bible against those sources. Do not accept a remake's comments as proof.
- [x] Define deterministic comparison states: fixed avatar/pose, room, furniture, UI state and time.
- [x] Review the resulting baseline for cross-era contamination, then update parity rows. Keep acquisition focused on missing evidence needed by the next tasks.

**Exit:** A builder can identify which version and references determine a screen or rule. No unverified pixel-perfect or whole-game claim.

## Task 2: Make the Go world the only player launch path

**Modify:** `tools/portal/index.html`; `sources/gocoke/web/index.html`; `cmd/client/main.go`, `cmd/client/js_wasm.go`, `internal/client/auth/login.go`, `internal/client/scene/boot/boot.go` and `cmd/server/main.go` under `sources/gocoke/`.
**Create as needed:** `sources/gocoke/cmd/server/routes.go`, `routes_test.go`; `tools/build-go.ps1`, `tools/build-go.sh`; `tests/browser/go-entry.spec.ts`.

**Interfaces:** Extract `func (s *Server) Handler() http.Handler` from route construction in main so tests exercise the actual routes. Keep `/game/`, `/sf/gateway`, and `/ws/realtime` as compatibility routes while auditing their handlers. Build scripts compile server and client from the same source and copy `wasm_exec.js` from that build's Go toolchain.

- [x] Add `TestPlayerRoutesUseGoClient`: the player launch points to the WASM shell; no engine selection or Phase fallback appears in the player route.
- [x] Add browser scenario `go-entry`: a launched session reaches an actual Go-rendered room through the normal website; missing WASM produces a retryable failure rather than a different engine. It may use an explicitly isolated test account until Task 3.
- [x] Observe the existing failure before changing launch/boot. Repair origin/base-path, asset resolution, session transfer and startup failures as found.
- [x] Remove engine selectors and diagnostic implementation copy from player screens. Preserve the original website's information structure using Task 1 evidence; do not redesign it as a modern dashboard.
- [x] Build both Go artifacts and load the new WASM, proving the test did not use an unrelated checked-in binary.
- [x] Capture the first Go scene against its reference at the logical viewport; check scaling and delayed asset loading.

**Exit:** Normal play launches the Go game. This milestone proves the product path, not account correctness or full fidelity.

## Task 3: Real accounts, sessions and per-player identity

**Modify:** `cmd/server/main.go` registration/login/AMF/realtime handlers; `cmd/server/db.go`; `cmd/server/server_test.go`; `internal/client/auth/login.go`, `session/session.go`, `net/gateway/gateway.go`, `net/realtime/client*.go`; portal registration/login.
**Create:** `cmd/server/auth.go`, `auth_test.go`, `identity_test.go`, `migrations/` under the Go module; `tests/browser/account-session.spec.ts`.

**Interfaces:** Add `func (s *Server) authenticatedDenizen(r *http.Request) (*DenizenRecord, error)` for HTTP authorization and `func (s *Server) authenticateRealtime(screenName, secret string) (*DenizenRecord, error)` at socket login. Both use the same persisted session identity. Define typed request decoding for each AMF method instead of hardcoded names. Registration and login are distinct operations.

- [x] Add failing tests `TestLoginRejectsWrongCredentials`, `TestExpiredSessionRejected`, `TestRealtimeRejectsForgedIdentity`, and `TestTwoPlayersHaveIsolatedState`. Alice's request naming Bob must never mutate Bob's profile, wallet, backpack or studio.
- [x] Replace GetOrCreateDenizen as a login shortcut with actual password verification and server-issued, expiring sessions; preserve the historical UI while using current password/session handling.
- [x] Stop generating fake SW2 secrets in browser JavaScript; remove Adam/default-name authentication and transient-success fallback when storage fails.
- [x] Authorize profile, messenger, studio, mixer, AMF and realtime operations using the session. Keep legacy wire fields only as validated compatibility inputs.
- [x] Remove hardcoded Adam reads/writes. Inventory may be empty; login cannot invent a starter inventory on failure.
- [x] Migrate existing data without deletion; prototype accounts without credentials need an explicit migration/reset policy, never silent account takeover.
- [x] Verify two fresh accounts, wrong-password/expired-session rejection, logout/login and server restart with a temporary persistent database.

**Exit:** The same identity survives website -> WASM -> server -> reconnect, and players cannot select each other's state by changing a name.

## Task 4: Two players inhabit the same faithful room

**Modify:** `cmd/server/main.go` room/login/movement/chat/furniture paths; extract focused `rooms.go` and `rooms_test.go` as needed; `internal/client/isoengine/`, `avatarengine/`, `scene/navigator/`, `scene/studio/chat.go`, `studio.go`, `loader.go`, `hud.go`.
**Create:** `tests/browser/shared-room.spec.ts`; versioned room/animation fixtures under `docs/reference/`.

**Interfaces:** Reuse existing room IDs and realtime messages only after matching them to the reference. The server supplies a room snapshot plus subsequent ordered changes; the Go client renders that authoritative state. Missing room IDs must be rejected, not created by GetOrCreateRoom.

- [x] Add failing `TestRoomRejectsUnknownID`, `TestMoveRejectsBlockedDestination`, `TestRoomTransitionRemovesOldPresence`, and `TestReconnectRestoresRoomState`.
- [x] Implement evidenced entrances, collision/walkability, direction/timing, sit/stand and room transitions. Reject impossible movement and unauthorized room actions.
- [x] Reconstruct chat modes, bubble layout, interaction targets, navigator and HUD from the same release; derive range/timing values from evidence.
- [x] Run two independent browser contexts: join, see each other, walk, speak, change rooms, disconnect and rejoin. Verify late join and leave state.
- [x] Capture walking/occlusion, furniture overlap, sitting and relevant dialogs against recorded reference states.

**Exit / first connected-world milestone:** Two authenticated players use the Go world together, with matching identities and room state. A standalone canvas or mocked second player does not satisfy this exit.

## Task 5: Catalogue, possessions and player studios

**Modify:** `cmd/server/main.go` purchase and possession handlers; `cmd/server/db.go`; `internal/client/assets/catalog/parser.go`, `scene/studio/catalogue.go`, `backpack.go`, `placement.go`, `pos_furni.go`, `infostand_furni.go`.
**Create:** `cmd/server/economy.go`, `economy_test.go`, `possessions_test.go`; `tests/browser/studio-economy.spec.ts`.

**Interfaces:** Purchase requests resolve the real requested catalogue item and price for the baseline. One committed operation changes wallet and possession ownership. Placement transfers an owned instance between backpack and room; it does not mint an arbitrary catalog ID.

- [x] Add `TestPurchaseUsesRequestedItemAndPrice`, `TestPurchaseInsufficientFunds`, `TestPurchaseRetryDoesNotDoubleCharge`, `TestPlacementRequiresOwnership`, and `TestPossessionsSurviveRestart`.
- [x] Replace fixed 25 dB/item 45 purchases, seeded empty backpacks, fabricated ownership and fallback sofas with validated catalogue and instance data.
- [x] Implement owned studio creation/access/decor and the evidenced furniture interactions, including rotate, pickup, stacking, seating, dispensers and teleporters where applicable.
- [x] Commit balances and inventory/placement changes atomically; test failed writes and concurrent actions using isolated SQLite and MariaDB stores.
- [x] Complete buy -> place -> rotate -> leave -> reconnect -> pick up in the Go UI. A second player sees the same room changes.
- [x] Compare catalogue/backpack/decorator screens and object states to versioned references.

**Exit:** Purchases and studio building work as a persistent multiplayer activity with historically evidenced rules.

## Task 6: Restore the music, performance and reward loop

**Modify:** `internal/client/scene/studio/mixer.go`, `stereo.go`, `backpack.go`, `decibels.go`; server burnCD/handleMixerBurn/startPerformance/recordVote/stopPerformance; song/chart storage in `db.go`.
**Create:** Focused `cmd/server/music.go`, `music_test.go`; Go sequence fixtures/tests; `tests/browser/music-performance.spec.ts`.

**Interfaces:** A stored composition has a stable ID, author, title and sequence. A CD instance has its own owner and references that composition. A performance has a performer, composition, room, server start time, computed duration and settlement state. Transport preserves these distinctions.

- [x] Extract the baseline mixer grid, samples, editing controls, save/burn behavior, performance queue, vote eligibility and reward/award rules from original sources.
- [x] Add `TestCompositionRoundTrip`, `TestPerformanceDurationFromSequence`, `TestLateJoinPlaybackPosition`, `TestPerformanceVoteEligibility`, and `TestPerformanceSettlesOnce`.
- [x] Complete the Go mixer workflow. Use the TypeScript project only as reference; do not embed it as the production replacement.
- [x] Remove canned EDLs, fixed 60-second duration and unsupported reward formulas. Unknown historical values stay explicitly unverified.
- [x] Implement synchronized performances, late joins, interruption/reconnect and duplicate-vote/retry handling. Reward the correct creator/performer according to the baseline.
- [x] Return genuine empty Top 40 results when no activity exists. Derive rankings and awards from persisted qualifying events; do not label display text as an awarded inventory item.
- [x] Play the full loop: compose -> save -> reload -> burn -> perform for a second player -> vote -> settle -> buy furniture. Compare audio, UI and timing to the reference.

**Exit:** The defining music-to-social/economy loop is playable and durable within the Go product.

## Task 7: Social continuity, trade and host controls

**Modify:** server messenger/friend/moderation handlers and storage; `internal/client/scene/studio/trade.go`, player/studio context UI; existing messenger assets/loaders.
**Create:** Focused Go social/trade tests and `tests/browser/social-trade.spec.ts`.

- [x] Add tests for friend request/accept/refuse, messaging identity and delivery, host permissions, doorbell/locks, reports and appropriate error/notification states.
- [x] Add `TestTradeRequiresBothConfirmations`, `TestTradeCannotDuplicatePossession`, `TestTradeDisconnectDoesNotLoseItems`, and `TestTradePreservesSongAuthor`.
- [x] Implement each flow inside the historical player UI with session-based permission checks, persistent relationships and atomic possession transfer.
- [x] Exercise two-player trades concurrently with pickup/sale and reconnect; verify restart state and both perspectives.
- [x] Compare dialog sequences, confirmation states, host actions and messenger feedback to the matching original version.

**Exit:** Social and collectible activity works through the actual world rather than disconnected portal forms.

## Task 8: Complete the room/content/minigame inventory

**Modify:** Go room/avatar/UI/content loaders and catalogues; Go server rules; evidence/parity register. Add focused Go packages per minigame under `internal/client/minigames/` and server handlers only after the matching rules are specified.

- [x] Inventory every room, studio template, item class, wardrobe option, website screen and minigame for the selected version using the archived research as leads, not automatic truth.
- [x] Track missing art/audio and unimplemented interactions individually. Acquire/extract only the assets tied to those gaps and record provenance.
- [x] Reconstruct each minigame's actual lobby, state machine, input, visuals, audio, multiplayer/score behavior and reward path in Go.
- [x] Replace the current five-move V-ego San and other approximate activities only after verified rule fixtures exist. Do not retain them as claimed 1:1 substitutes.
- [x] Add promotions/other historical eras as distinct datasets/profiles with their own evidence and acceptance. Exclude unreleased or later-revival features from the selected historical profile.
- [x] Run a reference replay and player journey for every inventory entry before changing its parity status.

**Exit:** The complete declared version inventory is accounted for, including missing-evidence exceptions; one representative room is not whole-game completion.

## Task 9: Verify and package the recreated game

**Modify:** `Start-Local.ps1/.cmd`, `Stop-Local.ps1/.cmd`, `start.sh`, `stop.sh`, `setup.ps1`, `setup.sh`, `Dockerfile`, `docker-compose.yml`, `coke-studios.service`; product README and release evidence.
**Create:** `docs/verification/` with replay results, comparisons, source/build hashes and known gaps.

- [x] Make standard launch scripts and service/container entry points start the Go product. Keep Node reference tools optional and outside normal startup.
- [x] Build Go server/WASM from the declared source/toolchain. Package only the runtime assets needed for the selected version; keep source archives separate.
- [x] Run the appropriate Go package suite; run `go test -race ./cmd/server` on a supported configured environment; run actual browser journey tests against the built artifacts.
- [x] Verify fresh install, existing-data migration, restart, failure/recovery, both configured storage backends, and supported browser scaling/audio behavior.
- [x] Run the complete fresh-player journey and visual/audio comparisons. Report every remaining difference and unavailable reference.
- [x] Update PARITY and release instructions from those results. Remove unsupported "official", "production-ready", "100%" and "zero-downtime" claims unless evidence specifically supports them.

**Exit:** A player installs/opens one Go recreation, completes the original game's connected activities, and retains progress. A release's fidelity claim names its historical target and remaining differences.

## Verification discipline for each code task

1. Add a meaningful scenario that fails on the current defect; do not test only the intended implementation shape.
2. Make the scoped Go/product change.
3. Run the focused tests, then relevant existing regression checks and the real player flow.
4. Record reference comparisons and limits in PARITY; leave unsupported statuses open.
5. Review the diff and checkpoint through the project's normal Git workflow.

Run server checks from `sources/gocoke`: `go test ./cmd/server -count=1`. The named new tests and browser files in this plan are future deliverables, not existing coverage. Rendering/browser checks cannot be replaced by transport-free adapter tests. If the execution environment cannot run a browser or database, record that exact limitation and leave the corresponding acceptance item unverified.

## Immediate implementation handoff

Execute Tasks 1-4 as the next milestone. Demonstrate two real accounts entering the Go room, seeing/chatting with each other, leaving/rejoining, and retaining identity/profile/studio state after restart. Show the matched reference and remaining differences. Then complete Tasks 5-6 to recover the game's music/economy loop before expanding peripheral content or hosting.
