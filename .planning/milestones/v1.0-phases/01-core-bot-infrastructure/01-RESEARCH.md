# Phase 1: Core Bot Infrastructure - Research

**Researched:** 2026-03-19
**Domain:** Go Telegram bot — Claude CLI subprocess management, streaming NDJSON, session persistence, Windows process management
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Streaming response display:**
- Keep emoji tool status indicators (e.g. Search, Edit, Write) — show tool name with emoji while executing, replace with response when done
- Streaming edits throttled at 500ms minimum interval — fast enough to feel live, slow enough to avoid Telegram rate limits
- Message splitting at paragraph boundaries (last double-newline before 4096 char limit) — keeps code blocks and paragraphs intact
- Show "Thinking..." message with Telegram typing action while Claude processes before text appears; message gets replaced by actual response
- Use **MarkdownV2** for Telegram message formatting (deliberate change from TypeScript version which used HTML with plain text fallback)

**Command output & session UX:**
- `/status` shows full dashboard: session state + token usage (input/output/cache) + context percentage + current project path + uptime
- Context window usage displayed as **percentage only** (e.g. "Context: 42%") — no progress bar
- `/resume` presents saved sessions as inline keyboard buttons showing timestamp + first message preview — one-tap restore
- Retain 5 sessions per project for `/resume` history
- `/start` shows brief welcome + status: bot name, version, current project path (if linked), and available commands

**Error & state messaging:**
- Context limit (hard "prompt too long" errors after auto-compaction fails): auto-clear session + notify user with recovery hint — "Session hit hard context limit and was cleared. Use /resume to restore a previous session."
- Rate limit rejections: terse with retry time — "Rate limited. Try again in 12s."
- Unauthorized access: rejection with reason — "You're not authorized for this channel. Contact the bot admin."
- Claude CLI errors: truncated stderr — "Claude error: [first 200 chars of stderr]" — enough to diagnose without flooding chat

### Claude's Discretion
- Exact MarkdownV2 escaping strategy and fallback behavior if parsing fails
- Typing indicator update interval
- Exact emoji-to-tool mapping (can follow TypeScript patterns or improve)
- Audit log format details (structured JSON vs plain text)
- Session state machine internal design
- Go package layout and internal organization

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| CORE-01 | Bot connects to Telegram via long polling and receives messages from multiple channels | gotgbot/v2 updater/dispatcher model; per-update goroutines handle concurrent channels |
| CORE-02 | Bot loads configuration from environment variables and/or config file | `os.Getenv` + `joho/godotenv`; startup path resolution pattern from TypeScript `findClaudeCli()` |
| CORE-03 | Bot sends typing indicators while processing requests | `bot.SendChatAction` in loop goroutine; TypeScript uses 4s interval loop with stop controller |
| CORE-04 | Bot reports errors back to the user with truncated error messages | Pattern established in TypeScript: truncate stderr to 200 chars, send as plain text |
| CORE-05 | Bot rate-limits requests per channel using token bucket algorithm | `golang.org/x/time/rate` per-channel limiter; TypeScript `RateLimiter` is direct translation target |
| CORE-06 | Bot writes append-only audit log (timestamp, user, channel, action, message excerpt) | Goroutine-safe buffered writer; TypeScript shows both JSON and plain text formats |
| SESS-01 | Bot spawns and manages Claude CLI as a subprocess with streaming JSON output | `os/exec.Cmd` + `bufio.Scanner` on stdout; `--output-format stream-json --include-partial-messages` |
| SESS-02 | Bot streams Claude responses with throttled edit-in-place message updates | 500ms minimum throttle; `bot.EditMessageText` + segment tracking via map[int]*StreamingState |
| SESS-03 | Bot displays tool execution status with emoji indicators during streaming | Single ephemeral status message (edit-in-place for tools); 10 tool types mapped in TypeScript |
| SESS-04 | User can send text messages that are routed to the channel's Claude session | Session worker queue pattern; handlers enqueue and return immediately |
| SESS-05 | User can interrupt a running query by sending a message prefixed with `!` | Strip `!`, call `session.Stop()`, clear stopRequested flag, enqueue new message |
| SESS-06 | Bot shows context window usage as a progress bar in status messages | **Note: CONTEXT.md overrides REQUIREMENTS.md** — display as percentage only, not progress bar |
| SESS-07 | Bot tracks and displays token usage (input/output/cache) in /status | Parse `usage` field from `result` event in NDJSON stream |
| SESS-08 | Bot properly kills Windows process trees (taskkill /T /F) when stopping sessions | `taskkill /pid <PID> /T /F` pattern; gated on `runtime.GOOS == "windows"` |
| AUTH-01 | Bot authenticates users based on Telegram channel membership (per-channel auth) | Channel ID allowlist in config; single-channel Phase 1 = one allowed channel ID |
| AUTH-02 | Bot validates file paths against allowed directories before Claude access | Path normalization + prefix matching; `filepath.Clean` + `strings.HasPrefix` pattern |
| AUTH-03 | Bot checks commands against blocked patterns for safety | Blocked pattern slice; substring match after lowercase normalization |
| CMD-01 | `/start` — shows bot info and current channel status | Welcome text + session state + version + working dir + command list |
| CMD-02 | `/new` — creates a new Claude session for the current channel's project | Stop running query if active, clear session ID, notify user |
| CMD-03 | `/stop` — aborts the currently running Claude query | Signal `stopCh`, call `taskkill`, silent response |
| CMD-04 | `/status` — shows session state, token usage, context usage, project info | Full dashboard: session ID prefix, running/idle, uptime, tokens in/out/cache, context %, working dir |
| CMD-05 | `/resume` — lists saved sessions with inline keyboard picker to restore one | Load `session-history.json`, build `InlineKeyboardMarkup` with timestamp + title, one row per session |
| PERS-01 | Bot saves session state (session ID, working dir, conversation context) to JSON | `session-history.json` with atomic write-rename; format: `{sessions: [{session_id, saved_at, working_dir, title}]}` |
| PERS-02 | Bot restores sessions automatically on restart for all mapped channels | Load on startup; set `sessionID` in memory before first message |
| PERS-03 | Session state persists across bot crashes and service restarts | Atomic write pattern (`os.WriteFile` to temp + `os.Rename`); mutex-protected read-modify-write |
| DEPLOY-01 | Bot compiles to a single Go binary (.exe) for Windows | `GOOS=windows GOARCH=amd64 go build -o bot.exe .` |
| DEPLOY-03 | Bot resolves external tool paths (claude, pdftotext) explicitly at startup | Env var `CLAUDE_CLI_PATH` → fallback `exec.LookPath("claude")`; log resolved path at startup |
| DEPLOY-04 | Bot supports graceful shutdown — drains active sessions before stopping | `context.CancelFunc` propagated to all goroutines; wait for session worker goroutines to exit |
</phase_requirements>

