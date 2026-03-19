# Project Research Summary

**Project:** gsd-tele — Go Telegram Bot with Multi-Project Claude Code Management
**Domain:** Telegram bot wrapping AI CLI subprocess, multi-project session isolation, Windows Service deployment
**Researched:** 2026-03-19
**Confidence:** HIGH

## Executive Summary

This project is a rewrite of an existing TypeScript Telegram bot into Go, with a significant architectural upgrade: the current bot supports one Claude Code session globally, while the Go version must support independent Claude sessions per Telegram channel (one channel = one project). The existing TypeScript codebase serves as the functional specification — all behaviors must be preserved or improved. The research-validated approach is to use `gotgbot/v2` for Telegram long-polling, `os/exec` with a `bufio.Scanner` for streaming NDJSON from Claude CLI, a `sync.RWMutex`-protected `SessionStore` for concurrent session management, and `kardianos/service` for Windows Service lifecycle. State is stored in JSON files with atomic write-rename patterns. No database is needed.

The recommended architecture decomposes into six clearly bounded layers: Telegram dispatch, middleware (auth, rate limit, channel resolver), handlers (per media type), session management (per-channel store + worker queue), Claude subprocess streaming, and JSON persistence. Each layer has a single responsibility. The multi-project isolation guarantee — the core value proposition — emerges from the channel-ID-keyed `SessionStore`: every channel gets its own `Session` struct, its own worker goroutine, and its own Claude subprocess. This is not a refactor of the TypeScript architecture; it requires deliberate design from day one.

The dominant risk category is infrastructure correctness under Windows, not feature complexity. Eight critical pitfalls are identified, six of which must be addressed in Phase 1 before any feature work: Windows process tree orphaning (`taskkill /T /F` required), concurrent map panics (mutex from day one), JSON file corruption (atomic writes), goroutine leaks from pipe cleanup (`WaitDelay` on exec.Cmd), session ID invalidation after context limit, and service PATH blindness. Telegram rate limiting under multiple simultaneous sessions is the primary Phase 2 risk. Any one of these pitfalls, if skipped, will cause production failures that require architectural rework to fix.

## Key Findings

### Recommended Stack

Go 1.26.1 is the correct language choice for this project: native goroutines match the per-project concurrent session model, single-binary output simplifies Windows Service deployment, and the target runtime is Windows-native. The full stack is well-understood with HIGH confidence on all core components.

**Core technologies:**
- **Go 1.26.1**: Runtime — latest stable; goroutines are the right concurrency model for N independent Claude sessions
- **gotgbot/v2 v2.0.0-rc.34**: Telegram Bot API — auto-generated from official spec, type-safe, Bot API 9.4 support, processes each update in its own goroutine; the "rc" tag is misleading — this is the active production branch with 313 importers
- **os/exec (stdlib)**: Claude CLI subprocess — with `bufio.Scanner` for NDJSON streaming, `WaitDelay` for pipe cleanup, and `taskkill /T /F` for Windows process tree kill
- **kardianos/service**: Windows Service — de facto standard for Go Windows Services; bakes install/uninstall/start/stop into the binary itself
- **golang.org/x/time/rate**: Per-channel and global rate limiting — stdlib extension, token-bucket implementation, zero additional dependencies
- **rs/zerolog**: Structured logging — zero-allocation performance appropriate for high-frequency streaming log lines
- **openai/openai-go**: Official OpenAI SDK — for Whisper voice transcription; prefer over community forks for long-term maintenance
- **encoding/json + sync.RWMutex**: State persistence — JSON files are sufficient; no database required or desirable

**What NOT to use:** go-telegram-bot-api v5 (stale, 2021), SQLite/GORM (explicitly out of scope), Docker (out of scope), any HTTP router (no HTTP endpoints needed).

### Expected Features

The feature set splits cleanly into P1 (launch requirements) and P2 (post-validation additions). All P1 features are required for the core value proposition to function; none can be deferred without breaking the multi-project promise.

