# Website structure, pages and menus

This guide separates three evidence sets: the early screenshot, the later promotional screenshot, and the 2015 CSWebsite reconstruction. The preserved rules page provides another late-site fragment. No complete original site crawl has been recovered in this session.

See [visual references](VISUAL-REFERENCES.md) for images and [generated links](generated/WEBSITE-LINKS.md) for every extracted destination. Source paths below are relative to the project root.

## Early Coke Music screenshot

Source: sources/cache-finder/docs/img/cokemusic-2002.webp. Visually inspected on 2026-10-24. The filename supplies a year label; the original capture date and URL are unknown.

The image shows a narrow left account column and a larger main content area. A Coke Music logo caps the left column. The account card contains a V-ego portrait, Email Address and Password fields, Edit Info and Submit controls, then Help and Register. Below it is a poll with three radio choices and a submit button; a Downloads tile sits below the poll.

The top navigation, left to right, reads:

| Menu | Visible evidence | Destination |
|---|---|---|
| Home | Top strip | Unknown from screenshot |
| Launching Pad | Top strip and large content tile | Unknown |
| Coke Studios | Top strip and illustrated studio tile | Unknown |
| Create Your Demo | Top strip; “Mix Music” content tile | Unknown |
| Coke FM | Top strip; horizontal player/promotion tile | Unknown |
| Downloads | Top strip and lower-left tile | Unknown |
| Help | Top strip and account card | Unknown |

Main content combines a large Launching Pad panel and Listen and Rank action; an introductory text panel; a summer-promotions panel; Coke FM; Mix Music/Create Your Demo; a Coke Studios scene inviting entry; Send A Virtual Postcard; and Invite A Friend. These are separate visible entry points, not interchangeable modern dashboard cards.

Do not turn visible labels into invented route URLs. This screen does not establish signed-in layout, validation errors, hover frames, popup chrome or precise behavior.

## CSWebsite shell: preserved reconstruction

Sources: sources/cs-website/templates/page.html, www/css/css_coke.css, server.js and www/js/library.js. README identifies a recreation of the website before MyCoke; source comments date the reconstruction to 2015.

The wrapper embeds badge.swf at 176 x 452, with the main navigation beginning at x=176. Navigation slices total 584 pixels and are 26 pixels high. CSS also sets a content iframe width of 688 at left=176, top=39: this discrepancy needs an original screen comparison before being treated as intentional historical geometry. “Start original” markers in CSS/JS are useful provenance leads, not independent authentication.

| Label in preserved wrapper | Target section | Template route present? |
|---|---|---|
| Home | home | Yes: home.html |
| Cokestudios | cokestudios-sub | No registration in server.js |
| Games | games-sub | Yes: games.html |
| Music-Mixer | music-mixer-sub | Yes: music-mixer.html |
| Music | music-sub | No registration |
| Decible Central | decible-central-sub | No registration; spelling preserved from source |
| Downloads | downloads-sub | Yes: downloads.html |
| Help | help-sub | Yes: help.html |

The reconstruction supplies shell routes and “-sub” content routes for music-mixer, help, registration, games, downloads, lpad and postcard, plus license. create_demo is rewritten to music-mixer. These are this reconstruction's route mappings; old-looking JSP paths do not prove they reproduce the original server's routing.

## Page-by-page evidence

