---
phase: 03-media-handlers-and-windows-service
verified: 2026-03-20T00:00:00Z
status: human_needed
score: 12/12 must-haves verified
re_verification: false
---

# Phase 03: Media Handlers and Windows Service — Verification Report

**Phase Goal:** Implement voice, photo, and document (PDF/text) media handlers plus NSSM-based Windows Service deployment documentation.

**Verified:** 2026-03-20
**Status:** HUMAN_NEEDED (all automated checks pass; DEPLOY-02 operational verification and live media tests require a running bot)
**Re-verification:** No — initial verification (phase was executed but never formally verified)

## Goal Achievement

### Observable Truths

| #   | Truth                                                                                                     | Status   | Evidence                                                                                                                      |
| --- | --------------------------------------------------------------------------------------------------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------- |
| 1   | voice.go checks `cfg.OpenAIAPIKey == ""` and returns a friendly error if absent                           | VERIFIED | `voice.go` line 64: `if cfg.OpenAIAPIKey == ""` guard; line 65: sends "Voice transcription not configured. Set OPENAI_API_KEY." |
| 2   | voice.go downloads OGG via `downloadToTemp` and transcribes with `context.WithTimeout(60s)`               | VERIFIED | `voice.go` line 83: `downloadToTemp(tgBot, voice.FileId, ".ogg")`; lines 97-100: `context.WithTimeout(context.Background(), 60*time.Second)` + `transcribeVoice` |
| 3   | voice.go enqueues transcript to Claude session via `store.GetOrCreate` + `sess.Enqueue`                   | VERIFIED | `voice.go` line 138: `sess := store.GetOrCreate(chatID, mapping.Path)`; line 196: `sess.Enqueue(qMsg)` — follows HandleText pattern |
| 4   | voice.go starts worker goroutine exactly once per session using `WorkerStarted()` guard and `wg.Add(1)`   | VERIFIED | `voice.go` lines 168-175: `if !sess.WorkerStarted() { sess.SetWorkerStarted(); wg.Add(1); go func(...) { defer wg.Done(); s.Worker(...) }(...) }` |
| 5   | photo.go selects largest PhotoSize via `photos[len(photos)-1]` and downloads to temp with `.jpg` suffix   | VERIFIED | `photo.go` line 95: `largest := photos[len(photos)-1]`; line 98: `downloadToTemp(tgBot, largest.FileId, ".jpg")` |
| 6   | photo.go `buildSinglePhotoPrompt` produces `[Photo: /path]` format                                        | VERIFIED | `photo.go` line 26: `prompt := fmt.Sprintf("[Photo: %s]", path)` — confirmed by `TestBuildSinglePhotoPrompt` PASS |
| 7   | media_group.go `MediaGroupBuffer.Add` resets timer per item using `time.AfterFunc` + `sync.Mutex`         | VERIFIED | `media_group.go` lines 53-54: `b.mu.Lock(); defer b.mu.Unlock()`; lines 70-76: `g.timer.Stop()` then `g.timer = time.AfterFunc(b.timeout, ...)` |
| 8   | photo.go `buildAlbumPrompt` produces `[Photos:\n1. ...\n2. ...]` format                                   | VERIFIED | `photo.go` lines 34-44: `sb.WriteString("[Photos:\n")` + `fmt.Sprintf("%d. %s\n", i+1, p)` — confirmed by `TestBuildAlbumPrompt` PASS |
| 9   | helpers.go `extractPDF` runs `pdftotext -layout <file> -` and returns partial output on non-zero exit     | VERIFIED | `helpers.go` line 164: `cmd := exec.CommandContext(ctx, pdfToTextPath, "-layout", filePath, "-")`; lines 167-170: `if exitErr, ok := err.(*exec.ExitError); ok && len(out) > 0 { return string(out), nil }` |
| 10  | document.go `classifyDocument` routes `.pdf` extension to PDF extraction                                  | VERIFIED | `document.go` lines 28-37: `if ext == ".pdf" { return "pdf" }; if isTextFile(filename) { return "text" }; return "unsupported"` — confirmed by `TestClassifyDocument` 12-case test PASS |
| 11  | helpers.go `textExtensions` has 18-entry extension map; `isTextFile` enforces 10MB and 100K char limits   | VERIFIED | `helpers.go` lines 33-52: `textExtensions` map with 18 entries (.md, .txt, .json, .yaml, .yml, .csv, .xml, .html, .css, .js, .ts, .py, .sh, .env, .log, .cfg, .ini, .toml); lines 26-30: `maxFileSize = 10*1024*1024`, `maxTextChars = 100_000` — confirmed by `TestIsTextFile_Extensions` and `TestConstants_Helpers` PASS |
| 12  | `docs/windows-service.md` exists with 5 sections: Prerequisites, Install, Manage, Env Vars, Troubleshooting | VERIFIED | File confirmed present (190 lines); sections: Prerequisites (line 3), Install Service (line 19), Manage Service (line 89), Environment Variables Reference (line 99), Troubleshooting (line 116) |