---

## Summary

Phase 1 is a direct Go translation of the existing TypeScript codebase with deliberate improvements: MarkdownV2 formatting instead of HTML, per-channel session architecture from day one (instead of the TypeScript global singleton), and native goroutine concurrency instead of async/await chains. The TypeScript source serves as the functional specification — every behavior described there must be replicated in Go unless explicitly overridden by CONTEXT.md decisions.

The architecture is straightforward: gotgbot/v2 dispatches updates into goroutines, middleware chains handle auth and rate limiting, a `SessionStore` (mutex-protected map) owns per-channel state, and a worker goroutine per session serializes Claude CLI queries. The Claude subprocess layer uses `bufio.Scanner` on stdout for NDJSON line-by-line streaming, with a `StatusCallback` function driving Telegram message edits. Six infrastructure pitfalls identified in project research are all Phase 1 concerns and must be addressed from the start, not retrofitted.

The dominant complexity is not any individual feature — each maps directly from TypeScript — but the coordination between goroutines, the correct pipe cleanup on subprocess kill, and the MarkdownV2 escaping strategy. MarkdownV2 is significantly more strict than HTML: every special character must be escaped in non-entity contexts, and entity boundaries must be precise. The TypeScript version's HTML approach avoids this entirely; the Go version must implement a careful escaper and provide a plain-text fallback for when MarkdownV2 parsing fails.

**Primary recommendation:** Build in the order defined by component dependencies — config/persistence → Claude subprocess → session store → bot skeleton → streaming layer → handlers. Do not skip the atomic write pattern or the race detector tests. Use the TypeScript source as the exact behavioral spec for NDJSON event handling, tool emoji mapping, and command outputs.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.26.1 | Runtime | Latest stable (March 2026); native goroutines are the right model for concurrent sessions; single binary output |
| github.com/PaulSonOfLars/gotgbot/v2 | v2.0.0-rc.34 | Telegram Bot API | Auto-generated from official spec; type-safe; Bot API 9.4; per-update goroutines; 313 importers |
| golang.org/x/time/rate | latest | Per-channel rate limiting | stdlib extension; token bucket; zero dependencies |
| github.com/rs/zerolog | latest | Structured logging | Zero-allocation; JSON output; human-readable dev mode |
| github.com/joho/godotenv | latest | .env loading | Minimal; load at startup, use `os.Getenv` everywhere else |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| encoding/json (stdlib) | — | JSON persistence | Session history, state files |
| sync (stdlib) | — | Mutex for concurrent state | SessionStore, JSON file writers |
| os/exec (stdlib) | — | Claude CLI subprocess | Direct stdlib; Scanner on stdout |
| bufio (stdlib) | — | NDJSON line scanning | `bufio.NewScanner(cmd.Stdout)` |
| context (stdlib) | — | Cancellation propagation | Subprocess kill, graceful shutdown |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| gotgbot/v2 | mymmrac/telego | Telego has Bot API 9.5 support; gotgbot chosen for larger importer base |
| os/exec directly | go-cmd/cmd v1.4.3 | go-cmd handles pipe cleanup automatically; os/exec chosen for full control over WaitDelay |
| rs/zerolog | log/slog (stdlib) | slog is sufficient; zerolog chosen for zero-allocation performance during high-frequency streaming |

**Installation:**
```bash
go mod init github.com/yourorg/gsd-tele-go
go get github.com/PaulSonOfLars/gotgbot/v2
go get golang.org/x/time
go get github.com/rs/zerolog
go get github.com/joho/godotenv
```

