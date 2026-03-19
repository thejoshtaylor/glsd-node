# Stack Research

**Domain:** Go Telegram bot — multi-project Claude CLI manager, Windows Service deployment
**Researched:** 2026-03-19
**Confidence:** HIGH (core stack), MEDIUM (supporting libraries)

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.26.1 | Runtime language | Latest stable (go1.26.1 as of March 2026); native goroutines are the right model for concurrent per-project Claude sessions; single binary output simplifies Windows Service deployment |
| gotgbot/v2 | v2.0.0-rc.34 (latest) | Telegram Bot API | Auto-generated from official Telegram Bot API spec; type-safe (no `interface{}`); supports Bot API 9.4; processes each update in its own goroutine; only dependency is stdlib; actively maintained |
| go-cmd/cmd | v1.4.3 | Claude CLI subprocess management | Thread-safe streaming stdout/stderr; non-blocking async execution; `Status()` callable from any goroutine; 100% test coverage, no race conditions; works on Windows; purpose-built for this exact problem |
| kardianos/service | latest | Windows Service management | The de facto standard for running Go programs as Windows Services; handles the non-trivial Win32 service callback API; single unified API that also works on Linux/macOS if needed later |
| openai/openai-go | latest (v0.x) | OpenAI Whisper transcription | Official OpenAI SDK for Go released July 2024; supports audio transcription endpoint; actively maintained by OpenAI; prefer over community forks |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| golang.org/x/time/rate | latest | Per-channel rate limiting | Standard library extension; token-bucket implementation; use `rate.NewLimiter(r, b)` per channel; no extra dependencies |
| joho/godotenv | latest | .env file loading | For development config; simple projects don't need Viper; load at startup then use `os.Getenv()` throughout |
| rs/zerolog | latest | Structured logging | Fastest Go logger with zero allocations; chainable API; JSON output for production, human-readable for dev; slightly faster than slog for high-frequency streaming log lines |
| encoding/json | stdlib | JSON file persistence | Standard library is sufficient; project-channel mappings are simple structs; no query needs that require a database |
| sync | stdlib | Concurrent-safe file writes | `sync.RWMutex` wrapping JSON read/write; embed mutex in state struct; use `RLock`/`RUnlock` for reads, `Lock`/`Unlock` for writes |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| Go 1.26.1 toolchain | Build, test, vet | `go build ./...`, `go test ./...`, `go vet ./...` |
| NSSM (external) | Service install during development | Faster for dev iteration than coding install/uninstall; use kardianos/service for production service install via `./bot install` |
| gopls | LSP for IDE support | Ships with Go toolchain; use with VS Code + Go extension |
| golangci-lint | Static analysis | Run before commits; catches common Go mistakes |

## Installation

```bash
# Initialize module
go mod init github.com/yourorg/gsd-tele-go

# Core Telegram library
go get github.com/PaulSonOfLars/gotgbot/v2

# CLI subprocess management
go get github.com/go-cmd/cmd

# Windows Service
go get github.com/kardianos/service

# OpenAI (Whisper transcription)
go get github.com/openai/openai-go

# Rate limiting (part of x/time — already in Go ecosystem)
go get golang.org/x/time

# .env loading
go get github.com/joho/godotenv

# Structured logging
go get github.com/rs/zerolog
```

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| gotgbot/v2 | go-telegram/bot | If you want a simpler, less abstracted API; gotgbot is better when type safety and API completeness matter |
| gotgbot/v2 | telegram-bot-api (go-telegram-bot-api) | Avoid — last release was v5.5.1 in December 2021 (3+ years stale, Bot API 5.x era) |
| gotgbot/v2 | mymmrac/telego | Telego is a solid alternative (Bot API 9.5 support, active); use if gotgbot RC status is a concern, though gotgbot's RC tag is misleading — it has been the active v2 branch for years |
| gotgbot/v2 | tucnak/telebot | Telebot adds opinionated abstractions that fight you when you need streaming updates or fine-grained control |
| go-cmd/cmd | os/exec directly | Only if you need to customize subprocess environment deeply; direct os/exec requires manual goroutine coordination for stdout/stderr to avoid race conditions and deadlocks |
| kardianos/service | NSSM only | NSSM is fine for personal dev machines, but kardianos/service bakes the install/uninstall/start/stop lifecycle into the binary itself — users run `./bot install` instead of requiring NSSM |
| openai/openai-go | sashabaranov/go-openai | sashabaranov is still widely used (7k+ stars) and works; prefer official library for long-term maintenance guarantee |
| rs/zerolog | log/slog (stdlib) | Use slog if you want zero external dependencies and don't need the performance of zerolog; for streaming bots emitting many log lines per second, zerolog's zero-allocation approach is measurably better |
| joho/godotenv | spf13/viper | Use Viper only if config grows to multiple formats (YAML, TOML) or needs live reload; for a .env-only bot, Viper is massive overkill |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| go-telegram-bot-api v5 | Last release December 2021 — does not support Bot API 6.x+ features (inline keyboards, reactions, etc.); actively unmaintained | gotgbot/v2 or telego |
| tucnak/telebot | Heavy framework opinions that limit control; poor fit for streaming output patterns; community has moved on | gotgbot/v2 |
| SQLite / GORM | PROJECT.md explicitly rules out database storage; JSON files are sufficient for project-channel mappings and session state | encoding/json + sync.RWMutex |
| Docker | PROJECT.md explicitly out of scope; adds operational complexity without benefit for a single-machine Windows bot | Windows Service via kardianos/service |
| Gorilla/mux, chi, or any HTTP router | No HTTP endpoints needed — Telegram long-polling only; avoid pulling in HTTP frameworks | None — use gotgbot's built-in dispatcher |
| sync/atomic for file state | Atomic ops work on primitives, not structs; JSON persistence requires mutex-protected reads and writes | sync.RWMutex |
| log.Fatal / fmt.Println for logging | No structured output; can't filter by level; no JSON format for log files | rs/zerolog or log/slog |

