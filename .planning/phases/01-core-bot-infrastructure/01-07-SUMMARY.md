---
phase: 01-core-bot-infrastructure
plan: 07
subsystem: handlers
tags: [commands, callback, session, telegram, gotgbot]
dependency_graph:
  requires: [01-04, 01-06]
  provides: [HandleStart, HandleNew, HandleStop, HandleStatus, HandleResume, HandleCallback]
  affects: [internal/bot/handlers.go]
tech_stack:
  added: []
  patterns: [pure-function routing for testability, percentage-only context display, inline keyboard resume picker]
key_files:
  created:
    - internal/handlers/command.go
    - internal/handlers/command_test.go
    - internal/handlers/callback.go
    - internal/handlers/callback_test.go
  modified: []
decisions:
  - parseCallbackData extracted as pure function so callback routing is fully testable without gotgbot types
  - buildStatusText extracted as pure function so status format is verifiable in unit tests without Bot dependency
  - handleCallbackResume edits the keyboard message in-place (removes buttons) to confirm restoration
  - Interrupt flag MarkInterrupt not set in /new and action:new — only text handler sets this (for ! prefix)
metrics:
  duration: 7min
  completed_date: "2026-03-20"
  tasks_completed: 2
  files_created: 4
---

# Phase 01 Plan 07: Command Handlers and Callback Summary

**One-liner:** Five bot commands (/start, /new, /stop, /status, /resume) and inline keyboard callback handler with pure-function routing, testable without Telegram API.

## Objective

Implement all Phase 1 command handlers and callback query routing for the gsd-tele-go Telegram bot.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | /start, /new, /stop, /status commands | aa38379 | command.go, command_test.go |
| 2 | /resume, callback handler, behavioral tests | 30b7ca6 | callback.go, callback_test.go (+ command.go fix) |

## What Was Built

### internal/handlers/command.go

- **HandleStart**: replies with welcome text including bot name, project path, current status, and command list
- **HandleNew**: stops any running query, clears session ID (persisted sessions already saved by OnQueryComplete), replies confirmation
- **HandleStop**: checks IsRunning(), calls Stop(), appropriate reply for each case
- **HandleStatus**: calls buildStatusText() with session and workingDir, sends plain text
- **buildStatusText**: pure function producing status dashboard — session ID (first 8 chars), query state with elapsed time, current tool, token usage (only if available), context % (only if available, percentage-only per CONTEXT.md), project path
- **HandleResume**: loads saved sessions via persist.LoadForChannel(), builds InlineKeyboardMarkup with one row per session, button label format: "2006-01-02 15:04 - title" (title capped at ButtonLabelMaxLength=30 chars), callback data "resume:<session_id>"
- **formatSessionLabel**: formats timestamp to readable "YYYY-MM-DD HH:MM" and truncates title

### internal/handlers/callback.go

- **HandleCallback**: answers callback query immediately (removes spinner), extracts chat/message IDs from callback message, routes by parseCallbackData result
- **parseCallbackData**: pure function — takes string, returns (callbackAction, payload), no external dependencies
- **handleCallbackResume**: extracts session ID from payload, calls store.GetOrCreate + sess.SetSessionID, edits keyboard message to confirm
- **handleCallbackStop**: mirrors /stop — checks IsRunning(), calls Stop()
- **handleCallbackNew**: mirrors /new — stops if running, clears session ID
- **action:retry**: placeholder reply "Retry not available yet" (deferred to v2)

## Test Coverage

20 tests across 2 test files, all passing:

- TestBuildStatusTextIdle — no session activity
- TestBuildStatusTextActive — session ID display (first 8 chars + "...")
- TestBuildStatusTextWithTool — tool line suppressed when not running
- TestBuildStatusTextNoTokens — Tokens: line absent when no usage data
- TestBuildStatusTextContextPercent — Context: line absent when nil
- TestBuildStatusTextNilSession — nil session safe
- TestBuildStatusTextSessionIDShortened — long IDs truncated correctly
- TestFormatSessionLabel — timestamp and title formatting
- TestFormatSessionLabelLongTitle — title capped at 30 chars
- TestFormatSessionLabelEmptyTitle — "(no title)" fallback
- TestBuildStatusTextRunningElapsed — QueryStarted() nil for fresh session
- TestBuildStatusTextUsageDataFormatted — token line format "in=X out=Y cache_read=Z cache_create=W"
- TestCallbackRouteResume — "resume:uuid" → callbackActionResume with payload
- TestCallbackRouteActionStop — "action:stop" → callbackActionStop
- TestCallbackRouteActionNew — "action:new" → callbackActionNew
- TestCallbackRouteActionRetry — "action:retry" → callbackActionRetry
- TestCallbackRouteUnknown — unrecognized data → callbackActionUnknown (no panic)
- TestResumeRestoresSessionID — end-to-end: save session, parse callback, apply to store, verify SessionID()
- TestCallbackParseResumePrefixStripped — prefix stripped from payload
- TestCallbackParseAllActions — all action strings validated

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] TestFormatSessionLabel expected wrong string**
- **Found during:** Task 2 full test run
- **Issue:** Test expected "Fix the authentication bug in the" (33 chars) as substring, but ButtonLabelMaxLength=30 truncates to "Fix the authentication bug in " (30 chars)
- **Fix:** Updated test to check for "Fix the authentication bug in" (29 chars) which is always present in the truncated output
- **Files modified:** internal/handlers/command_test.go
- **Commit:** 30b7ca6 (included in Task 2 commit)

## Verification Results

```
go test ./internal/handlers/ -v -count=1 — PASS (20/20 tests)
go build ./internal/handlers/            — OK
go build ./internal/bot/                 — OK
go build ./...                           — OK
```

## Self-Check: PASSED

All created files exist:
- FOUND: internal/handlers/command.go
- FOUND: internal/handlers/command_test.go
- FOUND: internal/handlers/callback.go
- FOUND: internal/handlers/callback_test.go

All commits exist:
- FOUND: aa38379 (Task 1)
- FOUND: 30b7ca6 (Task 2)
