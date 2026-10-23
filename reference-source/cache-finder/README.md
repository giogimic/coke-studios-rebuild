# Coke Studios Cache Finder

Coke Studios (also Coke Music, later MyCoke) was Coca-Cola's online music game, from June 2002 to December 2007. Fans have since saved most of it: the 2006–2007 client, nearly every room and furniture item, and the website games. Some pieces are still missing, though. The game's versions from before 2006 are gone, along with a few rooms, parts of the website games and the music mixer's interface.

Some of those files are probably still sitting on old computers. Shockwave downloaded every room, sprite and script through the web browser, and browsers kept copies. This tool searches a computer, an old hard drive or a disk image for those copies and packs anything it finds into one .zip that you can send to the project.

## Using it

**Windows (95, 98, ME, 2000, XP, Vista, 7, 8, 10 and 11).** Download `CokeCacheFinder.exe` and double-click it. Nothing needs installing. On a computer too old to download it, copy it over on a USB stick.

Windows may warn that the program is unrecognised, because it is not signed. Click **More info**, then **Run anyway**.

**Mac.** Download `CokeCacheFinder-mac.zip`, open it, then right-click `CokeCacheFinder` and choose **Open**. That second step gets past the "unidentified developer" warning.

**Linux.** Run `./CokeCacheFinder` from a terminal.

**Older Macs** (before macOS 10.13, PowerPC included) can build it from source. Install the developer tools, then run `make` in this folder. It is plain C with no dependencies.

It asks what to search:

1. **This computer.** This is the one to choose. It checks every drive, including USB drives and inserted CDs.
2. **This computer, plus deleted files.** This reads the raw disk for files deleted years ago and can take an hour or more. It needs administrator rights; on Windows Vista and later it asks for them.
3. **One folder, drive or disk image.** You can drag it onto the window.

Anything found is listed as it turns up. At the end you get a folder and a `.zip` next to the program, and the folder opens. **Send the .zip.** Press Ctrl+C at any point to stop early; whatever was found so far is kept.

### Old hard drives

An old drive does not need its old computer. Connect it to any computer with a USB adapter, then search it with option 3 or by dragging the drive onto the program. The same works for disk images (`.img`, `.dd`, `.001`, …), which are searched end to end. To search a drive's deleted space directly, run the tool as administrator (Windows) or with `sudo` (Mac and Linux) and name the device:

```
CokeCacheFinder \\.\E:
```

```
sudo ./CokeCacheFinder /dev/disk4
```

### Options

```
CokeCacheFinder [options] [folder | drive | disk image | device ...]
  --deleted      also search the raw disks for deleted files (slow, admin)
  --out FOLDER   where to put the results (default: next to the program)
  --no-zip       leave the results as a folder only
  --yes          ask no questions and do not wait at the end
```

## Your privacy