**Must have — table stakes:**
- Text message routing with streaming response (edit-in-place, throttled at 500ms minimum interval)
- Multi-project channel mapping with dynamic assignment — core differentiator; channel ID is the project key
- Independent Claude sessions per channel — the isolation guarantee
- Session lifecycle commands: `/new`, `/stop`, `/status`, `/start`, `/resume`
- Per-channel auth (channel membership = access)
- Rate limiting per channel and global across all outgoing Telegram API calls
- Session persistence with atomic JSON writes; `/resume` with inline keyboard picker
- Audit logging
- GSD inline keyboard with all 19 operations
- Contextual action buttons extracted from Claude responses
- Windows Service via NSSM

**Should have — differentiators (post-validation):**
- Voice transcription via OpenAI Whisper (mobile-first UX)
- Photo analysis with 1-second album buffering
- PDF document processing via pdftotext CLI
- Context window progress bar (parse `context_percent` from stream-json)
- Roadmap parsing in `/gsd` (phase progress display)
- Token usage display in `/status`
- ask_user MCP integration (inline keyboard for Claude clarifying questions)

**Defer to v2+:**
- Video/audio file transcription
- Archive extraction from documents
- `/retry` command
- Vault/knowledge base search integration

**Anti-features to avoid:** native Telegram streaming (15% commission), shared Claude sessions across channels (destroys isolation), webhook mode (requires HTTPS endpoint on Windows), Docker deployment (out of scope).

### Architecture Approach

The architecture follows a layered pipeline where each layer has a strict responsibility boundary enforced by Go's `internal/` package system. The key structural insight is that handlers never directly touch session internals — they call `SessionStore` methods which return `*Session` references, and sessions communicate with Claude processes via Go channels, not shared state. This makes concurrency bugs traceable and testable with the race detector.

**Major components:**
1. **gotgbot Updater + Dispatcher** — Long-poll Telegram updates; spawns one goroutine per update; no shared state
2. **Middleware chain** — Auth check, rate limit, channel-to-project resolver; all run before any handler is called
3. **Handler layer** — One file per update type (text, voice, photo, document, callback, commands); handlers enqueue messages and return immediately
4. **SessionStore** — `sync.RWMutex` + `map[int64]*Session`; each session owns a buffered message queue and a worker goroutine that serializes Claude queries
5. **Claude subprocess layer** — `os/exec.Cmd` with `bufio.Scanner` on stdout; NDJSON event parsing; `taskkill /T /F` on Windows for process tree kill
6. **StreamingState** — Per-query ephemeral state tracking Telegram message IDs and throttle timers; discarded after query completes
7. **Persistence layer** — Atomic write-rename for `channel-projects.json` and `session-history.json`; single `StateManager` with embedded mutex
8. **GSD package** — Isolated roadmap parsing, command extraction, inline keyboard builder
9. **Windows Service wrapper** — `svc/service.go` implements `svc.Handler`; wired in last without affecting application logic

**Key patterns from architecture research:**
- Channel-per-session message queue: buffered `chan QueuedMessage` (capacity 5) per session; worker goroutine drains serially; full queue returns "busy" reply
- NDJSON streaming via Scanner + StatusCallback: single goroutine owns stdout Scanner; typed events dispatched via injected callback function
- Atomic JSON persistence: write to temp file, `os.Rename` to target (atomic on Windows NTFS)

### Critical Pitfalls

Eight critical pitfalls identified; six are Phase 1 infrastructure concerns that cannot be deferred.

1. **Windows process tree orphaning** — Use `taskkill /pid <PID> /T /F` for all subprocess kills; `cmd.Process.Kill()` leaves Claude child processes running, accumulating as orphans across restarts. Gate on `runtime.GOOS == "windows"`.

2. **Concurrent map panic on SessionStore** — Wrap `channelID -> session` map in a struct with `sync.RWMutex` from day one. Run all tests with `-race`. This does not manifest in single-channel testing but crashes in production immediately.

