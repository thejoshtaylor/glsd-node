# Architecture Research

**Domain:** Go node software — CLI subprocess manager with outbound WebSocket transport
**Researched:** 2026-03-20
**Confidence:** HIGH

---

## The Core Transformation

The v1.1 codebase has a clean layer separation that makes this transformation tractable. The key insight: the `claude` and `session` packages are already transport-agnostic. They know nothing about Telegram. Only the `handlers`, `bot`, and `formatting` packages are Telegram-coupled.

The transformation is: **replace the `bot` layer with a `node` layer** that connects outbound via WebSocket instead of polling Telegram. Everything below `bot` — `session`, `claude`, `project`, `security`, `audit` — survives with minimal modification.

---

## What Stays vs What Changes

### Packages That Survive Intact

| Package | Status | Notes |
|---------|--------|-------|
| `internal/claude` | Unchanged | Process, events, BuildArgs — fully transport-agnostic |
| `internal/session` | Modified | Session/Worker logic survives; QueuedMessage changes slightly; store key type changes |
| `internal/project` | Mostly unchanged | MappingStore survives; key type changes from channelID int64 to projectName string |
| `internal/security` | Mostly unchanged | ValidatePath, CheckCommandSafety survive; ChannelRateLimiter stays but keyed by project |
| `internal/audit` | Unchanged | Logger stays as-is |
| `internal/config` | Significant change | Remove Telegram fields; add WebSocket server URL, node identity fields |
| `internal/formatting` | Reduced | Strip Telegram-specific formatting (MarkdownV2, HTML); keep tool status strings for heartbeat |

### Packages Removed

| Package | Reason |
|---------|--------|
| `internal/handlers` (all Telegram handlers) | All Telegram handler logic deleted — replaced by dispatch layer |
| `internal/bot` | Replaced by `internal/node` |
| `github.com/PaulSonOfLars/gotgbot/v2` | Removed from go.mod entirely |

### Packages Added

| Package | Purpose |
|---------|---------|
| `internal/node` | Top-level node lifecycle — replaces `internal/bot` |
| `internal/wsconn` | Outbound WebSocket connection manager with reconnect |
| `internal/protocol` | Message type definitions for the wire protocol |
| `internal/dispatch` | Routes incoming server commands to session/project handlers |

---

## System Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         Central Server                           │
│                   (separate repo / future work)                  │
└──────────────────────────┬───────────────────────────────────────┘
                           │  WebSocket (outbound from node)
                           │  wss://server.example.com/nodes/ws
                           │
