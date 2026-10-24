# Evidence and historical versions

Research qualification: 2026-10-24. These findings qualify historical claims in the Bible and baseline; they do not undo implementation work or its tests.

## Evidence levels

| Class | What it can establish | What still needs checking |
|---|---|---|
| Original page captured by an archive | Its saved markup, text, resource URLs and capture context | Missing assets, archive rewrites, redirects, original publication date |
| Original client/cast export | Preserved code, members, coordinates, strings and referenced assets | Export quality, original build identity, active versus obsolete code |
| Screenshot or recording | Visible state; recordings can establish sequences/timing | Capture date, edition, crop, scale, edits, missing offscreen states |
| Original developer's retrospective | First-person context and implementation/interaction recollections | Exact release applicability and precise constants |
| Community preservation package | Bytes preserved and the maintainer's annotations | Chain of custody, filename dates, modifications |
| Community reconstruction or current revival | That project's implementation and potentially preserved assets | Whether each behavior/layout ever existed in the original |
| Wiki, forum or player recollection | Discovery leads and corroborating descriptions | Primary evidence before adopting exact rules |

A file hash identifies bytes. It does not authenticate history by itself. A comment saying “original” or “1:1” is an assertion requiring corroboration.

## Keep these references separate

| Reference group | Local evidence / lead | Working interpretation |
|---|---|---|
| Early Coke Music website | cache-finder/docs/img/cokemusic-2002.webp; dated 2002/2003 archive leads | Screenshot shows a distinct navigation and account panel; 2002 comes from the preservation filename |
| Early client / V1 | cache-finder/docs/img/v1-alaska.webp; maintainer wanted list | Preserve separately; do not infer a complete playable V1 client exists |
| Beta | beta-ingame.webp, beta-splash.webp | Separate visual evidence; no release date authenticated here |
| Shark Tale promotion | sharktale-site.webp, sharktale-splash.webp, sharktale-studio.webp, whale-wash.webp | Promotion-specific screens; not a universal default world |
| March 14, 2006 client-labelled folder | sources/original-client-source/Clients/March 14, 2006 | Distinct recovered decompilation; label retained verbatim |
| May 3, 2006 client-labelled folder | sources/original-client-source/Clients/May 3, 2006 | Distinct candidate; do not silently merge with March |
| October 8, 2007 client-labelled folder | sources/original-client-source/Clients/October 8, 2007 | Later candidate; keep website and asset associations explicit |
| 2007 website fragments | uncover-the-music/_resources/_other/html/RevealChallengeRules.htm and css/styles07.css | Late website/rules material preserved in a community repo |
| CSWebsite reconstruction | sources/cs-website/README.md; 2015 source copyright | Pre-MyCoke recreation, not a dated original website capture |
| Decibel and other current revivals | Live sites, community source repos | Reference-only; later features are not historical proof |

All paths without a sources prefix in this table are relative to sources/.

## Corrections to unsupported certainty

- **760 x 520 is supported for the exported SF_Client movie.** sources/gocoke/web/Lingo/Movies.csv gives Director version 10.0 and stage rectangle (89, 50) to (849, 570). That row does not identify a March/May release date.
- **March and May are two candidates.** The baseline's combined “March 14 / May 3” designation is not a single authenticated release. A file-level association remains necessary before a parity claim.
- **1520 x 1040 does not prove Retina support.** The doubled drawRect in SF_Client/2_MAIN.ls is an observed allocation. Its rendering purpose must be traced; do not label it a historical Retina display requirement.
- **Timing numbers need specific evidence.** The baseline's 24–30 FPS and 12–16 tick descriptions require named source handlers or measured recordings. This research did not validate those ranges.
- **Website widths do not date the website.** The 176-pixel badge and 584-pixel navigation are observable in the CSWebsite reconstruction. Matching their sum to a Director stage width does not establish that the website and game are the same historical release.
- **The early screenshot has different menus.** It shows Launching Pad, Create Your Demo and Coke FM. Do not combine that screen with the reconstruction's Music/Games/Decibel Central navigation and call the result a single historical capture.
- **A window's rectangle may be a template.** The backpack is defined below the stage and the room bar extends beyond 520. Trace runtime placement before copying coordinates.
- **Embedded 2003 dates are authoring metadata.** They do not show that a window was shipped only in 2003 or that the entire cast belongs to 2003.
- **Different catalogues are already present.** The catalogue-parser copy and the complete-pack 5-23-6-labelled copy have identical SHA-256; the GoCoke catalogue differs. Keep all versions and compare rows before adopting prices or assortment.
- **Old/test variants are explicit.** Multiple catalogue and texture confirmation windows survive. Filename sorting is not a valid way to choose the active one.

## Reference selection record required before parity

Choose one client candidate; retain the exact archive/repository identity, hashes of client and casts, text/catalogue/feature datasets, stage dimensions, language, website capture and promotions enabled. Record which assets have an unresolved association. For each comparison, name a screen/state, use the reference's logical resolution, and distinguish observation from proposed reconstruction behavior.

The next fidelity work should close these evidence gaps while keeping the existing Go product moving. A working approximation is acceptable when labelled; relabelling it “authentic” does not resolve the missing evidence.
