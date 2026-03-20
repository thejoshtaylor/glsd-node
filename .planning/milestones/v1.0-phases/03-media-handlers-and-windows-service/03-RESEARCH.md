# Phase 3: Media Handlers and Windows Service - Research

**Researched:** 2026-03-20
**Domain:** Go media handler implementation (voice/photo/document) + NSSM Windows Service deployment
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- Voice messages: download OGG from Telegram, transcribe via OpenAI Whisper API, show transcript in chat, send transcribed text to Claude
- OPENAI_API_KEY read from config; if missing, reply "Voice transcription not configured" (don't crash)
- Show transcript to user: edit status message to `Transcribed: "transcribed text"` before sending to Claude
- On transcription failure: reply with error message ("Transcription failed"), no fallback
- Clean up downloaded OGG file after processing (temp file lifecycle)
- Photos: pass file paths in prompt — Claude CLI reads images from disk
- Single photos: download largest resolution, send path in prompt immediately
- Media group (album) buffering: 1-second timeout to collect all photos before sending as a batch
- Caption handling: first caption in the group wins; include after image paths in prompt
- No explicit photo count limit — Telegram caps media groups at 10
- Prompt format: single photo `[Photo: /path/to/file.jpg]\n\ncaption` / album `[Photos:\n1. path\n2. path]\n\ncaption`
- PDF: use `pdftotext` CLI with `-layout` flag; path resolved from PDFTOTEXT_PATH config
- Text files: read content directly, truncate at 100K characters
- Supported text extensions: .md, .txt, .json, .yaml, .yml, .csv, .xml, .html, .css, .js, .ts, .py, .sh, .env, .log, .cfg, .ini, .toml
- Max file size: 10MB — reject with clear error if exceeded
- Document media group buffering: same 1-second timeout pattern as photos
- Unsupported file types: reply with error listing supported types
- NSSM for service installation — `nssm install ClaudeTelegramBot <path-to-exe>`
- External tool paths: CLAUDE_PATH and PDFTOTEXT_PATH environment variables set in NSSM service config (no PATH reliance)
- Graceful shutdown: existing context.WithCancel + bot.Stop() pattern handles service stop signals
- Logging: NSSM redirects stdout/stderr to configurable log files — no code changes needed
- Install/uninstall documentation or helper script — not a separate CLI mode, just documented NSSM commands

### Claude's Discretion

- Go media group buffer implementation details (goroutine + timer pattern vs channel-based)
- Exact error message wording for media processing failures
- Temp file naming convention and cleanup strategy
- Whether to show "Processing..." placeholder before Claude response
- pdftotext error handling specifics

### Deferred Ideas (OUT OF SCOPE)

- MEDIA-06: Video message transcription/analysis
- MEDIA-07: Audio file (mp3, m4a, wav) transcription
- MEDIA-08: Archive file (zip, tar) extraction and analysis
- Auto-document feature from TypeScript version
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| MEDIA-01 | User can send voice messages; bot transcribes via OpenAI Whisper and processes as text | OpenAI `/v1/audio/transcriptions` POST with multipart form; `whisper-1` model; OGG is supported format |
| MEDIA-02 | User can send photos; bot forwards to Claude for visual analysis | gotgbot `msg.Photo []PhotoSize` (largest = last); `GetFile` + HTTP download; file path in prompt text |
| MEDIA-03 | Bot buffers photo albums (media groups) with a timeout before sending as a batch | `msg.MediaGroupId` string field; `time.AfterFunc(1*time.Second, ...)` goroutine pattern; sync.Mutex-protected map |
| MEDIA-04 | User can send PDF documents; bot extracts text via pdftotext and sends to Claude | `exec.CommandContext(ctx, cfg.PdfToTextPath, "-layout", filePath, "-")`; output captured via `cmd.Output()` |
| MEDIA-05 | User can send text/code files as documents; bot reads content and sends to Claude | `os.ReadFile` + slice to 100K chars; MIME or extension detection |
| DEPLOY-02 | Bot installs as a Windows Service (runs at boot, no terminal window) | NSSM `install`, `set AppEnvironmentExtra`, `set AppStdout/AppStderr`; documented command set; no binary changes needed |
</phase_requirements>

---

## Summary

Phase 3 extends the Go bot with four new handler files (`voice.go`, `photo.go`, `document.go`, `media_group.go`) and one config field addition (`PdfToTextPath`). All handlers follow the identical skeleton established in `HandleText`: auth + mapping check (already done by middleware) → download → prepare prompt → `session.Enqueue` with `CreateStatusCallback`. The TypeScript source files serve as a functional spec with direct mapping to Go equivalents.

The OpenAI Whisper API is a simple `multipart/form-data POST` to `https://api.openai.com/v1/audio/transcriptions` with `file` (the OGG bytes) and `model=whisper-1`. Go's standard `mime/multipart` package handles this without any additional library. The response is JSON `{"text": "..."}`.

NSSM (Non-Sucking Service Manager) is the locked deployment tool. It wraps the Go `.exe` as a Windows Service and manages log redirection. Zero code changes are needed — NSSM is configured entirely via documented command-line invocations. The critical detail is using `nssm set AppEnvironmentExtra` to inject `CLAUDE_CLI_PATH` and `PDFTOTEXT_PATH` without touching system PATH.

**Primary recommendation:** Implement `MediaGroupBuffer` as a reusable struct with a `sync.Mutex`-protected map and `time.AfterFunc` timers; have all three media handlers (`voice.go`, `photo.go`, `document.go`) call `session.Enqueue` exactly as `HandleText` does.

---

## Standard Stack

### Core (already in go.mod — no new dependencies needed)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.34 | Telegram bot; message filter constants (`message.Voice`, `message.Photo`, `message.Document`); `GetFile` API | Already in use |
| `net/http` | stdlib | Download files from Telegram CDN; POST to OpenAI Whisper API | No new dep |
| `mime/multipart` | stdlib | Build multipart form for Whisper API upload | No new dep |
| `os/exec` | stdlib | Invoke `pdftotext` CLI subprocess | No new dep |
| `time` | stdlib | `time.AfterFunc` for media group 1-second buffer timeout | No new dep |
| `sync` | stdlib | `sync.Mutex` for media group buffer map | No new dep |
| `os` | stdlib | `os.CreateTemp`, `os.Remove`, `os.ReadFile` for temp file management | No new dep |

### No New Dependencies

The entire Phase 3 implementation uses only the Go standard library and the existing `gotgbot/v2` dependency. There is no OpenAI Go SDK — the Whisper API is called directly with `net/http` + `mime/multipart`. This keeps the binary lean and avoids dependency churn.

**Verification:** `go mod tidy` after implementation should show no changes to `go.mod` or `go.sum`.

### NSSM (external deployment tool — not a Go dependency)

| Tool | Version | Source | Purpose |
|------|---------|--------|---------|
| NSSM | 2.24 (latest stable) | https://nssm.cc/download | Windows Service wrapper — no installer; single `.exe` |

---

## Architecture Patterns

### New Files

```
internal/handlers/
├── voice.go         # MEDIA-01: HandleVoice
├── photo.go         # MEDIA-02 + MEDIA-03: HandlePhoto + album buffering
├── document.go      # MEDIA-04 + MEDIA-05: HandleDocument + PDF/text extraction
└── media_group.go   # Shared MediaGroupBuffer struct used by photo.go + document.go
```

Config change:
```
internal/config/config.go  # Add PdfToTextPath field + PDFTOTEXT_PATH env var
```

Handler registration:
```
internal/bot/handlers.go   # Register voice, photo, document handler wrappers
```

### Pattern 1: Handler Skeleton (identical to HandleText)

Every new handler follows this sequence exactly:

```go
// Source: internal/handlers/text.go established pattern
func HandleVoice(tgBot *gotgbot.Bot, ctx *ext.Context, store *session.SessionStore,
    cfg *config.Config, auditLog *audit.Logger, persist *session.PersistenceManager,
    wg *sync.WaitGroup, mappings *project.MappingStore) error {

    // 1. Extract message fields
    msg := ctx.EffectiveMessage
    chatID := ctx.EffectiveChat.Id
    userID := ctx.EffectiveSender.Id()

    // 2. Mapping check (middleware already handled auth + rate limit)
    mapping, hasMapped := mappings.Get(chatID)
    if !hasMapped {
        _, err := tgBot.SendMessage(chatID, "No project linked. Send /project to link one.", nil)
        return err
    }

    // 3. Download file
    // 4. Prepare prompt text
    // 5. Ensure worker running (identical to HandleText)
    // 6. Enqueue with CreateStatusCallback
    // 7. Handle ErrCh asynchronously (identical to HandleText)
}
```

**Key insight:** Handlers do NOT need auth checks because the auth middleware in group -2 already runs before any handler. The mapping check IS needed (HandleText does it inline).

### Pattern 2: File Download Helper

```go
// downloadToTemp downloads a Telegram file by file_id to a temp file.
// Returns the local path. Caller is responsible for os.Remove.
func downloadToTemp(tgBot *gotgbot.Bot, fileID string, suffix string) (string, error) {
    file, err := tgBot.GetFile(fileID, nil)
    if err != nil {
        return "", fmt.Errorf("GetFile: %w", err)
    }
    url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", tgBot.Token, file.FilePath)
    resp, err := http.Get(url)
    if err != nil {
        return "", fmt.Errorf("download: %w", err)
    }
    defer resp.Body.Close()

    tmp, err := os.CreateTemp("", "tg_*"+suffix)
    if err != nil {
        return "", err
    }
    defer tmp.Close()
    if _, err := io.Copy(tmp, resp.Body); err != nil {
        os.Remove(tmp.Name())
        return "", err
    }
    return tmp.Name(), nil
}
```

Callers do `defer os.Remove(path)` immediately after a successful download.

### Pattern 3: MediaGroupBuffer (reusable struct)

```go
// Source: TypeScript media-group.ts mapped to Go idioms
type pendingGroup struct {
    paths   []string
    ctx     *ext.Context       // first message's context (for reply)
    caption string
    timer   *time.Timer
}

type MediaGroupBuffer struct {
    mu       sync.Mutex
    groups   map[string]*pendingGroup   // media_group_id -> pending
    timeout  time.Duration
    process  func(ctx *ext.Context, paths []string, caption string)
}

func NewMediaGroupBuffer(timeout time.Duration,
    process func(*ext.Context, []string, string)) *MediaGroupBuffer {
    return &MediaGroupBuffer{
        groups:  make(map[string]*pendingGroup),
        timeout: timeout,
        process: process,
    }
}

func (b *MediaGroupBuffer) Add(groupID string, path string, ctx *ext.Context, caption string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    g, ok := b.groups[groupID]
    if !ok {
        g = &pendingGroup{ctx: ctx, caption: caption}
        b.groups[groupID] = g
    }
    g.paths = append(g.paths, path)
    if caption != "" && g.caption == "" {
        g.caption = caption  // first caption wins
    }
    if g.timer != nil {
        g.timer.Stop()
    }
    captured := groupID
    g.timer = time.AfterFunc(b.timeout, func() {
        b.fire(captured)
    })
}

func (b *MediaGroupBuffer) fire(groupID string) {
    b.mu.Lock()
    g, ok := b.groups[groupID]
    if !ok {
        b.mu.Unlock()
        return
    }
    delete(b.groups, groupID)
    b.mu.Unlock()
    b.process(g.ctx, g.paths, g.caption)
}
```

**Instance per handler:** `photo.go` and `document.go` each hold a package-level `*MediaGroupBuffer` initialized at startup.

### Pattern 4: OpenAI Whisper API Call (no SDK)

```go
// Source: OpenAI API docs + TypeScript utils.ts transcribeVoice()
func transcribeVoice(ctx context.Context, apiKey string, filePath string) (string, error) {
    f, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer f.Close()

    var body bytes.Buffer
    w := multipart.NewWriter(&body)
    fw, _ := w.CreateFormFile("file", filepath.Base(filePath))
    io.Copy(fw, f)
    w.WriteField("model", "whisper-1")
    w.Close()

    req, _ := http.NewRequestWithContext(ctx, "POST",
        "https://api.openai.com/v1/audio/transcriptions", &body)
    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", w.FormDataContentType())

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("whisper API %d: %s", resp.StatusCode, b)
    }

    var result struct {
        Text string `json:"text"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    return result.Text, nil
}
```

### Pattern 5: pdftotext Invocation

```go
// Source: TypeScript document.ts extractText() + CONTEXT.md decision
func extractPDF(ctx context.Context, pdfToTextPath string, filePath string) (string, error) {
    cmd := exec.CommandContext(ctx, pdfToTextPath, "-layout", filePath, "-")
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("pdftotext: %w", err)
    }
    return string(out), nil
}
```

The `-` argument at the end sends output to stdout. `cmd.Output()` captures stdout; stderr is discarded (pdftotext version noise is irrelevant).

### Pattern 6: Photo Largest Resolution Selection

Telegram sends `msg.Photo` as `[]PhotoSize` sorted by ascending size. The largest is always the last element:

```go
photos := ctx.EffectiveMessage.Photo   // []gotgbot.PhotoSize
largest := photos[len(photos)-1]       // largest = last
fileID := largest.FileId
```

### Pattern 7: NSSM Service Installation

Complete installation sequence (documentation artifact, not code):

```cmd
# 1. Install service (run as Administrator)
nssm install ClaudeTelegramBot "C:\path\to\claude-telegram-bot.exe"

