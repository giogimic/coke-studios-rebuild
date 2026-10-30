# Progress review and course correction

Review date: 2026-10-24. Reviewed workspace HEAD: `66d93d8`.
Scope: product direction, source inspection, documentation accuracy, and the existing Go server unit suite. This is not a full browser playtest or an independent historical audit.

## Finding

The concern about drift is supported by the code. Useful preservation and Go implementation work exists, but the primary route, test criteria, and documentation treat working scaffolding and separate prototypes as a completed recreation. The correction is to make authentic connected gameplay the unit of progress.

## What to keep

- GoCoke's Go/Ebitengine client, asset loaders, avatar/isometric code, studio UI, mixer, and network packages.
- The locally added Go server and SQLite/MariaDB store as repairable implementation work.
- Collected original-client scripts, assets, audio, community rebuilds, extraction tools, manifests and credits.
- Existing test coverage as a regression starting point, with its assumptions reviewed.
- Debian/Linux deployment and Windows development support.

These are implementation assets; their presence does not establish original-game parity.

## Findings and required changes

Line references below describe the pre-correction code at the reviewed HEAD. Documentation references to the old Bible/README refer to the exact copies in `docs/history/`.

| Priority | Evidence | Consequence | Required direction |
|---|---|---|---|
| P0 | `tools/portal/index.html:1469` selects the Phase engine; `:1478` loads `/phase/?user=Adam...`; `:3633` launches Phase after registration | The normal player enters a JavaScript remake while Go is optional | One player route to the Go WASM client; repair Go boot and remove the production engine selector/fallback |
| P0 | `sources/gocoke/cmd/server/main.go:816`, `:821`, `:860`, `:870`, `:871` read/write Adam's balance/backpack and buy item 45 for 25 dB | Multiplayer economy is hardcoded to one identity and purchase result | Derive identity from the authenticated session; decode requested item; use the versioned catalogue and an atomic transaction |
| P0 | `cmd/server/main.go:374`, `:400`, `:412` collect password; `:444` calls GetOrCreateDenizen without it; `:498`, `:521` log in from supplied names; `:1230` accepts realtime Login without secret verification | The registration screen is not proof of account authentication or isolation | Implement real registration/login/session validation and authorize HTTP/AMF/realtime actions against it |
| P1 | `cmd/server/main.go:821` returns starter possessions when inventory is empty/error; `cmd/server/db.go:1036` invents chart entries when the query is empty | Empty accounts and empty charts cannot be observed honestly | Separate explicit test seeds; normal state must come from committed player activity |
| P1 | `cmd/server/main.go:1326` accepts the last client path point; `:1345` accepts supplied movement coordinates; `:162` creates missing rooms on demand | Server authority and original room/movement rules are not established | Validate versioned rooms, walkability, legal transitions, ownership and permissions before applying actions |
| P1 | `cmd/server/main.go:1993` fixes performance length to 60 seconds; `:2026` labels an unreferenced reward formula "1:1"; `:2085` records the DJ as creator | Music duration, rewards, and author-versus-performer semantics need reconstruction | Trace original sequence/performance/award rules; store song identity, author, owner and performer distinctly |
| P1 | `cmd/server/db.go:1049` implements V-ego San as a time-selected five-move opponent | A named minigame exists but historical parity has not been demonstrated | Mark it prototype; reconstruct the original lobby, match state, controls, animation, result and rewards in Go |
| P1 | Old `BIBLE.md:563` claims password hashing; `:576` mandates dual engines; `:330` says every component is verified | The spec was reinforcing behaviors inconsistent with the requested product and overstating verification | Replace the authoritative spec; retain old material as historical notes only |
| P1 | Old `BIBLE.md:62` asserts 128x64 tiles; `internal/client/isoengine/constants.go` uses layout sizes 34/51 and rotation constants; old `BIBLE.md:161` specifies 32-second songs while server code uses 60 seconds | Technical detail was promoted to historical truth without reconciliation | Create versioned source/measurement fixtures for geometry, timing and audio; do not treat either code or prose as automatic truth |
| P2 | `Start-Local.ps1`, `start.sh`, `Dockerfile`, `coke-studios.service` start Node; Go routes also serve Phase/mixer/portal | Runtime and deployment remain centered on a multi-demo launcher | Consolidate the playable product under Go once the canonical path works; retain reference tools separately |
| P2 | `cmd/server/server_test.go:91` calls handlers directly; `:44` expects a nonempty demo backpack; `tools/test-local.cjs` checks serving/fixtures | Passing these tests cannot prove two-browser gameplay, correct identity, or historical visuals | Retain appropriate regression checks and add player-flow, persistence, rejection, audio and visual reference evidence |

Paths beginning `cmd/server` and `internal/client` in the table are relative to `sources/gocoke/`.

## Verification performed

Command, from `sources/gocoke`:

```powershell
$env:GOPROXY = 'off'
go test ./cmd/server -count=1
```

Result: `ok gocoke/cmd/server 0.710s`.

This verifies that the existing server unit suite passes with cached dependencies. Several assertions encode current prototype behavior, so the pass is not a parity verdict. Browser rendering, WASM gameplay, two-player synchronization, restart persistence, MariaDB behavior, historical audio, and screenshot comparisons were not established by this command.

## Corrected priorities

1. Pin a coherent historical baseline and record what is known/unknown.
2. Make the Go client the sole product path; eliminate the need to switch engines.
3. Establish real identity, player-specific state, and a two-player room journey.
4. Reconstruct the music -> performance -> reward -> catalogue/studio loop.
5. Complete social, trade, room, minigame and version-specific content against evidence.
6. Validate a complete fresh-player experience and package that Go game.

Do not add more portal tabs, invented reward loops, or deployment complexity while these core journeys remain incomplete. Acquiring a missing asset is useful when it resolves a named parity gap; collection volume alone is not game progress.

## Changes made in this review

- Added root `AGENTS.md` so future work inherits the Go/fidelity mandate and completion criteria.
- Replaced `docs/BIBLE.md` with a product specification separating evidence, implementation, unknowns and acceptance.
- Replaced the README's recovery-hub/completion framing with the recreation goal and actual status.
- Added `docs/RECREATION-PLAN.md` and `docs/PARITY.md` with ordered work and honest verification states.
- Preserved the previous Bible and README byte-for-byte under `docs/history/`.
- Marked the older research summary as a lead inventory whose implementation claims require the new review.

These are direction and documentation corrections. The runtime issues in the table remain implementation work; no entry is marked fixed merely because it now has a plan.

## Current repair review — 2026-10-25

See [studio repair review](verification/STUDIO-REPAIR-2026-10-25.md) for current defects, fixes, verification and remaining work. The 2026-10-24 code line references above are historical. Later all-passed summaries did not establish a complete player journey; the current [parity register](PARITY.md) supersedes those claims.
