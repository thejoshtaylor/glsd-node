# Feature Research

**Domain:** Telegram bot wrapping AI CLI tools (Claude Code) with multi-project management
**Researched:** 2026-03-19
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Text message routing to Claude | Core interaction — without this the bot is useless | LOW | Synchronous text-in, response-out |
| Streaming response with live updates | Any AI bot doing >2s responses needs live feedback; users abandon silent bots | MEDIUM | Edit-in-place pattern: create message, throttle edits at ~500ms, final edit on complete |
| Session persistence across restarts | Users expect conversation continuity; losing context on restart is a regression | MEDIUM | JSON file with session ID; resume on startup |
| /start, /new, /stop, /status commands | Standard Telegram bot conventions; users try these first | LOW | `/start` = info + status; `/new` = clear session; `/stop` = abort running query |
| Auth gate on all handlers | Security baseline — unauthorized users must be rejected, not silently ignored | LOW | Per-channel membership check in the Go version; check on every handler |
| Error reporting back to user | Silent failures erode trust; users need to know when something went wrong | LOW | Error text in channel, truncated to 200 chars |
| Typing indicator while processing | Standard UX signal that the bot is working | LOW | Send chat action "typing" on a goroutine; stop on response complete |
| Rate limiting per channel | Prevents accidental or intentional abuse of Claude API | MEDIUM | Token bucket per channel ID; configurable burst and refill |
| Audit logging | Required for debugging; expected in any production tool | LOW | Append-only log: timestamp, user, action, first 100 chars of message |
| /resume — restore previous session | Claude context is expensive; losing work context is painful | MEDIUM | List saved sessions with inline keyboard; select to resume |

### Differentiators (Competitive Advantage)

Features that set the product apart. Not required, but valuable.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Multi-project with one channel per project | Core value proposition of this rewrite — competitors use a single chat or `/repo` switching | HIGH | One bot, N channels, each channel maintains independent Claude session; channel ID is the project key |
| Dynamic project-channel assignment | Unrecognized channel prompts user to link a project path — zero-config onboarding for new projects | HIGH | On first message from unknown channel: show directory browser inline keyboard; save mapping to JSON |
| GSD workflow integration via inline keyboard menus | The entire GSD command set accessible from a phone without typing slash commands | HIGH | 19 GSD operations mapped to inline keyboard; roadmap context shown with phase progress; contextual next-step buttons extracted from Claude's response |
| Contextual action buttons after responses | Claude's response may contain GSD commands or numbered options — extract them and render as tappable buttons | HIGH | Parse response for `/gsd:*` patterns and numbered lists; buildActionKeyboard() renders them as inline keyboard |
| Context window progress bar | Users can see how close they are to hitting the context limit before it fails | LOW | Parse `context_percent` from Claude CLI stream-json output; render as block bar |
| Query interrupt with `!` prefix | Interrupting a long-running query to redirect is essential for phone-based coding | MEDIUM | Detect `!` prefix on text handler; stop running process, clear stop flag, send new message |
| Voice message transcription | Mobile-first: typing long prompts is painful; voice is faster | MEDIUM | Download OGG from Telegram; send to OpenAI Whisper API; pipe transcript to text handler |
| Photo analysis with album buffering | Take a screenshot of an error and send it for analysis | MEDIUM | Buffer media group messages for 1s timeout to collect album; pass image URLs to Claude |
| PDF document processing | Send spec docs, PRDs, or error reports directly to the bot | MEDIUM | `pdftotext` CLI dependency; extract text, pass as message content |
| Per-project independent Claude sessions | Work on two projects simultaneously without context bleed | HIGH | One `ClaudeSession` (or equivalent) per channel, goroutine-safe |
| Windows Service deployment via NSSM | Runs at boot on Windows dev machines without a terminal window; proper service lifecycle | MEDIUM | NSSM wraps the Go binary; install script sets recovery actions; logs to file |
| Roadmap parsing and display in /gsd | Shows phase progress inline — Phase X/N done, next phase name — without leaving Telegram | MEDIUM | Parse `.planning/ROADMAP.md` checkboxes; display as context in /gsd message |
| ask_user MCP integration | Claude can ask clarifying questions via inline keyboard buttons during a session | HIGH | Watch for ask-user JSON files in temp dir; render options as inline keyboard; write answer back to file |
| Token usage reporting in /status | Shows input/output/cache tokens from last query — useful for cost awareness | LOW | Parse `usage` from stream-json; store on session object; display in /status |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Native Telegram streaming (Bot API) | Would give real live character-by-character streaming | Telegram charges 15% commission on all in-bot Star purchases when enabled; creates revenue dependency on Telegram; only useful for monetized bots | Edit-in-place pattern: throttled message edits simulate streaming without enabling native streaming |
| Database (SQLite/Postgres) for state | Feels more "production" | Adds dependency, schema migrations, backup complexity; JSON files are sufficient for single-user or small team use — and explicitly out of scope per PROJECT.md | JSON file persistence: project-channel mappings, session state, audit log |
| Shared Claude sessions across channels | Reduces Claude session overhead | Destroys the multi-project isolation guarantee — context from Project A leaks into Project B | Strict one-session-per-channel-ID mapping |
| Global user allowlist | Familiar pattern from single-channel bots | Doesn't scale with multi-channel model; who is "allowed" changes per project/channel | Per-channel auth: if you're in the channel, you have access; no global list to maintain |
| Webhook mode for Telegram updates | Lower latency than polling; common recommendation | Adds HTTPS endpoint requirement, certificate management, port exposure on Windows; for a local dev tool, long polling is perfectly adequate | Long polling: simpler, no network config, works behind NAT and firewalls |
| Docker deployment | Reproducible environment | Explicit out-of-scope in PROJECT.md; adds complexity for a Windows Service target; Go produces a single static binary anyway | Native Go binary + NSSM; binary compiles to a single `.exe` |
| Multi-user per channel | Multiple developers sharing one channel | Blurs accountability in audit log; creates conflicting session state (one user stops a query while another is waiting for results) | Per-channel auth accepts any channel member, but sessions are owned by the channel, not the user — first message wins |
| Inline message editing for streaming | Looks slicker — message updates in-place | Telegram rate-limits edits to ~2/s per chat; rapid edits trigger 429 errors and back-off; need throttle logic regardless | Throttle edits at 500ms minimum interval; batch content updates |
| Auto-commit or git push from bot | Convenience for one-command deploys | High risk of pushing broken code or exposing credentials; creates feedback loop without review | Use Claude's existing git tools inside the session; require explicit intent per commit |
| Conversation history search | Useful reference for past decisions | Telegram already provides message search in channels; duplicating it adds storage and complexity | Rely on Telegram's native channel search; audit log covers structured event history |

