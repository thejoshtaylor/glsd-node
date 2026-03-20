# Stack Research

**Domain:** Go Telegram bot — multi-project Claude CLI manager, Windows Service deployment
**Researched:** 2026-03-19 (updated 2026-03-20 for v1.1 bugfixes)
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

## v1.1 Bugfix Addendum: gotgbot/v2 APIs for Channel Auth and Long-Poll Timeout

*Added 2026-03-20. Focused scope: only the specific APIs needed for the two v1.1 bugs.*

---

### Bug 1: Channel Auth — EffectiveSender for Channel Posts

#### Root Cause

In `internal/bot/middleware.go`, `authMiddlewareWith` extracts `userID` via `ctx.EffectiveSender.Id()`. For Telegram channel posts (`update.ChannelPost`), the message has no `From` user — only a `SenderChat`. The `Sender.Id()` method returns the channel's chat ID (not a user ID), which will never match any entry in `AllowedUsers`, causing auth rejection for all legitimate channel messages.

The existing `security.IsAuthorized` function checks only user IDs. Channel posts have no user — only a chat.

#### gotgbot/v2 Types Verified (v2.0.0-rc.34, confirmed against GitHub v2 branch)

**`gotgbot.Sender` struct:**

```go
type Sender struct {
    User               *User
    Chat               *Chat
    IsAutomaticForward bool
    ChatId             int64    // ID of the destination chat
    AuthorSignature    string
}
```

**Sender classification methods:**

| Method | Condition | Use case |
|--------|-----------|----------|
| `IsUser()` | `Chat == nil && User != nil` | Normal user or bot DM |
| `IsBot()` | `User != nil && User.IsBot` | Bot-generated message |
| `IsChannelPost()` | `Chat != nil && Chat.Id == ChatId && Chat.Type == "channel"` | Direct channel post — THIS is the target case |
| `IsAnonymousAdmin()` | `Chat != nil && Chat.Id == ChatId && Chat.Type != "channel"` | Anonymous admin in group |
| `IsLinkedChannel()` | `Chat != nil && Chat.Id != ChatId && IsAutomaticForward` | Forwarded from linked channel |

