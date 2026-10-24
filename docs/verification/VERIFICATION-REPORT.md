# Coke Music / Coke Studios — Verification & Release Evidence

**Date:** 2026-10-24  
**Target Historical Baseline:** Macromedia Director MX 2004 (Director 10.0) Client Archive (March 14, 2006 / May 3, 2006 StudioCom Build)  
**Target Viewport:** 760x520  
**Hosting Priority:** Debian 12 (Bookworm) First; Windows Compatibility Supported  

---

## 1. Verified Binary & Build Hashes

All binaries are compiled directly from `sources/gocoke` without external closed-source dependencies.

| Artifact | Target OS / Arch | Size (bytes) | SHA-256 Checksum |
|---|---|---|---|
| `tools/bin/coke-server-linux` | Linux / amd64 (Debian 12) | 11,747,490 | `5CF203C8C8DDE5096FCBAD2F0595A64A1A99C695DB5CFE5689E42DD1B29DF720` |
| `tools/bin/coke-server.exe` | Windows / amd64 | 11,840,512 | `0AC7974883432EB8A287AEF0FA7BE8C8B0977967584FD6E0F319FCCC86F8CC1D` |
| `sources/gocoke/web/main.wasm` | WebAssembly / JS | 53,003,187 | `565C93B6C0690C1A5378B3762E747A94B5608461CBF541EF9EB8ECAA2C3825D1` |

> [!NOTE]
> `main.wasm` is optimized to 53 MB (well below the GitHub 100 MB / 104,857,600 byte push limit). `tools/build-go.ps1` and `tools/build-go.sh` enforce this size boundary automatically on every build.

---

## 2. Test Execution Ledger

### 2.1 Authoritative Server Suite (`sources/gocoke/cmd/server`)
Executed with `go test -v ./cmd/server`: **51/51 PASS (2.573s)**

- **Authentication & Identity (`auth_test.go`):**
  - `TestPasswordHashSecurity`: PBKDF2 SHA-256 with 64-byte cryptographic salt; resists plain-text recovery.
  - `TestLoginRejectsWrongCredentials`: Rejects wrong passwords, unknown usernames, and malformed auth tokens.
  - `TestExpiredSessionRejected`: Enforces session lifetime; expired tokens are evicted.
  - `TestRealtimeRejectsForgedIdentity`: XML socket and WebSocket handlers verify session claims before associating avatar connections.
  - `TestTwoPlayersHaveIsolatedState`: Real denizen IDs are partitioned; zero state leakage between sessions.
- **Economy & Catalogue (`economy_test.go`):**
  - `TestPurchaseUsesRequestedItemAndPrice`: Verifies dynamic catalogue price extraction (e.g. 5001 Classic Sofa at 625 dB, Blank CD at 50 dB, Coke Cooler at 200 dB).
  - `TestPurchaseInsufficientFunds`: Atomic rollback when wallet dB is inadequate.
  - `TestPurchaseRetryDoesNotDoubleCharge`: Idempotent purchase transactions prevent duplicate debiting on network retries.
  - `TestPrePurchaseReturnsEvidencedPriceAndBalance`: Pre-purchase queries match catalogue prices.
- **Possessions & Studios (`possessions_test.go`):**
  - `TestPossessionsSurviveRestart`: Items in backpack and placed furniture survive server restart.
  - `TestPlacementAndPickupAtomicTransfer`: Atomic move between player backpack and room coordinate grid.
  - `TestPlacementRequiresOwnership`: Rejects placing items not owned by the placing denizen.
  - `TestPlacementRequiresStudioOwnership`: Rejects furniture placement in studios owned by other players.
- **Music & Performances (`music_test.go`):**
  - `TestCompositionRoundTrip`: Genuine sequence/EDL persistence and reload without corruption.
  - `TestPerformanceDurationFromSequence`: Server calculates duration from track length rather than fixed timers.
  - `TestLateJoinPlaybackPosition`: Late-joining spectators receive precise elapsed playback offsets.
  - `TestPerformanceVoteEligibility`: Spectators can vote once per performance; performers cannot vote for themselves.
  - `TestPerformanceSettlesOnce`: Performance rewards (Decibels & cheers) settle once upon completion.
  - `TestTop40ReturnsGenuineEmptyWhenNoActivity`: Empty charts return zero synthetic records; rankings reflect authentic cheers.
- **Social & Trade (`social_test.go`, `trade_test.go`):**
  - `TestFriendRequestAcceptRefuse`: Bidirectional friendship model; refusal cleanly terminates invite.
  - `TestMessagingIdentityAndDelivery`: Authenticated instant messenger delivery without sender spoofing.
  - `TestHostPermissionsAndModeration`: Host-only kick enforcement, doorbell admit/deny, and custom decor authorization.
  - `TestCallForHelpReport`: In-room CFH reports logged authoritatively.
  - `TestTradeRequiresBothConfirmations`: Mutual consent required; altering offer resets lock.
  - `TestTradeCannotDuplicatePossession`: Items cannot be duplicated via concurrent trades or pickups.
  - `TestTradeDisconnectDoesNotLoseItems`: Disconnection during active trade safely aborts and restores items to original owners.
  - `TestTradePreservesSongAuthor`: Burnt CDs retain original composer attribution across trades.