**Score:** 12/12 truths verified

### Required Artifacts

| Artifact                               | Expected                                                                   | Status   | Details                                                                                              |
| -------------------------------------- | -------------------------------------------------------------------------- | -------- | ---------------------------------------------------------------------------------------------------- |
| `internal/handlers/voice.go`           | HandleVoice function exists, not a stub (line count > 50)                  | VERIFIED | 248 lines — substantive: OpenAI API key guard, OGG download, 60s transcription timeout, session enqueue |
| `internal/handlers/photo.go`           | HandlePhoto function exists, not a stub (line count > 50)                  | VERIFIED | 408 lines — substantive: single photo and album paths, buildSinglePhotoPrompt, buildAlbumPrompt, MediaGroupBuffer integration |
| `internal/handlers/document.go`        | HandleDocument function exists, not a stub (line count > 50)               | VERIFIED | 365 lines — substantive: classifyDocument, PDF extraction via extractPDF, text file reading, album buffering |
| `internal/handlers/media_group.go`     | MediaGroupBuffer type with Add/flush methods (line count > 30)             | VERIFIED | 92 lines — substantive: pendingGroup struct, MediaGroupBuffer with mu sync.Mutex, Add/fire methods, timer reset on each Add |
| `docs/windows-service.md`              | NSSM documentation with all 5 sections                                     | VERIFIED | 190 lines — covers Prerequisites (build, download NSSM, locate tool paths), Install Service (6 steps), Manage Service (command table), Environment Variables Reference (11 variables), Troubleshooting (4 scenarios) |

**Artifact substantiveness check:**

- `voice.go`: 248 lines — contains HandleVoice (line 41), transcribeVoice delegation (line 100), full worker/enqueue pattern (lines 138-246)
- `photo.go`: 408 lines — contains HandlePhoto (line 61), buildSinglePhotoPrompt (line 25), buildAlbumPrompt (line 33), sendPhotoToSession (line 192), sendAlbumToSession (line 299)
- `document.go`: 365 lines — contains HandleDocument (line 79), classifyDocument (line 28), sendDocToSession (line 264), makeDocAlbumProcessor (line 233)
- `media_group.go`: 92 lines — contains MediaGroupBuffer (line 27), NewMediaGroupBuffer (line 40), Add (line 52), fire (line 81)
- `docs/windows-service.md`: 190 lines — full NSSM installation guide with code blocks for every step

### Key Link Verification

| From                              | To                                        | Via                                                              | Status | Details                                                                                                          |
| --------------------------------- | ----------------------------------------- | ---------------------------------------------------------------- | ------ | ---------------------------------------------------------------------------------------------------------------- |
| `internal/bot/handlers.go`        | `internal/handlers/voice.go`              | `handlers.NewMessage(message.Voice, b.handleVoice)` line 38      | WIRED  | `bot/handlers.go` line 38: `dispatcher.AddHandler(handlers.NewMessage(message.Voice, b.handleVoice))` |
| `internal/bot/handlers.go`        | `internal/handlers/photo.go`              | `handlers.NewMessage(message.Photo, b.handlePhoto)` line 39      | WIRED  | `bot/handlers.go` line 39: `dispatcher.AddHandler(handlers.NewMessage(message.Photo, b.handlePhoto))` |
| `internal/bot/handlers.go`        | `internal/handlers/document.go`           | `handlers.NewMessage(message.Document, b.handleDocument)` line 40 | WIRED  | `bot/handlers.go` line 40: `dispatcher.AddHandler(handlers.NewMessage(message.Document, b.handleDocument))` |
| Bot wrapper functions              | Full parameter set including Phase 6 safety params | `auditLog`, `persist`, `WaitGroup()`, `globalAPILimiter` all passed | WIRED  | `bot/handlers.go` lines 56-68: all three wrappers pass `b.store, b.cfg, b.auditLog, b.persist, b.WaitGroup(), b.mappings, b.globalAPILimiter` — Phase 6 safety parameters included |

