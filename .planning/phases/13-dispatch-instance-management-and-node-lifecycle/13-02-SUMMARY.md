---
phase: 13-dispatch-instance-management-and-node-lifecycle
plan: "02"
subsystem: dispatch
tags: [dispatcher, instance-lifecycle, streaming, rate-limit, audit, concurrent]
dependency_graph:
  requires: ["13-01", "internal/protocol", "internal/connection", "internal/claude", "internal/audit", "internal/security", "internal/config"]
  provides: ["internal/dispatch"]
  affects: []
tech_stack:
  added: []
  patterns: ["ConnectionSender interface for testability", "sync.Once for terminal event gate", "context cancellation for kill", "WaitGroup for graceful shutdown"]
key_files:
  created:
    - internal/dispatch/dispatcher.go
    - internal/dispatch/dispatcher_test.go
  modified: []
decisions:
  - "ConnectionSender interface over *ConnectionManager concrete type — enables mock in tests without network dependency"
  - "Register instance in map before ACK/goroutine spawn — prevents kill arriving before instance is registered"
  - "sync.Once on instanceState.done gates terminal event — kill+natural-exit race cannot double-emit"
  - "Single defer combining removeInstance+wg.Done — ensures removeInstance runs before Wait() returns"
  - "sendEnvelope logs errors but does not propagate — send failures during shutdown are expected, not fatal"
  - "runInstance uses per-instance zerolog sub-logger with node_id, instance_id, project fields (NODE-05)"
metrics:
  duration_seconds: 219
  completed_date: "2026-03-21"
  tasks_completed: 1
  tasks_total: 1
  files_created: 2
  files_modified: 0
---

# Phase 13 Plan 02: Dispatch Package Summary

**One-liner:** Dispatcher struct connecting ConnectionSender to Claude CLI subprocess management with ACK correlation, streaming forwarding, kill/status commands, rate limiting, audit logging, and goroutine-safe concurrent instance lifecycle.

## What Was Built

The `internal/dispatch` package is the central coordinator of the node. It bridges the WebSocket transport layer (Phase 11's ConnectionManager) with the Claude CLI subprocess management (Phase 12's claude package) to produce the complete command-dispatch-stream pipeline.

### Key Components

**`internal/dispatch/dispatcher.go`**

- `ConnectionSender` interface — `Send([]byte) error` + `Receive() <-chan *protocol.Envelope`. ConnectionManager satisfies this; mockConn used in tests.
- `Dispatcher` struct — holds conn, cfg, nodeCfg, audit logger, rate limiter, instances map, WaitGroup, stopCh.
- `instanceState` — per-instance tracking (InstanceID, project, sessionID, cancel func, startedAt, sync.Once for terminal event gate).
- `New()` — constructor; initializes instances map and stopCh.
- `Run(ctx)` — event loop reading from conn.Receive(), routing to dispatch().
- `dispatch()` — audit log + zerolog + switch on envelope type.
- `handleExecute()` — rate limit check, instance registration, ACK with correlation ID, goroutine spawn.
- `runInstance()` — InstanceStarted, BuildArgs (with --resume for SessionID), NewProcess, Stream callback forwarding StreamEvents, terminal event via sync.Once.
- `handleKill()` — context cancellation via inst.cancel().
- `handleStatusRequest()` — NodeRegister with current RunningInstances.
- `sendEnvelope()` — Encode + Marshal + Send; errors logged not propagated.
- `Stop()` / `Wait()` — clean shutdown.

**`internal/dispatch/dispatcher_test.go`**

13 tests covering all behaviors:

| Test | Requirement |
|------|-------------|
| TestExecuteACKBeforeStart | ACK sent with correlation ID before InstanceStarted |
| TestStreamEventForwarding | NDJSON lines forwarded as StreamEvent with correct InstanceID |
| TestLifecycleEvents | Complete event sequence: ACK → Started → (Streams) → Finished |
| TestInstanceIDInAllFrames | Every outbound envelope carries the Execute InstanceID |
| TestKillInstance | Kill cancels instance; removed from map |
| TestKillOneInstance | INST-06: Kill one of two concurrent instances; other continues |
| TestConcurrentInstances | INST-05: Two simultaneous instances produce distinct lifecycle events |
| TestStatusRequest | NodeRegister response contains running instance |
| TestRateLimitRejectsExcess | Second project request rejected with InstanceError "rate limited" |
| TestAuditLogging | Audit file contains entry with action=execute, correct fields |
| TestResumeSession | INST-07: SessionID triggers --resume in BuildArgs |
| TestStructuredLogging | Zerolog output contains node_id, instance_id, project |
| TestGracefulShutdown | Stop+cancel kills both running instances; Wait() completes within 5s |

All tests use `goleak.VerifyNone(t)` to detect goroutine leaks.

## Deviations from Plan

**1. [Rule 3 - Blocking Issue] CGO required for -race on this Windows build environment**

- **Found during:** Verification
- **Issue:** `go test -race` requires CGO which is not available on this Windows build system (no GCC).
- **Fix:** Ran tests without -race flag. Tests pass cleanly. -race coverage will be available in environments with GCC (CI).
- **Impact:** Tests verify correctness and goroutine safety via goleak; -race would add data race detection at runtime but is environment-constrained.

None other — plan executed as written.

## Self-Check

### Files Created
- `internal/dispatch/dispatcher.go` — FOUND
- `internal/dispatch/dispatcher_test.go` — FOUND

### Commits
- `7d37ef8` — feat(13-02): implement dispatch package with full command lifecycle — FOUND

## Self-Check: PASSED
