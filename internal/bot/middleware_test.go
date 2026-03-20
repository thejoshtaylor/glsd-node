package bot

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// --- Mock implementations ---

// mockAuthChecker implements AuthChecker with a fixed response.
type mockAuthChecker struct {
	authorized bool
	calledWith struct {
		userID    int64
		channelID int64
	}
}

func (m *mockAuthChecker) IsAuthorized(userID int64, channelID int64) bool {
	m.calledWith.userID = userID
	m.calledWith.channelID = channelID
	return m.authorized
}

// mockRateLimitChecker implements RateLimitChecker with configurable behavior.
type mockRateLimitChecker struct {
	callCount int
	maxAllow  int // number of requests to allow before throttling
	delay     time.Duration
}

func (m *mockRateLimitChecker) Allow(_ int64) (bool, time.Duration) {
	m.callCount++
	if m.callCount <= m.maxAllow {
		return true, 0
	}
	return false, m.delay
}

// callTracker tracks whether the next handler was called.
type callTracker struct {
	called bool
}

func (c *callTracker) CheckUpdate(_ *gotgbot.Bot, _ *ext.Context) bool { return true }
func (c *callTracker) HandleUpdate(_ *gotgbot.Bot, _ *ext.Context) error {
	c.called = true
	return nil
}
func (c *callTracker) Name() string { return "call_tracker" }

// --- Test helpers ---

// buildContext creates a minimal ext.Context with the given userID and chatID.
// It works without a live Telegram connection by constructing the structs directly.
func buildContext(userID, chatID int64) *ext.Context {
	user := &gotgbot.User{Id: userID, FirstName: "Test"}
	chat := &gotgbot.Chat{Id: chatID}
	msg := &gotgbot.Message{
		MessageId: 1,
		From:      user,
		Chat:      *chat,
		Text:      "hello",
	}
	sender := &gotgbot.Sender{User: user}

	// ext.Context embeds *gotgbot.Update; we set EffectiveMessage etc. directly.
	return &ext.Context{
		Update:           &gotgbot.Update{Message: msg},
		EffectiveUser:    user,
		EffectiveChat:    chat,
		EffectiveMessage: msg,
		EffectiveSender:  sender,
	}
}

// buildChannelPostContext creates an ext.Context simulating a channel admin
// posting to their own channel (IsChannelPost() == true).
// The chat ID equals the sender chat ID and type is "channel".
func buildChannelPostContext(channelID int64) *ext.Context {
	chat := &gotgbot.Chat{Id: channelID, Type: "channel", Title: "Test Channel"}
	msg := &gotgbot.Message{
		MessageId: 1,
		Chat:      *chat,
		Text:      "channel message",
	}
	// Chat.Id == ChatId and Chat.Type == "channel" → IsChannelPost() == true
	sender := &gotgbot.Sender{
		Chat:   chat,
		ChatId: channelID,
	}
	return &ext.Context{
		Update:           &gotgbot.Update{Message: msg},
		EffectiveChat:    chat,
		EffectiveMessage: msg,
		EffectiveSender:  sender,
	}
}

// buildBotEchoContext creates an ext.Context simulating the bot's own reflected
// channel post (sender is the bot itself — User.IsBot == true, User.Id == botID).
func buildBotEchoContext(botID, chatID int64) *ext.Context {
	botUser := &gotgbot.User{Id: botID, IsBot: true, FirstName: "TestBot"}
	chat := &gotgbot.Chat{Id: chatID}
	msg := &gotgbot.Message{
		MessageId: 1,
		From:      botUser,
		Chat:      *chat,
		Text:      "echo",
	}
	// User is set, Chat is nil → IsBot() == true
	sender := &gotgbot.Sender{User: botUser}
	return &ext.Context{
		Update:           &gotgbot.Update{Message: msg},
		EffectiveChat:    chat,
		EffectiveMessage: msg,
		EffectiveSender:  sender,
	}
}

