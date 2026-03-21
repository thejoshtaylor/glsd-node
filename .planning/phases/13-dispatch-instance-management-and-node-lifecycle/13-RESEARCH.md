# Phase 13: Dispatch, Instance Management, and Node Lifecycle - Research

**Researched:** 2026-03-21
**Domain:** Go concurrency, subprocess management, command dispatch, graceful shutdown
**Confidence:** HIGH

## Summary

Phase 13 is a pure integration phase: it connects the already-working ConnectionManager (Phase 11) to the already-working Claude CLI subprocess manager (internal/claude/) via a new dispatch layer. All underlying primitives are in place — the work is wiring them together correctly.

The primary challenge is not "which library" but "how to coordinate concurrent goroutines" safely. There are three distinct concurrency problems: (1) fan-out from one inbound channel to N instance goroutines, (2) fan-in from N instance goroutines back to one outbound Send() channel, and (3) graceful shutdown that drains in-progress streams before killing subprocesses.

The audit package and rate limiter need adaptation: `audit.Event` still carries Telegram-era fields (`UserID int64`, `ChannelID int64`) that need to be replaced with node-oriented fields. The rate limiter keys on `int64` channel IDs but project-based rate limiting requires a string key. Both need changes before Phase 13 dispatch logic can use them correctly.

**Primary recommendation:** Build a single `internal/dispatch` package containing a `Dispatcher` struct. It owns the instance map, subscribes to `ConnectionManager.Receive()`, fans out to per-instance goroutines, fans in outbound events, and orchestrates graceful shutdown. Wire it in `main.go` alongside signal handling.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
All implementation choices are at Claude's discretion — pure infrastructure phase. Key areas:
- Dispatch architecture (how inbound frames from ConnectionManager route to handlers)
- Instance manager design (map of running instances, lifecycle tracking)
- Claude CLI spawning approach (leverage existing internal/claude/ package)
- Streaming output back to server (NDJSON events → protocol stream_event frames)
- Graceful shutdown orchestration (signal handling, instance draining)
- Audit log integration (extend existing internal/audit/ package)
- Rate limiting approach (leverage existing internal/security/ rate limiter)

### Claude's Discretion
All implementation choices.

### Deferred Ideas (OUT OF SCOPE)
None.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| PROTO-03 | Node receives and ACKs `run` commands before execution begins | Dispatcher reads Execute envelope from recvCh, sends ACK frame (type="ack", id=envelope.ID) before spawning goroutine |
| PROTO-04 | Node receives and handles `kill` commands to terminate specific instances | Dispatcher looks up instance by InstanceID in map, calls Process.Kill() or Session.Stop() |
| PROTO-05 | Node receives and responds to `status` queries with running instance list | Dispatcher builds NodeRegister payload from current instance map, calls ConnectionManager.Send() |
| INST-01 | Node spawns Claude CLI subprocess on `run` command with project working directory | claude.NewProcess() with WorkDir from ExecuteCmd.WorkDir; BuildArgs with optional SessionID resume |
| INST-02 | Each instance gets a UUID, included in every outgoing frame | InstanceID comes from ExecuteCmd.InstanceID (server-assigned); included in all StreamEvent/InstanceStarted/InstanceFinished/InstanceError payloads |
| INST-03 | Node streams Claude NDJSON output to server as `instance_chunk` events | StatusCallback wraps each ClaudeEvent as StreamEvent, marshals to Envelope, calls ConnectionManager.Send() |
| INST-04 | Node sends lifecycle events: `instance_started`, `instance_finished`, `instance_error` | Goroutine wrapping Process.Stream() sends InstanceStarted before Stream(), InstanceFinished or InstanceError after |
| INST-05 | Multiple Claude instances can run simultaneously in the same project directory | Each Execute spawns its own goroutine; no shared mutable state between instances; SessionStore already supports concurrent access |
| INST-06 | Node can kill a specific instance by ID on server command | KillCmd handler calls cancelFunc stored per instance; Process.Kill() if needed; emits InstanceError or InstanceFinished |
| INST-07 | Instances use `--resume SESSION_ID` to maintain persistent Claude sessions across restarts | ExecuteCmd.SessionID passed to claude.BuildArgs(); Session.SetSessionID() to persist; OnQueryComplete callback saves new sessionID |
| NODE-03 | Graceful shutdown drains active streams, kills remaining processes, sends disconnect | SIGINT/SIGTERM cancels context; Dispatcher.Stop() signals all instances; waits up to 10s with WaitGroup; ConnectionManager.Stop() last |
| NODE-04 | Per-project rate limiting on incoming `run` commands using token bucket | Need string-keyed rate limiter (current is int64); create ProjectRateLimiter wrapping golang.org/x/time/rate per project |
| NODE-05 | Structured logging with `node_id`, `instance_id`, `project` context fields | zerolog sub-loggers: log.With().Str("node_id",...).Str("instance_id",...).Str("project",...).Logger() |
| NODE-06 | Audit logging for all received commands with source and command type | audit.Event needs new fields; replace UserID/ChannelID with Source string and CommandType string |
</phase_requirements>

