# Project Research Summary

**Project:** gsd-tele — Go Telegram Bot with Multi-Project Claude Code Integration
**Domain:** Go Telegram bot with concurrent subprocess management, Windows Service deployment
**Researched:** 2026-03-19 (v1.0); updated 2026-03-20 (v1.1 bugfixes synthesized)
**Confidence:** HIGH

## Executive Summary

This project is a Go rewrite of an existing TypeScript Telegram bot, with a significant architectural upgrade: the Go version supports independent Claude Code sessions per Telegram channel (one channel = one project). The research-validated approach uses `gotgbot/v2` for Telegram long-polling, `go-cmd/cmd` for streaming subprocess management, a `sync.RWMutex`-protected `SessionStore` for concurrent session management, and `kardianos/service` for Windows Service lifecycle. State is stored in JSON files with atomic write-rename patterns; no database is needed or desirable.

The v1.1 milestone adds a layer of specificity: two production bugs emerged after v1.0 shipped that require targeted fixes. Channel-type auth failures reject all messages sent in a Telegram channel because `EffectiveSender.Id()` returns the channel's chat ID (a negative int64) for `ChannelPost` updates, and no channel chat ID is in the human user allowlist. Separately, the HTTP client timeout for long-polling (defaulting to 5 seconds from gotgbot's `BaseBotClient.DefaultTimeout`) is shorter than the `GetUpdatesOpts.Timeout` of 10 seconds, causing `context deadline exceeded` errors on every idle polling cycle. Both root causes are verified directly from the gotgbot v2 library source in the local module cache.

The dominant risk in v1.0 infrastructure is concurrency correctness under Windows (process tree orphaning, concurrent map panics, goroutine leaks, JSON file corruption). In v1.1, the dominant risk is regression: the channel auth fix modifies security-critical middleware, and the existing user-ID auth path must not be altered. Both v1.1 fixes are additive and surgical — no new dependencies, no architectural changes, no new production files. The HTTP timeout fix is a single-line change; the auth fix is a branching addition with corresponding new test cases.

## Key Findings

### Recommended Stack

Go 1.26.1 is the correct language choice for this project: native goroutines match the per-project concurrent session model, and single-binary output simplifies Windows Service deployment. The full stack is established from v1.0 with HIGH confidence on all core components. No stack changes are needed for v1.1.

**Core technologies:**
- **Go 1.26.1**: Runtime — latest stable; goroutines are the right concurrency model for N independent Claude sessions; single binary for Windows Service
- **gotgbot/v2 v2.0.0-rc.34**: Telegram Bot API — auto-generated from official spec, type-safe, Bot API 9.4 support; the "rc" tag is misleading — this is the active production branch with 313 importers
- **go-cmd/cmd v1.4.3**: Claude CLI subprocess — thread-safe streaming stdout/stderr, non-blocking async, Windows-compatible, purpose-built for this problem
- **kardianos/service**: Windows Service — de facto standard; bakes install/uninstall/start/stop into the binary itself
- **golang.org/x/time/rate**: Per-channel rate limiting — stdlib extension, token-bucket implementation, zero additional dependencies
- **rs/zerolog**: Structured logging — zero-allocation performance for high-frequency streaming log lines
- **openai/openai-go**: Whisper transcription — official OpenAI SDK since July 2024; prefer over community forks
- **encoding/json + sync.RWMutex**: State persistence — JSON files are sufficient; no database required

**Critical v1.1 API facts (verified from module cache source):**
- `BaseBotClient.DefaultTimeout` is hardcoded at 5 seconds and does not auto-scale with `GetUpdatesOpts.Timeout`
- `RequestOpts.Timeout` nested inside `GetUpdatesOpts` is the correct and targeted override point for polling timeout
- `Sender.IsUser()` (`Chat == nil && User != nil`) is the canonical test for human vs. non-human senders — more robust than `IsChannelPost()` alone because it also covers anonymous admins and linked-channel forwards
- Channel IDs are large negative int64 values; user IDs are positive — they can coexist in one allowlist without collision

**What NOT to use:** go-telegram-bot-api v5 (stale since 2021), SQLite/GORM (explicitly out of scope), Docker (out of scope), any HTTP router (no HTTP endpoints needed), `DefaultRequestOpts` on `BaseBotClient` for polling timeout (applies to all API calls; use per-request override instead).

### Expected Features

The feature set splits into v1.0 launch requirements, v1.1 bugfixes (both are baseline correctness — not optional), and post-validation additions.

