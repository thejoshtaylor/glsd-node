# Phase 6: Cross-Phase Safety Hardening - Research

**Researched:** 2026-03-20
**Domain:** Go bot handler safety infrastructure (typing indicators, audit logging, command safety)
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None — all implementation choices are at Claude's discretion.

### Claude's Discretion
All implementation choices. This is a pure infrastructure phase. The three safety mechanisms (typing indicators, audit logging, command safety checks) already exist and work in the text handler path; this phase wires them into the callback/GSD paths that currently lack them.

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| CORE-03 | Bot sends typing indicators while processing requests | `StartTypingIndicator` exists in `streaming.go`; must be called in `enqueueGsdCommand` and `handleCallbackResume`/`handleCallbackNew` before Claude work |
| CORE-06 | Bot writes append-only audit log (timestamp, user, channel, action, message excerpt) | `audit.Logger` and `audit.NewEvent` exist; `HandleCallback` lacks the `*audit.Logger` parameter; must be threaded through |
| AUTH-03 | Bot checks commands against blocked patterns for safety | `security.CheckCommandSafety` exists; `enqueueGsdCommand` sends text to Claude with no safety check; must add check before enqueueing |
</phase_requirements>

## Summary

Phase 6 is a pure wiring phase — no new infrastructure needs to be built. All three safety mechanisms exist and are proven in the text handler path (`text.go`). The gap is that `callback.go`'s `enqueueGsdCommand` function, which is the single convergence point for all GSD, option, resume-continuation, and askuser callbacks, bypasses all three safety layers.

The work is: (1) add `*audit.Logger` to `HandleCallback`'s signature and thread it to `enqueueGsdCommand`, (2) add a `StartTypingIndicator` call inside `enqueueGsdCommand` before `Enqueue`, and (3) add a `security.CheckCommandSafety` call inside `enqueueGsdCommand` before enqueueing the message. The typing indicator must stop in the streaming callback when the first response event arrives, exactly mirroring the text handler pattern.

Additionally, `handleCallbackNew` and `handleCallbackResume` need audit log entries written to record those lifecycle events, since they perform meaningful bot actions (starting fresh sessions, restoring sessions) that should appear in the audit trail.

**Primary recommendation:** Thread `*audit.Logger` into `HandleCallback` and `enqueueGsdCommand`, add `StartTypingIndicator` / `CheckCommandSafety` in `enqueueGsdCommand` following the exact text.go pattern, and add audit log entries in `handleCallbackNew` and `handleCallbackResume`.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `internal/audit` | project-local | Append-only NDJSON audit logger | Already in use in text.go; goroutine-safe via `sync.Mutex` |
| `internal/security` | project-local | `CheckCommandSafety` + `ValidatePath` | Already in use in text.go; pure function, no side effects |
| `internal/handlers` (streaming) | project-local | `StartTypingIndicator` / `TypingController` | Already in use in text.go, voice.go, photo.go, document.go |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `internal/config` | project-local | `BlockedPatterns` | Pass to `CheckCommandSafety` in callback path |
| `github.com/rs/zerolog/log` | already imported | Structured warning logging for blocked commands | Log blocked commands same as text.go does |

**Installation:** No new packages required. All dependencies are project-local or already imported.

## Architecture Patterns

### Recommended Project Structure
No structural changes. All changes are within existing files:
```
internal/handlers/
├── callback.go    # primary change file — enqueueGsdCommand, handleCallbackNew, handleCallbackResume
└── (no new files)
internal/bot/
└── handlers.go    # update handleCallback wrapper to pass b.auditLog
```

### Pattern 1: The Text Handler Safety Sequence (reference model)
**What:** The exact three-step sequence used in `text.go` that must be replicated in the callback path.
**When to use:** Before any message is enqueued to a Claude session.
**Example:**
```go
// Source: internal/handlers/text.go lines 91-194

// 1. AUDIT LOG — record the incoming event
if auditLog != nil {
    ev := audit.NewEvent("message", userID, chatID)
    excerpt := text
    if len(excerpt) > 100 {
        excerpt = excerpt[:100]
    }
    ev.Message = excerpt
    _ = auditLog.Log(ev)
}

// 2. COMMAND SAFETY CHECK — reject before touching Claude
safe, blockedPattern := security.CheckCommandSafety(text, config.BlockedPatterns)
if !safe {
    log.Warn().
        Int64("chat_id", chatID).
        Str("pattern", blockedPattern).
        Msg("Blocked message due to safety pattern")
    _, err := b.SendMessage(chatID, "Command blocked for safety: "+blockedPattern, nil)
    return err
}

// 3. TYPING INDICATOR — start before enqueueing
typingCtl := StartTypingIndicator(tgBot, chatID)

// Callback stops typing when first streaming event arrives:
qMsg := session.QueuedMessage{
    Callback: func(_ int64) claude.StatusCallback {
        typingCtl.Stop()           // <-- typing stops here
        return CreateStatusCallback(ss)
    },
    // ...
}
```

