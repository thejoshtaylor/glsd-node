# Feature Research

**Domain:** Telegram bot bugfix — channel auth and polling timeout
**Researched:** 2026-03-20
**Confidence:** HIGH

## Scope Note

This document covers v1.1 research only. The full feature landscape for v1.0 is preserved as context
below the v1.1 section. v1.1 is a targeted bugfix milestone: two specific failures observed in
production, one root-cause investigation into auth, one into polling timeouts.

---

## v1.1 Bugfix Features

### Bug 1: Channel Auth Failure — EffectiveSender is nil for Channel Posts

**What breaks:** Messages sent in a Telegram channel fail auth because
`ctx.EffectiveSender` is nil when the update comes from a channel post
(as opposed to a group message or DM). The auth middleware calls
`ctx.EffectiveSender.Id()` without a nil check, causing a nil pointer dereference
or returning `userID = 0`, which is never in the allowed-users list.

**Root cause — Telegram Bot API behavior by chat type:**

The Telegram Bot API distinguishes three update types for "messages":

| Update Field | Chat Type | `from` Field | `sender_chat` Field |
|---|---|---|---|
| `Update.Message` | DM | Populated with real User | nil |
| `Update.Message` | Group / Supergroup | Populated with real User | nil (or Chat for anon admin) |
| `Update.ChannelPost` | Channel | **nil** (absent per API spec) | Populated with the Channel chat |

The `from` field is explicitly optional in the Telegram Bot API: "Sender of the message; may be empty for messages sent to channels." This has been the documented behavior since Bot API was introduced and has not changed.

**gotgbot v2 behavior:**

gotgbot's `Sender` struct merges `from` (User) and `sender_chat` (Chat) fields. The `Id()` method
prefers `Chat.Id` when present: "When a message is being sent by a chat/channel, Telegram usually
populates the User field with dummy values. For this reason, we prefer to return the Chat.Id if it
is available."

`EffectiveSender` is populated by `NewContext()` based on which update fields are present. For a
`ChannelPost` update, `from` is nil, so the resulting `Sender.User` is nil. If `sender_chat` is
also absent (pure channel post with no forwarding), `EffectiveSender` may be nil entirely or may
have a non-nil `Sender` with a nil `User` and a non-nil `Chat`.

**Critical gotgbot behavior — handlers.NewMessage does NOT match ChannelPost by default:**

`handlers.NewMessage` only fires for `Update.Message` updates unless `SetAllowChannel(true)` is
called on the handler. Channel posts arrive as `Update.ChannelPost`, which is a separate field.

This means: the text, voice, photo, document handlers registered with `handlers.NewMessage` do NOT
currently receive channel post updates unless `AllowChannel` is set. The auth middleware runs on
ALL updates (group -2, `CheckUpdate` returns true), so it will run on channel post updates, but the
message handlers after it will not fire for those updates.

The auth failure scenario is therefore:

1. A channel post arrives as `Update.ChannelPost`.
2. The auth middleware runs (group -2, catches all updates).
3. `ctx.EffectiveSender` is nil or has `User = nil`.
4. `ctx.EffectiveSender.Id()` returns 0 (if Sender is non-nil but User is nil) or panics (if Sender is nil).
5. `IsAuthorized(0, channelID, allowedUsers)` returns false — auth rejected.
6. The message handlers in group 0 would not have fired anyway (no `AllowChannel`).

**Two-part fix required:**

1. Harden auth middleware: when `EffectiveSender` is nil OR `EffectiveSender.Id()` returns 0,
   fall back to `EffectiveChat.Id` for the auth decision. For a single-owner bot where the
   channel IS the auth unit, the channel ID is the correct identity to check.

2. Decide channel post handling policy: either enable `AllowChannel` on message handlers so the
   bot processes channel posts (making the channel a two-way command interface), OR keep channel
   posts as non-interactive (owner posts to channel are not routed to Claude). This is a product
   decision, not just a bug fix.

