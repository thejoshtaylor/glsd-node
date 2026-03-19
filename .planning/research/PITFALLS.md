# Pitfalls Research

**Domain:** Go Telegram bot with Claude CLI subprocess management, multi-project sessions, Windows Service deployment
**Researched:** 2026-03-19
**Confidence:** HIGH (most pitfalls verified against official docs, existing TypeScript codebase, and Go runtime issues)

---

## Critical Pitfalls

### Pitfall 1: Windows Process Tree Orphaning on Subprocess Kill

**What goes wrong:**
On Windows, `cmd.Process.Kill()` kills only the top-level process. Since the Claude CLI spawns with `shell: true` (or equivalent), you kill the `cmd.exe` wrapper but not the actual `claude.exe` child, which keeps running and holds file locks, network ports, and the session. Multiple orphaned claude processes accumulate across restarts. The TypeScript version already solved this with `taskkill /pid <pid> /T /F` — naively porting to Go's `cmd.Process.Kill()` will break this.

**Why it happens:**
Developers assume `Process.Kill()` terminates the whole tree. On Unix, negative PID signaling (`kill(-pgid, SIGTERM)`) kills process groups. Neither mechanism works on Windows — the OS uses Job Objects for process group killing, and `taskkill /T /F` is the practical userland approach.

**How to avoid:**
On Windows, implement a `killTree(pid int)` that calls `exec.Command("taskkill", "/pid", strconv.Itoa(pid), "/T", "/F").Run()`. Wrap this in a platform build tag. Do not use `cmd.Process.Kill()` as the sole termination path on Windows. Test by inspecting Task Manager after sending a stop command — the `claude.exe` process should disappear.

**Warning signs:**
- `claude.exe` processes accumulate in Task Manager after stop/restart commands
- Subsequent sessions fail with "session already active" or file lock errors
- CPU stays elevated after user-initiated stop
- `CLAUDECODE` environment variable leaks into nested sessions causing "nested session" errors

**Phase to address:**
Phase 1 (Core subprocess management) — this is load-bearing infrastructure for every session operation.

---

### Pitfall 2: Concurrent Map Access Panic on Multi-Project Sessions

**What goes wrong:**
The multi-project design routes each Telegram channel to an independent `ClaudeSession`. If a map of `channelID -> session` is accessed from multiple goroutines (one per incoming update) without a mutex, Go will panic with `fatal error: concurrent map read and map write`. This does not manifest in single-project testing but will crash the service in production once two channels send messages simultaneously.

**Why it happens:**
Go maps are not goroutine-safe. Most Telegram libraries (gotgbot, go-telegram-bot-api) dispatch each update in its own goroutine. The map is accessed from every handler call. Developers often forget this during initial development when testing with a single channel.

**How to avoid:**
Wrap the `channelID -> session` registry in a struct with an embedded `sync.RWMutex`. Use `RLock/RUnlock` for reads (looking up existing sessions) and `Lock/Unlock` for writes (registering new channels). Consider using `sync.Map` if the registry is read-heavy and written rarely. Run all tests with `-race` flag — the race detector will catch this before it crashes in production.

**Warning signs:**
- Works perfectly with one channel, fails unpredictably with two
- `fatal error: concurrent map read and map write` panic in logs
- Race detector (`go test -race`) reports data race on registry map
- Service restarts without explanation under load

**Phase to address:**
Phase 1 (Core bot structure) — design the session registry with mutex from day one, not as a retrofit.

---

### Pitfall 3: NSSM/Windows Service Subprocess PATH Blindness

**What goes wrong:**
The Windows Service runs as `SYSTEM` or a service account with a stripped PATH. When the bot tries to spawn `claude` (and later `pdftotext` or `ffmpeg`), `exec.LookPath` fails silently or finds wrong binaries. The TypeScript version already encountered this — the CLAUDE.md notes explicitly that PATH must include Homebrew paths. The Go version on Windows has the same problem: `%AppData%\npm`, `%USERPROFILE%\.local\bin`, and other user-scoped directories are absent from the service's PATH.

**Why it happens:**
Windows Services inherit the system's minimal PATH, not the interactive user's PATH. Tools installed via npm global (`claude`), scoop, or winget into user directories are invisible. Developers test interactively where PATH is rich, then deploy as a service and everything breaks.

**How to avoid:**
Resolve all external binary paths at startup with explicit absolute path configuration via environment variables (`CLAUDE_CLI_PATH`, `PDFTOTEXT_PATH`, `FFMPEG_PATH`). Fall back to `exec.LookPath` only if the explicit path is unset. Log the resolved path at startup so failures are diagnosable. In NSSM configuration, explicitly set the PATH environment variable under "Environment" to include required directories.

