# Pitfalls Research

**Domain:** Go WebSocket node — multiple Claude CLI subprocesses, custom protocol, Telegram removal
**Researched:** 2026-03-20 (v1.2 milestone)
**Confidence:** HIGH (codebase read directly; Go WebSocket library docs and community issues consulted)

---

## v1.2 Milestone Pitfalls — WebSocket Node, Multi-Instance Subprocess, Telegram Removal

These pitfalls are specific to the v1.2 milestone: replacing the Telegram bot layer with a custom WebSocket client that manages multiple concurrent Claude CLI subprocesses per project directory.

---

## Critical Pitfalls

### Pitfall WS-1: Reconnection Storm — No Backoff, No Jitter

**What goes wrong:**
The node loses its WebSocket connection and immediately retries. If the server restarts (e.g., deploy, crash), every node reconnects at the exact same moment, creating a thundering-herd that can crash the server before it finishes starting. Even for a single node, tight retry loops without backoff can consume CPU and produce thousands of log lines per minute, masking real error causes.

**Why it happens:**
The naive pattern is `for { conn, err = dial(); if err != nil { continue } }`. Developers add a `time.Sleep(1 * time.Second)` and consider it handled. That sleep is fixed — it provides no jitter and does not increase with consecutive failures.

**How to avoid:**
Use exponential backoff with jitter: start at 500ms, double on each failure, cap at 30 seconds, add `±25%` random jitter to spread reconnection times. Reset the delay to the minimum immediately after a successful connection is established (not after a successful handshake message — after confirmed success from the server). In Go:

```go
delay := 500 * time.Millisecond
const maxDelay = 30 * time.Second
for {
    conn, err := dialWebSocket(ctx, url)
    if err == nil {
        delay = 500 * time.Millisecond  // reset on success
        runLoop(conn)
    }
    jitter := time.Duration(rand.Int63n(int64(delay / 4)))
    time.Sleep(delay + jitter)
    if delay < maxDelay {
        delay *= 2
        if delay > maxDelay { delay = maxDelay }
    }
}
```

**Warning signs:**
Log lines with "dial: connection refused" appearing at exactly 1-second intervals. CPU spike on the node when server is down. Server logs showing burst of simultaneous connections from all nodes at restart.

**Phase to address:**
WebSocket client phase (first phase of v1.2). This is the foundation — get it right before adding protocol logic.

---

### Pitfall WS-2: Concurrent Write Panic on gorilla/websocket

**What goes wrong:**
Multiple goroutines write to the same `*websocket.Conn` simultaneously. gorilla/websocket explicitly does not support concurrent writes and panics with `"concurrent write to websocket connection"`. This is particularly likely in this system where streaming Claude output arrives on a worker goroutine while heartbeat pings are sent from a separate goroutine — both need to write to the same connection.

**Why it happens:**
Developers write `conn.WriteJSON(msg)` from the worker goroutine and `conn.WriteMessage(websocket.PingMessage, nil)` from a heartbeat goroutine without realising these share the same underlying write buffer. gorilla/websocket uses an `isWriting` flag and panics rather than silently corrupting frames.

**How to avoid:**
Funnel ALL writes through a single goroutine owning the connection, using a buffered send channel. The send goroutine reads from the channel and writes to the socket. All other goroutines put messages into the channel:

```go
type Conn struct {
    ws     *websocket.Conn
    sendCh chan []byte  // buffered, e.g. 64 entries
}

// Only this goroutine calls ws.WriteMessage
func (c *Conn) writePump(ctx context.Context) {
    ticker := time.NewTicker(pingInterval)
    defer ticker.Stop()
    for {
        select {
        case msg := <-c.sendCh:
            c.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
            if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
                return
            }
        case <-ticker.C:
            c.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
            c.ws.WriteMessage(websocket.PingMessage, nil)
        case <-ctx.Done():
            return
        }
    }
}
```

**Warning signs:**
Panic stack trace containing `gorilla/websocket` and `isWriting`. Intermittent panics that are hard to reproduce (timing-dependent).

**Phase to address:**
WebSocket client phase. Establish the write-pump pattern before wiring up Claude streaming output.

---

### Pitfall WS-3: Unbounded Send Channel Causes Memory Growth Under Backpressure

