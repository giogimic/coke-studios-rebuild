# Studio repair review — 2026-10-25

## Verdict

The user's reports are consistent with code defects, not just missing polish. The current game is not identical to Coke Music. The modern website is an approximation; Sumo remains a prototype. Several tests bypass the real Go client, so their old completion labels were too strong.

## Reproduced failures and repairs

1. **Invisible registered characters.** The portal emits `hr=.../3&hd=003/1&...`; the Go renderer requires RGB triples. Its parser skipped every body part. A regression using the website's exact format failed with all twelve body parts missing, then passed after adding a shared compatibility conversion. Registration/profile updates now save renderable definitions; existing records are interpreted on load without rewriting the database. Valid original RGB definitions remain intact. The old portal's skin palette is explicitly reconstruction data; neutral hair/clothing tint and legacy style mapping still require visual comparison with its separate preview assets.
2. **Room packets discarded during scene creation.** Entry sends `EnterRoom` before installing the scene. The WASM callback previously discarded avatars when no scene existed or delivered new-room events to the old active scene. Added an ordered inbox keyed by room and an explicit game-loop drain after scene installation. Tests cover delayed scenes, ordered spawn/movement/music, exactly-once draining and stale-room isolation. The connection now retains the actual current room and exits that room during transitions.
3. **DJ protocol disconnected from the UI.** The real client sends `CD_R cdPlId` and then `CD_P cdId`. The server had no request handler and accepted only a raw `s` sequence. Added the request/accept exchange, actual owned-CD lookup, saved composition/author resolution, XML escaping, and rejection of arbitrary sequences or unowned IDs. Active performances cannot be overwritten. Settlement emits the stop message the client understands. Late-join packets now include the artist/title attributes the renderer reads; actual seek synchronization is still open.
4. **London's visible DJ table was not interactive.** It is a static cast item, excluded from the placed-furniture hit tester. Added a Go hit area using the recovered member's actual image and registration point, connected to the same CD chooser. The test loads `Lingo/london/58_SceneXml.txt`, `Members.csv` and the table image and exercises the click callback. This verifies the cast-to-click wiring, not rendered pixel parity or the original walking/queue sequence.
5. **Builds could silently undo fixes.** Both build scripts restored the tracked old WASM when a new build exceeded 100 MiB. Removed that fallback. PowerShell now checks Go compiler exit codes. Removed session tickets from browser/WASM authentication logging.

## Why rooms still look wrong

`loadRoomDefinition`, `loadStudioBackground`, `loadMemberRegistry`, `loadRoomAssets` and the isometric data loader select London for layout 5. Many different public names share that layout value. The room inventory therefore counts labels, not complete room implementations. The server also contains aliases with unrelated names/layouts. No extra room was declared fixed in this repair.

Keep London I (`$SYSTEM$1`) as the first comparison target. Next, map every supported public-room ID to one matching cast, scene XML, PathMap, entrances and interactions. Disable or clearly mark unresolved destinations until they have their own assets and behavior; do not duplicate London under another name. The private studio B/D swaps were removed in this repair; client/server coordinate conversions and reference comparison still need verification before movement is considered correct.

## Ordered remaining work

1. **Prove one connected studio.** Run the website registration -> Go navigator -> London journey in two independent browser contexts using the newly built artifacts. Assert actual rendered avatars, identity, movement/chat, scene changes, disconnect/reconnect and missing-asset failures. Add screenshot comparisons at 760x520. Raw socket checks remain supplementary.
2. **Unify wardrobe and profile rendering.** The portal preview still uses a separate artwork/choice scheme. Use the same verified body-part IDs, colors, gender/body rules and compositing data on both sides; demonstrate the saved selection on both players' screens. Do not substitute another default avatar and call it a match.
3. **Complete the music loop in Go.** Exercise real composer controls, save/reload, burn failure paths, CD ownership, visible booth selection, synchronized playback/late-join seek, stop/reconnect and voting. Verify duration/sample resolution against the recovered sequencer. Authenticate reward rules; the current formula is still provisional.
4. **Restore room-specific behavior.** Associate each room with its own reference assets and collision map; fix depth/occlusion and public furniture actions. Prove private studio ownership and decoration with a second player observing.
5. **Replace Sumo as a complete game.** Inventory the preserved V-ego San executable/scripts, sprites, animation sequences, sounds, move-resolution logic, lobby/opponent behavior, timers, transitions and prize conditions. Resolve version uncertainty first. Build those in Go and connect authenticated state. HTML emoji fighters or a passing move matrix are not substitutes. Do not label the current prototype fixed.
6. **Modernize the website around the faithful game.** Retain the evidenced account/navigation/content hierarchy while improving responsive layout, readability and browser access, per the user's current direction. Keep one Go game and account/world. Research galleries, debugging controls and alternative engines stay outside normal play.