**Recommended policy:** The bot's design uses channels as project workspaces where the owner
sends messages TO the bot. Channel posts in a pure Telegram channel flow FROM the bot TO
subscribers — this is the opposite direction. The auth failure is happening because the bot is
added as an admin to a channel it also posts into (needed for sending responses), so its own
outbound posts generate `ChannelPost` updates that the auth middleware intercepts. The fix is
to make the auth middleware pass (or skip) updates where the sender is the bot itself, or to
filter out `ChannelPost` updates at the middleware level since the bot does not need to receive
channel posts as commands.

**Table stakes vs differentiator classification:**

| Feature | Classification | Rationale |
|---|---|---|
| Auth does not crash on channel post updates | TABLE STAKES | Any nil pointer dereference is a crash bug, not a feature gap |
| Auth passes updates where sender is the bot itself | TABLE STAKES | Bot's own messages must not trigger auth rejection |
| Explicit ChannelPost filter (skip or route) | TABLE STAKES | Undefined behavior on channel post updates must be resolved |
| Accept commands from channel posts | DIFFERENTIATOR | Would allow channel-as-command-interface pattern; not the v1.0 design |

---

### Bug 2: Long-Poll Timeout Error — HTTP Client Timeout Shorter Than getUpdates Timeout

**What breaks:** The bot logs `context deadline exceeded` errors during normal operation. These
appear periodically even with no user activity. The bot may temporarily stop receiving updates
until the next successful poll cycle.

**Root cause — timeout relationship in long polling:**

Long polling works by sending a `getUpdates` request with a `timeout` parameter (in seconds).
Telegram holds the HTTP connection open for up to that many seconds, then responds with either
an empty array (no updates) or the available updates. This is by design — the connection is
supposed to stay open.

The relationship that must hold:

```
HTTP client timeout > getUpdates timeout parameter
```

If the HTTP client timeout fires before the getUpdates timeout expires, the client closes the
connection before Telegram has finished its server-side wait. This generates a
`context deadline exceeded` error from the HTTP layer.

gotgbot v2's `updater.go` documents this explicitly in `PollingOpts`:

> "Using a non-0 'GetUpdatesOpts.Timeout' value. This is how 'long' telegram will hold the
> long-polling call while waiting for new messages... it is recommended you set your
> PollingOpts.Timeout value to be slightly bigger (eg, +1)" than the HTTP client timeout.

The wording is slightly confusing (it says set PollingOpts.Timeout bigger, but context indicates
it means the HTTP client request timeout should be set to polling timeout + buffer). The intent
is clear: HTTP request timeout = getUpdates timeout + margin.

**Current configuration in bot.go:**

```go
b.updater.StartPolling(b.bot, &ext.PollingOpts{
    DropPendingUpdates: false,
    GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
        Timeout: 10,
    },
})
```

The `GetUpdatesOpts.Timeout` is 10 seconds. No `RequestOpts` with a longer `Timeout` is passed.
gotgbot uses `http.DefaultClient` if no custom client is configured. `http.DefaultClient` has no
timeout (`Timeout: 0`), which means no timeout — this should NOT cause deadline exceeded errors
by itself.

The actual error source is likely `context.Context` propagation: the `StartPolling` call receives
the `ctx` from `b.Start()`, which is derived from the parent context. If that context has a
deadline or is cancelled, all in-flight HTTP requests will fail with `context deadline exceeded`.

**Correct fix:** Pass a `RequestOpts` with `Timeout` set to the polling interval + a generous
buffer (at least 5 seconds). The standard recommended pattern is:

```go
pollingTimeout := 30  // seconds Telegram holds the connection
b.updater.StartPolling(b.bot, &ext.PollingOpts{
    GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
        Timeout: pollingTimeout,
        RequestOpts: &gotgbot.RequestOpts{
            Timeout: time.Duration(pollingTimeout+5) * time.Second,
        },
    },
})
```

A `getUpdates` timeout of 30 seconds with an HTTP timeout of 35 seconds gives Telegram the full
window to respond while ensuring the HTTP layer closes stale connections if Telegram is unreachable.
The current value of 10 seconds for `GetUpdatesOpts.Timeout` is on the low end; 30 seconds is
the community standard for stable long-polling bots.

**Table stakes vs differentiator classification:**