**What goes wrong:**
The send channel is unbounded (or very large). When the server is slow to read — due to network congestion, server overload, or a slow consumer — Claude output accumulates in the send channel. The node continues producing streaming output from subprocesses, all of which is enqueued into the send channel. Memory grows until the Windows Service is killed by the OS or the output is meaningless (1000 lines buffered before the server reads the first).

**Why it happens:**
Developers use `make(chan []byte, 1024)` thinking "large buffer = no blocking." The buffer fills silently. There is no feedback to the subprocess that the server cannot keep up.

**How to avoid:**
Use a bounded send channel (32-64 entries is sufficient). When the send channel is full, apply one of two strategies:
- Drop the message and log a warning (acceptable for streaming output where freshness matters more than completeness)
- Apply backpressure to the session worker (harder but correct for command dispatch)

For streaming Claude output specifically: drop intermediate `assistant` stream events when the send channel is full. Never drop `result` events (they contain the final session ID needed for persistence).

```go
func (c *Conn) sendOrDrop(msg []byte) bool {
    select {
    case c.sendCh <- msg:
        return true
    default:
        // log warning: dropping streaming event due to backpressure
        return false
    }
}
```

**Warning signs:**
Node memory growing continuously while a Claude session is streaming. `len(sendCh)` near capacity in debug logs. Server reporting stale timestamps on streaming messages.

**Phase to address:**
WebSocket client phase, during streaming output integration.

---

### Pitfall WS-4: Session State Divergence After Reconnect

**What goes wrong:**
The node reconnects after a network interruption. In-flight Claude sessions were streaming when the connection dropped. The server has no knowledge of what happened during the disconnection — it does not know if the session completed, failed, or is still running. If the node re-registers and the server dispatches a new command to the same project, the node now has two overlapping Claude CLI processes in the same working directory.

**Why it happens:**
Reconnection logic re-sends the "hello" / registration message but does not include the current session states. The server assumes a fresh node. The protocol has no concept of "I was running X when I disconnected."

**How to avoid:**
The node's registration/hello message must include a session status snapshot: for each project, whether a session is idle or running, and if running, the session ID. The server must reconcile this against its own state and either cancel the stale command or acknowledge the running session. Design this into the protocol from day one — retrofitting it is difficult.

**Warning signs:**
Duplicate sessions visible in node logs after reconnect. Server dispatching commands to a project that already has an active session. Two `claude` processes appearing in `tasklist` for the same working directory.

**Phase to address:**
Protocol design phase (before WebSocket client implementation). This is a protocol contract, not an implementation detail.

---

### Pitfall WS-5: Missing Read Deadline Allows Goroutine Leak on Dead Connection

**What goes wrong:**
The WebSocket read loop blocks indefinitely on `conn.ReadMessage()`. When the server disappears (crash, network partition, NAT timeout), TCP does not always deliver a FIN — the connection appears open at the socket level. The read goroutine blocks forever, holding the connection object alive and preventing reconnection logic from triggering.

**Why it happens:**
gorilla/websocket's `ReadMessage` blocks until a frame arrives or an error occurs. A silently-dead TCP connection produces neither. Without a read deadline or server-side pings, the goroutine leaks.

**How to avoid:**
Set a read deadline that requires the server to send at least one ping within the deadline window:

```go
conn.SetReadDeadline(time.Now().Add(pongWait))
conn.SetPongHandler(func(string) error {
    conn.SetReadDeadline(time.Now().Add(pongWait))
    return nil
})
```

When `pongWait` expires with no pong received, `ReadMessage` returns a timeout error, triggering reconnection. The write-pump sends a ping every `pingInterval` (e.g., 30s); `pongWait` should be `pingInterval + 10s`.

**Warning signs:**
Node logs showing no activity for minutes but reporting itself as connected. Server health dashboard showing node as connected but not sending heartbeats. Node goroutine count growing on each reconnect (old goroutines never exit).

**Phase to address:**
WebSocket client phase, as part of the connection lifecycle design.

---

### Pitfall MP-1: Multiple Claude CLI Instances Writing to Same Working Directory

**What goes wrong:**
Two Claude CLI subprocesses run simultaneously in the same project directory. Both read and modify `.claude/` state files (session history, settings). Claude CLI does not use file locking. One process's writes corrupt the other's session state, causing `--resume` to fail with a deserialized garbage session ID or silently using a wrong context window.

**Why it happens:**
The project supports "multiple simultaneous Claude CLI instances per project directory" as a design goal. The natural implementation is to spawn a new `claude` process for each incoming command without checking whether another is running.