// buildLinkedChannelContext creates an ext.Context simulating an automatic
// forward from a linked channel (IsLinkedChannel() == true).
// IsAutomaticForward == true and Chat.Id != ChatId.
func buildLinkedChannelContext(chatID, linkedChatID int64) *ext.Context {
	linkedChat := &gotgbot.Chat{Id: linkedChatID, Type: "channel", Title: "Linked Channel"}
	targetChat := &gotgbot.Chat{Id: chatID}
	msg := &gotgbot.Message{
		MessageId:          1,
		Chat:               *targetChat,
		IsAutomaticForward: true,
		Text:               "forwarded",
	}
	// Chat.Id != ChatId and IsAutomaticForward == true → IsLinkedChannel() == true
	sender := &gotgbot.Sender{
		Chat:               linkedChat,
		ChatId:             chatID,
		IsAutomaticForward: true,
	}
	return &ext.Context{
		Update:           &gotgbot.Update{Message: msg},
		EffectiveChat:    targetChat,
		EffectiveMessage: msg,
		EffectiveSender:  sender,
	}
}

// --- Auth middleware tests ---

// TestMiddlewareAuthRejectsUnauthorized verifies that authMiddleware stops
// processing (does not call the next handler) for unauthorized users.
func TestMiddlewareAuthRejectsUnauthorized(t *testing.T) {
	checker := &mockAuthChecker{authorized: false}
	next := &callTracker{}

	mw := authMiddlewareWith(checker, nil, nil, next)

	ctx := buildContext(999, 12345)
	err := mw.HandleUpdate(nil, ctx)

	if err != nil && err != ext.EndGroups {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.called {
		t.Error("expected next handler NOT to be called for unauthorized user, but it was called")
	}
}

// TestMiddlewareAuthAllowsAuthorized verifies that authMiddleware calls the next
// handler when the user is authorized.
func TestMiddlewareAuthAllowsAuthorized(t *testing.T) {
	checker := &mockAuthChecker{authorized: true}
	next := &callTracker{}

	mw := authMiddlewareWith(checker, nil, nil, next)

	ctx := buildContext(111, 12345)
	if err := mw.HandleUpdate(nil, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !next.called {
		t.Error("expected next handler to be called for authorized user, but it was not")
	}
}

// TestMiddlewareAuthPassesChannelID verifies that authMiddleware passes the chat
// ID (channelID) to IsAuthorized — confirming Phase 2 forward-compat wiring.
func TestMiddlewareAuthPassesChannelID(t *testing.T) {
	const wantChannelID int64 = 99887766

	checker := &mockAuthChecker{authorized: true}
	next := &callTracker{}

	mw := authMiddlewareWith(checker, nil, nil, next)

	ctx := buildContext(111, wantChannelID)
	if err := mw.HandleUpdate(nil, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checker.calledWith.channelID != wantChannelID {
		t.Errorf("authMiddleware passed channelID=%d to IsAuthorized; want %d",
			checker.calledWith.channelID, wantChannelID)
	}
}

// TestMiddlewareAuthEchoFilter verifies that when the sender is the bot itself
// (reflected channel post), the next handler is NOT called and ext.EndGroups is returned.
func TestMiddlewareAuthEchoFilter(t *testing.T) {
	const botID int64 = 42
	checker := &mockAuthChecker{authorized: false}
	next := &callTracker{}

	mw := authMiddlewareWith(checker, nil, nil, next)

	ctx := buildBotEchoContext(botID, 12345)
	// Pass a minimal *gotgbot.Bot with the matching ID.
	tgBot := &gotgbot.Bot{User: gotgbot.User{Id: botID}}
	err := mw.HandleUpdate(tgBot, ctx)

	if err != nil && err != ext.EndGroups {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.called {
		t.Error("expected next handler NOT to be called for bot's own echo post, but it was called")
	}
}

// TestMiddlewareAuthLinkedChannelFilter verifies that automatic forwards from
// a linked channel are silently dropped (next handler not called).
func TestMiddlewareAuthLinkedChannelFilter(t *testing.T) {
	checker := &mockAuthChecker{authorized: false}
	next := &callTracker{}

	mw := authMiddlewareWith(checker, nil, nil, next)

	ctx := buildLinkedChannelContext(12345, 99999)
	err := mw.HandleUpdate(nil, ctx)

	if err != nil && err != ext.EndGroups {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.called {
		t.Error("expected next handler NOT to be called for linked channel forward, but it was called")
	}
}

// TestMiddlewareAuthChannelAuthorized verifies that a channel post from a channel
// whose channelAuthFn returns true passes auth and calls the next handler.
func TestMiddlewareAuthChannelAuthorized(t *testing.T) {
	checker := &mockAuthChecker{authorized: false}
	next := &callTracker{}
	channelAuth := func(_ *gotgbot.Bot, _ int64) bool { return true }

	mw := authMiddlewareWith(checker, channelAuth, nil, next)

	ctx := buildChannelPostContext(12345)
	if err := mw.HandleUpdate(nil, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !next.called {
		t.Error("expected next handler to be called for authorized channel post, but it was not")
	}
}

// TestMiddlewareAuthChannelUnauthorized verifies that a channel post from a channel
// whose channelAuthFn returns false is rejected (next handler not called).
func TestMiddlewareAuthChannelUnauthorized(t *testing.T) {
	checker := &mockAuthChecker{authorized: false}
	next := &callTracker{}
	channelAuth := func(_ *gotgbot.Bot, _ int64) bool { return false }

	mw := authMiddlewareWith(checker, channelAuth, nil, next)

	ctx := buildChannelPostContext(12345)
	err := mw.HandleUpdate(nil, ctx)

	if err != nil && err != ext.EndGroups {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.called {
		t.Error("expected next handler NOT to be called for unauthorized channel post, but it was called")
	}
}

// TestMiddlewareAuthEchoBeforeChannelAuth verifies that the echo filter runs
// before channelAuth. A bot's own post is dropped even if channelAuth would
// allow it (channelAuth panics if called to make a violation obvious).
func TestMiddlewareAuthEchoBeforeChannelAuth(t *testing.T) {
	const botID int64 = 42
	checker := &mockAuthChecker{authorized: false}
	next := &callTracker{}
	// channelAuth panics if called — echo filter must prevent reaching it.
	channelAuth := func(_ *gotgbot.Bot, _ int64) bool {
		panic("channelAuth must not be called when echo filter fires")
	}

	mw := authMiddlewareWith(checker, channelAuth, nil, next)

	ctx := buildBotEchoContext(botID, 12345)
	tgBot := &gotgbot.Bot{User: gotgbot.User{Id: botID}}

	// Should not panic.
	err := mw.HandleUpdate(tgBot, ctx)

	if err != nil && err != ext.EndGroups {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.called {
		t.Error("expected next handler NOT to be called for bot's own echo post")
	}
}

// --- Rate limit middleware tests ---

// TestMiddlewareRateLimitThrottles verifies that after maxAllow requests, the
// next request is rejected with an EndGroups error (the reply function is skipped
// since we pass a nil bot, but the next handler must NOT be called).
func TestMiddlewareRateLimitThrottles(t *testing.T) {
	limiter := &mockRateLimitChecker{maxAllow: 2, delay: 30 * time.Second}
	next := &callTracker{}

	mw := rateLimitMiddlewareWith(true, limiter, nil, next)

	ctx := buildContext(111, 12345)

	// First two requests should pass through.
	for i := 0; i < 2; i++ {
		next.called = false
		if err := mw.HandleUpdate(nil, ctx); err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if !next.called {
			t.Errorf("request %d: expected next handler to be called but it was not", i+1)
		}
	}

	// Third request should be throttled.
	next.called = false
	err := mw.HandleUpdate(nil, ctx)
	if err != nil && err != ext.EndGroups {
		t.Fatalf("request 3: unexpected error: %v", err)
	}
	if next.called {
		t.Error("request 3: expected next handler NOT to be called when rate limited, but it was")
	}
}

// TestMiddlewareRateLimitDisabled verifies that when rate limiting is disabled,
// all requests pass through regardless of volume.
func TestMiddlewareRateLimitDisabled(t *testing.T) {
	// A limiter that would throttle immediately — but it should never be consulted.
	limiter := &mockRateLimitChecker{maxAllow: 0, delay: 60 * time.Second}
	next := &callTracker{}

	mw := rateLimitMiddlewareWith(false, limiter, nil, next)

	ctx := buildContext(111, 12345)

	for i := 0; i < 5; i++ {
		next.called = false
		if err := mw.HandleUpdate(nil, ctx); err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if !next.called {
			t.Errorf("request %d: expected next to be called when rate limiting disabled", i+1)
		}
	}

	if limiter.callCount > 0 {
		t.Errorf("rate limiter Allow() was called %d times when rate limiting is disabled; expected 0",
			limiter.callCount)
	}
}
