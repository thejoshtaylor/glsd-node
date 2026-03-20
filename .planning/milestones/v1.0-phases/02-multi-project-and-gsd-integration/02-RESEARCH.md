# Phase 2: Multi-Project and GSD Integration - Research

**Researched:** 2026-03-19
**Domain:** Go multi-session state management, Telegram inline keyboard UX, regex-based response parsing
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Channel-project mapping**
- Free-text path entry: when bot receives a message from an unregistered channel, it asks the user to type/paste a directory path, validates it's under ALLOWED_PATHS before accepting
- Mappings stored in a single JSON file (`mappings.json` in DataDir) with `{channelID: {path, name, linkedAt}}` entries
- `/project` command for reassignment: shows current mapping + offers 'Change' button; typing `/project <path>` directly reassigns
- Lazy session start: linking saves the mapping only; Claude session starts on first actual message (no eager worker spawn)

**GSD keyboard menu**
- Same 8x2 grid layout as TypeScript version, but with a quick-actions row at the top featuring "Next" and "Progress" buttons
- Full operation list: Next, Progress, Quick Task, Plan Phase, Execute Phase, Discuss Phase, Research Phase, Verify Work, Audit Milestone, Pause Work, Resume Work, Check Todos, Add Todo, Add Phase, Remove Phase, New Project, New Milestone, Settings, Debug, Help
- Phase picker via inline keyboard: bot reads roadmap, shows available phases as buttons with status indicators (checkmark for complete, hourglass for in-progress)
- Status header above buttons: current phase name, progress (e.g., "3/8 plans complete"), project path
- Direct `/gsd:command-name` routing supported: power users can type e.g. `/gsd:execute-phase 2` to skip the keyboard

**Response button extraction**
- Extract `/gsd:` commands from Claude responses and render as two side-by-side buttons per command: "Run" (current session) and "Fresh" (clear session first, then run)
- Extract both numbered options (1. 2. 3.) AND lettered options (A. B. C. or a) b) c)) as tappable inline keyboard buttons; tapping sends the number or letter back to Claude
- No special /clear suggestion handling — user uses /new manually
- Buttons appear only when GSD commands or numbered/lettered options are detected; regular conversational responses stay clean with no keyboard

**Multi-session isolation**
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

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| PROJ-01 | Each Telegram channel maps to exactly one project (working directory) | MappingStore struct with channelID key; SessionStore.GetOrCreate already accepts workingDir per-channel |
| PROJ-02 | Each project has its own independent Claude CLI session running simultaneously | WorkerConfig.AllowedPaths and SafetyPrompt already per-worker; buildSafetyPrompt([]string) callable per-project |
| PROJ-03 | When bot receives a message from an unassigned channel, it prompts user to link a project | HandleText intercept pattern: check mappings before GetOrCreate; send link prompt if unmapped |
| PROJ-04 | Project-channel mappings persist to JSON file and survive restarts | PersistenceManager atomic write-rename pattern reusable; mappings.json in DataDir, same directory as session-history.json |
| PROJ-05 | User can reassign or unlink a channel from a project | /project command + callback; MappingStore.Set and MappingStore.Remove methods |
| GSD-01 | /gsd command presents all GSD operations as categorized inline keyboard menus | 20-operation table ported from TypeScript GSD_OPERATIONS; parseRoadmap for status header |
| GSD-02 | Bot extracts GSD slash commands from Claude responses and renders as tappable buttons | extractGsdCommands regex from TypeScript: `/gsd:([a-z-]+)(?:\s+([\d.]+))?`; two buttons per command: Run + Fresh |
| GSD-03 | Bot extracts numbered options from Claude responses and renders as tappable buttons | extractNumberedOptions from TypeScript; extend with lettered options (A. B. C. / a) b) c)) |
| GSD-04 | Bot displays roadmap phase progress inline when showing GSD status | parseRoadmap reads `.planning/ROADMAP.md`; regex `^- \[(.)\] \*\*Phase ([\d.]+): ([^*]+)\*\* - (.+)$` |
| GSD-05 | ask_user MCP integration — Claude can present clarifying questions via inline keyboard | askuser: callback prefix; temp file handshake in os.TempDir(); already exists in TypeScript callback.ts |
</phase_requirements>

## Summary

Phase 2 adds two orthogonal feature sets on top of the Phase 1 Go foundation: (1) multi-project channel isolation via a new MappingStore, and (2) the GSD workflow accessible through inline keyboards. The Phase 1 code was deliberately designed to make Phase 2 straightforward — `SessionStore` is already keyed by int64 channelID, `WorkerConfig` already carries per-worker `AllowedPaths` and `SafetyPrompt`, and `parseCallbackData` is a pure function easy to extend with new prefixes.

