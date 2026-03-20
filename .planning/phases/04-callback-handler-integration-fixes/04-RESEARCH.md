# Phase 4: Callback Handler Integration Fixes - Research

**Researched:** 2026-03-19
**Domain:** Go concurrency (sync.WaitGroup), callback handler wiring, Telegram API rate limiting
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DEPLOY-04 | Bot supports graceful shutdown — drains active sessions before stopping | FINDING-01 fix: callback workers must use bot WaitGroup, not package-level callbackWg |
| SESS-06 | Bot shows context window usage as a progress bar in status messages | Indirectly affected via FINDING-01 (callback workers can be killed mid-stream without drain) |
| PROJ-01 | Each Telegram channel maps to exactly one project (working directory) | FINDING-02 fix: handleCallbackResume/New must resolve mapping.Path, not cfg.WorkingDir |
| PROJ-03 | When bot receives a message from an unassigned channel, it prompts user to link a project | FINDING-02 fix: callback resume/new must honor per-channel mapping |
| PERS-03 | Session state persists across bot crashes and service restarts | FINDING-02 fix: GetOrCreate with wrong WorkingDir creates session in wrong directory, breaking persistence isolation |
| CORE-06 | Bot writes append-only audit log | FINDING-03 fix labeled as affecting CORE-06 per audit; rate limiter nil pass can cause 429s that silence audit events indirectly |
</phase_requirements>

---

## Summary

Phase 4 closes three specific integration bugs identified in the v1.0 milestone audit. All three are surgical changes to a single file — `internal/handlers/callback.go` — with no new abstractions required. The existing pattern from `HandleText` provides the exact model to follow for each fix.

**FINDING-01** (medium): `enqueueGsdCommand` uses a package-level `callbackWg` instead of the bot's `Bot.wg`. The fix is to thread `*sync.WaitGroup` from `HandleCallback` down to `enqueueGsdCommand` and replace `callbackWg.Add/Done` with the bot WaitGroup. The `callbackWg` package-level variable becomes unused and should be removed.

**FINDING-02** (medium): `handleCallbackResume` and `handleCallbackNew` call `store.GetOrCreate(chatID, cfg.WorkingDir)` instead of resolving the channel's mapping path first. Both functions already receive the `mappings *project.MappingStore` (indirectly — currently they do NOT receive it; it must be threaded through). The fix is to look up `mappings.Get(chatID)` and pass the resolved path to `GetOrCreate`. `handleCallbackNew` should also forward the mapping to `HandleNew` or replicate the same lookup pattern already in `HandleNew` in `command.go`.

**FINDING-03** (low): `enqueueGsdCommand` passes `nil` to `NewStreamingState` as the `globalLimiter`. The fix is to thread `*rate.Limiter` from `HandleCallback` down to `enqueueGsdCommand` via its call chain.

**Primary recommendation:** Make all three fixes in one commit to `internal/handlers/callback.go` plus the `bot/handlers.go` call site. Add three targeted unit tests that exercise each fix path without requiring live Telegram.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `sync.WaitGroup` | stdlib | Track goroutine lifetime for graceful shutdown | Already used in bot layer and text/voice/photo/document handlers |
| `golang.org/x/time/rate` | (project dep) | Telegram API rate limiter | Already used in `StreamingState.waitForRateLimit` |
| `github.com/user/gsd-tele-go/internal/project` | local | Resolve channel-to-path mapping | Already used in `enqueueGsdCommand` for the mapping lookup |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `context.Background()` | stdlib | Worker goroutine context | Same pattern as `HandleText` — workers outlive handler call |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Threading `*sync.WaitGroup` as param | Package-level callbackWg | Package-level is already the bug; threading matches the established HandleText pattern |
| Resolving mapping inside each callbackResume/New | Adding mappings param to callbackStop | Stop only needs the session store, not the mapping — no change needed there |

**Installation:** No new dependencies needed — all required packages already imported by the project.

---

## Architecture Patterns

### Established Pattern: HandleText

`HandleText` in `internal/handlers/text.go` is the canonical example for all three fixes:

