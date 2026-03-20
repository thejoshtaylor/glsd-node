# Phase 10: Protocol Definitions and Config - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Define all wire message types (Go structs in `internal/protocol/`) and config fields for the WebSocket-based node-server protocol. This phase produces the stable contract that all downstream phases (11-14) build against.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Key areas:
- Message struct field naming and JSON tags
- Envelope wrapper design (type discriminator approach)
- Config parsing strategy (extend existing `internal/config/` or new package)
- Test structure for round-trip marshal/unmarshal
- Hardware ID derivation method for node identity

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/config/config.go` — existing config loading with godotenv and env parsing; extend for new fields
- `internal/claude/events.go` — existing NDJSON event parsing pattern (may inform protocol event design)
- `internal/session/persist.go` — JSON persistence pattern for session data

### Established Patterns
- Go 1.24, module `github.com/user/gsd-tele-go`
- Logging: `github.com/rs/zerolog`
- Config: `github.com/joho/godotenv` for .env loading
- Package layout: `internal/{domain}/` with `_test.go` colocated
- Test files colocated with source (e.g., `config_test.go` next to `config.go`)

### Integration Points
- `internal/config/` — new env vars (`SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS`) extend existing config
- `.env` — new fields added alongside existing Telegram config
- Future `internal/connection/` (Phase 11) will import `internal/protocol/` types

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase

</specifics>

<deferred>
## Deferred Ideas

None

</deferred>
