# SCHEMA — Rail A JSON output

Rail A (`extract/extract_room.py`) writes one JSON file per room:

```json
{
  "room": "<room_name>",
  "source_cct": "http://127.0.0.1:8765/publicrooms/<room>.cct",
  "cast_lib_loaded_into": 19,
  "member_count": 85,
  "members": [
    {
      "cast_lib": 19,
      "cast_member": 122,
      "name": "<member_name>",
      "member_type": "<bitmap|script|text|sound|...>"
    }
  ],
  "text_bodies": {
    "<member_name>": "<text content>",
    "MapXml": "<...>",
    "SceneXml": "<...>",
    "EntryXml": "<...>"
  },
  "scripts": {
    "scripts": [
      {
        "cast_lib": 19,
        "cast_member": 46,
        "name": "<script_name>",
        "script_type": "score",
        "handlers": ["new", "init", "exitFrame"],
        "handler_sources": {
          "new": "<decompiled lingo source>",
          "init": "<decompiled lingo source>",
          "exitFrame": "<decompiled lingo source>"
        }
      }
    ],
    "total_count": 12,
    "handler_source_count": 94
  }
}
```

## Field reference

| Field | Type | Notes |
|---|---|---|
| `room` | string | Internal room name from the cct (`<name>` in the Director header), not necessarily the user-visible room id. |
| `source_cct` | string | URL the extractor loaded the cct from (CORS server). |
| `cast_lib_loaded_into` | int | Always `19` — the cast lib slot the cct is hot-swapped into for extraction. Member numbers in `members[].cast_lib` will match. |
| `member_count` | int | Total number of cast members in the cct. |
| `members[]` | array | Every cast member, in cast number order. |
| `members[].cast_lib` | int | Always equal to `cast_lib_loaded_into`. |
| `members[].cast_member` | int | The cast member number (1-indexed). |
| `members[].name` | string | The member's name as set in Director, possibly empty. |
| `members[].member_type` | string | Director member type: `bitmap`, `script`, `text`, `sound`, `field`, `palette`, `filmLoop`, etc. |
| `text_bodies` | object | Keyed by member name; value is the text content of that text/field/script member. Director-internal names like `MapXml`, `SceneXml`, `EntryXml` carry the structured per-room layout / scene / entry metadata. |
| `scripts` | object | `scripts[]` lists every script-type member, sorted by `(name, cast_member)` for stable diffs. `total_count` is the array length; `handler_source_count` is the number of decompiled handler bodies (sanity checks). |
| `scripts.scripts[].script_type` | string | Director script type: `score` (behavior), `movie`, `parent`. |
| `scripts.scripts[].handlers` | array | Handler (method) names defined on the script. |
| `scripts.scripts[].handler_sources` | object | Keyed by handler name; value is the decompiled Lingo source for that handler, or `<DECOMPILE_FAILED: …>` if decompilation failed (one bad handler never aborts the run). |

## Rail B outputs

PNGs for bitmap members are produced by Rail B (the Rust dumpers in
the fork), not Rail A. The Rust dumpers cross-reference Rail A's
`members[]` when `ROOM_JSON_DIR` is set to know which publicroom cast members
to render. The currently pinned development backend writes publicroom output:

- `<OUTPUT_ROOT>/rooms/<room_id>/<member_name>.png`
- `<OUTPUT_ROOT>/rooms/<room_id>/_members.json` — sidecar with
  per-member metadata (`name`, cast reference, registration point, decoded and
  original bit depth, alpha use, palette reference, width, and height).

The `_members.json` schema mirrors the dumper source — see
`vm-rust/tests/dump_cct_bitmaps.rs` in the fork for the canonical
shape.

Other dumper outputs are:

| Dumper | Output |
|---|---|
| `dump_studio_bitmaps` | `<OUTPUT_ROOT>/assets/rooms/` backgrounds, per-studio members, `_studios.json`, and `_studio_members.json` |
| `dump_studio_palette_variants` | `<OUTPUT_ROOT>/assets/rooms/_studio_palette_variants/` PNGs and `_studio_palette_variants.json` |
| `dump_engine_bitmaps` | `<OUTPUT_ROOT>/ui/` PNGs and `_engine_members.json` |
| `dump_furniture_bitmaps` | `<OUTPUT_ROOT>/furniture/data/` PNGs and `<OUTPUT_ROOT>/furniture/_cc_furniture_members.json` |
| `dump_avatar_bitmaps` | `<OUTPUT_ROOT>/avatars/<cast>/data/` PNGs and `<OUTPUT_ROOT>/avatars/<cast>/_members.json`; optional `people/_utm_comparison.json` |
| `dump_dcr_bitmaps` | `<OUTPUT_ROOT>/games/recycler/` PNGs, `_members.json`, and `sounds/` |
| `dump_dcr_score` | `<OUTPUT_ROOT>/games/recycler/` sprite-channel and behavior JSON |

The hardened backend emits skip
summaries. Full CCT runs write `_skip_summary.json`; filtered CCT runs write
`_skip_summary.filtered.json` so they cannot replace full-run evidence. DCR
summaries with bounded examples. Deterministic ordering and filtered/full scope
separation remain release gates for clean-checkout validation.

## Sound members

WAVs for sound members are also produced by Rail B (currently only
the `.dcr` dumper, `dump_dcr_bitmaps`; the `.cct` dumpers are
bitmap-only). For each named `CastMemberType::Sound` member, the
dumper calls `SoundChunk::to_wav()` to convert the cast bytes to
PCM WAV. Output (using the recycler dump as the example):

- `<OUTPUT_ROOT>/games/recycler/sounds/<member_name>.wav`
- `<OUTPUT_ROOT>/games/recycler/sounds/_sounds.json` — sidecar with
  per-sound metadata, one entry per WAV. Schema:

```json
[
  {
    "name": "<cast member name>",
    "filename": "<safe_name>.wav",
    "castLib": <int>,
    "castMember": <int>,
    "channels": <int>,
    "sampleRate": <Hz>,
    "bitsPerSample": <8 or 16>,
    "sampleCount": <int>,
    "codec": "raw_pcm | ima_adpcm | ...",
    "wavByteLength": <int>
  }
]
```

Cast member names are preserved verbatim in `name`; `filename`
applies the same alphanumeric-only sanitization the dumper uses for
PNG filenames. Unnamed sound members are skipped (they have no
useful identifier for downstream gameplay code to reference). The
canonical schema lives in `vm-rust/tests/dump_dcr_bitmaps.rs` in
the fork.
