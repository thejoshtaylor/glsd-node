---
phase: 17-dead-code-removal-and-test-fixes
plan: "01"
subsystem: security
tags: [dead-code, cleanup, telegram-removal, CLEAN-02, CLEAN-04]
dependency_graph:
  requires: []
  provides: [clean-security-package, session-package-removed]
  affects: [internal/security, internal/session]
tech_stack:
  added: []
  patterns: []
key_files:
  created: []
  modified:
    - internal/security/ratelimit.go
    - internal/security/ratelimit_test.go
    - internal/security/validate.go
    - internal/security/validate_test.go
  deleted:
    - internal/session/session.go
    - internal/session/session_test.go
    - internal/session/store.go
    - internal/session/store_test.go
    - internal/session/persist.go
    - internal/session/persist_test.go
    - internal/session/migrate.go
    - internal/session/migrate_test.go
decisions:
  - ChannelRateLimiter removed as Telegram-era dead code (no production imports)
  - IsAuthorized removed as Telegram-era dead code (no production imports)
  - internal/session removed entirely per CLEAN-04 (dispatcher manages sessions inline via proc.SessionID())
metrics:
  duration_seconds: 96
  completed_date: "2026-03-25"
  tasks_completed: 2
  tasks_total: 2
  files_modified: 4
  files_deleted: 8
requirements:
  - CLEAN-02
  - CLEAN-04
---

# Phase 17 Plan 01: Dead Code Removal (Security + Session) Summary

**One-liner:** Removed ChannelRateLimiter (int64-keyed), IsAuthorized (Telegram allowlist), and the entire internal/session package — all Telegram-era dead code with zero production imports.

## What Was Done

### Task 1: Delete ChannelRateLimiter and IsAuthorized dead code

Removed from `internal/security/ratelimit.go`:
- `ChannelRateLimiter` struct and all methods
- `NewChannelRateLimiter` constructor
- `ChannelRateLimiter.Allow(channelID int64) (bool, time.Duration)` method
- Unused `"time"` import

Removed from `internal/security/ratelimit_test.go`:
- `TestRateLimiterAllow`, `TestRateLimiterPerChannel`, `TestRateLimiterConcurrent`
- Unused `"time"` import

Removed from `internal/security/validate.go`:
- `IsAuthorized(userID int64, channelID int64, allowedUsers []int64) bool`

Removed from `internal/security/validate_test.go`:
- `TestIsAuthorizedYes`, `TestIsAuthorizedNo`, `TestIsAuthorizedChannelIDAccepted`, `TestIsAuthorizedEmptyList`

**Commit:** a23673e

### Task 2: Remove internal/session package entirely

Deleted all 8 files in `internal/session/`: session.go, session_test.go, store.go, store_test.go, persist.go, persist_test.go, migrate.go, migrate_test.go (2,004 lines total).

No production code in `internal/` or `cmd/` imported this package. The dispatcher manages session IDs directly via `proc.SessionID()`. The session package was Telegram-era scaffolding (one session per chat) partially migrated in Phase 12 but never wired into production.

**Commit:** c02b862

## Verification Results

- `grep -r "ChannelRateLimiter" internal/security/` — no matches
- `grep -r "IsAuthorized" internal/security/` — no matches
- `test ! -d internal/session` — exits 0
- `go build ./...` — exits 0
- `go test ./internal/security/... -run "TestProjectRateLimiter|TestValidatePath|TestCheckCommandSafety"` — all pass

## Decisions Made

- **ChannelRateLimiter removed**: was keyed by int64 (Telegram channel IDs), not used by dispatcher which uses ProjectRateLimiter with string project names
- **IsAuthorized removed**: was Telegram user allowlist check, no equivalent concept in WebSocket node
- **internal/session removed**: was Telegram one-session-per-chat model; dispatcher manages sessions inline

## Deviations from Plan

None — plan executed exactly as written.

Note: `TestValidatePathWindowsTraversal` was pre-existing failing on macOS (Windows filepath.Clean semantics required). This was addressed by a parallel agent adding a `runtime.GOOS != "windows"` skip guard — not part of this plan but compatible with its goals.

## Known Stubs

None.

## Self-Check: PASSED

- `internal/security/ratelimit.go` exists and contains only ProjectRateLimiter
- `internal/security/validate.go` exists and contains only ValidatePath and CheckCommandSafety
- `internal/session/` directory does not exist
- Commits a23673e and c02b862 exist in git log
