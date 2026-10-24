# Recovered game interfaces

The strongest local structural evidence is the set of Director-style window definitions under sources/gocoke/web/Lingo. The [complete index](generated/INTERFACE-INDEX.md) contains 117 windows and the [JSON](generated/interface-windows.json) preserves 2,856 elements with raw source lines. Existence is verified; active version and runtime behavior remain separate questions.

Paths in the tables are relative to sources/gocoke/web/Lingo/.

## How to read a window

Each definition can specify name, authoring date, version, rectangle and elements. Elements carry a cast member, media type, local position, dimensions, palette, ink/blend, text style, control ID and sometimes a localization key. Preserve the original spellings, including strech/streched variants. The export is not a complete description of event wiring.

Use generated/interface-text-links.json to find matching text keys. Match the right text-file version, then locate event handlers in generated/lingo-handlers.json. Consult the relevant Members.csv for cast identity and registration points; pixels alone do not give sprite anchors.

## Persistent room toolbar

Source: cc_room/62_room_bar.window.txt. Embedded date 14.1.2003; rectangle [0,451,760,539]; 16 elements. This is a template coordinate space, not proof of final screen placement.

| Control ID / purpose | Local x,y | Size | Observed styling |
|---|---|---|---|
| int_speechmode_speak | 29,50 | 39 x 13 | Verdana bold 10; key speak |
| int_speechmode_shout | 29,65 | 39 x 13 | Verdana bold 10; key shout |
| cc.speechswitch | 9,46 | 19 x 35 | Bitmap switch |
| room_info_text | 16,0 | 240 x 26 | Verdana bold 10; room information |
| room_tooltip_text | 482,71 | 135 x 13 | Centered tooltip |
| chat_field | 85,55 | 357 x 13 | Verdana bold 10; edit model |
| Jukebox | 455,50 | 22 x 22 | Active icon member |
| Navigator | 479,50 | 22 x 22 | Active icon member |
| Messenger | 503,50 | 22 x 22 | Active icon member |
| Decibels | 527,50 | 22 x 22 | Active icon member |
| Backpack | 551,50 | 22 x 22 | Active icon member |
| Catalogue | 575,50 | 22 x 22 | Active icon member |
| Sound | 599,50 | 22 x 22 | Active icon member |
| Help | 623,49 | 24 x 24 | Active icon member |

The background member is 760 x 61 at local (0,27). A singing overlay is also present. Runtime placement, hover/disabled frames, unread messaging indication, tooltip text and audio toggles need script tracing. Adding the template origin directly would put part of this bar below a 520-pixel stage.

## Navigator, rooms and account dialogs

| Source family | Recovered states | What to resolve |
|---|---|---|
| cc_navigator[1]/59_nav_public_start.window.txt | Public-room starting screen; 63 elements; rectangle [434,66,721,427] | Active initial tab, room source/order and population |
| 60_nav_public_info.window.txt | Public-room information; 73 elements; same rectangle | Selected destination details and entry |
| 61_nav_public_people.window.txt | People view; 72 elements; [215,98,502,459] | Difference in placement and list refresh |
| 62_nav_special_info.window.txt | Special information; 65 elements | Which promotion/room invokes it |
| 63–66 private variants | Private start, information, people, search | Search/no-results, permissions, owner state |
| 72_nav_private_modify.window.txt | Modify studio; 23 elements | Editable fields, validation and save |
| 73–75 private deletion variants | Three distinct confirmation/result windows | Exact deletion sequence and cancel behavior |
| 76_nav_private_mystudio_info.window.txt | My studio information; 64 elements | Ownership-specific controls |
| 9_login_a, 10_login_b, 11_login_c | Three login states | Trigger/error meaning from event handlers |
| 16_nav_input_password through 20_nav_doornotopens | Password entry, checking, rejection, door opens/refused | Full access-control sequence |
| 21_figure_namepage, 22_figure_infopage | V-ego name and profile screens; 81 and 40 elements | Relationship to website registration and avatar selection |

Numbers in grouped names identify exported files, not chronological order. Ringbell and other windows also appear in the complete index. Do not collapse the navigator into one generic room picker.

## Messenger