3. **Goroutine leak from uncleaned subprocess pipes** — Set `WaitDelay` on every `exec.Cmd` (available since Go 1.20); use `CommandContext` with timeout. Without this, killed subprocess pipes leave goroutines blocked indefinitely; service memory grows until restart is required.

4. **JSON persistence file corruption** — Use atomic write-rename pattern for all JSON writes: marshal to temp file, `os.Rename` to target. Protect read-modify-write with mutex. Direct `os.WriteFile` in a goroutine-concurrent environment will corrupt files on crash.

5. **editMessageText rate limit causing full bot blackout** — Telegram rate limits are per-bot, not per-chat. With N simultaneous streaming sessions, edit rate multiplies by N. Implement a single global API call rate limiter, parse `retry_after` from 429 responses, and scale throttle interval with active session count.

6. **Service PATH blindness** — Windows Service account has a stripped PATH; `claude`, `pdftotext`, and other user-installed tools are invisible. Resolve all external binary paths at startup via explicit environment variables (`CLAUDE_CLI_PATH`, etc.); fall back to `exec.LookPath` only as secondary. Log resolved paths at startup.

7. **Claude session ID stale after context limit** — When Claude hits context window limit, the session ID is silently invalidated. Implement `isContextLimitError()` matching TypeScript patterns; on detection, clear session ID from memory and JSON, notify user.

8. **Long-polling offset reset causing duplicate updates** — Use gotgbot's built-in offset management; ensure clean shutdown via context cancellation before service stop to drain the update queue and acknowledge pending updates.

## Implications for Roadmap

The architecture research defines a clear build order based on component dependencies. This order is non-negotiable: later phases depend on earlier ones being correct, and skipping infrastructure correctness in Phase 1 will cause cascading failures in Phases 2 and 3.

### Phase 1: Core Infrastructure and Claude Subprocess

**Rationale:** Everything depends on this. Config parsing, atomic JSON persistence, the Claude subprocess wrapper (with correct pipe cleanup and Windows process tree kill), the SessionStore (with mutex), and session state machine (including context limit detection) are all load-bearing infrastructure. No feature works without them.

**Delivers:** A bot that can start, send a message to Claude, stream the response back to one Telegram channel, and stop cleanly. Windows Service can run interactively (`go run .`) with PATH resolution logged.

**Addresses:** Text message handler, streaming response, session persistence, `/start`/`/new`/`/stop`/`/status` commands, per-channel auth, rate limiting, audit logging

**Avoids:** Pitfalls 1 (process tree), 2 (concurrent map), 3 (goroutine leak), 4 (JSON corruption), 5 (context limit), 6 (PATH blindness)

**Research flag:** Standard patterns — well-documented Go subprocess and concurrency patterns. Skip deep research-phase; use architecture file directly.

### Phase 2: Multi-Project Session Management

**Rationale:** Multi-project isolation is the core value proposition and the primary architectural difference from the TypeScript version. It builds directly on Phase 1's SessionStore. This phase also introduces the Telegram rate limit risk from concurrent streaming sessions, which must be addressed before shipping.

**Delivers:** Multiple Telegram channels each routing to independent Claude sessions with no context bleed. Channel registration flow (unknown channel prompts for project path link). GSD inline keyboard with all 19 operations and contextual action button extraction.

**Addresses:** Multi-project channel mapping, dynamic project-channel assignment, independent sessions per channel, GSD workflow integration, contextual action buttons, `/resume` with picker, roadmap parsing in `/gsd`

**Avoids:** Pitfall 4 (rate limit flood across sessions); shared-session anti-pattern; global-user-allowlist anti-pattern

**Research flag:** GSD command extraction and keyboard builder logic may benefit from a targeted research pass on the GSD framework's command structure and how to parse `/gsd:*` patterns robustly from Claude responses.

### Phase 3: Media Handlers and Windows Service Deployment

**Rationale:** Media handlers (voice, photo, PDF) are independent of each other and all build on the core text handler pipeline established in Phase 1. Windows Service deployment is the last step because it requires the full bot to be functional for meaningful testing, and the service lifecycle wrapping does not affect application logic.

