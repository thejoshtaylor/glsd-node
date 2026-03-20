---
phase: 01-core-bot-infrastructure
plan: 09
subsystem: bot-layer
tags: [handlers, wiring, gotgbot, callback, commands]
dependency_graph:
  requires: [01-07]
  provides: [command-wiring, callback-routing]
  affects: [01-VERIFICATION]
tech_stack:
  added: []
  patterns: [handler-delegation, callbackquery-filter]
key_files:
  created: []
  modified:
    - internal/bot/handlers.go
    - go.mod
decisions:
  - "Delegated all five command handlers to real implementations in bothandlers package"
  - "Registered callbackquery.All filter for callback query routing"
  - "go mod tidy automatically promoted gotgbot/v2 from indirect to direct once callbackquery import was added"
metrics:
  duration: 8min
  completed: "2026-03-19"
  tasks_completed: 2
  files_modified: 2
---

# Phase 01 Plan 09: Gap Closure — Wire Command Handler Stubs Summary

**One-liner:** Replaced five stub command handlers in bot layer with one-line delegations to real implementations; registered callback query routing; promoted gotgbot/v2 to direct dependency.

## What Was Built

Five command handler stubs in `internal/bot/handlers.go` that replied "not yet implemented" were replaced with one-line delegations to the real handler implementations built in Plan 07 (`internal/handlers/command.go` and `internal/handlers/callback.go`). A new `handleCallback` method was added and registered on the gotgbot dispatcher via `handlers.NewCallback(callbackquery.All, b.handleCallback)`. Running `go mod tidy` after adding the `callbackquery` import automatically promoted `gotgbot/v2` from the indirect to the direct require block in `go.mod`.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Wire command stubs to real handlers and register callback query handler | afe781e | internal/bot/handlers.go |
| 2 | Fix gotgbot/v2 direct dependency marker in go.mod | c454fda | go.mod |

## Verification Results

All 6 plan verification checks passed:
1. `go build ./...` exits 0
2. `go test ./... -count=1` — all 8 packages pass
3. No "not yet implemented" text in handlers.go
4. `bothandlers.HandleStart` wired in handlers.go
5. `handlers.NewCallback(callbackquery.All, ...)` registered in dispatcher
6. `gotgbot/v2` has no `// indirect` suffix in go.mod

## Deviations from Plan

None — plan executed exactly as written. `go mod tidy` handled the go.mod fix automatically after Task 1 added the `callbackquery` import.

## Self-Check: PASSED

Files exist:
- internal/bot/handlers.go: FOUND
- go.mod: FOUND

Commits exist:
- afe781e: FOUND
- c454fda: FOUND