All six definitions below use rectangle [221,59,548,386], a 327 x 327 template.

| File under cc_messenger[1] | Elements | Screen |
|---|---:|---|
| 10_console_myinfo.window.txt | 29 | Own information |
| 11_console_getmessage.window.txt | 20 | Received message |
| 12_console_getrequest.window.txt | 29 | Friend request |
| 13_console_friends.window.txt | 31 | Friends list |
| 14_console_compose.window.txt | 21 | Compose |
| 15_console_removefriend.window.txt | 20 | Remove friend |

Keep request acceptance/rejection, message receipt, composing and removal as distinct states. The existence of a composer is not evidence that offline delivery, capacity limits, invitations or read indicators are fully understood. Trace handlers and matching string keys before assigning those rules.

## Catalogue, wallet, backpack and trade

| Source | Evidence |
|---|---|
| cc_catalogue[1]/142,143,144 catalogue windows | Dynamic/category layouts with 39,24,57 elements at [188,70,580,458] |
| cc_catalogue[1]/22,23,24,25 catalogue windows | Other dynamic/page variants with 28,15,48,41 elements; 24 is explicitly old |
| cc_catalogue[1]/173_purchase_info.window.txt | 13-element purchase dialog at [200,145,489,285] |
| cc_catalogue[1]/202_delete_item.window.txt | 13-element delete confirmation at the same rectangle |
| cc_catalogue[1]/26_confirm_texture.window.txt | 14-element texture confirmation at [188,306,580,458]; OLD and OLDOLD versions have 21 |
| cc_interface[1]/28_cc_decibels.window.txt | 13-element Decibel display at [232,142,521,339] |
| entry/92_cc.backpack.window.txt | 36-element backpack at [431,520,669,856] before runtime repositioning |
| cc_room/43_sanfo_trading.window.txt | 49 elements, [208,178,548,396], six offer slots per side, agreement and cancel controls, warning text |
| cc_room/53_sanfo_trading.window_OLD.txt | 47 elements at [208,178,548,376]; preserve separately |

The trading variants have different heights and element counts; their presence does not establish which version shipped. Catalogue costs and category order come from a selected data file, not the geometry. Empty inventory, unaffordable purchase, cancellation and ownership failures must remain real states.

## Music, performance and voting

| Source | Evidence |
|---|---|
| mixer/6_cc_mixer.window.txt | 27-element mixer at [0,0,760,520] |
| mixer/41_burn_decision.window.txt | 13-element burn decision at [200,145,489,285] |
| mixer/43_paused.window.txt | 11-element paused dialog at [206,156,495,250] |
| cc_room/61_active_interface.window.txt | Five-element active interface at [117,231,525,249] |
| cc_room/64_personal_interface.window.txt | Two-element personal interface at [168,231,330,249] |
| cc_room/65_vote_interface.window.txt | Two-element voting interface at [174,231,393,249] |

The room folder also preserves CD-player, jukebox, infostand and performance-related members/scripts. The website OddCast mixer and the Director mixer export are distinct reference surfaces; their relationship must be demonstrated, not assumed. Compile a state sequence for compose, saved mix, CD burn, queue, performance, voting and settlement before a parity claim.

## Context actions and assistance

The isoengine folder contains exported text members for Kick Out, Move, Trade, Ask to friend, Rotate, Pick up, Delete and Dance. These .htm files are Director rich-text member exports, **not recovered website pages**. Availability depends on selected object, ownership, relationship and room state.

entry/100_cc.call_for_help.window.txt has 10 elements at [235,122,524,443]. cc_interface[1]/20_cc_help.window.txt has 19 at [237,138,526,458]; 29_MODERATOR_HELP.window.txt has 23 at [237,178,526,428]. Help, submitting a call and moderator tools are distinct screens. Staff tools belong outside normal player navigation.

## Before copying a layout into Go

Select the exact source variant; resolve its cast members and text keys; trace runtime repositioning and event handlers; record the visible state from a dated screenshot/recording; compare at logical resolution. Check hit areas, line wrapping, fonts, palette, transparency/ink, depth, hover/disabled states and transitions. A recreation screenshot using these assets is implementation evidence, not an independent historical reference.