┌──────────────────────────▼───────────────────────────────────────┐
│                        GSD Node Process                          │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                      internal/node                          │ │
│  │  Node struct — lifecycle orchestrator                       │ │
│  │  Owns wsconn, dispatch, project store, instance store       │ │
│  │  Manages graceful shutdown via context cancellation         │ │
│  │  Sends heartbeats to server via wsconn                      │ │
│  └────────────────────────┬────────────────────────────────────┘ │
│                           │                                      │
│  ┌────────────────────────▼────────────────────────────────────┐ │
│  │                     internal/wsconn                         │ │
│  │  ConnectionManager — outbound WebSocket lifecycle           │ │
│  │  Dial with exponential backoff (500ms to 30s max)           │ │
│  │  Read goroutine: incoming msgs → dispatch.recvCh            │ │
│  │  Write goroutine: drains sendCh → conn.Write                │ │
│  │  Ping/pong keepalive (30s interval)                         │ │
│  │  Reconnect on any error; re-register node after reconnect   │ │
│  └───────┬──────────────────────────────────┬──────────────────┘ │
│          │ incoming msgs                    │ outgoing msgs       │
│  ┌───────▼────────────────────────┐         │                    │
│  │      internal/dispatch         │         │                    │
│  │  Dispatcher — routes commands  │         │                    │
│  │  execute    → instance layer   │         │                    │
│  │  stop       → instance.Stop()  │         │                    │
│  │  new_session → clear sessionID │         │                    │
│  │  status     → instance.Status  │         │                    │
│  │  project_link → mappings.Set   │         │                    │
│  └───────┬────────────────────────┘         │                    │
│          │                                  │                    │
│  ┌───────▼────────────────────────────────────────────────────┐  │
│  │             internal/session (modified)                    │  │
│  │                                                            │  │
│  │  InstanceStore: map[instanceID string]*Instance            │  │
│  │                                                            │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │  │
│  │  │  Instance A  │  │  Instance B  │  │  Instance C  │     │  │
│  │  │  project: X  │  │  project: X  │  │  project: Y  │     │  │
│  │  │  worker goroutine (serial queue per instance)     │     │  │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘     │  │
│  └─────────┼─────────────────┼─────────────────┼─────────────┘  │
│            │                 │                 │                 │
│  ┌─────────▼─────────────────▼─────────────────▼─────────────┐  │
│  │              internal/claude (unchanged)                   │  │
│  │  Process — exec.Cmd wrapping claude CLI subprocess         │  │
│  │  Stream() — NDJSON event reader → StatusCallback           │  │
│  │  BuildArgs() — session resume, allowed paths, model        │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐   │
│  │   project/   │  │  security/   │  │       audit/         │   │
│  │  MappingStore│  │  ValidatePath│  │      Logger          │   │
│  │ (key: string)│  │  RateLimiter │  │    (unchanged)       │   │
│  └──────────────┘  └──────────────┘  └──────────────────────┘   │
│                                                                  │
│  data/                                                           │
│  ├── mappings.json        (project name → local path)           │
│  └── session-history.json (instance session IDs for resume)     │
└──────────────────────────────────────────────────────────────────┘
```

---

## Component Decomposition

### internal/node — Lifecycle Orchestrator

**Replaces:** `internal/bot`

**Responsibility:** Own the node's full lifecycle. Wire together wsconn, dispatch, instance store, project mappings, audit log. Handle startup (send registration), run heartbeat, shutdown (drain workers, close connection).

**Key struct:**
```go
type Node struct {
    cfg       *config.Config
    conn      *wsconn.ConnectionManager
    dispatch  *dispatch.Dispatcher
    instances *session.InstanceStore
    mappings  *project.MappingStore
    persist   *session.PersistenceManager
    auditLog  *audit.Logger
    wg        sync.WaitGroup
}
```

**Startup sequence:**
1. Load config, initialize stores, open audit log
2. Start `wsconn.ConnectionManager` — this dials the server with reconnect loop
3. On first successful connect: send `node_register` with node ID, version, project list
4. Start heartbeat ticker (30s interval, sends `node_status`)
5. Block on context cancellation (OS signal)
6. Drain: cancel context, wait for all instance workers, close wsconn

**Differences from `internal/bot`:**
- No gotgbot dependency
- No polling loop — instead, a connection that receives commands
- No Telegram API rate limiter (global 25 edits/sec) — not applicable
- Session restore on startup is the same: load persisted sessions, recreate instances

---

### internal/wsconn — Connection Manager

**New package** (no equivalent in v1.1)

**Responsibility:** Own the outbound WebSocket connection lifecycle. Reconnect automatically on any failure. Serialize all writes through a single goroutine. Expose `Send([]byte) error` safe for concurrent callers.

**Library:** `github.com/coder/websocket` (the active fork of the archived gorilla/websocket, formerly nhooyr.io/websocket). It uses `context.Context` throughout, handles concurrent writes via a built-in serializer, and is actively maintained as of 2024–2025. MEDIUM confidence on this recommendation — verified via official pkg.go.dev and coder/websocket GitHub; gorilla/websocket was archived in late 2022.

**Single-writer pattern:** One goroutine owns all writes; all other goroutines send to a buffered `sendCh chan []byte`. This is the standard Go pattern for safe WebSocket writes without external mutexes.

```go
type ConnectionManager struct {
    serverURL string
    sendCh    chan []byte          // all callers write here; single goroutine drains
    recvCh    chan protocol.Envelope
    stateCh   chan ConnState       // connected/reconnecting/disconnected
    done      chan struct{}
}