# 2. Set working directory
nssm set ClaudeTelegramBot AppDirectory "C:\path\to\bot"

# 3. Set environment variables (no PATH dependency)
nssm set ClaudeTelegramBot AppEnvironmentExtra CLAUDE_CLI_PATH="C:\Users\user\AppData\Roaming\npm\claude.cmd" PDFTOTEXT_PATH="C:\poppler\bin\pdftotext.exe" TELEGRAM_BOT_TOKEN="..." TELEGRAM_ALLOWED_USERS="..." OPENAI_API_KEY="..."

# 4. Configure log files
nssm set ClaudeTelegramBot AppStdout "C:\logs\claude-bot.log"
nssm set ClaudeTelegramBot AppStderr "C:\logs\claude-bot.err"

# 5. Start service
nssm start ClaudeTelegramBot

# Uninstall:
nssm stop ClaudeTelegramBot
nssm remove ClaudeTelegramBot confirm
```

**Critical:** `AppEnvironmentExtra` adds to the inherited service environment without replacing it. This is preferred over `AppEnvironment` which would replace system env entirely.

### Anti-Patterns to Avoid

- **Handler auth re-check:** Do NOT add auth checks inside voice/photo/document handlers. The auth middleware in dispatcher group -2 already rejects unauthorized users before any handler runs. Duplicating the check adds latency and complexity.
- **Global HTTP client timeout:** Do NOT use `http.DefaultClient` with no timeout for file downloads. A large file or slow network would block the goroutine indefinitely. Use `http.Client{Timeout: 30*time.Second}` or pass a context.
- **MediaGroupBuffer as globals in each handler:** Do NOT copy the TypeScript pattern of module-level `pendingGroups` maps. Go handlers share package scope; use a struct so the buffer is passed through the call chain and is testable.
- **Blocking the handler goroutine:** After `session.Enqueue`, DO NOT wait on `ErrCh` in the handler goroutine. Drain it in a separate goroutine (identical to HandleText pattern), or the Telegram dispatcher update loop stalls.
- **pdftotext PATH reliance:** Never call `exec.Command("pdftotext", ...)` — always use `cfg.PdfToTextPath` (DEPLOY-03 requirement, already established).

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Streaming Claude response during media handling | Custom streaming loop | Reuse `NewStreamingState` + `CreateStatusCallback` from `streaming.go` | Already battle-tested with throttling, multi-segment, MarkdownV2 |
| Worker lifecycle for media messages | New worker start logic | Reuse `sess.WorkerStarted()` + `sess.Enqueue(QueuedMessage{...})` exactly as HandleText | Guards against double-start, handles shutdown |
| Typing indicator | Goroutine with ticker | Reuse `StartTypingIndicator(tgBot, chatID)` from `streaming.go` | Already correct; sends every 4s |
| Temp directory | Custom dir creation | Use `os.TempDir()` (returns `%TEMP%` on Windows) | NSSM service has access to `%TEMP%` by default |
| HTTP retry logic | Custom retry wrapper | None — use single attempt with context timeout | Bot messages have a user waiting; retry hides real errors |

---

## Common Pitfalls

### Pitfall 1: Voice File Path — Windows Temp Dir

**What goes wrong:** `os.CreateTemp("", "voice_*.ogg")` creates a file in `os.TempDir()` which on Windows is `%TEMP%` (e.g., `C:\Users\user\AppData\Local\Temp`). When running as a Windows Service via NSSM, `%TEMP%` resolves to the SYSTEM account's temp dir (`C:\Windows\Temp`) rather than the user's temp dir.

**Why it happens:** NSSM services run as `SYSTEM` by default. The `%TEMP%` environment variable for SYSTEM is different from interactive user sessions.

**How to avoid:** Either (a) configure NSSM to run as the user account (`nssm set AppUser username`), or (b) make the temp dir configurable via `TEMP_DIR` env var with `os.TempDir()` as fallback, documented clearly.

**Warning signs:** File not found errors in document/voice handlers only when running as service.

### Pitfall 2: MediaGroupBuffer Timer Goroutine Leak

**What goes wrong:** If `time.AfterFunc` fires after the bot shuts down, it calls `b.process()` with a `*ext.Context` whose underlying Telegram connection is gone. The `tgBot.SendMessage` calls return errors, but the goroutine only exits once those calls complete.

**Why it happens:** `time.AfterFunc` timers are not cancelled on shutdown.

**How to avoid:** Check context liveness in the `fire()` callback, or track all timers and call `timer.Stop()` in a `Close()` method called during graceful shutdown. Alternatively, ignore Telegram API errors in the timer callback (errors.Is check) — the goroutine exits promptly even if API calls fail.

**Warning signs:** "Failed to send message" log lines appearing after bot shutdown log.

### Pitfall 3: Photo []PhotoSize Nil Check

**What goes wrong:** `msg.Photo` is `[]PhotoSize` (not a pointer). If the message filter is correct, `len(msg.Photo) > 0` is guaranteed — but the handler should still guard because `gotgbot` passes the struct regardless.

**Why it happens:** TypeScript used optional chaining; Go requires explicit length check.

**How to avoid:** Always check `len(ctx.EffectiveMessage.Photo) == 0` at the top of `HandlePhoto` and return nil.

### Pitfall 4: pdftotext Exit Code on Some PDFs

**What goes wrong:** `pdftotext` returns exit code 1 on encrypted/corrupted PDFs. `cmd.Output()` wraps this in an `*exec.ExitError`. The extracted text may be partial (written to stdout before the error).

**Why it happens:** pdftotext writes partial output before detecting encryption.

**How to avoid:** Use `cmd.Output()` and capture the error. On `*exec.ExitError`, still check if `out` is non-empty — if it has content, use it. Otherwise report "Failed to extract PDF text" to user.

### Pitfall 5: OpenAI Whisper Timeout

**What goes wrong:** Transcription of long voice messages can take 10-30 seconds. `http.DefaultClient` has no timeout, so a stalled OpenAI response blocks the goroutine indefinitely.

**Why it happens:** Default Go HTTP client has no timeout.

**How to avoid:** Use `http.NewRequestWithContext(ctx, ...)` where `ctx` is derived from the handler context, giving a natural deadline. Add an explicit `context.WithTimeout` of 60 seconds as a safety net.

### Pitfall 6: NSSM AppEnvironmentExtra Quoting

**What goes wrong:** Values with spaces in NSSM `set AppEnvironmentExtra` need proper quoting. Getting this wrong silently passes truncated or empty values.

**Why it happens:** NSSM parses the multi-value argument differently from the Windows shell.

**How to avoid:** Use the NSSM GUI (Application > Environment tab) or set via registry `REG_MULTI_SZ` directly for values containing spaces. Document this clearly with an example using a path without spaces if possible.

### Pitfall 7: Config Missing PdfToTextPath Field

**What goes wrong:** The CONTEXT.md says DEPLOY-03 already resolved PDFTOTEXT_PATH — but inspection of `internal/config/config.go` confirms **no `PdfToTextPath` field exists yet** in the Config struct. The field and env var parsing must be added in Phase 3.

**Why it matters:** The document handler will panic or fail silently without this field.

**How to avoid:** Add `PdfToTextPath string` to `Config` and parse `PDFTOTEXT_PATH` env var in `Load()`, logging the resolved path at startup (identical to `ClaudeCLIPath` logging pattern).

---

## Code Examples

### Register Media Handlers in handlers.go

```go
// Source: internal/bot/handlers.go established pattern
dispatcher.AddHandler(handlers.NewMessage(message.Voice, b.handleVoice))
dispatcher.AddHandler(handlers.NewMessage(message.Photo, b.handlePhoto))
dispatcher.AddHandler(handlers.NewMessage(message.Document, b.handleDocument))
```

Bot wrapper methods follow the identical thin-wrapper pattern:

```go
func (b *Bot) handleVoice(tgBot *gotgbot.Bot, ctx *ext.Context) error {
    return bothandlers.HandleVoice(tgBot, ctx, b.store, b.cfg, b.auditLog, b.persist, b.WaitGroup(), b.mappings)
}
```

### Config Addition

```go
// internal/config/config.go
type Config struct {
    // ... existing fields ...
    PdfToTextPath string  // resolved path to pdftotext CLI binary
    OpenAIAPIKey  string  // already present, confirmed in code
}

