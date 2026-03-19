# Architecture Research

**Domain:** Go Telegram Bot with Concurrent Subprocess Management (Multi-Project Claude Code Control)
**Researched:** 2026-03-19
**Confidence:** HIGH — derived from existing TypeScript codebase (functional spec), Go stdlib patterns, and gotgbot library docs

## Standard Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Telegram Layer                                │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  gotgbot Updater + Dispatcher (long-poll, goroutine/update) │    │
│  └──────────────────────────┬──────────────────────────────────┘    │
└─────────────────────────────│───────────────────────────────────────┘
                              │ Update (message, callback, voice, photo…)
┌─────────────────────────────▼───────────────────────────────────────┐
│                        Router / Middleware Layer                      │
│  ┌────────────────┐  ┌───────────────┐  ┌───────────────────────┐   │
│  │  Auth (channel │  │  Rate Limiter │  │  Channel→Project      │   │
│  │  membership)   │  │  (per channel)│  │  Resolver             │   │
│  └────────────────┘  └───────────────┘  └───────────────────────┘   │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ Resolved project context
┌──────────────────────────────▼──────────────────────────────────────┐
│                        Handler Layer                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐            │
│  │  Command │  │   Text   │  │  Media   │  │ Callback │            │
│  │ Handlers │  │ Handler  │  │ Handlers │  │ Handler  │            │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘            │
└───────│─────────────│─────────────│──────────────│──────────────────┘
        └─────────────┴─────────────┘              │
                      │ Send(message, channelID)    │
┌─────────────────────▼───────────────────────────▼──────────────────┐
│                        Session Manager                               │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  SessionStore  (sync.RWMutex + map[channelID]*Session)       │   │
│  └────────────────────────┬─────────────────────────────────────┘   │
│                           │ Get/Create session by channelID          │
│  ┌────────────────────────▼─────────────────────────────────────┐   │
│  │  Session (one per channel/project)                           │   │
│  │   • sessionID string (Claude resume token)                   │   │
│  │   • workingDir string                                        │   │
│  │   • isRunning bool                                           │   │
│  │   • msgQueue chan QueuedMessage (buffered)                   │   │
│  │   • stopCh chan struct{}                                     │   │
│  └────────────────────────┬─────────────────────────────────────┘   │
└────────────────────────────│────────────────────────────────────────┘
                             │ exec.Cmd (claude CLI subprocess)
┌────────────────────────────▼────────────────────────────────────────┐
│                        Subprocess Layer                              │
│  ┌────────────────────────────────────────────────────────────┐     │
│  │  ClaudeProcess (per active query)                          │     │
│  │   • stdin ← prompt text                                    │     │
│  │   • stdout → NDJSON line reader goroutine                  │     │
│  │   • stderr → error accumulator goroutine                   │     │
│  │   • Windows: taskkill /T /F for process tree kill          │     │
│  └────────────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
                             │ Streaming events (text, tool, done)
┌────────────────────────────▼────────────────────────────────────────┐
│                        Streaming / Response Layer                    │
│  ┌────────────────────────────────────────────────────────────┐     │
│  │  StreamingState (per active query, not shared)             │     │
│  │   • statusMsg *Message    (tool/thinking ephemeral msg)    │     │
│  │   • textSegments map      (segment → Telegram message)     │     │
│  │   • throttle ticker       (500ms edit rate limiter)        │     │
│  └────────────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
                             │ Telegram API calls (send/edit/delete)