```go
// text.go: wg threaded from bot layer
func HandleText(..., wg *sync.WaitGroup, mappings *project.MappingStore, ..., globalLimiter *rate.Limiter) error {
    mapping, hasMapped := mappings.Get(chatID)   // FINDING-02 fix pattern
    ...
    sess := store.GetOrCreate(chatID, mapping.Path)  // uses mapping.Path, not cfg.WorkingDir
    ...
    if !sess.WorkerStarted() {
        sess.SetWorkerStarted()
        wg.Add(1)                                // FINDING-01 fix pattern
        go func(s *session.Session, c session.WorkerConfig) {
            defer wg.Done()
            s.Worker(context.Background(), ...)
        }(sess, workerCfg)
    }
    ss := NewStreamingState(tgBot, chatID, globalLimiter)  // FINDING-03 fix pattern
    ...
}
```

### FINDING-01 Fix Pattern: Thread wg to enqueueGsdCommand

**Current (buggy):**
```go
// callback.go:391 — callbackWg is package-level, not tracked by Bot.Stop()
var callbackWg sync.WaitGroup

func enqueueGsdCommand(b *gotgbot.Bot, chatID int64, text string,
    store *session.SessionStore, mappings *project.MappingStore,
    cfg *config.Config) error {
    ...
    callbackWg.Add(1)
    go func(s *session.Session) {
        defer callbackWg.Done()
        ...
    }(sess)
```

**Fixed:**
```go
// Remove callbackWg package-level var.
// Add wg *sync.WaitGroup param to enqueueGsdCommand and all callers.
func enqueueGsdCommand(b *gotgbot.Bot, chatID int64, text string,
    store *session.SessionStore, mappings *project.MappingStore,
    cfg *config.Config, wg *sync.WaitGroup) error {
    ...
    wg.Add(1)
    go func(s *session.Session) {
        defer wg.Done()
        ...
    }(sess)
```

**Call site change in bot/handlers.go:**
```go
// handleCallback must thread b.WaitGroup()
func (b *Bot) handleCallback(tgBot *gotgbot.Bot, ctx *ext.Context) error {
    return bothandlers.HandleCallback(tgBot, ctx, b.store, b.persist, b.cfg,
        b.mappings, b.awaitingPath, b.WaitGroup(), b.globalAPILimiter)
}
```

### FINDING-02 Fix Pattern: Resolve mapping.Path in handleCallbackResume and handleCallbackNew

Both functions are currently called from `HandleCallback` with no mapping access:

**Current handleCallbackResume (buggy):**
```go
// callback.go:449 — uses cfg.WorkingDir regardless of channel project
func handleCallbackResume(..., cfg *config.Config, ...) error {
    sess := store.GetOrCreate(chatID, cfg.WorkingDir)  // WRONG
```

**Fixed:**
```go
// mappings must be threaded through HandleCallback -> handleCallbackResume
func handleCallbackResume(..., mappings *project.MappingStore, ...) error {
    workingDir := cfg.WorkingDir
    if m, ok := mappings.Get(chatID); ok {
        workingDir = m.Path
    }
    sess := store.GetOrCreate(chatID, workingDir)
```

Same pattern applies to `handleCallbackNew` at line 497.

Note: `HandleCallback` currently calls these without mappings:
```go
case callbackActionResume:
    return handleCallbackResume(b, ctx, store, cfg, chatID, msgID, payload)
case callbackActionNew:
    return handleCallbackNew(b, store, cfg, chatID, msgID)
```

Both call sites need `mappings` added as a parameter.

### FINDING-03 Fix Pattern: Thread globalLimiter to enqueueGsdCommand

**Current (buggy):**
```go
// callback.go:404 — nil bypasses Telegram API rate limiting
ss := NewStreamingState(b, chatID, nil)
```

**Fixed:**
```go
// Thread globalLimiter *rate.Limiter from HandleCallback -> enqueueGsdCommand
ss := NewStreamingState(b, chatID, globalLimiter)
```

`HandleCallback` already receives everything from the bot layer (wg and globalLimiter added by FINDING-01 fix). `enqueueGsdCommand` needs `*rate.Limiter` added to its signature.

### Complete Signature Changes