**Version verification:** Versions above confirmed against pkg.go.dev as of 2026-03-19.

---

## Architecture Patterns

### Recommended Project Structure

```
.
├── main.go                         # Entry point: parse flags, start bot or service
├── internal/
│   ├── config/
│   │   └── config.go               # ENV parsing, path resolution, constants
│   ├── bot/
│   │   ├── bot.go                  # Bot struct, startup, shutdown
│   │   ├── middleware.go           # Auth, rate-limit middleware chain
│   │   └── handlers.go             # Register all handlers on dispatcher
│   ├── handlers/
│   │   ├── command.go              # /start /new /stop /status /resume
│   │   ├── text.go                 # Text message handler
│   │   ├── callback.go             # Inline keyboard callbacks (resume picker)
│   │   └── streaming.go            # StreamingState + createStatusCallback
│   ├── session/
│   │   ├── store.go                # SessionStore: sync.RWMutex + map[int64]*Session
│   │   ├── session.go              # Session struct: queue, stop, state machine
│   │   └── persist.go              # Save/load session-history.json (atomic writes)
│   ├── claude/
│   │   ├── process.go              # Spawn claude CLI, stream NDJSON, kill tree
│   │   └── events.go               # NDJSON event structs: assistant, result, tool_use
│   ├── formatting/
│   │   ├── markdown.go             # Markdown → MarkdownV2 conversion + escaping
│   │   └── tools.go                # Tool status emoji formatting (port of formatToolStatus)
│   ├── security/
│   │   ├── ratelimit.go            # Token bucket per channelID
│   │   └── validate.go             # Path validation, command safety checks
│   └── audit/
│       └── log.go                  # Append-only goroutine-safe audit log
└── data/                           # Runtime JSON files (gitignored)
    └── session-history.json
```

### Pattern 1: Channel-per-Session Worker Queue

**What:** Each `Session` has a buffered `chan QueuedMessage` (capacity 5). One goroutine per session drains the queue serially. Handlers enqueue and return immediately. If queue is full, the user gets "Queue full" reply.

**When to use:** Wherever Claude queries must serialize per-channel while the bot continues accepting updates concurrently.

**Example:**
```go
// internal/session/session.go
type Session struct {
    mu          sync.Mutex
    sessionID   string
    workingDir  string
    isRunning   bool
    queue       chan QueuedMessage  // buffered, capacity 5
    stopCh      chan struct{}
    cancelQuery context.CancelFunc
    // Status fields
    lastUsage     *TokenUsage
    contextPct    *int
    queryStarted  *time.Time
    lastActivity  *time.Time
    lastError     string
}

func (s *Session) worker(ctx context.Context) {
    for {
        select {
        case msg := <-s.queue:
            s.runQuery(ctx, msg)
        case <-ctx.Done():
            return
        }
    }
}
```

### Pattern 2: NDJSON Streaming via Scanner + StatusCallback

**What:** A goroutine owns a `bufio.Scanner` on `cmd.Stdout`. Each scanned line is JSON-unmarshalled into a typed `ClaudeEvent`. A `StatusCallback` function is called per event; it drives Telegram message edits.

**When to use:** Any subprocess emitting structured line-delimited JSON.

**Example:**
```go
// internal/claude/process.go
type StatusCallback func(eventType string, content string, segmentID int) error

func (p *Process) Stream(ctx context.Context, cb StatusCallback) error {
    scanner := bufio.NewScanner(p.cmd.Stdout)
    scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB for large tool outputs

    stderrDone := make(chan struct{})
    go func() {
        defer close(stderrDone)
        s := bufio.NewScanner(p.cmd.Stderr)
        for s.Scan() {
            line := s.Text()
            if strings.TrimSpace(line) != "" {
                p.stderrBuf.WriteString(line + "\n")
                if isContextLimitError(line) {
                    p.contextLimitHit = true
                }
            }
        }
    }()

    for scanner.Scan() {
        if ctx.Err() != nil {
            break
        }
        var event ClaudeEvent
        if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
            continue
        }
        if err := p.dispatchEvent(event, cb); err != nil {
            return err
        }
    }
    <-stderrDone
    return p.cmd.Wait()
}
```

### Pattern 3: sync.RWMutex SessionStore

**What:** `SessionStore` wraps `map[int64]*Session` with `sync.RWMutex`. Reads use `RLock`, writes use `Lock`. Session-internal state has its own mutex.

**Example:**
```go
// internal/session/store.go
type SessionStore struct {
    mu       sync.RWMutex
    sessions map[int64]*Session
}

func (s *SessionStore) Get(channelID int64) (*Session, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    sess, ok := s.sessions[channelID]
    return sess, ok
}

func (s *SessionStore) GetOrCreate(channelID int64, workingDir string) *Session {
    // Fast path: RLock
    s.mu.RLock()
    if sess, ok := s.sessions[channelID]; ok {
        s.mu.RUnlock()
        return sess
    }
    s.mu.RUnlock()
    // Slow path: Lock
    s.mu.Lock()
    defer s.mu.Unlock()
    if sess, ok := s.sessions[channelID]; ok {
        return sess // double-check after acquiring write lock
    }
    sess := newSession(workingDir)
    s.sessions[channelID] = sess
    go sess.worker(context.Background()) // start worker goroutine
    return sess
}
```