The canonical functional spec lives in the TypeScript source (`src/handlers/commands.ts`, `src/handlers/callback.ts`, `src/formatting.ts`). Every GSD keyboard operation, regex pattern, callback routing rule, and phase picker flow is already implemented and battle-tested there. The Go port is a translation exercise, not a design exercise — extract the algorithms and data structures, reimplement in idiomatic Go.

The highest-risk areas are: (a) the global API rate limiter needed to prevent Telegram's 30-edits/minute limit from triggering flood errors when multiple channels stream simultaneously, and (b) the `HandleText` intercept that must check the mapping store before routing to a session, with proper state machine for the "awaiting path" UX flow.

**Primary recommendation:** Port the TypeScript GSD logic directly into Go — the patterns are proven; the task is faithful translation, not redesign.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/PaulSonOfLars/gotgbot/v2 | v2.0.0-rc.34 (already in go.mod) | Telegram Bot API client, inline keyboards, callback routing | Already in use; Phase 1 foundation |
| encoding/json | stdlib | mappings.json read/write | Same pattern as session-history.json PersistenceManager |
| regexp | stdlib | GSD command extraction, numbered/lettered option detection | Zero-dependency; patterns are compile-once |
| sync | stdlib | MappingStore mutex, global rate limiter mutex | Already used throughout codebase |
| golang.org/x/time/rate | v0.8.0 (already in go.mod) | Global API rate limiter (token bucket) | Already used for ChannelRateLimiter |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| os | stdlib | TempDir for ask_user MCP request files | GSD-05 askuser handshake |
| path/filepath | stdlib | Cross-platform path joining for DataDir/mappings.json | All file operations |
| bufio | stdlib | Roadmap ROADMAP.md line-by-line parsing | parseRoadmap implementation |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| regexp (stdlib) | github.com/dlclark/regexp2 | regexp2 supports lookbehind but is overkill; stdlib regex handles all required patterns |
| Per-worker safety prompt build | Cache safety prompts per path set | Caching saves microseconds but adds complexity; prompts are short strings, rebuild is fine |

**Installation:**
No new dependencies needed. All required packages are already in go.mod.

## Architecture Patterns

### New Package: `internal/project`

```
internal/
├── project/
│   ├── mapping.go       # MappingStore: channelID → ProjectMapping (JSON persistence)
│   └── mapping_test.go
├── handlers/
│   ├── callback.go      # Extended: gsd:, gsd-run:, gsd-fresh:, gsd-{op}:{phase}, option:, letter:, askuser:
│   ├── command.go       # Extended: HandleGsd, HandleProject, HandleNext
│   ├── gsd.go           # NEW: GSD_OPERATIONS table, parseRoadmap, buildGsdKeyboard, extractGsdCommands, extractOptions
│   ├── gsd_test.go
│   └── text.go          # Extended: mapping check, awaiting-path state, per-project WorkerConfig
├── bot/
│   ├── bot.go           # Extended: MappingStore field, lazy restore from mappings.json
│   └── handlers.go      # Extended: register /gsd, /project commands
```

### Pattern 1: MappingStore — Channel-to-Project Mapping
**What:** Thread-safe map from int64 channelID to ProjectMapping struct, backed by mappings.json in DataDir.
**When to use:** Every handler that needs a channel's working directory calls `mappings.Get(chatID)` instead of `cfg.WorkingDir`.

```go
// Source: canonical reference — port from TypeScript CONTEXT.md decisions
type ProjectMapping struct {
    Path     string `json:"path"`
    Name     string `json:"name"`
    LinkedAt string `json:"linked_at"` // RFC3339
}

type MappingStore struct {
    mu       sync.RWMutex
    mappings map[int64]ProjectMapping
    filePath string
}

func (ms *MappingStore) Get(channelID int64) (ProjectMapping, bool)
func (ms *MappingStore) Set(channelID int64, m ProjectMapping) error  // writes to disk
func (ms *MappingStore) Remove(channelID int64) error                  // writes to disk
func (ms *MappingStore) All() map[int64]ProjectMapping                 // shallow copy
func (ms *MappingStore) Load() error                                   // reads mappings.json at startup
```

**Persistence:** Same atomic write-rename pattern as PersistenceManager — marshal to temp file, `os.Rename` to mappings.json. Top-level JSON structure: `{"mappings": {"123456789": {"path": "...", "name": "...", "linked_at": "..."}}}`. Keys are string representations of int64 channelIDs (JSON object keys must be strings).