┌────────────────────────────▼────────────────────────────────────────┐
│                        Persistence Layer                             │
│  ┌─────────────────────┐  ┌──────────────────────────────────┐      │
│  │  channel-projects   │  │  session-history.json            │      │
│  │  .json              │  │  (sessionID, workingDir, title)  │      │
│  │  (channelID →       │  │                                  │      │
│  │   project path)     │  │                                  │      │
│  └─────────────────────┘  └──────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| gotgbot Updater/Dispatcher | Receive Telegram updates; dispatch each in own goroutine | `ext.NewUpdater` + `ext.NewDispatcher` from gotgbot v2 |
| Channel→Project Resolver | Map incoming channelID to a registered project path; prompt for registration if unknown | Middleware reading `channel-projects.json` |
| Auth Middleware | Verify the sender is a member of the channel (per-channel auth, not global allowlist) | Check `ctx.EffectiveSender` against channel ID |
| Rate Limiter | Token-bucket per channelID; reject excess within window | `sync.Map` of per-channel buckets, goroutine-safe |
| Session Manager (SessionStore) | Own and lifecycle-manage all per-project sessions; thread-safe map lookup | `sync.RWMutex` + `map[int64]*Session` |
| Session | Represent one project's Claude state: sessionID, workingDir, queue, stop signal | Struct with mutex, channel-based queue, context for cancellation |
| ClaudeProcess | Spawn and stream `claude` CLI; parse NDJSON from stdout; pipe prompt via stdin | `os/exec.Cmd`, `bufio.Scanner` on stdout, separate stderr goroutine |
| StreamingState | Track live Telegram messages per streaming query segment | Struct with message IDs, last-edit timestamps, throttle logic |
| Persistence (JSON files) | Load/save channel-project mappings and session history on disk | Atomic file writes with `encoding/json` |
| Audit Logger | Append-only log of all messages and actions | Goroutine-safe buffered writer to file |
| Media Handlers | Transcribe voice (OpenAI), extract PDFs (pdftotext), buffer photo albums | Handler functions per media type, 1s album buffer with timer |
| GSD Command Handler | Parse GSD roadmap phases, build inline keyboard menus for all /gsd: commands | Dynamic keyboard builder from ROADMAP.md parsing |
| Windows Service Entrypoint | Implement `svc.Handler` interface; route STOP/SHUTDOWN signals to shutdown channel | `golang.org/x/sys/windows/svc` |

## Recommended Project Structure

```
.
├── main.go                   # Entry point: Windows Service vs direct run
├── cmd/
│   └── bot/
│       └── main.go           # Alternate: single binary with subcommands
├── internal/
│   ├── bot/
│   │   ├── bot.go            # Bot struct, startup, shutdown
│   │   ├── middleware.go     # Auth, rate-limit, channel-resolver middleware chain
│   │   └── handlers.go       # Handler registration (wires all handlers to dispatcher)
│   ├── handlers/
│   │   ├── command.go        # /start /new /stop /status /resume /retry /gsd
│   │   ├── text.go           # Text message handler
│   │   ├── voice.go          # Voice → OpenAI transcription → text handler
│   │   ├── photo.go          # Photo / album buffering
│   │   ├── document.go       # PDF, text, archive files (routes audio docs to audio.go)
│   │   ├── audio.go          # Audio file transcription
│   │   ├── video.go          # Video messages
│   │   ├── callback.go       # Inline keyboard callbacks (ask_user, action menus, pickers)
│   │   └── streaming.go      # StreamingState struct + status callback factory
│   ├── session/
│   │   ├── store.go          # SessionStore: thread-safe map[channelID]*Session
│   │   ├── session.go        # Session struct: queue, stop, state
│   │   └── persist.go        # Save/load session-history.json
│   ├── claude/
│   │   ├── process.go        # Spawn claude CLI, stream NDJSON stdout, kill tree
│   │   └── events.go         # NDJSON event types: assistant, result, tool_use etc.
│   ├── registry/
│   │   ├── registry.go       # channel-projects.json: channelID ↔ projectPath
│   │   └── prompt.go         # "New channel — link a project?" flow
│   ├── gsd/
│   │   ├── roadmap.go        # Parse ROADMAP.md phases
│   │   ├── commands.go       # Extract /gsd: commands from text
│   │   └── keyboard.go       # Build inline keyboards for GSD menus
│   ├── formatting/
│   │   ├── markdown.go       # Markdown → Telegram HTML conversion
│   │   └── tools.go          # Tool status emoji formatting
│   ├── security/
│   │   ├── ratelimit.go      # Token bucket per channelID
│   │   └── validate.go       # Path validation, command safety checks
│   ├── media/
│   │   ├── voice.go          # OpenAI Whisper transcription
│   │   └── pdf.go            # pdftotext CLI wrapper
│   ├── audit/
│   │   └── log.go            # Append-only audit log
│   └── config/
│       └── config.go         # ENV parsing, constants
├── svc/
│   ├── service.go            # Windows Service svc.Handler implementation
│   └── install.go            # Install/uninstall helper (optional CLI flag)
└── data/                     # Runtime JSON files (gitignored)
    ├── channel-projects.json
    └── session-history.json
```

