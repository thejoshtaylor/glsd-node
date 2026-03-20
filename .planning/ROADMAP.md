# Roadmap: GSD Telegram Bot (Go Rewrite)

## Overview

Build a Go-native Telegram bot that controls Claude Code across multiple projects simultaneously, each in its own channel with its own Claude session. The build order is non-negotiable: correct infrastructure before multi-project features before media and deployment. Every phase delivers a coherent, verifiable capability that unblocks the next.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [ ] **Phase 1: Core Bot Infrastructure** - Single-channel bot that sends text to Claude and streams the response back, with all safety and persistence infrastructure correct
- [ ] **Phase 2: Multi-Project and GSD Integration** - Multiple independent Claude sessions across channels with full GSD workflow keyboard
- [ ] **Phase 3: Media Handlers and Windows Service** - Voice, photo, PDF processing and Windows Service deployment

## Phase Details

### Phase 1: Core Bot Infrastructure
**Goal**: A running Go bot that accepts text messages from one Telegram channel, routes them to a Claude CLI session, and streams the response back — with correct concurrency, persistence, auth, rate limiting, and audit logging
**Depends on**: Nothing (first phase)
**Requirements**: CORE-01, CORE-02, CORE-03, CORE-04, CORE-05, CORE-06, SESS-01, SESS-02, SESS-03, SESS-04, SESS-05, SESS-06, SESS-07, SESS-08, AUTH-01, AUTH-02, AUTH-03, CMD-01, CMD-02, CMD-03, CMD-04, CMD-05, PERS-01, PERS-02, PERS-03, DEPLOY-01, DEPLOY-03, DEPLOY-04
**Success Criteria** (what must be TRUE):
  1. User sends a text message to the bot and sees a live-updating response stream from Claude, with tool status emoji visible during execution
  2. User can run /start, /new, /stop, /status, /resume and each command produces correct output reflecting actual session state and token usage
  3. Bot survives a restart and automatically restores the previous session — user can resume without re-sending their message
  4. Bot rejects messages from users not in the channel, enforces rate limits per channel, and blocks forbidden path patterns before passing them to Claude
  5. Go binary compiles for Windows, resolves claude and pdftotext paths explicitly at startup (logged), and shuts down cleanly draining active sessions
**Plans:** 1/8 plans executed

Plans:
- [ ] 01-01-PLAN.md — Go module init, config package, audit logger
- [ ] 01-02-PLAN.md — Claude subprocess layer (NDJSON events, process spawn, streaming, kill)
- [x] 01-03-PLAN.md — Security subsystem (rate limiter, path validation, auth)
- [ ] 01-04-PLAN.md — Session management (store, worker queue, atomic JSON persistence)
- [ ] 01-05-PLAN.md — Formatting (MarkdownV2 conversion, tool emoji display)
- [ ] 01-06-PLAN.md — Bot skeleton, middleware, streaming state, text handler
- [ ] 01-07-PLAN.md — Command handlers (/start /new /stop /status /resume) and callbacks
- [ ] 01-08-PLAN.md — Main entry point, build verification, end-to-end smoke test

### Phase 2: Multi-Project and GSD Integration
**Goal**: Multiple Telegram channels each route to independent Claude sessions with no context bleed, and the GSD workflow is fully accessible via inline keyboard menus
**Depends on**: Phase 1
**Requirements**: PROJ-01, PROJ-02, PROJ-03, PROJ-04, PROJ-05, GSD-01, GSD-02, GSD-03, GSD-04, GSD-05
**Success Criteria** (what must be TRUE):
  1. User can operate two different project channels simultaneously — messages in channel A reach only session A's Claude, messages in channel B reach only session B's Claude
  2. When a message arrives in an unregistered channel, the bot prompts the user to link a project directory and completes the mapping
  3. Project-channel mappings survive a bot restart — channels reattach to their Claude sessions automatically
  4. User can tap /gsd and see all GSD operations as categorized inline keyboard buttons; tapping any button sends the correct command to Claude
  5. Claude responses containing /gsd: commands or numbered options render as tappable inline keyboard buttons in the Telegram message
**Plans**: TBD

### Phase 3: Media Handlers and Windows Service
**Goal**: Users can send voice messages, photos, and PDFs to any project channel and the bot processes them correctly; the bot installs as a Windows Service and starts at boot
**Depends on**: Phase 2
**Requirements**: MEDIA-01, MEDIA-02, MEDIA-03, MEDIA-04, MEDIA-05, DEPLOY-02
**Success Criteria** (what must be TRUE):
  1. User sends a voice message and receives a Claude response based on the transcribed text (via OpenAI Whisper)
  2. User sends a photo or an album of photos and Claude's response addresses the image content
  3. User attaches a PDF or text file and Claude's response addresses the document content
  4. Bot installs as a Windows Service via NSSM, starts automatically at boot without a terminal window, and resolves claude/pdftotext from explicit environment variables rather than PATH
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Core Bot Infrastructure | 1/8 | In Progress|  |
| 2. Multi-Project and GSD Integration | 0/TBD | Not started | - |
| 3. Media Handlers and Windows Service | 0/TBD | Not started | - |
