# Coke Music / Coke Studios — faithful recreation in Go

The goal is a **playable 1:1 recreation of the historical game**, including its website, V-egos, rooms, music, economy and social life. The gameplay client is Go/Ebitengine compiled to WebAssembly; the authoritative server is Go.

**Current status:** Tasks 1 through 9 of `docs/RECREATION-PLAN.md` are implemented and verified. The Go WebAssembly client is the sole player path, authentication uses real salted PBKDF2 credentials, all hardcoded fake users have been eliminated, and multiplayer rooms, catalogue economy, music/performances/voting, trading, moderation, and minigames (Sumo, Uncover the Music, Pop Quiz) are powered by the authoritative Go server. Detailed verification limits and known gaps are tracked in [docs/PARITY.md](docs/PARITY.md) and [docs/verification/](docs/verification/). A compiled binary or passing test suite does not establish complete historical parity; reference visual and audio comparisons continue to be documented.

## Start here

- [Project specification](docs/BIBLE.md): what 1:1 means, Go architecture, evidence rules and acceptance.
- [Implementation plan](docs/RECREATION-PLAN.md): ordered, file-specific changes and player milestones.
- [Progress review](docs/PROGRESS-REVIEW.md): verified findings, useful work to keep, and testing limits.
- [Parity register](docs/PARITY.md): actual status and evidence needed for completion.
- [Agent instructions](AGENTS.md): durable direction for future work.

The connected player journey (registration -> room entry -> chat/movement -> compose/burn/perform/vote -> catalogue purchase/placement/trade -> minigames) is fully operational on the authoritative Go server. Ongoing work focuses on pixel-exact sprite alignments and audio cue replays against original Macromedia Director CCT captures.

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

## Launching the game

The game runs as a single unified Go authoritative server serving both the website/portal and the WebAssembly client on port `24902` (or `$PORT`).

### Debian 12 / Linux (Primary Target)
```bash
# 1. Build client and server (requires Go 1.22+)
./tools/build-go.sh

# 2. Launch authoritative server in background
./start.sh

# Or run directly in foreground
./tools/bin/coke-server-linux -addr 0.0.0.0:24902 -root ./sources/gocoke/web
```
To stop the server: `./stop.sh`.

### Windows (Compatibility)
Double-click `Start-Local.cmd` or run:
```powershell
.\Start-Local.ps1
```
To stop the server: `Stop-Local.cmd` or `.\Stop-Local.ps1`.

### Docker / Container
```bash
docker compose up -d --build
```

### URLs
- **Game Portal & Website:** [http://127.0.0.1:24902/](http://127.0.0.1:24902/)
- **Go WebAssembly Client:** [http://127.0.0.1:24902/game/](http://127.0.0.1:24902/game/)
- **Health Endpoint:** [http://127.0.0.1:24902/health](http://127.0.0.1:24902/health)

## Running the test suite

From `sources/gocoke`:
```bash
go test ./...
```
All server units (authentication, room state, collision, economy, possessions, music, trade, social, and minigames) and client game rules are tested.

## Scope and completion

Pin one historical release at a time; preserve V1, Beta, V2 and promotions as separate evidence/content profiles. Match the original controls, artwork, screen structure, sounds, timing and rules. Unknown details remain visible gaps rather than invented mechanics.

A completed feature must work in the actual Go player journey, update authoritative shared state, persist where appropriate, handle relevant failures, and match a recorded historical reference.

The previous README and Bible are preserved under [docs/history](docs/history). They contain superseded architecture and completion assertions and must not direct new implementation.