**Must have — table stakes (v1.0 and v1.1):**
- Text message routing with streaming response (edit-in-place, throttled at 500ms minimum interval)
- Multi-project channel mapping with dynamic assignment — one channel = one project = one Claude session
- Independent Claude sessions per channel — the isolation guarantee; no context bleed between projects
- Session lifecycle commands: `/new`, `/stop`, `/status`, `/start`, `/resume`
- Per-channel auth (channel ID in allowlist = access; fixes v1.1 auth failure)
- Rate limiting per channel and global across all outgoing Telegram API calls
- Session persistence with atomic JSON writes; `/resume` with inline keyboard picker
- Audit logging
- GSD inline keyboard with all 19 operations
- Contextual action buttons extracted from Claude responses
- Windows Service deployment
- HTTP polling timeout correctly configured (`RequestOpts.Timeout > GetUpdatesOpts.Timeout`) — v1.1 fix
- Auth that does not reject channel post updates — v1.1 fix

**Should have — differentiators (post-validation):**
- Voice transcription via OpenAI Whisper (mobile-first UX)
- Photo analysis with 1-second album buffering
- PDF document processing via pdftotext CLI
- Context window progress bar (parse `context_percent` from stream-json)
- Roadmap parsing in `/gsd` (phase progress display)
- Token usage display in `/status`
- ask_user MCP integration (inline keyboard for Claude clarifying questions)

**Defer to v2+:**
- Video/audio file transcription, archive extraction from documents, `/retry` command
- Polling timeout configurable via env var (hardcoding 30s/35s is sufficient for now)
- Channel posts as a command interface (would require `SetAllowChannel(true)` on message handlers)

**Anti-features to avoid:** native Telegram streaming (15% commission), shared Claude sessions across channels (destroys isolation), webhook mode (requires HTTPS on Windows), Docker deployment (out of scope), global HTTP timeout increase (affects all API calls; use per-poll override only).

### Architecture Approach

The v1.0 architecture follows a layered pipeline with strict responsibility boundaries enforced by Go's `internal/` package system. Handlers never directly touch session internals; sessions communicate with Claude processes via Go channels, not shared state. The v1.1 changes are surgical: three existing files are modified, no new files are added, and no architectural boundaries change.

**Major components:**
1. **gotgbot Updater + Dispatcher** — Long-poll Telegram updates; spawns one goroutine per update; no shared state
2. **Middleware chain** — Auth (group -2), rate limit (group -1), channel-to-project resolver; all run before handlers; v1.1 auth fix is contained entirely within group -2
3. **Handler layer** — One file per update type (text, voice, photo, document, callback, commands); handlers enqueue messages and return immediately
4. **SessionStore** — `sync.RWMutex` + `map[int64]*Session`; each session owns a buffered message queue (capacity 5) and a worker goroutine that serializes Claude queries
5. **Claude subprocess layer** — `go-cmd/cmd` for streaming stdout; NDJSON event parsing; `taskkill /T /F` on Windows for process tree kill
6. **StreamingState** — Per-query ephemeral state tracking Telegram message IDs and throttle timers; discarded after query completes
7. **Persistence layer** — Atomic write-rename for `channel-projects.json` and `session-history.json`; single `StateManager` with embedded mutex
8. **GSD package** — Isolated roadmap parsing, command extraction, inline keyboard builder
9. **Windows Service wrapper** — `svc/service.go` implements `svc.Handler`; wired in last without affecting application logic

**v1.1 modified files (three only):**
- `internal/bot/bot.go` — `Start()`: add `GetUpdatesOpts.RequestOpts{Timeout: 35 * time.Second}` alongside `Timeout: 30`
- `internal/bot/middleware.go` — `authMiddlewareWith`: use `EffectiveSender.IsUser()` to branch user-ID vs. channel-ID auth paths
- `internal/security/validate.go` — `IsAuthorized`: treat negative channel IDs as valid principals in the allowed list

### Critical Pitfalls

**v1.0 infrastructure pitfalls (six must be addressed in Phase 1):**

1. **Windows process tree orphaning** — Use `taskkill /pid <PID> /T /F` for all subprocess kills; `cmd.Process.Kill()` leaves Claude child processes running and accumulating. Gate on `runtime.GOOS == "windows"`.

2. **Concurrent map panic on SessionStore** — Wrap `channelID -> session` map in a struct with `sync.RWMutex` from day one. Run all tests with `-race`. Manifests immediately in multi-session production use.

3. **Goroutine leak from uncleaned subprocess pipes** — Set `WaitDelay` on every `exec.Cmd` (available since Go 1.20); use `CommandContext` with timeout. Without this, killed subprocess pipes leave goroutines blocked indefinitely.

