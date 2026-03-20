# Phase 5: Fix Session Metrics and GSD Persistence - Research

**Researched:** 2026-03-20
**Domain:** Go session management — token/context capture from Claude subprocess events, OnQueryComplete wiring for GSD keyboard callbacks
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Token/context capture timing**
- Capture after `Stream()` returns, not during streaming — single point of capture at the same location where `sessionID` is already read (session.go ~line 351)
- Add `LastUsage() *UsageData` and `LastContextPercent() *int` accessor methods to the `Process` struct — symmetric with existing `SessionID()` pattern
- In `processMessage()`, read `proc.LastUsage()` and `proc.LastContextPercent()` after Stream() completes, write to `s.lastUsage` and `s.contextPercent` inside the existing mutex block
- On successful queries only — failed queries (errors, context limit) leave previous usage data intact rather than writing partial/misleading numbers

**Status display for fresh sessions**
- Omit the token usage and context percentage sections entirely when `LastUsage()` returns nil (fresh session, no queries run yet)
- Once a query completes, always show token section — matches existing nil-check pattern in `buildStatusText`

**GSD persistence wiring**
- Pass `*PersistenceManager` to `enqueueGsdCommand` and set `OnQueryComplete` on `WorkerConfig` — same pattern as text.go, voice.go, photo.go, document.go
- Thread `persist` param through the full callback chain: `HandleCallback` → `handleCallbackGsd/Resume/New/etc` → `enqueueGsdCommand` — matches how `wg` and `globalLimiter` were threaded in Phase 4
- Not a problem if worker was already started by text handler — `OnQueryComplete` is set at worker start time on `WorkerConfig`, persists for the worker's lifetime
- SavedSession uses same fields as text handler: channelID, workingDir, sessionID, timestamp, GSD command text as message preview — no special GSD flag

### Claude's Discretion
- Exact implementation of `Process.LastUsage()` / `Process.LastContextPercent()` storage (field vs computed)
- Whether to store the last result event on Process or extract usage/context separately during Stream()
- Test structure for the new capture logic

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| SESS-06 | Bot shows context window usage as a progress bar in status messages | `ClaudeEvent.ContextPercent()` already computes this from `modelUsage`; `Session.contextPercent` field exists with accessor; `buildStatusText` already reads it — just needs `processMessage()` to write it |
| SESS-07 | Bot tracks and displays token usage (input/output/cache) in /status | `ClaudeEvent.Usage *UsageData` on result events carries all four token counts; `Session.lastUsage` field exists with accessor; `buildStatusText` already reads it — just needs `processMessage()` to write it |
| PERS-01 | Bot saves session state (session ID, working dir, conversation context) to JSON | `PersistenceManager.Save()` fully works; text.go `OnQueryComplete` pattern works; `enqueueGsdCommand` in callback.go is the only call site missing this wiring |
</phase_requirements>

---

## Summary

Phase 5 closes two independent gaps that were left unfinished when the core infrastructure was built. Both gaps are small and surgical — they target specific empty blocks and missing parameters in already-functional code paths. No new infrastructure, packages, or types need to be created.

**Gap 1 — Session metrics:** `Session.lastUsage` and `Session.contextPercent` are declared fields with accessor methods, and `buildStatusText` already reads them. However, `processMessage()` in session.go never writes them. The `Process` struct captures the session ID from result events during `Stream()`, but does not capture `Usage` or `ModelUsage`. Adding two fields (`lastUsage *UsageData`, `lastContextPercent *int`) to `Process` and populating them in the streaming loop, then reading them in `processMessage()` after `Stream()` returns, closes the gap entirely.

**Gap 2 — GSD persistence:** `enqueueGsdCommand` in callback.go starts the worker goroutine without an `OnQueryComplete` closure, so GSD-triggered sessions are never persisted. The fix mirrors the pattern from text.go exactly: pass `*PersistenceManager` through the call chain from `HandleCallback` down to `enqueueGsdCommand`, and set `OnQueryComplete` on the `WorkerConfig`. The bot handler wrapper in `bot/handlers.go` already passes `b.persist` to `HandleCallback`, so no bot-layer changes are needed there.

