# Requirements: GSD Node

**Defined:** 2026-03-20
**Core Value:** Run and orchestrate multiple Claude Code instances across projects from a central server, with each node managing its own local Claude sessions independently.

## v1.2 Requirements

Requirements for the custom webapp milestone. Each maps to roadmap phases.

### Transport

- [ ] **XPORT-01**: Node connects outbound to server via WebSocket (`wss://`) with TLS
- [ ] **XPORT-02**: Node sends heartbeat ping every 30s; detects dead connection via pong timeout
- [ ] **XPORT-03**: Node automatically reconnects with exponential backoff (500ms–30s) and jitter
- [ ] **XPORT-04**: Node re-registers with server after every reconnect
- [ ] **XPORT-05**: Node sends explicit disconnect frame on clean shutdown
- [ ] **XPORT-06**: All outbound WebSocket writes go through a single writer goroutine

### Protocol

- [x] **PROTO-01**: Node sends registration frame on connect: node_id (hardware-derived), platform, project list, version
- [x] **PROTO-02**: Message envelope format: `type` + `id` + JSON payload for all frames
- [ ] **PROTO-03**: Node receives and ACKs `run` commands before execution begins
- [ ] **PROTO-04**: Node receives and handles `kill` commands to terminate specific instances
- [ ] **PROTO-05**: Node receives and responds to `status` queries with running instance list

### Instance Management

- [ ] **INST-01**: Node spawns Claude CLI subprocess on `run` command with project working directory
- [ ] **INST-02**: Each instance gets a UUID, included in every outgoing frame
- [ ] **INST-03**: Node streams Claude NDJSON output to server as `instance_chunk` events
- [ ] **INST-04**: Node sends lifecycle events: `instance_started`, `instance_finished`, `instance_error`
- [ ] **INST-05**: Multiple Claude instances can run simultaneously in the same project directory
- [ ] **INST-06**: Node can kill a specific instance by ID on server command
- [ ] **INST-07**: Instances use `--resume SESSION_ID` to maintain persistent Claude sessions across restarts

### Node Lifecycle

- [ ] **NODE-01**: Node ID auto-derived from hardware identifiers (machine ID or hostname hash) — not user-configured
- [ ] **NODE-02**: Config via `.env`: `SERVER_URL`, `SERVER_TOKEN`, `HEARTBEAT_INTERVAL_SECS`
- [ ] **NODE-03**: Graceful shutdown drains active streams, kills remaining processes, sends disconnect
- [ ] **NODE-04**: Per-project rate limiting on incoming `run` commands using token bucket
- [ ] **NODE-05**: Structured logging with `node_id`, `instance_id`, `project` context fields
- [ ] **NODE-06**: Audit logging for all received commands with source and command type

### Cleanup

- [ ] **CLEAN-01**: Remove `internal/bot/` and all Telegram handler code
- [ ] **CLEAN-02**: Remove gotgbot/v2 and openai-go dependencies from go.mod
- [ ] **CLEAN-03**: Remove TypeScript source files and Bun/npm configuration
- [ ] **CLEAN-04**: Migrate session persistence keys from Telegram channel IDs to project-based keys

### Documentation

- [ ] **DOCS-01**: Communication protocol spec — full message type catalog, envelope format, sequence diagrams, authentication handshake
- [ ] **DOCS-02**: Server backend structure spec — API design, data models, node management, OpenAI Whisper integration for voice-to-text (server-side)

## Future Requirements

Deferred to v1.2.x or later. Tracked but not in current roadmap.

### Post-Validation

- **FUTURE-01**: Token and context usage forwarded in `instance_finished` events
- **FUTURE-02**: Streaming chunk throttle — 100ms minimum interval per instance
- **FUTURE-03**: TLS certificate pinning for server connection
- **FUTURE-04**: Command timeout enforcement on node side
- **FUTURE-05**: Prometheus-compatible metrics endpoint

## Out of Scope

| Feature | Reason |
|---------|--------|
| Inbound listening port on node | Breaks NAT traversal requirement; nodes must not need firewall changes |
| Server-side command queue for offline nodes | Stale commands cause more problems than they solve; server operator reissues intentionally |
| Binary framing (protobuf, MessagePack) | JSON is debuggable, already used throughout; Claude CLI latency dwarfs serialization |
| Shared Claude sessions across projects | Validated out of scope in v1.0 — context bleed risk |
| HTTP REST fallback transport | Doubles protocol surface; reconnect handles transient failures |
| Node-side web dashboard | Server is the management plane; node is headless by design |
| Auto-discovery / mDNS | Unnecessary for single-server deployment; static URL is simpler |
| Frontend / UI | User will build separately; server spec is the contract |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| XPORT-01 | Phase 11 | Pending |
| XPORT-02 | Phase 11 | Pending |
| XPORT-03 | Phase 11 | Pending |
| XPORT-04 | Phase 11 | Pending |
| XPORT-05 | Phase 11 | Pending |
| XPORT-06 | Phase 11 | Pending |
| PROTO-01 | Phase 10 | Complete |
| PROTO-02 | Phase 10 | Complete |
| PROTO-03 | Phase 13 | Pending |
| PROTO-04 | Phase 13 | Pending |
| PROTO-05 | Phase 13 | Pending |
| INST-01 | Phase 13 | Pending |
| INST-02 | Phase 13 | Pending |
| INST-03 | Phase 13 | Pending |
| INST-04 | Phase 13 | Pending |
| INST-05 | Phase 13 | Pending |
| INST-06 | Phase 13 | Pending |
| INST-07 | Phase 13 | Pending |
| NODE-01 | Phase 10 | Pending |
| NODE-02 | Phase 10 | Pending |
| NODE-03 | Phase 13 | Pending |
| NODE-04 | Phase 13 | Pending |
| NODE-05 | Phase 13 | Pending |
| NODE-06 | Phase 13 | Pending |
| CLEAN-01 | Phase 12 | Pending |
| CLEAN-02 | Phase 12 | Pending |
| CLEAN-03 | Phase 12 | Pending |
| CLEAN-04 | Phase 12 | Pending |
| DOCS-01 | Phase 14 | Pending |
| DOCS-02 | Phase 14 | Pending |

**Coverage:**
- v1.2 requirements: 30 total
- Mapped to phases: 30
- Unmapped: 0

---
*Requirements defined: 2026-03-20*
*Last updated: 2026-03-20 after roadmap creation*
