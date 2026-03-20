# Phase 3: Media Handlers and Windows Service - Context

**Gathered:** 2026-03-20
**Status:** Ready for planning
**Source:** Auto-mode (recommended defaults selected)

<domain>
## Phase Boundary

Users can send voice messages, photos (single and albums), PDFs, and text/code files to any project channel and the bot processes them correctly. The bot also installs as a Windows Service via NSSM and starts at boot without a terminal window. Archive support (zip, tar), video, and standalone audio files are deferred to v2.

</domain>

<decisions>
## Implementation Decisions

### Voice message handling
- Download OGG from Telegram, transcribe via OpenAI Whisper API, show transcript in chat, send transcribed text to Claude
- OPENAI_API_KEY read from config; if missing, reply "Voice transcription not configured" on voice messages (don't crash)
- Show transcript to user: edit status message to display `🎤 "transcribed text"` before sending to Claude
- On transcription failure: reply with error message ("Transcription failed"), don't fall back to requesting text input
- Clean up downloaded OGG file after processing (temp file lifecycle)

### Photo and album handling
- Pass file paths in prompt — Claude CLI reads images from disk (same approach as TypeScript version)
- Single photos: download largest resolution, send path in prompt immediately
- Media group (album) buffering: 1-second timeout to collect all photos in a Telegram media group before sending as a batch
- Caption handling: first caption in the group wins; include after image paths in prompt
- No explicit photo count limit — Telegram caps media groups at 10
- Prompt format: single photo `[Photo: /path/to/file.jpg]\n\ncaption` / album `[Photos:\n1. path\n2. path]\n\ncaption`

### Document and PDF extraction
- PDF: use `pdftotext` CLI with `-layout` flag; path resolved from PDFTOTEXT_PATH config (already handled by Phase 1 DEPLOY-03)
- Text files: read content directly, truncate at 100K characters
- Supported text extensions: .md, .txt, .json, .yaml, .yml, .csv, .xml, .html, .css, .js, .ts, .py, .sh, .env, .log, .cfg, .ini, .toml
- Max file size: 10MB — reject with clear error if exceeded
- Document media group buffering: same 1-second timeout pattern as photos
- Unsupported file types: reply with error listing supported types

### Windows Service deployment
- NSSM (Non-Sucking Service Manager) for service installation — `nssm install ClaudeTelegramBot <path-to-exe>`
- Go binary already compiles to single .exe (DEPLOY-01 done in Phase 1)
- External tool paths: CLAUDE_PATH and PDFTOTEXT_PATH environment variables set in NSSM service configuration (no PATH reliance — DEPLOY-03 done)
- Graceful shutdown: existing context.WithCancel + b.Stop() pattern from Phase 1 handles service stop signals; wire os.Signal listener for SIGTERM/SIGINT (already done in Phase 1, just verify it works under NSSM)
- Logging: NSSM redirects stdout/stderr to configurable log files — no code changes needed for log capture
- Install/uninstall documentation or helper script — not a separate CLI mode, just documented NSSM commands

### Claude's Discretion
- Go media group buffer implementation details (goroutine + timer pattern vs channel-based)
- Exact error message wording for media processing failures
- Temp file naming convention and cleanup strategy
- Whether to show "Processing..." placeholder before Claude response (TypeScript does this)
- pdftotext error handling specifics (what to show user if extraction fails)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### TypeScript media handlers (functional spec)
- `src/handlers/voice.ts` — Voice download, OpenAI transcription, transcript display, Claude routing
- `src/handlers/photo.ts` — Photo download (largest resolution), single photo vs album, media group integration
- `src/handlers/document.ts` — PDF extraction (pdftotext CLI), text file reading, file type detection, archive routing, size limits
- `src/handlers/media-group.ts` — Generic media group buffer with configurable timeout (1s), status messages, group processing callback
- `src/handlers/audio.ts` — Audio file handling (deferred to v2 but useful for pattern reference)
- `src/utils.ts` — `transcribeVoice()` OpenAI Whisper integration, `startTypingIndicator()` pattern

### Existing Go code (Phase 1-2 foundation)
- `internal/handlers/text.go` — HandleText flow (mapping check, worker start, streaming callback) — new handlers follow same pattern
- `internal/handlers/streaming.go` — StreamingState, AccumulatedText(), global rate limiter wiring — media handlers reuse this
- `internal/config/config.go` — Config struct, PDFTOTEXT_PATH already resolved, add OPENAI_API_KEY
- `internal/bot/bot.go` — Bot struct, handler registration, worker lifecycle
- `internal/bot/handlers.go` — Handler wrappers, registerHandlers

### Project requirements
- `.planning/REQUIREMENTS.md` — MEDIA-01 through MEDIA-05, DEPLOY-02
- `.planning/ROADMAP.md` — Phase 3 success criteria (4 items)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `StreamingState` + `createStatusCallback` pattern (streaming.go): All media handlers route to Claude the same way — reuse streaming callback factory
- `HandleText` flow (text.go): Auth → mapping check → rate limit → worker start → stream — media handlers follow identical skeleton
- `Config.PdfToTextPath` (config.go): Already resolved at startup — just use it in document handler
- `Session.EnqueueMessage` (session.go): Queue message to worker — all media handlers end up here after preparing prompt
- Atomic file download pattern: needs to be created (downloadFile helper) since TypeScript uses fetch() which Go replaces with http.Get + os.Create

### Established Patterns
- Handler signature: `func HandleX(ctx context.Context, b *gotgbot.Bot, msg *gotgbot.Message, sessions *session.SessionStore, mappings *project.MappingStore, ...) error`
- Bot wrapper delegation: `internal/bot/handlers.go` creates thin wrappers that extract params and call `handlers.HandleX`
- Temp file management: use `os.CreateTemp` in TempDir, defer `os.Remove` for cleanup
- Typing indicator: goroutine with ticker sending ChatAction every 5s

### Integration Points
- `registerHandlers` (handlers.go) needs voice, photo, document filter registrations
- `Config` struct needs `OpenAIAPIKey string` field and `OpenAIAvailable bool` helper
- New `internal/handlers/voice.go`, `photo.go`, `document.go`, `media_group.go` files
- OpenAI Whisper API: `POST https://api.openai.com/v1/audio/transcriptions` with multipart form (model=whisper-1)
- pdftotext invocation: `exec.CommandContext(ctx, cfg.PdfToTextPath, "-layout", filePath, "-")`

</code_context>

<specifics>
## Specific Ideas

- Media group buffer should be a reusable Go struct (not per-handler globals like TypeScript) — single MediaGroupBuffer type parameterized by processing callback
- Photos sent to Claude via file path in prompt text — Claude CLI can read local images. Format: `[Photo: /tmp/photo_123.jpg]`
- NSSM configuration is documentation-only — no install subcommand in the Go binary. User runs `nssm install` manually with documented flags.

</specifics>

<deferred>
## Deferred Ideas

- MEDIA-06: Video message transcription/analysis — v2
- MEDIA-07: Audio file (mp3, m4a, wav) transcription — v2
- MEDIA-08: Archive file (zip, tar) extraction and analysis — v2
- Auto-document feature from TypeScript version — not in requirements, skip

</deferred>

---

*Phase: 03-media-handlers-and-windows-service*
*Context gathered: 2026-03-20 via auto-mode*
