---
phase: 05-fix-session-metrics-and-gsd-persistence
plan: "02"
subsystem: handlers
tags: [persistence, gsd, callback, session]
dependency_graph:
  requires: []
  provides: [GSD-callback-persistence]
  affects: [internal/handlers/callback.go, internal/handlers/command.go, internal/bot/handlers.go]
tech_stack:
  added: []
  patterns: [OnQueryComplete closure, persist param threading]
key_files:
  created: []
  modified:
    - internal/handlers/callback.go
    - internal/handlers/command.go
    - internal/bot/handlers.go
    - internal/handlers/callback_test.go
decisions:
  - HandleGsd in command.go also needed persist param threaded — auto-fixed as Rule 3 blocking issue (same pattern as Phase 04)
  - bot/handlers.go handleGsd wrapper updated to pass b.persist — consistent with all other handler wrappers
  - pre-existing unstaged process_test.go failures (LastUsage/LastContextPercent) are out-of-scope Phase 05 WIP — deferred
metrics:
  duration: 12min
  completed_date: "2026-03-20"
  tasks_completed: 2
  files_modified: 4
---

# Phase 05 Plan 02: GSD Callback Persistence Summary

Wire PersistenceManager into the GSD callback path so keyboard-triggered sessions are saved for /resume.

## What Was Built

`enqueueGsdCommand` now accepts `persist *session.PersistenceManager` and sets `OnQueryComplete` on `WorkerConfig`, matching the pattern in `text.go`. When a GSD-triggered worker completes a query it calls `persist.Save(SavedSession{...})` with the command text as the title (truncated to 50 chars), the project `WorkingDir`, and the `ChannelID`.

All intermediate functions — `handleCallbackGsd`, `handleCallbackGsdPhase`, `handleCallbackAskUser` — were updated to thread `persist` through to `enqueueGsdCommand`. All 6 call sites of `enqueueGsdCommand` and all 3 intermediate call sites in `HandleCallback` were updated.

`HandleGsd` in `command.go` (which also calls `enqueueGsdCommand` for direct `/gsd:...` commands) was also updated to accept and thread `persist`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] HandleGsd in command.go required persist param**
- **Found during:** Task 1 build verification (`go build ./...`)
- **Issue:** `HandleGsd` calls `enqueueGsdCommand` but did not have `persist` parameter; adding persist to `enqueueGsdCommand` broke compilation
- **Fix:** Added `persist *session.PersistenceManager` to `HandleGsd` signature and updated `bot/handlers.go` wrapper to pass `b.persist`
- **Files modified:** `internal/handlers/command.go`, `internal/bot/handlers.go`
- **Commit:** a61d739 (included in Task 1 commit)

### Out-of-scope Discoveries

- `internal/claude/process_test.go` has pre-existing unstaged test failures (`p.LastUsage`, `p.LastContextPercent` undefined) from Phase 05 work-in-progress on session metrics. These are NOT caused by this plan. Deferred to Phase 05 Plan 01 or the relevant metrics plan.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Thread persist param and add OnQueryComplete to enqueueGsdCommand | a61d739 |
| 2 | Add TestGsdOnQueryCompleteSavesSession and title truncation test | 41ac651 |

## Verification

- `go build ./...` exits 0
- `go test ./internal/session/... ./internal/handlers/...` passes
- `TestGsdOnQueryCompleteSavesSession` passes
- `TestGsdOnQueryCompleteTitleTruncation` passes
- All existing callback tests pass (no regressions)

## Self-Check

Files created/modified:
- internal/handlers/callback.go — MODIFIED (persist param, OnQueryComplete, imports)
- internal/handlers/command.go — MODIFIED (persist param on HandleGsd)
- internal/bot/handlers.go — MODIFIED (pass b.persist to HandleGsd)
- internal/handlers/callback_test.go — MODIFIED (added 2 tests)