## Verification and limits

`go test ./... -count=1` passed. New regressions cover the exact failed avatar definition, delayed room snapshots, London table click bounds and owned-CD protocol. The old raw-sequence server test was changed to mint an explicit test CD. The browser music protocol test now submits that CD's possession ID.

The Go WASM client compiled successfully from these sources. See the final build results below for packaged artifacts.

The test server used `.runtime/studio-repair/test.db`, separate from player data. Local HTTP failed with a socket access-permission error and Chromium timed out/refused the connection. No browser rendering, actual sound, two-client canvas, MariaDB, historical comparison or full restart test is claimed. No original archive/assets or player database were removed/reset. No public deployment was performed.

## Continuation repairs and final verification

- **Registration -> personal studio:** the server launch URL now includes the actual created studio ID; the portal stores that ID and the server-normalized appearance. The Go boot reads the requested room and uses the existing authenticated room-entry path. It no longer constructs and discards an extra scene before main creates the real one.
- **My Studios:** the missing AMF `getByOwnerName` handler now filters by owner and returns field names the Go navigator reads, including room ID, owner and occupancy. `TestRegistrationLaunchAndStudioDirectory` runs the real Go AMF client against the actual HTTP handler using an in-process transport, not a canned response or local socket.
- **Private layouts:** removed the client-only B/D swaps. `TestPrivateStudioLayoutIsNotSwapped` verifies layouts 1–4 retain the server's values. Public room identity/cast mapping is still unresolved.
- **Audio event boundaries:** a reproduced mixer bug repeated a one-loop sample to the end of the read buffer. Mixing is now clipped to each event and the song's remaining duration. A PCM-level regression verifies silence after the event and correct EOF.
- **Late joins:** the Go client consumes the server's offset and the streaming mixer supports frame-aligned seeking. A regression verifies playback resumes at the expected PCM frame. End-to-end speaker synchronization, network latency compensation and the historical sequencer timing still need live comparison.
- **Room audio reuse:** a regression reproduced `audio: context is already created` when constructing a second stereo player. Stereo players now share Ebitengine's audio context.

Final `go test ./... -count=1`: PASS. The opt-in render fixture is skipped by this default command. Four website inline JavaScript blocks parse; the PowerShell build script parses; `git diff --check` passes. `tools/build-go.ps1` successfully rebuilt the WASM client plus Windows and Linux servers. Exact file sizes and hashes are in [STUDIO-REPAIR-BUILD.json](STUDIO-REPAIR-BUILD.json); these identify the generated artifacts, not historical parity or independently reproducible builds.

**Native visual capture was attempted and blocked:** `TestLondonRenderFixture` reached Ebitengine startup, which returned `glfw: ChangeWindowMessageFilterEx failed: Access is denied.` No screenshot was produced. The opt-in fixture remains available for a normal development desktop via `GOCOKE_RENDER_SNAPSHOT=<absolute output PNG path>` and `go test ./internal/client/scene/studio -run TestLondonRenderFixture -count=1` from the Go module. It contains explicitly named fixture avatars; even a successful fixture capture is not proof of two real players connecting.

The outstanding acceptance gate is unchanged: actually play the connected journey through the rendered Go client, then compare its visuals/audio to the selected historical reference. Sumo, the modern website's fidelity, the full public-room inventory and historical reward rules remain incomplete.