### Pattern 2: Typing Indicator Lifecycle in enqueueGsdCommand
**What:** Typing starts before `sess.Enqueue()`, stops inside the `Callback` closure when the first streaming event fires.
**When to use:** Exactly as in text.go — the `Callback` factory is the correct stop point.
**Example (adapted for callback path):**
```go
// Source: pattern from internal/handlers/text.go lines 193-207
typingCtl := StartTypingIndicator(b, chatID)

qMsg := session.QueuedMessage{
    Text:   text,
    ChatID: chatID,
    Callback: func(_ int64) claude.StatusCallback {
        typingCtl.Stop()
        return CreateStatusCallback(ss)
    },
    ErrCh: make(chan error, 1),
}

if !sess.Enqueue(qMsg) {
    typingCtl.Stop()  // also stop on queue-full path
    _, err := b.SendMessage(chatID, "Queue full...", nil)
    return err
}
```

### Pattern 3: Audit Log for Callback-Triggered Events
**What:** Write audit entries for meaningful callback actions: GSD commands, session lifecycle operations.
**When to use:** In `enqueueGsdCommand` (covers GSD/option/askuser paths) and in `handleCallbackNew`/`handleCallbackResume`.
**Action strings to use** (consistent with existing audit entries in text.go):
- `"callback_gsd"` — for any enqueue through `enqueueGsdCommand`
- `"callback_new"` — for `handleCallbackNew`
- `"callback_resume"` — for `handleCallbackResume`

**Note on userID in callbacks:** The `*ext.Context` is not passed down to `enqueueGsdCommand`. The `callbackAction` handler in `HandleCallback` has access to `cq.Sender` (the `gotgbot.User` on the CallbackQuery). The user ID should be extracted in `HandleCallback` and passed to the sub-handlers as a parameter, or `0` is an acceptable fallback for the audit field since auth has already passed.

### Pattern 4: Signature Changes Required
**What:** `HandleCallback` and `enqueueGsdCommand` need `*audit.Logger` added to their parameter lists.
**Why:** `audit.Logger` is currently held on `Bot` struct (`b.auditLog`). `HandleCallback` is called via `bot/handlers.go`'s `handleCallback` wrapper which already has access to `b.auditLog`.

Current signature:
```go
func HandleCallback(b *gotgbot.Bot, ctx *ext.Context, store *session.SessionStore,
    persist *session.PersistenceManager, cfg *config.Config,
    mappings *project.MappingStore, awaitingPath *AwaitingPathState,
    wg *sync.WaitGroup, globalLimiter *rate.Limiter) error
```

Required new signature:
```go
func HandleCallback(b *gotgbot.Bot, ctx *ext.Context, store *session.SessionStore,
    persist *session.PersistenceManager, cfg *config.Config,
    mappings *project.MappingStore, awaitingPath *AwaitingPathState,
    wg *sync.WaitGroup, globalLimiter *rate.Limiter,
    auditLog *audit.Logger) error  // <-- add
```

`enqueueGsdCommand` similarly needs `auditLog *audit.Logger` added and all call sites in `callback.go` updated.

The wrapper in `bot/handlers.go` line 100 must pass `b.auditLog`:
```go
func (b *Bot) handleCallback(tgBot *gotgbot.Bot, ctx *ext.Context) error {
    return bothandlers.HandleCallback(tgBot, ctx, b.store, b.persist, b.cfg,
        b.mappings, b.awaitingPath, b.WaitGroup(), b.globalAPILimiter, b.auditLog)
}
```

### Anti-Patterns to Avoid
- **Passing userID as 0 without comment:** If `cq.Sender` is nil (rare but possible for channel posts), userID will be 0. This is acceptable; add a comment explaining it.
- **Stopping typing indicator in the error drain goroutine only:** The typing indicator must also be stopped on the queue-full branch (before `return err`), as in text.go.
- **Adding safety check after enqueueing:** Safety check MUST happen before `sess.Enqueue()` — once enqueued, the message is already heading to Claude.
- **Adding a `StartTypingIndicator` call in `handleCallbackGsd`, `handleCallbackGsdPhase`, etc.:** All these paths converge on `enqueueGsdCommand`; add it once there, not in each caller.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Typing indicator goroutine | Custom ticker loop | `StartTypingIndicator` in streaming.go | Already implemented with proper 4s cadence and `stop` channel |
| Audit log writes | Direct file I/O | `audit.Logger.Log()` | Already goroutine-safe with mutex; handles encoding |
| Command safety check | Custom string scanning | `security.CheckCommandSafety` | Already handles case-insensitive substring matching across all `BlockedPatterns` |