4. **JSON persistence file corruption** — Use atomic write-rename: marshal to temp file, `os.Rename` to target (atomic on Windows NTFS). Direct `os.WriteFile` in a goroutine-concurrent environment will corrupt files on crash.

5. **editMessageText rate limit causing full bot blackout** — Telegram rate limits are per-bot, not per-chat. With N simultaneous streaming sessions, edit rate multiplies by N. Implement a single global API call rate limiter; parse `retry_after` from 429 responses.

6. **Service PATH blindness** — Windows Service account has a stripped PATH. Resolve all external binary paths at startup via explicit env vars (`CLAUDE_CLI_PATH`, etc.); log resolved paths at startup.

**v1.1 bugfix pitfalls:**

7. **Auth regression — private chat users broken by channel fix** — Implement as an additive branch (`if sender.IsUser() ... else ...`); never restructure the existing user-ID check. Run all existing `TestMiddlewareAuth*` tests before merging. Failing existing tests is the hard regression gate.

8. **Non-user sender detection too narrow (`IsChannelPost()` only)** — Use `!sender.IsUser()` as the universal gate. `IsChannelPost()` misses anonymous admins and linked-channel forwards, both of which also have nil `User` fields. `IsUser()` catches all three cases with one check.

9. **HTTP timeout set globally instead of per-poll** — Use `RequestOpts` nested inside `GetUpdatesOpts`, not `DefaultRequestOpts` on `BaseBotClient`. Global timeout affects `sendMessage` and `editMessage`, which should fail fast. Only the polling loop needs the extended timeout.

10. **Operator config gap after auth fix** — After the code fix, operators must add their channel's numeric ID to `TELEGRAM_ALLOWED_USERS` in `.env`. The deployment checklist and `.env.example` must document this explicitly.

## Implications for Roadmap

The build order is constrained by dependencies. Infrastructure must precede features; single-project must precede multi-project; application logic must precede service wrapping. The v1.1 fixes are a discrete pre-phase that gates further feature work.

### Phase 0 (v1.1): Production Bugfixes

**Rationale:** Two live production failures must be resolved before continuing feature development. Both fixes are well-understood with HIGH-confidence root causes. Fixing them first clears the log noise and auth failures that would complicate testing of any new feature work.

**Sub-phase ordering:** HTTP timeout fix first (one line, no regression risk, clears log noise); channel auth fix second (logic change, security-critical, requires new tests).

**Delivers:** Elimination of `context deadline exceeded` log spam; channel messages passing auth; no behavior change for existing private-chat users.

**Addresses pitfalls:** V1.1-1 (channel auth), V1.1-2 (HTTP timeout), V1.1-3 (auth regression), V1.1-4 (sender type detection coverage), V1.1-5 (operator config documentation).

**Avoids:** Restructuring the existing user-ID auth path; increasing `GetUpdatesOpts.Timeout` without a matching `RequestOpts.Timeout`; setting global HTTP timeout on `BaseBotClient`.

**Research flag:** No additional research needed. Both root causes verified directly from gotgbot source. Canonical fix patterns documented in STACK.md and ARCHITECTURE.md.

### Phase 1: Core Infrastructure and Claude Subprocess

**Rationale:** Everything depends on this. Config parsing, atomic JSON persistence, the Claude subprocess wrapper (with correct pipe cleanup and Windows process tree kill), the SessionStore (with mutex), and session state machine are all load-bearing. No feature works without them being correct.

**Delivers:** A bot that can start, send a message to Claude, stream the response back to one Telegram channel, and stop cleanly. Windows Service can run interactively with PATH resolution logged.

**Addresses:** Text message handler, streaming response, session persistence, `/start`/`/new`/`/stop`/`/status` commands, per-channel auth, rate limiting, audit logging.

**Avoids:** Pitfalls 1 (process tree), 2 (concurrent map), 3 (goroutine leak), 4 (JSON corruption), 6 (context limit detection), 7 (PATH blindness).

**Research flag:** Standard patterns — well-documented Go subprocess and concurrency patterns. Architecture file covers everything needed. Skip research-phase.

### Phase 2: Multi-Project Session Management

**Rationale:** Multi-project isolation is the core value proposition. It builds directly on Phase 1's SessionStore. The Telegram rate limit risk from concurrent streaming sessions (Pitfall 5) must be addressed before shipping.

**Delivers:** Multiple Telegram channels routing to independent Claude sessions with no context bleed. Channel registration flow (unknown channel prompts for project path link). GSD inline keyboard with all 19 operations and contextual action button extraction.

