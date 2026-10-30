# Playability audit — 2026-10-29

## Scope and verdict

The reported broken journeys are credible. This review examines the current working tree, including the unfinished October 25 repairs, not only HEAD `e73eb35`. The Go server suite passes, but it does not establish a working Go browser game. No historical parity is claimed. Preserve the existing uncommitted work and all player databases.

Read next: [repair plan](../superpowers/plans/2026-10-29-playability-repair.md) and [continuation ledger](PLAYABILITY-HANDOFF.md). These supplement, not replace, BIBLE and RECREATION-PLAN.

## Findings

| Priority | Reported symptom / risk | Current evidence | Required correction |
|---|---|---|---|
| P0 | Missing or inconsistent players | Boot `startRealtimeLogin` sets ready immediately after sending login; its scripted progress does not wait for acceptance. Entry installs a room even after connection/entry failure. WASM navigation has no rejection recovery. | Explicit connection/login/room acceptance state, visible failure and retry, close/reconnect cleanup; exercise delayed/rejected packets. |
| P0 | Selected appearance differs in game | Portal `renderAvatar` uses separate `/avatar-assets/` composites and numeric palette definitions. `internal/avatar/definition.go` translates skin but assigns white clothing/hair and assumes style IDs correspond. | One verified appearance schema, supported choice manifest and Go-backed preview; compare exact saved appearance in both clients. Normalization alone does not prove matching appearance. |
| P0 | Profile can be overwritten by socket input | Server login and `spawnAvatarInRoom` accept supplied `def` over the stored account appearance. Avatar packets interpolate profile text directly into XML. | Use persisted authenticated appearance; escape and decode XML once; test hostile/stale definitions and quoted mission text for initial spawn and late join. |
| P0 | Multiplayer presence is fragile | `enterPlayerRoom` snapshots players under a read lock, releases it, then adds entrant under a new lock. Concurrent entrants can both miss each other. It also removes old presence before validating a destination. Socket login changes `Player.ID` from connection ID to denizen ID while the global map retains the connection key. | Serialize room snapshot/admission, validate before leave, retain stable connection identity, test real handler message ordering and disconnect/reconnect. |
| P1 | Cannot create rooms | Both `ui/navigator/overlay.go` and entry's private creation callback only log TODO. Store `CreateStudio` is called by registration; no ordinary creation gateway/HTTP handler exists. | Complete evidenced Go creation dialog -> authenticated handler -> persisted studio -> refreshed My Studios -> enter. An endpoint alone is insufficient. |
| P1 | Rotation sometimes does nothing / wrong furniture selected | InfoStand retains `*PlacedFurni` pointing into `[]PlacedFurni`; append/reallocation or removal invalidates that selection. `ValidateRotate` excludes self by pointer identity and move finds index by pointer identity. | Store stable possession ID in selection, resolve current item for each action, invalidate selection when removed. Reproduce append and removal cases. |
| P1 | Placement jumps or disagrees after rejoin | Local placement, rotation and `PlaceFromServer` independently adjust 2x2 origins. Incoming direction 0/4 subtracts row 1 and col 2, while outgoing adds only col 1. Rotation offset logic is duplicated and differs between initial load/update. | Define one reversible furniture anchor conversion per evidenced item class, test all four orientations and reconnect with actual catalogue assets. |
| P1 | Items appear placed despite failure | `PlaceHeldItem` / wall placement mutate room and remove backpack item before server confirmation; send/storage rejection merely logs. Server POS_STD adds new collision cell without removing previous cell for moved item. | Pending action/authoritative acknowledgement and denial restoration; atomic inventory-room transfer; clear old occupancy and validate complete footprint. |
| P1 | Collision disagrees with client | Server `LoadPathMap` accepts both `(col,row)` and `(row,col)` and all weights except 255, while client transforms columns/rows and has distinct weight semantics. Server blocks only one tile per item. | Source-derived shared coordinate and tile classification contract, full oriented footprints, blocked-path tests. Do not guess by adding more fallbacks. |
| P1 | Other public rooms display wrong scene | Many named rooms share layout 5; loaders choose London by layout. | Versioned room-to-cast/map manifest. London is first verified target; unresolved destinations remain incomplete. |
| P1 | Account boundaries still incomplete | Realtime legacy fallback accepts `screenName_...` for passwordless accounts. AMF social/decor branches use supplied identities or mutate without validated ownership. | Audit all mutation routes against the existing authentication helper; reject legacy takeover, provide explicit migration policy without resetting data. |
| P2 | Misleading verification | Browser `shared-room.spec.ts` drives raw sockets rather than the Go canvas. Previous render fixture was blocked. | Real independently registered browser contexts, visible avatars, actions through game UI, persisted state and negative-path assertions. |

## Verification at audit start

- `GOPROXY=off`, workspace `GOCACHE`, `go test ./cmd/server -count=1 -timeout=60s`: PASS (3.856s). This is the pre-change baseline.
- Full suite launched separately; result belongs in the continuation ledger once obtained.
- Attempt to start a hidden isolated local playtest server was automatically rejected: sandbox approval is disabled. No browser result is claimed, and no player database was used by that rejected launch.
- Historical sources read: knowledge README, EVIDENCE-AND-VERSIONS, INTERFACES, PLAYER-FLOWS; March/May 2006 association still unresolved. Existing controls support creation/rotation/pickup as intended journeys, not the invented placement constants.

## Acceptance gate

Two newly registered accounts with visibly different chosen appearances enter the same Go-rendered London room, see one another, move/chat, leave/rejoin and reconnect. One creates a private studio, buys a real item, places/moves/rotates/picks it up, and both clients agree. Restart a separate test server and confirm saved identities/studios/possessions. Inject unauthenticated, expired, rejected, delayed, and disconnected cases. Compare visual fixtures at 760x520 to the declared reference before claiming fidelity.
