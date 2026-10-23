# examples/

Reference consumers of the Rail A JSON schema. These are stdlib-only,
heavily-commented programs designed to be read once and adapted —
not used as-is in a production pipeline.

The canonical schema lives at [`../docs/SCHEMA.md`](../docs/SCHEMA.md).
If a field used here disagrees with that document, the document wins.

## translate-example.py

Reads one `extracted/<room>.json` file produced by
`extract/extract_room.py`, groups cast members by Director member type,
previews the three structured XML text bodies (`MapXml`, `SceneXml`,
`EntryXml`), and — if `--output-root` is passed — resolves each bitmap
member's name to the PNG path that Rail B's `dump_cct_bitmaps` writes
under `<output_root>/rooms/<room>/`. Here `output_root` is exactly the
`OUTPUT_ROOT` passed to `dump_cct_bitmaps`, not a project-specific public or
assets directory. Output is pretty-printed JSON on stdout.

```bash
# Schema walk only — no bitmap path resolution.
python3 examples/translate-example.py path/to/extracted/london.json

# With path resolution — each bitmap entry gains a `bitmap_path` field
# that is either the resolved absolute path or null.
python3 examples/translate-example.py path/to/extracted/london.json \
    --output-root /abs/path/to/output_root
```

The script demonstrates how to walk the schema. It does not parse XML
into tile coordinates, render anything, or implement any project's
specific room shape. Use it as a starting point when wiring the
extracted JSON into your own runtime.