**`HandleCallback` new signature:**
```go
func HandleCallback(b *gotgbot.Bot, ctx *ext.Context, store *session.SessionStore,
    persist *session.PersistenceManager, cfg *config.Config,
    mappings *project.MappingStore, awaitingPath *AwaitingPathState,
    wg *sync.WaitGroup, globalLimiter *rate.Limiter) error
```

**`enqueueGsdCommand` new signature:**
```go
func enqueueGsdCommand(b *gotgbot.Bot, chatID int64, text string,
    store *session.SessionStore, mappings *project.MappingStore,
    cfg *config.Config, wg *sync.WaitGroup, globalLimiter *rate.Limiter) error
```

**`handleCallbackResume` new signature:**
```go
func handleCallbackResume(b *gotgbot.Bot, _ *ext.Context, store *session.SessionStore,
    cfg *config.Config, mappings *project.MappingStore,
    chatID, msgID int64, sessionID string) error
```

**`handleCallbackNew` new signature:**
```go
func handleCallbackNew(b *gotgbot.Bot, store *session.SessionStore,
    cfg *config.Config, mappings *project.MappingStore,
    chatID, msgID int64) error
```

### Anti-Patterns to Avoid

- **Do not create a new WaitGroup in callback.go**: The bot already owns `b.wg`; a package-level WaitGroup is invisible to `Bot.Stop()`.
- **Do not call `wg.Add` after the goroutine might have already exited**: The `WorkerStarted()` guard prevents double-start; `wg.Add(1)` must be called before `go func`.
- **Do not pass `mappings` only to `enqueueGsdCommand` but not to `handleCallbackResume/New`**: Each function is called from a different branch of the switch statement and needs direct access.
- **Do not use `cfg.WorkingDir` as a fallback when no mapping exists in handleCallbackResume/New**: Unlike the text handler, a callback resume only makes sense if a session previously existed for that channel. If no mapping is found, the same "no project linked" message used elsewhere is appropriate.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Graceful shutdown drain | Custom channel-based wait | `sync.WaitGroup` (already in bot.go) | WaitGroup is the idiomatic Go pattern; bot.Stop() already calls `b.wg.Wait()` with a 30s timeout |
| API rate limiting | Per-callback ticker | `*rate.Limiter` (already in StreamingState) | Rate limiter already wired in text/voice/photo/document paths; nil was the oversight |
| Project path resolution | Separate lookup function | `mappings.Get(chatID)` inline | Same pattern as HandleText, HandleNew, HandleStatus — no abstraction needed |

---

## Common Pitfalls

### Pitfall 1: Forgetting internal callers of enqueueGsdCommand

**What goes wrong:** `enqueueGsdCommand` is called from six places in `callback.go`:
- `handleCallbackGsd` (line 258)
- `callbackActionGsdRun` branch (line 165)
- `callbackActionGsdFresh` branch (line 178)
- `handleCallbackGsdPhase` (line 299)
- `callbackActionOption` branch (line 191)
- `handleCallbackAskUser` (line 368)

**Why it happens:** Adding a parameter to `enqueueGsdCommand` will cause a compile error at every call site inside the same package. Missing even one will prevent compilation.

**How to avoid:** After changing the signature, run `go build ./...` or `go vet ./...` immediately — compiler will identify all remaining call sites.

**Warning signs:** Compile errors listing line numbers for each `enqueueGsdCommand(...)` call.

### Pitfall 2: wg.Add / wg.Done race

**What goes wrong:** `wg.Add(1)` must be called in the calling goroutine, not inside the spawned goroutine. Calling `wg.Add(1)` inside `go func` creates a race where `wg.Wait()` in `Bot.Stop()` might be called before the `Add`.

**Why it happens:** Pattern copy error.

**How to avoid:** Follow the exact pattern in `HandleText` lines 183-188: `sess.SetWorkerStarted()`, then `wg.Add(1)`, then `go func` with `defer wg.Done()`.

### Pitfall 3: handleCallbackNew signature touches a different call site than Resume

**What goes wrong:** `handleCallbackNew` is called at line 148 as:
```go
return handleCallbackNew(b, store, cfg, chatID, msgID)
```
`handleCallbackResume` is called at line 142 as:
```go
return handleCallbackResume(b, ctx, store, cfg, chatID, msgID, payload)
```
Both are distinct; updating only one will compile but leave the other broken at runtime.

