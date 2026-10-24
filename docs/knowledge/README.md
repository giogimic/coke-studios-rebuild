# Coke Music / Coke Studios knowledge base

Research update: 2026-10-24. Purpose: give the Go recreation a traceable reference for the original website, interfaces, pages, artwork and connected player journeys.

This is an evidence catalogue, not a claim of game completion. Original client exports, screenshots preserved by fans, original developers' recollections, community reconstructions and present-day revivals have different evidential value. Keep them distinct.

## Read in this order

1. [Evidence and versions](EVIDENCE-AND-VERSIONS.md): what is established, provisional or conflicting.
2. [Website structure](WEBSITE-STRUCTURE.md): navigation, page contents, forms, popups and missing destinations.
3. [Game interfaces](INTERFACES.md): recovered dialog families and controls.
4. [Player journeys](PLAYER-FLOWS.md): the states each connected interaction needs.
5. [Music](MUSIC.md) and [minigame rules](MINIGAMES.md): concrete behavior traced to preserved sources.
6. [Visual references](VISUAL-REFERENCES.md): inspected screenshots and their limits.
7. [Sources and retrieval](SOURCES.md), then [remaining gaps](GAPS.md).

## What is actually here

The generated corpus indexes existing preserved files; these are not newly downloaded file counts. Current exact totals are in [summary.json](generated/summary.json); integrity and source-line checks are recorded in [VERIFICATION.json](VERIFICATION.json). This pass indexed 14 preserved HTML pages (13 reconstruction templates/pages plus the Uncover rules page). The expansion includes 18,793 files, 117 window definitions, 2,856 window elements, 1,881 Lingo scripts and 14,797 handler declarations. Files with identical content remain preserved and are grouped by hash.

| Reference | Contents |
|---|---|
| [Recorded observations](observations.json) | Twelve source-specific findings with hashes, locators and confidence limits |
| [File inventory](generated/files.jsonl) | Path, size, SHA-256, provenance category, measured image dimensions where supported |
| [Asset gallery](generated/ASSET-GALLERY.html) | 446 local website/interface images and preserved screenshots; open original images for full-size inspection |
| [Interface index](generated/INTERFACE-INDEX.md) | Every recovered window, rectangle, embedded date and element count |
| [Window definitions](generated/interface-windows.json) | Element IDs, members, positions, fonts, colors, dimensions, text keys and original lines |
| [Website links](generated/WEBSITE-LINKS.md) | Static links and popup targets; missing targets are not treated as recovered pages |
| [Website pages](generated/website-pages.json) | Titles, forms, controls, images, embedded objects and text |
| [Text keys](generated/text-keys.json) | 731 records across text and feature datasets, retaining source/section/line; duplicate versions count separately |
| [Control actions](generated/interface-actions.json) | Recovered control IDs mapped to dispatch source lines and matching window elements |
| [Interface text links](generated/interface-text-links.json) | 508 text-key references joined to candidate text datasets |
| [Catalogue records](generated/catalogue-records.json) | 601 records across distinct preserved catalogues; not 601 unique purchasable items |
| [Lingo handlers](generated/lingo-handlers.json) | Handler name, parameters, source file and line |
| [Cast member index](generated/cast-members-index.json) | Locations of member tables needed for anchors and cast-name mapping |
| [Known-file fingerprints](generated/community-known-file-fingerprints.json) | 446 SHA-1 entries from the preservation project's known-file list |
| [Duplicate groups](generated/duplicate-files.json) | Identical byte content, without deleting any source |
| [Historical URLs](generated/historical-url-occurrences.json) | URLs found inside preserved text, with file and line |

## Rebuild or extend the collection

From the project root:

```powershell
node tools/research/index-knowledge.cjs
node tools/research/collect-captures.cjs --list
node tools/research/collect-captures.cjs --limit 5
node tools/research/verify-knowledge.cjs
```

The indexer reads preserved material and writes this directory's generated indexes. It does not run old Flash, Director, installers or cache-finder programs. Text parsing is limited to files up to 2 MiB; LASM and other binaries are inventoried rather than interpreted. HTML parsing is static; runtime links, malformed markup and Flash navigation need manual inspection.

The capture collector uses the explicit [38-item queue](acquisition-queue.json), records failures, retains redirects and hashes successful responses. Downloads go into sources/captures beneath this knowledge base. It never overwrites a previously captured payload. Re-running skips saved captures. Original capture timestamps must still be checked after retrieval.

Direct network downloads failed at DNS resolution in this session. Eighteen original-developer image URLs and twenty page/rules URLs are queued, **not represented as downloaded**. Web-readable research was summarized in the source notes. Existing local screenshots are usable immediately.

## Adding a historical claim

Record the source path or URL, file hash/capture timestamp, edition, exact observation, confidence and remaining uncertainty. A source file can establish a control label without proving its active runtime position. A screenshot can establish a visible menu without proving what its links do. A community remake can suggest where to look without proving original behavior.

Use this evidence to implement complete journeys in Go. This research gallery is for development and must not appear in the game's player flow.