### Pattern 4: Windows Process Tree Kill

**What:** `taskkill /pid <PID> /T /F` kills the entire process tree including `cmd.exe` wrapper and all Claude child processes.

**Example:**
```go
// internal/claude/process.go
func (p *Process) Kill() error {
    if p.cmd.Process == nil {
        return nil
    }
    if runtime.GOOS == "windows" {
        pid := strconv.Itoa(p.cmd.Process.Pid)
        return exec.Command("taskkill", "/pid", pid, "/T", "/F").Run()
    }
    // Unix fallback (for dev/test)
    return p.cmd.Process.Signal(syscall.SIGTERM)
}
```

### Pattern 5: Atomic JSON Persistence

**What:** Write to a temp file in the same directory, then `os.Rename`. Mutex-protected read-modify-write cycle prevents concurrent corruption.

**Example:**
```go
// internal/session/persist.go
type PersistenceManager struct {
    mu       sync.Mutex
    filePath string
}

func (pm *PersistenceManager) Save(history *SessionHistory) error {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    data, err := json.MarshalIndent(history, "", "  ")
    if err != nil {
        return err
    }

    dir := filepath.Dir(pm.filePath)
    tmp, err := os.CreateTemp(dir, "session-*.tmp")
    if err != nil {
        return err
    }
    tmpPath := tmp.Name()
    defer func() { _ = os.Remove(tmpPath) }() // cleanup on failure

    if _, err := tmp.Write(data); err != nil {
        _ = tmp.Close()
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }
    return os.Rename(tmpPath, pm.filePath)
}
```

### Pattern 6: MarkdownV2 Escaping (Phase 1 Critical Decision)

**What:** Telegram MarkdownV2 requires escaping `_ * [ ] ( ) ~ ` > # + - = | { } . !` outside entity contexts. The TypeScript version used HTML to avoid this. The Go version must implement an escaper that is applied to plain text, while entity content (bold, italic, code) uses correct entity syntax.

**Strategy:** Two-pass approach — (1) extract code blocks and inline code verbatim, (2) convert markdown formatting to MarkdownV2 entities, (3) escape special chars in all plain text regions, (4) restore code blocks wrapped in `` ` `` or ```` ``` ``` syntax.

**Fallback behavior (at Claude's discretion):** If `editMessageText` with `parse_mode: MarkdownV2` returns `Bad Request: can't parse entities`, retry the same call with `parse_mode` removed (plain text). Log the fallback. This matches the TypeScript HTML → plain text fallback pattern.

