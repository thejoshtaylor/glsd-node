# GSD Node

## What This Is

A local Go service that manages Claude Code CLI sessions across multiple projects and instances. Connects outbound to a central server via WebSocket, enabling remote orchestration without firewall changes. Each project maps to a working directory; multiple Claude CLI instances can run simultaneously within the same project. Includes full GSD workflow integration. Deploys as a Windows Service.

## Core Value

Run and orchestrate multiple Claude Code instances across projects from a central server, with each node managing its own local Claude sessions independently.

## Current Milestone: v1.2 Custom Webapp

**Goal:** Replace Telegram bot interface with a custom WebSocket-based communication protocol, transforming the bot into standalone node software that connects to a central server.

**Target features:**
- Remove Telegram and TypeScript dependencies entirely
- Implement outbound WebSocket connection to central server
- Support multiple simultaneous Claude CLI instances per project
- Node health/heartbeat and online status reporting
- Efficient command dispatch from server to node
- Deliver protocol spec and server backend spec as documentation

## Current State

**v1.2 in progress** — Phase 10 complete as of 2026-03-20.

- v1.0: 7 phases, 24 plans — full Go rewrite shipped
- v1.1: 2 phases, 2 plans — polling stability + channel auth
- v1.2 Phase 10: Protocol message types and node config — stable wire contract established
- v1.2 Phase 11: WebSocket ConnectionManager — dial, reconnect, heartbeat, single writer
- v1.2 Phase 12: Telegram removed, session keys migrated to instance UUID strings
- ~3,500 lines of Go across 24 files (massive deletion of Telegram/TS code)
- All automated tests pass (7 packages: audit, claude, config, connection, protocol, security, session)
- 4 human verification items deferred (live Telegram bot testing)

## Requirements

### Validated (v1.0)

- [x] Claude CLI subprocess management with streaming output — v1.0 Phase 1
- [x] Session persistence and resume across restarts — v1.0 Phase 1
- [x] Rate limiting per channel — v1.0 Phase 1
- [x] Audit logging — v1.0 Phase 1
- [x] Streaming responses from Claude with live message updates — v1.0 Phase 1
- [x] Markdown-to-HTML conversion for Telegram's message format — v1.0 Phase 1
- [x] Tool status emoji formatting in responses — v1.0 Phase 1
- [x] Safety layers: rate limiting, path validation, command safety checks — v1.0 Phases 1+6
- [x] Multi-project support: each project linked to a separate Telegram channel — v1.0 Phase 2
- [x] Independent Claude CLI sessions per project, running simultaneously — v1.0 Phase 2
- [x] Dynamic project-channel assignment — v1.0 Phase 2
- [x] Full GSD command integration via interactive Telegram button menus — v1.0 Phase 2
- [x] JSON file persistence for project-channel mappings and session state — v1.0 Phase 2
- [x] Media handling: voice (OpenAI Whisper), photos, PDFs (pdftotext), text/code documents — v1.0 Phase 3
- [x] Windows Service deployment via NSSM — v1.0 Phase 3
- [x] Callback handler integration fixes — v1.0 Phase 4
- [x] Token usage and context percentage in /status — v1.0 Phase 5
- [x] GSD keyboard sessions persist for /resume — v1.0 Phase 5
- [x] Cross-phase safety hardening (typing, audit, safety checks uniform) — v1.0 Phase 6

### Validated (v1.1)

- [x] Long-polling getUpdates without context deadline exceeded errors — v1.1 Phase 8
- [x] Channel auth via admin lookup — channels authorized if an allowed user is admin — v1.1 Phase 9
- [x] Echo loop prevention — bot's own reflected channel posts and linked-channel forwards filtered — v1.1 Phase 9

### Out of Scope

- macOS LaunchAgent support — Go version targets Windows only
- SQLite or database storage — JSON files sufficient
- Docker deployment — Windows Service is target platform
- Shared Claude sessions — each project must be independent
- Video/audio file transcription — deferred to future (MEDIA-06, MEDIA-07)
- Archive file extraction — deferred to future (MEDIA-08)
- Auth rejection suppression in public channels — deferred (AUTH-03)

## Context

The Go rewrite is complete — a ground-up redesign from the original ~3,300 line TypeScript/Bun application. The Go version is idiomatically Go with goroutines for concurrency and zerolog for structured logging. v1.2 pivots from Telegram bot to standalone node software that connects to a central server. The Telegram and TypeScript layers are being removed entirely.

Architecture: Node connects outbound to server (no firewall changes). Server manages multiple nodes, each node manages multiple projects (directories), each project can have multiple Claude CLI instances. Communication via WebSocket with a custom binary/JSON protocol.

Tech stack: Go 1.23+, zerolog, godotenv, golang.org/x/time, gorilla/websocket (or nhooyr.io/websocket)
External deps: claude CLI, pdftotext (poppler), NSSM (Windows Service)

## Constraints

- **Language**: Go — idiomatic Go patterns, goroutines for concurrency
- **Communication**: Outbound WebSocket to server — no inbound ports, no firewall changes
- **Claude CLI**: Wraps `claude` CLI subprocess with NDJSON streaming
- **PDF extraction**: `pdftotext` CLI dependency
- **Platform**: Windows 11, deployed as Windows Service via NSSM
- **Storage**: JSON files for all persistence (no database)
- **Multi-instance**: Multiple Claude CLI subprocesses per project directory

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go over TypeScript | User preference for Go rewrite | ✓ Good — 11K LOC, clean architecture |
| Clean rewrite over port | Better architecture with multi-project baked in from design | ✓ Good — cleaner than TS version |
| gotgbot/v2 for Telegram | Mature, well-maintained, good dispatcher model | ✓ Good — dispatcher groups work well for middleware |
| Separate channels per project | Clean separation, each project has its own space | ✓ Good — no context bleed |
| Independent Claude sessions | Allows simultaneous work on multiple projects | ✓ Good — goroutine-per-session model |
| Per-channel auth over allowlist | Simpler for multi-channel — membership = access | ✓ Good |
| JSON over SQLite | Simpler, no dependencies, sufficient for this use case | ✓ Good — atomic write-rename pattern |
| Windows Service over Task Scheduler | Runs at boot without login, proper service management | ✓ Good — NSSM handles restarts |
| NDJSON streaming over REST API | Matches claude CLI output format, real-time updates | ✓ Good — StatusCallback pattern |
| Token bucket rate limiting | Per-channel, goroutine-safe, configurable | ✓ Good — golang.org/x/time/rate |
| Admin lookup for channel auth | Zero config — channels auto-authorize if an allowed user is admin | ✓ Good — no channel IDs in .env needed |
| 15-min admin cache TTL | Balance freshness vs API load for GetChatAdministrators | ✓ Good — sync.Map with inline expiry |
| Outbound WebSocket over inbound API | Nodes behind NAT/firewalls — no user config needed | — Pending |
| Remove Telegram entirely | Node software doesn't need chat platform coupling | — Pending |
| Multiple instances per project | Parallel GSD execution in same directory | — Pending |
| Spec-first server design | Build node first, deliver specs for server repo | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-20 after v1.2 milestone start*