func (cm *ConnectionManager) Send(data []byte) error {
    select {
    case cm.sendCh <- data:
        return nil
    case <-cm.done:
        return ErrClosed
    default:
        return ErrBufferFull  // caller drops the event (streaming events are best-effort)
    }
}
```

**Reconnect loop:**
```go
func (cm *ConnectionManager) Run(ctx context.Context) {
    backoff := 500 * time.Millisecond
    for ctx.Err() == nil {
        if err := cm.runOnce(ctx); err != nil {
            log.Warn().Err(err).Dur("retry", backoff).Msg("reconnecting")
            select {
            case <-time.After(backoff + jitter(backoff)):
            case <-ctx.Done():
                return
            }
            if backoff < 30*time.Second {
                backoff = min(backoff*2, 30*time.Second)
            }
        }
    }
}
```

**Connection lifecycle within `runOnce`:**
1. Dial with `coder/websocket.Dial`
2. Start read goroutine: reads frames, unmarshals into `protocol.Envelope`, sends to `recvCh`
3. Start write goroutine: drains `sendCh`, sends frames; sends ping every 30s
4. Set read deadline to 90s; pong resets deadline
5. Either goroutine error → `runOnce` returns → reconnect loop restarts

---

### internal/protocol — Wire Protocol Definitions

**New package**

**Responsibility:** Define all message types exchanged between node and server. Single source of truth. Will become the basis for the protocol spec document.

**Envelope (all messages share this wrapper):**
```go
type Envelope struct {
    Type      string          `json:"type"`
    MessageID string          `json:"msg_id"`   // UUID; server-assigned on commands
    Payload   json.RawMessage `json:"payload"`
}
```

**Node → Server types:**
- `node_register` — on connect/reconnect: node ID, version, OS, project list, capabilities
- `node_status` — heartbeat: uptime, active instance count, per-project summaries, system health
- `stream_event` — wraps a raw `claude.ClaudeEvent` with `instance_id` tag
- `instance_started` — Claude subprocess began; includes `instance_id` and `working_dir`
- `instance_finished` — Claude completed; includes `instance_id`, `session_id` (for resume), usage stats
- `instance_error` — Claude failed; includes `instance_id`, error message
- `ack` — acknowledge a server command (optional, for at-least-once delivery)

**Server → Node types:**
- `execute` — run Claude: `instance_id` (UUID), `project` (name), `prompt` (text)
- `stop` — cancel running instance: `instance_id`
- `new_session` — clear session ID for an instance (next run starts fresh): `instance_id`
- `status_request` — request current state snapshot
- `project_link` — register project: `name`, `path` (local directory)
- `project_unlink` — deregister project: `name`

---

### internal/dispatch — Command Router

**New package**

**Responsibility:** Receive parsed `Envelope` values from `wsconn.recvCh` and route them to the appropriate handler function. Holds no state itself — delegates immediately to the instance store, mapping store, or sends a reply via wsconn.

This is the thin replacement for the gotgbot dispatcher's handler registration. One `switch` on `envelope.Type`, one method call per case.

```go
type Dispatcher struct {
    instances *session.InstanceStore
    mappings  *project.MappingStore
    conn      *wsconn.ConnectionManager
    cfg       *config.Config
    auditLog  *audit.Logger
    wg        *sync.WaitGroup
}

