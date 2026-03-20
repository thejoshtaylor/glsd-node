---
phase: 04-callback-handler-integration-fixes
plan: 01
subsystem: handlers/callback
tags: [bug-fix, concurrency, rate-limiting, graceful-shutdown, project-mapping]
dependency_graph:
  requires: []
  provides: [FINDING-01-closed, FINDING-02-closed, FINDING-03-closed]
  affects: [internal/handlers/callback.go, internal/handlers/command.go, internal/bot/handlers.go]
tech_stack:
  added: []
  patterns: [wg-injection, mapping-resolution, rate-limiter-threading]
key_files:
  created:
    - internal/handlers/callback_integration_test.go
  modified:
    - internal/handlers/callback.go
    - internal/handlers/command.go
    - internal/bot/handlers.go
decisions:
  - "HandleGsd in command.go also needed globalLimiter added (it called enqueueGsdCommand) — auto-fixed as Rule 3 blocking issue"
  - "callbackWg package-level var deleted; bot WaitGroup injected via HandleCallback signature"
  - "cfg.WorkingDir retained as fallback in handleCallbackResume/New (matching command.go pattern)"
metrics:
  duration: 15min
  completed_date: "2026-03-19"
  tasks_completed: 2
  files_changed: 4
---

# Phase 04 Plan 01: Callback Handler Integration Fixes Summary

**One-liner:** Threaded bot WaitGroup, mapping path resolution, and global API rate limiter through the callback handler chain to close three v1.0 audit findings.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Write regression tests for all three findings | 7cdc8be | internal/handlers/callback_integration_test.go |
| 2 | Apply all three FINDING fixes to callback.go and bot/handlers.go | ec6caeb | internal/handlers/callback.go, internal/handlers/command.go, internal/bot/handlers.go |

## What Was Built

### FINDING-01: Callback Workers Tracked by Bot WaitGroup (Graceful Shutdown)

Removed the package-level `var callbackWg sync.WaitGroup` from `callback.go`. Added `wg *sync.WaitGroup` and `globalLimiter *rate.Limiter` to `HandleCallback`'s signature. Threaded `wg` down through all helper functions (`handleCallbackGsd`, `handleCallbackGsdPhase`, `handleCallbackAskUser`, `enqueueGsdCommand`) so that goroutines started by callback-triggered `enqueueGsdCommand` calls now call `wg.Add(1)` and `defer wg.Done()` on the bot's main WaitGroup — ensuring they are drained during graceful shutdown.

### FINDING-02: handleCallbackResume and handleCallbackNew Resolve Mapping Path

Both functions previously called `store.GetOrCreate(chatID, cfg.WorkingDir)`, ignoring the per-channel project mapping. Updated both to add `mappings *project.MappingStore` to their signatures and resolve the working directory via:

```go
workingDir := cfg.WorkingDir
if m, ok := mappings.Get(chatID); ok {
    workingDir = m.Path
}
```

This matches the canonical pattern from `command.go` and ensures sessions are created in the correct per-project directory.

### FINDING-03: enqueueGsdCommand Passes globalLimiter to NewStreamingState

Changed `NewStreamingState(b, chatID, nil)` to `NewStreamingState(b, chatID, globalLimiter)`. The `globalLimiter` flows from `bot/handlers.go` through `HandleCallback` to `enqueueGsdCommand`.

### bot/handlers.go Call Sites

- `handleCallback` now calls `HandleCallback(..., b.WaitGroup(), b.globalAPILimiter)`
- `handleGsd` now calls `HandleGsd(..., b.WaitGroup(), b.globalAPILimiter)`

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] HandleGsd in command.go also calls enqueueGsdCommand**

- **Found during:** Task 2 (first `go build ./...` attempt)
- **Issue:** `command.go` line 335 called `enqueueGsdCommand(b, chatID, directCmd, store, mappings, cfg)` without `wg` or `globalLimiter`, causing a compile error after the signature change.
- **Fix:** Added `globalLimiter *rate.Limiter` to `HandleGsd` signature, added `golang.org/x/time/rate` import to `command.go`, updated call to `enqueueGsdCommand(..., wg, globalLimiter)`, and updated `bot/handlers.go` `handleGsd` call site to pass `b.globalAPILimiter`.
- **Files modified:** `internal/handlers/command.go`, `internal/bot/handlers.go`
- **Commit:** ec6caeb (included in Task 2 commit)

## Verification Results

```
go build ./...   → exit 0 (no compilation errors)
go test ./...    → all packages pass

callbackWg grep  → no matches (package-level var removed)
NewStreamingState nil grep → no matches (nil replaced with globalLimiter)
cfg.WorkingDir grep → lines 448, 506 only (fallback inside mapping-resolution blocks)
```

## Self-Check: PASSED

- internal/handlers/callback_integration_test.go: FOUND
- internal/handlers/callback.go: FOUND (modified)
- internal/bot/handlers.go: FOUND (modified)
- Commit 7cdc8be: FOUND
- Commit ec6caeb: FOUND
