# Requirements: GSD Telegram Bot (Go Rewrite)

**Defined:** 2026-03-19
**Core Value:** Control Claude Code remotely from Telegram across multiple projects simultaneously, each in its own channel with its own Claude session.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Core Infrastructure

- [x] **CORE-01**: Bot connects to Telegram via long polling and receives messages from multiple channels
- [x] **CORE-02**: Bot loads configuration from environment variables and/or config file
- [x] **CORE-03**: Bot sends typing indicators while processing requests
- [x] **CORE-04**: Bot reports errors back to the user with truncated error messages
- [x] **CORE-05**: Bot rate-limits requests per channel using token bucket algorithm
- [x] **CORE-06**: Bot writes append-only audit log (timestamp, user, channel, action, message excerpt)

### Claude Session Management

- [x] **SESS-01**: Bot spawns and manages Claude CLI as a subprocess with streaming JSON output
- [x] **SESS-02**: Bot streams Claude responses with throttled edit-in-place message updates
- [x] **SESS-03**: Bot displays tool execution status with emoji indicators during streaming
- [x] **SESS-04**: User can send text messages that are routed to the channel's Claude session
- [x] **SESS-05**: User can interrupt a running query by sending a message prefixed with `!`
- [x] **SESS-06**: Bot shows context window usage as a progress bar in status messages
- [x] **SESS-07**: Bot tracks and displays token usage (input/output/cache) in /status
- [x] **SESS-08**: Bot properly kills Windows process trees (taskkill /T /F) when stopping sessions

### Multi-Project Management

- [x] **PROJ-01**: Each Telegram channel maps to exactly one project (working directory)
- [ ] **PROJ-02**: Each project has its own independent Claude CLI session running simultaneously
- [ ] **PROJ-03**: When bot receives a message from an unassigned channel, it prompts user to link a project
- [x] **PROJ-04**: Project-channel mappings persist to JSON file and survive restarts
- [x] **PROJ-05**: User can reassign or unlink a channel from a project

### Authentication & Security

- [x] **AUTH-01**: Bot authenticates users based on Telegram channel membership (per-channel auth)
- [x] **AUTH-02**: Bot validates file paths against allowed directories before Claude access
- [x] **AUTH-03**: Bot checks commands against blocked patterns for safety

### Session Lifecycle Commands

- [x] **CMD-01**: `/start` — shows bot info and current channel status
- [x] **CMD-02**: `/new` — creates a new Claude session for the current channel's project
- [x] **CMD-03**: `/stop` — aborts the currently running Claude query
- [x] **CMD-04**: `/status` — shows session state, token usage, context usage, project info
- [x] **CMD-05**: `/resume` — lists saved sessions with inline keyboard picker to restore one

### Session Persistence

- [x] **PERS-01**: Bot saves session state (session ID, working dir, conversation context) to JSON
- [x] **PERS-02**: Bot restores sessions automatically on restart for all mapped channels
- [x] **PERS-03**: Session state persists across bot crashes and service restarts

### GSD Integration

- [ ] **GSD-01**: `/gsd` command presents all GSD operations as categorized inline keyboard menus
- [x] **GSD-02**: Bot extracts GSD slash commands from Claude responses and renders as tappable buttons
- [x] **GSD-03**: Bot extracts numbered options from Claude responses and renders as tappable buttons
- [x] **GSD-04**: Bot displays roadmap phase progress inline when showing GSD status
- [ ] **GSD-05**: ask_user MCP integration — Claude can present clarifying questions via inline keyboard

### Media Handling

- [ ] **MEDIA-01**: User can send voice messages; bot transcribes via OpenAI Whisper and processes as text
- [ ] **MEDIA-02**: User can send photos; bot forwards to Claude for visual analysis
- [ ] **MEDIA-03**: Bot buffers photo albums (media groups) with a timeout before sending as a batch
- [ ] **MEDIA-04**: User can send PDF documents; bot extracts text via pdftotext and sends to Claude
- [ ] **MEDIA-05**: User can send text/code files as documents; bot reads content and sends to Claude

### Deployment

