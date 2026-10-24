# Coke Music / Coke Studios recreation

## Product mandate

Build a faithful, playable 1:1 recreation of the historical Coke Music / Coke Studios game in Go. Fidelity covers the website, accounts, V-egos, rooms, interface, music, economy, social systems, and their connected behavior. A collection of demos or a developer portal does not satisfy this goal.

Read `docs/BIBLE.md`, `docs/RECREATION-PLAN.md`, `docs/PARITY.md`, and `docs/PROGRESS-REVIEW.md` before product work. These supersede the earlier architecture and completion claims preserved in `docs/history/`. The user's instructions take precedence.

## Implementation direction

- Continue GoCoke in `sources/gocoke`: Go/Ebitengine compiled to WebAssembly for gameplay, Go for the authoritative server. Repair that path when it breaks.
- HTML/CSS and minimal JavaScript may provide the historically faithful website, browser boot, and browser APIs. Gameplay, simulation, music sequencing, and economy logic belong in Go. Serving a JavaScript game from a Go HTTP server does not meet the language requirement.
- CokePhase, the TypeScript mixer, other community rebuilds, and extracted archives are reference material. Preserve them. Do not expand them into a second production game or silently use them as a fallback for the Go client.
- Deliver one coherent player journey and one shared account/world state. Engine pickers, source browsers, debug counters, protocol panels, and demo launchers belong in developer-only tools, outside the player flow.
- Retain Debian/Linux hosting and Windows development compatibility. Simplify launch/build plumbing around the Go product as its player path becomes usable; do not spend the next milestone expanding hosting features.

## Fidelity rules

- Pin a historical release and asset set before claiming parity. Keep V1, Beta, V2, promotions, and later fan changes distinct; do not invent a blended historical version.
- Use recovered scripts, assets, archived screens, and recordings as evidence. Record the source/build and confidence for each behavior or visual target. Community remake behavior and comments saying "1:1" are not historical proof.
- Unknown details remain explicitly unverified. Do not fill gaps with Habbo rules, invented rewards, generic minigames, placeholder rooms, or modern UI redesigns.
- Compare at the reference's logical resolution: layout, sprite anchors, hit areas, text, palette, depth order, animation timing, sound, and transitions all matter. Never adopt numbers from the old Bible without checking their sources.
- Never present seeded players, sample charts, hardcoded purchases, fake sessions, or canned success responses as real gameplay. Test fixtures must be explicit and excluded from normal play. Empty state and failure are real states.

## Delivery and verification

- Prioritize complete player journeys: register/log in -> enter the Go world -> meet another player -> persist progress; then compose -> save -> burn -> perform -> vote/reward -> buy/decorate/socialize.
- A feature is complete only when its player interaction, authoritative state change, persistence where applicable, failure paths, and reference comparison pass. Endpoint tests, a compiled binary, a WASM header, or screenshots of separate demos do not establish game parity.
- Update `docs/PARITY.md` with evidence and verification limits. Distinguish source present, implemented, behavior verified, and historical parity verified. Do not publish unsupported percentages or blanket "100% PASS" claims.
- Preserve user data, downloaded sources, licenses, and attribution. Use temporary databases for tests. Do not delete or reset `coke.db` to make a test pass.
- Scope refactoring to the next player milestone. Avoid a wholesale rewrite of useful Go rendering, assets, or protocol code.

## Immediate next work

Follow the ordered tasks in `docs/RECREATION-PLAN.md`: establish the reference baseline, make the Go client the only player route, and fix identity plus per-player server state. Remove the hardcoded Adam balance/backpack/purchase behavior before treating the economy as multiplayer.
