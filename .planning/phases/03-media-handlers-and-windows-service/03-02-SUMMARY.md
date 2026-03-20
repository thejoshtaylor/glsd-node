---
phase: 03-media-handlers-and-windows-service
plan: 02
subsystem: handlers
tags: [voice, photo, media, whisper, album-buffer]
dependency_graph:
  requires: [03-01]
  provides: [HandleVoice, HandlePhoto, buildSinglePhotoPrompt, buildAlbumPrompt, truncateTranscript]
  affects: [bot/handlers.go (wiring in plan 04)]
tech_stack:
  added: []
  patterns: [MediaGroupBuffer album batching, OpenAI Whisper transcription, temp file lifecycle management]
key_files:
  created:
    - internal/handlers/voice.go
    - internal/handlers/voice_test.go
    - internal/handlers/photo.go
    - internal/handlers/photo_test.go
  modified: []
  deleted:
    - internal/handlers/voice_stub.go
    - internal/handlers/photo_stub.go
decisions:
  - "Photo temp file cleanup deferred to async ErrCh goroutine (not defer in handler) to ensure Claude can read the file"
  - "Album photos cleaned up after MediaGroupBuffer fires and session processes the combined prompt"
  - "sendPhotoToSession and sendAlbumToSession are separate functions (not shared with HandleText) to avoid refactoring risk"
metrics:
  duration: "6 minutes"
  completed: "2026-03-20T03:59:37Z"
  tasks_completed: 2
  tasks_total: 2
  test_count: 5
  files_created: 4
  files_deleted: 2
---

# Phase 03 Plan 02: Voice and Photo Handlers Summary

HandleVoice downloads OGG from Telegram, transcribes via OpenAI Whisper with 60s timeout, shows truncated transcript, then enqueues raw transcript text to Claude session. HandlePhoto selects largest PhotoSize resolution, downloads to temp file, and routes single photos immediately or batches album photos via MediaGroupBuffer with 1-second timeout.

## What Was Built

### HandleVoice (voice.go)
- Guards for missing OpenAI API key with user-friendly error message
- Mapping check identical to HandleText pattern
- Downloads OGG via downloadToTemp helper
- Transcribes with 60-second timeout context via transcribeVoice
- Shows "Transcribing..." status, then edits to show transcript (truncated to 200 chars)
- Enqueues raw transcript text to Claude session with full worker lifecycle
- Async ErrCh drain with maybeAttachActionKeyboard on success
- Audit logging for voice_message events

### HandlePhoto (photo.go)
- Selects largest resolution: `photos[len(photos)-1]`
- Downloads via downloadToTemp with ".jpg" suffix
- Single photos: builds `[Photo: /path]` prompt, immediate enqueue
- Albums: lazily initializes MediaGroupBuffer with `time.Second` timeout
- Album callback builds `[Photos:\n1. /path\n2. /path\n...]` prompt format
- Temp file cleanup in async ErrCh goroutine (after Claude processes)
- Album cleanup in buffer callback after processing

### Pure Functions (tested)
- `truncateTranscript(s, maxLen)` - truncates with "..." suffix
- `buildSinglePhotoPrompt(path, caption)` - `[Photo: path]` format
- `buildAlbumPrompt(paths, caption)` - `[Photos:\n1. ...\n2. ...]` format

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Removed voice_stub.go and photo_stub.go**
- **Found during:** Task 1 and Task 2
- **Issue:** Plan 03-01 created stub files (voice_stub.go, photo_stub.go) to keep the package compilable. These conflicted with the real implementations.
- **Fix:** Deleted stub files; photo_stub.go was git-tracked so used `git rm`
- **Files modified:** voice_stub.go (deleted), photo_stub.go (deleted)
- **Commits:** e681fc5, 60d0e47

## Commits

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | HandleVoice | e681fc5 | voice.go, voice_test.go |
| 2 | HandlePhoto | 60d0e47 | photo.go, photo_test.go, -photo_stub.go |

## Verification

- `go build ./...` exits 0
- `go test ./internal/handlers/... -count=1` passes all 67 tests
- Voice: missing API key returns friendly error, transcription flow verified via helper tests
- Photo: single and album prompt formats verified, function existence confirmed