### Structure Rationale

- **internal/**: All packages private to the binary — enforces clean boundaries, prevents accidental external imports.
- **internal/claude/**: Isolated subprocess management. Nothing else touches `os/exec`. Produces typed events on a channel.
- **internal/session/**: Owns all concurrency over session state. Handlers never directly mutate session fields — they call methods on `SessionStore` or `Session`.
- **internal/handlers/**: Pure Telegram update handlers. Each file handles one update type. They call `session.Store`, `claude.Process`, and formatting helpers — no business logic lives here.
- **internal/gsd/**: GSD-specific logic isolated so it can evolve independently as GSD commands change.
- **svc/**: Windows-specific code isolated from the main application loop. `main.go` conditionally calls `svc.Run` or runs directly based on OS/flags.

## Architectural Patterns

### Pattern 1: Channel-per-Session Message Queue

**What:** Each `Session` owns a buffered `chan QueuedMessage` (capacity 5). A single goroutine per session drains the queue, running one Claude query at a time. New messages are enqueued; if full, the sender receives a "busy" reply.

**When to use:** Wherever a single Claude CLI process must be serialised per project, while the bot continues accepting Telegram updates concurrently.

**Trade-offs:** Simple and backpressure-safe. No mutex coordination needed inside the worker loop. The downside is that a slow Claude query blocks subsequent messages to that channel, which is the desired behaviour for this use case.

**Example:**
```go
// internal/session/session.go
type Session struct {
    mu          sync.Mutex
    sessionID   string
    workingDir  string
    queue       chan QueuedMessage  // buffered, capacity MaxQueueSize
    stopCh      chan struct{}
    cancelQuery context.CancelFunc // cancel running claude process
}

// Worker goroutine started once per session
func (s *Session) worker(ctx context.Context, store *SessionStore) {
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

### Pattern 2: NDJSON Event Streaming via Scanner + Status Callback

**What:** The Claude CLI emits NDJSON on stdout. A goroutine reads line-by-line with `bufio.Scanner`, decodes each JSON event, and calls a `StatusCallback` function with typed event data. The callback drives Telegram message editing.

**When to use:** Any long-running subprocess that emits structured line-delimited output.

**Trade-offs:** Clean separation between "what Claude said" and "how to display it in Telegram." The callback function is injected per query, making it testable. The scanner goroutine is simple and avoids the race conditions that arise from `StdoutPipe` + manual goroutines.

**Example:**
```go
// internal/claude/process.go
func (p *Process) Stream(ctx context.Context, cb StatusCallback) error {
    scanner := bufio.NewScanner(p.cmd.Stdout)
    stderrDone := make(chan struct{})
    go func() {
        defer close(stderrDone)
        scanStderr(p.cmd.Stderr, &p.stderrBuf)
    }()

    for scanner.Scan() {
        if ctx.Err() != nil {
            break
        }
        var event ClaudeEvent
        if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
            continue
        }
        cb(event)
    }
    <-stderrDone
    return p.cmd.Wait()
}
```

### Pattern 3: sync.RWMutex SessionStore

**What:** `SessionStore` is a struct with `sync.RWMutex` protecting a `map[int64]*Session`. Read operations (lookup) use `RLock`; write operations (create, delete) use `Lock`. Session-internal state is protected by the session's own mutex, so the store lock is held only briefly.

**When to use:** Multiple goroutines (one per Telegram update) need concurrent read access to sessions, with rare writes (session creation/deletion).

**Trade-offs:** Lower contention than a full `sync.Map` for this access pattern (infrequent writes, frequent reads). `sync.Map` would also work but provides less type safety without generics wrappers.

**Example:**
```go
// internal/session/store.go
type SessionStore struct {
    mu       sync.RWMutex
    sessions map[int64]*Session  // keyed by Telegram channelID
}

func (s *SessionStore) Get(channelID int64) (*Session, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    sess, ok := s.sessions[channelID]
    return sess, ok
}

func (s *SessionStore) GetOrCreate(channelID int64, cfg SessionConfig) *Session {
    s.mu.Lock()
    defer s.mu.Unlock()
    if sess, ok := s.sessions[channelID]; ok {
        return sess
    }
    sess := newSession(cfg)
    s.sessions[channelID] = sess
    return sess
}
```

### Pattern 4: Windows Process Tree Kill

**What:** On Windows, `claude` CLI spawns through `cmd.exe` when `shell: true` is set. Killing only the top-level PID leaves child processes running. Use `taskkill /pid <PID> /T /F` to kill the entire process tree.

**When to use:** Any subprocess spawned with `SysProcAttr` on Windows, especially shell-wrapped commands.

**Trade-offs:** `taskkill` is Windows-only — gated behind `runtime.GOOS == "windows"`. On Linux/macOS, `cmd.Process.Signal(syscall.SIGTERM)` to the process group works instead. Since this project targets Windows exclusively, the Windows path is the primary case.

**Example:**
```go
// internal/claude/process.go
func (p *Process) Kill() error {
    if runtime.GOOS == "windows" {
        pid := strconv.Itoa(p.cmd.Process.Pid)
        return exec.Command("taskkill", "/pid", pid, "/T", "/F").Run()
    }
    return p.cmd.Process.Signal(syscall.SIGTERM)
}
```

## Data Flow

### Inbound Message Flow (Text/Voice/Photo → Claude → Streaming Response)

```
Telegram Update arrives
    ↓
gotgbot Dispatcher (spawns goroutine per update)
    ↓
Middleware chain: Auth → RateLimit → ChannelResolver
    ↓ (project context attached)
Handler (text.go / voice.go / photo.go)
    ↓ (media preprocessing: transcription, pdf extract, album buffer)
session.Store.GetOrCreate(channelID)
    ↓
session.Enqueue(QueuedMessage)
    ↓ (returns immediately with "queued" reply if already running)
Session.worker goroutine (serial per session)
    ↓
claude.NewProcess(prompt, sessionID, workingDir, args)
    ↓ stdin ← prompt text
ClaudeProcess.Stream(ctx, statusCallback)
    ↓ stdout → bufio.Scanner → NDJSON events
statusCallback(event)
    ↓
    ├─ event.type == "assistant" / text block
    │       ↓ throttled
    │   StreamingState.EditOrSendSegment(segmentID, content)
    │       ↓
    │   bot.EditMessageText / bot.SendMessage
    │
    ├─ event.type == "assistant" / tool_use block
    │       ↓
    │   StreamingState.EditStatusMessage(toolDisplay)
    │       ↓
    │   bot.EditMessageText (status msg)
    │
    └─ event.type == "result"
            ↓
        session.UpdateSessionID(event.session_id)
        session.persist.Save()
            ↓
        StreamingState.FinalizeSegments()  (delete status msg, send final text)
```

### Channel Registration Flow (New/Unknown Channel)

```
Message arrives from unknown channelID
    ↓
ChannelResolver: no mapping found
    ↓
registry.Prompt(ctx): send inline keyboard "Link a project?"
    ↓ user taps project or types path
callback.go receives callbackQuery
    ↓
registry.Save(channelID, projectPath)  → channel-projects.json
    ↓
session.Store.GetOrCreate(channelID, {workingDir: projectPath})
    ↓
Proceed as normal message
```

### Stop / Interrupt Flow

```
/stop command or "!" prefix message
    ↓
handler calls session.Stop(channelID)
    ↓
Session.stopCh ← signal
    ↓ (worker goroutine reads stopCh)
Session.cancelQuery()  → context.CancelFunc for running claude.Process
    ↓
claude.Process.Kill()  → taskkill /T /F
    ↓
worker loop: query returns, dequeue next or wait
```

### State Management

```
Persistent State (JSON files)
    channel-projects.json    ← written on registration
    session-history.json     ← written when sessionID is received from CLI
    state.json               ← written when workingDir changes

In-Memory State (SessionStore)
    map[channelID]*Session
        sessionID
        workingDir
        isRunning
        queue (channel)
        lastActivity
        lastError

Per-Query Ephemeral State (StreamingState)
    map[segmentID]MessageID  ← Telegram messages created during streaming
    statusMsg MessageID      ← single tool/thinking status message
    throttle timers
    ↓ (discarded after query completes)
```

## Build Order (Phase Dependencies)

The component dependencies create a natural build order for the roadmap:

1. **Core infrastructure** — Config parsing, audit logging, JSON persistence helpers. No dependencies; everything else depends on these.

2. **Claude subprocess layer** (`internal/claude/`) — Process spawning, NDJSON streaming, kill. Depends only on stdlib. Must be built before any handler can send messages to Claude.

3. **Session management** (`internal/session/`) — SessionStore + Session + worker goroutine. Depends on the Claude subprocess layer for running queries.

4. **Telegram bot skeleton** — Bot startup, gotgbot updater, middleware chain (auth + rate limit + channel resolver). Requires registry (persistence). Can be built without handlers — just a /start echo to confirm connectivity.

5. **Streaming response layer** (`internal/handlers/streaming.go`) — StreamingState + status callback. Depends on both the session layer and the bot API. Once this works, real streaming is possible.

6. **Core handlers** — Text, commands (/new /stop /status /resume). Depend on session + streaming. This is the first fully usable version.

7. **Media handlers** — Voice, photo, document, audio, video. Each is independent once core handlers work. Voice depends on OpenAI Whisper API; PDFs depend on pdftotext CLI.

8. **GSD integration** — Roadmap parsing, GSD command extraction, dynamic inline keyboards. Depends on handlers and callback infrastructure.

9. **Windows Service wrapper** — `svc/service.go`. Depends on the full bot being functional. Can be wired in last without affecting any other component.

## Scaling Considerations

This bot is designed for personal/small-team use — one binary, one operator, multiple projects. Scaling concerns are minimal but the following deserve attention:

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 1-5 projects, 1 operator | Current design is sufficient. All state in memory + JSON files. |
| 10-20 projects | Add a max-sessions cap (already in TypeScript at MAX_SESSIONS=5). Session GC for inactive projects (LRU eviction from SessionStore). |
| 20+ concurrent active Claude queries | The bottleneck is Claude CLI processes (CPU, API tokens), not bot architecture. No structural changes needed. |

### Scaling Priorities

1. **First bottleneck:** Claude CLI spawning too many concurrent processes. Prevention: cap concurrent active queries with a semaphore (`chan struct{}` with capacity N) in SessionStore.
2. **Second bottleneck:** Telegram API rate limits when editing messages too frequently during streaming. Prevention: existing throttle (500ms minimum between edits) + auto-retry on 429.

## Anti-Patterns

### Anti-Pattern 1: Global Session Singleton

**What people do:** Have a single global `session` variable (as the TypeScript version does), adequate for single-project use.

**Why it's wrong:** The Go rewrite must support multiple concurrent projects. A global session cannot be keyed by channelID — all projects would share one Claude process slot.

**Do this instead:** `SessionStore` with `map[int64]*Session` keyed by channelID. Each channel gets its own independent session, worker goroutine, and subprocess.

### Anti-Pattern 2: Sharing StdoutPipe Across Goroutines

**What people do:** Open `cmd.StdoutPipe()`, then read from it in multiple goroutines to get parallelism.

**Why it's wrong:** `StdoutPipe` returns a single `io.ReadCloser` — concurrent reads race. Additionally, `cmd.Wait()` closes the pipe before all reads complete if not synchronised correctly.

**Do this instead:** A single goroutine owns the `bufio.Scanner` on stdout. Parsed events are sent to a typed Go channel or processed via callback. A separate goroutine handles stderr independently using its own scanner.

### Anti-Pattern 3: Blocking Handler Goroutines on Claude Queries

**What people do:** Start a Claude query directly inside the Telegram update handler goroutine and `await`/block until it finishes.

**Why it's wrong:** gotgbot runs each update in its own goroutine, which is fine — but if the handler blocks for 30-120 seconds on a Claude response, the goroutine is held for that duration. More importantly, subsequent messages to the same channel spawn additional handler goroutines that also try to start Claude queries, creating concurrency conflicts.

**Do this instead:** Handlers enqueue messages to `Session.queue` (a buffered channel) and return immediately. A single long-running worker goroutine per session drains the queue. Multiple incoming messages from one channel are naturally serialised.

### Anti-Pattern 4: Storing Auth State in Session

**What people do:** Store "is this user authorised?" flags inside the session struct, and check/update them per-message.

**Why it's wrong:** Auth is per-channel membership, not per-session. Sessions are created after auth passes. Mixing the two adds unnecessary state and makes auth logic hard to reason about.

**Do this instead:** Auth middleware runs before any session lookup. If auth fails, the handler is never called and no session is touched. The session struct contains only Claude-specific state.

### Anti-Pattern 5: Atomic Writes Skipped for JSON Persistence

**What people do:** `os.WriteFile(path, data, 0644)` directly, overwriting the file in place.

**Why it's wrong:** If the process crashes mid-write, the JSON file is corrupted. On Windows, `os.Rename` over an existing file works atomically (unlike some Unix implementations).

**Do this instead:** Write to a temp file in the same directory, then `os.Rename(tmpPath, targetPath)`. Add a `sync.Mutex` per JSON file to serialise concurrent writers.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Telegram Bot API | gotgbot v2 long-poll updater | Per-update goroutines; auto-retry on 429/5xx |
| Claude CLI (`claude`) | `os/exec.Cmd` subprocess, stdin/stdout pipe | NDJSON stream-json output format; `--resume` for session continuity |
| OpenAI Whisper API | HTTP POST to `api.openai.com/v1/audio/transcriptions` | Download voice file from Telegram, post multipart/form-data |
| pdftotext CLI | `os/exec.Command("pdftotext", "-", "-")` | stdin/stdout mode avoids temp files; must be on PATH |
| Windows Service Manager | `golang.org/x/sys/windows/svc` | Implement `svc.Handler` Execute method; route STOP/SHUTDOWN to `context.CancelFunc` |
| GSD toolchain | Reads `.planning/ROADMAP.md` via `os.ReadFile` | No subprocess — pure file parsing |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| Handler → SessionStore | Direct method call (`store.GetOrCreate`, `store.Get`) | Handler holds no reference to Session internals |
| SessionStore → Session | Returns `*Session`; callers use Session API methods only | Session fields are unexported; mutations via methods |
| Session.worker → claude.Process | Creates Process per query; blocks on `Process.Stream()` | Worker goroutine owns the Process lifetime |
| claude.Process → StatusCallback | Function call per parsed event | Callback is injected at query time; carries StreamingState closure |
| StatusCallback → Telegram API | gotgbot `bot.EditMessageText` / `bot.SendMessage` | All Telegram calls are in the callback, not in the process layer |
| registry ↔ JSON file | Read on startup + cache in memory; write on mutation with mutex | All writes are atomic (write-rename pattern) |
| audit.Log | Goroutine-safe buffered writer; other packages call `audit.Log(...)` | Single global logger, append-only |

## Sources

- [gotgbot v2 package docs](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2) — updater/dispatcher pattern, per-update goroutines, type-safe API
- [go-cmd/cmd library](https://github.com/go-cmd/cmd) — thread-safe subprocess streaming patterns for Go
- [DoltHub: Go os/exec patterns](https://www.dolthub.com/blog/2022-11-28-go-os-exec-patterns/) — `StdoutPipe` vs scanner vs `CommandContext` patterns
- [golang.org/x/sys/windows/svc example](https://pkg.go.dev/golang.org/x/sys/windows/svc/example) — Windows service implementation pattern
- Existing TypeScript implementation (`src/session.ts`, `src/handlers/streaming.ts`) — functional specification; Go rewrite preserves all behaviours

---
*Architecture research for: Go Telegram Bot with Multi-Project Claude Code Subprocess Management*
*Researched: 2026-03-19*
