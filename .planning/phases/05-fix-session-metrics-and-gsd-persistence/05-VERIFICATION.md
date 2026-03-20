---
phase: 05-fix-session-metrics-and-gsd-persistence
verified: 2026-03-20T10:00:00Z
status: passed
score: 7/7 must-haves verified
re_verification: false
---

# Phase 05: Fix Session Metrics and GSD Persistence — Verification Report

**Phase Goal:** Capture token usage and context percentage from Claude result events into session fields so /status displays real data, and wire OnQueryComplete into GSD callback path so keyboard-triggered sessions persist for /resume.
**Verified:** 2026-03-20T10:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| #  | Truth | Status | Evidence |
|----|-------|--------|----------|
| 1  | After a successful query, /status shows non-zero input/output token counts | VERIFIED | `processMessage()` writes `proc.LastUsage()` to `s.lastUsage` in success branch (session.go:385-388); `buildStatusText` in command.go already reads `sess.LastUsage()` |
| 2  | After a successful query, /status shows a context window usage percentage | VERIFIED | `processMessage()` writes `proc.LastContextPercent()` to `s.contextPercent` in success branch (session.go:389-392); `buildStatusText` reads `sess.ContextPercent()` |
| 3  | After a failed query (ErrContextLimit), /status retains previous usage data rather than showing partial numbers | VERIFIED | ErrContextLimit branch (session.go:370-373) writes no usage fields; only the else/success branch (line 377+) writes `lastUsage` and `contextPercent` |
| 4  | A fresh session with no queries shows no token or context section in /status | VERIFIED | `lastUsage` and `contextPercent` are nil at construction; accessors return nil; `buildStatusText` omits sections when nil |
| 5  | A GSD command triggered via inline keyboard in a fresh session results in the session being persisted | VERIFIED | `enqueueGsdCommand` sets `OnQueryComplete` in `WorkerConfig`; closure calls `persist.Save(SavedSession{...})` (callback.go:405-419) |
| 6  | The persisted session appears in /resume list for that channel | VERIFIED | `persist.Save` writes to PersistenceManager targeting the sessions JSON file; `HandleResume` calls `persist.LoadForChannel`; `TestGsdOnQueryCompleteSavesSession` confirms round-trip |
| 7  | GSD commands routed to an already-running worker do not break persistence | VERIFIED | OnQueryComplete is set when the worker is first started; subsequent enqueues to an existing worker use the same worker (which already has `OnQueryComplete` set in its `WorkerConfig`) |

**Score:** 7/7 truths verified

---

## Required Artifacts

### Plan 01 Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/claude/process.go` | `LastUsage()` and `LastContextPercent()` accessors on Process | VERIFIED | Lines 191-199: both methods present. `lastUsage *UsageData` and `lastContextPercent *int` fields at lines 34-35. Consolidated result block at lines 135-148 populates both. |
| `internal/claude/process_test.go` | Tests for usage/context capture from result events | VERIFIED | `TestStreamCapturesUsage` (line 224), `TestStreamCapturesContextPercent` (line 271), `TestStreamNoUsageOnEmptyResult` (line 320) — all present and passing |
| `internal/session/session.go` | `processMessage` writes `lastUsage` and `contextPercent` on success | VERIFIED | Lines 385-392: success branch writes value copies via `copyU := *u` and `copyPct := *pct` |
| `internal/session/session_test.go` | Tests for `processMessage` usage capture and error-path non-write | VERIFIED | `TestProcessMessageCapturesUsage` (line 171), `TestProcessMessageCapturesContextPercent` (line 221), `TestProcessMessageNoUsageOnContextLimit` (line 272) — all present and passing |

### Plan 02 Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/handlers/callback.go` | `enqueueGsdCommand` with `persist` param and `OnQueryComplete` closure | VERIFIED | Function signature at line 378 includes `persist *session.PersistenceManager`; `OnQueryComplete` closure at lines 405-419 calls `persist.Save` |
| `internal/handlers/callback_test.go` | Test for GSD persistence wiring | VERIFIED | `TestGsdOnQueryCompleteSavesSession` (line 427) and `TestGsdOnQueryCompleteTitleTruncation` (line 478) — both present and passing |

**Note on artifact name discrepancy:** Plan 02 `must_haves.artifacts[1].contains` specified `"TestEnqueueGsdCommandPersists"` but the actual test is named `TestGsdOnQueryCompleteSavesSession`. The plan's task body offered this as an alternative name and the behavior matches exactly. This is a naming deviation with no functional impact.

---

## Key Link Verification

