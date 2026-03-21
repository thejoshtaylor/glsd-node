---
phase: 14-protocol-and-server-spec-documents
plan: 01
subsystem: docs
tags: [websocket, protocol, specification, mermaid, json]

# Dependency graph
requires:
  - phase: 10-protocol-message-types-and-node-config
    provides: "Message type structs and Envelope wire format"
  - phase: 11-websocket-connection-manager
    provides: "ConnectionManager dial, reconnect, heartbeat, single writer"
  - phase: 13-dispatch-multi-instance-and-graceful-shutdown
    provides: "Dispatcher command handling and instance lifecycle"
provides:
  - "Complete wire protocol specification for server implementers"
  - "Message type catalog with JSON schemas and field semantics"
  - "Sequence diagrams for all major flows"
affects: [14-02-server-backend-spec, server-implementation]

# Tech tracking
tech-stack:
  added: []
  patterns: ["spec-from-code derivation"]

key-files:
  created: [docs/protocol-spec.md]
  modified: []

key-decisions:
  - "Grouped message types by direction (outbound vs inbound) for server team clarity"
  - "Included InstanceSummary sub-object schema inline within node_register rather than as separate section"
  - "Documented sync.Once terminal event guarantee in instance_error section rather than a separate concurrency section"

patterns-established:
  - "Spec-from-code: every detail in the spec traced to a specific source file and struct"

requirements-completed: [DOCS-01]

# Metrics
duration: 3min
completed: 2026-03-21
---

# Phase 14 Plan 01: Wire Protocol Specification Summary

**Complete WebSocket wire protocol spec with 10 message types, 5 Mermaid sequence diagrams, auth handshake, reconnect backoff, and concurrency model -- all derived from working node code**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-21T01:28:22Z
- **Completed:** 2026-03-21T01:31:12Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- Documented all 10 message types with direction, JSON schema, field notes, and trigger conditions
- Documented Envelope wire format including Encode/Decode functions and NewMsgID
- Documented Bearer token authentication handshake during WebSocket upgrade
- Documented connection lifecycle with exponential backoff parameters (500ms-30s, full jitter)
- Documented heartbeat mechanism with configurable interval and 3x pong timeout
- Created 5 Mermaid sequence diagrams (connect, execute-stream-finish, kill, reconnect, clean shutdown)
- Documented concurrency model (single writer, reader, heartbeat, dispatcher goroutines)
- Documented error handling for all edge cases (rate limiting, malformed envelopes, write failures, recvCh overflow)

## Task Commits

Each task was committed atomically:

1. **Task 1: Write docs/protocol-spec.md** - `8d37bfb` (feat)

## Files Created/Modified
- `docs/protocol-spec.md` - Complete wire protocol specification (590 lines)

## Decisions Made
- Grouped message types by direction (outbound then inbound) for server implementer clarity
- Included InstanceSummary sub-object schema inline within node_register section
- Documented the sync.Once terminal event guarantee within instance_error rather than separately
- Used numbered sections matching the plan's prescribed structure

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Protocol spec complete, ready for server backend spec (14-02)
- Server team can implement the full WebSocket protocol from this document alone

---
*Phase: 14-protocol-and-server-spec-documents*
*Completed: 2026-03-21*