**How to avoid:**
Enforce a per-project serialization policy: only one Claude CLI instance may be running in a given working directory at a time. The existing `Session` struct already serializes commands via its `queue` channel — this model must be preserved when adding multiple "instances." In v1.2, "multiple instances per project" should mean multiple *logical sessions* (e.g., GSD workflow stages) that are run serially within the project, not truly concurrent subprocesses in the same directory.

If truly parallel execution in the same directory is required later, each logical instance must use its own isolated subdirectory or a separate session namespace.

**Warning signs:**
`--resume` errors appearing only when two sessions start close together in time. Corrupted `.claude/` JSON visible in filesystem. Claude complaining about session not found after a concurrent run.

**Phase to address:**
Session manager design phase. Clarify in the session dispatcher whether "multiple instances" means concurrent-in-directory or serially-queued-in-directory. Default to serial.

---

### Pitfall MP-2: Subprocess Zombie / Resource Leak on Context Cancellation

**What goes wrong:**
A Claude CLI subprocess is killed via context cancellation (e.g., `/stop` command, node shutdown). The process exits but its goroutines reading `stdout` and `stderr` pipes remain blocked. On Windows, the subprocess may become a zombie if `cmd.Wait()` is not called, holding the PID and file handles. Over time, leaked subprocesses accumulate, exhausting file descriptor limits.

**Why it happens:**
`exec.Cmd.Wait()` must be called to release OS resources. If the code returns from the streaming loop on error without calling `Wait()`, or if the read goroutines block on closed pipes and `Wait()` never completes, the process lingers.

The existing codebase already sets `cmd.WaitDelay = 5 * time.Second` (Go 1.20+ feature) to cap how long `cmd.Wait()` blocks after the process exits. This must be preserved in v1.2.

**How to avoid:**
Always call `cmd.Wait()` in a defer, even on error paths. Set `WaitDelay` to a finite value so pipe-reading goroutines are forcibly unblocked:

```go
cmd.WaitDelay = 5 * time.Second
if err := cmd.Start(); err != nil { return err }
defer cmd.Wait()  // always called, releases OS resources
```

On Windows, use a process job object or `cmd.SysProcAttr` with `CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP` to ensure child processes are killed when the parent is killed. The existing `process.go` already uses `syscall` for this — do not remove it.

**Warning signs:**
`tasklist | grep claude` showing stale `claude.exe` processes after sessions end. File descriptor count growing over time. `cmd.Wait()` blocking indefinitely during shutdown.

**Phase to address:**
Session/subprocess management phase. Verify the existing `WaitDelay` pattern survives the Telegram removal refactor.

---

### Pitfall MP-3: Session ID Collision When Multiple Instances Use Same Persistence File

**What goes wrong:**
Two Claude CLI sessions for the same project complete at nearly the same time. Both attempt to write their new session ID to the JSON persistence file. The atomic write-rename pattern (write to temp file, rename over target) means one write wins and the other is silently lost. The losing session ID is orphaned — the `--resume` flag will point to the winning session, and the other session's context is permanently inaccessible.

**Why it happens:**
The current `StateManager` uses `sync.RWMutex` for in-process coordination but has no concept of which session ID belongs to which instance. When two instances complete simultaneously, both call `OnQueryComplete(newSessionID)` and both attempt to persist.

**How to avoid:**
Persist session IDs per-instance, keyed by a stable instance identifier (e.g., UUID assigned at instance creation). The persistence format must change from a single `sessionID` string per project to a map from instance ID to session ID. The `--resume` flag uses the instance's own session ID, not a shared project-level ID.

**Warning signs:**
Session IDs changing unexpectedly between commands in the same project. `/status` showing a session ID that doesn't match the last completed run. Log lines showing multiple `OnQueryComplete` calls within milliseconds for the same project.

**Phase to address:**
Session/subprocess management phase, during the multi-instance model design.

---

### Pitfall TG-1: Telegram-Coupled Abstractions Leak Into New Protocol Layer

**What goes wrong:**
The existing `StatusCallbackFactory` type takes a `chatID int64` — a Telegram-specific concept. The `QueuedMessage` struct has `ChatID int64` and `UserID int64` fields. The `formatting` package converts Markdown to Telegram HTML. If these types are reused directly in v1.2 without redesign, the new WebSocket protocol layer inherits Telegram semantics: node messages still carry `chatID`, the formatting layer still produces HTML, and the protocol spec becomes polluted with chat-platform concepts.