### Pattern 2: Awaiting-Path State Machine in HandleText
**What:** When an unregistered channel sends a message, the bot enters an "awaiting path" state for that channel. The next text message from the same channel is treated as a path input, not a Claude query.

```go
// Source: CONTEXT.md decision — free-text path entry flow
// Store awaiting state in-memory (lost on restart is acceptable — user re-sends)
type awaitingPathState struct {
    mu       sync.Mutex
    channels map[int64]bool
}

// HandleText flow:
// 1. Check mappings.Get(chatID) — if mapped, proceed normally
// 2. If NOT mapped AND NOT awaitingPath[chatID]:
//    - Set awaitingPath[chatID] = true
//    - Reply: "This channel has no project. Reply with the full path to link it."
//    - Return (do NOT route to Claude)
// 3. If NOT mapped AND awaitingPath[chatID]:
//    - Treat text as candidate path
//    - ValidatePath(text, cfg.AllowedPaths) — if invalid, reply error, stay in awaiting
//    - If valid: mappings.Set(chatID, ProjectMapping{Path: text, Name: basename(text), LinkedAt: now})
//    - Clear awaitingPath[chatID]
//    - Reply: "Linked to <path>. Send a message to start."
//    - Return (do NOT route to Claude yet — lazy start)
```

**Key insight:** The awaiting state is lost on restart but that is acceptable — the user will just re-send any message and the bot will prompt again.

### Pattern 3: Per-Project WorkerConfig in HandleText
**What:** When starting a worker for a channel, build WorkerConfig from the channel's mapping, not from cfg global.

```go
// Source: session.go WorkerConfig — AllowedPaths and SafetyPrompt already per-worker
// Port config.buildSafetyPrompt to accept project path
func buildProjectSafetyPrompt(projectPath string) string {
    return config.BuildSafetyPromptForPaths([]string{projectPath})
}

workerCfg := session.WorkerConfig{
    AllowedPaths: []string{mapping.Path},
    SafetyPrompt: buildProjectSafetyPrompt(mapping.Path),
    FilteredEnv:  config.FilteredEnv(),
    OnQueryComplete: ...,
}
```

### Pattern 4: GSD_OPERATIONS Table
**What:** Slice of [key, label, command] triples — identical structure to TypeScript.

```go
// Source: src/handlers/commands.ts GSD_OPERATIONS
type GsdOperation struct {
    Key     string // callback suffix, e.g. "execute"
    Label   string // button text, e.g. "Execute Phase"
    Command string // slash command, e.g. "/gsd:execute-phase"
}

var GSDOperations = []GsdOperation{
    // Quick-actions row (top, as per CONTEXT.md)
    {"next", "Next", "/gsd:next"},
    {"progress", "Progress", "/gsd:progress"},
    // Phase workflow
    {"quick", "Quick Task", "/gsd:quick"},
    {"plan", "Plan Phase", "/gsd:plan-phase"},
    {"execute", "Execute Phase", "/gsd:execute-phase"},
    {"discuss", "Discuss Phase", "/gsd:discuss-phase"},
    // ... (20 operations total per CONTEXT.md)
}

// Phase picker operations (show picker before sending)
var PhasePicker = map[string]string{
    "execute": "gsd-exec",
    "plan":    "gsd-plan",
    "discuss": "gsd-discuss",
    "research":"gsd-research",
    "verify":  "gsd-verify",
    "remove-phase": "gsd-remove",
}
```

### Pattern 5: Response Button Extraction
**What:** After each Claude response completes streaming, scan the full response text for GSD commands and options. If found, send a follow-up message with an inline keyboard.

```go
// Source: src/formatting.ts — extractGsdCommands, extractNumberedOptions
// Compile regexes once at package init
var (
    gsdCmdRegex     = regexp.MustCompile(`/gsd:([a-z-]+)(?:\s+([\d.]+))?`)
    numberedOptRegex = regexp.MustCompile(`(?m)^(\d+)\.\s+(.+)`)
    // Lettered options (Claude's discretion items — extend beyond TypeScript)
    letteredOptRegex = regexp.MustCompile(`(?m)^([A-Za-z])[.)]\s+(.+)`)
)

type GsdSuggestion struct {
    Command string // "/gsd:execute-phase 2"
    Label   string // "Execute Phase 2"
}

type OptionButton struct {
    Key   string // "1", "A", "a"
    Label string // truncated to ~28 chars
}

func ExtractGsdCommands(text string) []GsdSuggestion
func ExtractNumberedOptions(text string) []OptionButton  // 1. 2. 3.
func ExtractLetteredOptions(text string) []OptionButton  // A. B. or a) b)
```