## Standard Stack

### Core (all already in go.mod — no new dependencies)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `golang.org/x/time/rate` | v0.8.0 (in go.mod via golang.org/x/time) | Token bucket rate limiter | Already used in security package; production-proven |
| `github.com/rs/zerolog` | v1.34.0 | Structured logging with sub-loggers | Already used throughout; `.With()` chain for context fields |
| `go.uber.org/goleak` | v1.3.0 | Goroutine leak detection in tests | Already used in all existing tests |
| `encoding/json` (stdlib) | Go 1.24 | Envelope marshal/unmarshal | Already used in protocol package |
| `os/signal` (stdlib) | Go 1.24 | SIGINT/SIGTERM handling | Already used in main.go |
| `sync` (stdlib) | Go 1.24 | WaitGroup for shutdown coordination | Standard Go pattern |

**No new dependencies required.** All needed primitives are already imported.

### Packages to Create
| Package | Path | Purpose |
|---------|------|---------|
| `dispatch` | `internal/dispatch/` | Dispatcher struct, command fan-out, lifecycle events, graceful shutdown |

### Packages to Modify
| Package | Change |
|---------|--------|
| `internal/audit/` | Replace Telegram fields (UserID int64, ChannelID int64) with node-oriented fields (Source string, CommandType string) |
| `internal/security/` | Add string-keyed `ProjectRateLimiter` (or generalize existing to string keys) |
| `main.go` | Wire Dispatcher + ConnectionManager, replace TODO comment with real startup/shutdown sequence |

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── dispatch/
│   ├── dispatcher.go      # Dispatcher struct, Run(), Stop(), handleExecute/Kill/Status
│   └── dispatcher_test.go # Table-driven tests with mock server
├── audit/
│   ├── log.go             # MODIFIED: new Event fields
│   └── log_test.go        # existing tests updated
└── security/
    ├── ratelimit.go        # MODIFIED: add ProjectRateLimiter (string keys)
    └── ratelimit_test.go   # existing tests extended
main.go                    # MODIFIED: full wiring
```

### Pattern 1: Dispatcher as Central Coordinator

**What:** A single `Dispatcher` struct owns the running instance map, reads from `ConnectionManager.Receive()` in a loop, dispatches each envelope type to a handler method.

**When to use:** Always — single point of control makes shutdown sequencing deterministic.

**Key fields:**
```go
// Source: project codebase conventions (zerolog + sync patterns throughout)
type Dispatcher struct {
    conn    *connection.ConnectionManager
    cfg     *config.Config
    nodeCfg *config.NodeConfig
    log     zerolog.Logger
    audit   *audit.Logger
    limiter *security.ProjectRateLimiter

    mu        sync.RWMutex
    instances map[string]*instanceState // key: InstanceID

    wg      sync.WaitGroup  // tracks all instance goroutines
    stopCh  chan struct{}    // closed by Stop()
}