**How to avoid:** Update both switch case branches in the same edit.

### Pitfall 4: Removing callbackWg while the bot.Stop wait already references it

**What goes wrong:** If some code path still calls `callbackWg.Wait()`, removing the var causes a compile error that looks unrelated.

**Why it happens:** Not checking for all uses of the variable.

**How to avoid:** Grep for `callbackWg` across the package before removing. Currently the var is only used in lines 92, 391, and the implicit `Wait` would be in `Stop` — but `Bot.Stop()` only calls `b.wg.Wait()`, not `callbackWg.Wait()`, so there is no external `Wait` call. The var is safe to delete.

### Pitfall 5: handleCallbackResume/New no-mapping behavior

**What goes wrong:** If no mapping is found for the channel, calling `store.GetOrCreate(chatID, "")` creates a session with empty WorkingDir. This allows a broken session to exist.

**Why it happens:** Blind fallback to empty string.

**How to avoid:** Mirror the guard used in `HandleNew` (command.go:72-74): if no mapping and `cfg.WorkingDir` is also empty, send an error message and return.

---

## Code Examples

Verified patterns from the existing codebase:

### HandleText: wg + globalLimiter threading (source of truth)
```go
// internal/handlers/text.go:66-77
func HandleText(
    tgBot *gotgbot.Bot,
    ctx *ext.Context,
    store *session.SessionStore,
    cfg *config.Config,
    auditLog *audit.Logger,
    persist *session.PersistenceManager,
    wg *sync.WaitGroup,
    mappings *project.MappingStore,
    awaitingPath *AwaitingPathState,
    globalLimiter *rate.Limiter,
) error {
```

### HandleText: Worker start with bot WaitGroup (source of truth)
```go
// internal/handlers/text.go:180-188
if shouldStartWorker {
    sess.SetWorkerStarted()
    wg.Add(1)
    go func(s *session.Session, c session.WorkerConfig) {
        defer wg.Done()
        s.Worker(context.Background(), cfg.ClaudeCLIPath, c)
    }(sess, workerCfg)
}
```

### bot/handlers.go: how bot layer injects wg and globalLimiter
```go
// internal/bot/handlers.go:71-73
func (b *Bot) handleText(tgBot *gotgbot.Bot, ctx *ext.Context) error {
    return bothandlers.HandleText(tgBot, ctx, b.store, b.cfg, b.auditLog, b.persist,
        b.WaitGroup(), b.mappings, b.awaitingPath, b.globalAPILimiter)
}
```