**Primary recommendation:** Two focused edits — (1) add `lastUsage`/`lastContextPercent` capture to `Process.Stream()` and expose via accessors, then read in `processMessage()`; (2) thread `*PersistenceManager` into `enqueueGsdCommand` and set `OnQueryComplete` matching the text.go template exactly.

---

## Standard Stack

This phase operates entirely within the existing Go codebase. No new dependencies.

### Core (existing, no changes)
| Package | Purpose | Status |
|---------|---------|--------|
| `internal/claude` | Process struct, ClaudeEvent, UsageData, ContextPercent() | Extend: add two fields to Process |
| `internal/session` | Session struct, processMessage(), WorkerConfig | Extend: write lastUsage/contextPercent after Stream() |
| `internal/handlers/callback.go` | enqueueGsdCommand, HandleCallback | Extend: add persist param, set OnQueryComplete |
| `internal/session/persist.go` | PersistenceManager.Save() | Consume as-is |

### No New Dependencies

No new packages required. All types already imported. `sync.Mutex` protection already established.

---

## Architecture Patterns

### Existing Pattern: Process Accessor After Stream()

The established pattern for reading data from the completed `Process` after `Stream()` returns is already in use for `sessionID`:

```go
// process.go — existing
func (p *Process) SessionID() string {
    return p.sessionID
}

// process.go — inside Stream(), existing capture
if event.Type == "result" && event.SessionID != "" {
    p.sessionID = event.SessionID
}
```

New accessors for `lastUsage` and `lastContextPercent` follow this identical pattern:

```go
// process.go — new fields (inside Process struct)
lastUsage        *UsageData
lastContextPercent *int

// process.go — inside Stream(), new capture in the "result" event block
if event.Type == "result" {
    if event.SessionID != "" {
        p.sessionID = event.SessionID
    }
    if event.Usage != nil {
        p.lastUsage = event.Usage
    }
    if pct := event.ContextPercent(); pct != nil {
        p.lastContextPercent = pct
    }
}

// process.go — new accessor methods
func (p *Process) LastUsage() *UsageData {
    return p.lastUsage
}

func (p *Process) LastContextPercent() *int {
    return p.lastContextPercent
}
```

### Existing Pattern: processMessage() Success Block Write

The success branch in `processMessage()` already writes `sessionID` under mutex. The token/context data goes in the same block:

```go
// session.go — existing success block (lines ~369-378)
} else {
    // Success: update session ID and clear any previous error.
    if newSessionID != "" {
        s.sessionID = newSessionID
    }
    s.lastError = ""

    // NEW: capture usage from the completed process
    if u := proc.LastUsage(); u != nil {
        copy := *u
        s.lastUsage = &copy
    }
    if pct := proc.LastContextPercent(); pct != nil {
        copy := *pct
        s.contextPercent = &copy
    }
}
```

Key constraint: only write on success — the `ErrContextLimit` and general error branches above this must NOT set these fields.

### Existing Pattern: OnQueryComplete in WorkerConfig

The text.go pattern for `OnQueryComplete` is the template for the GSD path:

```go
// text.go — reference pattern (lines 154-177)
workerCfg := session.WorkerConfig{
    AllowedPaths: []string{mapping.Path},
    SafetyPrompt: config.BuildSafetyPrompt([]string{mapping.Path}),
    FilteredEnv:  config.FilteredEnv(),
    OnQueryComplete: func(sessionID string) {
        title := capturedText
        if len(title) > 50 {
            title = title[:50]
        }
        saved := session.SavedSession{
            SessionID:  sessionID,
            SavedAt:    time.Now().UTC().Format(time.RFC3339),
            WorkingDir: capturedMapping.Path,
            Title:      title,
            ChannelID:  capturedChatID,
        }
        if err := persist.Save(saved); err != nil {
            log.Warn().Err(err).Msg("Failed to persist session")
        }
    },
}
```

In `enqueueGsdCommand`, the same closure is added to `wCfg`. `capturedText` is the GSD command text (e.g. `/gsd:execute-phase 2`).

### Existing Pattern: Parameter Threading via HandleCallback

Phase 4 added `wg *sync.WaitGroup` and `globalLimiter *rate.Limiter` to `HandleCallback` and threaded them down. The same approach applies for `persist *session.PersistenceManager`:

```
HandleCallback(b, ctx, store, persist, cfg, mappings, awaitingPath, wg, globalLimiter)
  → handleCallbackGsd(... persist ...)
    → enqueueGsdCommand(... persist ...)
  → handleCallbackGsdPhase(... persist ...)
    → enqueueGsdCommand(... persist ...)
  → callbackActionGsdRun → enqueueGsdCommand(... persist ...)
  → callbackActionGsdFresh → enqueueGsdCommand(... persist ...)
  → callbackActionOption → enqueueGsdCommand(... persist ...)
  → handleCallbackAskUser(... persist ...)
    → enqueueGsdCommand(... persist ...)
```

`HandleCallback` already receives `persist *session.PersistenceManager` (line 107 of callback.go — confirmed in the source). The gap is that `persist` is never forwarded to `enqueueGsdCommand`.

### Worker Already Started Guard

When `enqueueGsdCommand` runs, the worker may already have been started by `HandleText`. The `workerStarted` guard prevents double-start. When the worker was started by `HandleText`, `OnQueryComplete` was already set on its `WorkerConfig` — meaning GSD commands routed to an already-running worker are already covered by the text-path persistence. The fix matters for cases where a GSD callback triggers the very first message in a session (worker not yet started) — `enqueueGsdCommand` starts the worker then, and must set `OnQueryComplete`.

### Anti-Patterns to Avoid

- **Writing usage data on error paths:** The `ErrContextLimit` and general error branches in `processMessage()` must not write `lastUsage`/`lastContextPercent`. Only the success branch writes them.
- **Non-atomic Process field reads:** `Process` fields (`lastUsage`, `lastContextPercent`) are written during `Stream()` and read after `Stream()` returns in the same goroutine — no mutex needed on `Process`. But the `Session` fields (`s.lastUsage`, `s.contextPercent`) are read by external callers via accessors and must be written under `s.mu`.
- **Pointer aliasing:** The `Session.lastUsage` field must store a copy of `*UsageData`, not the pointer from `Process`. The existing `LastUsage()` accessor already returns a copy — this only matters at write time.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead |
|---------|-------------|-------------|
| Token data parsing from Claude output | Custom regex/parser | `ClaudeEvent.Usage *UsageData` — already unmarshalled from NDJSON |
| Context percent calculation | Custom computation | `ClaudeEvent.ContextPercent()` — already implemented and tested |
| Atomic JSON persistence | Custom file write | `PersistenceManager.Save()` — atomic rename pattern already handles this |
| Per-project session trimming | Custom trim logic | `PersistenceManager` maxPerProject already trims on Save() |

---

## Common Pitfalls

### Pitfall 1: Writing Usage Data on Non-Success Paths
**What goes wrong:** If `s.lastUsage` is written in the `ErrContextLimit` branch or general error branch, `/status` shows misleading token counts from a failed/aborted query.
**Why it happens:** The Process successfully streams some events (including result events) before the context limit is detected in stderr. `proc.LastUsage()` may return non-nil even when `streamErr == claude.ErrContextLimit`.
**How to avoid:** Only write `s.lastUsage` and `s.contextPercent` in the `else` branch — the one that currently sets `s.sessionID` on success.
**Warning signs:** `/status` shows token counts after a "Context limit reached" error message.

### Pitfall 2: Process Field Thread Safety
**What goes wrong:** Accessing `p.lastUsage` from outside the goroutine that ran `Stream()` without a memory barrier.
**Why it happens:** Go memory model requires synchronization between writes in one goroutine and reads in another.
**How to avoid:** `Process` fields are always written during `Stream()` and read after `Stream()` returns in `processMessage()` — all in the same goroutine (the worker). No external code reads `Process` fields directly. No mutex needed on `Process`.
**Warning signs:** Race detector (`go test -race`) reports a data race on process fields.

### Pitfall 3: Missing Callers of enqueueGsdCommand
**What goes wrong:** `persist` param added to `enqueueGsdCommand` but some callers not updated — compilation error or some GSD paths still missing persistence.
**Why it happens:** `enqueueGsdCommand` is called from 5+ locations: `callbackActionGsdRun`, `callbackActionGsdFresh`, `handleCallbackGsd`, `handleCallbackGsdPhase`, `callbackActionOption`, `handleCallbackAskUser`. Missing any one causes a compile error.
**How to avoid:** The Go compiler enforces this — any call site with wrong arity fails to compile. Search for all `enqueueGsdCommand(` calls in callback.go and update all of them.
**Warning signs:** `go build` fails with "wrong number of arguments in call to enqueueGsdCommand".