type instanceState struct {
    instanceID string
    project    string
    sessionID  string          // current Claude session ID
    cancel     context.CancelFunc
    startedAt  time.Time
}
```

### Pattern 2: Per-Instance Goroutine with Fan-In to Send()

**What:** Each Execute command spawns one goroutine. That goroutine calls `claude.NewProcess` + `Process.Stream()`, converting each `ClaudeEvent` to a `StreamEvent` frame and calling `conn.Send()` directly. `ConnectionManager.Send()` is already goroutine-safe.

**Why this over session.Session.Worker:** The `Session.Worker` pattern queues messages serially. Phase 13 instances are one-shot executions (one prompt → stream until done), not queued multi-message sessions. The simpler direct goroutine approach matches the requirement.

**Execute flow:**
```go
// Source: project codebase (conn.Send + claude.NewProcess patterns)
func (d *Dispatcher) handleExecute(ctx context.Context, env *protocol.Envelope) {
    var cmd protocol.ExecuteCmd
    _ = env.Decode(&cmd)

    // 1. Rate limit check (PROTO-03 precondition)
    if !d.limiter.Allow(cmd.Project) {
        // send instance_error — rate limited
        return
    }

    // 2. ACK before execution begins (PROTO-03)
    d.sendACK(env.ID)

    // 3. Register instance state
    instCtx, cancel := context.WithCancel(ctx)
    d.mu.Lock()
    d.instances[cmd.InstanceID] = &instanceState{
        instanceID: cmd.InstanceID,
        project:    cmd.Project,
        sessionID:  cmd.SessionID,
        cancel:     cancel,
        startedAt:  time.Now(),
    }
    d.mu.Unlock()

    // 4. Spawn goroutine
    d.wg.Add(1)
    go d.runInstance(instCtx, cancel, cmd)
}
```

**runInstance:**
```go
func (d *Dispatcher) runInstance(ctx context.Context, cancel context.CancelFunc, cmd protocol.ExecuteCmd) {
    defer d.wg.Done()
    defer cancel()
    defer d.removeInstance(cmd.InstanceID)

    // Send instance_started
    d.sendEvent(protocol.TypeInstanceStarted, protocol.InstanceStarted{
        InstanceID: cmd.InstanceID,
        Project:    cmd.Project,
        SessionID:  cmd.SessionID,
    })

    args := claude.BuildArgs(cmd.SessionID, d.cfg.AllowedPaths, "", d.cfg.SafetyPrompt)
    proc, err := claude.NewProcess(ctx, d.cfg.ClaudeCLIPath, cmd.WorkDir, cmd.Prompt, args, config.FilteredEnv())
    if err != nil {
        d.sendEvent(protocol.TypeInstanceError, protocol.InstanceError{
            InstanceID: cmd.InstanceID,
            Error:      err.Error(),
        })
        return
    }

    // Stream: convert each ClaudeEvent to StreamEvent
    streamErr := proc.Stream(ctx, func(event claude.ClaudeEvent) error {
        raw, _ := json.Marshal(event)
        return d.sendEvent(protocol.TypeStreamEvent, protocol.StreamEvent{
            InstanceID: cmd.InstanceID,
            Data:       string(raw),
        })
    })

    // Persist new session ID
    if newID := proc.SessionID(); newID != "" {
        d.mu.Lock()
        if inst, ok := d.instances[cmd.InstanceID]; ok {
            inst.sessionID = newID
        }
        d.mu.Unlock()
    }

    if streamErr != nil && ctx.Err() == nil {
        d.sendEvent(protocol.TypeInstanceError, protocol.InstanceError{
            InstanceID: cmd.InstanceID,
            Error:      streamErr.Error(),
        })
    } else {
        d.sendEvent(protocol.TypeInstanceFinished, protocol.InstanceFinished{
            InstanceID: cmd.InstanceID,
            ExitCode:   0,
        })
    }
}
```

### Pattern 3: Graceful Shutdown Sequence (NODE-03)

**What:** Ordered shutdown that avoids zombie processes and satisfies the 10-second process-exit requirement.

**Sequence:**
```
SIGINT/SIGTERM received
  → cancel top-level context (propagates to all instance goroutines via instCtx)
  → each instance goroutine: Process.Kill() called when ctx.Done() fires in Stream()
  → d.wg.Wait() with 10s timeout — waits for all instance goroutines to exit
  → ConnectionManager.Stop() — sends NodeDisconnect frame, closes WebSocket
  → os.Exit(0) or return from main()
