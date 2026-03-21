---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Custom Webapp
status: unknown
stopped_at: Completed 14-01-PLAN.md
last_updated: "2026-03-21T01:31:38.117Z"
progress:
  total_phases: 5
  completed_phases: 5
  total_plans: 11
  completed_plans: 11
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-20)

**Core value:** Run and orchestrate multiple Claude Code instances across projects from a central server, with each node managing its own local Claude sessions independently.
**Current focus:** Phase 14 — protocol-and-server-spec-documents

## Current Position

Phase: 14 (protocol-and-server-spec-documents) — EXECUTING
Plan: 2 of 2

## Accumulated Context

### Decisions

- [Phase 09]: Echo filter runs before channel auth; admin lookup auth moots channel-ID config
- [v1.2]: Outbound WebSocket — nodes connect to server, not the other way around
- [v1.2]: Multiple Claude CLI instances per project directory
- [v1.2]: Remove Telegram and TypeScript entirely
- [v1.2]: Use `coder/websocket` — not gorilla (archived 2022, panics on concurrent writes)
- [v1.2]: Single writer goroutine for all WebSocket sends — correctness requirement, not optional
- [v1.2]: Deliver protocol spec + server backend spec as documentation deliverables
- [Phase 10]: RunningInstances field on NodeRegister has no omitempty tag — guarantees [] not null when empty
- [Phase 10]: Envelope.Payload is json.RawMessage — dispatch on Type before decoding avoids allocating unknown structs
- [Phase 10]: protocol package uses stdlib only (encoding/json) — no external dependencies
- [Phase 10-02]: machineid.ProtectedID used as primary node ID source, hostname sha256 fallback for containers/CI
- [Phase 10-02]: NodeConfig separate from Config — no Telegram env vars required for WebSocket config
- [Phase 11]: Two-phase stopCh check in Send() prevents race when sendCh has buffer capacity after Stop()
- [Phase 11]: recvCh uses non-blocking send with drop+warn to avoid stalling reader goroutine on inbound frames
- [Phase 11]: Writer goroutine owns clean shutdown: sends NodeDisconnect then conn.Close() before exiting — ensures disconnect frame written while connection is healthy
- [Phase 11]: Stop() defers m.cancel() until after m.stopped: prevents context cancellation race with coder/websocket closing connection before disconnect frame is sent
- [Phase 12]: QueuedMessage.UserID (Telegram user ID) dropped entirely — source tracking will be added differently in Phase 13 dispatch layer
- [Phase 12]: MigrationResult.UnmappedEntries uses descriptive strings for human-readable logs; migration is one-time operation
- [Phase 12-telegram-removal-and-session-migration]: Telegram constant block removed entirely — TelegramMessageLimit etc are node-irrelevant
- [Phase 12-telegram-removal-and-session-migration]: Default audit log renamed from claude-telegram-audit.log to gsd-node-audit.log
- [Phase 12-telegram-removal-and-session-migration]: Config.Load() no longer requires TELEGRAM_BOT_TOKEN or TELEGRAM_ALLOWED_USERS
- [Phase 13]: ProjectRateLimiter.Allow() returns bool only — dispatcher needs allow/deny not delay duration
- [Phase 13]: protocol.NewMsgID() is canonical ID generator; connection.generateMsgID() delegates to it
- [Phase 13]: audit.Event redesigned with Source/NodeID/InstanceID/Project — Telegram int64 fields removed entirely
- [Phase 13]: ConnectionSender interface over concrete ConnectionManager type — testability without network dependency
- [Phase 13]: sync.Once on instanceState.done gates terminal event — kill+natural-exit race cannot double-emit
- [Phase 13]: Instance registered in map before ACK/goroutine spawn — prevents kill arriving before instance is tracked
- [Phase 13]: ConnectionManager.Start() before Dispatcher.Run() — recv channel must be ready before dispatcher reads from it
- [Phase 13]: Shutdown order: cancel context -> Stop dispatcher -> Wait (10s timeout) -> Stop ConnectionManager — ensures disconnect frame sent after instances drain
- [Phase 14]: Server spec derived from working node source code; Whisper integration as REST endpoint separate from WebSocket protocol
- [Phase 14]: Grouped message types by direction (outbound vs inbound) for server implementer clarity

### Pending Todos

None.

### Blockers/Concerns

- [Phase 10]: Server authentication handshake exact first-frame format and server ACK must be pinned during protocol design — coordinate with server team or decide unilaterally
- [Phase 11]: `coder/websocket` read deadline API differs from gorilla — verify method names before writing connection lifecycle code
- [Phase 12]: `sessions.json` migration: entries keyed by channels with no `mappings.json` match are unrecoverable — migration script must log losses; test on production copy before first v1.2 deploy

## Performance Metrics

**Velocity:**

- v1.1: 2 plans across 2 phases
- v1.0 reference: 24 plans across 7 phases

## Session Continuity

Last session: 2026-03-21T01:31:38.110Z
Stopped at: Completed 14-01-PLAN.md
Resume file: None
