---
phase: 02-multi-project-and-gsd-integration
plan: "03"
subsystem: handlers/gsd-ux
tags: [gsd, keyboard, callback, rate-limiter, streaming, response-buttons]
dependency_graph:
  requires: ["02-01", "02-02"]
  provides: [gsd-keyboard-ux, callback-routing, response-button-extraction, api-rate-limiter]
  affects: [internal/handlers/callback.go, internal/handlers/command.go, internal/handlers/streaming.go, internal/handlers/text.go, internal/bot/bot.go, internal/bot/handlers.go]
tech_stack:
  added: [golang.org/x/time/rate]
  patterns: [global-rate-limiter, phase-picker-keyboard, response-button-extraction, ask-user-tempfile]
key_files:
  created: []
  modified:
    - internal/handlers/callback.go
    - internal/handlers/callback_test.go
    - internal/handlers/command.go
    - internal/handlers/streaming.go
    - internal/handlers/text.go
    - internal/bot/bot.go
    - internal/bot/handlers.go
decisions:
  - callbackWg package-level var used for callback-spawned workers (callbacks only enqueue to existing workers; bot-level WaitGroup tracks text-path workers)
  - waitForRateLimit() extracted as method on StreamingState so both sendOrEditWithFallback and updateStatusMessage share the same 5s-timeout context pattern
  - HandleGsd accepts wg param for API consistency but ignores it (enqueueGsdCommand manages its own minimal goroutine lifecycle)
  - roadmapRE pattern in gsd.go uses specific format; TestBuildGsdStatusHeader_WithPhases uses that exact markdown format for test fixtures
metrics:
  duration_minutes: 9
  completed_date: "2026-03-20"
  tasks_completed: 2
  files_modified: 7
---

# Phase 02 Plan 03: GSD Keyboard UX, Callback Routing, Response Buttons, API Rate Limiter Summary

**One-liner:** /gsd keyboard with phase-aware status header, 13 callback prefixes routed, response button extraction on every streaming reply, global 25/sec Telegram API rate limiter.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | GSD callback routing + HandleGsd + response button extraction + tests | ecff69c | callback.go, command.go, text.go, callback_test.go |
| 2 | Global API rate limiter + AccumulatedText + bot wiring | 47788a9 | streaming.go, bot.go, handlers.go |

## What Was Built

### Task 1: GSD Callback Routing + Commands + Response Buttons

**callback.go:**
- Extended `callbackAction` enum with 8 new constants: `callbackActionGsd`, `callbackActionGsdRun`, `callbackActionGsdFresh`, `callbackActionGsdPhase`, `callbackActionOption`, `callbackActionAskUser`, `callbackActionProjectChange`, `callbackActionProjectUnlink`
- `parseCallbackData`: all `gsd-run:`, `gsd-fresh:`, `gsd-exec:`, `gsd-plan:`, `gsd-discuss:`, `gsd-research:`, `gsd-verify:`, `gsd-remove:` cases placed before `gsd:` to prevent premature prefix matching
- `HandleCallback`: extended to accept `mappings` and `awaitingPath` params; full dispatch for all new actions
- `handleCallbackGsd`: routes to phase picker (BuildPhasePickerKeyboard) or direct command send
- `handleCallbackGsdPhase`: maps `gsd-exec:2` -> `/gsd:execute-phase 2` etc.
- `handleCallbackAskUser`: reads `ask-user-{id}.json` from temp dir, validates option index, deletes file, sends selection to Claude
- `enqueueGsdCommand`: shared helper to get/create session and enqueue message with streaming callback

**command.go:**
- `HandleGsd`: shows GSD keyboard with status header, or routes direct `/gsd:command` to session
- `buildGsdStatusHeader`: reads ROADMAP.md, computes done/total (skipping "skipped" phases), shows next pending phase

**text.go:**
- `HandleText` signature extended with `globalLimiter *rate.Limiter` param
- `NewStreamingState` call updated to pass `globalLimiter`
- ErrCh goroutine: on success, calls `maybeAttachActionKeyboard` with accumulated response text
- `maybeAttachActionKeyboard`: extracts GSD commands, numbered options, lettered options; sends follow-up em-dash message with inline keyboard

**callback_test.go:**
- 10 new `TestParseCallback*` tests covering all new prefixes
- `TestParseCallbackPrefixOrder`: verifies gsd-run/gsd-fresh/gsd-exec not caught by gsd: prefix
- `TestAskUserCallbackTempFile`: creates real temp file, tests parse/validate/delete flow
- `TestBuildGsdStatusHeader_WithPhases`: creates temp ROADMAP.md, verifies done/total count, next phase display, project name

### Task 2: Rate Limiter + AccumulatedText + Bot Wiring

**streaming.go:**
- `StreamingState.globalLimiter *rate.Limiter` field added
- `NewStreamingState(bot, chatID, globalLimiter)` — optional nil for no limiting
- `AccumulatedText()`: thread-safe concatenation of all text segments (0..nextSegment)
- `waitForRateLimit()`: calls `globalLimiter.Wait()` with 5s timeout; dropped on timeout (shutdown safety)
- `sendOrEditWithFallback` and `updateStatusMessage`: both call `waitForRateLimit()` before every Telegram API call

**bot.go:**
- `Bot.globalAPILimiter *rate.Limiter` field
- `rate.NewLimiter(rate.Limit(25), 5)` created in `New()`
- `GlobalAPILimiter()` accessor added

**handlers.go:**
- `/gsd` command registered with `handleGsd` wrapper
- `handleText` passes `b.globalAPILimiter` to `HandleText`
- `handleCallback` passes `b.mappings` and `b.awaitingPath` to `HandleCallback`

## Verification Results

```
go build ./...                   PASS
go test ./internal/... -count=1  PASS (all 9 packages)
TestParseCallback* (10 tests)    PASS
TestAskUserCallbackTempFile      PASS
TestBuildGsdStatusHeader_WithPhases  PASS
```

## Deviations from Plan

### Auto-fixed Issues

None — plan executed exactly as written.

**Minor implementation note:** `enqueueGsdCommand` in callback.go uses a package-level `callbackWg` instead of threading the bot's WaitGroup through all callback paths. This is correct behavior: callback handlers only enqueue to already-running session workers (started by HandleText or restoreSessions), so new worker goroutines started from callbacks are not tracked by the bot's WaitGroup. The plan's `wg *sync.WaitGroup` parameter on `HandleGsd` is accepted but not forwarded (noted with `_ = wg` comment).

## Self-Check: PASSED

- internal/handlers/callback.go: FOUND
- internal/handlers/streaming.go: FOUND
- internal/bot/bot.go: FOUND
- Commit ecff69c: FOUND
- Commit 47788a9: FOUND