```

**Critical:** `context.WithCancel` is passed to `claude.NewProcess`. When the context is cancelled, `exec.CommandContext` sends SIGKILL to the process. Combined with `cmd.WaitDelay = 5s` (already set in process.go), goroutines blocked on pipe reads will unblock within 5s.

**main.go wiring:**
```go
// Source: project pattern (existing signal handling in main.go)
ctx, cancel := context.WithCancel(context.Background())

connMgr := connection.NewConnectionManager(nodeCfg, log)
connMgr.Start(ctx)

dispatcher := dispatch.New(connMgr, cfg, nodeCfg, auditLog, rateLimiter, log)
go dispatcher.Run(ctx)

sig := <-sigCh
log.Info().Str("signal", sig.String()).Msg("shutting down")
cancel()

// Give instances up to 10s to drain
done := make(chan struct{})
go func() { dispatcher.Wait(); close(done) }()
select {
case <-done:
case <-time.After(10 * time.Second):
    log.Warn().Msg("shutdown timeout: forcing exit")
}

connMgr.Stop()
```

### Pattern 4: String-Keyed Rate Limiter

**What:** The existing `ChannelRateLimiter` keys on `int64` (Telegram channel IDs). Project rate limiting needs string keys.

**Solution:** Add `ProjectRateLimiter` to `internal/security/ratelimit.go` — same token bucket logic, string key:

```go
// Source: existing ChannelRateLimiter pattern in ratelimit.go
type ProjectRateLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter
    limit    rate.Limit
    burst    int
}

func NewProjectRateLimiter(requestsPerWindow, windowSeconds int) *ProjectRateLimiter {
    r := rate.Limit(float64(requestsPerWindow) / float64(windowSeconds))
    return &ProjectRateLimiter{
        limiters: make(map[string]*rate.Limiter),
        limit:    r,
        burst:    requestsPerWindow,
    }
}

func (p *ProjectRateLimiter) Allow(project string) bool {
    p.mu.Lock()
    l, ok := p.limiters[project]
    if !ok {
        l = rate.NewLimiter(p.limit, p.burst)
        p.limiters[project] = l
    }
    p.mu.Unlock()
    return l.Allow()
}
```

### Pattern 5: Audit Event Redesign

**What:** `audit.Event` has Telegram fields that are now wrong. Must replace before Phase 13 audit integration.

**New Event struct:**
```go
// Replace current audit.Event — drop UserID/ChannelID, add node fields
type Event struct {
    Timestamp   string `json:"timestamp"`
    Action      string `json:"action"`      // e.g. "execute", "kill", "status_request"
    Source      string `json:"source"`      // server-assigned command ID or origin identifier
    NodeID      string `json:"node_id"`
    InstanceID  string `json:"instance_id,omitempty"`
    Project     string `json:"project,omitempty"`
    Error       string `json:"error,omitempty"`
}