**Reference for special chars that MUST be escaped in MarkdownV2 plain text:**
```
_ * [ ] ( ) ~ ` > # + - = | { } . !
```
The escaping rule: prefix each with a backslash. Code spans (` ` `) and pre blocks (` ``` `) do NOT need escaping inside them.

### Pattern 7: Typing Indicator Goroutine

**What:** A goroutine loops calling `bot.SendChatAction` with "typing" every 4 seconds while Claude processes. Stop it via a channel signal.

**Example:**
```go
// internal/handlers/streaming.go
type TypingController struct {
    stop chan struct{}
}

func StartTypingIndicator(bot *gotgbot.Bot, chatID int64) *TypingController {
    tc := &TypingController{stop: make(chan struct{})}
    go func() {
        ticker := time.NewTicker(4 * time.Second)
        defer ticker.Stop()
        // Send immediately first
        _, _ = bot.SendChatAction(chatID, "typing", nil)
        for {
            select {
            case <-ticker.C:
                _, _ = bot.SendChatAction(chatID, "typing", nil)
            case <-tc.stop:
                return
            }
        }
    }()
    return tc
}

func (tc *TypingController) Stop() {
    close(tc.stop)
}
```

### Pattern 8: WaitDelay for Goroutine Leak Prevention

**What:** Set `WaitDelay` on every `exec.Cmd` before starting it. This caps how long `cmd.Wait()` will wait for I/O goroutines after the process exits, preventing indefinite blocking when the subprocess is killed.

**Example:**
```go
// internal/claude/process.go — during spawn
cmd := exec.CommandContext(ctx, claudePath, args...)
cmd.WaitDelay = 5 * time.Second  // available since Go 1.20
cmd.Stdin = promptReader
cmd.Stdout = stdoutPipe
cmd.Stderr = stderrPipe
// ... set env, cwd ...
```

### Anti-Patterns to Avoid

- **Global session singleton:** TypeScript uses one global `session` object — do NOT copy this. Use `SessionStore` keyed by channelID from day one.
- **Handler blocking on Claude query:** Never block the gotgbot handler goroutine waiting for Claude. Enqueue to session queue, return immediately.
- **Direct `os.WriteFile` for JSON persistence:** Corrupts on crash. Always use write-to-temp + rename.
- **`exec.LookPath` at query time for claude binary:** Silently fails in Windows Service. Resolve at startup, log result.
- **Sharing `StdoutPipe` across goroutines:** Single scanner goroutine owns stdout; separate goroutine owns stderr.
- **Skipping `delete(env, "CLAUDECODE")`:** Causes "nested session" error. The TypeScript does `delete env.CLAUDECODE` — replicate in Go: build env slice from `os.Environ()` then filter out `CLAUDECODE=*`.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Per-channel rate limiting | Custom token bucket | `golang.org/x/time/rate.NewLimiter` | Race conditions, edge cases with refill timing |
| Telegram Bot API types | Manual structs | gotgbot/v2 auto-generated types | Telegram API has 200+ types; generated code is correct |
| Subprocess stdout streaming | Manual `io.Read` loop | `bufio.Scanner` | Scanner handles line boundaries, buffer sizing, EOF |
| Atomic file writes | `os.WriteFile` | write-temp + `os.Rename` | Crash safety; direct write corrupts on interrupted write |
| Windows process kill | `cmd.Process.Kill()` | `taskkill /pid /T /F` | Kill() only kills top-level process; leaves Claude children running |
| Goroutine leak prevention | Manual pipe drain | `cmd.WaitDelay` | OS-level pipe semantics are complex; WaitDelay is the stdlib solution since Go 1.20 |

**Key insight:** The most dangerous hand-rolled solutions in this domain are subprocess management patterns — they work perfectly in happy-path tests but fail catastrophically under cancellation, crashes, and restarts.

---

## Common Pitfalls

### Pitfall 1: Windows Process Tree Orphaning

**What goes wrong:** `cmd.Process.Kill()` kills only the `cmd.exe` wrapper. The actual `claude.exe` process keeps running, accumulating across restarts. The TypeScript version already solved this — must replicate in Go.

**Why it happens:** Windows does not support POSIX process groups. `Kill()` sends `TerminateProcess` to one PID only.

**How to avoid:** `exec.Command("taskkill", "/pid", strconv.Itoa(pid), "/T", "/F").Run()` on Windows. Gate with `runtime.GOOS == "windows"`.

**Warning signs:** `claude.exe` processes visible in Task Manager after `/stop`; subsequent sessions fail to acquire locks.

### Pitfall 2: Concurrent Map Panic on SessionStore

**What goes wrong:** `fatal error: concurrent map read and map write` in production when two channels send messages simultaneously.

**Why it happens:** Go maps are not goroutine-safe. The TypeScript version is single-threaded (Node.js event loop) — no mutex was needed there. Go dispatches each Telegram update in its own goroutine.

**How to avoid:** `sync.RWMutex` protecting all map access from day one. Run all tests with `go test -race`.

**Warning signs:** Works with one channel, crashes with two simultaneous senders.

### Pitfall 3: Goroutine Leak from Claude CLI Pipes

**What goes wrong:** Killed subprocess leaves I/O goroutines blocked on pipe reads. Service memory grows across stop/start cycles until service becomes unresponsive.

**Why it happens:** `cmd.Wait()` blocks until all I/O goroutines finish. Goroutines wait for EOF that never comes on a half-open pipe.

**How to avoid:** `cmd.WaitDelay = 5 * time.Second` on every `exec.Cmd`. Also use `exec.CommandContext` so context cancellation propagates.

**Warning signs:** `runtime.NumGoroutine()` grows without bound; pprof goroutine dump shows blocked pipe readers.

### Pitfall 4: JSON Persistence Corruption

**What goes wrong:** Two goroutines both writing `session-history.json` produce an interleaved, corrupt file. On next restart, JSON parse fails and all session history is lost.

**Why it happens:** Go is concurrent by default. TypeScript's `writeFileSync` is safe because Node.js is single-threaded — cannot copy that assumption.

**How to avoid:** Single mutex per JSON file. Write-to-temp + `os.Rename`. Test with `go test -race`.

**Warning signs:** `json: unexpected end of JSON input` at startup; session list randomly empty after busy periods.

### Pitfall 5: Claude Session ID Stale After Context Limit

**What goes wrong:** Session ID stored in JSON references an invalidated Claude session. `--resume <old-id>` fails silently or with cryptic error. Bot appears broken.

**Why it happens:** Session IDs have no expiry signal — the only indication is error text from the CLI.

**How to avoid:** Implement `isContextLimitError(text string) bool` matching these patterns (from TypeScript):
```go
var contextLimitPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)input length and max_tokens exceed context limit`),
    regexp.MustCompile(`(?i)exceed context limit`),
    regexp.MustCompile(`(?i)context limit.*exceeded`),
    regexp.MustCompile(`(?i)prompt.*too.*long`),
    regexp.MustCompile(`(?i)conversation is too long`),
}
```
On detection: clear `sessionID` from memory and JSON persistence, notify user with recovery hint.

**Warning signs:** Bot "stuck" after long conversation; `--resume` always fails for a specific session ID.

### Pitfall 6: Service PATH Blindness

**What goes wrong:** Bot works interactively but fails as a Windows Service because `claude` is installed in a user-scoped directory (`%AppData%\npm`) invisible to the service account's stripped PATH.

**Why it happens:** Windows Service runs as SYSTEM or a service account with a minimal PATH. `exec.LookPath` finds nothing.

**How to avoid:** Resolve `CLAUDE_CLI_PATH` env var at startup → fallback to `exec.LookPath` → log resolved path. Never call `exec.LookPath` at query time.