| Feature | Classification | Rationale |
|---|---|---|
| No spurious context deadline exceeded errors during polling | TABLE STAKES | Periodic errors in production = unstable service |
| HTTP client timeout set correctly relative to polling timeout | TABLE STAKES | Fundamental long-poll configuration requirement |
| Polling timeout increased from 10s to 30s | TABLE STAKES | 10s generates 6x more getUpdates requests per minute; 30s is standard |
| Polling timeout configurable via env var | DIFFERENTIATOR | Nice-to-have for tuning; not blocking |

---

## Chat Type Behavior Reference

This table documents expected Telegram Bot API behavior per chat type for bot-received updates.
Use this when debugging auth, sender extraction, or handler routing.

| Scenario | Update Field | `from` (User) | `sender_chat` (Chat) | `EffectiveSender.Id()` |
|---|---|---|---|---|
| User DM to bot | `Message` | Populated | nil | User ID |
| User in group/supergroup | `Message` | Populated | nil | User ID |
| Anonymous admin in group | `Message` | Fake user (1087968824) | Group chat | Chat ID |
| Channel linked to discussion group (forwarded post) | `Message` | Fake user (777000) | Channel chat | Chat ID |
| Admin posting to channel (bot as admin) | `ChannelPost` | nil | nil (channel is EffectiveChat) | nil or 0 |
| Bot's own outbound message (bot posts to channel) | `ChannelPost` | nil | nil | nil or 0 |
| Callback query (inline keyboard button) | `CallbackQuery` | Populated | nil | User ID |

**Key rule:** In a `ChannelPost` update, the `from` field is absent per Telegram API spec.
`EffectiveSender` will be nil or have `Id() == 0`. Do not call `EffectiveSender.Id()` without
a nil guard on ChannelPost updates.

---

## Anti-Features for v1.1

| Anti-Feature | Why Tempting | Why Wrong | Correct Approach |
|---|---|---|---|
| Whitelist the channel ID instead of user ID in IsAuthorized | Would "fix" auth by treating channel as user | Conflates two different auth models; hides the real issue that ChannelPost updates should be filtered, not authed | Filter ChannelPost updates before auth; only auth updates that can have real senders |
| Increase getUpdates timeout to 60+ seconds | Longer poll = fewer requests | Telegram server enforces a maximum; the Telegram Bot API docs recommend positive values but 60s is the upper practical limit; very long timeouts complicate connection health monitoring | Use 30s with a 35s HTTP timeout — standard and documented |
| Set http.Client.Timeout globally to a short value | Seems like good hygiene | Short global timeout kills long-poll requests which are intended to wait | Set per-request timeout in RequestOpts for getUpdates only; leave other requests at default or a shorter value |
| Disable the auth middleware for ChannelPost updates via AllowChannel=false on handlers | Stops the handler from running, which avoids the crash | Middleware still runs; nil dereference still happens; AllowChannel controls handler dispatch, not middleware execution | Fix the middleware nil guard; separately decide on ChannelPost handling policy |

---

## Feature Dependencies (v1.1 changes)

```
[Auth middleware nil guard for EffectiveSender]
    └──must precede──> [ChannelPost routing decision]
                           (nil guard makes the middleware safe regardless of routing choice)

[Polling timeout increase to 30s]
    └──requires──> [RequestOpts.Timeout set to 35s]
                   (always set both together; setting only the Telegram timeout causes errors)
```

---

## v1.0 Feature Landscape (retained for context)

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Text message routing to Claude | Core interaction — without this the bot is useless | LOW | Synchronous text-in, response-out |
| Streaming response with live updates | Any AI bot doing >2s responses needs live feedback; users abandon silent bots | MEDIUM | Edit-in-place pattern: create message, throttle edits at ~500ms, final edit on complete |
| Session persistence across restarts | Users expect conversation continuity; losing context on restart is a regression | MEDIUM | JSON file with session ID; resume on startup |
| /start, /new, /stop, /status commands | Standard Telegram bot conventions; users try these first | LOW | `/start` = info + status; `/new` = clear session; `/stop` = abort running query |
| Auth gate on all handlers | Security baseline — unauthorized users must be rejected, not silently ignored | LOW | Per-channel membership check in the Go version; check on every handler |
| Error reporting back to user | Silent failures erode trust; users need to know when something went wrong | LOW | Error text in channel, truncated to 200 chars |
| Typing indicator while processing | Standard UX signal that the bot is working | LOW | Send chat action "typing" on a goroutine; stop on response complete |
| Rate limiting per channel | Prevents accidental or intentional abuse of Claude API | MEDIUM | Token bucket per channel ID; configurable burst and refill |
| Audit logging | Required for debugging; expected in any production tool | LOW | Append-only log: timestamp, user, action, first 100 chars of message |
| /resume — restore previous session | Claude context is expensive; losing work context is painful | MEDIUM | List saved sessions with inline keyboard; select to resume |

