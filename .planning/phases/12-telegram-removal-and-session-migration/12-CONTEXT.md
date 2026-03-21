# Phase 12: Telegram Removal and Session Migration - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning

<domain>
## Phase Boundary

Remove all Telegram and TypeScript dependencies from the codebase, and migrate session persistence from channel-ID-keyed to project-name-keyed (instance UUID). This cleans the codebase so the dispatch layer (Phase 13) builds against clean identity types with no legacy coupling.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Key areas:
- Order of removal (TypeScript first vs Go Telegram imports first)
- How to handle existing `internal/bot/`, `internal/handlers/` packages that depend on gotgbot
- Migration script approach (standalone Go binary vs embedded function)
- What to preserve vs delete in session persistence
- How to restructure `internal/session/` for UUID-keyed instances

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/session/session.go` — existing ClaudeSession management (needs key migration)
- `internal/session/persist.go` — JSON persistence (needs key format change)
- `internal/session/store.go` — session store (needs rekey from ChatID to InstanceID)
- `internal/config/config.go` — existing config with Telegram-specific vars to remove

### Established Patterns
- Go 1.24, module `github.com/user/gsd-tele-go`
- Logging: zerolog structured logging
- Config: godotenv for .env loading
- Session persistence: JSON file at configurable path
- gotgbot/v2 is the primary Telegram dependency in go.mod

### Integration Points
- `internal/bot/` — entire package is Telegram-specific (bot.go, handlers.go, middleware.go)
- `internal/handlers/` — all handlers are Telegram message type handlers
- `go.mod` — gotgbot/v2 and openai-go dependencies to remove
- `src/` — TypeScript source files to delete
- `package.json`, `tsconfig.json`, `bun.lockb` etc — npm/Bun artifacts to delete
- `internal/session/` — ChatID int64 → InstanceID string migration

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase

</specifics>

<deferred>
## Deferred Ideas

None

</deferred>
