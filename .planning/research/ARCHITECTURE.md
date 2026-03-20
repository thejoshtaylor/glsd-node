# Architecture Research

**Domain:** Go Telegram Bot with Concurrent Subprocess Management (Multi-Project Claude Code Control)
**Researched:** 2026-03-20 (updated for v1.1 Bugfixes milestone)
**Confidence:** HIGH — derived from direct source reading of gotgbot v2 rc.34 library code, existing bot source, and live test failures

---

## v1.1 Bugfix Integration Architecture

This section supersedes the original v1.0 research for the purposes of the v1.1 milestone. It documents the precise integration points, root causes, and proposed fixes for the two active bugs.

---

### Bug 1: Channel-Type Auth Failures

#### Root Cause (HIGH confidence — verified in gotgbot source)

When Telegram delivers a channel post, the update contains `update.ChannelPost`, not `update.Message`. In gotgbot's `ext/context.go` (`NewContext`), the switch on update type sets:

```go
case update.ChannelPost != nil:
    msg = update.ChannelPost
    chat = &update.ChannelPost.Chat
    // user is NOT set — remains nil
```

Because `user == nil` and `sender == nil` at that point, the fall-through logic at the end of `NewContext` calls `msg.GetSender()`. `GetSender()` (in `sender.go`) returns:

```go
&Sender{
    User:               m.From,       // nil for channel posts — no human sender
    Chat:               m.SenderChat, // the channel itself (non-nil)
    IsAutomaticForward: m.IsAutomaticForward,
    ChatId:             m.Chat.Id,
}
```

`Sender.Id()` prefers `Chat.Id` when `Chat != nil`, so `ctx.EffectiveSender.Id()` returns the channel's chat ID (a large negative int64 like `-1001234567890`), not a user ID.

In `authMiddlewareWith` (`internal/bot/middleware.go`), the current code extracts `userID` from `EffectiveSender.Id()` and passes it to `security.IsAuthorized`, which checks it against `allowedUsers []int64`. The channel chat ID is never in that list, so every channel post fails auth.

Note: `EffectiveSender` is never nil for channel posts — it is always populated via `msg.GetSender()`. The existing nil guard in `authMiddlewareWith` (`if ctx.EffectiveSender != nil`) does not trigger; the problem is that the ID it returns is wrong for the channel-as-sender case.

#### Where to Change

**Primary change: `internal/bot/middleware.go` — `authMiddlewareWith`**

The `authMiddlewareWith` function must distinguish between two cases before extracting a user ID:

- **User sender** (`EffectiveSender.IsUser() == true`): use `EffectiveSender.Id()` — this is a real user ID.
- **Channel/anonymous sender** (`EffectiveSender.IsChannelPost() == true` or `IsAnonymousAdmin()` etc.): fall back to channel-level auth using `channelID` alone, bypassing user-list check.

The channel-level auth path should consult `security.IsAuthorized` with the channel's own ID as both `userID` and `channelID`, or — more explicitly — add a second function `security.IsChannelAuthorized(channelID int64, allowedChannels []int64) bool` so the intent is unambiguous.

**Secondary change: `internal/security/validate.go` — `IsAuthorized`**

The comment "Phase 2 will add per-channel membership auth" is now the active work. `IsAuthorized` (or a new sibling `IsChannelAuthorized`) must accept a list of allowed channel IDs. The config layer must also expose an `AllowedChannels []int64` field, or re-use `AllowedUsers` by treating negative channel IDs as allowed principals.

The simplest approach that requires the least config change: treat a principal (user or channel) as authorized if their ID appears in the `AllowedUsers` list, regardless of whether it's a user ID or channel ID. Channel IDs are negative int64 values; user IDs are positive. No existing test would break. The operator adds their channel's numeric ID to `TELEGRAM_ALLOWED_USERS`.

**No change required: `internal/bot/middleware.go` — rate limit middleware**

Rate limiting already extracts `channelID` from `ctx.EffectiveChat.Id` independent of sender. No fix needed.

