# Phase 2: Multi-Project and GSD Integration - Context

**Gathered:** 2026-03-19
**Status:** Ready for planning

<domain>
## Phase Boundary

Multiple Telegram channels each route to independent Claude sessions with no context bleed, and the GSD workflow is fully accessible via inline keyboard menus. Each channel maps to exactly one project directory. Users can link, reassign, and unlink channels from projects. Claude responses containing GSD commands or numbered/lettered options render as tappable inline keyboard buttons.

</domain>

<decisions>
## Implementation Decisions

### Channel-project mapping
- Free-text path entry: when bot receives a message from an unregistered channel, it asks the user to type/paste a directory path, validates it's under ALLOWED_PATHS before accepting
- Mappings stored in a single JSON file (`mappings.json` in DataDir) with `{channelID: {path, name, linkedAt}}` entries
- `/project` command for reassignment: shows current mapping + offers 'Change' button; typing `/project <path>` directly reassigns
- Lazy session start: linking saves the mapping only; Claude session starts on first actual message (no eager worker spawn)

### GSD keyboard menu
- Same 8x2 grid layout as TypeScript version, but with a quick-actions row at the top featuring "Next" and "Progress" buttons
- Full operation list: Next, Progress, Quick Task, Plan Phase, Execute Phase, Discuss Phase, Research Phase, Verify Work, Audit Milestone, Pause Work, Resume Work, Check Todos, Add Todo, Add Phase, Remove Phase, New Project, New Milestone, Settings, Debug, Help
- Phase picker via inline keyboard: bot reads roadmap, shows available phases as buttons with status indicators (checkmark for complete, hourglass for in-progress)
- Status header above buttons: current phase name, progress (e.g., "3/8 plans complete"), project path
- Direct `/gsd:command-name` routing supported: power users can type e.g. `/gsd:execute-phase 2` to skip the keyboard

### Response button extraction
- Extract `/gsd:` commands from Claude responses and render as two side-by-side buttons per command: "Run" (current session) and "Fresh" (clear session first, then run)
- Extract both numbered options (1. 2. 3.) AND lettered options (A. B. C. or a) b) c)) as tappable inline keyboard buttons; tapping sends the number or letter back to Claude
- No special /clear suggestion handling — user uses /new manually
- Buttons appear only when GSD commands or numbered/lettered options are detected; regular conversational responses stay clean with no keyboard

### Multi-session isolation
- Per-project ALLOWED_PATHS: each project's allowed paths default to [projectDir] only; Claude in channel A can only access project A's directory
- Per-project safety prompt: built from per-project allowed paths (not global)
- Global API rate limiter (~25 edits/sec across all channels) on top of existing per-channel rate limiter to handle simultaneous streaming sessions
- Per-project session persistence: each project keeps its own session history (5 max per project); /resume shows only that project's sessions
- Lazy restore on restart: load mappings at startup but only create Session objects and start workers when a channel sends its first message

### Claude's Discretion
- Exact regex patterns for extracting GSD commands and numbered/lettered options from Claude responses
- Global API rate limiter implementation details (token bucket vs sliding window, exact limit)
- Internal structure of the mappings.json file (exact field names, metadata)
- How to parse roadmap progress for the GSD status header
- Phase picker button layout (single column vs grid)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing TypeScript implementation (functional spec for GSD and multi-project)
- `src/handlers/commands.ts` — GSD_OPERATIONS table (16 operations), handleGsd command, sendGsdCommand, phase picker flow, roadmap parsing
- `src/handlers/callback.ts` — GSD callback routing (gsd:, gsd-run:, gsd-fresh:, gsd-{op}:{phase}), numbered option callbacks, phase picker callbacks, new project flow
- `src/formatting.ts` — extractGsdCommands(), extractNumberedOptions(), buildActionKeyboard(), GsdCommandSuggestion type
- `src/registry.ts` — Project registry parser (markdown table), addProject(), path validation
- `src/handlers/text.ts` — Response button extraction integration (extractGsdCommands + extractNumberedOptions after streaming)

### Existing Go code (Phase 1 foundation)
- `internal/session/store.go` — SessionStore keyed by int64 channelID, GetOrCreate pattern
- `internal/session/session.go` — Session with immutable workingDir, Worker goroutine, WorkerConfig with AllowedPaths/SafetyPrompt
- `internal/handlers/callback.go` — parseCallbackData routing (resume:, action:*), HandleCallback
- `internal/handlers/command.go` — HandleStart/New/Stop/Status/Resume, buildStatusText
- `internal/bot/bot.go` — Bot struct, restoreSessions, session worker lifecycle
- `internal/bot/handlers.go` — registerHandlers, handler wiring pattern
- `internal/config/config.go` — Config struct (single WorkingDir), buildSafetyPrompt, FilteredEnv

### Project requirements
- `.planning/REQUIREMENTS.md` — Phase 2 requirements: PROJ-01 through PROJ-05, GSD-01 through GSD-05
- `.planning/ROADMAP.md` — Phase 2 success criteria (5 items)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `SessionStore` (store.go): Already keyed by channelID with thread-safe Get/GetOrCreate/Remove/All — ready for multi-project, just need per-channel working dirs
- `parseCallbackData` (callback.go): Pure function routing pattern — extend with gsd:, gsd-run:, gsd-fresh:, gsd-{op}:{phase}, option: prefixes
- `WorkerConfig` (session.go): Already carries AllowedPaths and SafetyPrompt per-worker — ready for per-project isolation
- `buildSafetyPrompt` (config.go): Takes []string paths and builds prompt — can be called per-project
- `PersistenceManager` (persist.go): JSON file read/write with max history — can be instantiated per-project or extended

### Established Patterns
- Handler registration via bot wrapper methods that delegate to `handlers` package functions
- Callback data parsing as pure function for testability
- Session worker goroutine started by bot layer, not by store
- Config loaded once at startup, passed through to handlers

### Integration Points
- `Config.WorkingDir` (single string) needs to become per-channel via mappings lookup
- `bot.restoreSessions` needs to read mappings.json instead of just session history
- `registerHandlers` needs new /gsd and /project command registrations
- Streaming callback (`StatusCallbackFactory`) needs to call button extraction after response completes
- `HandleText` needs to check mappings before routing to session, prompt for linking if unmapped

</code_context>

<specifics>
## Specific Ideas

- Quick-actions row at the top of GSD keyboard: "Next" button is the most important workflow-advancing shortcut
- Letter option extraction (A. B. C.) in addition to numbered options — Claude often uses lettered alternatives
- Per-project isolation is the strongest model: each channel is a sandbox with its own paths, safety prompt, and session history

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-multi-project-and-gsd-integration*
*Context gathered: 2026-03-19*