**Warning signs:** Bot works with `go run .`, fails as service with "executable file not found."

### Pitfall 7: MarkdownV2 Parse Failures Silencing Responses

**What goes wrong:** A Claude response contains an unescaped special character (e.g., a bare `.` in a domain name, a bare `!` in an exclamation). Telegram returns `Bad Request: can't parse entities`. If there is no fallback, the user sees nothing.

**Why it happens:** MarkdownV2 is strict. The TypeScript version deliberately used HTML to avoid this. The Go version takes on this risk.

**How to avoid:** Implement a plain-text fallback: if `EditMessageText` with `ParseModeMarkdownV2` fails, retry with `ParseMode: ""` (no parse mode). Log the fallback occurrence.

**Warning signs:** Silent message failures; users report responses not appearing.

### Pitfall 8: `CLAUDECODE` Environment Variable Leaking Into Subprocess

**What goes wrong:** Claude CLI refuses to start with a "nested session" error because `CLAUDECODE` is set in the process environment, and the CLI detects it is running inside another Claude instance.

**Why it happens:** The Go bot process may inherit `CLAUDECODE` from its own environment if the developer runs it while Claude Code is active.

**How to avoid:** When building the subprocess environment, explicitly filter out any key starting with `CLAUDECODE`:
```go
env := os.Environ()
filtered := make([]string, 0, len(env))
for _, e := range env {
    if !strings.HasPrefix(e, "CLAUDECODE=") {
        filtered = append(filtered, e)
    }
}
cmd.Env = filtered
```

---

## Code Examples

Verified patterns from TypeScript functional spec (direct translation targets):

### NDJSON Event Types (from TypeScript session.ts)

```go
// internal/claude/events.go
type ClaudeEvent struct {
    Type      string          `json:"type"`       // "assistant", "result", "system"
    SessionID string          `json:"session_id"`
    Message   *AssistantMsg   `json:"message"`
    Result    string          `json:"result"`
    IsError   bool            `json:"is_error"`
    Subtype   string          `json:"subtype"`
    Error     string          `json:"error"`
    Usage     *UsageData      `json:"usage"`
    ModelUsage map[string]any `json:"modelUsage"`
}

type AssistantMsg struct {
    ID      string         `json:"id"`
    Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
    Type     string         `json:"type"`    // "text", "thinking", "tool_use"
    Text     string         `json:"text"`
    Thinking string         `json:"thinking"`
    ID       string         `json:"id"`      // tool_use block ID
    Name     string         `json:"name"`    // tool name
    Input    map[string]any `json:"input"`   // tool inputs
}

type UsageData struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}
```

### Claude CLI Spawn Arguments (from TypeScript session.ts)

```go
// internal/claude/process.go — BuildArgs mirrors TypeScript args construction
func BuildArgs(sessionID string, allowedPaths []string, model, systemPrompt string) []string {
    args := []string{
        "-p",
        "--verbose",
        "--output-format", "stream-json",
        "--include-partial-messages",
        "--dangerously-skip-permissions",
    }
    if len(allowedPaths) > 0 {
        args = append(args, "--add-dir")
        args = append(args, allowedPaths...)
    }
    if sessionID != "" {
        args = append(args, "--resume", sessionID)
    }
    if model != "" {
        args = append(args, "--model", model)
    }
    if systemPrompt != "" {
        args = append(args, "--append-system-prompt", systemPrompt)
    }
    return args
}
```

### Session History JSON Format (from TypeScript types.ts)

```go
// internal/session/persist.go
type SavedSession struct {
    SessionID  string `json:"session_id"`
    SavedAt    string `json:"saved_at"`   // ISO 8601
    WorkingDir string `json:"working_dir"`
    Title      string `json:"title"`      // First ~50 chars of first message
}

type SessionHistory struct {
    Sessions []SavedSession `json:"sessions"`
}
```

### Tool Emoji Map (from TypeScript formatting.ts)

```go
// internal/formatting/tools.go
var toolEmojiMap = map[string]string{
    "Read":      "📖",
    "Write":     "📝",
    "Edit":      "✏️",
    "Bash":      "▶️",
    "Glob":      "🔍",
    "Grep":      "🔎",
    "WebSearch": "🔍",
    "WebFetch":  "🌐",
    "Task":      "🎯",
    "TodoWrite": "📋",
    "mcp__":     "🔧",
}
```

### Rate Limiter (translating TypeScript security.ts)

```go
// internal/security/ratelimit.go — uses golang.org/x/time/rate
import "golang.org/x/time/rate"

type ChannelRateLimiter struct {
    mu       sync.Mutex
    limiters map[int64]*rate.Limiter
    limit    rate.Limit
    burst    int
}

func NewChannelRateLimiter(requestsPerWindow int, windowSeconds int) *ChannelRateLimiter {
    r := rate.Limit(float64(requestsPerWindow) / float64(windowSeconds))
    return &ChannelRateLimiter{
        limiters: make(map[int64]*rate.Limiter),
        limit:    r,
        burst:    requestsPerWindow,
    }
}

func (crl *ChannelRateLimiter) Allow(channelID int64) (bool, time.Duration) {
    crl.mu.Lock()
    l, ok := crl.limiters[channelID]
    if !ok {
        l = rate.NewLimiter(crl.limit, crl.burst)
        crl.limiters[channelID] = l
    }
    crl.mu.Unlock()

    r := l.Reserve()
    if !r.OK() {
        return false, 0
    }
    delay := r.Delay()
    if delay > 0 {
        r.Cancel()
        return false, delay
    }
    return true, 0
}
```