**De-duplication:** Use a `map[string]bool` seen set — same command may appear multiple times in long Claude responses.

**Trigger threshold:** Lettered options require at least 2 consecutive items (same as TypeScript's numbered requirement). Single standalone letter items are not extracted.

### Pattern 6: Global API Rate Limiter
**What:** Single `rate.Limiter` shared across all channels to cap total Telegram API edit calls. Telegram's documented limit is 30 edit calls/second globally.

```go
// Source: golang.org/x/time/rate (already in go.mod)
// Target: ~25 edits/sec (leave headroom below 30/sec hard limit)
globalAPILimiter := rate.NewLimiter(rate.Limit(25), 5) // 25/sec, burst 5

// In streaming callback (CreateStatusCallback equivalent):
// Before each b.EditMessageText call:
globalAPILimiter.Wait(ctx) // blocks briefly if over limit
```

**Placement:** Global limiter lives on the `Bot` struct, passed to the streaming state factory. Per-channel limiter (existing ChannelRateLimiter) remains for message ingestion rate limiting — the global limiter is specifically for Telegram API edit calls during streaming.

### Pattern 7: parseRoadmap — Go Port
**What:** Read `.planning/ROADMAP.md` from a project directory; parse phases.

```go
// Source: src/handlers/commands.ts parseRoadmap
// Regex: ^- \[(.)\] \*\*Phase ([\d.]+): ([^*]+)\*\* - (.+)$
type RoadmapPhase struct {
    Number string // "2"
    Name   string // "Multi-Project and GSD Integration"
    Status string // "done", "pending", "skipped"
}

func ParseRoadmap(projectDir string) []RoadmapPhase {
    path := filepath.Join(projectDir, ".planning", "ROADMAP.md")
    // Read file; if not found, return empty slice (not an error)
    // Parse line by line with regexp
}
```

**Status mapping:** `[x]` → "done", `[~]` → "skipped", `[ ]` → "pending".

### Pattern 8: Callback Data Prefixes (Extended parseCallbackData)
**What:** Add new callback prefixes to the existing pure-function router.

```go
// Source: src/handlers/callback.ts — all prefix patterns ported
const (
    callbackActionGsd        // "gsd:{operation}"
    callbackActionGsdRun     // "gsd-run:{command}"
    callbackActionGsdFresh   // "gsd-fresh:{command}"
    callbackActionGsdPhase   // "gsd-exec:2", "gsd-plan:3", etc.
    callbackActionOption     // "option:1", "option:2", "option:A"
    callbackActionAskUser    // "askuser:{request_id}:{option_index}"
    callbackActionProject    // "project:change" or "project:unlink"
    callbackActionProjectSet // "project-set:{path_index}"
    // ... existing: resume:, action:stop/new/retry
)
```

**Callback data length limit:** Telegram enforces 64 bytes max on callback_data. Current `resume:<uuid>` is ~43 bytes (fine). New prefixes:
- `gsd:execute` = 11 bytes (fine)
- `gsd-run:/gsd:execute-phase 2` = 30 bytes (fine)
- `gsd-exec:2` = 10 bytes (fine)
- `option:1` = 8 bytes (fine)
- `askuser:<request_id>:<index>` — request_id must be short (use 8-char hex, not full UUID)

### Anti-Patterns to Avoid
- **Global WorkingDir for new sessions:** After Phase 2, `cfg.WorkingDir` must never be used as the working dir for a new channel session. Always look up the mapping.
- **Eager worker spawn on mapping link:** User decision is lazy start. Do not start a worker goroutine when the mapping is saved — only when the first real message arrives.
- **Double-starting workers for restored sessions:** The `shouldStartWorker` heuristic in HandleText (`SessionID() == "" && time.Since(StartedAt()) < 1s`) must be updated for Phase 2. Restored sessions (from mappings + session-history at startup) have their worker started by `restoreSessions`, not HandleText.
- **Blocking the streaming goroutine on rate limiter:** Use `globalAPILimiter.Wait(ctx)` (which respects context cancellation) not `globalAPILimiter.Allow()` which is non-blocking and would silently drop edits.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Token bucket rate limiting | Custom counter + time.Sleep | `golang.org/x/time/rate.Limiter` (already in go.mod) | Handles burst correctly, context-aware Wait, already used by ChannelRateLimiter |
| Atomic JSON file writes | Custom temp-file logic | Reuse PersistenceManager pattern (or extract helper) | atomic write-rename is non-trivial on Windows; pattern already battle-tested in persist.go |
| Regex for GSD commands | String splitting/indexing | `regexp.MustCompile` (stdlib) | The pattern is genuinely regex-shaped; hand-rolling is error-prone |
| Telegram inline keyboard building | Ad-hoc slice construction | Encapsulate in `buildGsdKeyboard()` / `buildOptionKeyboard()` functions | Keeps handler code readable; mirrors TypeScript's `buildActionKeyboard` |

**Key insight:** The TypeScript version is the functional spec. Every algorithm in it was arrived at through iteration. Port the algorithms faithfully — do not redesign.

## Common Pitfalls

### Pitfall 1: Callback Data Byte Length Overflow
**What goes wrong:** Telegram rejects `SendMessage` / `EditMessageText` with inline keyboards if any `callback_data` field exceeds 64 bytes.
**Why it happens:** GSD commands with long names and arguments: `gsd-run:/gsd:execute-phase 8` = 30 bytes (fine), but `gsd-fresh:/gsd:remove-phase 12` = 32 bytes (fine). Risk zone: `askuser:<request_id>:9` — if request_id is a full UUID (36 chars), total = ~47 bytes (still fine, but close). Keep request IDs to 8-char hex.
**How to avoid:** Add a `len(callbackData) <= 64` assertion in unit tests for all callback data constructors.
**Warning signs:** Telegram API returns `Bad Request: BUTTON_DATA_INVALID` error.

### Pitfall 2: JSON Object Keys Must Be Strings (channelID serialization)
**What goes wrong:** `map[int64]ProjectMapping` cannot be marshalled directly to JSON — Go's `encoding/json` requires map keys to be strings.
**Why it happens:** JSON spec requires object keys to be strings; int64 keys are not supported by stdlib encoder.
**How to avoid:** Use `map[string]ProjectMapping` internally in the JSON struct, with helpers to convert to/from int64. Pattern: `strconv.FormatInt(channelID, 10)` for serialization, `strconv.ParseInt(key, 10, 64)` for deserialization.
**Warning signs:** `json: unsupported type: map[int64]ProjectMapping` compile-time or runtime error.

### Pitfall 3: Worker Double-Start for Restored Sessions
**What goes wrong:** HandleText's `shouldStartWorker` heuristic fires for sessions that `restoreSessions` already started, creating two goroutines reading from the same queue. The queue has capacity 5 — both workers will dequeue messages, causing non-deterministic behavior.
**Why it happens:** The heuristic `SessionID() == "" && time.Since(StartedAt()) < 1s` will be false for restored sessions (they have a SessionID), but a session that was mapped but never had a Claude session (first message after fresh link) will have SessionID="" and a very recent StartedAt. The 1-second window is tight.
**How to avoid:** Add a `workerStarted bool` field to `Session`, protected by the mutex. Set it in `restoreSessions` (before starting goroutine) and in HandleText (before starting goroutine). Only start the goroutine if `!sess.WorkerStarted()`.
**Warning signs:** Two Claude CLI processes spawned for the same channel; duplicate responses.

### Pitfall 4: Global Rate Limiter Deadlock on Shutdown
**What goes wrong:** `globalAPILimiter.Wait(ctx)` blocks the streaming goroutine indefinitely when ctx is cancelled during shutdown, preventing the WaitGroup from completing.
**Why it happens:** `rate.Limiter.Wait` returns `ctx.Err()` when context is cancelled — this is correct behavior. But if the streaming goroutine doesn't check the error and return, it keeps blocking.
**How to avoid:** Always check the error from `Wait`: `if err := globalAPILimiter.Wait(ctx); err != nil { return }`.
**Warning signs:** Bot hangs for 30 seconds on shutdown (the WaitGroup timeout in bot.Stop).

### Pitfall 5: Telegram 429 "Too Many Requests" in Multi-Channel Streaming
**What goes wrong:** With 2+ channels streaming simultaneously, each calling EditMessageText every 500ms, the bot can hit 4+ edits/second per session. With N sessions, this compounds.
**Why it happens:** Each session's streaming callback fires independently. The global API rate limiter (Pitfall prevention) is the correct fix. Without it, Telegram returns 429 with a `retry_after` field.
**How to avoid:** Implement global rate limiter on the Bot struct. When Telegram returns 429, gotgbot surfaces it as an error; handle it in the streaming callback by sleeping `retry_after` seconds and retrying the edit once.
**Warning signs:** `TelegramAPIError 429: Too Many Requests` in logs during multi-channel streaming.

### Pitfall 6: Awaiting-Path State Lost on Restart
**What goes wrong:** A user triggers the "type your path" flow, then the bot restarts before they respond. On restart, the channel is still unmapped, but the awaiting-path state is gone. The next message re-triggers the prompt.
**Why it happens:** Awaiting-path state is in-memory only (as decided: lost on restart is acceptable).
**How to avoid:** The behavior is intentional and acceptable. Document it in the prompt message: "Reply with the full directory path to link this channel."
**Warning signs:** None — this is expected behavior.

### Pitfall 7: ParseRoadmap on Windows Paths with os.ReadFile
**What goes wrong:** Reading `.planning/ROADMAP.md` with forward-slash joined path fails on Windows if the project path uses backslashes.
**Why it happens:** `filepath.Join` uses the OS path separator; the roadmap file must be read via `filepath.Join(projectDir, ".planning", "ROADMAP.md")`, not string concatenation with `/`.
**How to avoid:** Always use `filepath.Join` for path construction in Go code. The TypeScript version uses `join()` from Node's `path` module which handles this correctly.
**Warning signs:** `os: no such file or directory` errors when parsing ROADMAP.md on Windows.

### Pitfall 8: Lettered Option Regex False-Positives
**What goes wrong:** The lettered option regex `^([A-Za-z])[.)]\s+(.+)` matches legitimate sentence beginnings like "A. This is a topic sentence." appearing mid-response.
**Why it happens:** Claude sometimes uses single-letter list markers that look identical to sentence structure.
**How to avoid:** Apply the same minimum-consecutive-items heuristic as numbered options: only extract lettered options if there are at least 2 consecutive items (B follows A, C follows B, etc.). Reset on any non-matching non-empty line. Limit to uppercase A-Z sequences (do not mix case within one sequence).
**Warning signs:** Option buttons appearing on non-choice responses.

## Code Examples

### MappingStore JSON Structure
```go
// Source: CONTEXT.md locked decision — {channelID: {path, name, linkedAt}}
// Note: JSON keys must be strings (channelID serialized as decimal string)
type mappingsFile struct {
    Mappings map[string]ProjectMapping `json:"mappings"`
}

// Serialization helper
func channelKey(channelID int64) string {
    return strconv.FormatInt(channelID, 10)
}
```

### Compile-Once Regex Patterns
```go
// Source: src/formatting.ts extractGsdCommands, extractNumberedOptions
var (
    // Matches: /gsd:execute-phase, /gsd:plan-phase 3, /gsd:next
    gsdCmdRE = regexp.MustCompile(`/gsd:([a-z-]+)(?:\s+([\d.]+))?`)

    // Matches: 1. Option text, 2. Option text (requires ^\d+\.)
    numberedOptRE = regexp.MustCompile(`(?m)^(\d+)\.\s+(.+)`)

    // Matches: A. Option, B. Option OR a) Option, b) Option
    letteredOptRE = regexp.MustCompile(`(?m)^([A-Z])[.)]\s+(.+)`)

    // Roadmap line parser
    roadmapRE = regexp.MustCompile(`^- \[(.)\] \*\*Phase ([\d.]+): ([^*]+)\*\* - (.+)$`)
)
```