**Delivers:** Voice transcription via OpenAI Whisper, photo analysis with album buffering, PDF processing via pdftotext, Windows Service install/uninstall/start/stop via NSSM with correct PATH configuration.

**Addresses:** Voice messages, photo analysis, PDF document processing, Windows Service deployment, long-polling offset handling on service stop/restart

**Avoids:** Pitfall 8 (offset reset on service restart); NSSM PATH blindness (explicit env vars in service config); pdftotext PATH issues

**Research flag:** Windows Service deployment via NSSM with explicit PATH configuration may need a targeted research pass on NSSM environment variable configuration for service accounts.

### Phase 4: Enhanced Features and Polish

**Rationale:** These features add value but have no architectural dependencies that require them to be built before Phase 3. They can be added after the core bot is validated in production.

**Delivers:** Context window progress bar, token usage in `/status`, ask_user MCP integration, `/retry` command, adaptive streaming throttle based on active session count.

**Addresses:** Context percent display, cost awareness, MCP server integration, UX polish

**Avoids:** ask_user JSON file polling in a hot loop (use in-memory channel instead)

**Research flag:** ask_user MCP integration likely needs a research-phase pass — MCP server configuration and the JSON file polling protocol are not well-documented in standard sources.

### Phase Ordering Rationale

- **Infrastructure before features:** The concurrent map panic and goroutine leak pitfalls will crash a production service. Correct infrastructure is not optional.
- **Single-project before multi-project:** Building and verifying the core Claude streaming pipeline with one session is far easier to debug than building multi-project isolation simultaneously.
- **Application logic before service wrapper:** The Windows Service wrapping is a thin lifecycle layer. Testing it requires the full bot to work first.
- **Media handlers are independent:** Voice, photo, and PDF handlers do not depend on each other. They can be built in any order in Phase 3 or split across Phase 3 and 4 if velocity allows.
- **GSD integration in Phase 2, not Phase 3:** The GSD keyboard is identified as P1 (core differentiator) in FEATURES.md. It should ship with multi-project support, not as a later addition.

### Research Flags

Phases likely needing a `/gsd:research-phase` during planning:
- **Phase 2 (GSD integration):** Claude response parsing for `/gsd:*` command extraction and contextual button rendering. The GSD command structure needs verification against current GSD framework state.
- **Phase 3 (Windows Service PATH):** NSSM environment variable configuration for service accounts running user-installed tools (`claude` via npm/scoop). Configuration details are sparse in official NSSM docs.
- **Phase 4 (ask_user MCP):** The ask_user JSON file protocol, MCP server configuration format, and the lifecycle of MCP integrations under the Go bot model are not covered by standard documentation.

Phases with standard patterns (skip research-phase):
- **Phase 1 (Core infrastructure):** Go subprocess management, `sync.RWMutex`, `bufio.Scanner`, atomic file writes — all well-documented stdlib patterns. Architecture file covers everything needed.
- **Phase 3 (Media handlers):** OpenAI Whisper API, pdftotext CLI, photo album buffering — all standard patterns with high-quality official documentation.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All core libraries verified via official sources (pkg.go.dev, GitHub). Go 1.26.1 confirmed as latest stable. gotgbot v2.0.0-rc.34 confirmed production-grade despite "rc" tag. |
| Features | HIGH | Functional spec derived from existing TypeScript codebase. Competitor analysis from three active open-source projects. Native streaming 15% commission verified via primary source. |
| Architecture | HIGH | Derived from existing TypeScript codebase as functional spec plus Go stdlib patterns. gotgbot dispatcher model verified via library docs. Windows process tree kill pattern verified against Go issue tracker. |
| Pitfalls | HIGH | Most pitfalls verified against official Go issue tracker, library docs, and existing TypeScript codebase. Windows-specific pitfalls have primary source references. |

**Overall confidence:** HIGH

### Gaps to Address

