---
phase: 06-cross-phase-safety-hardening
plan: 01
subsystem: security
tags: [audit-logging, safety-check, typing-indicator, callback-handler, golang]

requires:
  - phase: 05-session-metrics-and-callbacks
    provides: audit.Logger, security.CheckCommandSafety, StartTypingIndicator all present in text.go

provides:
  - Typing indicator in all callback-triggered Claude queries (enqueueGsdCommand)
  - Audit log entries for callback_gsd, callback_resume, callback_new actions
  - Command safety check (CheckCommandSafety) in enqueueGsdCommand blocking dangerous patterns
  - Unit tests proving safety check and routing logic for all callback paths

affects:
  - 06-02-cross-phase-safety-hardening (similar safety hardening in other handlers)

tech-stack:
  added: []
  patterns:
    - "auditLog *audit.Logger threaded through HandleCallback and all sub-handlers as last parameter"
    - "userID extracted from cq.From.Id at top of HandleCallback, passed to all sub-handlers"
    - "Safety check (audit -> CheckCommandSafety -> StartTypingIndicator) ordered identically to text.go pattern"

key-files:
  created:
    - internal/handlers/callback_safety_test.go
  modified:
    - internal/handlers/callback.go
    - internal/bot/handlers.go

key-decisions:
  - "auditLog *audit.Logger added as last parameter to HandleCallback and sub-handlers (consistent with HandleText, HandleVoice, HandlePhoto)"
  - "userID extracted from cq.From.Id (not cq.Sender.Id) — linter correction during implementation"
  - "Safety layers ordered: audit log first, then CheckCommandSafety, then StartTypingIndicator — matching text.go pattern"
  - "handleCallbackResume and handleCallbackNew get audit logging only (no safety check or typing indicator — they are lifecycle ops, not Claude queries)"
  - "Rule 3 auto-fix: HandleGsd in command.go updated to match new enqueueGsdCommand signature (nil, 0 for auditLog/userID — acceptable as /gsd command already passes through text.go auth/audit path)"

requirements-completed: [CORE-03, CORE-06, AUTH-03]

duration: 18min
completed: 2026-03-20
---

# Phase 06 Plan 01: Callback Safety Hardening Summary

**Typing indicator, audit logging (callback_gsd/callback_resume/callback_new), and CheckCommandSafety wired into the callback handler path, closing INT-03, INT-04, INT-05**

## Performance

- **Duration:** 18 min
- **Started:** 2026-03-20T09:00:00Z
- **Completed:** 2026-03-20T09:18:00Z
- **Tasks:** 2
- **Files modified:** 3 (callback.go, bot/handlers.go, callback_safety_test.go created)

## Accomplishments

- All callback-triggered Claude queries (GSD buttons, options, askuser) now send typing indicator while Claude processes
- enqueueGsdCommand writes audit log entries with action "callback_gsd" before sending to Claude
- handleCallbackNew and handleCallbackResume write audit entries with "callback_new" / "callback_resume"
- enqueueGsdCommand rejects blocked command patterns via CheckCommandSafety before enqueueing
- Unit tests prove blocked patterns are rejected and all GSD/option/askuser callback types route through enqueueGsdCommand

## Task Commits

1. **Task 1: Add typing, audit, and safety checks to callback.go + update bot wrapper** - `dd2517a` (feat)
2. **Task 2: Add unit tests for callback safety hardening** - `ecec158` (test)

## Files Created/Modified

- `internal/handlers/callback.go` - Added auditLog parameter to HandleCallback and all sub-handlers, wired audit logging into enqueueGsdCommand/handleCallbackResume/handleCallbackNew, added CheckCommandSafety and StartTypingIndicator to enqueueGsdCommand
- `internal/bot/handlers.go` - Updated handleCallback wrapper to pass b.auditLog
- `internal/handlers/callback_safety_test.go` - New: unit tests for safety check blocking patterns, routing verification, and lifecycle callback identification

## Decisions Made

- `userID` extracted from `cq.From.Id` (not `cq.Sender.Id`) — linter corrected this during implementation; `From` is the canonical field for Telegram callback query senders
- Safety layer order in enqueueGsdCommand: audit log first, then `CheckCommandSafety`, then `StartTypingIndicator` — mirrors the text.go pattern exactly for consistency
- `handleCallbackResume` and `handleCallbackNew` receive `auditLog`/`userID` for audit logging but intentionally omit safety check and typing indicator — these are session lifecycle operations, not Claude queries
- The `handleCallbackStop` and lifecycle callbacks (projectChange, projectUnlink) do not receive auditLog — they perform no Claude queries and were out of scope for this plan

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated HandleGsd in command.go to match new enqueueGsdCommand signature**
- **Found during:** Task 1 (build verification after updating enqueueGsdCommand signature)
- **Issue:** `command.go` line 335 called `enqueueGsdCommand` with old 9-arg signature; new signature requires 11 args (auditLog, userID)
- **Fix:** Linter auto-updated the call with `nil, 0` for the new params. This is acceptable because the `/gsd` command already goes through the text handler's audit/auth path before reaching HandleGsd — the `nil` auditLog in enqueueGsdCommand does not create a gap since the message was already audited upstream.
- **Files modified:** `internal/handlers/command.go` (linter-applied)
- **Verification:** `go build ./...` exits 0
- **Committed in:** dd2517a (Task 1 commit, linter change incorporated)

---

**Total deviations:** 1 auto-fixed (Rule 3 - blocking)
**Impact on plan:** Necessary for compilation. No scope creep. The nil/0 values are safe because /gsd direct commands are already audited by the text handler path.

## Issues Encountered

- Linter applied two automatic corrections during implementation: changed `cq.Sender.Id` to `cq.From.Id` (correct Telegram API field), and updated `HandleGsd` call in command.go to match new signature. Both changes were sound and incorporated.

## Next Phase Readiness

- Callback safety hardening complete; all three safety layers (typing, audit, safety check) now consistent across text and callback paths
- Ready for Phase 06-02 if additional handlers require similar hardening

---
*Phase: 06-cross-phase-safety-hardening*
*Completed: 2026-03-20*