func NewEvent(action, source, nodeID string) Event {
    return Event{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Action:    action,
        Source:    source,
        NodeID:    nodeID,
    }
}
```

**Note:** The existing `log_test.go` test for `audit` must be updated after this change.

### Anti-Patterns to Avoid

- **Do not use session.Session.Worker for Phase 13 instances.** The Worker pattern queues messages serially and carries session persistence logic suited to the Telegram era. Phase 13 instances are one-shot — spawn, stream, done. The direct goroutine pattern is simpler and maps cleanly to the protocol model.
- **Do not lock the instance map while calling Send().** `conn.Send()` can block if the channel is full. Holding a write lock during Send() causes deadlock if another goroutine tries to acquire the lock while Send() is waiting.
- **Do not call `Process.Kill()` without context cancellation.** Always cancel the process context first so `exec.CommandContext` handles cleanup; only call `Kill()` as a belt-and-suspenders fallback for the kill command handler.
- **Do not drop the WaitGroup count.** Every `wg.Add(1)` before `go runInstance()` must have a corresponding `defer wg.Done()` at the top of the goroutine — before any early return.
- **Do not marshal the full ClaudeEvent as the StreamEvent.Data payload.** The protocol defines `Data string` — marshal the ClaudeEvent to JSON first, then use the JSON string as Data. This keeps the protocol clean and the server able to re-parse the NDJSON.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Token bucket rate limiting | Custom counter with time.Ticker | `golang.org/x/time/rate` (already in go.mod) | Rate reset races, burst handling, atomic operations — solved |
| WebSocket write serialization | Multiple goroutines calling conn.Write | `ConnectionManager.Send()` (already done) | coder/websocket panics on concurrent writes; already solved in Phase 11 |
| Subprocess cleanup | Custom SIGKILL loop | `exec.CommandContext` + `cmd.WaitDelay` (already in process.go) | WaitDelay already set to 5s; context cancellation triggers SIGKILL automatically |
| Thread-safe instance map | sync.Map | plain `map` + `sync.RWMutex` | sync.Map has worse ergonomics for typed values; RWMutex is idiomatic Go |
| UUID generation | uuid library | Server-assigned InstanceID from ExecuteCmd | The server already assigns the UUID (per protocol design); node just echoes it |

**Key insight:** This phase is pure wiring. Every sub-problem has already been solved in an existing package. The risk is in coordination (goroutine lifecycle, shutdown ordering), not in any individual operation.

## Common Pitfalls

### Pitfall 1: ACK Before or After Goroutine Spawn?
**What goes wrong:** Sending the ACK inside `runInstance()` means the goroutine hasn't started yet when the ACK is sent, but there's a race window where the server could send a `kill` before the instance is registered in the map.
**Why it happens:** goroutine scheduling is non-deterministic.
**How to avoid:** Register the instanceState in the map BEFORE spawning the goroutine, and send the ACK AFTER registration but BEFORE the goroutine starts. The kill handler will find the instance in the map immediately.
**Warning signs:** Test where kill arrives immediately after execute causes "instance not found" log.

### Pitfall 2: Instance Map Lock During Send
**What goes wrong:** Holding `d.mu.Lock()` while calling `d.sendEvent()` (which calls `conn.Send()`) deadlocks if `conn.Send()` blocks and another goroutine calls any method that acquires `d.mu`.
**Why it happens:** `conn.Send()` blocks on a channel send; the instance goroutine may need the lock to update sessionID.
**How to avoid:** Never hold `d.mu` across `conn.Send()`. Collect needed data under lock, release lock, then send.
**Warning signs:** Test hangs with goroutine dump showing `d.mu.Lock()` and `conn.Send()` waiting on each other.

### Pitfall 3: WaitGroup Done Before Cleanup
**What goes wrong:** `wg.Done()` called before `removeInstance()` means `dispatcher.Wait()` returns while the instance is still in the map, causing a false status response.
**Why it happens:** Defer ordering — `defer wg.Done()` added after `defer removeInstance()` runs Done before Remove.
**How to avoid:** Use explicit defer ordering: `defer removeInstance()` first (runs last), `defer wg.Done()` second (runs first). Or: `defer func() { removeInstance(); wg.Done() }()`.
**Warning signs:** Status response shows instances that should be finished.

### Pitfall 4: Kill Command Race with Natural Completion
**What goes wrong:** Kill command arrives; goroutine also completes naturally at the same instant. Two outbound events emitted: `InstanceError` (from kill) and `InstanceFinished` (from natural completion).
**Why it happens:** Two code paths both try to send the lifecycle terminal event.
**How to avoid:** Use a `sync.Once` per instanceState to gate the terminal event send. First caller wins; second is no-op.
**Warning signs:** Server receives two terminal events for the same InstanceID.

### Pitfall 5: Shutdown Context Cancels Register Before ACK Sends
**What goes wrong:** During shutdown, the top-level context is cancelled. If the ACK frame and the InstanceStarted event use `ctx` directly, context cancellation causes `conn.Send()` to return `ErrStopped` before the frames are sent.
**Why it happens:** `conn.Send()` checks `stopCh` and `stopped` — both still open during graceful drain. But if the child context is cancelled first, `instCtx` is Done, and any `ctx.Err()` check in the code returns non-nil.
**How to avoid:** Send ACK and InstanceStarted using `context.Background()` or a dedicated send-only context that outlasts the instance context. Use the instance context only for the process itself.
**Warning signs:** Audit logs show received Execute commands with no corresponding ACK or InstanceStarted in server logs.

### Pitfall 6: audit.Event Compilation Breakage
**What goes wrong:** Changing `audit.Event` fields breaks all call sites of `audit.NewEvent()` across existing tests.
**Why it happens:** `audit.NewEvent` takes positional args (`action, userID, channelID`).
**How to avoid:** Update `audit.NewEvent` signature in the same plan wave as all call sites. There are no call sites in Phase 12-cleaned code except `log_test.go` — update both atomically.
**Warning signs:** `go build ./...` fails with "too many arguments to NewEvent".

## Code Examples

### Sending an Outbound Envelope
```go
// Source: protocol.Encode() + ConnectionManager.Send() — both exist in codebase
func (d *Dispatcher) sendEvent(msgType string, payload any) error {
    msgID := generateMsgID() // or use envelope.ID for correlation
    env, err := protocol.Encode(msgType, msgID, payload)
    if err != nil {
        d.log.Error().Err(err).Str("type", msgType).Msg("failed to encode envelope")
        return err
    }
    data, err := json.Marshal(env)
    if err != nil {
        return err
    }
    return d.conn.Send(data)
}
```

Note: `generateMsgID()` is unexported in `connection` package. The dispatcher should either use `crypto/rand` directly or the protocol package should expose a helper. The simplest approach: copy the 16-byte hex pattern into dispatch package.

### Dispatcher Main Loop
```go
// Source: ConnectionManager.Receive() returns <-chan *protocol.Envelope
func (d *Dispatcher) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-d.stopCh:
            return
        case env, ok := <-d.conn.Receive():
            if !ok {
                return
            }
            d.dispatch(ctx, env)
        }
    }
}

