# Phase 16: Instance Lifecycle Fixes - Research

**Researched:** 2026-03-25
**Domain:** Go subprocess management, exit code extraction, protocol message enrichment
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
All implementation choices are at Claude's discretion — discuss phase was skipped per user setting. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Claude's Discretion
All implementation choices.

### Deferred Ideas (OUT OF SCOPE)
None — discuss phase skipped.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| INST-04 | Node sends lifecycle events: `instance_started`, `instance_finished`, `instance_error` | `instance_started` and `instance_error` already implemented. Gap: `InstanceFinished.ExitCode` is hardcoded `0` and `InstanceFinished` lacks `SessionID`. Fix is in `dispatcher.go` `runInstance()`. |
| INST-07 | Instances use `--resume SESSION_ID` to maintain persistent Claude sessions across restarts | `claude.BuildArgs()` already adds `--resume` when `sessionID != ""`. Gap: the server never learns the **new** session ID assigned during a fresh session, because `InstanceFinished` does not carry `SessionID`. Fix: add `SessionID` field to `InstanceFinished` and populate it from `proc.SessionID()`. |
</phase_requirements>

---

## Summary

Phase 16 closes two gaps in the instance lifecycle event system. Both gaps are small, precisely located, and require changes to three files: `internal/protocol/messages.go`, `internal/dispatch/dispatcher.go`, and the two documentation files in `docs/`.

**Gap 1 (INST-04 partial):** `InstanceFinished.ExitCode` is always sent as `0` (hardcoded). The real process exit code is available from `cmd.Wait()` via Go's `exec.ExitError`. `Process.Stream()` returns the `cmd.Wait()` error, which is an `*exec.ExitError` when the process exits non-zero. The fix is: in `runInstance()`, type-assert the `streamErr` to `*exec.ExitError` and extract `.ExitCode()` before emitting `InstanceFinished`.

**Gap 2 (INST-07 completion):** `InstanceFinished` has no `SessionID` field. The server cannot learn the session ID assigned by Claude during a fresh run (no `--resume` was given, so a new session ID is minted). The process already captures this in `proc.SessionID()`. The fix is: add `SessionID string` to `protocol.InstanceFinished`, populate it from `proc.SessionID()` (or `inst.sessionID` which is already updated from `proc.SessionID()` just above the terminal event emission), and update the two spec docs.

**Primary recommendation:** Three targeted edits — `protocol.InstanceFinished` struct, `dispatcher.runInstance()` terminal event block, and doc updates — with a new test covering both behaviors.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `os/exec` stdlib | Go stdlib | Subprocess exit code extraction via `*exec.ExitError` | Already in use throughout `internal/claude/process.go` |
| `errors` stdlib | Go stdlib | `errors.As()` for type-asserting `exec.ExitError` | Idiomatic Go error unwrapping; `exec.ExitError` may be wrapped by `cmd.Wait()` |

No new dependencies required.

---

## Architecture Patterns

### How `cmd.Wait()` exit codes work in Go

When a subprocess exits with a non-zero code, `cmd.Wait()` returns a `*exec.ExitError`. The exit code is extracted via `.ExitCode()`:

```go
// Source: Go stdlib os/exec documentation
import (
    "errors"
    "os/exec"
)

var exitErr *exec.ExitError
exitCode := 0
if errors.As(err, &exitErr) {
    exitCode = exitErr.ExitCode()
}
```

`errors.As()` is preferred over a direct type assertion `err.(*exec.ExitError)` because `cmd.Wait()` may wrap the error (e.g., when `WaitDelay` fires, the error is a `*exec.ExitError` wrapped inside `*exec.ExitError` or another wrapper). Using `errors.As()` unwraps correctly regardless.

`ExitCode()` returns `-1` if the process was killed by a signal (Unix) or the exit code cannot be determined. This is the correct value to propagate — the server can distinguish signal-killed (`-1`) from clean exit (`0`) from CLI error (non-zero positive).

### Where to extract the exit code

`Process.Stream()` (in `internal/claude/process.go`) already returns `waitErr = p.cmd.Wait()`. This error flows to `runInstance()` in `dispatcher.go` as `streamErr`. The extraction must happen at the call site in `runInstance()` — NOT inside `Process.Stream()` — because `Stream()` treats any non-nil `waitErr` as a stream error already (context-limit detection reuses the same return path).

The existing terminal event block in `runInstance()` (lines 255-270 of `dispatcher.go`) is exactly the right place:

```go
// Current (hardcoded 0):
d.sendEnvelope(protocol.TypeInstanceFinished, protocol.NewMsgID(), protocol.InstanceFinished{
    InstanceID: cmd.InstanceID,
    ExitCode:   0,
})

// After fix (real exit code + session ID):
exitCode := 0
var exitErr *exec.ExitError
if errors.As(streamErr, &exitErr) {
    exitCode = exitErr.ExitCode()
}
d.sendEnvelope(protocol.TypeInstanceFinished, protocol.NewMsgID(), protocol.InstanceFinished{
    InstanceID: cmd.InstanceID,
    ExitCode:   exitCode,
    SessionID:  proc.SessionID(),
})
```

Note: `proc.SessionID()` is called directly here (not `inst.sessionID`) because `proc.SessionID()` is already set from the streaming result events before `Stream()` returns. The `inst.sessionID` update block above (lines 244-252) is redundant but harmless — it was added for the status-request handler. Both values are identical at this point.

### `InstanceFinished` struct change

Add `SessionID` field to `protocol.InstanceFinished`:

```go
// Source: internal/protocol/messages.go
type InstanceFinished struct {
    InstanceID string `json:"instance_id"`
    ExitCode   int    `json:"exit_code"`
    SessionID  string `json:"session_id,omitempty"`
}
```

`omitempty` is correct: when a session was killed before producing a result event, `proc.SessionID()` returns `""` and the field should be absent from JSON (consistent with the `InstanceStarted` and `InstanceSummary` structs which use the same convention).

### Existing `InstanceStarted` already carries SessionID

`InstanceStarted` already has a `SessionID` field populated from `cmd.SessionID` (the resume ID passed in). But that is the *input* session ID (for resume). `InstanceFinished.SessionID` carries the *output* session ID (the one Claude assigned for this run) — allowing the server to store it and pass it as `session_id` in the next `execute` command.

### Documentation update scope

Two files need updating:

**`docs/protocol-spec.md` — Section 3.1.5 `instance_finished`:**
- Add `session_id` to JSON schema example
- Add row to the field table: `session_id | string | string | Claude session ID from this run. Omitted when empty (omitempty). Server should store this for use as session_id in future execute commands.`
- Update exit_code notes: `Real OS exit code from cmd.Wait(). 0 = clean exit, -1 = killed by signal, positive = CLI error.`
- Update Diagram 2 to show `instance_finished {instance_id, exit_code: <real>, session_id: <new>}`

**`docs/server-spec.md` — Section 3 Instance Model:**
- The `session_id` field description currently says "Populated from the `instance_started` event." This must be updated: session ID is now also populated (or superseded) from `instance_finished`. The `instance_finished` value is the authoritative final session ID the server should persist.

### Anti-Patterns to Avoid

- **Direct type assertion `err.(*exec.ExitError)`:** Will panic or miss if the error is wrapped. Use `errors.As()`.
- **Adding `ExitCode` to `InstanceError`:** The success criteria does not call for this, and `InstanceError` already carries a textual description. Keep them separate.
- **Calling `proc.SessionID()` before `Stream()` returns:** `sessionID` is populated during streaming; it is only valid after `Stream()` returns. The current code already does this correctly (call is on line 245, after `streamErr` is assigned on line 231).
- **Using `inst.sessionID` instead of `proc.SessionID()` for the finished event:** Both are equivalent at this point, but `proc.SessionID()` is the authoritative source. Use it directly to avoid confusion.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Exit code extraction | Custom exit code parsing from stderr | `errors.As(err, &exec.ExitError)` then `.ExitCode()` | stdlib handles signal-killed (-1), normal exit, and wrapped errors correctly |

---

## Common Pitfalls

### Pitfall 1: ExitCode not extracted when `streamErr` is nil

**What goes wrong:** When a process exits 0, `cmd.Wait()` returns `nil`. `proc.Stream()` returns `nil`. The `exitCode` extraction via `errors.As(nil, ...)` returns `false`. `exitCode` stays `0`. This is correct behavior — no special handling needed.

**Why it happens:** Developers worry about the nil case. It is handled automatically: `errors.As(nil, ...)` is a no-op and `exitCode` defaults to `0`.

### Pitfall 2: Thinking `streamErr != nil` means `InstanceError` path

**What goes wrong:** The existing terminal event block only emits `InstanceFinished` when `streamErr == nil || ctx.Err() != nil`. When context is cancelled (kill), `streamErr` is non-nil BUT `ctx.Err()` is non-nil, so we still take the `InstanceFinished` path. This is intentional.

