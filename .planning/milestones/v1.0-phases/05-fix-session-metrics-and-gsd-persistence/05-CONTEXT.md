# Phase 5: Fix Session Metrics and GSD Persistence - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Capture token usage and context percentage from Claude result events into session fields so `/status` displays real data, and wire `OnQueryComplete` into the GSD callback path so keyboard-triggered sessions persist for `/resume`.

</domain>

<decisions>
## Implementation Decisions

### Token/context capture timing
- Capture after `Stream()` returns, not during streaming — single point of capture at the same location where `sessionID` is already read (session.go ~line 351)
- Add `LastUsage() *UsageData` and `LastContextPercent() *int` accessor methods to the `Process` struct — symmetric with existing `SessionID()` pattern
- In `processMessage()`, read `proc.LastUsage()` and `proc.LastContextPercent()` after Stream() completes, write to `s.lastUsage` and `s.contextPercent` inside the existing mutex block
- On successful queries only — failed queries (errors, context limit) leave previous usage data intact rather than writing partial/misleading numbers

### /status display for fresh sessions
- Omit the token usage and context percentage sections entirely when `LastUsage()` returns nil (fresh session, no queries run yet)
- Once a query completes, always show token section — matches existing nil-check pattern in `buildStatusText`

### GSD persistence wiring
- Pass `*PersistenceManager` to `enqueueGsdCommand` and set `OnQueryComplete` on `WorkerConfig` — same pattern as text.go, voice.go, photo.go, document.go
- Thread `persist` param through the full callback chain: `HandleCallback` → `handleCallbackGsd/Resume/New/etc` → `enqueueGsdCommand` — matches how `wg` and `globalLimiter` were threaded in Phase 4
- Not a problem if worker was already started by text handler — `OnQueryComplete` is set at worker start time on `WorkerConfig`, persists for the worker's lifetime
- SavedSession uses same fields as text handler: channelID, workingDir, sessionID, timestamp, GSD command text as message preview — no special GSD flag

### Claude's Discretion
- Exact implementation of `Process.LastUsage()` / `Process.LastContextPercent()` storage (field vs computed)
- Whether to store the last result event on Process or extract usage/context separately during Stream()
- Test structure for the new capture logic

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Session metrics (token capture)
- `internal/claude/events.go` — `ClaudeEvent.ContextPercent()`, `UsageData`, `ModelUsage` types and computation logic
- `internal/claude/events_test.go` — Existing tests for ContextPercent calculation
- `internal/session/session.go` — `Session.lastUsage`, `Session.contextPercent` fields (exist but never populated), `processMessage()` lines 347-384 (capture gap)
- `internal/handlers/command.go` — `buildStatusText()` lines 182-196 (reads LastUsage/ContextPercent for /status display)
- `internal/handlers/command_test.go` — `TestBuildStatusTextContextPercent` (existing test for display)

### GSD persistence (OnQueryComplete wiring)
- `internal/handlers/callback.go` — `enqueueGsdCommand()` lines 373-425 (missing OnQueryComplete), `HandleCallback()` signature
- `internal/handlers/text.go` — `HandleText()` lines 155-165 (reference pattern for OnQueryComplete wiring)
- `internal/session/persist.go` — `PersistenceManager.Save()` (persistence API)
- `internal/bot/handlers.go` — Bot handler wrappers (where persist param gets threaded)

### Project requirements
- `.planning/REQUIREMENTS.md` — SESS-06, SESS-07, PERS-01
- `.planning/ROADMAP.md` — Phase 5 success criteria (3 items)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `Session.lastUsage` / `Session.contextPercent` fields: Already declared with mutex protection and accessor methods (`LastUsage()`, `ContextPercent()`) — just need to be written to
- `ClaudeEvent.ContextPercent()`: Tested computation method on events — Process needs to capture the last result event's output
- `PersistenceManager.Save()`: Fully working persistence with atomic JSON writes and per-project trimming
- `buildStatusText()`: Already reads `sess.LastUsage()` and `sess.ContextPercent()` — will display data once fields are populated

### Established Patterns
- `proc.SessionID()`: Accessor on Process struct read after `Stream()` — new `LastUsage()`/`LastContextPercent()` follow identical pattern
- `OnQueryComplete` closure in text.go: `func(sessionID string) { persist.Save(...) }` — copy this pattern for GSD
- Phase 4 signature threading: `wg *sync.WaitGroup` and `globalLimiter *rate.Limiter` were added to HandleCallback → enqueueGsdCommand chain — `persist *PersistenceManager` follows the same approach

### Integration Points
- `processMessage()` success branch (session.go ~line 369-378): Empty block where `s.lastUsage` and `s.contextPercent` should be set from Process accessors
- `enqueueGsdCommand()` WorkerConfig creation (callback.go ~line 391-395): Missing `OnQueryComplete` field
- `HandleCallback` signature (callback.go ~line 106): Needs `persist *PersistenceManager` param
- Bot handler wrapper for callback (bot/handlers.go): Needs to pass `b.persist` through

</code_context>

<specifics>
## Specific Ideas

No specific requirements — the gaps are precisely identified code locations with well-established patterns to follow.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 05-fix-session-metrics-and-gsd-persistence*
*Context gathered: 2026-03-20*