// In Load():
cfg.PdfToTextPath = os.Getenv("PDFTOTEXT_PATH")
// No fallback — document handler checks cfg.PdfToTextPath == "" and replies with error
log.Info().Str("pdftotext_path", cfg.PdfToTextPath).Msg("pdftotext path configured")
```

### Enqueueing from Media Handler (identical to HandleText)

```go
// After preparing prompt string `promptText`:
ss := NewStreamingState(tgBot, chatID, globalLimiter)
typingCtl := StartTypingIndicator(tgBot, chatID)

qMsg := session.QueuedMessage{
    Text:   promptText,
    ChatID: chatID,
    UserID: userID,
    Callback: func(_ int64) claude.StatusCallback {
        typingCtl.Stop()
        return CreateStatusCallback(ss)
    },
    ErrCh: make(chan error, 1),
}
if !sess.Enqueue(qMsg) {
    _, err := ctx.EffectiveMessage.Reply(tgBot, "Queue full, please wait.", nil)
    typingCtl.Stop()
    return err
}
go func() {
    err, ok := <-qMsg.ErrCh
    if !ok || err == nil {
        fullText := ss.AccumulatedText()
        if fullText != "" {
            maybeAttachActionKeyboard(tgBot, chatID, fullText)
        }
        return
    }
    // error handling same as HandleText
}()
return nil
```

### Voice Transcript Display

```go
// After transcription succeeds:
statusMsg, _ := tgBot.SendMessage(chatID, "Transcribing...", &gotgbot.SendMessageOpts{
    DisableNotification: true,
})
transcript, err := transcribeVoice(ctx, cfg.OpenAIAPIKey, voicePath)
if err != nil {
    tgBot.EditMessageText(
        fmt.Sprintf("Transcription failed: %s", truncate(err.Error(), 100)),
        &gotgbot.EditMessageTextOpts{ChatId: chatID, MessageId: statusMsg.MessageId},
    )
    return nil
}
displayTranscript := transcript
if len(displayTranscript) > 200 {
    displayTranscript = displayTranscript[:197] + "..."
}
tgBot.EditMessageText(
    fmt.Sprintf("Transcribed: \"%s\"", displayTranscript),
    &gotgbot.EditMessageTextOpts{ChatId: chatID, MessageId: statusMsg.MessageId},
)
// promptText = transcript (full, not truncated)
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| OpenAI `whisper-1` model only | `whisper-1`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe` now available | Early 2025 | Locked to `whisper-1` per TypeScript spec; newer models are more accurate but cost more |
| NSSM 2.24 last stable release | Same — no newer stable release | 2014 (2.24 still current) | NSSM is mature and stable; no version concern |
| gotgbot v2 rc.34 | Same version in go.mod | — | No upgrade needed for Phase 3 features |

**Deprecated/outdated:**
- None relevant to this phase.

---

## Open Questions

1. **NSSM `%TEMP%` directory under SYSTEM account**
   - What we know: SYSTEM account uses `C:\Windows\Temp` for `%TEMP%`
   - What's unclear: Whether the target machine runs NSSM as SYSTEM or a user account
   - Recommendation: Document `TEMP_DIR` env var as optional override; default to `os.TempDir()` which works in both scenarios when NSSM is configured to log on as a specific user account

2. **OpenAI Whisper model: `whisper-1` vs `gpt-4o-transcribe`**
   - What we know: TypeScript version uses `gpt-4o-transcribe`; `whisper-1` is also valid
   - What's unclear: Whether the user wants the newer/more accurate model
   - Recommendation: Make model configurable via `WHISPER_MODEL` env var, defaulting to `whisper-1` (locked in TypeScript spec). Document the option.

3. **PdfToTextPath empty when no PDF tool installed**
   - What we know: Config field not yet present; must be added
   - What's unclear: Should the bot start if PDFTOTEXT_PATH is not set, or fail startup?
   - Recommendation: Allow startup without it (OpenAI key is optional; pdftotext should be too). When a PDF is received with `cfg.PdfToTextPath == ""`, reply "PDF extraction not configured. Set PDFTOTEXT_PATH."

---

## Validation Architecture

nyquist_validation is enabled in .planning/config.json.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib (`testing` package) |
| Config file | none — `go test ./...` convention |
| Quick run command | `go test ./internal/handlers/... -run TestMedia -v -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MEDIA-01 | Voice download + transcription API call + prompt routing | unit (mock HTTP) | `go test ./internal/handlers/... -run TestHandleVoice` | Wave 0 |
| MEDIA-01 | OpenAI key missing → error reply, no crash | unit | `go test ./internal/handlers/... -run TestHandleVoice_NoAPIKey` | Wave 0 |
| MEDIA-02 | Single photo: largest PhotoSize selected, path in prompt | unit | `go test ./internal/handlers/... -run TestHandlePhoto_Single` | Wave 0 |
| MEDIA-03 | Album: 2 photos buffered, timer fires, batch prompt built | unit | `go test ./internal/handlers/... -run TestMediaGroupBuffer` | Wave 0 |
| MEDIA-04 | PDF: pdftotext invoked with -layout flag, output in prompt | unit (mock exec) | `go test ./internal/handlers/... -run TestHandleDocument_PDF` | Wave 0 |
| MEDIA-04 | PDF extraction failure → user-facing error | unit | `go test ./internal/handlers/... -run TestHandleDocument_PDFError` | Wave 0 |
| MEDIA-05 | Text file: content read, truncated at 100K | unit | `go test ./internal/handlers/... -run TestHandleDocument_Text` | Wave 0 |
| MEDIA-05 | Unsupported file type → error listing supported types | unit | `go test ./internal/handlers/... -run TestHandleDocument_Unsupported` | Wave 0 |
| MEDIA-05 | File > 10MB → rejection error | unit | `go test ./internal/handlers/... -run TestHandleDocument_TooBig` | Wave 0 |
| DEPLOY-02 | NSSM commands documented correctly | manual | review .planning/docs or README | manual-only |