## Feature Dependencies

```
[Per-channel auth]
    └──required by──> [Multi-project channel mapping]
                          └──required by──> [Independent Claude sessions per channel]
                                                └──required by──> [GSD workflow integration]

[Streaming response with live updates]
    └──required by──> [Contextual action buttons after responses]
                          └──enhances──> [GSD workflow integration]

[Session persistence]
    └──required by──> [/resume command]

[Voice transcription]
    └──depends on──> [OpenAI Whisper API key config]

[PDF processing]
    └──depends on──> [pdftotext CLI on PATH]

[ask_user MCP integration]
    └──depends on──> [MCP server configuration]
    └──depends on──> [Streaming response callback system]

[Roadmap parsing in /gsd]
    └──depends on──> [Multi-project channel mapping] (need to know working dir)

[Contextual action buttons]
    └──depends on──> [Response text parsing (GSD command extraction)]
    └──conflicts──> [Native Telegram streaming] (streaming API owns the message lifecycle)
```

### Dependency Notes

- **Multi-project mapping requires per-channel auth**: Channel ID is both the auth unit and the project key. You cannot have one without the other in this architecture.
- **Independent sessions require multi-project mapping**: The channel-to-session lookup is the mechanism that provides isolation.
- **GSD integration requires independent sessions**: GSD commands run in the context of a specific project; without per-project sessions, GSD loses project context.
- **Contextual buttons conflict with native Telegram streaming**: When native streaming is enabled, Telegram controls the message lifecycle and edit-in-place behavior cannot be combined cleanly. The throttled-edit approach is the correct path.
- **Voice and PDF require external CLI/API**: These are optional at launch if the external dependency is unavailable; the text path remains fully functional.

## MVP Definition

### Launch With (v1)

Minimum viable product — what's needed to validate the concept.

- [ ] Text message handler with streaming to Claude CLI — core interaction loop
- [ ] Multi-project channel mapping with dynamic assignment — core value proposition
- [ ] Independent Claude sessions per channel — isolation guarantee
- [ ] /new, /stop, /status, /start commands — session lifecycle management
- [ ] Per-channel auth (channel membership = access) — security baseline
- [ ] Rate limiting per channel — abuse prevention
- [ ] Audit logging — debugging capability
- [ ] Session persistence and /resume — continuity across restarts
- [ ] Windows Service via NSSM — deployment target
- [ ] GSD inline keyboard with all 19 operations — key differentiator
- [ ] Contextual action buttons extracted from responses — enables phone-based GSD workflow

### Add After Validation (v1.x)

Features to add once core is working.

