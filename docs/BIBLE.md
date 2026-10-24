# Coke Music / Coke Studios recreation specification

Updated 2026-10-24. This is the product specification; the ordered work is in [RECREATION-PLAN.md](RECREATION-PLAN.md). See [PROGRESS-REVIEW.md](PROGRESS-REVIEW.md) for code findings and [PARITY.md](PARITY.md) for current evidence.

## The goal

Recreate the historical game as faithfully as the surviving evidence allows, with the game client and server implemented in Go. The intended result is a place people can play: create a V-ego, meet others in the original rooms, make and perform music, receive feedback, earn and spend Decibels, furnish studios, collect and trade, and maintain friendships.

"1:1" is the acceptance target for the selected historical version. It is not a synonym for loading original assets. It includes appearance, sound, timing, controls, player journeys, game rules, and durable shared state.

## Current truth

There is useful Go client code, recovered artwork/audio/scripts, and a locally developed Go server with storage and protocol handlers. The project is an incomplete recreation. The current portal defaults to CokePhase JavaScript; several server operations use demo behavior. The server unit suite passed during the 2026-10-24 review, but end-to-end historical parity has not been established.

The previous Bible mixed historical research, proposed architecture, invented implementation values, and completion claims. It is preserved verbatim in [history/BIBLE-before-realignment-2026-10-24.md](history/BIBLE-before-realignment-2026-10-24.md). Its later reconstruction research remains a useful lead inventory. Its formulas, dates, rule descriptions, and "verified" assertions require source checking; they are not normative specifications.

## Decisions and constraints

- The gameplay client is Go/Ebitengine compiled to WebAssembly.
- The authoritative game server is Go.
- HTML/CSS and minimal JavaScript may implement the historical website, browser boot, and browser APIs; gameplay rules and music sequencing belong in Go.
- GoCoke is the implementation foundation. Preserve and repair its useful components rather than rewriting the client wholesale.
- A Go server serving CokePhase does not satisfy the Go client requirement.
- All players use one canonical game path and one account/world state.
- Debian/Linux hosting and Windows development remain supported.
- Existing SQLite and MariaDB data must be preserved. Storage changes require migrations and restart/rollback tests.
- Historical fidelity is judged against one identified release at a time.
- An unknown historical detail remains unverified until evidence resolves it.
- Test/sample state is excluded from normal play.
- Future completion reports must distinguish implementation, behavioral verification, and historical parity.

## Reference baseline and evidence

The user has specified the game and the Go requirement, but has not selected an exact dated edition in this review. Do not silently interpret "Coke Music / Coke Studios" as a single hybrid of all years.

The local original-client archive contains March 14, 2006; May 3, 2006; and October 8, 2007 directories under `sources/original-client-source/Clients/`. Their presence does not establish which one matches every GoCoke asset. The first task is to map recovered assets and scripts to one reproducible reference build and record its exact identity, hashes, logical viewport, and source confidence. Website evidence may have a separately dated capture; document its relationship to the client.

Use the best-matched recovered client as the initial implementation baseline, explicitly recorded as a working choice until historical identification is complete. Preserve earlier versions and promotions as separately tagged scope. Do not label that choice as a user-approved preference or claim whole-game fidelity while it is unsettled.

Evidence priority:

1. Matching recovered original scripts, data, and assets.
2. Dated original screenshots/recordings and archived help/website pages.
3. Original developer material tied to the relevant version.
4. Community preservation/rebuilds as corroboration and implementation references.

A code comment, the old Bible, or a fan remake alone cannot prove an original rule. Each parity record needs a reference identifier, exact path/capture, version, observed behavior, implementation owner, and verification result. Conflicting evidence stays visible.

## Product shape

The historical website and in-world client are one product. Recreate their original transitions and content structure instead of adding a modern recovery dashboard.

