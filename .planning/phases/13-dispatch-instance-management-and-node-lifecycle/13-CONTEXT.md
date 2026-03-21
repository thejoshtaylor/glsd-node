# Phase 13: Dispatch, Instance Management, and Node Lifecycle - Context

**Gathered:** 2026-03-21
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the full working node: command dispatch from server, multi-instance Claude CLI management, streaming output back to server, kill/status commands, graceful shutdown, audit logging, and per-project rate limiting. This is the integration phase that connects ConnectionManager (Phase 11) with Claude CLI subprocess management to produce the complete node.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Key areas:
- Dispatch architecture (how inbound frames from ConnectionManager route to handlers)
- Instance manager design (map of running instances, lifecycle tracking)
- Claude CLI spawning approach (leverage existing internal/claude/ package)
- Streaming output back to server (NDJSON events → protocol stream_event frames)
- Graceful shutdown orchestration (signal handling, instance draining)
- Audit log integration (extend existing internal/audit/ package)
- Rate limiting approach (leverage existing internal/security/ rate limiter)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/connection/manager.go` — ConnectionManager with Send()/Receive() for server communication
- `internal/protocol/messages.go` — All wire types: Execute, Kill, StatusRequest, StreamEvent, InstanceStarted/Finished/Error
- `internal/claude/process.go` — Existing Claude CLI subprocess management with streaming JSON parsing
- `internal/claude/events.go` — NDJSON event types from Claude CLI output
- `internal/session/session.go` — Session management (now string-keyed)
- `internal/audit/log.go` — Audit logging infrastructure
- `internal/security/ratelimit.go` — Token bucket rate limiter
- `internal/config/node_config.go` — NodeConfig with ServerURL, ServerToken, NodeID

### Established Patterns
- Go 1.24, zerolog structured logging
- Goroutine-based concurrency with channels
- TDD with goleak for goroutine leak detection
- Protocol: Envelope-based message framing with type discriminator

### Integration Points
- `ConnectionManager.Receive()` → dispatch inbound commands (Execute, Kill, StatusRequest)
- `ConnectionManager.Send()` → send outbound events (StreamEvent, InstanceStarted/Finished/Error)
- `internal/claude/` → spawn Claude CLI subprocesses per Execute command
- `internal/audit/` → log all received commands and lifecycle events
- `internal/security/` → rate limit per-project Execute commands
- `main.go` → wire everything together, handle OS signals

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase

</specifics>

<deferred>
## Deferred Ideas

None

</deferred>