func (d *Dispatcher) Run(ctx context.Context, recvCh <-chan protocol.Envelope) {
    for {
        select {
        case env := <-recvCh:
            d.handle(ctx, env)
        case <-ctx.Done():
            return
        }
    }
}
```

The `handle` method is a `switch env.Type` routing to `d.handleExecute`, `d.handleStop`, etc. Each handler is a method, making them individually testable.

---

### internal/session — Modified for Multiple Instances

**Existing package, modified**

**What changes:** In v1.1, the key is `channelID int64` — one Session per Telegram channel. In v1.2, the key is `instanceID string` — one Instance per server-assigned UUID. Multiple instances can share the same project directory, running simultaneously.

**Key data structure change:**

```
v1.1: SessionStore  map[int64]*Session
v1.2: InstanceStore map[string]*Instance  (string = UUID from server)
```

**What stays on the Instance struct (renamed from Session):**
- `sessionID string` — Claude `--resume` flag value — unchanged
- `workingDir string` — immutable after construction — unchanged
- `state SessionState` — idle/running/stopping — unchanged
- `queue chan QueuedMessage` — buffered, still serial per instance — unchanged
- `stopCh chan struct{}` — unchanged
- `cancelQuery context.CancelFunc` — unchanged
- `Worker()` loop logic — unchanged

**What changes on `QueuedMessage`:**

```go
// v1.1
type QueuedMessage struct {
    Text     string
    ChatID   int64           // Telegram chat ID — REMOVED
    UserID   int64           // Telegram user ID — REMOVED
    Callback StatusCallbackFactory
    ErrCh    chan error
}

// v1.2
type QueuedMessage struct {
    Text       string
    InstanceID string         // server-assigned UUID — ADDED
    Callback   claude.StatusCallback  // already resolved, not a factory
    ErrCh      chan error
}
```

The `StatusCallbackFactory` indirection (a function that returns a function, used to defer Telegram message creation until streaming started) is eliminated. In v1.2 the callback is created before enqueue, since there is no message ID to await.

**What changes on `WorkerConfig`:**

```go
// OnQueryComplete in v1.1: saves to persist.Save(SavedSession{...})
// OnQueryComplete in v1.2: sends instance_finished message via wsconn.Send(...)
// Signature unchanged; implementation at call site changes.
```

---

## Data Flow: Telegram → WebSocket

### v1.1 Flow (Telegram — for comparison)

```
Telegram update
    → gotgbot dispatcher
    → auth middleware (channel admin lookup)
    → HandleText()
        → mapping check (chatID → project path)
        → session.GetOrCreate(chatID)
        → start Worker goroutine if needed
        → StreamingState{bot, chatID}  (manages Telegram message IDs)
        → session.Enqueue(QueuedMessage{Callback: CreateStatusCallback(ss)})
            → Worker picks up message
            → claude.NewProcess → proc.Stream(cb)
                → cb called per NDJSON event:
                    text event  → bot.EditMessage (throttled 500ms)
                    tool event  → bot.EditMessage (status update)
                    result event → bot.DeleteMessage (status), flushPending
```

### v1.2 Flow (WebSocket)

```
Server sends: {"type":"execute","payload":{"instance_id":"uuid-1","project":"myapp","prompt":"..."}}
    → wsconn read goroutine
    → recvCh (chan protocol.Envelope)
    → dispatch.Dispatcher.handle()
        → handleExecute():
            → mappings.Get("myapp") → resolve /local/path/to/myapp
            → security.ValidatePath(path, cfg.AllowedPaths)
            → instances.GetOrCreate("uuid-1", "/local/path/to/myapp")
            → start Worker goroutine if needed (same wg.Add(1) pattern)
            → cb = wsStreamCallback("uuid-1", wsconn)
            → instance.Enqueue(QueuedMessage{InstanceID:"uuid-1", Callback:cb})
                → Worker picks up message
                → claude.NewProcess → proc.Stream(cb)
                    → cb called per NDJSON event:
                        ANY event → marshal stream_event{instance_id, event}
                                  → wsconn.Send(bytes)   (no throttle)
                → stream done → send instance_finished{instance_id, session_id}