**`Sender.Id()` behavior:**
- If `Chat != nil`: returns `Chat.Id` (the channel's ID, not a user ID)
- Else if `User != nil`: returns `User.Id`
- Else: returns `0`

**How `EffectiveSender` is populated for channel posts** (verified from `ext/context.go`):

For `update.ChannelPost != nil`, the context sets `EffectiveMessage = update.ChannelPost` and `EffectiveChat = &update.ChannelPost.Chat`. No `EffectiveUser` is set. The `Sender` is derived by `msg.GetSender()`:

```go
// What GetSender() returns for a channel post:
&Sender{
    User:               m.From,           // nil — channel posts have no From
    Chat:               m.SenderChat,     // the channel that posted
    IsAutomaticForward: m.IsAutomaticForward,
    ChatId:             m.Chat.Id,        // same as Chat.Id for direct channel posts
}
```

Because `Chat.Id == ChatId` and `Chat.Type == "channel"`, `IsChannelPost()` returns `true`.

**`EffectiveChat` is always reliable.** It is populated for all update types including channel posts. `ctx.EffectiveChat.Id` gives the channel's ID without depending on sender.

#### Fix Pattern

Auth must distinguish between user messages (auth by user ID) and channel posts (auth by channel ID). The `IsChannelPost()` method on `Sender` is the canonical gotgbot signal:

```go
// Replace the current userID extraction block in authMiddlewareWith:

var userID int64
var isChannelMessage bool

if ctx.EffectiveSender != nil {
    if ctx.EffectiveSender.IsChannelPost() || ctx.EffectiveSender.IsAnonymousAdmin() {
        // No user ID available — auth by channel ID instead
        isChannelMessage = true
    } else {
        userID = ctx.EffectiveSender.Id()
    }
}

var channelID int64
if ctx.EffectiveChat != nil {
    channelID = ctx.EffectiveChat.Id
}
```

Since `MappingStore` already tracks which channel IDs are registered projects, the auth rule for channel posts is: **a channel post is authorized if and only if the channel ID is registered in `MappingStore`**. This requires no new data — it uses the existing mapping infrastructure.

---

### Bug 2: Long-Poll Timeout — HTTP Client vs getUpdates Timeout

#### Root Cause

Current code in `internal/bot/bot.go`:

```go
b.updater.StartPolling(b.bot, &ext.PollingOpts{
    GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
        Timeout: 10,
    },
})
```

`GetUpdatesOpts.Timeout` (value: `10`) tells Telegram's server to hold the connection for up to 10 seconds. However, `gotgbot.NewBot` is called with `nil` BotOpts, which means `BaseBotClient` applies its `DefaultTimeout` of **5 seconds** to every HTTP request via context deadline.

Telegram holds the connection for 10 seconds; the HTTP client cancels it after 5 seconds. Every long-poll cycle where no update arrives in the first 5 seconds produces `context deadline exceeded`.

#### gotgbot/v2 Types Verified (v2.0.0-rc.34, confirmed against GitHub v2 branch and request.go)

**`BaseBotClient` struct** (in `request.go`):

```go
type BaseBotClient struct {
    Client             http.Client   // underlying HTTP client
    UseTestEnvironment bool
    DefaultRequestOpts *RequestOpts  // applied to every request when no per-call opts override
}
```

**`RequestOpts` struct:**

```go
type RequestOpts struct {
    Timeout time.Duration  // context deadline for HTTP request
    APIURL  string
}
```

Timeout semantics:
- Positive value: sets a specific context deadline
- Negative value: no timeout (infinite) — use `-1 * time.Second` for no limit
- Zero: falls through to `DefaultTimeout` (5 seconds hardcoded in `request.go`)

**`GetUpdatesOpts` with per-call `RequestOpts`:**

```go
type GetUpdatesOpts struct {
    Offset         int64
    Limit          int64
    Timeout        int64          // seconds — server-side hold duration
    AllowedUpdates []string
    RequestOpts    *RequestOpts   // overrides DefaultRequestOpts for this call only
}
```

**`BotOpts` struct** (passed to `gotgbot.NewBot`):

```go
type BotOpts struct {
    BotClient         BotClient    // inject custom BaseBotClient
    DisableTokenCheck bool
    RequestOpts       *RequestOpts // used only for the initial GetMe validation call
}
```

#### Fix Pattern (Recommended: per-request timeout in GetUpdatesOpts)

Set `RequestOpts.Timeout` inside `GetUpdatesOpts` so only the long-poll call gets a longer timeout. All other API calls (sendMessage, editMessage, etc.) retain the default 5-second timeout.

```go
b.updater.StartPolling(b.bot, &ext.PollingOpts{
    DropPendingUpdates: false,
    GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
        Timeout: 30,   // seconds — Telegram holds connection up to this long
        RequestOpts: &gotgbot.RequestOpts{
            Timeout: 35 * time.Second,  // HTTP deadline must exceed the server-side hold
        },
    },
})
```

The rule: `RequestOpts.Timeout` must be greater than `GetUpdatesOpts.Timeout` (converted to duration). The official gotgbot sample uses +1 second. Using +5 seconds provides a safer buffer for slow networks.

**Official sample** (`samples/middlewareBot/main.go`, GitHub v2 branch) uses exactly:

```go
GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
    Timeout: 9,
    RequestOpts: &gotgbot.RequestOpts{
        Timeout: time.Second * 10,
    },
},
```

This is the canonical pattern. The current code sets `Timeout: 10` but omits `RequestOpts`, which is the bug.

**Alternative: DefaultRequestOpts on BaseBotClient** (not recommended for this case)

```go
tgBot, err := gotgbot.NewBot(cfg.TelegramToken, &gotgbot.BotOpts{
    BotClient: &gotgbot.BaseBotClient{
        Client: http.Client{},
        DefaultRequestOpts: &gotgbot.RequestOpts{
            Timeout: 35 * time.Second,
        },
    },
})
```

This applies a 35-second timeout to ALL API calls from this bot, including sendMessage and editMessage. Not recommended — it only adds unnecessary latency tolerance on calls that should fail fast.

#### What NOT to Do

| Avoid | Why |
|-------|-----|
| Increase `GetUpdatesOpts.Timeout` without adding `RequestOpts.Timeout` | Reproduces the same bug at a longer duration |
| Set `http.Client.Timeout` on the underlying `http.Client` in `BaseBotClient` | Affects all requests; can't be overridden per-call; less flexible |
| Use negative `RequestOpts.Timeout` globally | Removes all timeouts from every API call; hides real network failures |

---

### Summary Table for v1.1 Fixes

| Bug | Current Code | Fix | API Used |
|-----|-------------|-----|----------|
| Channel auth failure | `EffectiveSender.Id()` returns channel ID, not user ID | Check `EffectiveSender.IsChannelPost()` before extracting user ID; auth channel posts by `channelID` via `MappingStore` | `gotgbot.Sender.IsChannelPost()`, `gotgbot.Sender.IsUser()` |
| Long-poll timeout | `GetUpdatesOpts{Timeout: 10}` with no `RequestOpts`; default HTTP timeout is 5s | Add `RequestOpts: &gotgbot.RequestOpts{Timeout: 35 * time.Second}` inside `GetUpdatesOpts` alongside `Timeout: 30` | `gotgbot.GetUpdatesOpts.RequestOpts`, `gotgbot.RequestOpts` |

No dependency changes. Both fixes are pure configuration/logic changes to existing files. No new packages needed.

---

### Additional v1.1 Sources

- [gotgbot ext package — pkg.go.dev v2.0.0-rc.34](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2@v2.0.0-rc.34/ext) — PollingOpts struct, long-poll Timeout docs and warning about context deadline exceeded — HIGH confidence
- [gotgbot context.go — GitHub v2 branch](https://github.com/PaulSonOfLars/gotgbot/blob/v2/ext/context.go) — ChannelPost case in context population verified — HIGH confidence
- [gotgbot sender.go — GitHub v2 branch](https://github.com/PaulSonOfLars/gotgbot/blob/v2/sender.go) — Sender struct, IsChannelPost(), Id() methods verified — HIGH confidence
- [gotgbot request.go — GitHub v2 branch](https://github.com/PaulSonOfLars/gotgbot/blob/v2/request.go) — BaseBotClient, DefaultRequestOpts, DefaultTimeout (5s), getTimeoutContext verified — HIGH confidence
- [gotgbot middleware sample — GitHub v2 branch](https://github.com/PaulSonOfLars/gotgbot/blob/v2/samples/middlewareBot/main.go) — canonical pattern Timeout:9 + RequestOpts:10s confirmed — HIGH confidence

---

*Stack research for: Go Telegram bot with multi-project Claude CLI management, Windows Service deployment*
*Researched: 2026-03-19, updated 2026-03-20 for v1.1 bugfixes*