### Sampling Rate

- **Per task commit:** `go test ./internal/handlers/... -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/handlers/voice_test.go` — covers MEDIA-01 cases
- [ ] `internal/handlers/photo_test.go` — covers MEDIA-02 and MEDIA-03
- [ ] `internal/handlers/document_test.go` — covers MEDIA-04 and MEDIA-05
- [ ] `internal/handlers/media_group_test.go` — covers MediaGroupBuffer timer logic in isolation

Note: The existing test infrastructure (`go test ./...`) covers all packages. The new test files follow the exact same pattern as `internal/handlers/text_test.go` and `internal/handlers/command_test.go`.

---

## Sources

### Primary (HIGH confidence)

- gotgbot v2.0.0-rc.34 source — `gen_types.go` (Voice, PhotoSize, Document, Message.MediaGroupId fields), `ext/handlers/filters/message/message.go` (Voice, Photo, Document filter functions), `gen_methods.go` (GetFile signature) — inspected directly from local module cache
- `internal/handlers/text.go` — established handler pattern (mapping check, worker start, enqueue, ErrCh async drain) — inspected directly
- `internal/handlers/streaming.go` — StreamingState, CreateStatusCallback, StartTypingIndicator — inspected directly
- `internal/config/config.go` — Config struct (OpenAIAPIKey present, PdfToTextPath absent — confirmed gap) — inspected directly
- `src/handlers/voice.ts`, `photo.ts`, `document.ts`, `media-group.ts`, `utils.ts` — TypeScript functional spec — inspected directly