- **It only reads.** It never changes, moves or deletes anything, and it never connects to the internet.
- **The zip holds only game files**: files that came from the game's websites, or Shockwave and Flash files that contain the game's own words. Everything in the zip is listed in `report.txt` beside it, so you can check before sending.
- **Your Windows user name is removed** from every path in the report, along with anything before an `@` (Internet Explorer's history uses `name@site`).
- **Web addresses are kept without their query strings** (everything after `?`), because those can carry session ids and screen names. Addresses containing "pass" or "pwd" are dropped entirely.
- **Cookie folders are not searched.** If Shockwave remembered a Coke Studios screen name (its `sfsn` preference), the report says so, but the name itself is never read.
- **Pictures and pages with "coke" in their name are listed but never copied**, because they might be personal. If they are screenshots of the game, the project would love them, but sending them is your choice.

## What it looks for

- **Shockwave and Director files** (`.cct`, `.dcr`, `.dir`, `.cst`, …) and **Flash movies** (`.swf`), recognised by their contents rather than their names, wherever they are.
- **Internet Explorer's cache** (`Temporary Internet Files`, IE 5 to 9). Its `index.dat` says which address every cached file came from and when the server last changed it. That date is how a cast's version is told apart from the archive's copies. Shockwave in Internet Explorer downloaded through this cache, which is why the archive's casts still have names like `cc_room[1].cct`.
- **Firefox's cache** (Firefox 1 to 31, and Mozilla and Netscape 7), read through its `_CACHE_MAP_`, so pictures and pages from the game's sites come back too.
- **Inside cache files, temp files, swap files and disk images**, for game files packed into larger files (Safari's `Cache.db`, Chrome's block files, Opera, Mac Internet Explorer's single cache file).
- **Deleted files in free space**, with the `--deleted` search. On a disk whose cache was cleared years ago, this is often the only way anything comes back. A file carved out of free space has no name and no address, so the tool **reads what it is from its own contents**: a room cast carries its room's name all through it (`london_…`, `walmart_…`), so a recovered Wal-Mart cast is reported as Wal-Mart, a London cast as the London room, and the catalogue, the client movie, the V-ego art and the rest are named the same way. A match is saved under that name (`wal-mart-music-downloads_at_131072.cct`).
- **Old addresses** of the game's sites, in browser history, caches and favourites. These show which files existed even when the files themselves are gone.
- **Every file is checked against the project's archive** by SHA-1 (`src/known.c`), so the report can say what is new. Files the archive already has are listed with where and when they were saved, but not copied.

The game's sites were `cokemusic.com`, `cokestudios.com`, `mycoke.com` and `studiocom.com` (Studiocom built the second version). The tool also counts the music mixer at `audiomixer.oddcast.com` and the MyCoke Blogs at `blogs.icoke.com`.

## What the project is hoping for

The report flags these as **WANTED**: pieces the project knows existed but has no copy of. Every new file from the game is worth sending, even when it isn't on this list.

**The game's older clients.** The archive's oldest client is from March 2006. Three earlier editions are entirely gone:
- **Version 1** (June 2002 – December 2004), with the bartenders, the original catalogue and the disco light modes.
- **The 2.0 beta** (2003–2004).
- **The Shark Tale edition**, which ran on its own server from September to December 2004.

Any new Shockwave file dated 2002–2004 is flagged as one of these.

**Rooms**
- **The Coming Soon Theatre**: the Batman Begins lobby with the poster dispenser.
- **Wal-Mart Music Downloads**: only screenshots survive.
- **The Shark Tale underwater studio background**, the uncompressed one that filled the black space around private studios.
- **Ray's Rooftop Party's navigator icon**, the one icon the archive's interface cast is missing.
- **An older public room list** (`PublicStudios_English.xml`). Every copy in the archive is from 2006–2007: those have no descriptions for mycoke Arena or Club Cherry, no Wal-Mart at all, and nothing from the v1 era.

**Games**
- **Whale Wash Builder**, the first Shark Tale game. Nothing at all from it is known to survive.
- **Whale Wash**, the Shark Tale game that was later reskinned into Recycler.
- **The original Pop Quiz** (2002, by GamesThatWork): its questions and art. Only the later Sports Challenge version survives.
- **Uncover the Music's tile faces and their sounds** (`content/cards/00.swf`–`28.swf`).
- **Sports Challenge's announcer** (`resources/audio/STREAM…_NN.swf`): 62 of its 65 lines are missing.
- **Coaster's winning cutscene** (`resources/CoasterAnim.swf`) — the animation of the coaster finishing its loop when a team wins. A screen recording of it survives in the game's animator's reel; the Flash movie itself doesn't.

(The later MyCoke website and mobile games — Champrun, Celebra El Gol, Desktop Kicks and Hoops — are not on this list: they ran on their own sites, did not use a V-ego and did not award Coke Studios decibels or furni, so they are not part of Coke Studios.)

**Music**
- **Saved mixes and the Music Mixer's server data.** The mixer interface itself survives (`am_habbo_v2.swf`, the OddCast AudioMixer Flash app), but OddCast's `audiomixer.oddcast.com` backend — where mixes were saved and preset mixes served — is dead, so any mix someone saved, or the preset mixes, would only turn up in a cache.

The **Infinite Jukebox** is not on this list: it was a music *player* that streamed third-party songs from FreshTracksMusic (later myTracks), an outside service — the furni, catalogue row and in-game interface all survive, and the songs were never Coca-Cola's files to lose.

**Everything else**
- **Some V-ego styles' own colour palettes** (the `AvatarConfig` XML). The five main palettes survive — the website's `avatar_engine.swf` carries a copy — but a few hairstyles, shirts, pants and shoes had palettes of their own that only the server's file held.
- **The Promo Poster's content** (`content/dynamicPosterContent.xml`).
- **MyCoke screensavers** and other downloads. The tool searches installed screensavers (`.scr`).
- **Screenshots**, especially from 2002–2004 and of the Rocawear clothes, of which almost no pictures survive.

## Looking by hand

If you would rather look yourself, these are the places that matter most. On Windows, turn on hidden files first.

| System | Where |
|---|---|
| Windows 95/98/ME | `C:\WINDOWS\Temporary Internet Files\` |
| Windows 2000/XP | `C:\Documents and Settings\<you>\Local Settings\Temporary Internet Files\Content.IE5\` |
| Windows Vista/7 | `C:\Users\<you>\AppData\Local\Microsoft\Windows\Temporary Internet Files\Content.IE5\` (and `Low\Content.IE5`) |
| Firefox (Windows) | `…\Local Settings\Application Data\Mozilla\Firefox\Profiles\*\Cache\` (XP) or `AppData\Local\Mozilla\Firefox\Profiles\*\Cache\` |
| Mac | `~/Library/Caches/` (Safari, Firefox), `~/Library/Preferences/Explorer/` (Internet Explorer 5) |
| Upgraded PCs | `C:\Windows.old\` holds the previous Windows' user folders |

Look for files ending in `.cct`, `.dcr`, `.swf`, and anything with `[1]` in its name. Backups, old CDs and "My Documents" copied from an old PC count too.

## Building it

**Windows:** run `build.bat` with Visual Studio's C++ tools installed. It makes one `build\CokeCacheFinder.exe` that runs on every Windows from 95 to 11. To get there, the .exe links no C runtime, imports only functions Windows 95 already had (anything newer is looked up while running), is compiled without SSE, and has its header patched to Windows 4.0. `tools\check-imports.ps1` checks the import rule, and the CI runs it on every push.

**Mac and Linux:** `make` (or `make universal` for a Mac binary for both Intel and Apple Silicon, `make static` for a Linux binary that runs anywhere).

Pushing a tag such as `v1.0` makes GitHub build all three and publish them as a release (`.github/workflows/release.yml`).

## Keeping the archive list current

When the archive gains files, regenerate the fingerprints so the tool stops calling them new:

```
CokeCacheFinder --make-known src\known.c <archive folder> [<archive folder> ...]
```

It fingerprints every Shockwave, Flash, text and XML file under those folders. Rebuild afterwards. `--classify <folder>` prints how each file there would be judged, which is how the word lists in `src/coke.c` were checked against the archive.

## Layout

| File | What it does |
|---|---|
| `src/main.c` | The questions, the search order, the summary |
| `src/scan.c` | Walking drives, reading first bytes, searching inside files and raw disks |
| `src/formats.c` | Recognising Shockwave and Flash files, and finding where one ends |
| `src/coke.c` | Everything specific to Coke Studios: its words, its sites, the wanted list |
| `src/findings.c` | Deciding about each find, saving it, removing names from paths |
| `src/iecache.c`, `src/ffcache.c` | Internet Explorer's `index.dat`, Firefox's `_CACHE_MAP_` |
| `src/report.c` | `report.txt`, the zip, and `--make-known` |
| `src/inflate.c`, `src/hash.c`, `src/zip.c` | zlib decoding, SHA-1 and CRC-32, writing zips |
| `src/plat_win32.c`, `src/plat_posix.c` | The operating system: Windows 95–11 without a C runtime, and macOS/Linux |

## License

MIT, see [LICENSE](LICENSE). This is a fan preservation project. It is not affiliated with or endorsed by The Coca-Cola Company. "Coke", "Coca-Cola" and "Coke Studios" are its trademarks.