```

**The key simplification:** `StreamingState` (~350 lines of Telegram edit-in-place throttling, segment splitting, MarkdownV2 conversion) is replaced by `wsStreamCallback` (~30 lines). The server and its webapp clients handle all presentation.

---

## Multiple Instances Per Project

### v1.1 Constraint

One Session per channel. A second message while one is running queues (up to 5) or returns "queue full". Parallelism = one Claude process per project.

### v1.2 Model

Multiple instances can run simultaneously in the same project directory. Each `instance_id` is a UUID the server generates per `execute` command. The `InstanceStore` is keyed by `instance_id`, not by project.

This means project "myapp" can have instances "uuid-1" and "uuid-2" running simultaneously, each with their own Worker goroutine and Claude subprocess.

**Instance lifecycle:**

1. Server sends `execute{instance_id:"uuid-1", project:"myapp", prompt:"..."}`
2. Dispatcher resolves project path → `instances.GetOrCreate("uuid-1", "/path/to/myapp")`
3. Worker goroutine starts; Claude subprocess spawns
4. Streaming: node sends `stream_event{instance_id:"uuid-1", event:{...}}` per NDJSON line
5. Claude completes: node sends `instance_finished{instance_id:"uuid-1", session_id:"..."}`
6. Instance remains in store (idle) — worker goroutine blocks on queue receive (no CPU)

**Session resume across instances:**

Each instance tracks its own `sessionID`. When the server wants to resume, it sends `execute` with the same `instance_id` as before. The `InstanceStore` finds the existing idle instance (still has its `sessionID` set). The Worker passes `--resume <sessionID>` to the next Claude invocation.

If the server sends a new UUID, a fresh instance starts with no session history (new Claude context).

**Instance cleanup:**

Idle instances consume negligible resources (goroutine blocks, no subprocess running). Two cleanup triggers:
1. Server sends `stop{instance_id:"uuid-1"}` — stops running query; instance remains idle
2. Node-side idle timeout (configurable, e.g., 1 hour) — if an instance has been idle since that long, remove it from the store and signal the Worker to exit

**Capacity limit:**

Each running Claude CLI process uses significant memory (~200MB). The node should enforce `MAX_INSTANCES` (default: 10). If `execute` would exceed this limit, send `instance_error{reason:"node at capacity"}` instead of spawning.

---

## Integration Points: Old vs New

### Component Mapping

| v1.1 Component | v1.2 Replacement | Notes |
|----------------|-----------------|-------|
| `internal/bot` | `internal/node` | Same lifecycle role; different transport |
| `ext.Updater` (polling) | `wsconn.ConnectionManager` (outbound) | No inbound ports required |
| `ext.Dispatcher` (gotgbot) | `internal/dispatch.Dispatcher` | Thin switch; no middleware chain needed |
| Auth middleware | Replaced by server-side auth | Server authenticates; node trusts server commands |
| Rate limit middleware | Per-project `RateLimiter` in dispatch | Same token-bucket code, different key |
| `StreamingState` (350 lines) | `wsStreamCallback` (~30 lines) | Presentation logic moves to server/client |
| `handlers/text.go` | `dispatch.handleExecute` | Same session/enqueue logic; no Telegram API calls |
| `handlers/command.go` | Protocol messages (status_request, etc.) | Commands become message types |
| Channel-to-project mapping (int64 key) | Project name-to-path mapping (string key) | MappingStore code unchanged; key type changes |

### Existing Interfaces Preserved

| Interface | Location | What Changes |
|-----------|----------|-------------|
| `claude.StatusCallback` | `internal/claude/process.go` | Unchanged — the integration seam |
| `session.Worker()` | `internal/session/session.go` | Unchanged — same loop, same processMessage |
| `claude.Process.Stream()` | `internal/claude/process.go` | Unchanged |
| `claude.BuildArgs()` | `internal/claude/events.go` | Unchanged |
| `project.MappingStore.Get/Set` | `internal/project/mapping.go` | Key type changes int64→string; method signatures unchanged |
| `audit.Logger.Log()` | `internal/audit/log.go` | Unchanged |

### The StatusCallback Bridge

This is the central integration point. The `claude.StatusCallback` signature (`func(ClaudeEvent) error`) is unchanged. In v1.1 it drives Telegram API calls; in v1.2 it drives WebSocket sends. The `claude` and `session` packages never know which transport is in use.

```go
// v1.1: StreamingState-based callback (created in handlers/streaming.go)
cb = CreateStatusCallback(ss)  // ss knows bot client + chatID; throttles + edits messages

