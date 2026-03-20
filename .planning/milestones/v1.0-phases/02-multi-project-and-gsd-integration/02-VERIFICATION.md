---
phase: 02-multi-project-and-gsd-integration
verified: 2026-03-19T00:00:00Z
status: human_needed
score: 10/10 must-haves verified
human_verification:
  - test: "Send a message to a new unlinked channel"
    expected: "Bot replies asking for a project directory path"
    why_human: "Requires live Telegram bot interaction"
  - test: "Reply with a valid project path to link channel"
    expected: "Bot confirms 'Linked to {path}.' and subsequent messages route to Claude"
    why_human: "Requires live Telegram interaction and filesystem directory presence"
  - test: "Type /project in a linked channel"
    expected: "Bot shows current mapping with Change and Unlink inline buttons"
    why_human: "Requires live Telegram interaction to verify inline keyboard renders"
  - test: "Type /gsd in a linked channel"
    expected: "Bot shows status header with phase progress and 20-operation inline keyboard (Next + Progress on top row, remaining 18 in 2-column rows)"
    why_human: "Requires live Telegram interaction to verify keyboard layout and status content"
  - test: "Tap a phase-picker operation (e.g. Execute Phase) in /gsd keyboard"
    expected: "Bot shows phase picker with checkmark/hourglass indicators for each non-skipped phase"
    why_human: "Requires live Telegram interaction and a project with a ROADMAP.md to parse"
  - test: "Have Claude respond with /gsd: commands or numbered options (1. 2. 3.)"
    expected: "Bot sends a follow-up message with Run/Fresh or option buttons after streaming completes"
    why_human: "Requires live Claude session interaction to trigger response button extraction"
  - test: "Type /resume after using two different project channels"
    expected: "Each channel's /resume shows only sessions for that channel's mapped project (no cross-project bleed)"
    why_human: "Requires running two simultaneous mapped channels and saved sessions to filter"
  - test: "Run two simultaneous Claude sessions in two mapped channels"
    expected: "No Telegram 429 rate limit errors in logs; both sessions stream without errors"
    why_human: "Requires concurrent bot operation; rate limiter behavior only visible under load"
---

# Phase 2: Multi-Project and GSD Integration Verification Report

**Phase Goal:** Multiple Telegram channels each route to independent Claude sessions with no context bleed, and the GSD workflow is fully accessible via inline keyboard menus
**Verified:** 2026-03-19
**Status:** human_needed (all automated checks passed; 8 items require live bot testing)
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | MappingStore.Get returns correct mapping for a registered channel | VERIFIED | `mapping.go:49` — Get with RLock, map lookup, returns (ProjectMapping, bool) |
| 2 | MappingStore.Set persists to JSON and survives Load() | VERIFIED | `mapping.go:58` — Set calls saveLocked(); `TestMappingPersistence` passes |
| 3 | MappingStore.Remove deletes mapping and persists the change | VERIFIED | `mapping.go:67` — Remove calls saveLocked(); `TestMappingReassign` passes |
| 4 | HandleText checks MappingStore and prompts for path if unmapped | VERIFIED | `text.go:103-131` — awaitingPath check then mappings.Get; prompts with "Reply with the full directory path" |
| 5 | Per-project WorkerConfig uses mapping.Path for AllowedPaths and SafetyPrompt | VERIFIED | `text.go:156` — `config.BuildSafetyPrompt([]string{mapping.Path})`; `TestWorkerConfigPerProject` passes |
| 6 | /resume filters sessions to only matching project path | VERIFIED | `command.go:230` — `s.WorkingDir == mapping.Path` filter in HandleResume |
| 7 | /gsd shows 20-operation inline keyboard with status header | VERIFIED | `command.go:324-348` — HandleGsd builds header via buildGsdStatusHeader; `BuildGsdKeyboard` called; 20 entries in GSDOperations confirmed by `TestGSDOperationsCount` |
| 8 | All GSD callback prefixes route correctly with prefix priority | VERIFIED | `callback.go:60-82` — gsd-run: before gsd: in switch; `TestParseCallbackPrefixOrder` passes |
| 9 | Response buttons extracted after Claude streaming completes | VERIFIED | `text.go:222-224` — ss.AccumulatedText() called on success; maybeAttachActionKeyboard fires |
| 10 | Global API rate limiter prevents 429 errors during multi-channel streaming | VERIFIED | `streaming.go:228-232` — globalLimiter.Wait(); `bot.go:74` — rate.NewLimiter(25, 5) |

