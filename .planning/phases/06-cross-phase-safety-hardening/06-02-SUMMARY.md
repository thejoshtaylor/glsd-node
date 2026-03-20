---
phase: 06-cross-phase-safety-hardening
plan: 02
subsystem: handlers/security
tags: [safety, media, voice, photo, document, CheckCommandSafety]
dependency_graph:
  requires: [internal/security/validate.go, internal/config/config.go]
  provides: [CheckCommandSafety coverage for voice/photo/document handlers]
  affects: [internal/handlers/voice.go, internal/handlers/photo.go, internal/handlers/document.go]
tech_stack:
  added: []
  patterns: [security.CheckCommandSafety early-return pattern before enqueue]
key_files:
  created: []
  modified:
    - internal/handlers/voice.go
    - internal/handlers/photo.go
    - internal/handlers/document.go
    - internal/handlers/command.go
    - internal/handlers/callback.go
decisions:
  - Photo safety check guards the caption only (not the file path, which is bot-controlled)
  - Document safety check guards both caption AND extracted content (content may contain injected commands from adversarial PDFs)
  - Early returns in photo.go and document.go call os.Remove(path) explicitly before returning to avoid leaving downloaded files on disk
  - command.go call to enqueueGsdCommand passes nil/0 for auditLog/userID (HandleGsd has no audit params in Phase 06)
metrics:
  duration: 6min
  completed: 2026-03-20
  completed_tasks: 2
  total_tasks: 2
  files_modified: 5
---

# Phase 06 Plan 02: Add CheckCommandSafety to Media Handlers Summary

CheckCommandSafety wired into voice, photo, and document handlers blocking adversarial content before it reaches the Claude session.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add CheckCommandSafety to voice.go | daaf15b | voice.go, command.go |
| 2 | Add CheckCommandSafety to photo.go and document.go | 6d78247 | photo.go, document.go |

## What Was Built

All three media handlers now call `security.CheckCommandSafety` before enqueuing to the Claude session:

- **voice.go**: Checks the transcribed text. If blocked, sends user message and returns without enqueueing.
- **photo.go**: Checks the caption (if non-empty). If blocked, removes the downloaded photo and returns.
- **document.go**: Checks both the caption (if non-empty) AND the extracted document content. Both guards independently block and clean up the downloaded file.

The safety check is consistent with the existing pattern in `text.go` and `callback.go` (added in Plan 06-01).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed pre-existing build errors from partial Plan 06-01 work**
- **Found during:** Task 1 build verification
- **Issue:** callback.go used `cq.Sender` (invalid field on gotgbot v2 `CallbackQuery`); the correct field is `cq.From`. Also, `command.go` called `enqueueGsdCommand` with old arity (missing `auditLog` and `userID` params added by Plan 06-01).
- **Fix:** Changed `cq.Sender.Id` to `cq.From.Id` in HandleCallback; updated the one call site in HandleGsd to pass `nil, 0` for the new params.
- **Files modified:** `internal/handlers/callback.go`, `internal/handlers/command.go`, `internal/bot/handlers.go`
- **Commit:** daaf15b (included with Task 1)

## Verification

- `go build ./...` exits 0
- `go test ./...` exits 0 (all packages pass, 87 handler tests)
- `grep security.CheckCommandSafety internal/handlers/voice.go internal/handlers/photo.go internal/handlers/document.go` confirms all three files contain the check

## Self-Check: PASSED

All modified files present. Both task commits verified in git log.
