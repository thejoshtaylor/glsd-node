# GSD Telegram Bot (Go Rewrite)

## What This Is

A Telegram bot that lets you control Claude Code from your phone via text, voice, photos, and documents. Built in Go with 11,257 LOC across 50 files. Supports multiple simultaneous projects, each linked to its own Telegram channel with independent Claude sessions. Includes full GSD workflow integration with interactive button menus. Deploys as a Windows Service.

## Core Value

Control Claude Code remotely from Telegram across multiple projects simultaneously, each in its own channel with its own Claude session.

## Current State

**v1.0 shipped 2026-03-20** — Full Go rewrite complete.

- 7 phases, 24 plans, 44 requirements — all complete
- 11,257 lines of Go across 50 files (49 .go files + main.go)
- All automated tests pass (9 packages, 77+ handler tests)
- 5 human verification items deferred (live bot testing)
- 2 Nyquist VALIDATION.md files still in draft (Phases 5, 6)

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

### Active

(See REQUIREMENTS.md for v1.1 requirements)

## Current Milestone: v1.1 Bugfixes

**Goal:** Fix auth failures in Telegram channels and resolve polling timeout errors for stable daily use.

**Target features:**
- Fix channel-type auth: messages in Telegram channels fail auth because EffectiveSender is nil/channel ID
- Fix getUpdates polling timeout: HTTP client timeout shorter than long-poll duration causes context deadline exceeded
- Any additional bugs surfaced during investigation

### Out of Scope

- macOS LaunchAgent support — Go version targets Windows only
- SQLite or database storage — JSON files sufficient
- Docker deployment — Windows Service is target platform
- Shared Claude sessions — each project must be independent
- Video/audio file transcription — deferred to v2 (MEDIA-06, MEDIA-07)
- Archive file extraction — deferred to v2 (MEDIA-08)

## Context

The Go rewrite is complete — a ground-up redesign from the original ~3,300 line TypeScript/Bun application. The Go version is idiomatically Go with goroutines for concurrency, gotgbot/v2 for Telegram, and zerolog for structured logging.

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

---
*Last updated: 2026-03-20 after v1.1 milestone start*