### Differentiators (Competitive Advantage)

Features that set the product apart. Not required, but valuable.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Multi-project with one channel per project | Core value proposition of this rewrite — competitors use a single chat or `/repo` switching | HIGH | One bot, N channels, each channel maintains independent Claude session; channel ID is the project key |
| Dynamic project-channel assignment | Unrecognized channel prompts user to link a project path — zero-config onboarding for new projects | HIGH | On first message from unknown channel: show directory browser inline keyboard; save mapping to JSON |
| GSD workflow integration via inline keyboard menus | The entire GSD command set accessible from a phone without typing slash commands | HIGH | 19 GSD operations mapped to inline keyboard; roadmap context shown with phase progress; contextual next-step buttons extracted from Claude's response |
| Contextual action buttons after responses | Claude's response may contain GSD commands or numbered options — extract them and render as tappable buttons | HIGH | Parse response for `/gsd:*` patterns and numbered lists; buildActionKeyboard() renders them as inline keyboard |
| Context window progress bar | Users can see how close they are to hitting the context limit before it fails | LOW | Parse `context_percent` from Claude CLI stream-json output; render as block bar |
| Query interrupt with `!` prefix | Interrupting a long-running query to redirect is essential for phone-based coding | MEDIUM | Detect `!` prefix on text handler; stop running process, clear stop flag, send new message |
| Voice message transcription | Mobile-first: typing long prompts is painful; voice is faster | MEDIUM | Download OGG from Telegram; send to OpenAI Whisper API; pipe transcript to text handler |
| Photo analysis with album buffering | Take a screenshot of an error and send it for analysis | MEDIUM | Buffer media group messages for 1s timeout to collect album; pass image URLs to Claude |
| PDF document processing | Send spec docs, PRDs, or error reports directly to the bot | MEDIUM | `pdftotext` CLI dependency; extract text, pass as message content |
| Per-project independent Claude sessions | Work on two projects simultaneously without context bleed | HIGH | One `ClaudeSession` (or equivalent) per channel, goroutine-safe |
| Windows Service deployment via NSSM | Runs at boot on Windows dev machines without a terminal window; proper service lifecycle | MEDIUM | NSSM wraps the Go binary; install script sets recovery actions; logs to file |
| Roadmap parsing and display in /gsd | Shows phase progress inline — Phase X/N done, next phase name — without leaving Telegram | MEDIUM | Parse `.planning/ROADMAP.md` checkboxes; display as context in /gsd message |
| ask_user MCP integration | Claude can ask clarifying questions via inline keyboard buttons during a session | HIGH | Watch for ask-user JSON files in temp dir; render options as inline keyboard; write answer back to file |
| Token usage reporting in /status | Shows input/output/cache tokens from last query — useful for cost awareness | LOW | Parse `usage` from stream-json; store on session object; display in /status |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Native Telegram streaming (Bot API) | Would give real live character-by-character streaming | Telegram charges 15% commission on all in-bot Star purchases when enabled; creates revenue dependency on Telegram; only useful for monetized bots | Edit-in-place pattern: throttled message edits simulate streaming without enabling native streaming |
| Database (SQLite/Postgres) for state | Feels more "production" | Adds dependency, schema migrations, backup complexity; JSON files are sufficient for single-user or small team use — and explicitly out of scope per PROJECT.md | JSON file persistence: project-channel mappings, session state, audit log |
| Shared Claude sessions across channels | Reduces Claude session overhead | Destroys the multi-project isolation guarantee — context from Project A leaks into Project B | Strict one-session-per-channel-ID mapping |
| Global user allowlist | Familiar pattern from single-channel bots | Doesn't scale with multi-channel model; who is "allowed" changes per project/channel | Per-channel auth: if you're in the channel, you have access; no global list to maintain |
| Webhook mode for Telegram updates | Lower latency than polling; common recommendation | Adds HTTPS endpoint requirement, certificate management, port exposure on Windows; for a local dev tool, long polling is perfectly adequate | Long polling: simpler, no network config, works behind NAT and firewalls |
| Docker deployment | Reproducible environment | Explicit out-of-scope in PROJECT.md; adds complexity for a Windows Service target; Go produces a single static binary anyway | Native Go binary + NSSM; binary compiles to a single `.exe` |
| Multi-user per channel | Multiple developers sharing one channel | Blurs accountability in audit log; creates conflicting session state (one user stops a query while another is waiting for results) | Per-channel auth accepts any channel member, but sessions are owned by the channel, not the user — first message wins |
| Inline message editing for streaming | Looks slicker — message updates in-place | Telegram rate-limits edits to ~2/s per chat; rapid edits trigger 429 errors and back-off; need throttle logic regardless | Throttle edits at 500ms minimum interval; batch content updates |
| Auto-commit or git push from bot | Convenience for one-command deploys | High risk of pushing broken code or exposing credentials; creates feedback loop without review | Use Claude's existing git tools inside the session; require explicit intent per commit |
| Conversation history search | Useful reference for past decisions | Telegram already provides message search in channels; duplicating it adds storage and complexity | Rely on Telegram's native channel search; audit log covers structured event history |

