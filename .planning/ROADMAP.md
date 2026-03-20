# Roadmap: GSD Node

## Milestones

- v1.0 GSD Telegram Bot Go Rewrite — Phases 1-7 (shipped 2026-03-20) — [Archive](milestones/v1.0-ROADMAP.md)
- v1.1 Bugfixes — Phases 8-9 (shipped 2026-03-20) — [Archive](milestones/v1.1-ROADMAP.md)
- v1.2 Custom Webapp — Phases 10-14 (in progress)

## Phases

<details>
<summary>v1.0 GSD Telegram Bot Go Rewrite (Phases 1-7) - SHIPPED 2026-03-20</summary>

See [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md) for full phase details.

</details>

<details>
<summary>v1.1 Bugfixes (Phases 8-9) - SHIPPED 2026-03-20</summary>

See [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md) for full phase details.

- [x] Phase 8: Polling Stability — Fix HTTP client timeout so long-poll cycles complete without errors
- [x] Phase 9: Channel Auth — Fix channel sender auth and filter reflected bot messages

</details>

### v1.2 Custom Webapp (In Progress)

**Milestone Goal:** Replace Telegram bot interface with a custom WebSocket-based communication protocol, transforming the bot into standalone node software that connects to a central server.

- [x] **Phase 10: Protocol Definitions and Config** - Define all wire message types and extend config with WebSocket env vars (completed 2026-03-20)
- [ ] **Phase 11: WebSocket Connection Manager** - Build and validate outbound WebSocket client with reconnect and heartbeat
- [ ] **Phase 12: Telegram Removal and Session Migration** - Delete Telegram layer and migrate session identity from channel IDs to project keys
- [ ] **Phase 13: Dispatch, Instance Management, and Node Lifecycle** - Implement command dispatch, multi-instance management, and end-to-end node wiring
- [ ] **Phase 14: Protocol and Server Spec Documents** - Write wire protocol spec and server backend spec from working implementation

## Phase Details

### Phase 10: Protocol Definitions and Config
**Goal**: All wire message types and config fields exist so every downstream phase can build against a stable contract
**Depends on**: Nothing (first phase of v1.2)
**Requirements**: PROTO-01, PROTO-02, NODE-01, NODE-02
**Success Criteria** (what must be TRUE):
  1. `internal/protocol/messages.go` compiles with all inbound and outbound message struct definitions (`Envelope`, `execute`, `kill`, `status_request`, `node_register`, `stream_event`, `instance_started`, `instance_finished`, `instance_error`) and round-trip marshal/unmarshal tests pass
  2. `.env` accepts `SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS` and the node auto-derives its ID from hardware identifiers without any user-set config field
  3. The registration frame struct includes a running-instance snapshot field, preventing session divergence after reconnect from day one
**Plans:** 2/2 plans complete
Plans:
- [x] 10-01-PLAN.md — Protocol message types and Envelope with round-trip tests
- [x] 10-02-PLAN.md — Node ID derivation and NodeConfig with env var parsing

### Phase 11: WebSocket Connection Manager
**Goal**: A validated `ConnectionManager` that dials outbound, reconnects automatically, and serializes all writes through a single goroutine — the foundation every other phase depends on
**Depends on**: Phase 10
**Requirements**: XPORT-01, XPORT-02, XPORT-03, XPORT-04, XPORT-05, XPORT-06
**Success Criteria** (what must be TRUE):
  1. Node connects to a local mock server over `wss://` and the connection survives a simulated network drop, reconnecting with exponential backoff (500ms–30s with jitter) within observable delay
  2. Heartbeat ping fires every 30 seconds; the connection manager detects a dead server (no pong) within 90 seconds and initiates reconnect without goroutine leak (`go test -race ./...` clean)
  3. Node sends a registration frame after every reconnect, including re-register after recovery from drop
  4. Node sends an explicit disconnect frame on clean shutdown and does not reconnect afterward
  5. A stress test sending concurrent frames from multiple goroutines completes without panic — the single writer goroutine serializes all writes
