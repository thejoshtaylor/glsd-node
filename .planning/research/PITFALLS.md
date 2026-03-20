# Pitfalls Research

**Domain:** Go Telegram bot with Claude CLI subprocess management, multi-project sessions, Windows Service deployment
**Researched:** 2026-03-20 (v1.1 additions); 2026-03-19 (v1.0 original)
**Confidence:** HIGH (codebase read directly; gotgbot source and official Telegram API docs consulted)

---

## v1.1 Milestone Pitfalls — Channel Auth and HTTP Timeout Bugfixes

These pitfalls are specific to the v1.1 milestone: fixing channel-type auth failures and getUpdates polling timeout errors in the existing Go/gotgbot codebase.

---

### Pitfall V1.1-1: Auth Passes Zero userID for Channel Posts

**What goes wrong:**
Channel posts arrive as `channel_post` update type. Telegram does not populate the `from` field for these updates — it is nil per the official Bot API spec ("Sender of the message; may be empty for messages sent to channels"). In the current middleware (`internal/bot/middleware.go`), when `ctx.EffectiveSender` is non-nil but its underlying `User` field is nil (because the sender is a channel, not a user), `ctx.EffectiveSender.Id()` returns the channel's chat ID — a large negative number (e.g. `-1001234567890`). This value is not in `cfg.AllowedUsers`, so the bot rejects every message sent by the channel itself.

The current `IsAuthorized` (`security/validate.go`) compares `userID` against `allowedUsers []int64`. For a channel post, `userID` is the channel chat ID, not a human user ID. No channel chat ID is in the human user allowlist, so auth always fails.

Separately, some update types can have `EffectiveSender == nil`. The middleware guards this with `if ctx.EffectiveSender != nil`, so no nil-pointer panic occurs — but `userID` silently becomes `0`, which is also not in any allowlist, causing a silent reject.

**Why it happens:**
The auth design was user-ID-based: "is this human in the allowlist?" Channel posts have no human sender — they come from the channel entity itself. When gotgbot constructs `EffectiveSender` from a channel post, `Sender.User` is nil and `Sender.Chat` is populated. The `Id()` method returns `Sender.Chat.Id` in this case, which is the channel's own ID rather than a user ID from the allowlist. The mismatch is structural: user auth checks do not apply to channel sender types.

**How to avoid:**
Change the auth strategy for channel posts. The correct check is: "is this message coming from a channel that this bot is configured to serve?" — i.e., check `EffectiveChat.Id` against the configured project channel IDs in `MappingStore`, rather than checking the sender user ID against `allowedUsers`.

Concrete implementation pattern:
```go
// In IsAuthorized or in the middleware directly:
if sender.IsUser() {
    // Human sender path — existing logic unchanged
    return isUserInAllowlist(sender.User.Id, allowedUsers)
} else {
    // Channel/chat sender path — authorize by channel ID in MappingStore
    _, ok := mappingStore.Get(channelID)
    return ok
}
```

The two auth paths must be independent branches. Do not replace the existing `allowedUsers` check — only add the channel branch.

**Warning signs:**
- Log entries showing `auth_rejected` events for messages you sent yourself from the channel.
- All messages in a Telegram channel result in "You're not authorized" replies from the bot.
- Audit log shows `userID` values that are large negative numbers (channel IDs look like `-1001234567890`).
- Bot responds correctly in private chat with a human user but rejects all channel messages.

**Phase to address:**
v1.1 Phase 1 — channel auth fix. This is the primary goal of the milestone.

---

### Pitfall V1.1-2: HTTP Client Timeout Shorter Than Long-Poll Duration

