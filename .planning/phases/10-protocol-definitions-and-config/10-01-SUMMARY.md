---
phase: 10-protocol-definitions-and-config
plan: 01
subsystem: api
tags: [go, websocket, protocol, json, encoding]

# Dependency graph
requires: []
provides:
  - "internal/protocol package with Envelope, Encode, Decode and all wire message types"
  - "Round-trip JSON marshal/unmarshal verified for all 8 message types"
  - "NodeRegister empty-array guarantee for running_instances field"
affects:
  - 11-websocket-connection
  - 12-session-migration
  - 13-multi-instance-manager
  - 14-server-backend-spec

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Envelope dispatcher pattern: json.RawMessage payload decoded after inspecting Type"
    - "Table-driven parallel round-trip tests for all message types"

key-files:
  created:
    - internal/protocol/messages.go
    - internal/protocol/messages_test.go
  modified: []

key-decisions:
  - "RunningInstances field on NodeRegister has no omitempty tag — guarantees [] not null when empty"
  - "Envelope.Payload is json.RawMessage — dispatch on Type before decoding avoids allocating unknown structs"
  - "No external dependencies added — stdlib encoding/json only"
  - "Version constant 1.2.0 defined in package for node handshake and NodeRegister payloads"

patterns-established:
  - "Protocol pattern: Encode(type, id, payload) -> Envelope -> json.Marshal -> send over WebSocket"
  - "Dispatch pattern: json.Unmarshal -> inspect Envelope.Type -> env.Decode(&typedStruct)"

requirements-completed: [PROTO-01, PROTO-02]

# Metrics
duration: 2min
completed: 2026-03-20
---

# Phase 10 Plan 01: Protocol Message Types and Envelope Summary

**Envelope dispatcher + all 8 wire message structs with round-trip JSON tests using stdlib only**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-20T23:06:13Z
- **Completed:** 2026-03-20T23:08:52Z
- **Tasks:** 1 (TDD: 2 commits — test then impl)
- **Files modified:** 2

## Accomplishments

- Created `internal/protocol/messages.go` with `Envelope`, `Encode`, `Decode`, `Version`, 8 type constants, and all 10 named structs
- Created `internal/protocol/messages_test.go` with `TestEnvelopeRoundTrip` (8 subtests), `TestNodeRegisterEmptyInstances`, and `TestEncodeError`
- All 10 tests pass; `go vet` and `go build` clean

## Task Commits

Each task was committed atomically via TDD:

1. **Task 1 RED: Failing tests** - `02a94fa` (test)
2. **Task 1 GREEN: Implementation** - `6a6fff4` (feat)

**Plan metadata:** (pending docs commit)

_Note: TDD task has two commits — failing test first, then implementation._

## Files Created/Modified

- `internal/protocol/messages.go` - Envelope frame, Encode/Decode helpers, Version const, 8 type constants, 10 struct types (ExecuteCmd, KillCmd, StatusRequest, NodeRegister, InstanceSummary, StreamEvent, InstanceStarted, InstanceFinished, InstanceError)
- `internal/protocol/messages_test.go` - Table-driven round-trip tests for all 8 envelope types plus empty-array and encode-error tests

## Decisions Made

- `RunningInstances` on `NodeRegister` intentionally omits `omitempty` — field must serialize as `[]` not `null` when no instances are running; callers must initialize with `make([]InstanceSummary, 0)`
- `Envelope.Payload` typed as `json.RawMessage` defers decoding until the dispatcher inspects `Type`, avoiding allocation of an unknown struct
- No external dependencies — `encoding/json` from stdlib is sufficient for the protocol layer

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. `go` binary not in default bash PATH on this Windows environment; resolved by adding `/c/Program Files/Go/bin` to PATH for test/vet/build commands.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `internal/protocol` package is stable and importable; all downstream phases (11-14) can import it immediately
- No breaking changes expected — Envelope + type constants are the stable contract
- Blocker documented in STATE.md: server authentication handshake first-frame format must be decided before Phase 11 connection lifecycle code is written

---
*Phase: 10-protocol-definitions-and-config*
*Completed: 2026-03-20*

## Self-Check: PASSED

- internal/protocol/messages.go: FOUND
- internal/protocol/messages_test.go: FOUND
- .planning/phases/10-protocol-definitions-and-config/10-01-SUMMARY.md: FOUND
- commit 02a94fa (test): FOUND
- commit 6a6fff4 (feat): FOUND