### Pitfall 4: Worker Already Started — OnQueryComplete Not Updated
**What goes wrong:** The worker goroutine captures `WorkerConfig` by value at start time. If the worker was started by `HandleText` without `persist`, and a GSD command arrives later, `enqueueGsdCommand` skips the worker-start branch (`workerStarted == true`), and the existing worker runs without `OnQueryComplete`.
**Why it happens:** `WorkerConfig` is passed by value to `Worker()`. The closure is baked in when the goroutine starts — it cannot be retroactively updated.
**How to avoid:** This is acceptable by design per CONTEXT.md: "Not a problem if worker was already started by text handler — OnQueryComplete is set at worker start time on WorkerConfig, persists for the worker's lifetime." The text.go path already sets `OnQueryComplete` for workers it starts. The fix only covers workers started fresh by `enqueueGsdCommand` itself.
**Warning signs:** `/resume` only shows sessions from text-triggered queries, not from GSD-triggered queries that ran in a fresh session.

### Pitfall 5: handleCallbackGsd and handleCallbackGsdPhase Signature Gaps
**What goes wrong:** `handleCallbackGsd` and `handleCallbackGsdPhase` are internal helpers called by `HandleCallback`. They call `enqueueGsdCommand` but don't yet receive `persist`. Their signatures must also be extended.
**Why it happens:** Same threading pattern as Phase 4 for `wg`/`globalLimiter` — intermediate functions need the param even if they just forward it.
**How to avoid:** Extend `handleCallbackGsd`, `handleCallbackGsdPhase`, and `handleCallbackAskUser` signatures to accept `persist *session.PersistenceManager` and pass it through to `enqueueGsdCommand`.
**Warning signs:** Compile error "undefined: persist" in these intermediate functions.

---

## Code Examples

### Process Struct Extension (claude/process.go)

Add two fields to the `Process` struct and capture them in the streaming loop:

```go
// Source: internal/claude/process.go — new fields in Process struct
type Process struct {
    cmd               *exec.Cmd
    stdout            io.ReadCloser
    stderr            io.ReadCloser
    stderrBuf         strings.Builder
    contextLimitHit   bool
    sessionID         string
    lastUsage         *UsageData   // NEW
    lastContextPercent *int        // NEW
}
```

In `Stream()`, extend the existing `event.Type == "result"` block:

```go
// Source: internal/claude/process.go — inside Stream() scanning loop
if event.Type == "result" {
    if event.SessionID != "" {
        p.sessionID = event.SessionID
    }
    // NEW: capture usage and context percent from the result event
    if event.Usage != nil {
        p.lastUsage = event.Usage
    }
    if pct := event.ContextPercent(); pct != nil {
        p.lastContextPercent = pct
    }
    // existing context limit check follows...
    if isContextLimitError(event.Result) {
        p.contextLimitHit = true
    }
}
```

Add accessor methods after the existing `SessionID()` method:

```go
// Source: internal/claude/process.go — new accessors
func (p *Process) LastUsage() *UsageData {
    return p.lastUsage
}

func (p *Process) LastContextPercent() *int {
    return p.lastContextPercent
}
```

### processMessage() Success Block (session/session.go)

Add capture in the existing success `else` branch, after `s.sessionID = newSessionID`:

```go
// Source: internal/session/session.go — processMessage() success branch
} else {
    if newSessionID != "" {
        s.sessionID = newSessionID
    }
    s.lastError = ""
    // NEW: capture usage metrics from the completed process
    if u := proc.LastUsage(); u != nil {
        copy := *u
        s.lastUsage = &copy
    }
    if pct := proc.LastContextPercent(); pct != nil {
        copy := *pct
        s.contextPercent = &copy
    }
}
```

### enqueueGsdCommand Signature and OnQueryComplete (handlers/callback.go)

Add `persist *session.PersistenceManager` parameter and `OnQueryComplete` closure:

