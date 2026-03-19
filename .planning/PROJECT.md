# GSD Telegram Bot (Go Rewrite)

## What This Is

A Telegram bot that lets you control Claude Code from your phone via text, voice, photos, and documents — rewritten from TypeScript to Go. Supports multiple simultaneous projects, each linked to its own Telegram channel with independent Claude sessions. Includes full GSD workflow integration with interactive button menus.

## Core Value

Control Claude Code remotely from Telegram across multiple projects simultaneously, each in its own channel with its own Claude session.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Full rewrite of Telegram bot in Go (not a port — clean redesign)
- [ ] Multi-project support: each project linked to a separate Telegram channel
- [ ] Independent Claude CLI sessions per project, running simultaneously
- [ ] Dynamic project-channel assignment: unrecognized channels prompt user to link a project
- [ ] Full GSD command integration via interactive Telegram button menus (all /gsd: commands)
- [ ] Updated GSD button functionality matching latest GSD version from GitHub
- [ ] All media types: text, voice transcription (OpenAI), photos, PDFs (pdftotext), video, audio, documents
- [ ] Per-channel auth: channel membership = bot access (no global allowlist)
- [ ] JSON file persistence for project-channel mappings and session state
- [ ] Streaming responses from Claude with live message updates
- [ ] Windows Service deployment via NSSM — runs at boot, no terminal window
- [ ] Rate limiting per channel
- [ ] Audit logging

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
*Last updated: 2026-03-19 after initialization*