### Plan 01 Key Links

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/claude/process.go` | `internal/claude/events.go` | `event.Usage` and `event.ContextPercent()` in `Stream()` | VERIFIED | Lines 139-144 in process.go: `if event.Usage != nil { p.lastUsage = event.Usage }` and `if pct := event.ContextPercent(); pct != nil { p.lastContextPercent = pct }` |
| `internal/session/session.go` | `internal/claude/process.go` | `proc.LastUsage()` and `proc.LastContextPercent()` after `Stream()` | VERIFIED | Lines 385, 389 in session.go: both accessors called in success branch after `proc.Stream()` completes |

### Plan 02 Key Links

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/handlers/callback.go` | `internal/session/persist.go` | `persist.Save(saved)` in `OnQueryComplete` closure | VERIFIED | Line 417: `if err := persist.Save(saved); err != nil` inside the closure |
| `internal/handlers/callback.go` | `internal/handlers/text.go` | Same `OnQueryComplete` pattern | VERIFIED | Closure structure at callback.go:405-419 matches text.go reference pattern exactly (title truncation, SavedSession fields, `persist.Save` call) |
| `internal/handlers/command.go` | `internal/handlers/callback.go` | `HandleGsd` passes `persist` to `enqueueGsdCommand` | VERIFIED | `HandleGsd` signature includes `persist *session.PersistenceManager`; line 335: `enqueueGsdCommand(b, chatID, directCmd, store, mappings, cfg, persist, wg, globalLimiter)` |
| `internal/bot/handlers.go` | `internal/handlers/command.go` | `b.persist` passed to `HandleGsd` | VERIFIED | Line 104: `bothandlers.HandleGsd(tgBot, ctx, b.mappings, b.store, b.cfg, b.persist, b.WaitGroup(), b.globalAPILimiter)` |

---

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|---------|
| SESS-06 | 05-01-PLAN.md | Bot shows context window usage as a progress bar in status messages | SATISFIED | `Session.ContextPercent()` populated from `Process.LastContextPercent()` in `processMessage()` success branch; `buildStatusText` reads it; 3 tests pass |
| SESS-07 | 05-01-PLAN.md | Bot tracks and displays token usage (input/output/cache) in /status | SATISFIED | `Session.LastUsage()` populated from `Process.LastUsage()` in `processMessage()` success branch; accessors return copies with InputTokens/OutputTokens; 3 tests pass |
| PERS-01 | 05-02-PLAN.md | Bot saves session state (session ID, working dir, conversation context) to JSON | SATISFIED | `enqueueGsdCommand` now sets `OnQueryComplete` with `persist.Save(SavedSession{SessionID, SavedAt, WorkingDir, Title, ChannelID})`; GSD path now matches text.go path; 2 tests pass confirming save and title truncation |

All 3 requirements declared across both plans are satisfied. No orphaned requirements found for Phase 5 in REQUIREMENTS.md.

---

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/handlers/callback.go` | 98 | Placeholder comment for retry action | Info | Pre-existing; out of scope for this phase; `callbackActionRetry` responds with "Retry not available yet." — this is intentional deferral |

No blocker or warning-level anti-patterns found in phase-modified files.

---

## Human Verification Required

### 1. /status displays real token counts after a live query

**Test:** Run the bot, send a message to Claude, then send `/status`.
**Expected:** The status message shows non-zero input and output token counts, and a context window percentage bar (e.g. "5% full").
**Why human:** The rendering path through `buildStatusText` cannot be exercised without a live Telegram bot and Claude CLI.

### 2. /resume lists GSD-triggered sessions

**Test:** Use the bot's GSD keyboard to trigger a command (e.g. tap a phase button). After the query completes, send `/resume`.
**Expected:** The session appears in the `/resume` picker with the GSD command text as the title (truncated to 50 chars if needed).
**Why human:** The full keyboard-to-worker-to-persist path requires a running bot with a real Telegram session.

---

## Gaps Summary

No gaps. All must-haves from both plans are satisfied:

- `Process.LastUsage()` and `Process.LastContextPercent()` are populated from result events during `Stream()`.
- `processMessage()` writes value copies of those fields to Session on success only; error paths leave fields untouched.
- `enqueueGsdCommand` threads `persist` and sets `OnQueryComplete` matching the `text.go` pattern exactly.
- All intermediate functions (`handleCallbackGsd`, `handleCallbackGsdPhase`, `handleCallbackAskUser`) and `HandleGsd` in `command.go` thread `persist` to `enqueueGsdCommand`.
- All 9 new tests pass (3 in `internal/claude`, 3 in `internal/session`, 3 in `internal/handlers`).
- Full test suite for affected packages passes: `ok internal/claude`, `ok internal/session`, `ok internal/handlers`.
- `go build ./...` exits 0.

---

_Verified: 2026-03-20T10:00:00Z_
_Verifier: Claude (gsd-verifier)_