### HandleNew: mapping-or-fallback pattern (mirrors what Resume/New callbacks need)
```go
// internal/handlers/command.go:66-76
workingDir := cfg.WorkingDir
if m, ok := mappings.Get(chatID); ok {
    workingDir = m.Path
}
if workingDir == "" {
    _, err := b.SendMessage(chatID, "No project linked. Use /project to link one.", nil)
    return err
}
sess := store.GetOrCreate(chatID, workingDir)
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `callbackWg` (package-level) | `wg *sync.WaitGroup` (injected from bot) | Phase 4 | Callback workers now drained by `Bot.Stop()` |
| `cfg.WorkingDir` in callback resume/new | `mapping.Path` resolved per channel | Phase 4 | Multi-project sessions resume in correct directory |
| `nil` globalLimiter in callback path | Injected `*rate.Limiter` | Phase 4 | Callback streaming respects global 25/sec Telegram limit |

**Deprecated/outdated:**
- `callbackWg` package-level var in `callback.go`: was a temporary placeholder documented with comment "// callbackWg is a no-op WaitGroup used by callback handlers"; Phase 4 removes it entirely.

---

## Open Questions

1. **Should enqueueGsdCommand accept the full WorkerConfig or just wg?**
   - What we know: The worker config (AllowedPaths, SafetyPrompt) is already built inline from `mapping.Path` inside `enqueueGsdCommand`. Only `wg` and `globalLimiter` are missing.
   - What's unclear: Whether to keep WorkerConfig construction in `enqueueGsdCommand` or lift it to callers.
   - Recommendation: Keep it in `enqueueGsdCommand` — it already has `mapping` in scope and all six callers benefit from one change point.

2. **Should handleCallbackNew also call persist.Save like the /new command does?**
   - What we know: `HandleNew` in command.go does not directly call persist.Save — persistence happens in the `OnQueryComplete` callback set up by `HandleText`. `handleCallbackNew` only clears the session ID; it relies on the next message going through `enqueueGsdCommand` which starts a worker.
   - What's unclear: Whether the `OnQueryComplete` is set on callback-spawned workers.
   - Recommendation: The callback worker starts with `WorkerConfig` built in `enqueueGsdCommand`, which does NOT set `OnQueryComplete`. This is a pre-existing limitation (callback workers don't auto-persist). Do NOT add persistence wiring in Phase 4 — the findings only require the path fix.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib |
| Config file | none (standard `go test`) |
| Quick run command | `"/c/Program Files/Go/bin/go" test ./internal/handlers/... -run TestCallback -v` |
| Full suite command | `"/c/Program Files/Go/bin/go" test ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DEPLOY-04 | Callback worker uses bot WaitGroup, not callbackWg | unit | `go test ./internal/handlers/... -run TestEnqueueGsdCommand_WgTracked` | ❌ Wave 0 |
| PROJ-01/PROJ-03/PERS-03 | handleCallbackResume uses mapping.Path, not cfg.WorkingDir | unit | `go test ./internal/handlers/... -run TestHandleCallbackResume_UsesMapping` | ❌ Wave 0 |
| PROJ-01/PROJ-03/PERS-03 | handleCallbackNew uses mapping.Path, not cfg.WorkingDir | unit | `go test ./internal/handlers/... -run TestHandleCallbackNew_UsesMapping` | ❌ Wave 0 |
| CORE-06 | enqueueGsdCommand passes globalLimiter to NewStreamingState | unit | `go test ./internal/handlers/... -run TestEnqueueGsdCommand_RateLimiter` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `"/c/Program Files/Go/bin/go" test ./internal/handlers/... -run TestCallback -v`
- **Per wave merge:** `"/c/Program Files/Go/bin/go" test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/handlers/callback_integration_test.go` — new file covering the four regression tests above
- [ ] No framework install needed — stdlib `testing` package

**Note on testability:** The callback handler functions (`handleCallbackResume`, `handleCallbackNew`, `enqueueGsdCommand`) call live Telegram API methods (`b.SendMessage`, `b.EditMessageText`). Tests that exercise the full functions require a mock `*gotgbot.Bot`. The existing test suite uses `parseCallbackData` (pure function) and avoids live bot calls. New tests should follow the same strategy: test the path-resolution logic in isolation (e.g., verify that `mappings.Get` is called and the result used) by extracting testable helper logic, or use a table-driven test that verifies the `WorkingDir` stored on the session after the callback handler applies it — using a real `SessionStore` but a stubbed bot.

The compile check (`go build ./...`) is itself a meaningful regression gate since all three fixes manifest as type errors if any call site is missed.

---

## Sources

### Primary (HIGH confidence)
- Direct code reading: `internal/handlers/callback.go` — current buggy implementation, all three findings
- Direct code reading: `internal/handlers/text.go` — canonical correct pattern for wg, mapping, globalLimiter
- Direct code reading: `internal/bot/bot.go` — Bot.wg, Bot.Stop(), Bot.WaitGroup(), Bot.GlobalAPILimiter()
- Direct code reading: `internal/bot/handlers.go` — call site pattern showing how bot layer injects wg and globalLimiter
- Direct code reading: `internal/handlers/callback_test.go` — existing test infrastructure
- `.planning/v1.0-MILESTONE-AUDIT.md` — authoritative finding descriptions with exact file:line references

### Secondary (MEDIUM confidence)
- Go `sync.WaitGroup` documentation (stdlib, well-established) — wg.Add/Done pattern

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries already in use; no new dependencies
- Architecture: HIGH — all three fixes are direct replications of the existing HandleText pattern
- Pitfalls: HIGH — derived from direct code reading, not speculation

**Research date:** 2026-03-19
**Valid until:** Indefinite — code-level research, valid until callback.go changes
