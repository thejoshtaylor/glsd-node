---
phase: 02-multi-project-and-gsd-integration
plan: 01
subsystem: project
tags: [go, json-persistence, regex, inline-keyboard, gotgbot]

# Dependency graph
requires:
  - phase: 01-core-bot-infrastructure
    provides: session/persist.go atomic write pattern, session/store.go RWMutex pattern, config.ButtonLabelMaxLength
provides:
  - MappingStore: channel-to-project mapping with JSON persistence (internal/project)
  - GSDOperations: 20-entry operations table (internal/handlers)
  - ExtractGsdCommands: /gsd:command regex extractor with deduplication
  - ExtractNumberedOptions: consecutive numbered list detector (2+ required)
  - ExtractLetteredOptions: sequential uppercase letter detector (A→B→C)
  - ParseRoadmap: ROADMAP.md line parser for [x]/[ ]/[~] status
  - BuildGsdKeyboard: 10-row inline keyboard builder
  - BuildResponseKeyboard: contextual response keyboard builder
  - BuildPhasePickerKeyboard: phase selector keyboard builder
affects:
  - 02-02 (project link/unlink wiring uses MappingStore)
  - 02-03 (GSD command dispatch wiring uses GSDOperations and keyboard builders)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Atomic write-rename persistence: json.MarshalIndent → os.CreateTemp → os.Rename (replicated from session/persist.go)"
    - "String-keyed JSON for int64 maps: channelKey()/strconv.FormatInt for serialization, strconv.ParseInt for deserialization"
    - "Compile-once package-level regex vars: regexp.MustCompile at init time"
    - "gsdOpIndex: precomputed map[string]GsdOperation for O(1) label lookup during extraction"

key-files:
  created:
    - internal/project/mapping.go
    - internal/project/mapping_test.go
    - internal/handlers/gsd.go
    - internal/handlers/gsd_test.go
  modified: []

key-decisions:
  - "MappingStore uses string-keyed JSON (same as mappingsFile pattern from RESEARCH.md Pitfall 2) because JSON object keys must be strings"
  - "ExtractLetteredOptions requires letter continuity (A→B, B→C) not just 2+ consecutive to avoid false positives from unrelated lettered content"
  - "PhasePickerKeyboard skips 'skipped' status phases entirely — they are not actionable"
  - "gsdOpIndex precomputed at init to avoid linear scan per extraction call"

patterns-established:
  - "Pure functions: all GSD logic is side-effect free, tested without gotgbot types"
  - "Keyboard callback data stays under 64 bytes — verified by TestCallbackDataLength"

requirements-completed: [PROJ-01, PROJ-04, PROJ-05, GSD-02, GSD-03, GSD-04]

# Metrics
duration: 5min
completed: 2026-03-19
---

# Phase 2 Plan 01: Foundational Modules Summary

**Thread-safe MappingStore with atomic JSON persistence and 20-operation GSD pure-function layer with regex extractors, roadmap parser, and keyboard builders**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-19T03:14:00Z
- **Completed:** 2026-03-19T03:19:00Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- MappingStore providing CRUD + atomic JSON persistence for channel-to-project mappings (7 tests)
- GSD operations table (20 entries), regex extractors (ExtractGsdCommands, ExtractNumberedOptions, ExtractLetteredOptions), roadmap parser (ParseRoadmap), and three keyboard builders (16 tests)
- All callback data verified <= 64 bytes via TestCallbackDataLength

## Task Commits

Each task was committed atomically:

1. **Task 1: MappingStore package** - `fd9bf54` (feat)
2. **Task 2: GSD pure functions** - `43c5592` (feat)

## Files Created/Modified

- `internal/project/mapping.go` - MappingStore with Get/Set/Remove/All/Load + atomic persistence
- `internal/project/mapping_test.go` - 7 unit tests covering CRUD, persistence, reassign, missing file
- `internal/handlers/gsd.go` - GSDOperations table, PhasePickerOps, regex extractors, roadmap parser, 3 keyboard builders
- `internal/handlers/gsd_test.go` - 16 unit tests covering all extractors, parser, keyboard builders, callback data length

## Decisions Made

- String-keyed JSON for int64 channel IDs: JSON object keys must be strings, so `strconv.FormatInt` serializes and `strconv.ParseInt` deserializes (avoids JSON unmarshaling failure)
- ExtractLetteredOptions requires sequential letters (A→B, B→C) not merely 2+ items — prevents false positives from non-list uppercase letter patterns
- BuildPhasePickerKeyboard silently skips "skipped" phases — they have no actionable callback
- Precomputed `gsdOpIndex` map at package init for O(1) label lookup per match in ExtractGsdCommands

## Deviations from Plan

None — plan executed exactly as written. Extra tests added (TestGSDOperationsCount, TestPhasePickerKeyboard_SkipsSkipped, TestExtractNumberedOptions_Three) beyond the 13 specified to improve coverage, which is within plan scope.

## Issues Encountered

- Go binary not in default bash PATH on Windows — resolved by extending PATH to `/c/Users/jtayl/AppData/Local/Programs/Go/bin` in each bash call.

## Next Phase Readiness

- Plan 02 (project link/unlink wiring) can import `internal/project.MappingStore` directly
- Plan 03 (GSD command dispatch) can import `internal/handlers.GSDOperations`, `ExtractGsdCommands`, `BuildGsdKeyboard`, `BuildResponseKeyboard`, `BuildPhasePickerKeyboard`
- No blockers

---
*Phase: 02-multi-project-and-gsd-integration*
*Completed: 2026-03-19*
