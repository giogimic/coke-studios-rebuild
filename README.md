# Coke Music / Coke Studios — faithful recreation in Go

The goal is a **playable 1:1 recreation of the historical game**, including its website, V-egos, rooms, music, economy and social life. The gameplay client is Go/Ebitengine compiled to WebAssembly; the authoritative server is Go.

**Current status: incomplete recreation.** There is substantial recovered source/assets and useful Go client/server code. The current launcher still defaults to a JavaScript client, and parts of the server use demo identity/economy behavior. The 2026-10-24 review identifies these defects and sets the repair order. A working endpoint or a compiled binary does not establish historical parity.

## Start here

- [Project specification](docs/BIBLE.md): what 1:1 means, Go architecture, evidence rules and acceptance.
- [Implementation plan](docs/RECREATION-PLAN.md): ordered, file-specific changes and player milestones.
- [Progress review](docs/PROGRESS-REVIEW.md): verified findings, useful work to keep, and testing limits.
- [Parity register](docs/PARITY.md): actual status and evidence needed for completion.
- [Agent instructions](AGENTS.md): durable direction for future work.

The next milestone is a reference-matched entrance into the Go world with two separate players, real identity, synchronized room activity and persistent state. The next connected loop is compose -> save -> burn -> perform -> vote/reward -> buy/decorate.

## Working code and reference material

| Location | Role |
|---|---|
| `sources/gocoke/cmd/client`, `internal/client` | Go gameplay implementation to continue and repair |
| `sources/gocoke/cmd/server` | Local Go server, protocol handlers and SQLite/MariaDB store |
| `sources/gocoke/web` | WASM shell and recovered game assets |
| `tools/portal` | Existing website/launcher requiring fidelity and Go-entry correction |
| `sources/original-client-source` | Recovered original scripts for historical behavior research |
| `sources/cokephase`, `music-mixer`, other collected projects | Reference implementations and assets, not alternate production engines |
| `archives`, `reference-source` | Preserved releases and earlier source snapshots |
| `docs/SOURCE-MANIFEST.json`, `DOWNLOAD-RESULTS.json` | Acquisition/provenance records; these do not measure game completion |

Keep licenses and upstream attribution. [CREDITS.md](docs/CREDITS.md) and [RESEARCH.md](docs/RESEARCH.md) provide leads and provenance. Acquisition records and older research claims require their own checks before being used as release guarantees.

## Inspecting the current build

Existing Windows entry: `Start-Local.cmd`. Existing Linux entry: `./start.sh`. These still start the legacy Node-managed environment; the plan replaces that as the standard runtime after the Go player path is validated.

The existing portal URL is [http://127.0.0.1:24902/](http://127.0.0.1:24902/); the Go shell route is [http://127.0.0.1:24902/game/](http://127.0.0.1:24902/game/). These are current development entry points, not a claim that a faithful end-to-end game is ready. The portal's default Phase route is a known defect.

Run the existing Go server checks from `sources/gocoke`:

```powershell
go test ./cmd/server -count=1
```

This suite passed during the review. Full browser gameplay, two-player persistence and historical visual/audio parity still require the checks in the plan.

## Scope and completion

Pin one historical release at a time; preserve V1, Beta, V2 and promotions as separate evidence/content profiles. Match the original controls, artwork, screen structure, sounds, timing and rules. Unknown details remain visible gaps rather than invented mechanics.

A completed feature must work in the actual Go player journey, update authoritative shared state, persist where appropriate, handle relevant failures, and match a recorded historical reference.

The previous README and Bible are preserved under [docs/history](docs/history). They contain superseded architecture and completion assertions and must not direct new implementation.
