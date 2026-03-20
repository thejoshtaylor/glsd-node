---
phase: 01-core-bot-infrastructure
plan: "06"
subsystem: bot-skeleton
tags: [gotgbot, middleware, streaming, text-handler, graceful-shutdown]
dependency_graph:
  requires: [01-01, 01-02, 01-03, 01-04, 01-05]
  provides: [internal/bot, internal/handlers/streaming, internal/handlers/text]
  affects: [main.go, 01-07, 01-08]
tech_stack:
  added: [gotgbot/v2 (already in go.mod)]
  patterns:
    - Interface-based middleware for testability without live Telegram connection
    - WaitGroup-tracked worker goroutines for graceful shutdown
    - 500ms throttled MarkdownV2 edit-in-place with plain text fallback
    - Typing indicator goroutine with stop channel
key_files:
  created:
    - internal/bot/bot.go
    - internal/bot/middleware.go
    - internal/bot/middleware_test.go
    - internal/bot/handlers.go
    - internal/handlers/streaming.go
    - internal/handlers/text.go
    - data/.gitkeep
  modified:
    - internal/formatting/markdown.go
decisions:
  - "Interface-based AuthChecker/RateLimitChecker in middleware allows behavioral tests without live Telegram connection"
  - "nil-bot guard in middleware reply paths enables unit testing without panics"
  - "Worker goroutine heuristic: start if SessionID empty AND StartedAt within 1 second (new session detection)"
  - "context.Background() for HandleText-spawned workers — bot context threading deferred to Plan 07"
metrics:
  duration_min: 35
  completed_date: "2026-03-19"
  tasks_completed: 3
  files_changed: 8
---

# Phase 01 Plan 06: Bot Skeleton and Streaming Layer Summary

Bot skeleton wired with gotgbot long-polling, auth/rate-limit middleware chain, interface-based testability, 500ms-throttled MarkdownV2 streaming, and WaitGroup-tracked session workers for graceful shutdown.

## What Was Built

### Task 1: Bot Skeleton with Middleware (3ad5031)

`internal/bot/bot.go` — `Bot` struct owns the full lifecycle:
- `New()` creates gotgbot client, session store, persistence manager, rate limiter, audit log
- `Start(ctx)` restores persisted sessions then starts long polling; blocks on ctx.Done()
- `Stop()` cancels workers, stops updater, waits for WaitGroup drain (30s timeout), closes audit log
- `WaitGroup()` exposes `*sync.WaitGroup` for HandleText to register new workers
- `restoreSessions()` loads session history, creates sessions in store, starts worker goroutines

`internal/bot/middleware.go` — Interface-based middleware for clean unit testing:
- `AuthChecker` interface: `IsAuthorized(userID, channelID int64) bool`
- `RateLimitChecker` interface: `Allow(channelID int64) (bool, time.Duration)`
- `authMiddlewareWith()` and `rateLimitMiddlewareWith()` are the testable implementations
- Auth rejection message: "You're not authorized for this channel. Contact the bot admin."
- Rate limit message: "Rate limited. Try again in Xs."
- `IsAuthorized(userID, channelID, ...)` passes channelID per Phase 2 forward-compat requirement

`internal/bot/middleware_test.go` — 5 behavioral tests:
- `TestMiddlewareAuthRejectsUnauthorized` — next handler NOT called for unauthorized user
- `TestMiddlewareAuthAllowsAuthorized` — next handler called for authorized user
- `TestMiddlewareAuthPassesChannelID` — channelID from ctx.EffectiveChat.Id is passed to IsAuthorized
- `TestMiddlewareRateLimitThrottles` — 3rd request rejected when limit is 2
- `TestMiddlewareRateLimitDisabled` — all requests pass when rate limiting is off

`internal/bot/handlers.go` — `registerHandlers()` wires dispatcher groups:
- Group -2: auth middleware (all updates)
- Group -1: rate limit middleware (all updates)
- Group 0: text message handler, command stubs (/start /new /stop /status /resume)

### Task 2: StreamingState and StatusCallback (65a919e)

`internal/handlers/streaming.go`:
- `StreamingState` — throttled edit-in-place with segment tracking, 500ms minimum between edits
- `CreateStatusCallback()` — handles ClaudeEvent types: text accumulation, thinking, tool_use, result
- `editOrSendSegment()` — sends new message or edits existing; MarkdownV2 with StripMarkdown fallback
- `updateStatusMessage()` / `deleteStatusMessage()` — ephemeral tool/thinking status messages
- `StartTypingIndicator()` / `TypingController.Stop()` — typing action goroutine every 4 seconds

### Task 3: Text Message Handler (5c0b94c)

`internal/handlers/text.go`:
- `HandleText()` — routes messages to session queue with full middleware pipeline
- Interrupt prefix `!`: strips, calls MarkInterrupt()+Stop() on running session
- Command safety via `config.BlockedPatterns` (not cfg field — package-level var)
- Worker goroutine started with `wg.Add(1)` / `defer wg.Done()`
- Queue full reply: "Queue full, please wait for the current query to finish."
- Async error handling: context limit → recovery hint; Claude error → truncated stderr (200 chars)
- Persistence callback saves SavedSession after each successful query

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed replacePair() called with 3 arguments in formatting/markdown.go**
- **Found during:** Task 1 (first go build attempt)
- **Issue:** `StripMarkdown()` called `replacePair(text, "**", "")` with 3 args but function signature was `replacePair(text, marker string) string` (2 args)
- **Fix:** Changed calls to `replacePair(text, "**")` and `replacePair(text, "__")` — matching the function that already hardcodes replacement to `""`
- **Files modified:** internal/formatting/markdown.go (lines 391-392)
- **Commit:** 3ad5031

**2. [Rule 2 - Missing Critical] Added nil-bot guard in middleware reply paths**
- **Found during:** Task 1 middleware test run (panic in TestMiddlewareAuthRejectsUnauthorized)
- **Issue:** `ctx.EffectiveMessage.Reply(nil, ...)` panicked when bot parameter was nil in unit tests
- **Fix:** Added `if tgBot != nil` guard before reply calls in both auth and rate limit middleware
- **Files modified:** internal/bot/middleware.go
- **Commit:** 3ad5031

**3. [Rule 2 - Missing Critical] Used config.BlockedPatterns package var instead of cfg field**
- **Found during:** Task 3 (first go build — cfg.BlockedPatterns undefined)
- **Issue:** Plan specified `cfg.BlockedPatterns` but Config struct has no such field; it's a package-level var `config.BlockedPatterns`
- **Fix:** Changed to `config.BlockedPatterns`
- **Files modified:** internal/handlers/text.go
- **Commit:** 5c0b94c

**4. [Rule 1 - Bug] Worker heuristic for new vs restored sessions**
- **Found during:** Task 3 design
- **Issue:** HandleText cannot distinguish "new session needing a worker" from "existing session already has a worker" because Session.IsRunning() refers to active queries, not whether a worker goroutine exists
- **Fix:** Heuristic: start worker only if `SessionID() == ""` AND `StartedAt()` is within 1 second (brand new session). Restored sessions have workers started by `restoreSessions()`.
- **Files modified:** internal/handlers/text.go
- **Note:** Deferred to Plan 07: pass bot context to HandleText so workers receive proper cancellation

## Self-Check: PASSED

All files exist, all commits found, `go build ./...` passes, `go test ./internal/bot/ -run TestMiddleware` passes (5/5 tests green).