- [ ] Voice transcription via OpenAI Whisper — add when users report typing friction on mobile
- [ ] Photo analysis with media group buffering — add when users send screenshots
- [ ] PDF document processing — add when users need to share spec documents
- [ ] Context window progress bar — add when users hit context limit unexpectedly
- [ ] Roadmap parsing in /gsd — add once GSD integration is confirmed stable
- [ ] Token usage in /status — add for cost-awareness requests
- [ ] ask_user MCP integration — add when MCP server configuration is validated

### Future Consideration (v2+)

Features to defer until product-market fit is established.

- [ ] Video/audio file transcription — niche use case; voice messages cover most mobile audio
- [ ] Archive extraction from documents — low frequency, adds complexity
- [ ] Retry last message (/retry) — nice-to-have; easy to add but not essential
- [ ] Vault/knowledge base search integration — specific to Obsidian workflows; not generalizable

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Text message + streaming | HIGH | MEDIUM | P1 |
| Multi-project channel mapping | HIGH | HIGH | P1 |
| Independent Claude sessions per channel | HIGH | HIGH | P1 |
| Session lifecycle commands | HIGH | LOW | P1 |
| Per-channel auth | HIGH | LOW | P1 |
| GSD inline keyboard | HIGH | MEDIUM | P1 |
| Contextual action buttons | HIGH | MEDIUM | P1 |
| Session persistence + /resume | HIGH | MEDIUM | P1 |
| Rate limiting | MEDIUM | LOW | P1 |
| Audit logging | MEDIUM | LOW | P1 |
| Windows Service (NSSM) | HIGH | LOW | P1 |
| Voice transcription | HIGH | MEDIUM | P2 |
| Photo analysis | MEDIUM | MEDIUM | P2 |
| PDF processing | MEDIUM | MEDIUM | P2 |
| Context window progress bar | MEDIUM | LOW | P2 |
| Roadmap parsing in /gsd | MEDIUM | LOW | P2 |
| Token usage display | LOW | LOW | P2 |
| ask_user MCP | MEDIUM | HIGH | P2 |
| Video transcription | LOW | MEDIUM | P3 |
| Archive extraction | LOW | HIGH | P3 |
| /retry command | LOW | LOW | P3 |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | ccgram (Go/tmux bridge) | claude-code-telegram (Python) | droid-telegram-bot (Node) | Our Approach |
|---------|-------------------------|-------------------------------|---------------------------|--------------|
| Multi-project | Topic-per-tmux-window (forum topics) | `/repo` command to switch directories | Single working dir per session | One Telegram channel per project — cleanest separation |
| Session management | Auto-sync with tmux windows | Per user/project auto-persistence | Reply-to-message resumes session | Per-channel, persisted to JSON, /resume picker |
| Streaming | Live status line, MarkdownV2 | Typing indicator + verbose levels | Streaming toggle per session | Throttled edit-in-place with tool status message |
| Voice | Whisper via Groq or OpenAI | Via Mistral or OpenAI | Not mentioned | OpenAI Whisper API |
| Auth | No detail provided | User whitelist | User allowlist config | Per-channel membership (no global list) |
| GSD integration | None | None | None | Full 19-command inline keyboard + contextual buttons |
| Deployment | Systemd (Linux) | Not specified | Systemd (Linux) | NSSM Windows Service |
| ask_user | Inline keyboard for prompts | Not mentioned | Once/Always/Deny permission buttons | ask_user JSON file polling, inline keyboard response |

## Sources

- [alexei-led/ccgram — Telegram tmux bridge for Claude Code](https://github.com/alexei-led/ccgram): Topic-based multi-session architecture, voice transcription, live status, interactive prompt keyboards
- [RichardAtCT/claude-code-telegram](https://github.com/RichardAtCT/claude-code-telegram): Conversational + terminal modes, webhook events, git integration, verbose streaming levels
- [factory-ben/droid-telegram-bot](https://github.com/factory-ben/droid-telegram-bot): Reply-based session continuity, permission prompts, autonomy levels, streaming toggle
- [Streaming responses in Telegram bots comes at a cost](https://durovscode.com/streaming-responses-telegram-bots): Native streaming 15% commission model — reason to use edit-in-place instead
- [GSD Get Shit Done framework](https://github.com/gsd-build/get-shit-done): Full command list and workflow structure used for GSD integration spec
- [How to handle streaming responses without markdown parsing errors — Latenode Community](https://community.latenode.com/t/how-to-handle-streaming-responses-from-google-ai-in-telegram-bot-without-markdown-parsing-errors/21646): Edit-in-place pattern for streaming, HTML vs MarkdownV2 tradeoffs
- Existing TypeScript codebase (`src/`) — functional specification for feature parity

---
*Feature research for: Go Telegram Bot with multi-project Claude Code integration*
*Researched: 2026-03-19*