**How to handle:** The exit code extraction must happen in both the `InstanceFinished` path. When context is cancelled (SIGTERM/kill), `cmd.Wait()` typically returns an `*exec.ExitError` with `ExitCode() == -1` on Unix. Propagate this honestly.

**Correct structure:**
```go
inst.done.Do(func() {
    exitCode := 0
    var exitErr *exec.ExitError
    if errors.As(streamErr, &exitErr) {
        exitCode = exitErr.ExitCode()
    }

    if streamErr != nil && ctx.Err() == nil {
        // Non-context stream error → InstanceError
        d.sendEnvelope(protocol.TypeInstanceError, ...)
    } else {
        // Clean exit or context-cancelled → InstanceFinished
        d.sendEnvelope(protocol.TypeInstanceFinished, protocol.NewMsgID(), protocol.InstanceFinished{
            InstanceID: cmd.InstanceID,
            ExitCode:   exitCode,
            SessionID:  proc.SessionID(),
        })
    }
})
```

### Pitfall 3: `proc` is nil when `NewProcess` fails

**What goes wrong:** If `claude.NewProcess()` fails, `proc` is nil. The code returns early via `inst.done.Do(InstanceError)` before reaching the streaming block. `proc.SessionID()` is never called in that path. No change needed — the early return is already in place.

### Pitfall 4: Existing test `TestLifecycleEvents` may need updating

**What goes wrong:** `TestLifecycleEvents` decodes `InstanceFinished` and checks `InstanceID` but does not check `ExitCode` or `SessionID`. Adding new fields is additive and does not break existing decode. However, a new test asserting the real exit code and session ID population should be added.

---

## Code Examples

### Extracting exit code from `Stream()` return value

```go
// Source: Go stdlib os/exec, errors packages
import (
    "errors"
    "os/exec"
)

streamErr := proc.Stream(ctx, cb)

exitCode := 0
var exitErr *exec.ExitError
if errors.As(streamErr, &exitErr) {
    exitCode = exitErr.ExitCode()
}
// exitCode is now: 0 (clean), -1 (signal-killed), or positive integer (CLI error)
```

### Updated `InstanceFinished` emission

```go
// In dispatcher.go runInstance(), inside inst.done.Do():
exitCode := 0
var exitErr *exec.ExitError
if errors.As(streamErr, &exitErr) {
    exitCode = exitErr.ExitCode()
}

if streamErr != nil && ctx.Err() == nil {
    instLog.Error().Err(streamErr).Msg("instance stream error")
    d.sendEnvelope(protocol.TypeInstanceError, protocol.NewMsgID(), protocol.InstanceError{
        InstanceID: cmd.InstanceID,
        Error:      streamErr.Error(),
    })
} else {
    instLog.Info().Int("exit_code", exitCode).Msg("instance finished")
    d.sendEnvelope(protocol.TypeInstanceFinished, protocol.NewMsgID(), protocol.InstanceFinished{
        InstanceID: cmd.InstanceID,
        ExitCode:   exitCode,
        SessionID:  proc.SessionID(),
    })
}
```

### Mock Claude script that emits a result event with session_id (for test)

The existing `ndjsonLine()` helper in `dispatcher_test.go` already emits a result event with `session_id`. It can be used directly to test that `InstanceFinished.SessionID` is populated from the captured session ID.

### Test pattern for exit code assertion