func (d *Dispatcher) dispatch(ctx context.Context, env *protocol.Envelope) {
    // Audit every inbound command (NODE-06)
    d.auditCommand(env)

    switch env.Type {
    case protocol.TypeExecute:
        d.handleExecute(ctx, env)
    case protocol.TypeKill:
        d.handleKill(env)
    case protocol.TypeStatusRequest:
        d.handleStatusRequest()
    default:
        d.log.Warn().Str("type", env.Type).Msg("unknown envelope type, ignoring")
    }
}
```

### Kill Handler
```go
// Source: instanceState.cancel pattern; Process.Kill() from process.go
func (d *Dispatcher) handleKill(env *protocol.Envelope) {
    var cmd protocol.KillCmd
    if err := env.Decode(&cmd); err != nil {
        d.log.Error().Err(err).Msg("failed to decode KillCmd")
        return
    }

    d.mu.RLock()
    inst, ok := d.instances[cmd.InstanceID]
    d.mu.RUnlock()

    if !ok {
        d.log.Warn().Str("instance_id", cmd.InstanceID).Msg("kill: instance not found")
        return
    }

    // Cancel the instance context — triggers cmd.Context cancellation in claude.NewProcess
    inst.cancel()
    // InstanceError/InstanceFinished will be sent by the instance goroutine when it exits
}
```

### Status Response
```go
// Source: protocol.NodeRegister + InstanceSummary types from messages.go
func (d *Dispatcher) handleStatusRequest() {
    d.mu.RLock()
    summaries := make([]protocol.InstanceSummary, 0, len(d.instances))
    for _, inst := range d.instances {
        summaries = append(summaries, protocol.InstanceSummary{
            InstanceID: inst.instanceID,
            Project:    inst.project,
            SessionID:  inst.sessionID,
        })
    }
    d.mu.RUnlock()

    reg := protocol.NodeRegister{
        NodeID:           d.nodeCfg.NodeID,
        Platform:         runtime.GOOS,
        Version:          protocol.Version,
        Projects:         d.projectList(),
        RunningInstances: summaries,
    }
    _ = d.sendEvent(protocol.TypeNodeRegister, reg)
}
```

### Zerolog Sub-Logger Context Fields (NODE-05)
```go
// Source: zerolog documentation; consistent with existing zerolog usage
instanceLog := d.log.With().
    Str("node_id", d.nodeCfg.NodeID).
    Str("instance_id", cmd.InstanceID).
    Str("project", cmd.Project).
    Logger()
