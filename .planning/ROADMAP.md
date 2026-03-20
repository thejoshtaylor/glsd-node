# Roadmap: GSD Telegram Bot (Go Rewrite)

## Overview

Build a Go-native Telegram bot that controls Claude Code across multiple projects simultaneously, each in its own channel with its own Claude session. The build order is non-negotiable: correct infrastructure before multi-project features before media and deployment. Every phase delivers a coherent, verifiable capability that unblocks the next.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [x] **Phase 1: Core Bot Infrastructure** - Single-channel bot that sends text to Claude and streams the response back, with all safety and persistence infrastructure correct (completed 2026-03-20)
- [x] **Phase 2: Multi-Project and GSD Integration** - Multiple independent Claude sessions across channels with full GSD workflow keyboard (completed 2026-03-20)
- [ ] **Phase 3: Media Handlers and Windows Service** - Voice, photo, PDF processing and Windows Service deployment
- [x] **Phase 4: Callback Handler Integration Fixes** - Fix WaitGroup tracking, wrong working directory, and missing rate limiter in callback handlers (completed 2026-03-20)
- [x] **Phase 5: Fix Session Metrics and GSD Persistence** - Capture token/context data into session fields and wire OnQueryComplete into GSD callback path (completed 2026-03-20)
- [x] **Phase 6: Cross-Phase Safety Hardening** - Add typing indicators, audit logging, and command safety checks to all message paths (completed 2026-03-20)
- [ ] **Phase 7: Phase 3 Verification and Nyquist Compliance** - Formal verification of Phase 3, roadmap status update, Nyquist compliance

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
**Plans:** 8/8 plans complete

Plans:
- [x] 01-01-PLAN.md — Go module init, config package, audit logger
- [x] 01-02-PLAN.md — Claude subprocess layer (NDJSON events, process spawn, streaming, kill)
- [x] 01-03-PLAN.md — Security subsystem (rate limiter, path validation, auth)
- [ ] 01-04-PLAN.md — Session management (store, worker queue, atomic JSON persistence)
- [x] 01-05-PLAN.md — Formatting (MarkdownV2 conversion, tool emoji display)
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
**Plans:** 4/4 plans complete

Plans:
- [x] 02-01-PLAN.md — MappingStore (channel-project JSON persistence) + GSD pure functions (operations table, regex extractors, roadmap parser)
- [ ] 02-02-PLAN.md — Multi-project HandleText, /project command, per-project WorkerConfig, workerStarted, restoreSessions
- [ ] 02-03-PLAN.md — GSD /gsd command, callback routing extension, response button extraction, global API rate limiter, ask_user MCP
- [ ] 02-04-PLAN.md — Integration test suite + human verification checkpoint

### Phase 3: Media Handlers and Windows Service
**Goal**: Users can send voice messages, photos, and PDFs to any project channel and the bot processes them correctly; the bot installs as a Windows Service and starts at boot
**Depends on**: Phase 2
**Requirements**: MEDIA-01, MEDIA-02, MEDIA-03, MEDIA-04, MEDIA-05, DEPLOY-02
**Success Criteria** (what must be TRUE):
  1. User sends a voice message and receives a Claude response based on the transcribed text (via OpenAI Whisper)
  2. User sends a photo or an album of photos and Claude's response addresses the image content
  3. User attaches a PDF or text file and Claude's response addresses the document content
  4. Bot installs as a Windows Service via NSSM, starts automatically at boot without a terminal window, and resolves claude/pdftotext from explicit environment variables rather than PATH
**Plans:** 4 plans

Plans:
- [x] 03-01-PLAN.md — Config additions (PdfToTextPath) + shared helpers (downloadToTemp, transcribeVoice, extractPDF) + MediaGroupBuffer
- [ ] 03-02-PLAN.md — Voice handler (Whisper transcription) + Photo handler (single + album buffering)
- [ ] 03-03-PLAN.md — Document handler (PDF extraction via pdftotext + text file reading)
- [ ] 03-04-PLAN.md — Bot dispatcher wiring + NSSM Windows Service documentation + human verification