### HandleText Mapping Check (Pseudocode)
```go
// Source: CONTEXT.md locked decision — mapping check before session routing
func HandleText(tgBot *gotgbot.Bot, ctx *ext.Context, store *session.SessionStore,
    mappings *project.MappingStore, awaitingPath *awaitingState, cfg *config.Config, ...) error {

    chatID := ctx.EffectiveChat.Id
    text   := ctx.EffectiveMessage.Text

    // 1. Check if awaiting path input from this channel
    if awaitingPath.IsAwaiting(chatID) {
        return handlePathInput(tgBot, ctx, chatID, text, mappings, awaitingPath, cfg)
    }

    // 2. Check if channel has a project mapping
    mapping, ok := mappings.Get(chatID)
    if !ok {
        awaitingPath.Set(chatID)
        _, err := tgBot.SendMessage(chatID,
            "This channel has no linked project. Reply with the full directory path to link it.", nil)
        return err
    }

    // 3. Channel is mapped — proceed with per-project config
    workerCfg := session.WorkerConfig{
        AllowedPaths: []string{mapping.Path},
        SafetyPrompt: config.BuildSafetyPromptForPaths([]string{mapping.Path}),
        FilteredEnv:  config.FilteredEnv(),
        OnQueryComplete: ...,
    }
    sess := store.GetOrCreate(chatID, mapping.Path)
    // ... rest of existing HandleText logic ...
}
```