- [x] **DEPLOY-01**: Bot compiles to a single Go binary (.exe) for Windows
- [ ] **DEPLOY-02**: Bot installs as a Windows Service (runs at boot, no terminal window)
- [x] **DEPLOY-03**: Bot resolves external tool paths (claude, pdftotext) explicitly at startup, not via PATH lookup
- [x] **DEPLOY-04**: Bot supports graceful shutdown — drains active sessions before stopping

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Enhanced Media

- **MEDIA-06**: User can send video messages for transcription/analysis
- **MEDIA-07**: User can send audio files (mp3, m4a, wav) for transcription
- **MEDIA-08**: User can send archive files (zip, tar) for extraction and analysis

### Enhanced UX

- **UX-01**: /retry command to resend the last message
- **UX-02**: Adaptive edit throttle based on Telegram rate limit responses
- **UX-03**: MCP server configuration support via bot commands

## Out of Scope

| Feature | Reason |
|---------|--------|
| Native Telegram streaming API | Triggers 15% commission on in-bot purchases; edit-in-place is sufficient |
| SQLite/database storage | JSON files are sufficient; avoids schema migration complexity |
| Docker deployment | Windows Service is the target; Go produces a single binary |
| Webhook mode | Requires HTTPS/certs/port exposure; long polling works behind NAT |
| Global user allowlist | Per-channel auth replaces this; doesn't scale with multi-channel model |
| Shared Claude sessions | Destroys multi-project isolation guarantee |
| Multi-user session ownership | One session per channel, not per user; first message wins |
| Auto-commit/push from bot | High risk of pushing broken code; use Claude's built-in git tools |
| macOS LaunchAgent | Previous version had this; Go version targets Windows only |
| Conversation history search | Telegram already provides native channel search |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| CORE-01 | Phase 1 | Complete |
| CORE-02 | Phase 1 | Complete |
| CORE-03 | Phase 1 | Complete |
| CORE-04 | Phase 1 | Complete |
| CORE-05 | Phase 1 | Complete |
| CORE-06 | Phase 1 | Complete |
| SESS-01 | Phase 1 | Complete |
| SESS-02 | Phase 1 | Complete |
| SESS-03 | Phase 1 | Complete |
| SESS-04 | Phase 1 | Complete |
| SESS-05 | Phase 1 | Complete |
| SESS-06 | Phase 1 | Complete |
| SESS-07 | Phase 1 | Complete |
| SESS-08 | Phase 1 | Complete |
| PROJ-01 | Phase 2 | Complete |
| PROJ-02 | Phase 2 | Pending |
| PROJ-03 | Phase 2 | Pending |
| PROJ-04 | Phase 2 | Complete |
| PROJ-05 | Phase 2 | Complete |
| AUTH-01 | Phase 1 | Complete |
| AUTH-02 | Phase 1 | Complete |
| AUTH-03 | Phase 1 | Complete |
| CMD-01 | Phase 1 | Complete |
| CMD-02 | Phase 1 | Complete |
| CMD-03 | Phase 1 | Complete |
| CMD-04 | Phase 1 | Complete |
| CMD-05 | Phase 1 | Complete |
| PERS-01 | Phase 1 | Complete |
| PERS-02 | Phase 1 | Complete |
| PERS-03 | Phase 1 | Complete |
| GSD-01 | Phase 2 | Pending |
| GSD-02 | Phase 2 | Complete |
| GSD-03 | Phase 2 | Complete |
| GSD-04 | Phase 2 | Complete |
| GSD-05 | Phase 2 | Pending |
| MEDIA-01 | Phase 3 | Pending |
| MEDIA-02 | Phase 3 | Pending |
| MEDIA-03 | Phase 3 | Pending |
| MEDIA-04 | Phase 3 | Pending |
| MEDIA-05 | Phase 3 | Pending |
| DEPLOY-01 | Phase 1 | Complete |
| DEPLOY-02 | Phase 3 | Pending |
| DEPLOY-03 | Phase 1 | Complete |
| DEPLOY-04 | Phase 1 | Complete |

**Coverage:**
- v1 requirements: 44 total
- Mapped to phases: 44
- Unmapped: 0

---
*Requirements defined: 2026-03-19*
*Last updated: 2026-03-19 after roadmap creation*
