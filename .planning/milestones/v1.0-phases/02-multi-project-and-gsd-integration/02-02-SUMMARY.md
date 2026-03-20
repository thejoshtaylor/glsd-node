---
phase: 02-multi-project-and-gsd-integration
plan: "02"
subsystem: multi-project-routing
tags: [mapping-store, per-project-sessions, worker-lifecycle, bot-wiring]
dependency-graph:
  requires: ["02-01"]
  provides: ["02-03", "02-04"]
  affects: [internal/handlers/text.go, internal/handlers/command.go, internal/bot/bot.go]
tech-stack:
  added: []
  patterns:
    - AwaitingPathState: in-memory per-channel flag for path-prompt flow
    - workerStarted bool: single-start guard on Session prevents double goroutine launch
    - mapping.Path as WorkingDir: ties session history grouping to project dir
key-files:
  created:
    - internal/handlers/text_test.go
  modified:
    - internal/handlers/text.go
    - internal/handlers/command.go
    - internal/bot/bot.go
    - internal/bot/handlers.go
    - internal/session/session.go
    - internal/config/config.go
    - internal/config/config_test.go
decisions:
  - Use capturedText[:50] for session Title in OnQueryComplete (Session.LastMessageExcerpt does not exist; original approach preserved)
  - workerStarted bool field on Session replaces the StartedAt heuristic (SessionID empty + within 1s) — more reliable, race-condition free
  - handlePathInput stays on retry loop (sends error messages) rather than clearing awaitingPath on invalid input
  - HandleResume filters by mapping.Path only when hasMapped; without mapping it shows all channel sessions (graceful fallback)
metrics:
  duration: 6min
  completed: "2026-03-20T02:49:07Z"
  tasks: 2
  files: 7
---

# Phase 02 Plan 02: Multi-Project Routing and MappingStore Wiring Summary

Per-project message routing in HandleText using MappingStore, with AwaitingPathState for unmapped channels, per-project WorkerConfig, workerStarted single-start guard, /project command, and /resume filtering by project path.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Session.workerStarted field + BuildSafetyPrompt export | 4a3d297 |
| 2 | Multi-project HandleText, /project, /resume filtering, bot wiring, tests | a058d59 |

## What Was Built

### Task 1: Foundation

Added `workerStarted bool` to `Session` struct with `WorkerStarted()` getter and `SetWorkerStarted()` setter (mutex-protected). Exported `buildSafetyPrompt` as `BuildSafetyPrompt` so handler code can build per-project safety prompts without depending on `cfg.SafetyPrompt`.

### Task 2: Full Multi-Project Routing

**text.go:** Complete rewrite of HandleText routing logic:
- `AwaitingPathState` struct tracks which channels are waiting for a path reply
- Mapping check runs before interrupt handling: unmapped channels get path-prompt flow; mapped channels route to project session
- `handlePathInput` validates path under AllowedPaths, checks directory exists, saves mapping, clears awaiting state
- `WorkerConfig` built with `AllowedPaths: []string{mapping.Path}` and `config.BuildSafetyPrompt([]string{mapping.Path})` — fully per-project
- `OnQueryComplete` saves `WorkingDir: capturedMapping.Path` ensuring PersistenceManager's per-WorkingDir trimming applies per project
- Worker started with `!sess.WorkerStarted()` guard + `sess.SetWorkerStarted()` before goroutine launch

**command.go:** Updated all commands for mapping awareness:
- `HandleProject`: shows current mapping with Change/Unlink inline buttons; direct reassignment with `/project /path`
- `HandleResume`: filters `LoadForChannel` results to sessions where `s.WorkingDir == mapping.Path`
- `HandleStart`, `HandleStatus`, `HandleNew`: all accept `*project.MappingStore`, use `mapping.Path` when available

**bot.go:**
- Added `mappings *project.MappingStore` and `awaitingPath *bothandlers.AwaitingPathState` fields to `Bot`
- `New()` creates `MappingStore`, calls `mappings.Load()` at startup
- `restoreSessions()` uses `mappings.Get(channelID)` to build per-project `WorkerConfig`; calls `sess.SetWorkerStarted()` before goroutine

**handlers.go:** Added `/project` command registration; updated all wrappers to pass `b.mappings` and `b.awaitingPath`.

**text_test.go (new):** `TestWorkerConfigPerProject` verifies BuildSafetyPrompt contains project path and produces distinct output per project. `TestHandleTextUnmapped` verifies AwaitingPathState lifecycle (Set → IsAwaiting → Clear, multi-channel independence).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Session.LastMessageExcerpt() does not exist**
- **Found during:** Task 2
- **Issue:** Plan specified `sess.LastMessageExcerpt()` for the OnQueryComplete title, but this method was never added to the Session struct
- **Fix:** Reverted to the original approach: `capturedText[:50]` (same as the pre-plan HandleText)
- **Files modified:** internal/handlers/text.go
- **Commit:** a058d59

**2. [Rule 1 - Bug] config_test.go referenced old unexported buildSafetyPrompt**
- **Found during:** Task 2 verification (go test)
- **Issue:** Existing test called `buildSafetyPrompt(paths)` which became `BuildSafetyPrompt` in Task 1
- **Fix:** Updated `config_test.go` line 176 to call `BuildSafetyPrompt(paths)`
- **Files modified:** internal/config/config_test.go
- **Commit:** a058d59

## Self-Check: PASSED

All files verified present. Both commits (4a3d297, a058d59) verified in git log.