instanceLog.Info().Msg("instance started")
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Telegram channel ID as session key | String instance ID as session key | Phase 12 | SessionStore already uses string keys — ready for Phase 13 |
| audit.Event with UserID/ChannelID | Needs node-oriented fields | Phase 13 | Breaking change to audit package; all call sites in tests |
| ChannelRateLimiter keyed on int64 | ProjectRateLimiter needed for string keys | Phase 13 | New struct in same file; old struct stays for backward compat |
| main.go TODO comment | Full ConnectionManager + Dispatcher wiring | Phase 13 | main.go grows to ~80 lines of real startup code |

**Deprecated/outdated:**
- `session.Session.Worker`: The queued worker pattern was designed for Telegram multi-message sessions. Phase 13 instances are one-shot — use direct goroutines, not the Worker queue.
- `audit.NewEvent(action, userID, channelID)` signature: Must be replaced with node-appropriate signature in this phase.

## Open Questions

1. **Should `generateMsgID()` be exported from the connection package or duplicated?**
   - What we know: It's currently unexported in `connection/manager.go`; the dispatcher needs to assign IDs to outbound frames
   - What's unclear: Whether a shared `protocol.NewMsgID()` helper is cleaner than duplicating the 5-line function
   - Recommendation: Add `protocol.NewMsgID()` exported function in `internal/protocol/messages.go` — it's a protocol concern, and the connection package can use it too

2. **Should the ACK message use the same `id` as the Execute envelope (for correlation)?**
   - What we know: The protocol defines `id` on Envelope but doesn't specify ACK correlation behavior explicitly
   - What's unclear: Whether the server matches ACK by `id` or by InstanceID in payload
   - Recommendation: Use `env.ID` as the ACK envelope's `id` field for correlation, include InstanceID in ACK payload so the server can match either way

