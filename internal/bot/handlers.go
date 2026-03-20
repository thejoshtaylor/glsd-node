package bot

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"

	bothandlers "github.com/user/gsd-tele-go/internal/handlers"
)

// registerHandlers wires up the full middleware and handler chain on dispatcher.
//
// Group ordering:
//   - Group -2: auth middleware (runs first; rejects unauthorized users)
//   - Group -1: rate limit middleware (runs second; throttles per-channel)
//   - Group  0: message and command handlers
func (b *Bot) registerHandlers(dispatcher *ext.Dispatcher) {
	// --- Middleware groups ---
	// Build a placeholder terminal handler that the middleware delegates to.
	// The middleware wraps the *next* handler; we chain them manually.
	// In gotgbot, middleware is registered as handlers in lower-numbered groups.

	// Auth middleware: wraps a no-op so that after passing auth, processing falls
	// through to group -1 and then 0.
	authPass := &passthroughHandler{name: "auth_pass"}
	dispatcher.AddHandlerToGroup(b.authMiddleware(authPass), -2)

	// Rate limit middleware.
	rateLimitPass := &passthroughHandler{name: "rate_limit_pass"}
	dispatcher.AddHandlerToGroup(b.rateLimitMiddleware(rateLimitPass), -1)

	// --- Text message handler ---
	dispatcher.AddHandler(handlers.NewMessage(message.Text, b.handleText))

	// --- Media message handlers ---
	dispatcher.AddHandler(handlers.NewMessage(message.Voice, b.handleVoice))
	dispatcher.AddHandler(handlers.NewMessage(message.Photo, b.handlePhoto))
	dispatcher.AddHandler(handlers.NewMessage(message.Document, b.handleDocument))

	// --- Command handlers ---
	dispatcher.AddHandler(handlers.NewCommand("start", b.handleStart))
	dispatcher.AddHandler(handlers.NewCommand("new", b.handleNew))
	dispatcher.AddHandler(handlers.NewCommand("stop", b.handleStop))
	dispatcher.AddHandler(handlers.NewCommand("status", b.handleStatus))
	dispatcher.AddHandler(handlers.NewCommand("resume", b.handleResume))
	dispatcher.AddHandler(handlers.NewCommand("project", b.handleProject))
	dispatcher.AddHandler(handlers.NewCommand("gsd", b.handleGsd))

	// --- Callback query handler ---
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.All, b.handleCallback))
}

// handleVoice is the bot-layer wrapper that calls the handlers.HandleVoice function.
func (b *Bot) handleVoice(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleVoice(tgBot, ctx, b.store, b.cfg, b.auditLog, b.persist, b.WaitGroup(), b.mappings, b.globalAPILimiter)
}

// handlePhoto is the bot-layer wrapper that calls the handlers.HandlePhoto function.
func (b *Bot) handlePhoto(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandlePhoto(tgBot, ctx, b.store, b.cfg, b.auditLog, b.persist, b.WaitGroup(), b.mappings, b.globalAPILimiter)
}

// handleDocument is the bot-layer wrapper that calls the handlers.HandleDocument function.
func (b *Bot) handleDocument(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleDocument(tgBot, ctx, b.store, b.cfg, b.auditLog, b.persist, b.WaitGroup(), b.mappings, b.globalAPILimiter)
}

// handleText is the bot-layer wrapper that calls the handlers.HandleText function.
func (b *Bot) handleText(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleText(tgBot, ctx, b.store, b.cfg, b.auditLog, b.persist, b.WaitGroup(), b.mappings, b.awaitingPath, b.globalAPILimiter)
}

func (b *Bot) handleStart(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleStart(tgBot, ctx, b.store, b.cfg, b.mappings)
}

func (b *Bot) handleNew(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleNew(tgBot, ctx, b.store, b.persist, b.cfg, b.mappings)
}

func (b *Bot) handleStop(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleStop(tgBot, ctx, b.store)
}

func (b *Bot) handleStatus(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleStatus(tgBot, ctx, b.store, b.cfg, b.mappings)
}

func (b *Bot) handleResume(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleResume(tgBot, ctx, b.persist, b.mappings)
}

func (b *Bot) handleProject(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleProject(tgBot, ctx, b.mappings, b.awaitingPath, b.cfg)
}

func (b *Bot) handleCallback(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleCallback(tgBot, ctx, b.store, b.persist, b.cfg, b.mappings, b.awaitingPath)
}

func (b *Bot) handleGsd(tgBot *gotgbot.Bot, ctx *ext.Context) error {
	return bothandlers.HandleGsd(tgBot, ctx, b.mappings, b.store, b.cfg, b.WaitGroup())
}

// passthroughHandler is a no-op handler used as the terminal target for middleware
// chains. When the middleware calls next.HandleUpdate(), this handler returns nil
// allowing the update to fall through to the next dispatcher group.
type passthroughHandler struct {
	name string
}

func (p *passthroughHandler) CheckUpdate(_ *gotgbot.Bot, _ *ext.Context) bool {
	return true
}

func (p *passthroughHandler) HandleUpdate(_ *gotgbot.Bot, _ *ext.Context) error {
	return nil
}

func (p *passthroughHandler) Name() string {
	return p.name
}