All four key links: WIRED.

### Requirements Coverage

| Requirement | Source Plan | Description                                                                    | Status                | Evidence                                                                                                   |
| ----------- | ----------- | ------------------------------------------------------------------------------ | --------------------- | ---------------------------------------------------------------------------------------------------------- |
| MEDIA-01    | 03-01-PLAN  | User can send voice messages; bot transcribes via OpenAI Whisper and processes as text | SATISFIED       | `voice.go`: API key guard (line 64), OGG download (line 83), 60s timeout + transcription (lines 97-100), session enqueue (line 196); `TestHandleVoice_NoAPIKey` PASS |
| MEDIA-02    | 03-02-PLAN  | User can send photos; bot forwards to Claude for visual analysis                | SATISFIED             | `photo.go`: largest photo selection (line 95), temp download (line 98), `buildSinglePhotoPrompt` (line 26), session enqueue; `TestBuildSinglePhotoPrompt` PASS |
| MEDIA-03    | 03-02-PLAN  | Bot buffers photo albums (media groups) with a timeout before sending as a batch | SATISFIED            | `media_group.go`: `MediaGroupBuffer` with timer reset (lines 70-76); `photo.go`: `buildAlbumPrompt` (line 34); `TestMediaGroupBuffer_MultipleItems` PASS, `TestBuildAlbumPrompt` PASS |
| MEDIA-04    | 03-01-PLAN  | User can send PDF documents; bot extracts text via pdftotext and sends to Claude | SATISFIED             | `helpers.go` `extractPDF` (line 163): `pdftotext -layout` invocation, partial extraction on non-zero exit; `document.go` `classifyDocument` routes `.pdf`; `TestExtractPDF_Success` PASS, `TestClassifyDocument` PASS |
| MEDIA-05    | 03-03-PLAN  | User can send text/code files as documents; bot reads content and sends to Claude | SATISFIED            | `helpers.go` `textExtensions` 18-entry map (lines 33-52), `maxFileSize=10*1024*1024`, `maxTextChars=100_000`; `document.go` text reading path (lines 170-176); `TestIsTextFile_Extensions` PASS, `TestConstants_Helpers` PASS |
| DEPLOY-02   | 03-04-PLAN  | Bot installs as a Windows Service (runs at boot, no terminal window)            | SATISFIED (HUMAN_NEEDED for operational verification) | `docs/windows-service.md` (190 lines) covers all NSSM steps from installation to log configuration; operational verification requires a Windows machine with NSSM |

### Anti-Patterns Found

Scanned files: `internal/handlers/voice.go`, `internal/handlers/photo.go`, `internal/handlers/document.go`, `internal/handlers/helpers.go`, `internal/handlers/media_group.go`.

| File            | Line | Pattern | Severity | Impact  |
| --------------- | ---- | ------- | -------- | ------- |
| None found      | —    | —       | —        | —       |

No TODOs, FIXMEs, placeholder returns, stub handlers, or console-log-only implementations detected. All five source files are substantive implementations. One technical note: `helpers.go` line 169 contains `_ = exitErr` (assigned to blank identifier) as a deliberate pattern to acknowledge the exit error while using `len(out) > 0` for the partial extraction decision — this is correct behavior, not a stub or suppression of real errors.

### Human Verification Required

#### 1. NSSM Service Installs and Starts at Boot (DEPLOY-02)

**Test:** On a Windows machine, follow `docs/windows-service.md` to install the bot as a Windows Service via NSSM. Reboot the machine and confirm the service starts automatically without a user login.
**Expected:** `nssm status ClaudeTelegramBot` shows RUNNING after boot. Bot responds to Telegram messages.
**Why human:** Requires Windows with NSSM installed, Administrator privileges, and a physical/virtual machine reboot. Cannot be exercised by unit tests.

