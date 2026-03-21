---
phase: 14-protocol-and-server-spec-documents
plan: 02
subsystem: documentation
tags: [server-spec, websocket, protocol, documentation]
dependency_graph:
  requires: [14-01]
  provides: [server-spec]
  affects: [docs]
tech_stack:
  added: []
  patterns: [spec-driven-design]
key_files:
  created:
    - docs/server-spec.md
  modified: []
decisions:
  - "Server spec derived from working node source code, not intention documents"
  - "Whisper integration described as server-side REST endpoint, separate from WebSocket protocol"
  - "Terminal event guarantee documented as relying on node's sync.Once gate"
metrics:
  duration: "2m 18s"
  completed: "2026-03-21T01:30:45Z"
---

# Phase 14 Plan 02: Server Backend Specification Summary

Server backend specification covering WebSocket endpoint, data models, command dispatch, and OpenAI Whisper integration -- all derived from working node implementation source code.

## What Was Done

### Task 1: Write docs/server-spec.md

Created `docs/server-spec.md` with 10 sections covering the complete server implementation contract:

1. **Overview** -- Server role, architecture properties, communication model
2. **WebSocket Endpoint** -- URL, Bearer auth during HTTP upgrade, first-frame expectation (`node_register`), connection behavior (reconnect, heartbeat, clean disconnect)
3. **Data Models** -- Node model (7 fields: node_id, platform, version, projects, connected_at, last_heartbeat, status) and Instance model (7 fields: instance_id, node_id, project, session_id, status, started_at, finished_at) with relationship rules
4. **Command Dispatch** -- execute (with full response flow diagram), kill (wait for terminal event), status_request (correlated by envelope ID)
5. **Inbound Event Handling** -- 7 event types with trigger conditions and required server actions; terminal event guarantee via sync.Once
6. **State Reconciliation** -- Four-case reconciliation algorithm for node reconnect with running_instances
7. **Node Health Monitoring** -- Heartbeat-based health (30s pings, 90s stale threshold), clean disconnect sequence
8. **OpenAI Whisper Integration** -- REST endpoint for audio upload, Whisper API call, text-to-execute pipeline; entirely server-side
9. **Security Considerations** -- Token rotation, node identity tracking, rate limiting, command validation, audit trail
10. **Deployment Topology** -- Connection diagram, NAT-friendly architecture, single-port design

## Deviations from Plan

None -- plan executed exactly as written.

## Decisions Made

1. **Spec derived from source code**: Every data model field, command payload, and event type was verified against the actual Go structs in `internal/protocol/messages.go`, `internal/connection/`, and `internal/dispatch/dispatcher.go`.
2. **Whisper as REST endpoint**: Documented as a separate REST endpoint rather than part of the WebSocket protocol, since binary audio does not belong in JSON text frames.
3. **Terminal event guarantee**: Documented that the server does NOT need to deduplicate terminal events, relying on the node's `sync.Once` gate.

## Verification

All acceptance criteria passed:
- All 10 sections present with correct headers
- `node_id` referenced 9 times, `instance_id` 13 times
- `execute`, `kill`, `status_request` commands documented
- Bearer auth documented (4 references)
- Whisper referenced 8 times (case-insensitive)
- `running_instances` referenced 6 times (reconnect reconciliation)

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | a92aab0 | Server backend specification document |

## Self-Check: PASSED

- [x] `docs/server-spec.md` exists (379 lines)
- [x] Commit a92aab0 exists
