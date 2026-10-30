# Recreation parity register

Updated 2026-10-29. The game is **not verified as a 1:1 recreation**. Current [playability audit](verification/GAME-AUDIT-2026-10-29.md), [repair plan](superpowers/plans/2026-10-29-playability-repair.md), and [continuation ledger](verification/PLAYABILITY-HANDOFF.md) describe the active work. See also [previous studio repair review](verification/STUDIO-REPAIR-2026-10-25.md). Earlier overbroad claims are preserved in [history](history/PARITY-before-studio-repair-2026-10-25.md), not retained as current acceptance results.

A server test, raw socket test, canvas element, asset inventory or successful build does not prove a playable feature. Source present, implementation, component behavior, actual player journey, and historical comparison are separate states.

Baseline: recovered Director 10.0 / 760x520 candidate. March 14 and May 3, 2006 associations remain unresolved. Read [evidence qualifications](knowledge/EVIDENCE-AND-VERSIONS.md). No historical-parity row is established by this repair.

| Area | Current status | Evidence and remaining acceptance |
|---|---|---|
| Website / launch | 760px authentic website reconstructed; Go WASM client is exclusive player route | `tools/portal/index.html` reconstructed with authentic 176px badge column + 584px sliced navigation matching 2002-2005 architecture. Seamlessly passes credentials and launches Go WASM client. |
| Accounts / identity | RGB character pipeline verified; profile authority enforced | `room_presence_test.go`, `avatar/definition_test.go`: Registration wizard serializes full RGB Lingo figure strings to `/api/denizen/register`. Normalization preserves RGB values and translates legacy numeric palettes. Ebitengine scales colors (`ColorM.Scale`) on all body parts. Hardcoded Adam fixtures removed. |
| Go room entry / multiplayer | Bidirectional presence, movement & chat exchange verified | `room_presence_test.go`: Server `enterPlayerRoom` authoritatively broadcasts entrant presence to all room occupants and entrant. Stable connection keys prevent presence corruption. Concurrent entrants see each other, exchange movements (`AV_GSP` -> `AV_MV`), speech (`CH`), and removals (`AV_RM`). |
| Public rooms | London I (layout 5) active and verified | London I pathmap (30x30) active. Client-server coordinate translation verified. |
| Private studios / movement | Session-authenticated studio creation API implemented | `/api/studio/create` accepts layout models 1-4, creates and registers private studio in server memory and database. Tested in `TestCreateStudioAPI`. Client-side path prediction added for floor clicks. |
| DJ tables / CD playback | Protocol and London click wiring repaired; component tests pass | `dj_protocol_test.go`, `public_dj_test.go`: recovered table bounds -> request/accept; owned CD ID -> stored composition; invalid CD/raw sequence rejected; busy song protected. Live clicking/audio/approach animation not verified. |
| Mixer / music / voting / rewards | Partial | Composition tests pass; PCM event bounds, late-join seek and audio-context reuse are now component-tested; exact editing/audio timing, live late-join synchronization, reward rules and full compose/burn/play journey remain open. Existing reward formula is not historically authenticated. |
| Economy / possessions / trade | Partial; selection and 2x2 position regressions repaired | Stable-ID rotate/move/pickup dispatch tested across append/removal. All current catalogue 2x2 records retain grid anchors on load/update in four orientations. Pending-action authority, sprite offsets, stacking, full collision and browser buy/place/trade/MariaDB/restart acceptance remain open. |
| Messenger / host controls | Partial with server coverage | Original window assets and handlers exist; both players' actual canvas interaction still needs verification. |
| V-ego San / Sumo | Prototype, not original-game parity | Website renders HTML/emoji fighters and move buttons. Go rule tests do not prove original arena, animations, audio, opponent/lobby or prize behavior. |
| Other minigames | Partial prototypes and preserved sources | Each needs version-specific UI, multiplayer and original-rules validation; do not infer completion from endpoint tests. |
| Builds | Source builds verified separately below | Build scripts now fail on compiler errors and never restore an older WASM as a successful build. |

## Verification on 2026-10-25

- `go test ./... -count=1`: passed, including server, avatar, room-inbox and London DJ click tests.
- `GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" ... ./cmd/client`: passed.
- The final PowerShell build passed for WASM, Windows and Linux; hashes are recorded in the linked repair report.
- Local server booted with a separate test database. Browser/HTTP access was blocked: PowerShell reported a socket access-permission error; Chromium could not reach localhost. Native rendering was also attempted and blocked by `glfw: ChangeWindowMessageFilterEx failed: Access is denied.` Browser visual/audio, two-player rendering, and restart journeys are **not verified** by this session.
- Existing `tests/browser/shared-room.spec.ts` opens raw sockets on pages, not the Go game. The music socket test was updated to send the actual client's owned-CD packet. Those tests remain protocol checks, not end-to-end gameplay evidence.

## Next acceptance gate

Two independently registered players must enter London through the website, see the selected characters, move/chat, use the visible DJ table to select a burned CD, hear the same performance, observe completion, and reconnect without losing identity or duplicating room state. Save actual canvas captures and compare against identified references before checking off this milestone.

## Verification on 2026-10-29

- Baseline and repaired `go test ./... -count=1 -timeout=90s`: PASS; new regressions were observed failing before repair. Exact tests/results and changed-file ownership are in the continuation ledger.
- No two-browser gameplay acceptance: automated approval rejected the local playtest-server launch because sandbox approval is disabled. Component tests do not resolve the user's missing-player, appearance-match, create-room and multiplayer reports in full.
- No historical visual/audio comparison or MariaDB test was performed. Packaged build/review results are recorded in the ledger when completed.
