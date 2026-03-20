---
phase: 03-media-handlers-and-windows-service
plan: "01"
subsystem: handlers/config
tags: [media, helpers, config, tdd, go]
dependency_graph:
  requires: []
  provides:
    - internal/handlers/helpers.go
    - internal/handlers/media_group.go
    - internal/config/config.go (PdfToTextPath field)
  affects:
    - internal/handlers/voice.go (wave 2 — uses transcribeVoice, downloadToTemp)
    - internal/handlers/photo.go (wave 2 — uses downloadToTemp, MediaGroupBuffer)
    - internal/handlers/document.go (wave 2 — uses extractPDF, isTextFile, MediaGroupBuffer)
tech_stack:
  added: []
  patterns:
    - "httpClient with explicit 60s timeout to avoid default no-timeout pitfall"
    - "transcribeVoiceURL/downloadFromURL testable helpers with injectable URL parameter"
    - "MediaGroupBuffer with time.AfterFunc + sync.Mutex — timer reset per Add for batch window extension"
    - "TDD RED→GREEN per task, failing tests committed before implementation"
key_files:
  created:
    - internal/handlers/helpers.go
    - internal/handlers/media_group.go
    - internal/handlers/helpers_test.go
    - internal/handlers/media_group_test.go
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
decisions:
  - "transcribeVoiceURL / downloadFromURL as internal testable helpers: public transcribeVoice/downloadToTemp call them; tests inject mock HTTP server URL without needing a live gotgbot.Bot"
  - "MediaGroupBuffer.Add signature uses chatID/userID int64 (not *ext.Context): avoids gotgbot dependency in media_group.go, makes unit testing straightforward without live bot"
  - "extractPDF partial extraction: on non-zero exit code, if stdout is non-empty return it as success (handles encrypted/partially-corrupted PDFs per Pitfall 4)"
  - "First non-empty caption wins in MediaGroupBuffer: empty string from items without captions does not block a real caption from a later item"
metrics:
  duration: "7 minutes"
  completed_date: "2026-03-20"
  tasks_completed: 2
  files_changed: 6
---

# Phase 03 Plan 01: Media Handler Infrastructure Summary

Shared media handler infrastructure built with TDD: config additions, file download helper, OpenAI Whisper transcription, PDF extraction, and the reusable MediaGroupBuffer.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add PdfToTextPath to Config + shared helpers | 045f8e2 | config.go, helpers.go, helpers_test.go, config_test.go |
| 2 | Create MediaGroupBuffer with timer-based batching | c22a15c | media_group.go, media_group_test.go |

## What Was Built

### Config Addition (internal/config/config.go)

`PdfToTextPath string` field added after `OpenAIAPIKey`. Parsed from `PDFTOTEXT_PATH` env var in `Load()`. Logs at startup when set (mirrors `ClaudeCLIPath` logging pattern). Optional — bot starts without it; document handler will reply with a user-facing error when PDF is received.

### helpers.go (internal/handlers/helpers.go)

Four exported functions with module-level `httpClient` (60s timeout):

- `downloadToTemp(bot, fileID, suffix) (string, error)` — calls `bot.GetFile`, builds Telegram CDN URL, downloads to temp file via `downloadFromURL`
- `transcribeVoice(ctx, apiKey, filePath) (string, error)` — multipart POST to OpenAI Whisper; delegates to `transcribeVoiceURL` for testability
- `extractPDF(ctx, pdfToTextPath, filePath) (string, error)` — runs `pdftotext -layout <file> -`; partial extraction on non-zero exit if stdout non-empty
- `isTextFile(filename) bool` — case-insensitive extension lookup against 17-entry map

Constants: `maxFileSize = 10*1024*1024`, `maxTextChars = 100_000`.

Internal testable helpers: `transcribeVoiceURL(ctx, key, path, endpoint)` and `downloadFromURL(url, suffix)` — accept mock server URLs so tests work without live Telegram/OpenAI connections.

### media_group.go (internal/handlers/media_group.go)

`MediaGroupBuffer` struct with `sync.Mutex`-protected `groups` map. `NewMediaGroupBuffer(timeout, processFn)` constructor. `Add(groupID, path, chatID, userID, caption)` appends path and resets the `time.AfterFunc` timer — each new item extends the batch window. `fire()` called by the timer goroutine: removes group under lock, then calls `process` outside lock (avoids deadlock). First non-empty caption wins.

## Test Results

```
ok  github.com/user/gsd-tele-go/internal/handlers  2.287s
ok  github.com/user/gsd-tele-go/internal/config    0.749s
go build ./...  (no errors)
```

Tests written: 14 total (8 helper tests + 6 MediaGroupBuffer tests).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Removed duplicate `contains` function**
- **Found during:** Task 1 test compilation
- **Issue:** `callback_test.go` already declares `func contains(s, substr string) bool` in the `handlers` test package; adding another `contains` in `helpers_test.go` caused a redeclaration compile error
- **Fix:** Removed the duplicate from `helpers_test.go`; tests use the existing `contains` from `callback_test.go` (same package)
- **Files modified:** `internal/handlers/helpers_test.go`

**2. [Rule 1 - Bug] Removed `args []string` declared-not-used**
- **Found during:** Task 1 test compilation
- **Issue:** `var args []string` declared but not used in the `TestExtractPDF_Success` Windows branch
- **Fix:** Removed the unused variable; Windows batch script approach only needs `cmdPath`
- **Files modified:** `internal/handlers/helpers_test.go`

None of the deviations required Rule 4 (architectural changes). All were minor test fixes resolved inline.

## Self-Check: PASSED

Files created/modified:
- FOUND: internal/config/config.go (PdfToTextPath field + PDFTOTEXT_PATH parsing)
- FOUND: internal/handlers/helpers.go
- FOUND: internal/handlers/media_group.go
- FOUND: internal/handlers/helpers_test.go
- FOUND: internal/handlers/media_group_test.go
- FOUND: internal/config/config_test.go (PdfToTextPath tests)

Commits:
- ca18acb: test(03-01): add failing tests for helpers and config (RED)
- 045f8e2: feat(03-01): add PdfToTextPath to Config and shared helper functions
- 21560d7: test(03-01): add failing tests for MediaGroupBuffer (RED)
- c22a15c: feat(03-01): implement MediaGroupBuffer with timer-based batching
