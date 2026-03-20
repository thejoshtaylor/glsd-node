# Phase 9: Channel Auth - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Fix channel-type authorization so the bot accepts messages from authorized Telegram channels and prevents echo loops from its own reflected channel posts.

</domain>

<decisions>
## Implementation Decisions

### Channel Authorization Model
- Authorize channels by admin lookup: call `GetChatAdministrators()` and check if any admin's user ID is in the `TELEGRAM_ALLOWED_USERS` list
- Cache admin lookup results with a 15-minute TTL to avoid repeated API calls
- The bot must be an admin in the channel for the lookup to work
- Anonymous admin senders in authorized channels pass auth (channel is authorized via admin lookup)
- Check lives in `security.IsAuthorized()` or a new channel-auth helper called from middleware

### Rejection Behavior in Channels
- Reply with rejection message in unauthorized channels (same behavior as DMs)
- Audit log all rejected channel messages with channel ID
- 15-minute cache TTL for admin lookup results

### Echo Loop Prevention
- Compare `msg.From.Id` to `bot.Id` to detect bot's own reflected messages
- Filter in auth middleware before auth check for cheapest early exit
- Debug-level log only for echo-filtered messages (not audit-worthy)
- Also filter `msg.IsAutomaticForward` to prevent double-processing of linked-channel forwards

### Claude's Discretion
- Internal implementation details of the admin cache (sync.Map, expiry struct, etc.)
- Exact placement of echo check within the middleware function
- Test structure and coverage approach

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/security/validate.go` — `IsAuthorized(userID, channelID, allowedUsers)` already accepts channelID param (unused)
- `internal/bot/middleware.go` — `authMiddlewareWith()` has `ctx.EffectiveSender` and `ctx.EffectiveChat` extraction
- `internal/bot/middleware.go` — `AuthChecker` interface for testable auth injection
- `internal/bot/middleware_test.go` — existing auth middleware tests

### Established Patterns
- Middleware uses gotgbot dispatcher groups (-2 for auth, -1 for rate limit, 0 for handlers)
- `ext.EndGroups` returned to stop processing on rejection
- `audit.NewEvent()` for structured audit logging
- Tests use mock `AuthChecker` interface

### Integration Points
- `middleware.go:52-54` — `EffectiveSender` extraction where channel sender is nil/channel-typed
- `validate.go:42-49` — `IsAuthorized()` function that needs channel auth logic
- `bot.go` — bot instance has access to `gotgbot.Bot.Id` for echo detection
- `handlers.go:28` — auth middleware registration point

</code_context>

<specifics>
## Specific Ideas

- User prefers admin lookup over channel ID allowlist for zero-config channel authorization
- Rejection messages should be visible in channels (not silent) — same UX as DMs

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