| Page/source under sources/cs-website | Contents or controls recovered | Missing / qualification |
|---|---|---|
| templates/home.html | Website wrapper/assets, messenger popup reference, reconstruction attribution | Contains modern author/license content; do not copy that into historical home |
| templates/lpad.html | Launching Pad template and artwork references | Not a complete recovered artist/music service |
| templates/music-mixer.html | Mixer introduction, studio/game links, Create Your Demo launch | /music-mixer-app/ and openMixer are reconstruction wiring |
| templates/games.html | Game landing/launch code; 760 x 520 and alternate 760 x 550 popup configurations | Need exact capture to select version and chrome |
| templates/downloads.html | Five category links: AIM, screensavers, Coke FM, MP3s, wallpaper | Category destinations have no matching subpage registrations in server.js |
| templates/help.html | FAQs, Music Pass, Decibels, Studios, Create Your Demo, Downloads, feedback | faqs.htm and feedback.jsp not supplied as complete working pages here |
| templates/preregistration.html | Entry to registration-go and site framing | Multi-step registration-go destination not implemented by this source set |
| templates/preregister.html | Cookie requirement and browser-specific help | Historical browser instructions, not a current account implementation |
| www/popup/whyregister.htm | Membership explanation, register and close actions | My Thirst ID / SMS text needs dated original-site corroboration |
| templates/postcard.html | Card/music selection, message, sender and up to five recipients, preview/send | Server servlet, delivery behavior and full media set unverified |
| templates/license.html | 2015 reconstruction license/attribution | Development provenance, not an original game page |
| templates/404.html | Reconstruction error page | Not evidence for an original error screen |

## Help and Music Pass

The preserved help page has a compact topic index, anchored sections and repeated Back to Top links. Named sections are The Music Pass, Earning and Using Decibels, Coke Studios, Create Your Demo and Downloads. FAQs is a separate link. Feedback opens a 380 x 320 window.

Music Pass text describes a persistent V-ego identity/look, Decibel information and access to earning activities. These are source-text observations. Its amounts, account conditions and exact sidebar state must be attached to a dated version before becoming game rules.

The separate “why register” text distinguishes membership, earning, games, surveys, SMS registration and a My Thirst ID. It is evidence that the site extended beyond the room client; it does not justify inventing a modern social feed or generic account dashboard.

## Postcard flow

Preserved form action: POST /servlet/PostCard. Hidden fields include register, popup, msgID, moviefilename and message_text. Sender identity and recipient name/email fields cover five recipients. The page provides a card/music selector and preview/send controls. Preview uses a 510 x 460 popup; confirmation uses 640 x 480.

This identifies a website flow, not an in-game messenger. Its source links include /registration/index.htm, privacy policy and rules. Some JavaScript has archive-rewritten URLs; retain those raw references in the source inventory.

Required evidence still missing: allowed custom-message states, all selectable cards/music, original validation wording, send success/failure states, member versus guest differences. Do not send mail or contact historical addresses while researching.

## Shark Tale promotional page

Source: sources/cache-finder/docs/img/sharktale-site.webp; visually inspected. The image contains Shark Tale @ Coke Studios, a “Have a code?” promotion, Members Only/Register Now, Whale Wash Builder and Whale Wash Activity. Footer text includes a 2004 copyright. Several image resources are visibly missing in the preserved screenshot; missing areas are not blank design intent.

Treat it as a promotion-specific branch. Do not use this screenshot to infer that every historical home page had Shark Tale navigation or code redemption.

## Later MyCoke fragments

sources/uncover-the-music/_resources/_other/html/RevealChallengeRules.htm preserves an Official Rules page with a Close control. Its associated css/styles07.css references original mycoke.com/images/2007 assets: registration step panels, input and captcha backgrounds, and rounded-corner images.

These CSS selectors and URLs are evidence of referenced assets, not proof that complete registration steps are present. Only the rules page and a few corner/spacer GIFs are saved in this folder. See [minigame evidence](MINIGAMES.md) for its concrete gameplay rules.

## Pages still needing original captures

Home in both signed-out and signed-in states; all registration steps and errors; account/profile editing; password recovery; Music Pass/Decibel tracker; Decibel Central; Coke FM/player; Launching Pad and artist detail; charts (daily/weekly); game-specific lobbies and instructions; downloads categories; invite-a-friend; feedback; complete legal/rules pages; closed/full/maintenance states; promotion code entry and results.

The acquisition queue contains exact discovered archive links. Additional routes must come from actual page links, recovered code or archive indexes. A plausible filename is not evidence.