### Response Button Extraction Integration
```go
// Source: src/handlers/commands.ts sendGsdCommand — post-streaming button injection
// Called after proc.Stream() completes, with the full accumulated response text
func maybeAttachActionKeyboard(tgBot *gotgbot.Bot, chatID int64, responseText string) {
    gsdCmds := gsd.ExtractGsdCommands(responseText)
    numbered := gsd.ExtractNumberedOptions(responseText)
    lettered := gsd.ExtractLetteredOptions(responseText)

    if len(gsdCmds) == 0 && len(numbered) == 0 && len(lettered) == 0 {
        return // clean response — no keyboard
    }

    keyboard := gsd.BuildResponseKeyboard(gsdCmds, numbered, lettered)
    tgBot.SendMessage(chatID, "—", &gotgbot.SendMessageOpts{
        ReplyMarkup:         keyboard,
        DisableNotification: true,
    })
}
```

### ask_user MCP Callback (GSD-05)
```go
// Source: src/handlers/callback.ts — askuser:{request_id}:{option_index}
// ask_user writes a JSON file to os.TempDir(); bot reads it on button press
type askUserRequest struct {
    Question string   `json:"question"`
    Options  []string `json:"options"`
    Status   string   `json:"status"`
}

// Callback data format: "askuser:a1b2c3d4:0"  (8-char hex request ID, option index)
// On button press:
// 1. Read os.TempDir()/ask-user-{requestID}.json
// 2. Validate option index
// 3. Edit message to show selection
// 4. Delete temp file
// 5. Send selected option text to Claude via session.Enqueue
```