3. **Exit code on SIGTERM kill — what to report in InstanceFinished?**
   - What we know: `Process.Kill()` sends SIGTERM on Unix; the process may exit with -1 or 137 depending on OS
   - What's unclear: Whether to report the real exit code or a sentinel value
   - Recommendation: Report `proc.cmd.ProcessState.ExitCode()` from `cmd.Wait()` result, which will be -1 on Unix kill; document this in the InstanceFinished comment

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib testing (go test) |
| Config file | none (standard go test) |
| Quick run command | `go test ./internal/dispatch/... -timeout 30s` |
| Full suite command | `go test ./... -race -timeout 120s` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PROTO-03 | Execute received, ACK sent before goroutine runs | unit | `go test ./internal/dispatch/... -run TestExecuteACKBeforeStart -v` | Wave 0 |
| PROTO-04 | Kill command terminates the targeted instance | unit | `go test ./internal/dispatch/... -run TestKillInstance -v` | Wave 0 |
| PROTO-05 | Status request returns current running instances | unit | `go test ./internal/dispatch/... -run TestStatusRequest -v` | Wave 0 |
| INST-01 | Claude subprocess spawned with correct workdir | unit | `go test ./internal/dispatch/... -run TestSpawnWithWorkDir -v` | Wave 0 |
| INST-02 | InstanceID present in every outbound frame | unit | `go test ./internal/dispatch/... -run TestInstanceIDInAllFrames -v` | Wave 0 |
| INST-03 | NDJSON events forwarded as stream_event frames | unit | `go test ./internal/dispatch/... -run TestStreamEventForwarding -v` | Wave 0 |
| INST-04 | instance_started + instance_finished/error emitted | unit | `go test ./internal/dispatch/... -run TestLifecycleEvents -v` | Wave 0 |
| INST-05 | Two simultaneous instances both execute | unit | `go test ./internal/dispatch/... -run TestConcurrentInstances -v` | Wave 0 |
| INST-06 | Kill targeted instance, others unaffected | unit | `go test ./internal/dispatch/... -run TestKillOneInstance -v` | Wave 0 |
| INST-07 | --resume passed when SessionID non-empty | unit | `go test ./internal/dispatch/... -run TestResumeSession -v` | Wave 0 |
| NODE-03 | Graceful shutdown: all subprocesses exit within 10s | unit | `go test ./internal/dispatch/... -run TestGracefulShutdown -timeout 30s` | Wave 0 |
| NODE-04 | Rate limit rejects excess run commands per project | unit | `go test ./internal/security/... -run TestProjectRateLimiter -v` | Wave 0 |
| NODE-05 | Structured log entries contain node_id/instance_id/project | unit | `go test ./internal/dispatch/... -run TestStructuredLogging -v` | Wave 0 |
| NODE-06 | All commands appear in audit log with source+type | unit | `go test ./internal/dispatch/... -run TestAuditLogging -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/dispatch/... -race -timeout 30s`
- **Per wave merge:** `go test ./... -race -timeout 120s`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/dispatch/dispatcher.go` — package doesn't exist yet
- [ ] `internal/dispatch/dispatcher_test.go` — all dispatch tests
- [ ] `internal/security/ratelimit.go` — add `ProjectRateLimiter` (string keys)
- [ ] `internal/audit/log.go` — update `Event` struct and `NewEvent()` signature
- [ ] `internal/audit/log_test.go` — update after Event redesign

All test files in Wave 0 because the `dispatch` package is new. Existing test files for `audit` and `security` need updates but are not new.

## Sources

### Primary (HIGH confidence)
- Direct source code inspection: `internal/connection/manager.go`, `internal/protocol/messages.go`, `internal/claude/process.go`, `internal/session/store.go` — full API surface confirmed
- Direct source code inspection: `internal/audit/log.go`, `internal/security/ratelimit.go` — confirmed adaptation requirements
- Go 1.24 stdlib: `os/signal`, `sync`, `context`, `os/exec` — standard library behavior

### Secondary (MEDIUM confidence)
- Go stdlib `exec.CommandContext` documentation: context cancellation sends SIGKILL to process group; WaitDelay caps I/O goroutine wait time
- zerolog sub-logger pattern: `.With().Str(...).Logger()` — verified against zerolog v1.34.0 usage in existing code

### Tertiary (LOW confidence — not needed, all patterns confirmed from source)
None required. All patterns verified from the existing codebase and Go stdlib.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all packages already in go.mod and in use
- Architecture: HIGH — all integration points verified by source code inspection
- Pitfalls: HIGH — derived from actual code paths (lock ordering, goroutine lifecycle, defer ordering)
- Test map: HIGH — test names are prescriptive, not exploratory

**Research date:** 2026-03-21
**Valid until:** 2026-04-21 (stable Go stdlib; no external library changes expected)