**Why it happens:**
Reuse feels faster. "Just change the transport and keep the rest" is tempting. But the chatID/userID model is not the right identity model for a node-server protocol where identity is node ID + project ID + instance ID.

**How to avoid:**
Perform a clean-break refactor during Telegram removal. Define new protocol-level identity types: `NodeID`, `ProjectID`, `InstanceID`. Replace `StatusCallbackFactory` with a protocol-agnostic event emitter. Move the Telegram-specific formatting package out of the core and archive it (don't delete — it contains tested logic that may be useful for reference or future protocol display layers).

Concretely: the new `QueuedMessage` equivalent should reference `InstanceID`, not `chatID`. The status callback should produce protocol messages, not Telegram API calls.

**Warning signs:**
`int64` chatID values appearing in WebSocket message schemas. `formatting.go` being imported by the new transport layer. Test files referencing Telegram bot methods.

**Phase to address:**
Telegram removal phase. This must be done before the WebSocket protocol layer is built, not after — retrofitting identity models is expensive.

---

### Pitfall TG-2: Session Persistence Keys Tied to Telegram Channel IDs

**What goes wrong:**
The current persistence file (`sessions.json`) keys session state by Telegram channel ID (`int64`). In v1.2, channel IDs have no meaning. If the persistence format is carried over unchanged, the v1.2 node cannot load persisted session data without knowing the old channel IDs. The session resume capability — a core feature — is broken on first startup after migration.

**Why it happens:**
The migration path is not planned. Developers assume "we'll just clear the session state" but users lose active Claude context windows and the `/resume` capability that was explicitly designed in v1.0.

**How to avoid:**
Design a persistence migration step as part of the Telegram removal phase:
1. Define the new persistence key format: `projectID` (directory path or its hash) + `instanceID`
2. Write a one-time migration that reads the old `sessions.json`, maps channel IDs to project directories via the existing `mappings.json`, and writes a new `sessions.json` with the v1.2 format
3. Test the migration on a copy of production data before deploying

**Warning signs:**
Empty session store on first v1.2 startup. `--resume` always starting fresh sessions. Log lines "no session found for project" immediately after migration.

**Phase to address:**
Telegram removal phase. Migration must ship with the first v1.2 build, not as a follow-up.

---

### Pitfall TG-3: Removing gotgbot Dependency Before All Its Functionality Is Reimplemented

**What goes wrong:**
The team removes `gotgbot/v2` from `go.mod` to mark the Telegram removal as "done." But several non-obvious pieces of functionality lived inside the gotgbot integration: rate limiting (per-channel `rate.Limiter` instances keyed by channel ID), the dispatch queue ordering, the middleware chain (auth, logging, audit). These are not imported from gotgbot directly but were designed around its update dispatch model. Removing gotgbot without auditing what depended on its dispatch model leaves gaps.

**Why it happens:**
`go mod tidy` removes the dependency cleanly (no import errors), giving a false sense of completion. The functionality gaps only appear at runtime.

**How to avoid:**
Before removing the gotgbot dependency, audit every handler and middleware file for functionality that must be preserved in v1.2:
- Rate limiting (move to per-project, per-instance config)
- Audit logging (currently logs chatID — must be updated to log nodeID/projectID)
- Auth middleware (replace with WebSocket authentication at connection time)
- The dispatch ordering guarantees (currently: one goroutine per update)

Create a checklist of all non-transport functionality and verify each one has a v1.2 equivalent before removing the import.

**Warning signs:**
Missing rate limiting after migration. Audit log entries with zero values for node/project identity. Commands processed out of order under concurrent load.

**Phase to address:**
Telegram removal phase. Run the audit before writing any replacement code.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Reuse `int64` chatID as project key | No persistence migration needed | Breaks conceptual model; ties node to Telegram even after removal | Never |
| Unbounded send channel | No dropped messages | OOM under backpressure; hides slow server | Never |
| Fixed 1s reconnect sleep | Simple to write | Reconnection storms if server restarts | Never for production |
| Skip session status in hello message | Simpler protocol | Orphaned sessions after any reconnect | Never |
| Single goroutine for reads AND writes | Simpler code | Forces explicit demultiplexing; blocks on slow writes during read | Never — always separate read/write goroutines |
| One shared persistence file for all instances | No format change | Session ID collision on concurrent completion | Only if serial-per-project is enforced |
| Import `formatting` package in transport layer | Reuse existing code | Telegram HTML in protocol messages | Never |

---

## Integration Gotchas

Common mistakes when connecting to external services.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Claude CLI (NDJSON) | Reading stdout line-by-line without a scanner buffer limit | Use `bufio.Scanner` with an explicit buffer size (default 64KB is too small for large tool outputs); set to 1MB |
| Claude CLI | Assuming process exits cleanly when context is cancelled | Always call `cmd.Wait()` in defer; set `WaitDelay = 5s` to prevent pipe goroutine leaks |
| Claude CLI | Not filtering `CLAUDECODE=` from env before spawning subprocess | Nested claude invocations produce errors; the existing `FilteredEnv` pattern must be preserved in v1.2 |
| WebSocket server | Sending auth token as URL query parameter | Token appears in server access logs and any proxy logs; send in first message after connection or use `Sec-WebSocket-Protocol` header during handshake |
| WebSocket server | Not setting write deadline before every write | A slow or dead server causes `WriteMessage` to block indefinitely; always `SetWriteDeadline(time.Now().Add(writeTimeout))` before each write |
| Windows Service | Not propagating SIGTERM/SERVICE_CONTROL_STOP to subprocess tree | Claude CLI child processes remain after service stop; use a process job object or `CREATE_NEW_PROCESS_GROUP` |

---

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Logging every streaming NDJSON line at DEBUG level | Log files grow GB/day per active session | Log at TRACE level with a build tag; only log final result events and errors at INFO | 3+ active sessions streaming simultaneously |
| JSON marshalling inside send-channel hot path | CPU spike during streaming output; GC pressure | Pre-marshal messages; reuse `json.Encoder` with a buffer pool | High-frequency streaming (>100 events/sec) |
| Synchronous persistence on every session event | File I/O on the worker goroutine; serializes all output | Batch persistence: write on query completion, not on every streaming event | Sessions with long multi-step tool sequences |
| Creating a new `*json.Decoder` per NDJSON line | Allocations per line for long-running sessions | Create one decoder per subprocess and reuse it | Sessions with >1000 tool events |

---

## Security Mistakes

Domain-specific security issues beyond general web security.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Node ID is just a hostname or machine name | Trivially spoofed; any machine can impersonate any node | Node ID must be a UUID generated at first run and stored in config; server validates against pre-registered node list |
| Shared static API key for all nodes | Key leak compromises all nodes simultaneously | Per-node API keys; rotation mechanism in the protocol |
| Auth token in WebSocket URL | Token logged by load balancers, proxies, and server access logs | Send auth in first protocol message after connection; server closes unauthenticated connections after a timeout (e.g., 5s) |
| Passing raw server command strings to `exec.Command` | Command injection if server is compromised | Never use `exec.Command` with shell interpolation; always use slice args: `exec.Command("claude", args...)` — the existing code does this correctly; verify it survives the refactor |
| Allowing the server to specify arbitrary working directories | Path traversal to sensitive directories outside project roots | Validate every server-specified path against the node's `ALLOWED_PATHS` list before use; the existing `security.ValidatePath` must be applied at the WebSocket dispatch layer |

---

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **WebSocket client connected:** Verify read deadline is set and pong handler resets it — a connection with no deadline appears working until the server dies silently
- [ ] **Telegram removed:** Run `go mod tidy && go build ./...` — zero import errors does not mean zero functionality gaps; audit middleware, rate limiting, and audit logging separately
- [ ] **Multi-instance implemented:** Check `tasklist` on Windows for stale `claude.exe` processes after a session stop — goroutine-level cleanup may appear clean while OS processes linger
- [ ] **Persistence migrated:** Start v1.2 node after migration and verify `/status` shows existing sessions with correct session IDs, not fresh empty sessions
- [ ] **Reconnection works:** Kill the server while a Claude session is streaming; verify the session completes locally, the node reconnects, and the final result is delivered to the server without duplicates
- [ ] **Backpressure handled:** Use a slow mock server that reads WebSocket messages at 1 msg/sec while Claude streams at 50 msg/sec; verify node memory stays bounded
- [ ] **Shutdown is clean:** Stop the Windows Service while a Claude session is running; verify `claude.exe` is not visible in Task Manager 10 seconds later

---

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Reconnection storm hit server | LOW | Add exponential backoff + jitter to node reconnect logic; redeploy; server recovers once reconnect pressure drops |
| Concurrent write panic | HIGH | Panic crashes the node process; NSSM restarts it; add write-pump pattern before next deploy; examine crash dump for goroutine that violated the single-writer rule |
| Session ID collision corrupted persistence | MEDIUM | Manually edit `sessions.json` to restore correct session IDs; add per-instance session ID storage to prevent recurrence |
| Zombie subprocesses after service stop | LOW | `taskkill /F /IM claude.exe`; fix `WaitDelay` and process group handling before next deploy |
| Telegram types leaked into protocol | HIGH | Protocol redesign required; server and node must be updated together; data migration for any persisted messages using chatID as identity |
| Session persistence broken after migration | MEDIUM | Run migration script on `sessions.json` using `mappings.json` as the channel-to-directory lookup; restart node |

---

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| WS-1: Reconnection storm | Phase: WebSocket client | Simulate server restart; verify exponential backoff in logs |
| WS-2: Concurrent write panic | Phase: WebSocket client | `go test -race ./...` on connection package; stress-test with concurrent streaming |
| WS-3: Unbounded send channel / backpressure | Phase: WebSocket client + streaming integration | Slow mock server test; verify node memory is bounded under backpressure |
| WS-4: Session state divergence after reconnect | Phase: Protocol design (before implementation) | Kill server during active session; verify reconnect sends session status snapshot |
| WS-5: Missing read deadline / goroutine leak | Phase: WebSocket client | Kill server TCP without FIN (firewall rule); verify node detects dead connection within pongWait |
| MP-1: Multiple CLI instances in same directory | Phase: Session/subprocess manager design | Two concurrent commands to same project; verify serial execution via queue |
| MP-2: Subprocess zombie on cancellation | Phase: Session/subprocess manager | Stop session mid-run; verify `claude.exe` absent from tasklist within 10s |
| MP-3: Session ID collision | Phase: Session/subprocess manager | Two sessions complete simultaneously; verify both session IDs are independently persisted |
| TG-1: Telegram-coupled abstractions | Phase: Telegram removal | `grep -r "chatID\|ChatID\|TelegramID" internal/` returns zero results in transport layer |
| TG-2: Session persistence key migration | Phase: Telegram removal | Fresh v1.2 node loads existing sessions; `/status` shows correct session IDs |
| TG-3: Removing dependency before reimplementing | Phase: Telegram removal | Audit checklist completed before `go get` removal; all middleware functionality accounted for |

---

## Sources

- [gorilla/websocket concurrent write issues](https://github.com/gorilla/websocket/issues/390) — "panic: concurrent write to websocket connection" — HIGH confidence (official library issues)
- [gorilla/websocket pkg.go.dev](https://pkg.go.dev/github.com/gorilla/websocket) — Connections support one concurrent reader and one concurrent writer — HIGH confidence
- [coder/websocket vs gorilla comparison](https://websocket.org/guides/languages/go/) — library recommendations for new projects — MEDIUM confidence
- [WebSocket reconnection state sync guide](https://websocket.org/guides/reconnection/) — state divergence after reconnect is the hard problem — MEDIUM confidence
- [Backpressure in WebSocket streams](https://skylinecodes.substack.com/p/backpressure-in-websocket-streams) — unbounded buffers and OOM — MEDIUM confidence
- [WebSocket authentication best practices](https://ably.com/blog/websocket-authentication) — token in first message, not URL — MEDIUM confidence
- [Go graceful shutdown patterns](https://dev.to/yanev/a-deep-dive-into-graceful-shutdown-in-go-484a) — context cancellation, WaitGroup, subprocess cleanup — MEDIUM confidence
- [Cross-platform file locking in Go](https://www.chronohq.com/blog/cross-platform-file-locking-with-go) — advisory locking between processes — MEDIUM confidence
- [gofrs/flock](https://github.com/gofrs/flock) — thread-safe file locking for concurrent process access — MEDIUM confidence
- [Go WebSocket 2025 forum discussion](https://forum.golangbridge.org/t/websocket-in-2025/38671) — library ecosystem current state — MEDIUM confidence
- Codebase direct read: `internal/session/session.go`, `internal/session/store.go`, `internal/claude/process.go` — existing patterns (WaitDelay, FilteredEnv, double-checked locking) — HIGH confidence

---

*Pitfalls research for: Go WebSocket node replacing Telegram bot — multi-instance Claude CLI subprocess management*
*Researched: 2026-03-20 (v1.2 milestone)*
