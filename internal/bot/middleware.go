package bot

import (
	"fmt"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"

	"github.com/user/gsd-tele-go/internal/audit"
	"github.com/user/gsd-tele-go/internal/security"
)

// AuthChecker is the interface used by authMiddleware to check whether a user is
// authorised.  Extracted as an interface so tests can inject a mock without a
// live Telegram connection.
type AuthChecker interface {
	IsAuthorized(userID int64, channelID int64) bool
}

// RateLimitChecker is the interface used by rateLimitMiddleware to check whether
// a channel is within its rate limit.
type RateLimitChecker interface {
	Allow(channelID int64) (bool, time.Duration)
}

// ChannelAuthFn checks whether a channel is authorized by looking up its admins.
// Returns true if any admin's user ID is in the allowed-users list.
// This function is called on cache miss; the middleware handles caching.
type ChannelAuthFn func(tgBot *gotgbot.Bot, channelID int64) bool

// defaultAuthChecker wraps security.IsAuthorized with the bot's AllowedUsers list.
type defaultAuthChecker struct {
	allowedUsers []int64
}

func (a *defaultAuthChecker) IsAuthorized(userID int64, channelID int64) bool {
	return security.IsAuthorized(userID, channelID, a.allowedUsers)
}

// authMiddleware returns a gotgbot Handler that rejects messages from users who
// are not in the allowed-users list.  It calls security.IsAuthorized, passing
// channelID for Phase 2 forward-compatibility.
func (b *Bot) authMiddleware(next ext.Handler) ext.Handler {
	checker := &defaultAuthChecker{allowedUsers: b.cfg.AllowedUsers}
	cache := security.NewChannelAuthCache(15 * time.Minute)
	channelAuthFn := func(tgBot *gotgbot.Bot, channelID int64) bool {
		if tgBot == nil {
			return false
		}
		if authorized, hit := cache.Lookup(channelID); hit {
			return authorized
		}
		admins, err := tgBot.GetChatAdministrators(channelID, nil)
		if err != nil {
			log.Warn().Err(err).Int64("channel_id", channelID).Msg("channel admin lookup failed")
			cache.Store(channelID, false)
			return false
		}
		for _, member := range admins {
			user := member.GetUser()
			for _, allowed := range b.cfg.AllowedUsers {
				if user.Id == allowed {
					cache.Store(channelID, true)
					return true
				}
			}
		}
		cache.Store(channelID, false)
		return false
	}
	return authMiddlewareWith(checker, channelAuthFn, b.auditLog, next)
}

// authMiddlewareWith is the testable implementation of authMiddleware.
func authMiddlewareWith(checker AuthChecker, channelAuth ChannelAuthFn, auditLog *audit.Logger, next ext.Handler) ext.Handler {
	return &middlewareHandler{
		name: "auth",
		check: func(_ *gotgbot.Bot, ctx *ext.Context) bool {
			return true // run for all updates
		},
		handle: func(tgBot *gotgbot.Bot, ctx *ext.Context) error {
			// Echo filter: drop bot's own reflected channel posts (AUTH-02).
			if sender := ctx.EffectiveSender; sender != nil {
				// Check if sender is this bot (reflected channel post).
				if sender.User != nil && sender.User.IsBot && tgBot != nil && sender.User.Id == tgBot.Id {
					log.Debug().Int64("bot_id", tgBot.Id).Msg("echo filtered: bot's own channel post")
					return ext.EndGroups
				}
				// Check if sender is a linked channel automatic forward.
				if sender.IsLinkedChannel() {
					log.Debug().Msg("echo filtered: automatic forward from linked channel")
					return ext.EndGroups
				}
			}

			var userID int64
			if ctx.EffectiveSender != nil {
				userID = ctx.EffectiveSender.Id()
			}

			var channelID int64
			if ctx.EffectiveChat != nil {
				channelID = ctx.EffectiveChat.Id
			}

			if !checker.IsAuthorized(userID, channelID) {
				// Channel post fallback: authorize via admin lookup (AUTH-01).
				if ctx.EffectiveSender != nil && ctx.EffectiveSender.IsChannelPost() && channelAuth != nil {
					if channelAuth(tgBot, channelID) {
						return next.HandleUpdate(tgBot, ctx)
					}
				}

				// Log the rejection.
				if auditLog != nil {
					ev := audit.NewEvent("auth_rejected", userID, channelID)
					ev.Message = "unauthorized access attempt"
					_ = auditLog.Log(ev)
				}

				// Reply and stop processing (skip if bot is nil, e.g. in unit tests).
				if tgBot != nil && ctx.EffectiveMessage != nil {
					_, _ = ctx.EffectiveMessage.Reply(tgBot, "You're not authorized for this channel. Contact the bot admin.", nil)
				}
				return ext.EndGroups
			}

			return next.HandleUpdate(tgBot, ctx)
		},
	}
}

// rateLimitMiddleware returns a gotgbot Handler that rejects messages from
// channels that have exceeded their rate limit.
func (b *Bot) rateLimitMiddleware(next ext.Handler) ext.Handler {
	return rateLimitMiddlewareWith(b.cfg.RateLimitEnabled, b.rateLimiter, b.auditLog, next)
}

// rateLimitMiddlewareWith is the testable implementation of rateLimitMiddleware.
func rateLimitMiddlewareWith(enabled bool, limiter RateLimitChecker, auditLog *audit.Logger, next ext.Handler) ext.Handler {
	return &middlewareHandler{
		name: "rate_limit",
		check: func(_ *gotgbot.Bot, ctx *ext.Context) bool {
			return true // run for all updates
		},
		handle: func(tgBot *gotgbot.Bot, ctx *ext.Context) error {
			if !enabled {
				return next.HandleUpdate(tgBot, ctx)
			}

			var channelID int64
			if ctx.EffectiveChat != nil {
				channelID = ctx.EffectiveChat.Id
			}

			allowed, delay := limiter.Allow(channelID)
			if !allowed {
				seconds := int(delay.Round(time.Second).Seconds())
				if seconds <= 0 {
					seconds = 1
				}

				// Log the throttle.
				if auditLog != nil {
					var userID int64
					if ctx.EffectiveSender != nil {
						userID = ctx.EffectiveSender.Id()
					}
					ev := audit.NewEvent("rate_limited", userID, channelID)
					ev.Message = fmt.Sprintf("rate limited for %ds", seconds)
					_ = auditLog.Log(ev)
				}

				// Reply and stop processing (skip if bot is nil, e.g. in unit tests).
				if tgBot != nil && ctx.EffectiveMessage != nil {
					msg := fmt.Sprintf("Rate limited. Try again in %ds.", seconds)
					_, _ = ctx.EffectiveMessage.Reply(tgBot, msg, nil)
				}
				return ext.EndGroups
			}

			return next.HandleUpdate(tgBot, ctx)
		},
	}
}

// middlewareHandler is a generic ext.Handler implementation for middleware.
type middlewareHandler struct {
	name   string
	check  func(*gotgbot.Bot, *ext.Context) bool
	handle func(*gotgbot.Bot, *ext.Context) error
}

func (m *middlewareHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return m.check(b, ctx)
}

func (m *middlewareHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	return m.handle(b, ctx)
}

func (m *middlewareHandler) Name() string {
	return m.name
}