## Feature Dependencies (v1.0)

```
[Per-channel auth]
    └──required by──> [Multi-project channel mapping]
                          └──required by──> [Independent Claude sessions per channel]
                                                └──required by──> [GSD workflow integration]

[Streaming response with live updates]
    └──required by──> [Contextual action buttons after responses]
                          └──enhances──> [GSD workflow integration]

[Session persistence]
    └──required by──> [/resume command]

[Voice transcription]
    └──depends on──> [OpenAI Whisper API key config]

[PDF processing]
    └──depends on──> [pdftotext CLI on PATH]

[ask_user MCP integration]
    └──depends on──> [MCP server configuration]
    └──depends on──> [Streaming response callback system]

[Roadmap parsing in /gsd]
    └──depends on──> [Multi-project channel mapping] (need to know working dir)

[Contextual action buttons]
    └──depends on──> [Response text parsing (GSD command extraction)]
    └──conflicts──> [Native Telegram streaming] (streaming API owns the message lifecycle)
```

## Sources

- [Telegram Bot API — Message object](https://core.telegram.org/bots/api#message): `from` field documented as "may be empty for messages sent to channels"; `sender_chat` field present for anonymous admin and channel-forwarded messages. HIGH confidence.
- [gotgbot v2 sender.go](https://github.com/PaulSonOfLars/gotgbot/blob/v2/sender.go): `Sender.Id()` prefers `Chat.Id` over `User.Id` for channel/anonymous senders; `IsChannelPost()` checks Chat type and ID match. HIGH confidence.
- [gotgbot v2 ext/handlers/message.go](https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2/ext/handlers): `SetAllowChannel(true)` required for handler to match `ChannelPost` updates; default is false. HIGH confidence.
- [gotgbot v2 ext/updater.go](https://github.com/PaulSonOfLars/gotgbot/blob/v2/ext/updater.go): `PollingOpts` comments recommend HTTP request timeout = getUpdates timeout + buffer. HIGH confidence.
- [pyTelegramBotAPI issue #2810](https://github.com/python-telegram-bot/python-telegram-bot/issues/2810): Anonymous admin user ID 777000 and 1087968824 sentinel values documented in community. MEDIUM confidence.
- [Long Polling — Telegram.Bot .NET guide](https://telegrambots.github.io/book/3/updates/polling.html): HTTP client timeout must exceed getUpdates timeout parameter. MEDIUM confidence.

---
*Feature research for: Go Telegram Bot with multi-project Claude Code integration*
*Researched: 2026-03-20 (v1.1 bugfix update)*
