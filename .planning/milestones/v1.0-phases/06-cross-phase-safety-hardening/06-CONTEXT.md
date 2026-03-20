# Phase 6: Cross-Phase Safety Hardening - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Ensure typing indicators, audit logging, and command safety checks apply uniformly to all message paths — callback handlers, GSD buttons, resume, and new session flows — not just the text handler.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. The three safety mechanisms (typing indicators, audit logging, command safety checks) already exist and work in the text handler path; this phase wires them into the callback/GSD paths that currently lack them.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `StartTypingIndicator(bot, chatID)` in `internal/handlers/streaming.go` — returns `*TypingController` with `.Stop()`
- `audit.Logger` initialized in `internal/bot/bot.go` and passed through middleware
- `security.CheckCommandSafety(text, patterns)` in `internal/security/validate.go`

### Established Patterns
- Text handler (`text.go:135`) calls `CheckCommandSafety` before sending to Claude
- Text handler (`text.go:193`) starts typing indicator before Claude call
- Document, photo, voice handlers all use `StartTypingIndicator` already
- Auth middleware logs denied access via audit logger

### Integration Points
- `internal/handlers/callback.go` — callback handler lacks typing, audit, and safety checks
- Callback handler spawns Claude sessions for GSD buttons, resume, new commands
- `internal/bot/bot.go` — audit logger available on Bot struct

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>