### /gsd Command Status Header
```go
// Source: src/handlers/commands.ts handleGsd status header
// Port parseRoadmap + build status text
func buildGsdStatusHeader(projectPath string) string {
    phases := parseRoadmap(projectPath)
    if len(phases) == 0 {
        return filepath.Base(projectPath) + "\nNo ROADMAP.md found"
    }
    done := 0
    total := 0
    var next *RoadmapPhase
    for i := range phases {
        if phases[i].Status == "skipped" { continue }
        total++
        if phases[i].Status == "done" { done++ }
        if phases[i].Status == "pending" && next == nil { next = &phases[i] }
    }
    header := fmt.Sprintf("%s\n%d/%d phases complete", filepath.Base(projectPath), done, total)
    if next != nil {
        header += fmt.Sprintf("\n\nNext: Phase %s: %s", next.Number, next.Name)
    }
    return header
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Single global WorkingDir (Phase 1) | Per-channel mapping from MappingStore | Phase 2 | Config.WorkingDir becomes a fallback only |
| GSD via text commands only | GSD via inline keyboard with phase picker | Phase 2 | Power users retain text interface; mobile users get tap-to-run |
| TypeScript GSD_OPERATIONS (16 ops) | Go port with 20 ops (adds Next, Quick Task, Add Phase, Remove Phase) | Phase 2 | "Next" as top-priority quick action; CONTEXT.md decision |

**Deprecated/outdated:**
- `cfg.WorkingDir` as the session working directory for newly-created sessions: After Phase 2, this field is only used as a fallback for legacy mapped sessions that pre-date mappings.json.

## Open Questions

1. **Streaming accumulator for response button extraction**
   - What we know: `proc.Stream()` fires a StatusCallback for each event; the final `result` event carries the full response text via `proc.FinalText()` or similar field.
   - What's unclear: Does the existing `claude.Process` accumulate the full response text, or does the streaming callback need to do it?
   - Recommendation: Examine `internal/claude/process.go` and the `Stream()` method — if it doesn't already accumulate, add an `AccumulatedText()` accessor or pass the text via `OnQueryComplete` callback.

2. **/project command arg handling (direct reassignment)**
   - What we know: CONTEXT.md says "typing `/project <path>` directly reassigns." In grammY, `ctx.match` provides the argument. In gotgbot with gotgbot's `handlers.NewCommand`, the match is via `ctx.Args()` or message text parsing.
   - What's unclear: Does gotgbot/ext expose command arguments conveniently?
   - Recommendation: Parse `strings.TrimPrefix(ctx.EffectiveMessage.Text, "/project ")` — simple, no framework magic needed.

3. **ask_user MCP file availability**
   - What we know: The TypeScript version reads from `os.TempDir()/ask-user-{requestID}.json`. The MCP server writes this file when Claude invokes the ask_user tool.
   - What's unclear: Whether the same MCP server is configured/running for the Go bot, and whether it writes to the same TempDir path on Windows.
   - Recommendation: GSD-05 implementation should include a test with a synthetic temp file to verify the handshake works; do not assume the MCP server is available during unit tests.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | none — go test ./... discovers all *_test.go |
| Quick run command | `go test ./internal/handlers/... ./internal/project/...` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PROJ-01 | MappingStore.Get returns mapping for registered channel | unit | `go test ./internal/project/... -run TestMappingGet` | Wave 0 |
| PROJ-02 | WorkerConfig built with per-project AllowedPaths | unit | `go test ./internal/handlers/... -run TestWorkerConfigPerProject` | Wave 0 |
| PROJ-03 | HandleText sends link prompt for unmapped channel | unit | `go test ./internal/handlers/... -run TestHandleTextUnmapped` | Wave 0 |
| PROJ-04 | MappingStore persists to JSON and survives reload | unit | `go test ./internal/project/... -run TestMappingPersistence` | Wave 0 |
| PROJ-05 | MappingStore.Set replaces existing mapping | unit | `go test ./internal/project/... -run TestMappingReassign` | Wave 0 |
| GSD-01 | parseCallbackData routes gsd: prefix correctly | unit | `go test ./internal/handlers/... -run TestCallbackGsd` | Wave 0 |
| GSD-02 | ExtractGsdCommands finds /gsd:command patterns | unit | `go test ./internal/handlers/... -run TestExtractGsdCommands` | Wave 0 |
| GSD-03 | ExtractNumberedOptions and ExtractLetteredOptions | unit | `go test ./internal/handlers/... -run TestExtractOptions` | Wave 0 |
| GSD-04 | parseRoadmap parses all three status variants | unit | `go test ./internal/handlers/... -run TestParseRoadmap` | Wave 0 |
| GSD-05 | askuser callback reads temp file and sends selection | unit | `go test ./internal/handlers/... -run TestAskUserCallback` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/project/... ./internal/handlers/...`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/project/mapping_test.go` — covers PROJ-01, PROJ-04, PROJ-05
- [ ] `internal/handlers/gsd_test.go` — covers GSD-01, GSD-02, GSD-03, GSD-04, GSD-05
- [ ] `internal/handlers/text_test.go` (extend) — covers PROJ-02, PROJ-03

## Sources

### Primary (HIGH confidence)
- `src/handlers/commands.ts` — GSD_OPERATIONS table (19 ops in file, CONTEXT.md specifies 20 with Next added), parseRoadmap, handleGsd, sendGsdCommand — read directly
- `src/handlers/callback.ts` — all callback prefix routing patterns, PHASE_PICKER_OPS, PHASE_CALLBACK_MAP — read directly
- `src/formatting.ts` — extractGsdCommands regex, extractNumberedOptions algorithm, buildActionKeyboard structure — read directly
- `internal/session/store.go` — SessionStore API, double-checked locking pattern — read directly
- `internal/session/session.go` — WorkerConfig fields, Worker goroutine lifecycle — read directly
- `internal/session/persist.go` — atomic write-rename pattern reusable for MappingStore — read directly
- `internal/config/config.go` — buildSafetyPrompt signature (takes []string paths) — read directly
- `internal/handlers/callback.go` — parseCallbackData pure function pattern — read directly
- `internal/handlers/text.go` — shouldStartWorker heuristic, WorkerConfig construction — read directly
- `internal/bot/bot.go` — restoreSessions pattern, WaitGroup tracking — read directly

### Secondary (MEDIUM confidence)
- Telegram Bot API docs (from knowledge): callback_data 64-byte limit — well-established constraint
- golang.org/x/time/rate documentation (from knowledge, verified by existing use in ratelimit.go): `Wait(ctx)` is context-aware

### Tertiary (LOW confidence)
- Telegram 30-edits/second global rate limit: commonly cited in community resources; official docs state limits without specifying exact numbers for edit calls. Use 25/sec as conservative headroom.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all packages already in go.mod; patterns already used in Phase 1
- Architecture: HIGH — direct port from TypeScript functional spec with verified Go idioms
- Pitfalls: HIGH — most derived from direct code inspection of existing codebase + TypeScript-to-Go translation risks
- Regex patterns: HIGH — ported verbatim from working TypeScript with minor Go syntax adaptation

**Research date:** 2026-03-19
**Valid until:** 2026-06-01 (stable — gotgbot, Go stdlib, Telegram API all stable)