#### Middleware Chain Impact

The dispatcher group order is unchanged. The auth middleware at group -2 becomes channel-aware but its position and interface (`AuthChecker`) stay the same. The `defaultAuthChecker.IsAuthorized` wrapper in `middleware.go` calls `security.IsAuthorized` — this call site changes to pass the correct principal ID.

```
Group -2  authMiddleware        ← MODIFIED: sender-type-aware ID extraction
Group -1  rateLimitMiddleware   ← unchanged
Group  0  all handlers          ← unchanged
```

#### Test Impact

**`internal/bot/middleware_test.go`**

Existing tests use `buildContext(userID, chatID)` which constructs a `Sender{User: user}` — these are user-sender scenarios and continue to pass unchanged.

New tests must be added:

1. `TestMiddlewareAuthAllowsChannelSender` — build a context where `EffectiveSender.Chat` is non-nil and `EffectiveSender.Chat.Id` matches an allowed ID; verify next handler is called.
2. `TestMiddlewareAuthRejectsUnknownChannel` — same but the channel ID is not in the allowed list; verify `EndGroups`.
3. `TestMiddlewareAuthChannelPostNilUser` — simulate exactly what gotgbot produces for a channel post: `Sender{User: nil, Chat: &Chat{Id: channelID}}`.

A helper `buildChannelContext(channelID int64)` is needed, producing an `ext.Context` where:
- `EffectiveSender = &gotgbot.Sender{Chat: &gotgbot.Chat{Id: channelID, Type: "channel"}, ChatId: channelID}`
- `EffectiveChat = &gotgbot.Chat{Id: channelID}`
- `EffectiveMessage = &gotgbot.Message{Chat: gotgbot.Chat{Id: channelID}}` (no `From` field)

**`internal/security/validate_test.go`**

If `IsAuthorized` is extended or a new `IsChannelAuthorized` is added, corresponding unit tests must cover the negative-ID case.

---

### Bug 2: getUpdates Polling Timeout (context deadline exceeded)

#### Root Cause (HIGH confidence — verified in gotgbot source)

Two independent timeout clocks exist:

1. **Telegram server-side long-poll timeout**: `GetUpdatesOpts.Timeout = 10` (seconds). This tells the Telegram server to hold the HTTP connection open for up to 10 seconds waiting for new updates, then respond with an empty array.

2. **HTTP client context timeout**: `BaseBotClient` in gotgbot uses `DefaultTimeout = 5 * time.Second` for all requests unless overridden. The `pollingLoop` in `ext/updater.go` calls `bot.RequestWithContext(ctx, "getUpdates", v, opts)` using the `reqOpts` extracted from `GetUpdatesOpts.RequestOpts`. Since `GetUpdatesOpts.RequestOpts` is currently `nil` in the bot's `StartPolling` call, the `BaseBotClient.getTimeoutContext` falls through to `DefaultTimeout = 5s`.

The HTTP client cancels the connection at 5 seconds; the server is holding it for 10 seconds. The result is `context deadline exceeded` on every poll cycle. The bot still works (the loop retries) but generates constant error logs and wastes a round-trip per cycle.

The gotgbot documentation in `updater.go` (lines 93-95) explicitly notes this:

> "If you are seeing lots of 'context deadline exceeded' errors on GetUpdates, this is likely the cause. Keep in mind that a timeout of 10 does not mean you only get updates every 10s... When setting this, it is recommended you set your PollingOpts.Timeout value to be slightly bigger (eg, +1)."

The comment says `PollingOpts.Timeout` but what it actually means is `GetUpdatesOpts.RequestOpts.Timeout` — the HTTP-level timeout passed to the bot client.

#### Where to Change

**Primary change: `internal/bot/bot.go` — `New` function**

The `gotgbot.NewBot` call currently passes `nil` for opts:

```go
tgBot, err := gotgbot.NewBot(cfg.TelegramToken, nil)
```

To configure a longer HTTP timeout for polling, pass a `BotOpts` with a `BotClient` that has an appropriately large `http.Client.Timeout`, or use `DefaultRequestOpts`.