**Warning signs:**
- Bot works when run manually (`go run .`) but not as a service
- `exec: "claude": executable file not found in %PATH%` in service logs
- PDF extraction silently produces empty results
- Service starts successfully but all Claude queries fail immediately

**Phase to address:**
Phase 3 (Windows Service deployment) — but the binary path resolution architecture should be built in Phase 1 to avoid rework.

---

### Pitfall 4: editMessageText Flood Limit Causing Full Bot Lockout

**What goes wrong:**
Streaming responses generate a high volume of `editMessageText` calls (one per throttle interval per active session). With multiple simultaneous sessions and streaming active, the bot easily exceeds Telegram's rate limits. When a 429 occurs, **the entire bot is blocked** for the `retry_after` duration — not just the affected chat. All users across all projects experience the blackout. The TypeScript version throttles updates (STREAMING_THROTTLE_MS) but this is a single-session design. With N simultaneous projects all streaming, the total rate multiplies.

**Why it happens:**
Telegram rate limits are per-bot, not per-chat. Developers test with one active session and acceptable throttling, but N concurrent sessions multiply the edit rate by N. `editMessageText` and `sendMessage` share the same per-bot flood limits. There is no "burst budget" — once you hit 429, everything stops.

**How to avoid:**
Implement a global rate limiter (token bucket or sliding window) across all outgoing Telegram API calls, not per-session. Use a single shared API call queue with configurable max messages-per-second. When a 429 is received, parse `retry_after` from the error response and back off globally. Increase `STREAMING_THROTTLE_MS` proportional to the number of active sessions (e.g., base_throttle * active_session_count). Consider skipping intermediate streaming updates entirely under high load and only sending final results.

**Warning signs:**
- Random "bot unresponsive" windows affecting all chats simultaneously
- 429 errors in logs followed by silence
- Streaming works fine with one project, fails under two or more simultaneous sessions
- `retry_after` values in error responses (look for these in Telegram API error payloads)

**Phase to address:**
Phase 2 (Multi-project session management) — the rate limiting architecture must account for N sessions before the multi-project feature ships.

---

### Pitfall 5: Claude CLI `--resume` Session ID Mismatch After Context Limit

**What goes wrong:**
When Claude hits its context window limit, the session is invalidated on the Claude side but the bot still holds the session ID in its JSON persistence. On the next message, `--resume <old-session-id>` is passed to the CLI, which either fails silently or returns an error that looks like a network error. The bot appears broken. The TypeScript version handles this by detecting `context limit exceeded` patterns in stderr/stdout and auto-clearing the session — replicating this detection is critical.

**Why it happens:**
Session IDs are opaque strings with no expiry signal. The only indication a session is dead is a pattern match on error output from the CLI. Developers implementing basic session persistence miss this edge case.

**How to avoid:**
Implement `isContextLimitError(text string) bool` matching the patterns already proven in the TypeScript version: `input length and max_tokens exceed context limit`, `exceed context limit`, `context limit.*exceeded`, `prompt.*too.*long`, `conversation is too long`. On detection: clear the session ID from memory and JSON persistence, notify the user, and return a clean error — do not retry with the same session ID.

**Warning signs:**
- User reports bot "stuck" after a long conversation
- `claude` CLI exits non-zero with stderr containing "context" or "limit"
- Sessions accumulate in the JSON persistence file with no new successful messages
- `--resume` calls always fail for a specific session ID

**Phase to address:**
Phase 1 (Claude session wrapper) — this is part of the core session state machine.

---

### Pitfall 6: JSON Persistence File Corruption Under Concurrent Writes

**What goes wrong:**
Multiple goroutines (one per channel session) may attempt to write the session persistence file simultaneously. A concurrent write produces a half-written JSON file. On the next restart, JSON parse fails and all session state is lost. This is worse than no persistence — the file exists but is unreadable, and naive error handling silently treats it as empty.

**Why it happens:**
Go's `os.WriteFile` is not atomic — it writes in place. If two goroutines call it simultaneously, the resulting file is a corruption of both writes. The TypeScript version uses `writeFileSync` (which is single-threaded in Node.js, so not an issue), but Go's goroutine model makes this race realistic.

**How to avoid:**
Use a single goroutine (or a mutex-protected writer) for all persistence writes. The pattern is: write to a temp file in the same directory, then `os.Rename` to the target path (on Windows, use `MoveFileEx` via `golang.org/x/sys/windows` for atomic rename, or use `github.com/natefinch/atomic`). Protect the read-modify-write cycle with a mutex. Test by running concurrent save operations under the race detector.