**Plans**: TBD

### Phase 12: Telegram Removal and Session Migration
**Goal**: Zero Telegram imports in the codebase and session persistence keyed by project name rather than channel ID, so the dispatch layer can be built against clean identity types
**Depends on**: Phase 10
**Requirements**: CLEAN-01, CLEAN-02, CLEAN-03, CLEAN-04
**Success Criteria** (what must be TRUE):
  1. `grep -r "gotgbot\|openai\|telegram" internal/` returns no matches; `go.mod` no longer references `gotgbot/v2` or `openai-go`
  2. TypeScript source files and Bun/npm configuration files are absent from the repository
  3. `internal/session` compiles with `InstanceStore` keyed by `string` (instance UUID); `QueuedMessage` uses `InstanceID string` rather than `ChatID int64`
  4. `sessions.json` on disk uses project-name keys; a migration script converts existing channel-ID-keyed records, logging any unmappable entries rather than silently discarding them
**Plans**: TBD

### Phase 13: Dispatch, Instance Management, and Node Lifecycle
**Goal**: The node receives commands from the server, manages multiple Claude CLI instances simultaneously, streams their output, and shuts down cleanly — the full working node
**Depends on**: Phase 11, Phase 12
**Requirements**: PROTO-03, PROTO-04, PROTO-05, INST-01, INST-02, INST-03, INST-04, INST-05, INST-06, INST-07, NODE-03, NODE-04, NODE-05, NODE-06
**Success Criteria** (what must be TRUE):
  1. Node receives a `run` command, ACKs it before execution begins, spawns a Claude CLI subprocess in the correct project working directory, and streams NDJSON output back as `instance_chunk` events — all visible against a mock server
  2. Two simultaneous `run` commands targeting the same project both execute; each carries a distinct UUID in every outgoing frame; `instance_started`, `instance_finished`, and `instance_error` lifecycle events are emitted for each
  3. Node receives a `kill` command with an instance ID and the targeted Claude subprocess terminates; subsequent `instance_error` or `instance_finished` is emitted; other running instances are unaffected
  4. Node responds to a `status` query with the current running instance list
  5. On SIGINT/SIGTERM: active streams drain, remaining Claude subprocesses are killed (verified absent from process list within 10 seconds), disconnect frame is sent, and the process exits cleanly — no zombie processes
  6. All received commands appear in the audit log with source and command type; per-project rate limiting rejects excess `run` commands; structured log entries include `node_id`, `instance_id`, and `project` fields
**Plans**: TBD

### Phase 14: Protocol and Server Spec Documents
**Goal**: Written specs that give the server team a complete contract for implementing the server side — derived from the working node, not from intention
**Depends on**: Phase 13
**Requirements**: DOCS-01, DOCS-02
**Success Criteria** (what must be TRUE):
  1. `docs/protocol-spec.md` exists with the full message type catalog, `Envelope` format, authentication handshake sequence, reconnect behavior, and at least one sequence diagram covering the execute-stream-finish flow
  2. `docs/server-spec.md` exists describing the WebSocket endpoint contract, data models the server must maintain per node and per instance, and the OpenAI Whisper integration point for server-side voice-to-text
**Plans**: TBD

## Progress

**Execution Order:** 10 → 11 → 12 → 13 → 14

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-7. v1.0 Phases | v1.0 | 24/24 | Complete | 2026-03-20 |
| 8. Polling Stability | v1.1 | 1/1 | Complete | 2026-03-20 |
| 9. Channel Auth | v1.1 | 1/1 | Complete | 2026-03-20 |
| 10. Protocol Definitions and Config | v1.2 | 2/2 | Complete   | 2026-03-20 |
| 11. WebSocket Connection Manager | v1.2 | 0/? | Not started | - |
| 12. Telegram Removal and Session Migration | v1.2 | 0/? | Not started | - |
| 13. Dispatch, Instance Management, and Node Lifecycle | v1.2 | 0/? | Not started | - |
| 14. Protocol and Server Spec Documents | v1.2 | 0/? | Not started | - |