**Key insight:** Zero new infrastructure. All building blocks are ready; this phase is exclusively about connecting them to the callback path.

## Common Pitfalls

### Pitfall 1: Typing Indicator Leak on Queue Full
**What goes wrong:** Typing indicator goroutine runs indefinitely if `sess.Enqueue()` returns false and `typingCtl.Stop()` is not called on that branch.
**Why it happens:** The error path returns early, bypassing the `Callback` closure where `Stop()` would normally be called.
**How to avoid:** Add `typingCtl.Stop()` immediately before returning on the queue-full branch (see text.go line 212).
**Warning signs:** Telegram showing "typing" for a chat indefinitely after a "Queue full" message.

### Pitfall 2: UserID Not Available Deep in Call Stack
**What goes wrong:** `enqueueGsdCommand` does not have access to `ctx.CallbackQuery.Sender` because `*ext.Context` is not passed to it.
**Why it happens:** The function was designed to take minimal params; it receives `chatID` but not `userID`.
**How to avoid:** Either (a) add `userID int64` parameter to `enqueueGsdCommand`, extracting it from `cq.Sender.Id` in `HandleCallback` before dispatch, or (b) pass `0` with a comment. Option (a) is cleaner for audit quality. If `cq.Sender` is nil, pass 0.
**Warning signs:** All callback audit entries have `user_id: 0`.

### Pitfall 3: Import Cycle Risk
**What goes wrong:** Adding `audit` import to `callback.go` triggers import cycle if audit imports handlers.
**Why it happens:** Go's strict import rules.
**How to avoid:** Verify that `internal/audit` has no dependency on `internal/handlers`. From reading the audit package (`log.go`), it only imports `encoding/json`, `os`, `sync`, `time` — no cycle risk.
**Warning signs:** `go build` fails with "import cycle not allowed".

### Pitfall 4: Test Coverage Gap
**What goes wrong:** The safety check is silently bypassed for specific callback types because the test only covers `enqueueGsdCommand` but not all branches that call it.
**Why it happens:** `handleCallbackAskUser`, `handleCallbackGsd`, `handleCallbackGsdPhase`, `callbackActionOption` all route through `enqueueGsdCommand`, so a single fix covers all — but tests should verify it.
**How to avoid:** Add a unit test that directly calls the safety-check logic for a blocked pattern and verifies it returns an error before enqueueing.

### Pitfall 5: handleCallbackResume Does Not Call enqueueGsdCommand
**What goes wrong:** `handleCallbackResume` only sets the session ID; it does not enqueue a message to Claude. It therefore needs no typing indicator or safety check. But it DOES need an audit log entry.
**Why it happens:** Resume is a lifecycle operation, not a Claude query trigger.
**How to avoid:** Add only the audit log entry (`"callback_resume"`) to `handleCallbackResume`. Do not add typing or safety check — they are irrelevant here.

### Pitfall 6: handleCallbackNew Does Not Call enqueueGsdCommand Either
**What goes wrong:** Same as Pitfall 5 — `handleCallbackNew` just clears the session ID. Typing and safety check are not needed.
**How to avoid:** Add only audit log entry (`"callback_new"`). No typing indicator or safety check.

## Code Examples

Verified patterns from the existing codebase:

### Audit log entry pattern (from text.go)
```go
// Source: internal/handlers/text.go lines 91-99
if auditLog != nil {
    ev := audit.NewEvent("callback_gsd", userID, chatID)
    excerpt := text
    if len(excerpt) > 100 {
        excerpt = excerpt[:100]
    }
    ev.Message = excerpt
    _ = auditLog.Log(ev)
}
```

### CheckCommandSafety call pattern (from text.go)
```go
// Source: internal/handlers/text.go lines 135-144
safe, blockedPattern := security.CheckCommandSafety(text, config.BlockedPatterns)
if !safe {
    log.Warn().
        Int64("chat_id", chatID).
        Int64("user_id", userID).
        Str("pattern", blockedPattern).
        Msg("Blocked message due to safety pattern")
    _, err := b.SendMessage(chatID, "Command blocked for safety: "+blockedPattern, nil)
    return err
}
```

### StartTypingIndicator with Callback.Stop pattern (from text.go)
```go
// Source: internal/handlers/text.go lines 193-215
typingCtl := StartTypingIndicator(tgBot, chatID)

qMsg := session.QueuedMessage{
    Text:   text,
    ChatID: chatID,
    Callback: func(_ int64) claude.StatusCallback {
        typingCtl.Stop()
        return CreateStatusCallback(ss)
    },
    ErrCh: make(chan error, 1),
}

if !sess.Enqueue(qMsg) {
    _, err := ctx.EffectiveMessage.Reply(tgBot,
        "Queue full, please wait for the current query to finish.", nil)
    typingCtl.Stop()
    return err
}
```