```go
// Source: internal/handlers/callback.go — enqueueGsdCommand
func enqueueGsdCommand(b *gotgbot.Bot, chatID int64, text string,
    store *session.SessionStore, mappings *project.MappingStore,
    cfg *config.Config, persist *session.PersistenceManager,  // NEW
    wg *sync.WaitGroup, globalLimiter *rate.Limiter) error {

    mapping, hasMapped := mappings.Get(chatID)
    // ... existing mapping check ...

    sess := store.GetOrCreate(chatID, mapping.Path)

    capturedText := text
    capturedChatID := chatID
    capturedMapping := mapping

    if !sess.WorkerStarted() {
        sess.SetWorkerStarted()
        wg.Add(1)
        go func(s *session.Session) {
            defer wg.Done()
            wCfg := session.WorkerConfig{
                AllowedPaths: []string{mapping.Path},
                SafetyPrompt: config.BuildSafetyPrompt([]string{mapping.Path}),
                FilteredEnv:  config.FilteredEnv(),
                OnQueryComplete: func(sessionID string) {  // NEW
                    title := capturedText
                    if len(title) > 50 {
                        title = title[:50]
                    }
                    saved := session.SavedSession{
                        SessionID:  sessionID,
                        SavedAt:    time.Now().UTC().Format(time.RFC3339),
                        WorkingDir: capturedMapping.Path,
                        Title:      title,
                        ChannelID:  capturedChatID,
                    }
                    if err := persist.Save(saved); err != nil {
                        log.Warn().Err(err).Msg("Failed to persist GSD session")
                    }
                },
            }
            s.Worker(context.Background(), cfg.ClaudeCLIPath, wCfg)
        }(sess)
    }
    // ... rest unchanged ...
}
```

Note: The `log` package import (`github.com/rs/zerolog/log`) may need to be added to callback.go if not already present.

---

## Integration Map

### What Already Works (do not change)

| Component | Status |
|-----------|--------|
| `Session.lastUsage` / `Session.contextPercent` fields | Declared with mutex, accessors exist — just never written |
| `Session.LastUsage()` / `Session.ContextPercent()` accessors | Return copies, nil-safe — ready to use |
| `buildStatusText()` token/context display | Already reads LastUsage/ContextPercent, omits lines when nil |
| `PersistenceManager.Save()` | Fully functional, per-project trimming works |
| `HandleCallback` signature | Already has `persist *session.PersistenceManager` (line 107) |
| `bot/handlers.go handleCallback` wrapper | Already passes `b.persist` — no changes needed |
| `ClaudeEvent.Usage *UsageData` unmarshalling | Already works, tested |
| `ClaudeEvent.ContextPercent()` computation | Already works, tested |

### What Needs Changing (precise locations)

| File | Location | Change |
|------|----------|--------|
| `internal/claude/process.go` | Process struct | Add `lastUsage *UsageData`, `lastContextPercent *int` |
| `internal/claude/process.go` | Stream() result event block | Capture usage and contextPercent from event |
| `internal/claude/process.go` | After SessionID() | Add `LastUsage()` and `LastContextPercent()` accessors |
| `internal/session/session.go` | processMessage() success else block | Read proc.LastUsage() and proc.LastContextPercent(), write to s fields under mutex |
| `internal/handlers/callback.go` | enqueueGsdCommand signature | Add `persist *session.PersistenceManager` param |
| `internal/handlers/callback.go` | enqueueGsdCommand worker start | Add OnQueryComplete closure matching text.go pattern |
| `internal/handlers/callback.go` | handleCallbackGsd signature | Thread persist param through |
| `internal/handlers/callback.go` | handleCallbackGsdPhase signature | Thread persist param through |
| `internal/handlers/callback.go` | handleCallbackAskUser signature | Thread persist param through |
| `internal/handlers/callback.go` | All call sites of enqueueGsdCommand | Add persist argument (5 locations) |

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | none (standard `go test ./...`) |
| Quick run command | `go test ./internal/claude/... ./internal/session/... ./internal/handlers/...` |
| Full suite command | `go test ./... -race` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SESS-07 | `proc.LastUsage()` returns UsageData captured from result event | unit | `go test ./internal/claude/... -run TestLastUsage` | ❌ Wave 0 |
| SESS-06 | `proc.LastContextPercent()` returns percent captured from result event | unit | `go test ./internal/claude/... -run TestLastContextPercent` | ❌ Wave 0 |
| SESS-07 | `processMessage()` writes lastUsage to session after successful Stream() | unit | `go test ./internal/session/... -run TestProcessMessageCapturesUsage` | ❌ Wave 0 |
| SESS-06 | `processMessage()` writes contextPercent to session after successful Stream() | unit | `go test ./internal/session/... -run TestProcessMessageCapturesContextPercent` | ❌ Wave 0 |
| SESS-07 | `processMessage()` does NOT write lastUsage on ErrContextLimit | unit | `go test ./internal/session/... -run TestProcessMessageNoUsageOnContextLimit` | ❌ Wave 0 |
| PERS-01 | `enqueueGsdCommand` calls OnQueryComplete which calls persist.Save on success | unit | `go test ./internal/handlers/... -run TestEnqueueGsdCommandPersists` | ❌ Wave 0 |
| PERS-01 | GSD-triggered session appears in LoadForChannel after query completes | integration | `go test ./internal/handlers/... -run TestGsdPersistenceEndToEnd` | ❌ Wave 0 |

