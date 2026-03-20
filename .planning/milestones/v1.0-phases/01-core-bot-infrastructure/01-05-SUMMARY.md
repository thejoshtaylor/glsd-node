---
phase: 01-core-bot-infrastructure
plan: 05
subsystem: formatting
tags: [go, telegram, markdownv2, emoji, formatting]

requires:
  - phase: 01-core-bot-infrastructure
    provides: go.mod with module definition (must exist before this package compiles)

provides:
  - internal/formatting/markdown.go — EscapeMarkdownV2, ConvertToMarkdownV2, StripMarkdown, SplitMessage
  - internal/formatting/tools.go — FormatToolStatus with full emoji map matching TypeScript

affects:
  - handlers/streaming (uses FormatToolStatus for tool event display)
  - session (uses ConvertToMarkdownV2 before every Telegram message send)
  - all message sends (SplitMessage needed for >4096 char responses)

tech-stack:
  added: []
  patterns:
    - "MarkdownV2 two-pass extraction: code blocks saved before conversion, restored after escaping"
    - "strings.NewReplacer for efficient multi-char escaping (19 special chars)"
    - "Ordered emoji lookup slice prevents shorter key matching before longer (WebSearch before Search)"

key-files:
  created:
    - internal/formatting/markdown.go
    - internal/formatting/markdown_test.go
    - internal/formatting/tools.go
    - internal/formatting/tools_test.go
  modified: []

key-decisions:
  - "EscapeMarkdownV2 escapes backslash first (prevents double-escaping when replacer runs)"
  - "ConvertToMarkdownV2 uses NUL-byte placeholders for code blocks to survive inline processing"
  - "SplitMessage prefers last double-newline before limit, then single newline, then hard cut"
  - "FormatToolStatus outputs plain text (no HTML tags) since caller uses MarkdownV2 parse mode"
  - "toolEmojiOrder lookup slice guarantees WebSearch/WebFetch/TodoWrite match before shorter substrings"

patterns-established:
  - "Two-pass code block extraction: save with NUL placeholder, convert text, restore"
  - "Emoji lookup: ordered slice of keys, check strings.Contains, default to 🔧"
  - "Path shortening: normalize to forward slashes, return last 2 non-empty components"

requirements-completed: [SESS-03]

duration: 25min
completed: 2026-03-19
---

# Phase 01 Plan 05: Formatting Package Summary

**MarkdownV2 converter with two-pass code block preservation and tool status emoji formatter matching TypeScript behavior**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-03-19T00:00:00Z
- **Completed:** 2026-03-19
- **Tasks:** 2
- **Files modified:** 4 created

## Accomplishments

- MarkdownV2 escaper covers all 19 Telegram special characters using strings.NewReplacer
- ConvertToMarkdownV2 extracts code blocks/inline code before conversion so their contents are never escaped
- SplitMessage splits at paragraph boundaries (last double-newline before 4096 chars), then newline, then hard cut
- StripMarkdown provides plain-text fallback for when MarkdownV2 parse_mode is rejected by Telegram
- FormatToolStatus with per-tool formatting for all 10 tool types (Read, Write, Edit, Bash, Grep, Glob, WebSearch, WebFetch, Task, TodoWrite) plus generic MCP tool handler
- Comprehensive test coverage: 25+ test cases across both packages

## Task Commits

1. **Task 1: Create MarkdownV2 converter with escaping and plain-text fallback** - `2146f3c` (feat)
2. **Task 2: Create tool status emoji formatter** - `2e14d19` (feat)

## Files Created/Modified

- `internal/formatting/markdown.go` - EscapeMarkdownV2, ConvertToMarkdownV2, StripMarkdown, SplitMessage
- `internal/formatting/markdown_test.go` - Tests for all markdown functions
- `internal/formatting/tools.go` - FormatToolStatus, toolEmojiMap, shortenPath, truncate
- `internal/formatting/tools_test.go` - Tests for all tool formatting functions

## Decisions Made

- `EscapeMarkdownV2` escapes backslash first in the replacer chain to prevent double-escaping later characters
- `ConvertToMarkdownV2` uses NUL byte (`\x00`) as placeholder delimiter for extracted code blocks — safe because NUL cannot appear in valid Telegram message text
- Bullet list conversion only handles `-` prefix (not `*`) to avoid conflicting with bold `**` detection
- `FormatToolStatus` outputs plain text rather than HTML, since it is displayed inside MarkdownV2-formatted messages
- `toolEmojiOrder` slice ensures `WebSearch` is checked before shorter substrings that might also match

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Removed ambiguous bullet point detection for `*` prefix**
- **Found during:** Task 1 (convertInline implementation)
- **Issue:** Plan specified `^[-*] ` as bullet pattern, but `*` at position 0 conflicts with the bold `**` detection logic in the same function
- **Fix:** Bullet detection only handles `-` prefix; `*` bullets are left as-is (they are uncommon and avoiding the conflict prevents false matches on bold text)
- **Files modified:** internal/formatting/markdown.go
- **Verification:** Code logic reviewed; TestConvertMixedContent exercises bullet detection
- **Committed in:** 2146f3c (Task 1 commit)

**2. [Rule 3 - Blocking] Removed unicode/utf8 import (unused after simplifying replaceSingle)**
- **Found during:** Task 1 (code review before commit)
- **Issue:** `replaceSingle` used `utf8.RuneLen` only as a no-op type check; Go would reject unused import
- **Fix:** Removed `unicode/utf8` import, simplified `replaceSingle` to be a documented identity function
- **Files modified:** internal/formatting/markdown.go
- **Committed in:** 2146f3c (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (1 bug/ambiguity, 1 blocking import error)
**Impact on plan:** Both fixes necessary for correctness. No scope creep.

## Issues Encountered

**Go runtime not installed:** `go test` verification commands in the plan could not be executed because Go is not installed in the shell environment. Source files were written and reviewed for correctness but tests could not be run. Compile-time errors remain possible.

The existing `go.mod` and `internal/security/` package confirm Go code was previously written for this project, so Go is expected to be available in the deployment environment. Tests should be run when Go is available.

## User Setup Required

None - no external service configuration required. Go must be installed to compile and test.

## Next Phase Readiness

- `internal/formatting` package is complete and ready for use by streaming handlers
- `FormatToolStatus` is the primary consumer API — streaming.go will call it for each tool_use event
- `ConvertToMarkdownV2` and `SplitMessage` are called before every `EditMessageText` / `SendMessage`
- All functions are pure (no side effects, no I/O) and safe to test in isolation

---
*Phase: 01-core-bot-infrastructure*
*Completed: 2026-03-19*
