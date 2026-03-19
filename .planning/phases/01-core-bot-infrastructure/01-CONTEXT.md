# Phase 1: Core Bot Infrastructure - Context

**Gathered:** 2026-03-19
**Status:** Ready for planning

<domain>
## Phase Boundary

A running Go bot that accepts text messages from one Telegram channel, routes them to a Claude CLI session, and streams the response back — with correct concurrency, persistence, auth, rate limiting, and audit logging. Single-channel only; multi-project support is Phase 2.

</domain>

<decisions>
## Implementation Decisions

### Streaming response display
- Keep emoji tool status indicators (e.g. Search, Edit, Write) — show tool name with emoji while executing, replace with response when done
- Streaming edits throttled at 500ms minimum interval — fast enough to feel live, slow enough to avoid Telegram rate limits
- Message splitting at paragraph boundaries (last double-newline before 4096 char limit) — keeps code blocks and paragraphs intact
- Show "Thinking..." message with Telegram typing action while Claude processes before text appears; message gets replaced by actual response
- Use **MarkdownV2** for Telegram message formatting (deliberate change from TypeScript version which used HTML with plain text fallback)

### Command output & session UX
- `/status` shows full dashboard: session state + token usage (input/output/cache) + context percentage + current project path + uptime
- Context window usage displayed as **percentage only** (e.g. "Context: 42%") — no progress bar
- `/resume` presents saved sessions as inline keyboard buttons showing timestamp + first message preview — one-tap restore
- Retain 5 sessions per project for `/resume` history
- `/start` shows brief welcome + status: bot name, version, current project path (if linked), and available commands

### Error & state messaging
- Context limit (hard "prompt too long" errors after auto-compaction fails): auto-clear session + notify user with recovery hint — "Session hit hard context limit and was cleared. Use /resume to restore a previous session."
- Rate limit rejections: terse with retry time — "Rate limited. Try again in 12s."
- Unauthorized access: rejection with reason — "You're not authorized for this channel. Contact the bot admin."
- Claude CLI errors: truncated stderr — "Claude error: [first 200 chars of stderr]" — enough to diagnose without flooding chat

### Claude's Discretion
- Exact MarkdownV2 escaping strategy and fallback behavior if parsing fails
- Typing indicator update interval
- Exact emoji-to-tool mapping (can follow TypeScript patterns or improve)
- Audit log format details (structured JSON vs plain text)
- Session state machine internal design
- Go package layout and internal organization

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing TypeScript implementation (functional spec)
- `src/session.ts` — Claude CLI subprocess management, NDJSON streaming, session persistence, context limit detection patterns
- `src/handlers/streaming.ts` — StreamingState callback factory, message throttling, Telegram message splitting
- `src/handlers/commands.ts` — /start, /new, /stop, /status, /resume command implementations
- `src/handlers/text.ts` — Text message handler with auth check, rate limit, interrupt support
- `src/security.ts` — RateLimiter (token bucket), path validation, command safety checks
- `src/formatting.ts` — Markdown-to-HTML conversion (reference for MarkdownV2 equivalent), tool status emoji formatting
- `src/config.ts` — Environment parsing, MCP loading, safety prompts
- `src/types.ts` — Shared TypeScript types (map to Go structs)
- `src/utils.ts` — Audit logging, typing indicators

### Research & architecture
- `.planning/research/SUMMARY.md` — Full research summary with stack decisions, pitfalls, architecture approach
- `.planning/research/PITFALLS.md` — Eight critical pitfalls; six must be addressed in Phase 1
- `.planning/research/ARCHITECTURE.md` — Recommended Go architecture layers
- `.planning/research/STACK.md` — Stack decisions with rationale
- `.planning/codebase/ARCHITECTURE.md` — Existing TypeScript architecture analysis
- `.planning/codebase/CONVENTIONS.md` — Existing coding conventions (for behavioral reference, not style copying)

### Project requirements
- `.planning/REQUIREMENTS.md` — Phase 1 requirements: CORE-01 through CORE-06, SESS-01 through SESS-08, AUTH-01 through AUTH-03, CMD-01 through CMD-05, PERS-01 through PERS-03, DEPLOY-01, DEPLOY-03, DEPLOY-04
- `.planning/ROADMAP.md` — Phase 1 success criteria (5 items)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `src/session.ts` ClaudeSession class: functional spec for Go session manager — NDJSON event types, streaming protocol, session persistence format
- `src/handlers/streaming.ts` StreamingState: functional spec for streaming callback — throttle logic, message accumulation, split behavior
- `src/security.ts` RateLimiter: token bucket algorithm reference — can translate directly to `golang.org/x/time/rate`
- `src/formatting.ts` tool emoji map: complete mapping of tool names to emoji indicators

### Established Patterns
- NDJSON streaming from Claude CLI stdout with event types: `assistant`, `result`, `thinking`, `tool_use`
- Edit-in-place message updates with throttling to avoid Telegram rate limits
- Session persistence as JSON with multi-session history (max 5 per working dir)
- Interrupt support via `!` prefix on messages
- Context limit detection via stderr pattern matching

### Integration Points
- Claude CLI subprocess: spawned with `--output-format stream-json` and system prompt flags
- Telegram Bot API: long polling via gotgbot/v2 updater
- JSON files for state: session history, working directory state
- External tool resolution: `claude` and `pdftotext` paths resolved at startup

</code_context>

<specifics>
## Specific Ideas

- MarkdownV2 is a deliberate change from the TypeScript version's HTML approach — user preference for native Telegram formatting
- Context percentage as plain number, not a visual bar — keep status output clean and scannable
- Auth rejection should include reason ("not authorized for this channel") rather than a bare "Unauthorized" — friendlier for legitimate users misconfigured

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-core-bot-infrastructure*
*Context gathered: 2026-03-19*
