# GSD Telegram Bot (Go Rewrite)

## What This Is

A Telegram bot that lets you control Claude Code from your phone via text, voice, photos, and documents. Built in Go with ~11,600 LOC across 52 files. Supports multiple simultaneous projects, each linked to its own Telegram channel with independent Claude sessions. Includes full GSD workflow integration with interactive button menus. Deploys as a Windows Service.

## Core Value

Control Claude Code remotely from Telegram across multiple projects simultaneously, each in its own channel with its own Claude session.

## Current State

**v1.1 shipped 2026-03-20** — Bugfix release complete.

- v1.0: 7 phases, 24 plans — full Go rewrite shipped
- v1.1: 2 phases, 2 plans — polling stability + channel auth
- ~11,600 lines of Go across 52 files
- All automated tests pass (9 packages)
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

The Go rewrite is complete — a ground-up redesign from the original ~3,300 line TypeScript/Bun application. The Go version is idiomatically Go with goroutines for concurrency, gotgbot/v2 for Telegram, and zerolog for structured logging. v1.1 added channel authorization via admin lookup and fixed polling timeouts.

Tech stack: Go 1.23+, gotgbot/v2, zerolog, godotenv, golang.org/x/time
External deps: claude CLI, pdftotext (poppler), OpenAI Whisper API, NSSM (Windows Service)

## Constraints

- **Language**: Go — idiomatic Go patterns, goroutines for concurrency
- **Telegram API**: gotgbot/v2 (PaulSonOfLars/gotgbot)
- **Claude CLI**: Wraps `claude` CLI subprocess with NDJSON streaming
- **Voice transcription**: OpenAI Whisper API
- **PDF extraction**: `pdftotext` CLI dependency
- **Platform**: Windows 11, deployed as Windows Service via NSSM
- **Storage**: JSON files for all persistence (no database)

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

---
*Last updated: 2026-03-20 after v1.1 milestone*