**Addresses:** Multi-project channel mapping, dynamic project-channel assignment, independent sessions per channel, GSD workflow integration, contextual action buttons, `/resume` with picker, roadmap parsing in `/gsd`.

**Avoids:** Pitfall 5 (rate limit flood across sessions); shared-session anti-pattern; global-user-allowlist anti-pattern.

**Research flag:** GSD command extraction and keyboard builder logic may benefit from a targeted research pass on the GSD framework's current command structure and `/gsd:*` pattern parsing.

### Phase 3: Media Handlers and Windows Service Deployment

**Rationale:** Media handlers are independent of each other and all build on the core text handler pipeline from Phase 1. Windows Service wrapping is last because it requires the full bot to be functional for meaningful lifecycle testing.

**Delivers:** Voice transcription (OpenAI Whisper), photo analysis with album buffering, PDF processing (pdftotext), Windows Service install/uninstall/start/stop via NSSM with correct PATH configuration.

**Addresses:** Voice messages, photo analysis, PDF document processing, Windows Service deployment, long-polling offset handling on service stop/restart.

**Avoids:** Pitfall 8 (offset reset on service restart); pdftotext PATH issues; NSSM environment variable configuration gaps.

**Research flag:** Windows Service deployment via NSSM with explicit PATH configuration may need a targeted research pass on NSSM environment variable configuration for service accounts.

### Phase 4: Enhanced Features and Polish

**Rationale:** These features add value but have no architectural dependencies that require them before Phase 3. They can be added after the core bot is validated in production.

**Delivers:** Context window progress bar, token usage in `/status`, ask_user MCP integration, `/retry` command, adaptive streaming throttle based on active session count.

**Avoids:** ask_user JSON file polling in a hot loop (use in-memory channel instead).

**Research flag:** ask_user MCP integration likely needs a research-phase pass — MCP server configuration and the JSON file polling protocol are not well-documented in standard sources.

### Phase Ordering Rationale

- **Bugfixes before features:** Live production failures block confidence in any feature work built on top
- **Infrastructure before features:** Concurrent map panic and goroutine leak pitfalls crash production; correct infrastructure is not optional
- **Single-project before multi-project:** Verifying the core Claude streaming pipeline with one session is far easier to debug than building multi-project isolation simultaneously
- **Application logic before service wrapper:** The Windows Service wrapping is a thin lifecycle layer; testing it requires the full bot working first
- **Media handlers are independent:** Voice, photo, and PDF handlers do not depend on each other and can be sequenced within Phase 3 in any order

### Research Flags

Needs `/gsd:research-phase` during planning:
- **Phase 2 (GSD integration):** Claude response parsing for `/gsd:*` command extraction and contextual button rendering; GSD command structure needs verification against current framework state
- **Phase 3 (Windows Service PATH):** NSSM environment variable configuration for service accounts running user-installed tools; configuration details are sparse in official NSSM docs
- **Phase 4 (ask_user MCP):** The ask_user JSON file protocol, MCP server configuration format, and lifecycle under the Go bot model are not covered by standard documentation

Standard patterns (skip research-phase):
- **Phase 0 (v1.1 bugfixes):** Root causes verified from gotgbot source. Fix patterns fully specified in STACK.md and ARCHITECTURE.md.
- **Phase 1 (Core infrastructure):** Go subprocess management, `sync.RWMutex`, `bufio.Scanner`, atomic file writes — all well-documented stdlib patterns
- **Phase 3 (Media handlers):** OpenAI Whisper API, pdftotext CLI, photo album buffering — standard patterns with high-quality official documentation

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All core libraries verified via official sources (pkg.go.dev, GitHub, module cache). v1.1 API details verified directly from gotgbot source files. |
| Features | HIGH | v1.0 functional spec derived from existing TypeScript codebase. v1.1 bug behaviors observed in production and root-caused against official Telegram Bot API spec and gotgbot source. |
| Architecture | HIGH | v1.0 derived from TypeScript codebase as functional spec. v1.1 changes verified by reading gotgbot library source in local module cache and existing bot source files directly. |
| Pitfalls | HIGH | Most pitfalls verified against official Go issue tracker, library docs, and existing codebase. v1.1 pitfalls derived from live production failures and source verification. |

**Overall confidence:** HIGH

### Gaps to Address

