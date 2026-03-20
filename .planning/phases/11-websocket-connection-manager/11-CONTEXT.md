# Phase 11: WebSocket Connection Manager - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the `ConnectionManager` in `internal/connection/` that dials outbound to the server over WebSocket, reconnects automatically with exponential backoff, sends heartbeat pings, serializes all writes through a single goroutine, and sends registration frames on every connect/reconnect. This is the transport foundation all downstream phases build on.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Key areas:
- WebSocket library choice (gorilla/websocket vs nhooyr.io/websocket)
- Internal channel/goroutine architecture for write serialization
- Reconnect backoff strategy implementation (500ms–30s with jitter per success criteria)
- Heartbeat ping/pong implementation approach
- Mock server design for tests
- Error handling and logging patterns

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/protocol/messages.go` — All wire message types (Phase 10), including `NodeRegister`, `Envelope`, `Encode`/`Decode` helpers
- `internal/config/node_config.go` — `LoadNodeConfig()` with `ServerURL`, `ServerToken`, `HeartbeatIntervalSecs`, `NodeID`
- `internal/config/node_id.go` — `DeriveNodeID()` for registration frames

### Established Patterns
- Go 1.24, module `github.com/user/gsd-tele-go`
- Logging: `github.com/rs/zerolog` — structured logging with fields
- Config: godotenv for .env loading
- Package layout: `internal/{domain}/` with colocated `_test.go` files
- Protocol: `protocol.Encode(type, id, payload)` returns `[]byte`, `protocol.Decode(data)` returns `*Envelope`

### Integration Points
- Imports `internal/protocol` for message encoding/decoding
- Imports `internal/config` for `NodeConfig` (server URL, token, heartbeat interval, node ID)
- Future `internal/dispatch/` (Phase 13) will use ConnectionManager to send/receive frames

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase

</specifics>

<deferred>
## Deferred Ideas

None

</deferred>