#### 2. Voice Message Transcription End-to-End (MEDIA-01)

**Test:** With OPENAI_API_KEY configured, send a voice message to the bot from Telegram. Observe the "Transcribing..." status message, then the transcript display, then Claude's response.
**Expected:** Bot transcribes the voice message accurately and routes the transcript to Claude as if it were a text message. A response with action buttons appears.
**Why human:** Requires a live Telegram connection, real OpenAI Whisper API call, and a running bot with configured credentials. The unit test (`TestTranscribeVoice_Success`) uses a mock HTTP server.

#### 3. Photo Album Rendering in Claude (MEDIA-03)

**Test:** Send 2-3 photos in a single Telegram album (media group) to the bot. Wait ~1 second for the buffer to fire.
**Expected:** Bot receives all photos as a batch, formats them as `[Photos:\n1. /tmp/...\n2. /tmp/...]` and sends that combined prompt to Claude. A single streaming response covers all photos.
**Why human:** Requires live Telegram message sequencing, 1-second real-time timer behavior, and actual file downloads. The unit test (`TestMediaGroupBuffer_MultipleItems`) uses in-memory paths without Telegram.

---

## Build and Test Results

```
go build ./...   → exit 0 (all packages compile)
go test ./...    → all 9 packages PASS (no failures)

Packages tested:
  ?    github.com/user/gsd-tele-go          [no test files]
  ok   github.com/user/gsd-tele-go/internal/audit       1.148s
  ok   github.com/user/gsd-tele-go/internal/bot         1.556s
  ok   github.com/user/gsd-tele-go/internal/claude      2.415s
  ok   github.com/user/gsd-tele-go/internal/config      1.249s
  ok   github.com/user/gsd-tele-go/internal/formatting  1.110s
  ok   github.com/user/gsd-tele-go/internal/handlers    3.171s
  ok   github.com/user/gsd-tele-go/internal/project     1.280s
  ok   github.com/user/gsd-tele-go/internal/security    1.163s
  ok   github.com/user/gsd-tele-go/internal/session     2.444s

go test ./internal/handlers/... -v -count=1 (targeted handler suite):
  TestHandleVoice_NoAPIKey                    PASS
  TestHandleVoice_FunctionExists              PASS
  TestTranscribeVoice_Success                 PASS
  TestTranscribeVoice_AuthError               PASS
  TestExtractPDF_Success                      PASS
  TestExtractPDF_CommandError                 PASS
  TestDownloadToTemp_Success                  PASS
  TestIsTextFile                              PASS
  TestIsTextFile_Extensions                   PASS
  TestConstants_Helpers                       PASS
  TestMediaGroupBuffer_SingleItem             PASS
  TestMediaGroupBuffer_MultipleItems          PASS
  TestMediaGroupBuffer_IndependentGroups      PASS
  TestMediaGroupBuffer_FirstCaptionWins       PASS
  TestMediaGroupBuffer_EmptyCaptionSkipped    PASS
  TestMediaGroupBuffer_ConcurrentAdd          PASS
  TestBuildSinglePhotoPrompt                  PASS
  TestBuildAlbumPrompt                        PASS
  TestHandlePhoto_FunctionExists              PASS
  TestClassifyDocument                        PASS
  TestBuildDocumentPrompt                     PASS
  TestTruncateText                            PASS
  TestSupportedExtensionsList                 PASS
  (+ 54 pre-existing callback/command/text/session handler tests PASS)

Total: 77 tests PASS, 0 FAIL
```

## Gaps Summary

No automated gaps found. All twelve must-have truths are satisfied by line-by-line source inspection and live test suite results. All six requirements are SATISFIED by implemented code with unit test evidence.

Three items require human verification (DEPLOY-02 operational, MEDIA-01 end-to-end transcription, MEDIA-03 album buffering timing) — these are inherently live-bot behaviors that cannot be exercised by unit tests. They are documented in the Human Verification Required section above and align exactly with the manual-only items listed in `03-VALIDATION.md`.

**Note on observable truth count:** The plan specified minimum 12 Observable Truths rows. This report contains exactly 12 rows, all VERIFIED, covering all six requirements with multiple truths per requirement.

---

_Verified: 2026-03-20_
_Verifier: Claude (gsd-executor phase 07-01)_