- **Operator config documentation:** The v1.1 channel auth fix requires operators to add their channel's numeric ID to `TELEGRAM_ALLOWED_USERS`. This is a deployment gap, not a code gap. The `.env.example` file and task plan for the auth fix must document this explicitly.
- **AllowChannel policy decision:** FEATURES.md identifies whether channel posts should be routed as commands (vs. filtered out) as a product decision. The recommended policy (filter out `ChannelPost` updates; the bot does not receive channel posts as commands) should be confirmed before Phase 0 begins.
- **kardianos/service Windows 11 edge cases:** 137 open issues; core functionality is stable (MEDIUM confidence on edge cases). Relevant for Phase 3; test service install/uninstall/start/stop on the actual target machine early.
- **NSSM environment variable configuration:** How NSSM passes environment to services and whether `%USERPROFILE%` paths resolve for the service account needs hands-on verification in Phase 3.
- **GSD framework current command structure:** The 19 GSD commands referenced in FEATURES.md are based on the framework state as of research date. The keyboard builder must match the current framework; read `.claude/commands/gsd/` during Phase 2 planning.
- **ask_user MCP JSON file protocol:** Exact format, file path conventions, and lifecycle are not documented in public sources. Inspect the existing TypeScript `src/handlers/callback.ts` as the functional spec during Phase 4 planning.

## Sources

### Primary (HIGH confidence)
- Existing TypeScript codebase (`src/session.ts`, `src/handlers/`, `src/config.ts`) — functional specification for Go rewrite
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/request.go` — `DefaultTimeout = 5s`, `BaseBotClient`, `getTimeoutContext` verified (v1.1)
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/sender.go` — `GetSender`, `Id()`, `IsUser()`, `IsChannelPost()` methods verified (v1.1)
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/ext/context.go` — `NewContext` switch, sender population for `ChannelPost` case verified (v1.1)
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/ext/updater.go` — `PollingOpts` timeout documentation comment verified (v1.1)
- `internal/bot/middleware.go`, `internal/bot/bot.go`, `internal/security/validate.go` — existing source read directly (v1.1)
- [gotgbot middleware sample on GitHub v2 branch](https://github.com/PaulSonOfLars/gotgbot/blob/v2/samples/middlewareBot/main.go) — canonical `Timeout: 9` + `RequestOpts: 10s` pattern confirmed
- [Telegram Bot API — Message object](https://core.telegram.org/bots/api#message) — `from` field documented as optional for channel posts
- [gotgbot v2 pkg.go.dev](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2) — updater/dispatcher model, Bot API 9.4 support, published Feb 17, 2026
- [Go downloads page](https://go.dev/dl/) — go1.26.1 confirmed latest stable as of March 2026
- [go-cmd/cmd GitHub](https://github.com/go-cmd/cmd) — v1.4.3, thread-safe subprocess streaming, 100% test coverage, Windows support
- [openai/openai-go GitHub](https://github.com/openai/openai-go) — official SDK since July 2024
- [kardianos/service GitHub](https://github.com/kardianos/service) — Windows XP+ service support, de facto standard
- [golang.org/x/time/rate pkg.go.dev](https://pkg.go.dev/golang.org/x/time/rate) — token bucket rate limiter
- [Go race detector](https://go.dev/doc/articles/race_detector) — concurrent map detection tooling

### Secondary (MEDIUM confidence)
- [mymmrac/telego GitHub](https://github.com/mymmrac/telego) — alternative Telegram library comparison (Bot API 9.5 support)
- [alexei-led/ccgram](https://github.com/alexei-led/ccgram) — competitor analysis: topic-based multi-session, voice transcription patterns
- [RichardAtCT/claude-code-telegram](https://github.com/RichardAtCT/claude-code-telegram) — competitor analysis: conversational + terminal modes, git integration
- [betterstack Go logging comparison](https://betterstack.com/community/guides/logging/best-golang-logging-libraries/) — zerolog performance benchmarks
- [Telegram flood limits (grammY)](https://grammy.dev/advanced/flood) — rate limit behavior documentation
- [Killing child processes in Go](https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773) — process group kill patterns
- [Long Polling — Telegram.Bot .NET guide](https://telegrambots.github.io/book/3/updates/polling.html) — HTTP client timeout must exceed getUpdates timeout (cross-library confirmation)
- [kardianos/service vs NSSM comparison](https://paulbradley.dev/go-windows-service/) — deployment tradeoffs

### Tertiary (LOW confidence)
- NSSM environment variable configuration for service accounts — not verified from primary source; needs hands-on testing in Phase 3
- ask_user MCP JSON file protocol — inferred from TypeScript callback handler; not documented in public sources

---
*Research completed: 2026-03-19 (v1.0), updated 2026-03-20 (v1.1 bugfixes)*
*Ready for roadmap: yes*