**Option A (preferred): Configure `BaseBotClient` with `DefaultRequestOpts`**

```go
tgBot, err := gotgbot.NewBot(cfg.TelegramToken, &gotgbot.BotOpts{
    BotClient: &gotgbot.BaseBotClient{
        Client: http.Client{},
        DefaultRequestOpts: &gotgbot.RequestOpts{
            Timeout: 30 * time.Second, // covers any long-poll timeout up to 29s
        },
    },
})
```

`DefaultRequestOpts.Timeout` is used by `getTimeoutContext` when no per-request opts override it. This sets a 30-second context deadline for all API calls, which is more than sufficient for a 10-second long-poll.

**Option B: Override per-poll via `GetUpdatesOpts.RequestOpts`**

```go
b.updater.StartPolling(b.bot, &ext.PollingOpts{
    DropPendingUpdates: false,
    GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
        Timeout: 10,
        RequestOpts: &gotgbot.RequestOpts{
            Timeout: 15 * time.Second, // long-poll timeout (10s) + 5s buffer
        },
    },
})
```

This is the approach gotgbot's own documentation recommends and is more targeted — only the polling loop gets the extended timeout; all other API calls retain the default 5-second timeout.

**Recommendation: Option B**. It is the approach documented in gotgbot's own comments, affects only the polling loop, and is the minimal change. Option A changes the timeout for all API operations including message sends, which is unnecessary.