The main player experience must contain:
- Historical entry, registration, returning-player login, V-ego creation/editing, and the appropriate launch transition.
- The original world viewport, room navigation, HUD, dialogs, chat, avatar interaction, and furniture interaction.
- The original music workflow, from composing and saving to CDs, performances, voting, reputation, and rewards.
- Persistent studio ownership, inventory, catalogue, placement/rotation/pickup, trading, and the applicable economy.
- Friends, messenger, host controls, reporting/moderation, errors, and reconnect states.
- Version-appropriate public rooms, minigames, promotional areas, prizes, and website content.

Engine selection, technology labels, source collections, and prototype launchers are not player features. Existing host/community branding may remain in an outer hosting context if required; do not pass it off as historical interface artwork.

## Technical direction

Continue `sources/gocoke/cmd/client` and `internal/client` for rendering and gameplay. Continue `cmd/server` for HTTP, authentication, protocol adapters, authoritative state, and persistence. Extract focused files as individual features change; a broad framework rewrite is unnecessary.

The Go server should serve the website, WASM client/assets, and game endpoints from one public origin. Existing AMF and realtime codecs are compatibility boundaries to audit and retain where they help fidelity. Their current implementation is not automatically an accurate reproduction of the original protocol.

Session identity must come from verified authentication. URLs, localStorage, client-supplied names, and arbitrary XML attributes cannot authorize a player or select another player's inventory. Server rules determine ownership, legal actions, prices, rewards, and committed state.

CokePhase and CSMusicMixer remain executable reference tools where useful. Their source/assets can inform Go work subject to their licenses. They are not parallel shipping engines or substitutes for a broken Go feature.

## Fidelity acceptance

| Dimension | Required evidence |
|---|---|
| Visuals | Reference/current/diff captures at the same logical resolution and state; correct art, text, alignment, sprite anchors, layering, hit areas, scaling and window geometry |
| Animation | Reference frame order, orientation, state transitions and timing; walking, sitting, drinking, performing, and applicable gestures |
| Sound/music | Matching identified samples, scheduling and mix behavior; save/load round-trip and synchronized room playback |
| Interaction | The same click/key produces the reference action, feedback, dialog, next state, and error behavior |
| World | Correct room identity/layout, entrances, walkability, object behavior, public/private access and occupancy |
| State | Separate player identities, server-authorized mutations, correct persistence across logout/restart, atomic trades/purchases/rewards |
| Multiplayer | Two independent browser sessions see matching joins/leaves, motion, chat, objects, performance and reconnect state |
| Historical scope | Every completed screen/rule is tagged to the chosen version; exceptions and unavailable evidence are listed |

Static reference comparisons should be exact where possible. Any tolerance or dynamic mask must be justified per fixture, recorded, and reviewed; do not use an arbitrary percentage to conceal differences. A deterministic replay should fix time, avatar, room, camera, and animation state for comparisons.

Modern implementation may improve transport, storage, password handling, and browser compatibility without altering evidenced player-facing behavior. Recreating historical vulnerabilities is not a fidelity requirement.

## Unknowns must not become invented rules

Re-derive projection, sprite scale, movement/animation timing, mixer structure/duration, chat ranges, catalogue prices, starting balances, voting/reward formulas, chart periods, and minigame rules from matching sources. Existing constants are candidate implementations, not proof.

The old Bible's 128x64 tile diagram, 16-bar/32-second song statement, and the server's fixed 60-second performances illustrate why these need reconciliation. Do not replace one unsupported constant with another.

Prototype minigames and sample charts stay explicitly incomplete until their actual state machines, rules, UI and rewards are implemented. Missing mechanics are work remaining, not permission to substitute an easier game.

## Definition of done

A feature may be marked historical-parity verified only when:
1. Its versioned reference and expected behavior are recorded.
2. The normal player reaches it through the Go product.
3. Its appearance, input, output, timing and sound match the reference.
4. Its state changes are authorized and persist when the feature requires persistence.
5. Multiplayer and failure paths relevant to that feature pass.
6. A reproducible test/replay and comparison artifacts exist.
7. Remaining differences are documented rather than hidden by fallback content.

The first milestone is a faithful entrance into a shared Go world. It is an incremental delivery toward the full recreation, not a reduction of scope to a technology demo.
