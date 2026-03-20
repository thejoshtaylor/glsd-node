# GSD Telegram Bot (Go Rewrite)

## What This Is

A Telegram bot that lets you control Claude Code from your phone via text, voice, photos, and documents — rewritten from TypeScript to Go. Supports multiple simultaneous projects, each linked to its own Telegram channel with independent Claude sessions. Includes full GSD workflow integration with interactive button menus.

## Core Value

Control Claude Code remotely from Telegram across multiple projects simultaneously, each in its own channel with its own Claude session.

## Requirements

### Validated

- [x] Claude CLI subprocess management with streaming output — Validated in Phase 1: Core Bot Infrastructure
- [x] Session persistence and resume across restarts — Validated in Phase 1
- [x] Rate limiting per channel — Validated in Phase 1
- [x] Audit logging — Validated in Phase 1
- [x] Streaming responses from Claude with live message updates — Validated in Phase 1
- [x] Markdown-to-HTML conversion for Telegram's message format — Validated in Phase 1
- [x] Tool status emoji formatting in responses — Validated in Phase 1
- [x] Safety layers: rate limiting, path validation, command safety checks — Validated in Phase 1

### Active

- [ ] Per-channel auth: channel membership = bot access (no global allowlist)

### Validated

- [x] Callback handler integration: workers drain on shutdown, resume/new use correct project dir, streaming respects rate limiter — Validated in Phase 4: Callback Handler Integration Fixes
- [x] Token usage tracking (input/output/cache) displayed in /status — Validated in Phase 5: Fix Session Metrics and GSD Persistence
- [x] Context window usage percentage displayed in /status — Validated in Phase 5: Fix Session Metrics and GSD Persistence
- [x] GSD keyboard-triggered sessions persist for /resume — Validated in Phase 5: Fix Session Metrics and GSD Persistence
- [x] Media handling: voice transcription (OpenAI), photos, PDFs (pdftotext), text/code documents — Validated in Phase 3
- [x] Windows Service deployment via NSSM — runs at boot, no terminal window — Validated in Phase 3
- [x] Multi-project support: each project linked to a separate Telegram channel — Validated in Phase 2
- [x] Independent Claude CLI sessions per project, running simultaneously — Validated in Phase 2
- [x] Dynamic project-channel assignment: unrecognized channels prompt user to link a project — Validated in Phase 2
- [x] Full GSD command integration via interactive Telegram button menus (all /gsd: commands) — Validated in Phase 2
- [x] JSON file persistence for project-channel mappings and session state — Validated in Phase 2

### Out of Scope

- macOS LaunchAgent support — previous version had this, Go version targets Windows
- Standalone binary compilation with Bun — replaced by native Go binary
- SQLite or database storage — JSON files preferred
- Docker deployment — Windows Service only
- Shared Claude sessions — each project is independent

## Context

The current implementation is a ~3,300 line TypeScript/Bun application. The Go rewrite is a ground-up redesign, not a line-by-line port. The existing codebase serves as a functional specification of what the bot should do, but the Go version should be idiomatically Go.

Key existing capabilities to preserve:
- Claude CLI subprocess management with streaming output
- Session persistence and resume across restarts
- Media group buffering for photo albums
- Tool status emoji formatting in responses
- Markdown-to-HTML conversion for Telegram's message format
- MCP server configuration support
- Safety layers: rate limiting, path validation, command safety checks

The bot currently runs on Windows 11. The Go version should be designed for Windows from the start, with a proper Windows Service installation path.

GSD integration currently has basic button support. The Go version needs full coverage of all /gsd: commands with dynamically generated inline keyboard menus.

## Constraints

- **Language**: Go — idiomatic Go patterns, goroutines for concurrency
- **Telegram API**: Use a mature Go Telegram library (e.g., telebot, gotgbot, or telegram-bot-api)
- **Claude CLI**: Wraps `claude` CLI subprocess (same approach as TypeScript version)
- **Voice transcription**: OpenAI Whisper API (same as current)
- **PDF extraction**: `pdftotext` CLI dependency (same as current)
- **Platform**: Windows 11, deployed as Windows Service
- **Storage**: JSON files for all persistence (no database)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go over TypeScript | User preference for Go rewrite | — Pending |
| Clean rewrite over port | Better architecture with multi-project baked in from design | — Pending |
| Separate channels per project | Clean separation, each project has its own space | — Pending |
| Independent Claude sessions | Allows simultaneous work on multiple projects | — Pending |
| Per-channel auth over allowlist | Simpler for multi-channel — membership = access | — Pending |
| JSON over SQLite | Simpler, no dependencies, sufficient for this use case | — Pending |
| Windows Service over Task Scheduler | Runs at boot without login, proper service management | — Pending |

---
*Last updated: 2026-03-20 — Phase 5 complete (session metrics + GSD persistence)*