```go
// Pattern mirrors existing TestLifecycleEvents in dispatcher_test.go
var fin protocol.InstanceFinished
if err := finEnv.Decode(&fin); err != nil {
    t.Fatalf("decode: %v", err)
}
if fin.ExitCode != 0 {
    t.Errorf("ExitCode = %d, want 0", fin.ExitCode)
}
if fin.SessionID != "sess-lc" {
    t.Errorf("SessionID = %q, want %q", fin.SessionID, "sess-lc")
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Hardcoded `ExitCode: 0` | Real exit code from `exec.ExitError.ExitCode()` | Phase 16 | Server can now distinguish clean exit from signal-killed from CLI error |
| No `SessionID` on `InstanceFinished` | `SessionID` populated from `proc.SessionID()` | Phase 16 | Server learns the session ID from a fresh run; can pass it as `--resume` in next execute |

---

## Open Questions

1. **Should `InstanceError` also carry `SessionID`?**
   - What we know: Success criteria says only `InstanceFinished` needs it. An errored instance may have streamed partial output and captured a session ID before erroring.
   - What's unclear: Whether the server would ever want to resume from a partial/errored session.
   - Recommendation: Out of scope for Phase 16. FUTURE-01 defers token/usage forwarding to the same future milestone where this could also be added.

2. **Log field for exit code in the `instance finished` log line?**
   - What we know: The existing log line is `instLog.Info().Msg("instance finished")`.
   - Recommendation: Add `.Int("exit_code", exitCode)` to the log event for observability. Low-risk additive change.

---

## Environment Availability

Step 2.6: SKIPPED (no external dependencies — pure Go stdlib code changes, no CLI tools or services required beyond existing `go test`).

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + goleak |
| Config file | none (standard `go test`) |
| Quick run command | `go test ./internal/dispatch/... ./internal/protocol/...` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| INST-04 | `InstanceFinished.ExitCode` reflects real process exit code (0 for clean exit) | unit | `go test ./internal/dispatch/... -run TestLifecycleEvents` | Existing test covers it; may need assertion update |
| INST-04 | `InstanceFinished.ExitCode` is `-1` when process is killed by signal | unit | `go test ./internal/dispatch/... -run TestInstanceFinishedRealExitCode` | Wave 0 gap |
| INST-07 | `InstanceFinished.SessionID` populated from `proc.SessionID()` after fresh run | unit | `go test ./internal/dispatch/... -run TestInstanceFinishedSessionID` | Wave 0 gap |
| INST-04/07 | `protocol.InstanceFinished` marshals/unmarshals `session_id` correctly | unit | `go test ./internal/protocol/... -run TestInstanceFinishedJSON` | Wave 0 gap |

### Sampling Rate
- **Per task commit:** `go test ./internal/dispatch/... ./internal/protocol/...`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work` (excluding pre-existing `TestValidatePathWindowsTraversal` failure in `internal/security`)

### Wave 0 Gaps
- [ ] `internal/dispatch/dispatcher_test.go` — add `TestInstanceFinishedRealExitCode` (covers INST-04 non-zero exit code path)
- [ ] `internal/dispatch/dispatcher_test.go` — add `TestInstanceFinishedSessionID` (covers INST-07: session ID from fresh run propagated to InstanceFinished)
- [ ] `internal/protocol/messages_test.go` — add `TestInstanceFinishedJSON` (verify new `session_id` field marshals/unmarshals correctly, omitempty behavior)

---

## Project Constraints (from CLAUDE.md)

| Directive | Applies To Phase 16? |
|-----------|---------------------|
| Keep files under 500 lines | Yes — both files being edited are well under 500 lines |
| Use typed interfaces for all public APIs | Yes — `InstanceFinished` is a public struct; `SessionID` field follows existing pattern |
| ALWAYS run tests after making code changes | Yes — `go test ./...` after each edit |
| ALWAYS verify build succeeds before committing | Yes — `go build ./...` check |
| NEVER hardcode API keys/secrets | Not applicable |
| Use event sourcing for state changes | Already satisfied by the existing protocol event design |
| Prefer TDD London School (mock-first) for new code | Yes — new tests use the existing `mockConn` + `createMockClaude` pattern |

**Build command:** `go build ./...` (project uses Go, not `bun`)
**Test command:** `go test ./...`

Note: The CLAUDE.md in this repo references `bun run start` / `bun run typecheck` from the old TypeScript bot, which has been removed (Phase 12). The Go project does not use Bun. The effective build/test commands are the Go toolchain commands listed above.

---

## Sources

### Primary (HIGH confidence)
- Direct code inspection of `internal/claude/process.go` — confirmed `Process.SessionID()` returns session from result events; confirmed `Stream()` returns raw `cmd.Wait()` error
- Direct code inspection of `internal/dispatch/dispatcher.go` — confirmed `ExitCode: 0` hardcoding at line 266; confirmed `proc.SessionID()` captured into `inst.sessionID` at lines 244-252 but NOT propagated to `InstanceFinished`
- Direct code inspection of `internal/protocol/messages.go` — confirmed `InstanceFinished` struct lacks `SessionID` field
- Go stdlib `os/exec` documentation — `ExitCode()` returns -1 for signal-killed, 0 for clean, positive for explicit exit code; `errors.As()` for unwrapping

### Secondary (MEDIUM confidence)
- `docs/protocol-spec.md` and `docs/server-spec.md` — confirmed which sections reference `instance_finished` and `session_id`; confirmed scope of doc updates needed

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — pure stdlib, no new dependencies
- Architecture: HIGH — changes are pinpoint and fully visible in source
- Pitfalls: HIGH — derived from reading the actual code paths
- Doc update scope: HIGH — confirmed by reading both spec files

**Research date:** 2026-03-25
**Valid until:** Stable (no external dependencies; valid until the codebase is refactored)
