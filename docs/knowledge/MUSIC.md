# Music creation, saved slots, CDs and playback

This investigation found a useful distinction: the recovered Director CC-3001 interface is a five-slot saved-mix / play / edit / burn surface. Editing launches an external mixer and pauses the in-game interface. It should not be described as a four-track editor merely because a modern reconstruction has four tracks.

## Evidence traced locally

| Source relative to sources/gocoke/web | Observation | Confidence / limit |
|---|---|---|
| data/Text_English.txt, MUSIC MIXER section | CC-3001 Deluxe title describes five slots; Play/Edit/Burn labels; empty/loading/playing/editing text | Direct text-file observation; selected dataset version still unresolved |
| Lingo/mixer/37__MIXER_.ls, updatecontent | Initializes slots 1–5, then displays returned mix names | Direct recovered code |
| Same, playMix | Missing slot returns without playing; existing song passes to loadOddCastSequence then play | Direct recovered code; sound output not replayed |
| Same, editMix | Stops sequencer, opens paused.window, obtains mixer URL, launches javascript:openMixer(...) | Direct recovered code |
| Same, editMix parameters | Uses door, client, session SID, mix number; existing mix also includes MID | Documents historic integration shape, not a requirement to restore obsolete authentication |
| Same, burnMix | Requires an existing mix and backpack with at least one blank CD; otherwise ALERT_NO_BLANK_CDS | Direct recovered client check; server enforcement still separate |
| Same, burnMix | Opens burn_decision.window and substitutes current blank-CD count into BURN_DESC | Direct recovered code |
| Lingo/entry/67_WindowInteractivity.ls | Play/Edit/Burn control handlers are assigned for each of five slots; BURN_OK, BURN_CANCEL and unpause are distinct | Direct recovered UI wiring |
| Lingo/isoengine/124_ACTION_MIXER.ls | Moves toward the item; double click invokes use; use requires standing in front and enabled furniture use | Direct recovered code |
| Same, openmixer | MIXER feature flag; room owner or moderator allowed; otherwise MIXER_NOT_OWNER | Direct recovered client code; does not alone prove historical server authorization |
| Lingo/cc_room/113_cc_cdplayer.window.txt | 28-element CD player template | Source present; interaction replay pending |

The CC-3001 window occupies [0,0,760,520] in its definition. That is not proof it contains the complete music editor. The external editor needs its own screenshots, SWF/source identification and behavior trace.

## Burn and reward rules: do not overreach

The inspected burn code establishes a blank-CD requirement and confirmation. It does **not** establish an additional Decibel burn fee. Trace the burn confirmation handler, gateway and selected historical catalogue before asserting costs, item consumption or transaction results.

The results text contains placeholders for up votes, down votes and awarded Decibels. A placeholder proves the display field exists, not the payout formula. Likewise DRINK_COKE contains a points placeholder; it does not prove a daily cap or reward value.

Local text lists room-owner restrictions and missing-blank-CD errors. Preserve these failure states in the player journey rather than displaying a generic success toast.

## Other preserved music evidence

sources/music-mixer is a community reconstruction, with preserved asset packs and supporting code. Its behavior is not an independent specification for the original editor.

sources/cache-finder/src/known.c lists an am_habbo_v2.swf fingerprint and the community README discusses the OddCast editor and missing server-side mix/preset data. A known fingerprint proves that maintainer indexed a file; it does not mean this workspace contains that exact binary. Check the inventory before claiming recovery.

The Infinite Jukebox has separate catalogue, playback and playlist window definitions: cc_room/158, 208, 222 and OLD 223. Keep that service distinct from player-created mixes and CDs. Third-party streamed tracks must not be substituted for original mix samples.

Next evidence to acquire: complete original editor screenshots, its exact SWF version, button/track interactions, sound packs and tempo/sequence interpretation, save/return states, a burned CD's metadata, performance timing, voting result examples and archived chart pages.