- **kardianos/service Windows 11 compatibility:** The library has 137 open issues and the issues list was not fully reviewed. Core Windows Service functionality is considered stable (MEDIUM confidence), but edge cases under Windows 11 are unverified. Plan: test service install/uninstall/start/stop on the actual target machine early in Phase 3.
- **NSSM environment variable configuration for user-installed tools:** How NSSM passes environment to services and whether `%USERPROFILE%` paths resolve for the service account needs hands-on verification. Plan: include an explicit PATH configuration test in the Phase 3 acceptance checklist.
- **GSD framework current command structure:** The 19 GSD commands referenced in FEATURES.md are based on the GSD framework state as of research date. The keyboard builder must match the current framework. Plan: read `.claude/commands/gsd/` directory during Phase 2 planning.
- **ask_user MCP JSON file protocol:** The exact format, file path conventions, and lifecycle of the ask_user MCP integration are not documented in public sources. Plan: inspect the existing TypeScript implementation's `src/handlers/callback.ts` as the functional spec during Phase 4 planning.

## Sources

### Primary (HIGH confidence)
- Existing TypeScript codebase (`src/session.ts`, `src/handlers/`, `src/config.ts`) — functional specification for Go rewrite; proven solutions for context limit detection, process tree killing, HTML/plain fallback
- [gotgbot v2 pkg.go.dev](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2) — updater/dispatcher model, Bot API 9.4 support, published Feb 17, 2026
- [Go downloads page](https://go.dev/dl/) — go1.26.1 confirmed latest stable as of March 2026
- [go-cmd/cmd GitHub](https://github.com/go-cmd/cmd) — v1.4.3, thread-safe subprocess streaming, 100% test coverage, Windows support
- [openai/openai-go GitHub](https://github.com/openai/openai-go) — official SDK since July 2024
- [kardianos/service GitHub](https://github.com/kardianos/service) — Windows XP+ service support, de facto standard
- [golang.org/x/time/rate pkg.go.dev](https://pkg.go.dev/golang.org/x/time/rate) — token bucket rate limiter
- [Telegram streaming 15% commission](https://durovscode.com/streaming-responses-telegram-bots) — primary source for native streaming anti-feature
- [Go race detector](https://go.dev/doc/articles/race_detector) — concurrent map detection tooling
- [natefinch/atomic](https://github.com/natefinch/atomic) — atomic file write library for Windows
- [Go os/exec issue #23019](https://github.com/golang/go/issues/23019) — goroutine leak via pipes confirmed in Go issue tracker

### Secondary (MEDIUM confidence)
- [mymmrac/telego GitHub](https://github.com/mymmrac/telego) — alternative Telegram library comparison (Bot API 9.5 support)
- [alexei-led/ccgram](https://github.com/alexei-led/ccgram) — competitor analysis: topic-based multi-session, voice transcription patterns
- [RichardAtCT/claude-code-telegram](https://github.com/RichardAtCT/claude-code-telegram) — competitor analysis: conversational + terminal modes, git integration
- [factory-ben/droid-telegram-bot](https://github.com/factory-ben/droid-telegram-bot) — competitor analysis: permission prompts, autonomy levels
- [DoltHub os/exec patterns](https://www.dolthub.com/blog/2022-11-28-go-os-exec-patterns/) — StdoutPipe vs Scanner vs CommandContext tradeoffs
- [betterstack Go logging comparison](https://betterstack.com/community/guides/logging/best-golang-logging-libraries/) — zerolog performance benchmarks
- [Telegram flood limits (grammY)](https://grammy.dev/advanced/flood) — rate limit behavior documentation
- [Killing child processes in Go](https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773) — process group kill patterns
- [kardianos/service vs NSSM comparison](https://paulbradley.dev/go-windows-service/) — deployment tradeoffs

### Tertiary (LOW confidence)
- NSSM environment variable configuration for service accounts — not verified from primary source; needs hands-on testing
- ask_user MCP JSON file protocol — inferred from TypeScript callback handler; not documented in public sources

---
*Research completed: 2026-03-19*
*Ready for roadmap: yes*
