# Connected player journeys

These are reconstruction research checklists, not declarations that the Go implementation passes them. “Observed” means the named source contains the state/control. “Unresolved” means the exact original behavior still needs evidence. Cross-reference [interfaces](INTERFACES.md), [music](MUSIC.md) and [minigames](MINIGAMES.md).

| Journey | Evidence currently present | States still needing reference confirmation |
|---|---|---|
| Website -> membership -> V-ego -> game | Early account-panel screenshot; preregistration/whyregister templates; figure name/info dialogs; SF_Client boot scripts | Registration step order, email versus screen-name login by era, duplicates/invalid values, signed-in badge, expired login |
| Navigator -> room -> another player | Public/private/info/people/search windows; password and door dialogs; room toolbar | Room sorting, full rooms, live counts, entry position, loading/error states, doorbell owner response |
| Select player -> profile/friend request -> message | Figure/profile windows; context member Ask to friend; messenger request/friends/compose/received/removal windows | Offline behavior, unread indication, limits, request cancellation, invite wording |
| Select mixer -> choose saved slot -> edit -> return | Five-slot CC-3001 text, play/edit/burn handlers, paused window, external mixer launch | Original editor screen states and complete save callback/result sequence |
| Saved mix -> blank CD -> burn -> owned CD | Backpack blank-CD query, no-blank-CD alert, burn count/confirmation, CD-player window | Authoritative original burn transaction and errors; fees beyond the blank CD are not established here |
| Room performance -> listener vote -> result | Active/personal/vote windows; thumbs-up/down/results text; early Alaska screenshot | Queue capacity, timing, vote eligibility, disconnect handling, reward formula, daily/weekly charts |
| Catalogue -> buy -> backpack -> decorate | Catalogue variants/data, purchase dialog, backpack, move/rotate/pickup/delete context members | Version-specific prices, unaffordable/failed purchase wording, wall/floor preview, collision and ownership details |
| Player -> trade -> mutual agreement -> transfer | Two-sided six-slot trade layout, agreement/cancel controls, trade text | Offer-change resets, simultaneous accept rules, cancellation/disconnect handling and durable transaction |
| Help -> assistance request -> acknowledgement | Help, call-for-help, moderator-help, call-sent windows | Which menu opens each, input limits, moderation routing and status wording |
| Website -> minigame lobby -> shared match -> Decibels | Uncover preserved rules; other game source packages and visual leads | Version selection, exact art/sound, complete rules and payouts for each game separately |
| Website -> postcard -> preview -> send | Preserved postcard form and popup sizes | All card media, member restrictions, validation, success/error pages |
| Website -> downloads/music/charts | Category links, artist/Launching Pad material, archived chart leads | Actual destination payloads and era-specific content |

## Use the states to direct implementation

1. Pick the historical edition and entry screen before adjusting a menu.
2. Write the evidence-supported normal path and the empty, cancel, denied, missing-resource and disconnected states.
3. Identify what belongs to the website, Go client and authoritative Go server while retaining one account/world state.
4. Compare visuals and interactions against the selected reference. Server tests are additional evidence, not a substitute.
5. Update PARITY only at the level actually verified.

Modern requirements such as secure credentials and authoritative transactions are engineering obligations. They should support the recovered player-facing behavior without being presented as newly discovered historical rules.

Do not invent amounts, sample popularity charts, fake online users or successful actions to fill a missing screen. A feature flag in FeatureSet.txt shows a named capability in that dataset; it does not establish complete original rules or current implementation status.
