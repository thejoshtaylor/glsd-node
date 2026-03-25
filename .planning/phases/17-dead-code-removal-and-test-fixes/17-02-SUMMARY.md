---
phase: 17-dead-code-removal-and-test-fixes
plan: "02"
subsystem: testing
tags: [race-condition, test-fix, platform-guard]
dependency_graph:
  requires: []
  provides: [race-clean-test-suite]
  affects: [internal/dispatch, internal/security]
tech_stack:
  added: []
  patterns: [thread-safe-buffer, platform-skip-guard]
key_files:
  created: []
  modified:
    - internal/dispatch/dispatcher_test.go
    - internal/security/validate_test.go
decisions:
  - "zerolog.Nop() in newTestDispatcher eliminates race without requiring safeBuffer where log output is not inspected"
  - "safeBuffer wraps bytes.Buffer with sync.Mutex so TestStructuredLogging can safely read log output after concurrent goroutine writes"
  - "runtime.GOOS guard skips TestValidatePathWindowsTraversal on macOS — backslash traversal resolution requires Windows filepath.Clean semantics"
metrics:
  duration_seconds: 106
  completed_date: "2026-03-25"
  tasks_completed: 2
  files_modified: 2
---

# Phase 17 Plan 02: Fix Test Infrastructure Data Race and Platform Test Summary

Thread-safe safeBuffer for zerolog in dispatch tests, and runtime.GOOS skip guard for Windows-only traversal test.

## What Was Built

Two test-only fixes to make `go test -race ./...` pass clean on macOS:

1. **Dispatch test data race** — `bytes.Buffer` used as zerolog writer was concurrently written by `Run()` and `runInstance()` goroutines. Fixed by introducing a `safeBuffer` type (mutex-wrapped `bytes.Buffer`) and switching `newTestDispatcher` to `zerolog.Nop()` (no logging needed there). `TestStructuredLogging` uses `safeBuffer` to safely read log output post-goroutine.

2. **Windows traversal test on macOS** — `TestValidatePathWindowsTraversal` tested `filepath.Clean` behavior on backslash paths, which is Windows-only semantics. Added `runtime.GOOS != "windows"` skip guard so the test is correctly skipped on macOS instead of failing.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Fix dispatch test data race with thread-safe logger | b5829a8 | internal/dispatch/dispatcher_test.go |
| 2 | Add platform guard to TestValidatePathWindowsTraversal | d12b960 | internal/security/validate_test.go |

## Verification

- `go test -race ./internal/dispatch/...` — PASS, no DATA RACE output
- `go test -v ./internal/security/...` — TestValidatePathWindowsTraversal SKIP, TestValidatePathWindows PASS
- `go test -race ./...` — all 7 test packages pass, zero races, zero failures

## Deviations from Plan

None - plan executed exactly as written.

## Self-Check: PASSED

- internal/dispatch/dispatcher_test.go — safeBuffer type present, zerolog.Nop() in newTestDispatcher, var logBuf safeBuffer in TestStructuredLogging
- internal/security/validate_test.go — runtime.GOOS guard and t.Skip present in TestValidatePathWindowsTraversal
- Commits b5829a8 and d12b960 exist in git log