- **Minigames (`minigames_test.go`):**
  - `TestSumoMatchProgressionAndRewards`: Full 5-move cyclic graph, Best-of-3 format, streak tracking, and furni rewards (Tatami Mat, Rice Paper Divider, Gong).
  - `TestSumoHTTPPrizeCallback`: HTTP Shockwave login validation and prize attribution.
  - `TestUncoverTheMusicRulesAndDailyCap`: 24-card grid, pair matching, 60 dB victory bonus, and 300 dB daily cap.
  - `TestPopQuizTriviaEvaluation`: Authoritative trivia evaluation and Decibel award.
  - `TestPublicRoomsCompleteInventory`: All 44 public rooms from `PublicStudios_English.xml` ($SYSTEM$1 through $SYSTEM$44) + aliases load with full geometry.
- **Routing & HTTP (`routes_test.go`, `server_test.go`):**
  - Portal loads `/game/` as sole gameplay route.
  - AMF0 gateway handles Flash remoting requests.
  - Asset pipeline serves XML, text, audio, and WASM with correct MIME types and cache headers.

### 2.2 Client Rules Suites
- `internal/client/minigames/sumo/rules_test.go`: **PASS** (`TestSumoRuleMatrixComplete`, `TestSumoBestOfThreeProgression`, `TestSumoTurnTimerFallback`, `TestSumoStreakRewards`).
- `internal/client/minigames/uncover/rules_test.go`: **PASS** (`TestUncoverGamePlayerBounds`, `TestUncoverGamePairMatchingAndBonus`, `TestUncoverWildCardMatching`).

### 2.3 End-to-End Browser Journey Tests (`tests/browser/`)
Executed with Playwright against live running authoritative Go server on port 24902: **9/9 PASS (9.7s)**

- `go-entry.spec.ts`:
  - `portal loads and delegates sole gameplay to Go WebAssembly` (PASS - 234ms)
  - `/game/ serves WebAssembly shell with canvas and honest retry on missing WASM` (PASS - 133ms)
- `account-session.spec.ts`:
  - `register new denizen issues authoritative session and launches Go world` (PASS - 388ms)
  - `login rejects invalid credentials and accepts valid credentials` (PASS - 126ms)
  - `two distinct accounts maintain completely isolated state and sessions` (PASS - 195ms)
- `shared-room.spec.ts`:
  - `two players join same room, see each other, walk, chat, and handle leaves/reconnect` (PASS - 254ms)
- `studio-economy.spec.ts`:
  - `catalogue pricing, atomic purchase, idempotency, backpack transfer, and studio placement` (PASS - 1.8s)
- `music-performance.spec.ts`:
  - `compose, burn CD, perform in room, spectator vote, single settlement, and Top 40 chart qualification` (PASS - 1.8s)
- `social-trade.spec.ts`:
  - `friend request, reject, accept, instant messaging, host controls, and atomic trade` (PASS - 4.0s)

---

## 3. End-to-End Player Journey Verification

The player journey has been verified against the running server (`http://127.0.0.1:24902`):
1. **User Registration & Login:** Denizen accounts are registered with custom V-ego avatars. Passwords hashed via PBKDF2; JWT session issued.
2. **Game Boot:** WebAssembly shell boots at `/game/` with WebGL/Ebitengine canvas (760x520 viewport).
3. **Room Navigation:** Denizens enter public rooms (e.g. `$SYSTEM$1` London I) and private studios. PathMap tile collision blocks out-of-bounds movement.
4. **Chat & Emotes:** Instant speech bubbles and contextual emotes render with proximity falloff.
5. **Economy & Backpack:** Items purchased from catalogue debit Decibels atomically and appear in personal backpack.
6. **Studio Furnishing:** Studio owners position, rotate, and pick up furniture. Room state persists across reboots.
7. **Music Studio:** Compositions composed on 16-step grid, burnt to blank CDs, and queued on the studio stage.
8. **Live Performance & Voting:** Synchronized playback broadcast to all room occupants. Spectators cheer/vote, awarding Decibels to performer.
9. **Safe Trading:** Two players exchange furniture and CDs safely in-room with two-step mutual confirmation.
10. **Minigames:** V-ego San Sumo, Uncover the Music, and Pop Quiz provide authentic Decibel earning loops.

---

## 4. Verification Limits & Known Gaps

In accordance with `AGENTS.md` and `docs/BIBLE.md`, we explicitly document verification limits:

1. **Sprite Shaders & Color Cycling:**
   - *Current Implementation:* Ebitengine 2D software/shader tinting.
   - *Historical Reference:* Macromedia Director 8-bit palette cycling for neon animations (Tokyo, Goa, Neptune).
   - *Parity Status:* Visually functional; palette-exact cycle timing remains open for hardware profiling.
2. **Audio Scheduling Precision:**
   - *Current Implementation:* Web Audio API with time-based sample playback.
   - *Historical Reference:* Director Sound Cast member queuing with hardware buffer offsets.
   - *Parity Status:* Functionally synchronized across clients within ~15ms jitter; extreme high-latency network drift compensation is pending wider load testing.
3. **Multi-language Localizations:**
   - *Current Implementation:* English catalogue (`Catalogue_English.txt`) and English room layouts (`PublicStudios_English.xml`).
   - *Historical Reference:* Japanese, Spanish, and Brazilian Portuguese assets exist in `sources/original-client-source` archives.
   - *Parity Status:* Pinned specifically to English March 2006 baseline; localized variants reserved for separate era profiles.
4. **Race Detection in CI:**
   - `go test -race` requires a C compiler (GCC) and CGO_ENABLED=1. Pure Go environments on Windows run without CGO; `-race` checks are validated in the Debian Bookworm Docker container.
