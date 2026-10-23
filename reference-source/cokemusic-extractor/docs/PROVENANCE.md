# PROVENANCE — cokemusic-extractor

## What this repo is

Tools to extract structured data from the cast files of an abandoned
2005-2007 browser game (CokeMusic / Coke Studios). The game was
discontinued by Coca-Cola in 2007 and has been offline since. Fan
archival projects have mirrored cast binaries from period community
sources.

## What this repo is NOT

- **Not a runtime port.** This is format-conversion only. To play
  Director files in a browser, see
  [igorlira/dirplayer-rs](https://github.com/igorlira/dirplayer-rs)
  (which this repo's bitmap + sound dumpers depend on).
- **Not a distribution channel for the game's assets.** This repo
  ships tools only — no cast files (`.cct`, `.dcr`), no extracted
  PNGs, no extracted JSON.

## Cast files

Cast files originate from The Coca-Cola Company circa 2005-2007.
The files are not redistributed here. Users supply their own
(typically sourced from community Internet Archive mirrors of the
period client). The lockfile + downstream tooling can ship SHA256
manifests to verify provenance.

## Trademarks

"CokeMusic", "Coke Studios", and "Coca-Cola" are trademarks of The
Coca-Cola Company. This project is not affiliated with, sponsored
by, or endorsed by The Coca-Cola Company. The trademarked terms
appear in this codebase only as factual references to the product
this toolchain extracts data from.

## License

This toolchain: GPL-3.0-only (see [`LICENSE`](../LICENSE)).

The dirplayer-rs fork (`reowens/dirplayer-rs`) which this toolchain
depends on at runtime is also GPL-3.0, inherited from the upstream
project [igorlira/dirplayer-rs](https://github.com/igorlira/dirplayer-rs).
Upstream credit and lineage are preserved at the GitHub fork level
(the "forked from" badge appears on the repo page).

The extractor and its dirplayer dependency both declare `GPL-3.0-only`.
Release manifests are authenticated by signed Git tags. This tools-only
repository does not release cast payloads or generated/proprietary asset
bundles.