**What goes wrong:**
The bot's `Start()` method sets `GetUpdatesOpts.Timeout: 10`, telling Telegram to hold the long-polling connection open for up to 10 seconds while waiting for updates. However, the gotgbot HTTP client's hardcoded default request timeout is **5 seconds** (`DefaultTimeout = time.Second * 5` in gotgbot's `request.go`). Because 5 < 10, the HTTP client closes the connection before Telegram's 10-second hold completes, producing `context deadline exceeded` errors on every polling cycle when no messages arrive within 5 seconds.

The error is non-fatal — gotgbot's updater retries automatically — but it generates continuous log noise, causes unnecessary TCP reconnects (one every ~5 seconds during idle), and can slow down `Stop()` because the updater waits for in-flight requests to resolve.

**Why it happens:**
Developers set `GetUpdatesOpts.Timeout` (the Telegram API-level polling duration, in integer seconds) without realizing there is a separate HTTP-level timeout that governs the underlying `net/http` request (`RequestOpts.Timeout`, a `time.Duration`). These are two independent timeouts at different layers. The gotgbot default HTTP timeout was not designed to auto-scale with the polling duration. The `RequestOpts` must be nested inside `GetUpdatesOpts`, not at the `PollingOpts` level — the nesting is non-obvious.

**How to avoid:**
Pass a `RequestOpts` with a `Timeout` value strictly greater than `GetUpdatesOpts.Timeout`. The canonical gotgbot middleware sample demonstrates the exact pattern:

```go
b.updater.StartPolling(b.bot, &ext.PollingOpts{
    GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
        Timeout: 9, // Telegram holds connection for up to 9 seconds
        RequestOpts: &gotgbot.RequestOpts{
            Timeout: 10 * time.Second, // HTTP timeout must be > long-poll duration
        },
    },
})
```

The rule: `RequestOpts.Timeout` (in `time.Duration`) must be strictly greater than `GetUpdatesOpts.Timeout` (in integer seconds, converted). A buffer of 1–5 seconds is conventional. The gotgbot documentation states: "It is recommended you set your PollingOpts.Timeout value to be slightly bigger (eg, +1)."

**Warning signs:**
- `context deadline exceeded` errors logged repeatedly even when the bot is idle.
- Log entries cycling approximately every 5 seconds with timeout errors during quiet periods.
- `Stop()` taking longer than expected to return because in-flight polling requests have not resolved.
- Pattern: errors appear at regular intervals, not tied to user activity.

**Phase to address:**
v1.1 Phase 1 or Phase 2 — this is a one-line fix in `bot.go:Start()` and can be addressed in the same pass as the channel auth fix.

---

### Pitfall V1.1-3: Auth Regression — Private Chat Users Broken by Channel Auth Fix

**What goes wrong:**
When updating `IsAuthorized` to handle channel senders, it is easy to inadvertently change the logic path for normal private-chat or group messages from human users, causing them to fail auth.

Common regression patterns:
- Adding an early return for channel senders that falls through incorrectly for human users.
- Checking `channelID` against `MappingStore` for all update types, including private chats that are not in the mapping store, causing those to fail.
- Removing or restructuring the `allowedUsers` loop while adding channel ID checks.

**Why it happens:**
Channel auth and user auth are fundamentally different checks conflated into one function. The fix for channel auth requires branching on sender type, and that branch must be mutually exclusive with the existing user auth path. Developers adding the channel path sometimes restructure the entire function body, inadvertently altering the existing path.

**How to avoid:**
Implement the fix as an additive branch, not a rewrite:
```
if sender.IsUser():
    authorize by userID in allowedUsers  // existing code, unmodified
else:
    authorize by channelID in MappingStore  // new code
```

After the fix, run all existing middleware tests (`TestMiddlewareAuthAllowsAuthorized`, `TestMiddlewareAuthRejectsUnauthorized`, `TestMiddlewareAuthPassesChannelID`). If they still pass, the regression risk is low. Add new tests for the channel-sender path before considering the fix complete.

**Warning signs:**
- Existing `TestMiddlewareAuth*` tests fail after the channel auth change.
- Private chat messages to the bot stop working after the fix is deployed.
- Human users who were previously authorized start receiving "You're not authorized" replies.

**Phase to address:**
v1.1 Phase 1 — verification step that must accompany the channel auth fix in the same phase.

---

### Pitfall V1.1-4: EffectiveSender Sender-Type Detection Misses Linked Channel and Anonymous Admin Cases

**What goes wrong:**
`EffectiveSender.IsChannelPost()` returns true only when the sender is a channel admin posting directly to that same channel (the channel is both sender and destination, update type is `channel_post`). There are two other non-human sender types that also have nil `User` fields:

1. **Linked channel forwards** — when a channel posts a message that is automatically forwarded to its linked discussion group, the update type is `message` (not `channel_post`) and `EffectiveSender.IsLinkedChannel()` is true.
2. **Anonymous admins** — when a group admin posts as the group itself, `EffectiveSender.IsAnonymousAdmin()` is true and `User` may be a dummy value.

Checking only `IsChannelPost()` misses both cases, causing them to fall into the user auth path with a non-user ID, and fail auth.

**Why it happens:**
The gotgbot `Sender` type has four distinct states (user, channel post, linked channel, anonymous admin) that map to different field combinations. Developers write for the most common case (`channel_post`) without accounting for the other three. The official docs warn: "It may be better to rely on EffectiveSender instead of EffectiveUser, which allows for easier use in the case of linked channels, anonymous admins, or anonymous channels."

**How to avoid:**
Use `sender.IsUser()` as the canonical test. If `IsUser()` returns false, the sender is a non-human entity of some kind, and channel-ID-based auth applies:
```go
isChannelSender := !sender.IsUser()
// or equivalently: sender.User == nil (but use the provided method)
```
This is more robust than checking `IsChannelPost() || IsLinkedChannel() || IsAnonymousAdmin()` individually.

**Warning signs:**
- Bot works for direct channel posts but fails for messages from anonymous admins ("Admin posted as group").
- Bot works in the main channel but fails in a linked discussion group for forwarded posts.
- Auth works for some channel interactions but not others with no obvious pattern.

**Phase to address:**
v1.1 Phase 1 — extend the channel sender detection to cover all non-user sender types, not just channel posts.

---

### Pitfall V1.1-5: Middleware Returns EndGroups Incorrectly When Adding New Auth Branch

**What goes wrong:**
In gotgbot's dispatcher, `ext.EndGroups` stops processing all remaining handler groups — it is a hard stop for the entire update. The current auth middleware correctly returns `ext.EndGroups` on rejection. However, when adding channel auth logic, it is tempting to add a conditional early return in the middle of the middleware that uses `EndGroups` for cases that should actually fall through to the next check (e.g., returning `EndGroups` when the sender type check is inconclusive, rather than falling through to the user check).

The `passthroughHandler` pattern used in this codebase (returning `nil` from a no-op) means: "completed this group, move to next." If a new conditional block returns `EndGroups` when it should pass through, all business handlers in group 0 stop receiving that update type.

**Why it happens:**
The distinction between `ext.EndGroups` (stop everything), `ext.ContinueGroups` (skip remaining handlers in this group, try next group), and `nil`/passthrough (match completed, move to next group) is subtle. In a middleware-as-handler pattern (negative dispatcher groups), the behavior is: `EndGroups` from group -2 prevents group -1 and group 0 from seeing the update. This is correct for auth rejection but wrong for conditional auth logic that has not yet decided.

**How to avoid:**
Only return `ext.EndGroups` at the definitive "this update is rejected" point. For all other control flow within the middleware, fall through to `return next.HandleUpdate(tgBot, ctx)` (allow) or return the rejection only after all auth checks are complete. Keep the middleware structure linear: one single decision point at the end, not multiple early-return `EndGroups` for different sub-conditions.

**Warning signs:**
- A handler in group 0 (business logic) is never reached for certain update types after the middleware change.
- The middleware tests pass in isolation but business handler tests fail for channel update types.
- Logs show updates being received but no handler responding, with no `auth_rejected` event logged.

**Phase to address:**
v1.1 Phase 1 — when modifying the middleware chain for channel auth. Review every return path in the modified middleware before committing.

---

### Pitfall V1.1-6: Bot Replies "You're Not Authorized" Into the Channel Itself

**What goes wrong:**
When auth fails for a channel post, the current middleware attempts to reply with "You're not authorized..." using `ctx.EffectiveMessage.Reply()`. For channel posts, this reply goes back into the channel itself — not to a human user's DM. The channel becomes cluttered with bot error replies that no human explicitly triggered.

Additionally, `Reply()` may fail for certain channel configurations (channels where the bot can post but not reply to specific messages), producing a secondary error that is logged as an error but ignored.

**Why it happens:**
The reply-on-rejection logic was designed for private chats and groups where a human user triggered the unauthorized request and should see the rejection message. Channel posts are triggered by the channel itself, not by a human requesting something — no human needs to see the rejection in the channel timeline.

**How to avoid:**
Before attempting to reply on auth rejection, check whether the update is a channel post:
```go
if ctx.EffectiveMessage != nil && !sender.IsChannelPost() {
    _, _ = ctx.EffectiveMessage.Reply(tgBot, "You're not authorized...", nil)
}
```
For channel-originated updates, log the rejection but do not reply into the channel.

**Warning signs:**
- The channel timeline contains "You're not authorized" messages from the bot.
- Auth rejection audit events are logged but reply errors are also logged immediately after.
- Users report seeing error messages appearing in the channel that they did not trigger.

**Phase to address:**
v1.1 Phase 1 — part of the channel auth fix. Handle the reply behavior for channel-type rejections differently from user-type rejections.

---

### Pitfall V1.1-7: GetUpdatesOpts.Timeout=0 (Short Polling) Causing Rapid-Fire Requests

**What goes wrong:**
If `GetUpdatesOpts.Timeout` is set to 0 (or omitted, which defaults to 0), Telegram responds immediately with whatever updates are pending rather than holding the connection. The updater then immediately calls `getUpdates` again, creating a tight polling loop. Telegram will begin rate-limiting these requests after a short period, causing increasing response delays and eventually producing errors indistinguishable from connection timeouts.

This is a related but distinct issue from Pitfall V1.1-2: the current code already uses `Timeout: 10`, but a change that accidentally resets this to 0 during the fix would cause a different (and harder to diagnose) class of failures.

**Why it happens:**
`0` is a valid Go zero value and the default. If `GetUpdatesOpts` is re-created during the fix without preserving the `Timeout: 10` value, it silently reverts to short polling.

**How to avoid:**
Always set a non-zero `Timeout` in `GetUpdatesOpts` for production polling. The canonical recommendation is 9–30 seconds. Include a comment explaining why the value is set:
```go
GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
    // Non-zero timeout enables long-polling. Zero would use short-polling,
    // which causes rapid-fire requests that trigger Telegram rate limiting.
    Timeout: 9,
    RequestOpts: &gotgbot.RequestOpts{
        Timeout: 10 * time.Second, // Must be > Timeout above
    },
},
```

**Warning signs:**
- `getUpdates` calls appear in logs many times per second (short polling cadence).
- Telegram starts returning slower responses or 429 errors unrelated to message volume.
- Bot feels "laggy" even for simple commands.

**Phase to address:**
v1.1 Phase 1 or Phase 2 — timeout configuration fix. Verify the timeout value is preserved during the fix.

---

## v1.0 Pitfalls — Original Milestone (2026-03-19)

These pitfalls were identified during the initial Go rewrite and remain relevant for ongoing development.

---

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
| Hardcoding `GetUpdatesOpts.Timeout: 10` without matching `RequestOpts.Timeout` | Simpler startup code | Continuous `context deadline exceeded` log noise; misleading error signals | Never — always pair these two values |
| Single `IsAuthorized(userID, channelID, allowedUsers)` handling all sender types without branching | Fewer function parameters | Logic becomes entangled when channel and user auth rules differ | Only if the two paths remain clearly separated via branching; document the invariant |
| Returning `ext.EndGroups` from all middleware regardless of rejection reason | Consistent behavior | Prevents adding fine-grained group-level middleware later | Acceptable for auth/rate-limit (total-stop rejections); not for informational or conditional middleware |
| No test for `ctx.EffectiveSender == nil` path | Less test code | Silent userID=0 behavior may pass auth if 0 is ever added to allowedUsers | Never — the nil path should be explicitly tested |
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
| gotgbot long polling | Setting `GetUpdatesOpts.Timeout` without matching `RequestOpts.Timeout` inside it — HTTP client times out before Telegram responds | Set `RequestOpts.Timeout = time.Duration(GetUpdatesOpts.Timeout+1) * time.Second` nested inside `GetUpdatesOpts.RequestOpts` |
| gotgbot long polling | Using `Timeout: 0` (short polling) in production | Use a non-zero timeout (9–30s) to avoid rapid-fire requests that trigger Telegram rate limits |
| Telegram channel posts | Checking `msg.From.Id` for auth on channel post messages | Check `EffectiveChat.Id` for channel-originated messages; `From` is nil for channel posts per the Bot API spec |
| gotgbot `EffectiveSender.Id()` | Calling `Id()` without checking sender type — returns channel chat ID (negative int64) for channel posts | Check `sender.IsUser()` first; only compare against `allowedUsers` if `IsUser()` is true |
| gotgbot dispatcher groups | Adding middleware to group 0 alongside business handlers — execution order within a group is undefined | Always use dedicated negative group numbers for middleware (-2, -1, etc.) as the codebase already does |
| gotgbot `Stop()` on shutdown | Calling `Stop()` while long-poll requests are in-flight with short HTTP timeouts — slow shutdown | Ensure `RequestOpts.Timeout` is set so in-flight requests resolve within the shutdown window |
| Telegram Bot API | Calling `answerCallbackQuery` only when you have something to show | Call `answerCallbackQuery` on every callback or Telegram shows a loading spinner indefinitely |
| Telegram Bot API | Assuming `editMessageText` succeeds silently | It returns `Bad Request: message is not modified` when content is identical — treat this as success, not an error |
| Telegram Bot API | Setting `parse_mode: HTML` without validating output | Unclosed HTML tags cause the entire message to fail; always have a plain-text fallback send |
| Claude CLI | Setting `CLAUDECODE` env var in subprocess env | Causes "nested Claude session" error; explicitly delete it from the subprocess environment |
| Claude CLI | Calling `--resume` with a session ID from a different working directory | Sessions are directory-scoped in Claude; verify working dir matches before resuming |
| OpenAI Whisper API | Sending voice OGG files directly from Telegram | Telegram voice notes are OGG/Opus; Whisper accepts OGG but needs the correct content-type header (`audio/ogg`) |
| pdftotext (Windows) | Assuming it's in PATH for the service account | Must be installed and PATH configured explicitly in NSSM environment settings |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Short-polling (Timeout=0) instead of long-polling | Rapid-fire getUpdates calls; Telegram begins delaying responses; CPU spikes | Always use Timeout >= 1 in GetUpdatesOpts | Immediately at any load level |
| HTTP client timeout shorter than long-poll duration | `context deadline exceeded` every polling cycle even when idle; high TCP reconnect rate | Pair RequestOpts.Timeout > GetUpdatesOpts.Timeout as described above | Immediately, but silently — bot functions but with log noise |
| editMessageText on every streaming chunk | 429 flood errors, bot blackouts | Throttle per-session, aggregate across sessions, respect global rate budget | At 2+ simultaneous active streaming sessions |
| Blocking on `cmd.Wait()` in request goroutine | Handler goroutine hangs, updates queue up | Always run subprocess in its own goroutine, communicate via channel | Immediately on any stop/restart while streaming |
| Saving session JSON on every received streaming event | Excessive disk I/O, file contention | Save session ID only once on first receipt, save state only on meaningful change | With fast Claude responses emitting many events |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Accepting userID=0 as authorized | If `0` is accidentally added to allowedUsers, all unauthenticated channel updates pass | Explicitly reject userID == 0 and channelID == 0 in `IsAuthorized` as invalid identifiers |
| Channel auth by channel username string instead of ID | Channel usernames can change; auth check becomes invalid after rename | Always use channel numeric ID (negative int64) from `EffectiveChat.Id`, never username strings |
| Authorizing all messages from any channel without verifying the channel is in MappingStore | Any channel the bot is added to could control it | Channel auth must confirm the channel ID is in the configured project mappings, not just "any channel" |
| Logging auth rejection with full message content | Audit log leaks message text from unauthorized senders | Log only sender ID, channel ID, and update type on auth rejection — never message text |
| Per-channel auth based only on channel ID (no membership check) | Any user who finds the channel ID can control Claude sessions | Validate channel membership or maintain an explicit allowlist of channel IDs in config |
| Passing user-provided text directly as Claude prompt without sanitization | Prompt injection could instruct Claude to exfiltrate files or run dangerous commands | Apply the same `--allowed-paths` considerations as the TypeScript version; rate limit prompt length |
| Storing bot token in plain text in service environment | Service account compromise exposes bot token | Store token in Windows Credential Manager or environment variable set only in NSSM |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Bot replies "You're not authorized" into the channel itself | Channel becomes cluttered with error replies; no human sees or needs them | Check sender type before replying; for channel-originated updates, log the rejection but do not reply into the channel |
| No reply possible for channel posts — `Reply()` fails silently | Secondary error logged; confusion during debugging | Guard `Reply()` with `if ctx.EffectiveMessage != nil && !sender.IsChannelPost()` |
| Continuous `context deadline exceeded` errors in Windows Service logs | Log file grows rapidly; real errors masked | Fix the HTTP timeout pairing (V1.1-2) — stops log noise completely |
| No feedback between message send and first streaming chunk | User thinks bot is broken, sends duplicate messages | Send "typing..." action immediately, then a "Processing..." message within 1 second |
| Streaming updates too fast (< 500ms interval) | Telegram notification spam, 429 errors | Throttle to minimum 1500ms between edits during streaming |
| Silent failure when channel has no linked project | User confused why bot doesn't respond | Prompt: "This channel has no linked project. Reply with the project path to link it." |
| Callback query buttons staying active after selection | User taps same button twice thinking first tap failed | Call `editMessageReplyMarkup` to remove keyboard immediately on first callback |

---

## "Looks Done But Isn't" Checklist

- [ ] **Channel auth fix (v1.1):** Tests pass with mock channel sender — verify with live channel by sending a message from the channel itself (not just as an admin user in personal DM)
- [ ] **HTTP timeout fix (v1.1):** Log shows zero `context deadline exceeded` errors during a 60-second idle period — verify by tailing the Windows Service log file after deployment
- [ ] **Auth regression (v1.1):** All 77+ existing handler tests still pass after middleware changes — run `go test ./...` before committing
- [ ] **Nil EffectiveSender (v1.1):** Explicit test added for `ctx.EffectiveSender == nil` case in `TestMiddlewareAuth*` — this path currently has no dedicated test
- [ ] **Channel reply guard (v1.1):** `ctx.EffectiveMessage.Reply()` in auth rejection path is guarded against channel-type updates — confirm no "unauthorized" messages appear in the channel timeline
- [ ] **RequestOpts nesting (v1.1):** Confirm the `RequestOpts` is set on `GetUpdatesOpts.RequestOpts` (not at the `PollingOpts` level) — the nesting is non-obvious
- [ ] **Windows Service deployment:** Test by actually installing via NSSM and rebooting — interactive `go run` masks PATH and environment differences
- [ ] **Multi-project sessions:** Test with two channels sending messages simultaneously, not sequentially — concurrency bugs only appear under concurrent load
- [ ] **Stop/resume flow:** Verify that stopping a query mid-stream and immediately sending a new message results in exactly one active Claude process
- [ ] **Context limit handling:** Let a session actually hit the context limit (or mock it) — verify session is cleared, user is notified, next message starts fresh
- [ ] **Rate limiting across sessions:** Start two projects both streaming simultaneously and verify no 429 errors in the first 5 minutes
- [ ] **JSON persistence:** Kill the process with `taskkill /F` (hard kill), restart, verify session list is intact and the file is valid JSON

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Auth regression breaks private chat (v1.1) | LOW | Revert `IsAuthorized` change; re-deploy service; JSON state files are unaffected |
| Wrong timeout values deployed (v1.1) | LOW | Update `bot.go` Start() config; restart Windows Service via NSSM; no data migration needed |
| Middleware EndGroups in wrong place — handlers silent (v1.1) | MEDIUM | Add debug logging to identify which group swallows updates; correct return value; redeploy |
| Channel auth accepts all channels — security regression (v1.1) | MEDIUM | Revert channel auth change; add MappingStore verification before accepting channel sender; redeploy |
| Orphaned claude.exe processes accumulating | LOW | `taskkill /IM claude.exe /F` from admin prompt; add to NSSM pre-start script |
| Corrupted session JSON file | LOW | Delete the file; bot recreates it; sessions lost but service functional |
| Bot globally rate-limited (429) | LOW | Wait for `retry_after` duration (typically 1-60s); implement automatic back-off in code |
| Concurrent map panic (crash) | MEDIUM | Service auto-restarts via NSSM; fix requires adding mutex protection before next deploy |
| Session ID invalid after context limit | LOW | User runs `/new` command to start fresh session |
| Goroutine leak degrading service | HIGH | Restart service (NSSM); fix requires adding `WaitDelay` and context cancellation before next deploy |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Auth passes zero userID for channel posts (V1.1-1) | v1.1 Phase 1 — channel auth fix | Send a message from the channel; bot must respond, not reject |
| HTTP client timeout shorter than long-poll (V1.1-2) | v1.1 Phase 1 — bot startup config | Idle for 60s; confirm zero `context deadline exceeded` in log |
| Auth regression — private chat broken (V1.1-3) | v1.1 Phase 1 — same fix, verification | All 77+ existing tests pass; private chat messages still work after deploy |
| IsChannelPost() misses linked channel (V1.1-4) | v1.1 Phase 1 — channel sender detection | Test with anonymous admin message; test with linked discussion group forward |
| EndGroups scope confusion in middleware (V1.1-5) | v1.1 Phase 1 — when touching middleware chain | All existing middleware tests pass; no group 0 handlers silently skipped |
| Bot replies into channel on auth rejection (V1.1-6) | v1.1 Phase 1 — reply guard | No "unauthorized" messages appear in the channel timeline after fix |
| GetUpdatesOpts.Timeout reverts to 0 (V1.1-7) | v1.1 Phase 1 or 2 — config fix | Non-zero Timeout confirmed in code review; log does not show short-polling cadence |
| Windows process tree orphaning | v1.0 Phase 1 — subprocess management | Test: stop a query, check Task Manager for orphaned claude.exe |
| Concurrent map panic | v1.0 Phase 1 — bot structure | Test: `-race` flag + two channels sending simultaneously |
| NSSM PATH blindness | v1.0 Phase 1 + Phase 3 | Test: run as Windows Service, not interactively |
| editMessageText flood limit | v1.0 Phase 2 — multi-project sessions | Test: 3+ simultaneous streaming sessions, monitor for 429 |
| Session ID stale after context limit | v1.0 Phase 1 — session wrapper | Test: mock context limit error in session handler |
| JSON persistence corruption | v1.0 Phase 1 — persistence layer | Test: `go test -race` on concurrent save operations |
| Goroutine leak from pipes | v1.0 Phase 1 — subprocess management | Test: pprof goroutine count across 20 stop/start cycles |
| Long-polling offset reset | v1.0 Phase 3 — service deployment | Test: kill service mid-poll, restart, verify no duplicate processing |

---

## Sources

- **gotgbot v2 source — `request.go`** (DefaultTimeout = 5s): https://github.com/PaulSonOfLars/gotgbot/blob/v2/request.go
- **gotgbot v2 source — `bot.go`** (BotOpts, no HTTPClient field): https://github.com/PaulSonOfLars/gotgbot/blob/v2/bot.go
- **gotgbot v2 source — `sender.go`** (Sender type, IsChannelPost, IsLinkedChannel, IsUser): https://github.com/PaulSonOfLars/gotgbot/blob/v2/sender.go
- **gotgbot v2 middleware sample** (paired timeout pattern, Timeout:9 + RequestOpts.Timeout:10s): https://github.com/PaulSonOfLars/gotgbot/blob/v2/samples/middlewareBot/main.go
- **gotgbot v2 ext documentation** (PollingOpts, EffectiveSender, context deadline exceeded note): https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2/ext
- **Telegram Bot API — Message object** ("from" field nil for channel posts): https://core.telegram.org/bots/api#message
- **Project codebase** — `internal/bot/middleware.go`, `internal/bot/bot.go`, `internal/bot/handlers.go`, `internal/security/validate.go`, `internal/bot/middleware_test.go` (direct read, HIGH confidence)
- TypeScript reference implementation (`src/session.ts`, `src/handlers/streaming.ts`) — proven solutions for process tree killing, context limit detection
- [Telegram flood limits (grammY)](https://grammy.dev/advanced/flood) — rate limit behavior documentation
- [Telegram Bot API errors list](https://github.com/TelegramBotAPI/errors) — complete error code reference
- [Go race detector](https://go.dev/doc/articles/race_detector) — `-race` flag for concurrent map detection

---
*Pitfalls research for: Go Telegram bot (gotgbot v2) — v1.1 channel auth, HTTP timeouts, middleware; v1.0 subprocess, concurrency, Windows Service*
*Researched: 2026-03-20*