**Warning signs:**
- `json: unexpected end of JSON input` or similar parse errors at startup
- Sessions lost after service restart despite persistence code
- Intermittent "empty sessions list" after busy periods
- Race detector flags the persistence file write path

**Phase to address:**
Phase 1 (session persistence) — build atomic write from the start, not as a follow-up fix.

---

### Pitfall 7: Goroutine Leak from Uncleaned Claude CLI stdout/stderr Pipes

**What goes wrong:**
When `os/exec` is used with `StdoutPipe()` or `StderrPipe()`, Go spawns goroutines to copy data between the pipe and internal buffers. If the subprocess is killed mid-stream (user-initiated stop) and the pipe is not drained before `cmd.Wait()`, these goroutines block indefinitely waiting for EOF that never comes. Over time, the service accumulates hundreds of leaked goroutines, slowly consuming memory until the service becomes unresponsive.

**Why it happens:**
The Go docs for `StdoutPipe` warn that `Wait` blocks until all goroutines finish copying — which requires EOF on the pipe. An abruptly killed subprocess may leave the pipe in a state where the write end is closed (the process is dead) but Go's internal copy goroutine hasn't acknowledged it yet. Using `cmd.Stdout = &buf` (direct assignment) instead of `StdoutPipe` avoids the goroutine issue but blocks until process exit.

**How to avoid:**
Set a `WaitDelay` on `exec.Cmd` (available since Go 1.20) — this caps how long `Wait` will wait for I/O goroutines after the process exits, preventing indefinite blocking. Alternatively, use `context.WithTimeout` and `CommandContext` so the subprocess gets a kill signal on timeout. Drain pipes explicitly before calling `Wait` on stopped processes, or use the `go-cmd/cmd` library which handles this correctly cross-platform. Use `pprof` goroutine dumps to verify goroutine count stays stable across many stop/start cycles.

**Warning signs:**
- `runtime.NumGoroutine()` grows without bound across session stop/starts
- Service memory grows slowly over days of operation
- `/debug/pprof/goroutine` shows many blocked goroutines on pipe read
- Service becomes sluggish after many stop/start cycles without restarting

**Phase to address:**
Phase 1 (subprocess management) — set `WaitDelay` and use `CommandContext` from the start.

---

### Pitfall 8: Long-Polling Offset Reset Causing Duplicate or Missed Updates

**What goes wrong:**
If the offset is not persisted across restarts (or is reset to 0 on error), the bot re-processes all unacknowledged updates from the last 24 hours. For this bot, that means re-sending Claude messages for every message received during the downtime — potentially dozens of unwanted Claude queries firing at once on service restart.

**Why it happens:**
The offset must be incremented to `last_update_id + 1` after each `getUpdates` batch and acknowledged to Telegram. If the process crashes mid-batch before confirming the offset, or if the offset is not persisted to disk, it resets to 0. Most Go Telegram libraries handle this internally (gotgbot, go-telegram-bot-api), but only while the process is running — a crash or service restart resets in-memory state.