### Phase 4: Callback Handler Integration Fixes
**Goal**: Fix three integration findings in the callback handler chain so that callback-spawned workers drain on shutdown, callback resume/new use the correct project directory, and callback-triggered streaming respects the global API rate limiter
**Depends on**: Phase 3
**Requirements**: DEPLOY-04, SESS-06, PROJ-01, PROJ-03, PERS-03, CORE-06
**Gap Closure:** Closes FINDING-01, FINDING-02, FINDING-03 from v1.0 audit
**Success Criteria** (what must be TRUE):
  1. Callback-spawned workers are tracked by the bot's main WaitGroup and drained during graceful shutdown
  2. handleCallbackResume and handleCallbackNew resolve the channel's project mapping path, not cfg.WorkingDir
  3. enqueueGsdCommand passes the global API rate limiter to StreamingState, not nil

Plans:
- [ ] 04-01-PLAN.md — Thread bot WaitGroup, mapping lookup, and globalAPILimiter through callback handler chain

### Phase 5: Fix Session Metrics and GSD Persistence
**Goal**: Capture token usage and context percentage from Claude result events into session fields so /status displays real data, and wire OnQueryComplete into GSD callback path so keyboard-triggered sessions persist for /resume
**Depends on**: Phase 4
**Requirements**: SESS-06, SESS-07, PERS-01
**Gap Closure:** Closes INT-01, INT-02, Flow "Token usage and context display" from v1.0 audit
**Success Criteria** (what must be TRUE):
  1. After a query completes, /status shows non-zero input/output/cache token counts matching the last Claude response
  2. After a query completes, /status shows a context window usage percentage that reflects actual usage
  3. A GSD command triggered via inline keyboard button results in the session being persisted — it appears in /resume
**Plans:** 2/2 plans complete

Plans:
- [ ] 05-01-PLAN.md — Capture token usage and context percent from Process into Session fields
- [ ] 05-02-PLAN.md — Thread PersistenceManager into GSD callback chain and set OnQueryComplete

### Phase 6: Cross-Phase Safety Hardening
**Goal**: Ensure typing indicators, audit logging, and command safety checks apply uniformly to all message paths — not just text handler
**Depends on**: Phase 5
**Requirements**: CORE-03, CORE-06, AUTH-03
**Gap Closure:** Closes INT-03, INT-04, INT-05 from v1.0 audit
**Success Criteria** (what must be TRUE):
  1. Callback-triggered Claude calls (GSD buttons, resume, new) send a typing indicator while processing
  2. GSD/callback-triggered operations write entries to the audit log
  3. Voice transcripts, photo captions, document content, and GSD callback commands are checked by CheckCommandSafety before reaching Claude
**Plans:** 2/2 plans complete

Plans:
- [ ] 06-01-PLAN.md — Wire typing indicators, audit logging, and safety checks into callback handler path
- [ ] 06-02-PLAN.md — Add CheckCommandSafety to voice, photo, and document handlers

### Phase 7: Phase 3 Verification and Nyquist Compliance
**Goal**: Formally verify Phase 3 implementation (media handlers + Windows Service) that was executed but never verified, update Phase 3 roadmap status, and achieve Nyquist compliance for Phases 3 and 4
**Depends on**: Phase 6
**Requirements**: MEDIA-01, MEDIA-02, MEDIA-03, MEDIA-04, MEDIA-05, DEPLOY-02
**Gap Closure:** Closes 6 orphaned requirements from v1.0 audit
**Success Criteria** (what must be TRUE):
  1. 03-VERIFICATION.md exists with observable truths verified for all 6 requirements
  2. Phase 3 roadmap status updated to Complete
  3. Nyquist VALIDATION.md for Phases 3 and 4 shows nyquist_compliant: true

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Core Bot Infrastructure | 8/8 | Complete   | 2026-03-20 |
| 2. Multi-Project and GSD Integration | 4/4 | Complete   | 2026-03-20 |
| 3. Media Handlers and Windows Service | 1/4 | In Progress | - |
| 4. Callback Handler Integration Fixes | 1/1 | Complete   | 2026-03-20 |
| 5. Fix Session Metrics and GSD Persistence | 2/2 | Complete   | 2026-03-20 |
| 6. Cross-Phase Safety Hardening | 2/2 | Complete   | 2026-03-20 |
| 7. Phase 3 Verification and Nyquist Compliance | 0/0 | Pending | - |
