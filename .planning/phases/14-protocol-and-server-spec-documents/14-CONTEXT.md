# Phase 14: Protocol and Server Spec Documents - Context

**Gathered:** 2026-03-21
**Status:** Ready for planning

<domain>
## Phase Boundary

Write protocol-spec.md and server-spec.md documentation derived from the working node code (Phases 10-13). These specs give the server team everything they need to implement the server side — message catalog, Envelope format, authentication, reconnect behavior, sequence diagrams, WebSocket endpoint contract, data models, and Whisper integration point.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Key areas:
- Document structure and formatting
- Level of detail in sequence diagrams (Mermaid vs ASCII)
- How to organize the message type catalog
- Server data model recommendations
- Whisper integration description depth

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/protocol/messages.go` — All wire message types, Envelope format, type constants
- `internal/connection/manager.go` — Connection lifecycle, reconnect behavior, heartbeat
- `internal/connection/dial.go` — Auth handshake (Bearer token), backoff strategy
- `internal/connection/heartbeat.go` — Ping/pong intervals and timeouts
- `internal/connection/register.go` — Registration frame on connect/reconnect
- `internal/dispatch/dispatcher.go` — Command dispatch, instance lifecycle events, ACK flow
- `internal/config/node_config.go` — Configuration fields

### Established Patterns
- All specs should be derived from actual code, not from intention
- Mermaid diagrams work well in markdown for sequence flows

### Integration Points
- `docs/` directory — standard location for project documentation
- The specs are the deliverable — no code changes needed

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase

</specifics>

<deferred>
## Deferred Ideas

None

</deferred>
