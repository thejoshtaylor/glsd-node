# Phase 9: Channel Auth - Research

**Researched:** 2026-03-20
**Domain:** Go / gotgbot v2 channel authorization and echo-loop prevention
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Authorize channels by admin lookup: call `GetChatAdministrators()` and check if any admin's user ID is in the `TELEGRAM_ALLOWED_USERS` list
- Cache admin lookup results with a 15-minute TTL to avoid repeated API calls
- The bot must be an admin in the channel for the lookup to work
- Anonymous admin senders in authorized channels pass auth (channel is authorized via admin lookup)
- Check lives in `security.IsAuthorized()` or a new channel-auth helper called from middleware
- Reply with rejection message in unauthorized channels (same behavior as DMs)
- Audit log all rejected channel messages with channel ID
- 15-minute cache TTL for admin lookup results
- Compare `msg.From.Id` to `bot.Id` to detect bot's own reflected messages
- Filter in auth middleware before auth check for cheapest early exit
- Debug-level log only for echo-filtered messages (not audit-worthy)
- Also filter `msg.IsAutomaticForward` to prevent double-processing of linked-channel forwards

### Claude's Discretion
- Internal implementation details of the admin cache (sync.Map, expiry struct, etc.)
- Exact placement of echo check within the middleware function
- Test structure and coverage approach

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| AUTH-01 | Bot correctly authorizes messages in Telegram channels where sender is the channel itself (non-user sender) | `Sender.IsChannelPost()` detects channel-as-sender; `GetChatAdministrators()` resolves channel identity to admin user IDs; cached `ChannelAuthCache` feeds updated `IsAuthorized()` logic |
| AUTH-02 | Bot filters its own messages reflected back as ChannelPost updates without processing them | `tgBot.Id` (bot's own user ID) compared against `msg.From.Id` for echo detection; `Sender.IsAutomaticForward` catches linked-channel forwards; early-exit in middleware before auth check |
</phase_requirements>

## Summary

Phase 9 fixes two related channel authorization defects in the Go rewrite. The first defect (AUTH-01) is that channel posts arrive with `SenderChat` populated and `From` nil (or a dummy user), so the current `IsAuthorized(userID, ...)` check against the user allowlist always fails — the channel itself has no user ID in the list. The fix is: when the sender is a channel post (`Sender.IsChannelPost()`), look up the channel's administrators via `GetChatAdministrators()`, check whether any admin's user ID is in `AllowedUsers`, and cache the result for 15 minutes.

The second defect (AUTH-02) is an echo loop: after the bot posts a message into a channel, Telegram reflects that message back as a ChannelPost update. The bot then re-processes its own output. The fix is a cheapest-first early exit in auth middleware: if the sender is the bot itself (`Sender.IsBot()` or `msg.From.Id == tgBot.Id`), or if `Sender.IsLinkedChannel()` (automatic forward), drop the update immediately with a debug-level log and return `ext.EndGroups`.

Both fixes land entirely in the middleware/security layer. No handler code changes are required.

**Primary recommendation:** Add an `IsChannelAuthorized(botClient, channelID, allowedUsers, cache)` helper in `internal/security`; wire it into `authMiddlewareWith` before the existing user-ID check; add echo-filter as the very first guard in `authMiddlewareWith`.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.34 | Telegram Bot API client | Already in use; provides `GetChatAdministrators`, `Sender`, `ChatMember` |
| Go standard library `sync` | stdlib | `sync.Map` or mutex-guarded map for cache | No external dependency needed |
| Go standard library `time` | stdlib | TTL tracking via `time.Time` expiry fields | No external dependency needed |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/rs/zerolog` | v1.34.0 | Structured logging | Already in use; use `log.Debug()` for echo-filtered messages |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `sync.Map` with expiry struct | `golang.org/x/sync/singleflight` | singleflight prevents thundering herd on cache miss, but adds a dependency; `sync.Map` is sufficient given 15-min TTL and low channel count |
| In-process cache | Redis/external cache | External cache adds ops complexity; unnecessary for a single-operator bot |

**Installation:**

No new packages needed. All dependencies are already in `go.mod`.

**Version verification:** gotgbot v2.0.0-rc.34 confirmed present in module cache at `C:/Users/jtayl/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.34/`.

## Architecture Patterns

### Recommended Project Structure

No new directories needed. Changes are confined to:

```
internal/
├── security/
│   ├── validate.go          # Extend IsAuthorized(); add ChannelAuthCache
│   └── validate_test.go     # Add channel auth tests
└── bot/
    ├── middleware.go         # Add echo filter + channel auth branch in authMiddlewareWith
    └── middleware_test.go    # Add channel/echo tests
```

### Pattern 1: Channel Sender Detection via Sender.IsChannelPost()

**What:** gotgbot's `Sender` struct provides `IsChannelPost()` to detect when a channel is posting in its own feed. This is the reliable gate — not a nil check on `From`.

**When to use:** In `authMiddlewareWith`, after the echo filter, when `ctx.EffectiveSender != nil && ctx.EffectiveSender.IsChannelPost()`.

**Example:**
```go
// Source: gotgbot v2.0.0-rc.34 sender.go
// IsChannelPost returns true if the Sender is a channel admin posting to that same channel.
func (s Sender) IsChannelPost() bool {
    return s.Chat != nil && s.Chat.Id == s.ChatId && s.Chat.Type == "channel"
}
```

The channel's own ID is available as `ctx.EffectiveSender.Chat.Id` (same as `ctx.EffectiveChat.Id`).

### Pattern 2: Echo Detection via Sender.IsBot() and IsLinkedChannel()

**What:** gotgbot's `Sender` struct provides `IsBot()` to detect bot-as-sender, and `IsLinkedChannel()` for automatic forwards from a linked channel.

**When to use:** As the first guard in `authMiddlewareWith`, before any auth logic.

**Example:**
```go
// Source: gotgbot v2.0.0-rc.34 sender.go

// IsBot returns true if the Sender is a bot.
func (s Sender) IsBot() bool {
    return s.Chat == nil && s.User != nil && s.User.IsBot
}

// IsLinkedChannel returns true if the Sender is a linked channel sending to the group it is linked to.
func (s Sender) IsLinkedChannel() bool {
    return s.Chat != nil && s.Chat.Id != s.ChatId && s.IsAutomaticForward
}
```

For the echo case specifically, compare `ctx.EffectiveSender.User.Id == tgBot.Id` as the canonical check — `tgBot` is the `*gotgbot.Bot` passed into `HandleUpdate`. `IsBot()` alone would catch any bot, not just this bot.

### Pattern 3: GetChatAdministrators() for Channel Auth Lookup

**What:** `bot.GetChatAdministrators(chatId, nil)` returns `[]ChatMember`. Each `ChatMember` exposes `GetUser() User`. Iterate and compare `member.GetUser().Id` against `allowedUsers`.

**When to use:** On cache miss for a given channel ID.

**Example:**
```go
// Source: gotgbot v2.0.0-rc.34 gen_methods.go line 2450
// GetChatAdministrators returns an Array of ChatMember objects (excluding bots per API docs note).
// NOTE: API docs say "which aren't bots" but in practice the bot itself may appear.
// Safe to iterate all and compare user IDs.
admins, err := tgBot.GetChatAdministrators(channelID, nil)
if err != nil {
    // treat as unauthorized; log warning
    return false
}
for _, member := range admins {
    user := member.GetUser()
    for _, allowed := range allowedUsers {
        if user.Id == allowed {
            return true
        }
    }
}
return false
```

### Pattern 4: TTL Cache with sync.Map

**What:** A simple expiry-entry cache using `sync.Map` so concurrent middleware goroutines don't race.

**When to use:** Wrap the `GetChatAdministrators` call. Store `cacheEntry{authorized bool, expiresAt time.Time}`. On lookup: if entry exists and not expired, return cached value; otherwise call API, store new entry.

**Example:**
```go
type cacheEntry struct {
    authorized bool
    expiresAt  time.Time
}

type ChannelAuthCache struct {
    m   sync.Map
    ttl time.Duration
}

func (c *ChannelAuthCache) Get(channelID int64) (authorized bool, ok bool) {
    v, loaded := c.m.Load(channelID)
    if !loaded {
        return false, false
    }
    entry := v.(cacheEntry)
    if time.Now().After(entry.expiresAt) {
        c.m.Delete(channelID)
        return false, false
    }
    return entry.authorized, true
}

func (c *ChannelAuthCache) Set(channelID int64, authorized bool) {
    c.m.Store(channelID, cacheEntry{
        authorized: authorized,
        expiresAt:  time.Now().Add(c.ttl),
    })
}
```

### Pattern 5: Wiring into AuthChecker Interface

**What:** The existing `AuthChecker` interface has `IsAuthorized(userID, channelID int64) bool`. The `defaultAuthChecker` wraps `security.IsAuthorized`. To add channel lookup, the checker needs access to the `*gotgbot.Bot` client and the cache.

**Key decision:** The `authMiddlewareWith` function takes an `AuthChecker` interface — but `AuthChecker.IsAuthorized` has no access to the Telegram bot client needed for `GetChatAdministrators`. Two viable approaches:

**Option A — Extend AuthChecker** (recommended by CONTEXT.md "or a new channel-auth helper called from middleware"): Add a `ChannelAuthChecker` interface or extend `defaultAuthChecker` to hold `botClient` and `cache`. The `authMiddlewareWith` handle func already receives `tgBot *gotgbot.Bot` — pass it or the cache lookup function into a new struct that satisfies `AuthChecker`.

**Option B — Two-phase check in middleware handle func**: Keep `AuthChecker` unchanged. In the `handle` func, after `checker.IsAuthorized(userID, channelID)` returns false, check if sender is a channel post and call a separate `channelAuthLookup(tgBot, channelID, allowedUsers, cache)` function directly. This avoids changing the interface.

Option B is lower-risk (no interface change, no test mock changes needed) and matches "additive branch" from STATE.md accumulated decisions.

### Anti-Patterns to Avoid

- **Checking `msg.From == nil`** to detect channel posts: gotgbot populates `From` with a dummy user for backward compatibility. Use `Sender.IsChannelPost()` instead.
- **Using `IsChannelPost()` alone for echo filtering**: `IsChannelPost()` only checks that the chat is a channel — it doesn't confirm the sender is THIS bot. Compare `User.Id == tgBot.Id`.
- **Calling `GetChatAdministrators` on every message**: The API will rate-limit the bot. Always check cache first.
- **Storing the full admin list in cache**: Store only the boolean result `authorized`. The admin list is stale after 15 minutes anyway and only the boolean matters.
- **Changing the `AuthChecker` interface** unnecessarily: The mock in `middleware_test.go` would need updating. Prefer the additive-branch pattern.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Channel sender detection | Custom `From == nil` nil checks | `Sender.IsChannelPost()` | gotgbot already encodes all edge cases (SenderChat, ChatId comparison, type check) |
| Bot-sender detection | Custom `User.IsBot && User.Username == botUsername` | `Sender.IsBot()` + `User.Id == tgBot.Id` | gotgbot handles dummy-user edge cases; ID comparison is authoritative |
| Admin lookup | Custom Telegram HTTP call | `bot.GetChatAdministrators(chatId, nil)` | Already available in the pinned gotgbot version |
| Concurrent map access | Mutex-guarded plain map | `sync.Map` | Race-free without explicit lock management |

**Key insight:** gotgbot's `Sender` type was designed exactly for these cases. The library authors wrote `IsChannelPost`, `IsAnonymousAdmin`, `IsLinkedChannel`, etc. specifically because these distinctions are tricky from raw `Update` fields.

## Common Pitfalls

### Pitfall 1: GetChatAdministrators Returns "non-bot" Admins Only (per API docs) — But Bot Itself May Appear

**What goes wrong:** The Telegram API documentation says `GetChatAdministrators` returns admins "which aren't bots." In practice, the calling bot itself often appears in the list. Code that assumes the bot is never in the list may miss filtering it out.

**Why it happens:** The API doc note is about the purpose (don't use it to list human admins), not a strict exclusion. The bot appears as `ChatMemberAdministrator` when it was explicitly added as admin.

**How to avoid:** Iterate all returned members and compare `user.Id` — don't filter by `IsBot` before comparing. The auth check will correctly match the bot's own ID against `AllowedUsers` (which won't contain it, so no security regression).

**Warning signs:** Channel auth passes for a channel where the only "admin" in the lookup result is the bot itself.

### Pitfall 2: Anonymous Admin vs. Channel Post Confusion

**What goes wrong:** `Sender.IsAnonymousAdmin()` (admin posting anonymously in a group) and `Sender.IsChannelPost()` (channel posting in its own feed) have different `Chat.Id == s.ChatId` conditions. Mixing them up causes either missed channel posts or false positives in groups.

**Why it happens:** Both cases have `s.Chat != nil`. The difference is whether `Chat.Id == ChatId` (same chat = channel post or anon admin) and `Chat.Type == "channel"`.

**How to avoid:** Use the exact method from gotgbot. Do not replicate the condition manually. `IsChannelPost()` is the right gate for AUTH-01.

**Warning signs:** Auth works in DMs but anonymous-admin group messages get incorrectly treated as channel posts.

### Pitfall 3: Echo Filter Must Run Before Cache Lookup

**What goes wrong:** If the echo filter is placed after the cache lookup, the bot's own messages will trigger an admin lookup for the channel, dirtying the cache unnecessarily and wasting an API call on first occurrence.

**Why it happens:** Ordering concern — cheapest check first.

**How to avoid:** Structure the middleware handle func as:
1. Echo filter (sender is this bot OR sender is linked channel automatic forward) → `ext.EndGroups`
2. Channel post check → cache lookup or API call
3. Normal user auth check

**Warning signs:** Bot's own channel messages appear in audit log as auth rejections (they should be debug-only).

### Pitfall 4: IsAuthorized Signature Already Has channelID — But the Body Ignores It

**What goes wrong:** `security.IsAuthorized` has the signature `IsAuthorized(userID, channelID int64, allowedUsers []int64)` but the body only checks `userID`. Adding channel-based auth by extending this function's body is one path, but the function has no access to a bot client for API calls.

**Why it happens:** The signature was future-proofed but the implementation was not extended.

**How to avoid:** Do NOT add the API call inside `security.IsAuthorized`. That function belongs in a pure, testable package with no I/O. Instead, implement the lookup in a `bot`-layer helper (or a new `security.ChannelAuthCache` struct that accepts results from the API layer). The middleware handle func orchestrates: call cache/API → pass result to `IsAuthorized` by constructing a synthetic authorized user ID, or simply bypass `IsAuthorized` for the channel case with a direct boolean.

**Warning signs:** `internal/security` package imports `github.com/PaulSonOfLars/gotgbot/v2` — that would be a layer violation.

### Pitfall 5: The tgBot Parameter Is Nil in Unit Tests

**What goes wrong:** `authMiddlewareWith` already guards `if tgBot != nil && ctx.EffectiveMessage != nil` before calling `Reply`. The new channel auth lookup also calls `tgBot.GetChatAdministrators()`. The same nil guard pattern is required, or tests will panic.

**Why it happens:** Unit tests pass `nil` for `tgBot` to avoid a live Telegram connection.

**How to avoid:** In the channel auth lookup path, check `if tgBot == nil { return false }` before calling any bot methods. The mock `AuthChecker` handles the test case; the bot client nil-guard handles the rest.

## Code Examples

Verified patterns from gotgbot v2.0.0-rc.34 source:

### Sender Type Detection
```go
// Source: gotgbot v2.0.0-rc.34 sender.go

// Detect echo (bot's own channel post reflected back):
if sender := ctx.EffectiveSender; sender != nil {
    if sender.IsBot() && sender.User != nil && sender.User.Id == tgBot.Id {
        log.Debug().Int64("bot_id", tgBot.Id).Msg("echo filtered: bot's own channel post")
        return ext.EndGroups
    }
    if sender.IsLinkedChannel() {
        log.Debug().Msg("echo filtered: automatic forward from linked channel")
        return ext.EndGroups
    }
}

// Detect channel post (channel is the sender):
if ctx.EffectiveSender.IsChannelPost() {
    channelID := ctx.EffectiveSender.Chat.Id
    // ... proceed to admin lookup
}
```

### GetChatAdministrators Call
```go
// Source: gotgbot v2.0.0-rc.34 gen_methods.go
// Signature: GetChatAdministrators(chatId int64, opts *GetChatAdministratorsOpts) ([]ChatMember, error)

admins, err := tgBot.GetChatAdministrators(channelID, nil)
if err != nil {
    log.Warn().Err(err).Int64("channel_id", channelID).Msg("channel admin lookup failed")
    return false
}
for _, member := range admins {
    user := member.GetUser() // GetUser() is on the ChatMember interface
    for _, allowed := range allowedUsers {
        if user.Id == allowed {
            return true
        }
    }
}
return false
```

### ChannelAuthCache (discretionary implementation)
```go
// Source: standard library sync + time

type cacheEntry struct {
    authorized bool
    expiresAt  time.Time
}

type ChannelAuthCache struct {
    m   sync.Map
    ttl time.Duration
}

func NewChannelAuthCache(ttl time.Duration) *ChannelAuthCache {
    return &ChannelAuthCache{ttl: ttl}
}

func (c *ChannelAuthCache) Lookup(channelID int64) (authorized bool, hit bool) {
    v, ok := c.m.Load(channelID)
    if !ok {
        return false, false
    }
    entry := v.(cacheEntry)
    if time.Now().After(entry.expiresAt) {
        c.m.Delete(channelID)
        return false, false
    }
    return entry.authorized, true
}

func (c *ChannelAuthCache) Store(channelID int64, authorized bool) {
    c.m.Store(channelID, cacheEntry{
        authorized: authorized,
        expiresAt:  time.Now().Add(c.ttl),
    })
}
```

### Updated defaultAuthChecker for Channel Support
```go
// Pattern: additive branch in defaultAuthChecker — does not change AuthChecker interface

type defaultAuthChecker struct {
    allowedUsers  []int64
    channelCache  *security.ChannelAuthCache  // or defined in bot package
    botClient     *gotgbot.Bot                // needed for GetChatAdministrators
}

func (a *defaultAuthChecker) IsAuthorized(userID int64, channelID int64) bool {
    // Normal user check (unchanged path)
    if security.IsAuthorized(userID, channelID, a.allowedUsers) {
        return true
    }
    // Channel post: userID will be the channel's chat ID (Sender.Id() returns Chat.Id)
    // For channel posts, try admin lookup
    // NOTE: caller (middleware) has already confirmed sender.IsChannelPost() before calling
    // this path, so we know this is a channel-origin message.
    // Channel posts set Chat != nil, so userID == channelID from Sender.Id()
    // The channel lookup is keyed on channelID.
    return false // planner will resolve exact split
}
```

**Note for planner:** The exact split of responsibility (which layer calls the API, which layer holds the cache, how to pass bot client into the checker) is Claude's Discretion. The recommended approach is Option B from Pattern 5: keep `AuthChecker` interface unchanged, add a second conditional block in the middleware `handle` func after the existing `checker.IsAuthorized` call.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Check `msg.From == nil` for channel detection | Use `Sender.IsChannelPost()` | gotgbot introduced `Sender` type in v2 | Reliable across all channel/group/anon-admin cases |
| No echo filtering | Filter on `Sender.IsBot() && User.Id == tgBot.Id` | Phase 9 | Prevents reprocessing bot's own channel messages |

**Deprecated/outdated:**

- `msg.From == nil` as channel-post indicator: `From` may be a dummy user, not nil. The `Sender` abstraction handles this.
- Checking `msg.IsAutomaticForward` directly on `Message`: Available on the raw `Message` struct, but `Sender.IsLinkedChannel()` encapsulates the same condition more clearly.

## Open Questions

1. **Where exactly does `ChannelAuthCache` live?**
   - What we know: CONTEXT.md says "or a new channel-auth helper called from middleware"; `internal/security` is the natural home but cannot import gotgbot
   - What's unclear: Whether the cache struct lives in `internal/security` (pure, no API calls) with the lookup function in `internal/bot`, or entirely in `internal/bot`
   - Recommendation: Define `ChannelAuthCache` in `internal/security` (no gotgbot import needed — it's just a sync.Map cache), implement the API call in `internal/bot/middleware.go` as a package-level `lookupChannelAdmins(tgBot, channelID, allowedUsers, cache)` helper.

2. **How does `defaultAuthChecker` gain access to the bot client?**
   - What we know: `b.authMiddleware(authPass)` creates `defaultAuthChecker{allowedUsers: b.cfg.AllowedUsers}` — no bot client reference today
   - What's unclear: Whether to add `botClient *gotgbot.Bot` to `defaultAuthChecker`, or pass `tgBot` from the middleware handle func into a separate helper
   - Recommendation: Option B (additive branch in handle func) avoids touching `defaultAuthChecker` and the `AuthChecker` interface. The handle func already receives `tgBot *gotgbot.Bot`.

3. **STATE.md blocker: do operators need to add channel numeric ID to TELEGRAM_ALLOWED_USERS?**
   - What we know: STATE.md documents "operators must add their channel's numeric ID to TELEGRAM_ALLOWED_USERS" as a concern
   - What's unclear: With admin-lookup auth, the channel ID is NOT needed in the allowlist — any admin's user ID suffices. This concern appears to be from an earlier design iteration.
   - Recommendation: With the CONTEXT.md decision (admin lookup, not channel ID allowlist), no config change is needed from operators. The plan documentation should confirm this explicitly.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package (no external framework) |
| Config file | none — standard `go test ./...` |
| Quick run command | `go test ./internal/bot/... ./internal/security/...` |
| Full suite command | `go test ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AUTH-01 | Channel post from authorized channel is allowed | unit | `go test ./internal/bot/ -run TestMiddlewareAuthChannelAuthorized -v` | ❌ Wave 0 |
| AUTH-01 | Channel post from unauthorized channel is rejected | unit | `go test ./internal/bot/ -run TestMiddlewareAuthChannelUnauthorized -v` | ❌ Wave 0 |
| AUTH-01 | Admin lookup result is cached (cache hit skips API call) | unit | `go test ./internal/security/ -run TestChannelAuthCacheHit -v` | ❌ Wave 0 |
| AUTH-01 | Cache expires after TTL and triggers fresh lookup | unit | `go test ./internal/security/ -run TestChannelAuthCacheExpiry -v` | ❌ Wave 0 |
| AUTH-02 | Bot's own reflected channel post is dropped before auth | unit | `go test ./internal/bot/ -run TestMiddlewareAuthEchoFilter -v` | ❌ Wave 0 |
| AUTH-02 | IsAutomaticForward (linked channel) is dropped before auth | unit | `go test ./internal/bot/ -run TestMiddlewareAuthLinkedChannelFilter -v` | ❌ Wave 0 |
| AUTH-01 + AUTH-02 | Regular user DM/group message still passes auth unchanged | unit | `go test ./internal/bot/ -run TestMiddlewareAuthAllowsAuthorized -v` | ✅ (existing) |

### Sampling Rate
- **Per task commit:** `go test ./internal/bot/... ./internal/security/...`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/bot/middleware_test.go` — add channel auth and echo filter test cases (file exists, needs new test functions)
- [ ] `internal/security/validate_test.go` — add `TestChannelAuthCacheHit`, `TestChannelAuthCacheExpiry` (file exists, needs new tests)
- [ ] `internal/security/validate.go` or new `internal/security/channel_auth.go` — add `ChannelAuthCache` struct (new code)

*(Existing test infrastructure in `internal/bot/` and `internal/security/` covers the test runner setup — no new framework install needed.)*

## Sources

### Primary (HIGH confidence)
- gotgbot v2.0.0-rc.34 `sender.go` — `IsChannelPost()`, `IsBot()`, `IsLinkedChannel()`, `IsAutomaticForward`, `Sender.Id()` semantics — read directly from module cache
- gotgbot v2.0.0-rc.34 `gen_methods.go` line 2450 — `GetChatAdministrators(chatId int64, opts) ([]ChatMember, error)` signature — read directly from module cache
- gotgbot v2.0.0-rc.34 `gen_types.go` — `ChatMember` interface, `GetUser() User` method — read directly from module cache
- `internal/bot/middleware.go` — existing auth middleware structure, `AuthChecker` interface, `tgBot *gotgbot.Bot` handle func parameter — read directly
- `internal/security/validate.go` — `IsAuthorized` signature and body — read directly
- `.planning/phases/09-channel-auth/09-CONTEXT.md` — locked decisions — read directly

### Secondary (MEDIUM confidence)
- STATE.md accumulated decisions: "Use `sender.IsUser()` as the universal gate for non-human senders" — confirms gotgbot Sender methods are the established pattern in this project
- STATE.md accumulated decisions: "Auth fix must be an additive branch — never restructure the existing user-ID check path" — confirms Option B approach

### Tertiary (LOW confidence)
- Telegram Bot API documentation note "which aren't bots" on `getChatAdministrators` — not independently verified in this session; caveat documented in Pitfall 1

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies; all in module cache
- Architecture: HIGH — verified against actual source of both gotgbot and existing middleware
- Pitfalls: HIGH for gotgbot API pitfalls (verified from source); MEDIUM for Telegram API behavior (API docs note about bots in admin list)

**Research date:** 2026-03-20
**Valid until:** 2026-04-20 (stable library; gotgbot is pinned; TTL applies to Telegram API behavior claims)