**No change to `BotOpts`/`BaseBotClient` needed** unless the project also wants to override the `GetMe` startup timeout (currently using a hardcoded 10s in gotgbot's `NewBot`).

#### Middleware Chain Impact

None. This is a pure `bot.go` change at the constructor/startup level. No middleware, handlers, or session code is affected.

#### Test Impact

This is an infrastructure configuration fix. There are no unit tests for `New()` currently, and the polling loop cannot be unit tested without a live Telegram connection. The fix is verified by observing the absence of `context deadline exceeded` log lines in production.

If the project wishes to add a construction test, it can verify that `tgBot.BotClient` is of type `*gotgbot.BaseBotClient` and that its `DefaultRequestOpts.Timeout` is non-zero. This is a low-value test and not required.

---

## Integration Points Summary (v1.1)

### Modified Files

| File | Change Type | What Changes |
|------|-------------|--------------|
| `internal/bot/middleware.go` | Modified | `authMiddlewareWith`: use `EffectiveSender.IsUser()` to branch between user-ID and channel-ID auth paths |
| `internal/security/validate.go` | Modified or extended | `IsAuthorized`: accept negative channel IDs in allowed list, or add `IsChannelAuthorized` |
| `internal/bot/bot.go` | Modified | `Start`: add `GetUpdatesOpts.RequestOpts` with `Timeout: 15 * time.Second` |

### New Files

None required for either fix.

### Test Files Requiring New Cases

| File | New Tests |
|------|-----------|
| `internal/bot/middleware_test.go` | `buildChannelContext`, `TestMiddlewareAuthAllowsChannelSender`, `TestMiddlewareAuthRejectsUnknownChannel`, `TestMiddlewareAuthChannelPostNilUser` |
| `internal/security/validate_test.go` | Tests for channel ID (negative int64) in allowed list |

---

## Build Order (v1.1)

Dependencies between the two fixes are independent — either can be built first.

**Recommended order:**

1. **HTTP timeout fix** (`bot.go`) — one-line change, no tests to write, immediate value. Eliminates log noise for all subsequent testing.

2. **Channel auth fix** — requires understanding the Sender type system, new test helper, potentially a `security` package change. Build second so the test environment is clean.

Ordering rationale: The HTTP timeout fix is pure configuration with no logic branches. Getting it done first removes a source of spurious log output before investigating auth failures, making auth debugging cleaner.

---

## Data Flow Changes for Channel Auth

### Before Fix: Channel Post Auth Flow

```
ChannelPost update arrives
    ↓
gotgbot NewContext:
    msg = update.ChannelPost
    chat = &update.ChannelPost.Chat
    user = nil
    sender = msg.GetSender()
        → Sender{User: nil, Chat: &Chat{Id: -100xxx}, ChatId: -100xxx}
    ↓
authMiddlewareWith:
    userID = ctx.EffectiveSender.Id()
           = sender.Chat.Id          ← channel ID (negative)
    channelID = ctx.EffectiveChat.Id ← same channel ID
    ↓
security.IsAuthorized(-100xxx, -100xxx, allowedUsers)
    → iterates allowedUsers (positive user IDs)
    → no match → returns false
    ↓
Auth rejected. EndGroups. Bot ignores the message.
```

### After Fix: Channel Post Auth Flow

```
ChannelPost update arrives
    ↓
gotgbot NewContext: (unchanged)
    sender = Sender{User: nil, Chat: &Chat{Id: -100xxx}}
    ↓
authMiddlewareWith (MODIFIED):
    if ctx.EffectiveSender.IsUser() {
        principalID = ctx.EffectiveSender.Id()  ← user ID path
    } else {
        principalID = ctx.EffectiveChat.Id       ← channel ID path
    }
    ↓
security.IsAuthorized(principalID, channelID, allowedUsers)
    → allowedUsers contains -100xxx (operator added channel ID to config)
    → match → returns true
    ↓
Auth passes. Processing continues to group -1 (rate limit) and group 0 (handlers).
```

---

## Gotgbot Sender Type Reference

For channel-aware auth, the relevant `Sender` classification methods (from `sender.go`) are:

| Method | Returns true when |
|--------|-------------------|
| `IsUser()` | `Chat == nil && User != nil` — a real human or bot user |
| `IsBot()` | `Chat == nil && User != nil && User.IsBot` |
| `IsChannelPost()` | `Chat != nil && Chat.Id == ChatId && Chat.Type == "channel"` — admin posting in their own channel |
| `IsAnonymousAdmin()` | `Chat != nil && Chat.Id == ChatId && Chat.Type != "channel"` — anonymous group admin |
| `IsAnonymousChannel()` | `Chat != nil && Chat.Id != ChatId && Chat.Type == "channel"` — channel sending to a linked group |

For the auth fix, the gate is `IsUser()`. Any sender where `IsUser() == false` is a non-human principal (channel, anonymous admin) and should be authed by `EffectiveChat.Id` against an allowed-channels list, not by a personal user ID.

---

## Existing Architecture (Unchanged)

The system overview, component responsibilities, project structure, patterns, and scaling considerations from the v1.0 research remain valid. The v1.1 fixes are surgical changes within existing components and do not alter any architectural boundaries.

Key invariants that remain unchanged:
- Middleware group order: auth (-2), rate limit (-1), handlers (0)
- `AuthChecker` interface: `IsAuthorized(userID int64, channelID int64) bool` — signature unchanged, interpretation of `userID` broadened
- `defaultAuthChecker` wrapper: updated to pass correct principal ID
- `ext.Updater` / `ext.Dispatcher` model: unchanged
- Session, persistence, streaming layers: unaffected

---

## Sources

- `internal/bot/middleware.go` — current auth middleware source (direct read)
- `internal/bot/bot.go` — current `New` and `Start` functions (direct read)
- `internal/security/validate.go` — `IsAuthorized` implementation (direct read)
- `internal/bot/middleware_test.go` — existing test coverage (direct read)
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/ext/context.go` — `NewContext` switch, sender population logic (direct read)
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/sender.go` — `GetSender`, `Id`, `IsUser`, `IsChannelPost` methods (direct read)
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/request.go` — `DefaultTimeout = 5s`, `BaseBotClient`, `getTimeoutContext` (direct read)
- `~/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/ext/updater.go` — `pollingLoop`, `PollingOpts` documentation comment on timeout mismatch (direct read)

---
*Architecture research for: Go Telegram Bot — v1.1 Bugfixes (channel auth + polling timeout)*
*Researched: 2026-03-20*