## Stack Patterns by Variant

**For per-project session isolation:**
- One `ClaudeSession` struct per project, holding its own `go-cmd/cmd.Cmd` instance
- Sessions stored in a `sync.Map` keyed by channel ID (chat ID as int64)
- Each session runs its goroutine loop independently; no shared state between projects

**For streaming Claude output to Telegram:**
- Poll `cmd.Status().Stdout` lines at a configurable interval (e.g., 200ms)
- Buffer lines into a single Telegram message edit (avoid hitting Telegram's 30 msg/min per chat limit)
- Use gotgbot's `EditMessageText` to update a single in-flight message

**For Windows Service lifecycle:**
- Implement `kardianos/service.Interface` (two methods: `Start(s service.Service)` and `Stop(s service.Service)`)
- CLI flags: `install`, `uninstall`, `start`, `stop`, `run` (interactive)
- Use `context.Context` cancellation to propagate shutdown to all active sessions

**For JSON state persistence:**
- Single `StateManager` struct with embedded `sync.RWMutex`
- Atomic write pattern: marshal to temp file, `os.Rename` to target (rename is atomic on Windows NTFS)
- Load on startup, save on every mutation

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| gotgbot/v2 v2.0.0-rc.34 | Go 1.18+ | The "rc" tag is misleading; this is the active production branch; 313 projects import it |
| go-cmd/cmd v1.4.3 | Go 1.13+ | Stable v1; last updated June 2024 |
| kardianos/service latest | Go 1.17+, Windows XP+ | Works on Windows 11; 137 open issues but core Windows Service functionality is stable |
| openai/openai-go latest | Go 1.21+ | Official SDK; stability warning in README — check changelog on upgrade |
| golang.org/x/time latest | Go 1.18+ | Part of official golang.org/x family; stable |
| rs/zerolog latest | Go 1.15+ | v1 stable; no breaking changes policy |

## Sources

- [gotgbot releases page](https://github.com/PaulSonOfLars/gotgbot/releases) — version v2.0.0-rc.34, Bot API 9.4 support (HIGH confidence)
- [gotgbot pkg.go.dev](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2) — published Feb 17, 2026 (HIGH confidence)
- [mymmrac/telego GitHub](https://github.com/mymmrac/telego) — Bot API 9.5, March 2026 (HIGH confidence via WebFetch)
- [go-telegram-bot-api GitHub](https://github.com/go-telegram-bot-api/telegram-bot-api) — v5.5.1, Dec 2021, confirmed stale (HIGH confidence)
- [go-cmd/cmd GitHub](https://github.com/go-cmd/cmd) — v1.4.3, June 2024, Windows support confirmed (HIGH confidence)
- [kardianos/service GitHub](https://github.com/kardianos/service) — Windows XP+ support, no Windows 11-specific issues found (MEDIUM confidence — issues list not fully reviewed)
- [openai/openai-go GitHub](https://github.com/openai/openai-go) — official SDK since July 2024 (HIGH confidence)
- [Go downloads page](https://go.dev/dl/) — go1.26.1 is latest stable as of March 2026 (HIGH confidence)
- [golang.org/x/time/rate pkg.go.dev](https://pkg.go.dev/golang.org/x/time/rate) — standard token bucket (HIGH confidence)
- [betterstack Go logging comparison](https://betterstack.com/community/guides/logging/best-golang-logging-libraries/) — zerolog performance benchmarks (MEDIUM confidence)
- [joho/godotenv GitHub](https://github.com/joho/godotenv) — simple .env loading, widely used (HIGH confidence)
- [kardianos/service NSSM comparison](https://paulbradley.dev/go-windows-service/) — deployment tradeoffs (MEDIUM confidence)

---
*Stack research for: Go Telegram bot with multi-project Claude CLI management, Windows Service deployment*
*Researched: 2026-03-19*