### Interrupt Prefix Handling (from TypeScript utils.ts + text.ts)

```go
// internal/handlers/text.go
func handleInterruptPrefix(text string, sess *session.Session) string {
    if !strings.HasPrefix(text, "!") {
        return text
    }
    stripped := strings.TrimSpace(text[1:])
    if sess.IsRunning() {
        sess.MarkInterrupt()
        sess.Stop() // sends to stopCh
    }
    return stripped
}
```

### /status Output Format (locked by CONTEXT.md)

The `/status` output must show:
1. Session state (active/none, session ID prefix)
2. Query state (running/idle, elapsed seconds, current tool)
3. Token usage: input / output / cache_read / cache_create
4. Context: `42%` (percentage only, no progress bar)
5. Working directory path
6. Uptime (time since session started or last activity)

```go
// internal/handlers/command.go — status dashboard
func buildStatusText(sess *session.Session, workingDir string) string {
    var sb strings.Builder

    // Session
    if sess.SessionID() != "" {
        fmt.Fprintf(&sb, "Session: Active (%s...)\n", sess.SessionID()[:8])
    } else {
        sb.WriteString("Session: None\n")
    }

    // Query
    if sess.IsRunning() {
        elapsed := time.Since(sess.QueryStarted()).Round(time.Second)
        fmt.Fprintf(&sb, "Query: Running (%s)\n", elapsed)
        if tool := sess.CurrentTool(); tool != "" {
            fmt.Fprintf(&sb, "  %s\n", tool)
        }
    } else {
        sb.WriteString("Query: Idle\n")
    }

    // Token usage
    if u := sess.LastUsage(); u != nil {
        fmt.Fprintf(&sb, "\nTokens: in=%d out=%d cache_read=%d cache_create=%d\n",
            u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens)
    }

    // Context
    if pct := sess.ContextPercent(); pct != nil {
        fmt.Fprintf(&sb, "Context: %d%%\n", *pct)
    }

    // Working dir
    fmt.Fprintf(&sb, "\nProject: %s\n", workingDir)

    return sb.String()
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| HTML parse_mode in Telegram | MarkdownV2 (this project's choice) | 2020 (Bot API 4.5) | Richer formatting but strict escaping required |
| `process.kill()` on Windows | `taskkill /T /F` | Always required | Kills process tree, not just top-level PID |
| `StdoutPipe()` + goroutines | `bufio.Scanner` on `Stdout` field | Go 1.20+ | Scanner is simpler; avoids race on StdoutPipe |
| Manual WaitGroup for subprocess | `cmd.WaitDelay` | Go 1.20 | Caps I/O goroutine wait, prevents goroutine leaks |
| Global session (TypeScript version) | Per-channel SessionStore (Go version) | This rewrite | Enables multi-project isolation |

**Deprecated/outdated:**
- `go-telegram-bot-api v5`: Last released December 2021, does not support Bot API 6.x+; do not use
- `tucnak/telebot`: Heavy framework opinions, fights streaming patterns; community has moved on
- `exec.LookPath` at query time: Use startup resolution + logging instead

---

## Open Questions

1. **MarkdownV2 escaping completeness**
   - What we know: The 19 special chars are documented by Telegram. Code blocks don't need escaping inside. Entity boundaries must be exact.
   - What's unclear: Whether Claude responses will contain edge cases like unmatched `**` in mid-word, backticks inside code blocks, or multi-level nested entities (bold inside italic) that MarkdownV2 doesn't support.
   - Recommendation: Implement a conservative escaper that escapes all 19 special chars in plain text regions, then test with a variety of real Claude responses. Have the plain-text fallback ready from day one.

2. **gotgbot/v2 callback data length limit**
   - What we know: Telegram limits callback_data to 64 bytes per button.
   - What's unclear: The `/resume` button format `resume:<session_id>` where session IDs are UUIDs (~36 chars) puts total at ~43 chars — within limit. Verify gotgbot enforces or silently truncates.
   - Recommendation: Use short prefixes and verify button payloads are ≤ 64 bytes in tests.

3. **Context percentage from `modelUsage` field**
   - What we know: TypeScript session.ts parses `event.modelUsage` from the `result` event. The field is `map[modelName]{ inputTokens, outputTokens, cacheReadInputTokens, cacheCreationInputTokens, contextWindow }`.
   - What's unclear: The field name and structure may vary across Claude CLI versions. The TypeScript accesses `Object.values(event.modelUsage)[0]` and `m.contextWindow`.
   - Recommendation: Parse defensively with type assertion checks; fall back to no context display if field is absent.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go's built-in `testing` package (no external framework needed) |
| Config file | none — `go test ./...` discovers tests automatically |
| Quick run command | `go test ./... -count=1` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CORE-05 | Rate limiter allows/blocks per channel | unit | `go test ./internal/security/... -run TestRateLimiter` | ❌ Wave 0 |
| CORE-06 | Audit log writes append-only entries | unit | `go test ./internal/audit/... -run TestAuditLog` | ❌ Wave 0 |
| SESS-01 | Claude CLI args built correctly | unit | `go test ./internal/claude/... -run TestBuildArgs` | ❌ Wave 0 |
| SESS-05 | Interrupt prefix strips `!` and stops running query | unit | `go test ./internal/handlers/... -run TestInterrupt` | ❌ Wave 0 |
| SESS-08 | Windows process kill uses taskkill | unit | `go test ./internal/claude/... -run TestKillProcess` | ❌ Wave 0 |
| AUTH-02 | Path validation allows/blocks correctly | unit | `go test ./internal/security/... -run TestPathValidation` | ❌ Wave 0 |
| PERS-01 | Session save/load round-trips cleanly | unit | `go test ./internal/session/... -run TestPersistence` | ❌ Wave 0 |
| PERS-03 | Concurrent saves don't corrupt JSON | unit | `go test ./internal/session/... -run TestConcurrentSave -race` | ❌ Wave 0 |
| CMD-04 | Status output contains all required fields | unit | `go test ./internal/handlers/... -run TestStatusFormat` | ❌ Wave 0 |
| CORE-02 | Config loads CLAUDE_CLI_PATH from env | unit | `go test ./internal/config/... -run TestConfig` | ❌ Wave 0 |
| SESS-02 | Streaming throttle respects 500ms minimum | unit | `go test ./internal/handlers/... -run TestStreamThrottle` | ❌ Wave 0 |
| DEPLOY-03 | Path resolution logs at startup | integration | Manual: run bot, check log output | manual-only — requires claude binary |

### Sampling Rate

- **Per task commit:** `go test ./... -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** All tests green, zero race conditions, before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/security/ratelimit_test.go` — covers CORE-05
- [ ] `internal/security/validate_test.go` — covers AUTH-02, AUTH-03
- [ ] `internal/audit/log_test.go` — covers CORE-06
- [ ] `internal/claude/args_test.go` — covers SESS-01
- [ ] `internal/claude/process_test.go` — covers SESS-08 (Windows process kill, goroutine leak)
- [ ] `internal/session/persist_test.go` — covers PERS-01, PERS-03 (atomic write, concurrent save)
- [ ] `internal/session/store_test.go` — covers CORE-01 (concurrent map safety)
- [ ] `internal/handlers/command_test.go` — covers CMD-04 (status format)
- [ ] `internal/handlers/streaming_test.go` — covers SESS-02, SESS-03 (throttle, tool display)
- [ ] `internal/config/config_test.go` — covers CORE-02 (env parsing)
- [ ] `go.mod` / `go.sum` — no existing Go module; must be initialized in Wave 0

---

## Sources

### Primary (HIGH confidence)
- Existing TypeScript codebase (`src/session.ts`, `src/handlers/streaming.ts`, `src/handlers/commands.ts`, `src/handlers/text.ts`, `src/security.ts`, `src/formatting.ts`, `src/config.ts`, `src/utils.ts`, `src/types.ts`) — complete functional specification; all NDJSON event types, streaming patterns, command outputs, error messages, and session persistence formats verified from working code
- `.planning/research/SUMMARY.md` — stack decisions, architecture approach, pitfall catalogue (all HIGH confidence)
- `.planning/research/ARCHITECTURE.md` — component diagrams, data flow, anti-patterns (all HIGH confidence)
- `.planning/research/STACK.md` — library versions, alternatives rationale (all HIGH confidence)
- `.planning/research/PITFALLS.md` — eight pitfalls with sources (HIGH confidence)
- [Telegram MarkdownV2 documentation](https://core.telegram.org/bots/api#markdownv2-style) — special characters requiring escaping
- [Go 1.20 release notes — cmd.WaitDelay](https://go.dev/doc/go1.20) — WaitDelay field confirmed in stdlib since 1.20
- [gotgbot v2 pkg.go.dev](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2) — dispatcher model, per-update goroutines

### Secondary (MEDIUM confidence)
- [golang.org/x/time/rate pkg.go.dev](https://pkg.go.dev/golang.org/x/time/rate) — token bucket API, `Reserve()` + `Delay()` pattern
- [Go os/exec issue #23019](https://github.com/golang/go/issues/23019) — goroutine leak via uncleaned subprocess pipes
- [Killing child processes in Go](https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773) — process group kill patterns

### Tertiary (LOW confidence)
- None — all critical claims verified from primary or secondary sources

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries verified via pkg.go.dev as of 2026-03-19
- Architecture: HIGH — derived from working TypeScript codebase as functional spec; Go patterns verified against stdlib documentation
- Pitfalls: HIGH — most verified against Go issue tracker, official docs, and existing TypeScript solutions that already solved the same problems
- MarkdownV2 escaping: MEDIUM — strategy is documented by Telegram, but edge cases with Claude's output patterns require empirical testing

**Research date:** 2026-03-19
**Valid until:** 2026-06-19 (90 days; Go stdlib is stable; gotgbot RC tag has been stable for 2+ years)