// v1.2: wsStreamCallback (created in dispatch package)
func wsStreamCallback(instanceID string, cm *wsconn.ConnectionManager) claude.StatusCallback {
    return func(event claude.ClaudeEvent) error {
        payload, _ := json.Marshal(protocol.StreamEvent{
            InstanceID: instanceID,
            Event:      event,
        })
        env, _ := json.Marshal(protocol.Envelope{
            Type:    "stream_event",
            Payload: payload,
        })
        return cm.Send(env)  // non-blocking; drops if buffer full
    }
}
```

---

## Configuration Changes

### Remove from `.env`

```
TELEGRAM_BOT_TOKEN       — removed (no Telegram)
TELEGRAM_ALLOWED_USERS   — removed (server handles auth)
```

### Add to `.env`

```
WS_SERVER_URL      # wss://server.example.com/nodes/ws (required)
NODE_ID            # unique identifier, e.g. "desktop-home" (required)
NODE_SECRET        # shared secret for server authentication (required)
MAX_INSTANCES      # max simultaneous Claude processes (default: 10)
```

### Keep (with modified meaning)

```
CLAUDE_WORKING_DIR   # default root; projects must be under this or ALLOWED_PATHS
ALLOWED_PATHS        # restricts which directories project paths can use
CLAUDE_CLI_PATH      # unchanged
DATA_DIR             # unchanged — same JSON files
AUDIT_LOG_PATH       # unchanged
RATE_LIMIT_*         # unchanged — now per-project instead of per-channel
```

---

## Project Structure

```
gsd-node/                        (or rename from gsd-tele-go)
├── main.go                      # Load config, create Node, signal handling
├── go.mod                       # Remove gotgbot; add github.com/coder/websocket
├── internal/
│   ├── node/
│   │   └── node.go              # Node struct, Start/Stop, heartbeat, registration
│   ├── wsconn/
│   │   ├── conn.go              # ConnectionManager, dial loop, reconnect backoff
│   │   └── conn_test.go         # Mock server tests for reconnect, send, receive
│   ├── protocol/
│   │   ├── messages.go          # Envelope, all inbound/outbound message structs
│   │   └── messages_test.go     # Marshal/unmarshal round-trip tests
│   ├── dispatch/
│   │   ├── dispatch.go          # Dispatcher.Run(), handle(), per-type handlers
│   │   └── dispatch_test.go     # Unit tests for each command type
│   ├── session/
│   │   ├── instance.go          # Instance struct (renamed from Session)
│   │   ├── instance_test.go
│   │   ├── store.go             # InstanceStore map[string]*Instance
│   │   ├── store_test.go
│   │   ├── worker.go            # Worker loop (logic unchanged from session.go)
│   │   ├── persist.go           # PersistenceManager (unchanged)
│   │   └── persist_test.go
│   ├── claude/                  # UNCHANGED
│   │   ├── events.go
│   │   └── process.go
│   ├── project/                 # MappingStore: key type changes, logic unchanged
│   │   ├── mapping.go
│   │   └── mapping_test.go
│   ├── security/                # ValidatePath, CheckCommandSafety (unchanged)
│   │   ├── ratelimit.go
│   │   └── validate.go
│   ├── audit/                   # Logger (unchanged)
│   │   └── log.go
│   └── config/
│       └── config.go            # Remove Telegram fields; add WS_SERVER_URL, NODE_ID
├── data/
│   ├── mappings.json
│   └── session-history.json
└── .env
```

---

## Architectural Patterns

### Pattern 1: Single-Writer WebSocket Channel

**What:** One goroutine owns all WebSocket writes. All other goroutines send byte slices to a buffered channel. The writer goroutine drains the channel and calls `conn.Write`.

**When to use:** Always for WebSocket clients. Concurrent writes to a WebSocket connection corrupt the frame stream. This pattern eliminates the need for a write mutex and integrates naturally with select-based shutdown.

**Trade-offs:** A full send channel means the caller drops the event. For streaming events this is acceptable (best-effort). For critical messages like `instance_finished`, the dispatcher should retry or use a priority queue if the channel is full.

### Pattern 2: Reconnect Loop with Exponential Backoff + Jitter

**What:** A `for` loop wraps the connection lifecycle. On any error, wait with exponential backoff before redialing. Add jitter to prevent thundering herd if multiple nodes reconnect simultaneously after a server restart.

**When to use:** Any outbound WebSocket client where the server may restart or be temporarily unreachable.

**Parameters:** Start at 500ms, double each failure, cap at 30s, add 10–20% random jitter. Reset to 500ms after a connection that lasts more than 10s (distinguishes "brief flap" from "sustained outage").

### Pattern 3: Instance-Tagged Stream Events

**What:** Every message flowing from node to server carries the originating `instance_id`. The server uses this tag to route events to the correct client session or conversation thread.

**When to use:** Any system where multiple concurrent processes share a single transport channel.

**Alternative rejected:** One WebSocket connection per Claude instance. This was rejected because: O(instances) connections per node, complex reconnect coordination, connection establishment latency delays instance startup.

### Pattern 4: StatusCallback as Transport Bridge

**What:** The `claude.StatusCallback` function signature (`func(ClaudeEvent) error`) is the seam between subprocess management and transport. Change the transport by providing a different callback implementation.

**When to use:** When the subprocess management layer must remain transport-agnostic. This is the central design pattern enabling the v1.1 → v1.2 transformation without touching `internal/claude` or `internal/session`.

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Concurrent WebSocket Writes

**What people do:** Call `conn.Write()` from multiple goroutines concurrently.
**Why it's wrong:** Corrupts the WebSocket frame stream; panics under the race detector.
**Do this instead:** Single write goroutine consuming from a buffered channel.

### Anti-Pattern 2: Blocking StatusCallback

**What people do:** Have the `StatusCallback` block waiting for a network write to complete before returning.
**Why it's wrong:** The Worker goroutine processes Claude NDJSON events synchronously. A blocking callback stalls event processing, backs up the subprocess stdout pipe buffer, and can deadlock the Claude CLI process.
**Do this instead:** Non-blocking send to `wsconn.sendCh`. Drop the event if the buffer is full (streaming events are best-effort). The `instance_finished` message carries the durable session ID and must be delivered reliably via a retry or priority mechanism.

### Anti-Pattern 3: One WebSocket Per Claude Instance

**What people do:** Open a new WebSocket connection to the server for each Claude subprocess to avoid multiplexing complexity.
**Why it's wrong:** N active instances = N connections. Server reconnect management scales linearly. Connection establishment latency per-instance adds latency to every `execute` command.
**Do this instead:** Multiplex all instances over one connection with `instance_id` tagging.

### Anti-Pattern 4: Carrying Telegram Key Semantics (channelID) into WebSocket

**What people do:** Keep `channelID int64` as the session key and map it to a WebSocket concept.
**Why it's wrong:** WebSocket has no concept of "channels". The natural key in the new model is `instanceID string` (server-assigned UUID per execute command). Mixing these breaks session resume.
**Do this instead:** Use `instanceID` as the atomic identity unit. Project name is a lookup key to resolve the local directory path — separate concern.

### Anti-Pattern 5: Presentation Logic in Node

**What people do:** Keep `formatting.ConvertToMarkdownV2()` in the streaming callback "since it's already there".
**Why it's wrong:** Markdown formatting is presentation logic that belongs in the webapp client. Adding it to the node couples the node to a specific UI, makes the protocol carry rendered HTML instead of structured events, and prevents the server from serving multiple client types.
**Do this instead:** Forward raw `claude.ClaudeEvent` structs as JSON. Strip `internal/formatting` down to tool status strings (used only in `node_status` heartbeat).

---

## Build Order (Dependencies First)

Building in this order ensures each phase is testable in isolation before the next builds on it.

**Phase 1 — Protocol + Config (no external deps)**
Define `internal/protocol/messages.go` (all message structs, Envelope). Update `internal/config/config.go` (remove Telegram fields, add WS_SERVER_URL, NODE_ID, NODE_SECRET, MAX_INSTANCES). Write round-trip marshal/unmarshal tests for all message types. This unblocks all subsequent phases and produces the protocol spec artifact.

**Phase 2 — Connection Manager (external dep: coder/websocket)**
Build `internal/wsconn` with full reconnect loop, read/write goroutines, ping/pong, and the single-writer channel pattern. Test with a local mock WebSocket server (stdlib `net/http` upgrader). This is the riskiest new infrastructure — validate reconnect behavior and write serialization before building on top.

**Phase 3 — Session Layer Migration**
Rename `Session` → `Instance`, `SessionStore` → `InstanceStore`. Change key type from `int64` to `string`. Update `QueuedMessage` (remove ChatID/UserID, add InstanceID, change Callback to direct not factory). Update `WorkerConfig.OnQueryComplete` to accept a callback that sends `instance_finished`. Run all existing session tests with minor fixture updates — the Worker logic is unchanged.

**Phase 4 — Dispatch + wsStreamCallback**
Build `internal/dispatch` with routing for all command types. Implement `wsStreamCallback`. Wire dispatch to instance store, mapping store, and wsconn. Unit-test each command handler with mock stores.

**Phase 5 — Node Lifecycle**
Build `internal/node` to wire everything: wsconn + dispatch + instance store + mapping store + audit + heartbeat + graceful shutdown. Update `main.go` to instantiate `Node` instead of `Bot`. Remove `internal/bot`, `internal/handlers`, gotgbot from go.mod. End-to-end test: connect to a local mock server, send `execute`, verify `stream_event` messages arrive.

**Phase 6 — Protocol Spec + Server Spec Documents**
Write the wire protocol spec (all message types, envelope format, authentication handshake, sequencing guarantees). Write the server interface spec (what HTTP/WebSocket endpoints the server must expose, what message sequences the node expects). These are deliverables for the server repo.

---

## Scaling Considerations

The node is a single-machine service. It scales horizontally by running more nodes. The relevant per-node constraints:

| Dimension | v1.1 | v1.2 | Constraint |
|-----------|------|------|------------|
| Simultaneous Claude runs | 1 per channel | N per node (configurable MAX_INSTANCES) | ~200MB RAM per Claude process |
| Projects per node | Unbounded (JSON) | Unbounded (JSON) | Negligible |
| Streaming throughput | Telegram rate-limited (25 edits/sec global) | Unbounded over WebSocket | Node CPU (JSON marshal ~100µs/event) |
| Connection overhead | Telegram polling (1 HTTP req/10s) | 1 WebSocket connection | Negligible |
| Reconnect impact | N/A | Queued sends dropped during disconnect | Instance events lost if connection drops mid-stream |

**Reconnect event loss:** If the WebSocket connection drops while a Claude instance is streaming, `sendCh` fills up and subsequent events are dropped. The `instance_finished` (or `instance_error`) message carries the durable data (session ID, usage stats). The server should treat any gap in `stream_event` messages as a potential dropout and request a status refresh after reconnect.

---

## Sources

- Existing codebase (v1.1, 11,600 lines Go) — HIGH confidence (primary source; direct file reads)
- [coder/websocket GitHub](https://github.com/coder/websocket) — MEDIUM confidence (WebSearch, pkg.go.dev verified)
- [WebSocket.org: Go WebSocket Guide](https://websocket.org/guides/languages/go/) — MEDIUM confidence (WebSearch)
- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines) — HIGH confidence (official Go blog)
- [Go by Example: Stateful Goroutines](https://gobyexample.com/stateful-goroutines) — HIGH confidence (official resource)

---

*Architecture research for: GSD Node v1.2 — WebSocket transport replacing Telegram*
*Researched: 2026-03-20*