### Secondary (MEDIUM confidence)

- https://www.nssm.cc/usage — NSSM install syntax, AppDirectory, AppEnvironmentExtra, AppStdout/AppStderr — fetched directly
- OpenAI audio transcription API — endpoint `POST /v1/audio/transcriptions`, parameters `file` (multipart) + `model`, response `{"text": "..."}`, supported formats including OGG — verified via WebSearch against official docs reference

### Tertiary (LOW confidence)

- NSSM SYSTEM account %TEMP% behavior — based on general Windows Service knowledge; should be verified on target machine

---

## Metadata

**Confidence breakdown:**

- Standard stack: HIGH — no new dependencies; everything in stdlib or existing go.mod
- Architecture patterns: HIGH — directly derived from existing Go code that is already tested and running
- Whisper API specifics: MEDIUM — endpoint and params confirmed via search but official docs returned 403; TypeScript code confirms the pattern
- NSSM specifics: MEDIUM — official nssm.cc docs fetched and verified
- NSSM SYSTEM %TEMP% pitfall: LOW — general Windows knowledge; verify on target machine
- Config gap (PdfToTextPath missing): HIGH — confirmed by direct code inspection

**Research date:** 2026-03-20
**Valid until:** 2026-04-20 (stable domain — OpenAI and NSSM APIs are not fast-moving)