**How to avoid:**
Use a library that handles long-polling correctly (gotgbot's updater persists nothing — rely on its in-process offset tracking and ensure clean shutdown via context cancellation before service stop). On SIGTERM/service stop signal, call the library's stop method, which sends a final `getUpdates` with the current offset to acknowledge pending updates. Do not call `deleteWebhook` in a loop at startup — it causes 400 errors if no webhook was set.

**Warning signs:**
- Duplicate Claude queries fired after service restart
- Messages from "hours ago" appearing in logs after reboot
- 400 `ETELEGRAM: 400 Bad Request: there is no webhook active` in logs at startup
- Bot "catches up" on old messages with no rate limiting

**Phase to address:**
Phase 3 (Windows Service deployment) — the service stop signal handler must cleanly drain the update queue before exit.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| `sync.Map` everywhere instead of typed registry struct with RWMutex | Less boilerplate | Loses type safety, harder to reason about invariants | Never — the registry is small and well-defined |
| Single global session instead of per-channel registry | Faster MVP | Blocks entire multi-project feature; requires full redesign | Only for a throwaway prototype, not this project |
| Skip `WaitDelay` on exec.Cmd | Simpler code | Goroutine leaks on stop | Never — set it in the constructor |
| Write JSON directly without temp-file rename | Simpler code | File corruption on crash | Never — use atomic write from day one |
| Hardcode throttle ms at fixed 1500ms | Simple | Floods Telegram at 3+ concurrent sessions | MVP only; replace with adaptive throttle before multi-project ships |
| Use `exec.LookPath` at query time for claude binary | No config needed | Silent failure in Windows Service environment | Never — resolve paths at startup with explicit env var fallback |
| Ignore `retry_after` in 429 responses | Simpler retry logic | Repeated 429 cascades blocking all channels | Never — parse and respect `retry_after` |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Telegram Bot API | Calling `answerCallbackQuery` only when you have something to show | Call `answerCallbackQuery` on every callback or Telegram shows a loading spinner indefinitely |
| Telegram Bot API | Assuming `editMessageText` succeeds silently | It returns `Bad Request: message is not modified` when content is identical — treat this as success, not an error |
| Telegram Bot API | Setting `parse_mode: HTML` without validating output | Unclosed HTML tags cause the entire message to fail; always have a plain-text fallback send |
| Telegram Bot API | Media groups (photo albums): treating each photo as independent | Albums arrive as separate messages with the same `media_group_id`; buffer with a short timeout (500ms-1s) to collect all photos before processing |
| Claude CLI | Setting `CLAUDECODE` env var in subprocess env | Causes "nested Claude session" error; explicitly delete it from the subprocess environment |
| Claude CLI | Passing prompt on command line | Shell escaping breaks on special characters; pipe via stdin instead |
| Claude CLI | Calling `--resume` with a session ID from a different working directory | Sessions are directory-scoped in Claude; verify working dir matches before resuming |
| OpenAI Whisper API | Sending voice OGG files directly from Telegram | Telegram voice notes are OGG/Opus; Whisper accepts OGG but needs the correct content-type header (`audio/ogg`) |
| pdftotext (Windows) | Assuming it's in PATH for the service account | Must be installed and PATH configured explicitly in NSSM environment settings |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| editMessageText on every streaming chunk | 429 flood errors, bot blackouts | Throttle per-session, aggregate across sessions, respect global rate budget | At 2+ simultaneous active streaming sessions |
| Blocking on `cmd.Wait()` in request goroutine | Handler goroutine hangs, updates queue up | Always run subprocess in its own goroutine, communicate via channel | Immediately on any stop/restart while streaming |
| Polling tmpdir for ask_user files in a hot loop | CPU spike, unnecessary I/O | Use a channel or in-memory queue instead of filesystem polling | Immediately at scale |
| Saving session JSON on every received streaming event | Excessive disk I/O, file contention | Save session ID only once on first receipt, save state only on meaningful change | With fast Claude responses emitting many events |
| Allocating `[]byte` buffers in streaming line reader without pooling | Memory pressure during long sessions | Use `bufio.Scanner` with appropriate buffer size, reuse buffers via `sync.Pool` if needed | With very long Claude responses (code generation tasks) |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Per-channel auth based only on channel ID (no membership check) | Any user who finds the channel ID can control Claude sessions | Validate channel membership against Telegram's `getChatMember` API or maintain an explicit allowlist of channel IDs in config |
| Passing user-provided text directly as Claude prompt without sanitization | Prompt injection could instruct Claude to exfiltrate files or run dangerous commands | Apply the same `--allowed-paths` and `--dangerously-skip-permissions` considerations as the TypeScript version; consider rate limiting prompt length |
| Storing bot token in plain text in service environment | Service account compromise exposes bot token | Store token in Windows Credential Manager or environment variable set only in NSSM, not in the source config file |
| Logging full message content in audit log | User message privacy | Log message metadata (length, type, channel) not content; make content logging opt-in via debug flag |
| `shell: true` on Windows exec.Cmd for Claude subprocess | cmd.exe injection if prompt reaches command line args | Pipe prompt via stdin exclusively, never construct command line from user input |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| No feedback between message send and first streaming chunk (Claude can take 2-10s to start) | User thinks bot is broken, sends duplicate messages | Send "typing..." action immediately, then a "Processing..." message within 1 second of receiving the message |
| Streaming updates too fast (< 500ms interval) | Telegram client notification spam, 429 errors | Throttle to minimum 1500ms between edits during streaming, more under load |
| Silent failure when channel has no linked project | User confused why bot doesn't respond | When an unrecognized channel sends a message, immediately prompt: "This channel has no linked project. Reply with the project path to link it." |
| Deleting tool status messages at "done" even when tools showed important info | User can't see what tools were called | Delete tool status messages, not tool result messages; or keep last tool status visible for 30s |
| Callback query buttons staying active after selection | User taps the same button twice thinking first tap failed | Call `editMessageReplyMarkup` to remove or replace the keyboard immediately on first callback |

---

## "Looks Done But Isn't" Checklist

- [ ] **Windows Service deployment:** Test by actually installing via NSSM and rebooting — interactive `go run` masks PATH and environment differences.
- [ ] **Multi-project sessions:** Test with two channels sending messages simultaneously, not sequentially — concurrency bugs only appear under concurrent load.
- [ ] **Stop/resume flow:** Verify that stopping a query mid-stream and immediately sending a new message results in exactly one active Claude process, not zero or two.
- [ ] **Context limit handling:** Let a session actually hit the context limit (or mock it) — verify session is cleared, user is notified, next message starts fresh.
- [ ] **Media group buffering:** Send a photo album (3+ photos simultaneously) and verify they're collected into one Claude call, not three separate ones.
- [ ] **Rate limiting across sessions:** Start two projects both streaming simultaneously and verify no 429 errors in the first 5 minutes.
- [ ] **Service restart recovery:** Stop the service mid-Claude-query, restart it, send a new message — verify the orphaned Claude process was cleaned up and the new session works.
- [ ] **JSON persistence:** Kill the process with `taskkill /F` (hard kill), restart, verify session list is intact and the file is valid JSON.
- [ ] **Callback query acknowledgement:** Click every inline keyboard button and verify no spinning loading indicators remain after the action completes.
- [ ] **HTML parse mode fallback:** Send a response that contains malformed Markdown (e.g., unclosed backtick) and verify it degrades to plain text rather than failing silently.

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Orphaned claude.exe processes accumulating | LOW | `taskkill /IM claude.exe /F` from admin prompt; add to NSSM pre-start script |
| Corrupted session JSON file | LOW | Delete the file; bot recreates it; sessions lost but service functional |
| Bot globally rate-limited (429) | LOW | Wait for `retry_after` duration (typically 1-60s); implement automatic back-off in code |
| Concurrent map panic (crash) | MEDIUM | Service auto-restarts via NSSM; fix requires adding mutex protection before next deploy |
| Session ID invalid after context limit | LOW | User runs `/new` command to start fresh session |
| Service account missing PATH for claude binary | MEDIUM | Update NSSM environment variables, restart service; no code change needed |
| Goroutine leak degrading service | HIGH | Restart service (NSSM); fix requires adding `WaitDelay` and context cancellation before next deploy |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Windows process tree orphaning | Phase 1: Core subprocess | Test: stop a query, check Task Manager for orphaned claude.exe |
| Concurrent map panic | Phase 1: Core bot structure | Test: `-race` flag + two channels sending simultaneously |
| NSSM PATH blindness | Phase 1: Config architecture + Phase 3: Service deployment | Test: run as Windows Service, not interactively |
| editMessageText flood limit | Phase 2: Multi-project sessions | Test: 3+ simultaneous streaming sessions, monitor for 429 |
| Session ID stale after context limit | Phase 1: Session wrapper | Test: mock context limit error in session handler |
| JSON persistence corruption | Phase 1: Persistence layer | Test: `go test -race` on concurrent save operations |
| Goroutine leak from pipes | Phase 1: Subprocess management | Test: pprof goroutine count across 20 stop/start cycles |
| Long-polling offset reset | Phase 3: Service deployment | Test: kill service mid-poll, restart, verify no duplicate processing |

---

## Sources

- TypeScript reference implementation (`src/session.ts`, `src/handlers/streaming.ts`) — proven solutions for process tree killing, context limit detection, HTML/plain fallback
- [Go os/exec issue #23019: Wait goroutines](https://github.com/golang/go/issues/23019) — confirms goroutine copy behavior with pipes
- [Go issue #53863: exec within goroutine CPU 100%](https://github.com/golang/go/issues/53863) — Windows subprocess edge cases
- [Go issue #22278: Pipe to network on Windows](https://github.com/golang/go/issues/22278) — Windows-specific pipe behavior
- [Killing child processes in Go](https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773) — process group kill patterns
- [Go path security blog](https://go.dev/blog/path-security) — PATH lookup in subprocesses
- [natefinch/atomic](https://github.com/natefinch/atomic) — atomic file write library
- [Telegram flood limits (grammY)](https://grammy.dev/advanced/flood) — rate limit behavior documentation
- [Telegram Bot API errors list](https://github.com/TelegramBotAPI/errors) — complete error code reference
- [Go race detector](https://go.dev/doc/articles/race_detector) — `-race` flag for concurrent map detection
- [go-cmd/cmd](https://github.com/go-cmd/cmd) — cross-platform subprocess management wrapper
- [Windows Job Objects for process groups](https://gist.github.com/hallazzang/76f3970bfc949831808bbebc8ca15209) — Windows-specific process tree kill

---
*Pitfalls research for: Go Telegram bot with Claude CLI subprocess management, multi-project sessions, Windows Service*
*Researched: 2026-03-19*