### Existing Relevant Tests (pass today, must continue passing)
| File | Tests | Coverage |
|------|-------|---------|
| `internal/claude/events_test.go` | TestContextPercent, TestUnmarshalResultEvent | ClaudeEvent.Usage, ContextPercent() |
| `internal/session/session_test.go` | TestSessionIDAccessors | Pattern for accessor tests |
| `internal/handlers/command_test.go` | TestBuildStatusTextContextPercent, TestBuildStatusTextNoTokens | Display layer |
| `internal/handlers/callback_test.go` | TestCallbackRouteResume, TestResumeRestoresSessionID | Callback routing |

### Sampling Rate
- **Per task commit:** `go test ./internal/claude/... ./internal/session/... ./internal/handlers/...`
- **Per wave merge:** `go test ./... -race`
- **Phase gate:** Full suite green + race detector clean before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/claude/process_test.go` — add TestLastUsage, TestLastContextPercent covering capture from result events
- [ ] `internal/session/session_test.go` — add TestProcessMessageCapturesUsage, TestProcessMessageCapturesContextPercent, TestProcessMessageNoUsageOnContextLimit
- [ ] `internal/handlers/callback_test.go` — add TestEnqueueGsdCommandPersists testing OnQueryComplete fires when worker starts

Note: The session-level processMessage tests will require a fake Claude subprocess (fixture file pattern) similar to the approach in process_test.go. A simpler alternative is a new helper `setLastUsageForTest` on Session (unexported field write via test-package access since session_test.go is `package session`).

---

## Sources

### Primary (HIGH confidence)
- Direct source code inspection: `internal/claude/process.go` — Process struct, Stream() implementation, SessionID() accessor pattern
- Direct source code inspection: `internal/claude/events.go` — ClaudeEvent types, UsageData, ContextPercent() method
- Direct source code inspection: `internal/session/session.go` — Session struct fields, processMessage() success block, WorkerConfig.OnQueryComplete
- Direct source code inspection: `internal/handlers/callback.go` — HandleCallback signature (line 107 confirms persist already present), enqueueGsdCommand, all call sites
- Direct source code inspection: `internal/handlers/text.go` — OnQueryComplete reference implementation
- Direct source code inspection: `internal/session/persist.go` — PersistenceManager.Save() API
- Direct source code inspection: `internal/bot/handlers.go` — handleCallback wrapper confirming b.persist is already passed

### Secondary (MEDIUM confidence)
- `internal/handlers/command_test.go` — confirms buildStatusText display contract and nil-handling already tested
- `internal/claude/events_test.go` — confirms ContextPercent computation tested, UsageData unmarshalling tested

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all code inspected directly from source
- Architecture patterns: HIGH — patterns extracted from working code in the same codebase
- Integration points: HIGH — exact line references verified by reading source files
- Pitfalls: HIGH — derived from direct analysis of the success/error branches in processMessage()

**Research date:** 2026-03-20
**Valid until:** Stable (internal codebase, no external dependencies changing)