**Score:** 10/10 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/project/mapping.go` | MappingStore with Get/Set/Remove/All/Load | VERIFIED | All 5 methods present; os.Rename atomic write; string key serialization via strconv |
| `internal/project/mapping_test.go` | Unit tests for MappingStore persistence and CRUD | VERIFIED | TestMappingPersistence, TestMappingReassign present; all project tests pass |
| `internal/handlers/gsd.go` | GSD operations table, regex extractors, roadmap parser, keyboard builders | VERIFIED | 20-entry GSDOperations, all 4 compile-once regexes, all 9 exported functions present |
| `internal/handlers/gsd_test.go` | Unit tests for regex extractors and roadmap parser | VERIFIED | TestExtractGsdCommands_Dedup, TestExtractLetteredOptions_NonSequential, TestCallbackDataLength present |
| `internal/handlers/text.go` | Multi-project HandleText with mapping check and awaiting-path state | VERIFIED | mappings.Get(chatID), awaitingPath.IsAwaiting, handlePathInput, WorkingDir: mapping.Path all present |
| `internal/handlers/text_test.go` | Unit tests for per-project WorkerConfig and unmapped channel behavior | VERIFIED | TestWorkerConfigPerProject, TestHandleTextUnmapped present and pass |
| `internal/session/session.go` | workerStarted bool field on Session | VERIFIED | workerStarted field at line 123; WorkerStarted() and SetWorkerStarted() at lines 263-272 |
| `internal/bot/bot.go` | MappingStore field on Bot, lazy restore from mappings | VERIFIED | mappings field at line 35; mappings.Load() at line 69; SetWorkerStarted in restoreSessions at line 226 |
| `internal/handlers/command.go` | HandleProject command and HandleResume with per-project filtering | VERIFIED | HandleProject at line 270; project:change/unlink callback data; WorkingDir filter at line 230 |
| `internal/handlers/callback.go` | Extended callback routing for all new prefixes | VERIFIED | All 8 new callbackAction constants; gsd-run: before gsd: in switch; all action handlers present |
| `internal/handlers/callback_test.go` | Callback routing tests including askuser temp file and GSD status header | VERIFIED | TestParseCallbackGsd, TestParseCallbackPrefixOrder, TestAskUserCallbackTempFile, TestBuildGsdStatusHeader_WithPhases all present |
| `internal/handlers/streaming.go` | Global API rate limiter integration and AccumulatedText | VERIFIED | globalLimiter field, AccumulatedText() method, globalLimiter.Wait() call all present |
| `internal/config/config.go` | BuildSafetyPrompt exported | VERIFIED | `func BuildSafetyPrompt` at line 210; no unexported `buildSafetyPrompt` remains |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/project/mapping.go` | JSON file on disk | os.Rename atomic write | WIRED | `mapping.go:160` — os.Rename(tmpPath, ms.filePath) |
| `internal/handlers/gsd.go` | ROADMAP.md on disk | os.ReadFile + roadmapRE line parsing | WIRED | `gsd.go:228-239` — os.ReadFile + roadmapRE.FindStringSubmatch |
| `internal/handlers/text.go` | `internal/project/mapping.go` | mappings.Get(chatID) lookup before session routing | WIRED | `text.go:108` — mappings.Get(chatID) present before session routing |
| `internal/bot/bot.go` | `internal/project/mapping.go` | MappingStore.Load() at startup | WIRED | `bot.go:68-70` — NewMappingStore + mappings.Load() |
| `internal/handlers/text.go` | `internal/config/config.go` | BuildSafetyPromptForPaths per-project | WIRED | `text.go:156` — config.BuildSafetyPrompt([]string{mapping.Path}) |
| `internal/handlers/command.go` | `internal/session/persist.go` | HandleResume filters by mapping.Path via WorkingDir match | WIRED | `command.go:228-233` — s.WorkingDir == mapping.Path filter loop |
| `internal/handlers/callback.go` | `internal/handlers/gsd.go` | GSD callback prefix routing to operations table | WIRED | `callback.go:31` — callbackActionGsd constant; dispatch at line 155 |
| `internal/handlers/text.go` | `internal/handlers/gsd.go` | Response button extraction after streaming completes | WIRED | `text.go:222-224` — AccumulatedText + maybeAttachActionKeyboard; `text.go:268-270` — ExtractGsdCommands/ExtractNumberedOptions/ExtractLetteredOptions called |
| `internal/handlers/streaming.go` | `golang.org/x/time/rate` | globalLimiter.Wait(ctx) before EditMessageText | WIRED | `streaming.go:228-232` — ss.globalLimiter.Wait(ctx) |
| `internal/bot/handlers.go` | `internal/handlers/command.go` | /gsd and /project command registration | WIRED | `handlers.go:43-44` — AddHandler for "project" and "gsd" |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PROJ-01 | 02-01, 02-02 | Each channel maps to exactly one project | SATISFIED | MappingStore enforces 1:1 mapping; Set overwrites any existing mapping per channel |
| PROJ-02 | 02-02 | Each project has its own independent Claude CLI session simultaneously | SATISFIED | store.GetOrCreate(chatID, mapping.Path) creates per-channel session; mapping.Path as WorkingDir isolates sessions |
| PROJ-03 | 02-02 | Unassigned channel prompts user to link a project | SATISFIED | `text.go:108-131` — unmapped channel sets awaitingPath and prompts |
| PROJ-04 | 02-01, 02-04 | Channel-project mappings persist to JSON and survive restarts | SATISFIED | MappingStore.saveLocked() writes JSON; Load() at bot startup restores state; TestMappingPersistence passes |
| PROJ-05 | 02-01, 02-02 | User can reassign or unlink a channel | SATISFIED | HandleProject supports direct path argument and Change/Unlink inline buttons; callbackActionProjectUnlink calls mappings.Remove |
| GSD-01 | 02-03, 02-04 | /gsd presents all GSD operations as categorized inline keyboard menus | SATISFIED | HandleGsd builds status header + BuildGsdKeyboard with 20 operations; TestGSDOperationsCount=20 passes |
| GSD-02 | 02-01, 02-03 | Bot extracts GSD slash commands from Claude responses as tappable buttons | SATISFIED | ExtractGsdCommands + BuildResponseKeyboard + maybeAttachActionKeyboard wired in text.go after streaming |
| GSD-03 | 02-01, 02-03 | Bot extracts numbered options from Claude responses as tappable buttons | SATISFIED | ExtractNumberedOptions wired in maybeAttachActionKeyboard; callbackActionOption handles option: callbacks |
| GSD-04 | 02-01, 02-03 | Bot displays roadmap phase progress inline when showing GSD status | SATISFIED | buildGsdStatusHeader calls ParseRoadmap and formats "N/M phases complete" + next phase |
| GSD-05 | 02-03, 02-04 | ask_user MCP integration — Claude can present questions via inline keyboard | SATISFIED | callbackActionAskUser in callback.go reads temp JSON file, parses options, sends selected option to Claude; TestAskUserCallbackTempFile passes |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/handlers/callback.go` | 100 | Comment "placeholder (deferred to v2)" for action:retry | Info | Documentation only — action:retry is properly parsed and routed; the comment notes it is deferred to v2 in terms of full implementation. Not a stub. |

No blockers or warnings found. The `return nil` occurrences throughout the codebase are legitimate Go error-return patterns, not empty implementations.

### Human Verification Required

#### 1. Unlinked Channel Prompt Flow

**Test:** Send any text message from a Telegram channel that has never been linked to a project.
**Expected:** Bot replies "This channel has no linked project. Reply with the full directory path to link it." Then reply with a valid directory under ALLOWED_PATHS — bot should confirm "Linked to {path}." and return to normal session routing.
**Why human:** Requires a live running bot and Telegram client; the prompting and state transition rely on Telegram message delivery that cannot be simulated in unit tests.

#### 2. /project Command Inline Keyboard

**Test:** In a linked channel, type `/project`.
**Expected:** Bot sends a message showing "Current project: {name}\nPath: {path}\nLinked: {timestamp}" with Change and Unlink inline keyboard buttons. Tapping Change should prompt for a new path; tapping Unlink should remove the mapping.
**Why human:** Requires live Telegram interaction to verify inline keyboard renders and buttons function correctly.

#### 3. /gsd Keyboard Layout and Status Header

**Test:** In a channel linked to a project with a `.planning/ROADMAP.md` file, type `/gsd`.
**Expected:** Bot sends a status header showing project name, phase progress count (e.g. "1/3 phases complete"), and next pending phase. Keyboard should have Next and Progress on the top row, with 9 rows of 2 below.
**Why human:** Visual keyboard layout verification requires Telegram client; status accuracy depends on actual ROADMAP.md content.

#### 4. Phase Picker for Phase-Specific Operations

**Test:** Tap "Execute Phase" (or similar) in the /gsd keyboard.
**Expected:** Bot sends a second message with one button per non-skipped phase, each showing a status indicator (checkmark for done, hourglass for pending) plus the phase number and name.
**Why human:** Requires live keyboard interaction; relies on ParseRoadmap reading an actual ROADMAP.md.

#### 5. Response Button Extraction After Streaming

**Test:** Send a message that prompts Claude to reply with a numbered list or GSD commands (e.g. "List 3 options for my next action" or "What GSD commands should I run?").
**Expected:** After Claude's streamed response completes, bot sends a follow-up message with option buttons (for numbered/lettered lists) or Run/Fresh buttons (for GSD commands).
**Why human:** Requires a live Claude session and realistic response content to trigger extraction logic.

#### 6. Per-Project /resume Isolation

**Test:** Use two different channels, each linked to a different project directory. Have each channel complete at least one Claude interaction (to create a saved session). Then type `/resume` in each channel.
**Expected:** Each channel's /resume shows only the sessions belonging to that channel's linked project path — sessions from the other project do not appear.
**Why human:** Requires multiple mapped channels, multiple saved sessions, and verifying cross-project isolation which cannot be done without running the full bot.

#### 7. No Context Bleed Between Simultaneous Sessions

**Test:** In two simultaneously linked channels, send different messages to each and observe that each channel receives only its own Claude response (no responses crossing channels).
**Expected:** Claude session for channel A responds only in channel A; channel B responses go only to channel B.
**Why human:** Session isolation under concurrent use requires live bot operation with two active Telegram channels.

#### 8. Global Rate Limiter Under Streaming Load

**Test:** With two or more channels simultaneously streaming Claude responses, check the bot logs for Telegram 429 (Too Many Requests) errors.
**Expected:** No 429 errors appear; both sessions stream concurrently without rate limit failures.
**Why human:** Rate limiting under concurrent load is a runtime behavior invisible to static analysis or unit tests.

### Gaps Summary

No automated gaps found. All must-have truths are verified at all three levels (exists, substantive, wired). The build compiles clean, all 9 test packages pass, and go vet is clean. Eight human verification items remain because they require a live running bot with a real Telegram connection, a valid Claude CLI, and concurrent channel activity.

---

_Verified: 2026-03-19_
_Verifier: Claude (gsd-verifier)_