### Sender ID extraction from CallbackQuery (in HandleCallback)
```go
// Source: internal/handlers/callback.go — cq already available
var userID int64
if cq.Sender != nil {
    userID = cq.Sender.Id
}
// Pass userID to enqueueGsdCommand and other sub-handlers
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Safety only in text handler | Safety in all message paths | Phase 6 | Closes INT-03, INT-04, INT-05 from v1.0 audit |

**Gaps being closed (from v1.0 milestone audit):**
- INT-03: Callback-triggered Claude calls lack typing indicator
- INT-04: GSD/callback operations not written to audit log
- INT-05: Voice transcripts, photo captions, document content, GSD callback commands not checked by `CheckCommandSafety`

Note: The phase success criteria also mentions "voice transcripts, photo captions, document content" for CheckCommandSafety. Reviewing `voice.go`, `photo.go`, and `document.go` is needed during planning to confirm whether those handlers already call `CheckCommandSafety`. From reading the CONTEXT.md code insights, those handlers already use `StartTypingIndicator` but the safety check is not mentioned. The planner should include investigation and potential fix for those handlers too, or confirm they're out of scope (the phase description names them in the success criteria).

## Open Questions

1. **Do voice/photo/document handlers already call CheckCommandSafety?**
   - What we know: CONTEXT.md says "document, photo, voice handlers all use StartTypingIndicator already" — but says nothing about safety checks for those handlers
   - What's unclear: Whether INT-05 applies to just the callback path or also to media handlers
   - Recommendation: Plan should read voice.go, photo.go, document.go and add `CheckCommandSafety` if missing — the phase success criteria explicitly names "voice transcripts, photo captions, document content"

2. **Should blocked-command audit entries be written for callback-path blocks?**
   - What we know: text.go does not currently write an audit entry specifically for blocked commands (it logs via zerolog but not audit)
   - What's unclear: Whether to write a `"blocked_command"` audit event when `CheckCommandSafety` rejects
   - Recommendation: Follow the text.go pattern exactly — log via zerolog only, no separate audit entry for blocked commands. Keep behavior consistent across paths.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | none — `go test ./...` standard |
| Quick run command | `go test ./internal/handlers/... -run TestCallback -v` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CORE-03 | Callback path starts typing indicator before enqueue | unit | `go test ./internal/handlers/... -run TestTypingIndicatorCallback -v` | ❌ Wave 0 |
| CORE-06 | enqueueGsdCommand writes audit entry | unit | `go test ./internal/handlers/... -run TestAuditLogCallback -v` | ❌ Wave 0 |
| AUTH-03 | enqueueGsdCommand rejects blocked patterns | unit | `go test ./internal/handlers/... -run TestSafetyCheckCallback -v` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/handlers/... -v`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/handlers/callback_safety_test.go` — covers AUTH-03: `CheckCommandSafety` called before enqueue in `enqueueGsdCommand`
- [ ] `internal/handlers/callback_audit_test.go` (or add to existing `callback_test.go`) — covers CORE-06: audit entries written for callback actions
- [ ] Typing indicator test approach: `StartTypingIndicator` is a goroutine; test via a mock bot or by verifying `TypingController.Stop()` is called (testable by wrapping in a helper or using a fake `*gotgbot.Bot` that records `SendChatAction` calls)

*(Existing `callback_test.go` and `callback_integration_test.go` provide the pattern for unit tests in this package.)*

## Sources

### Primary (HIGH confidence)
- Direct source read: `internal/handlers/callback.go` — full implementation of callback handler and all sub-handlers
- Direct source read: `internal/handlers/text.go` — reference safety pattern (audit, CheckCommandSafety, StartTypingIndicator)
- Direct source read: `internal/handlers/streaming.go` — `StartTypingIndicator`, `TypingController`, `CreateStatusCallback`
- Direct source read: `internal/security/validate.go` — `CheckCommandSafety` signature and behavior
- Direct source read: `internal/audit/log.go` — `Logger`, `NewEvent`, `Log` API
- Direct source read: `internal/bot/bot.go` — `auditLog` field on `Bot` struct
- Direct source read: `internal/bot/handlers.go` — `handleCallback` wrapper signature; where `b.auditLog` must be passed
- Direct source read: `internal/config/config.go` — `BlockedPatterns` slice

### Secondary (MEDIUM confidence)
- Direct source read: `internal/handlers/callback_test.go` — establishes existing test patterns for callback package
- Direct source read: `.planning/phases/06-cross-phase-safety-hardening/06-CONTEXT.md` — user decisions and code insights

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries are project-local with full source available
- Architecture: HIGH — patterns are copied directly from existing text.go implementation
- Pitfalls: HIGH — derived from direct code reading, not inference

**Research date:** 2026-03-20
**Valid until:** Indefinite — project-internal code, not subject to upstream version changes
