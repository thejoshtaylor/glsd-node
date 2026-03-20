// Package bot provides the top-level Bot struct that wires together the gotgbot
// updater, middleware chain, session store, persistence, rate limiter, and audit log.
package bot

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	"github.com/user/gsd-tele-go/internal/audit"
	"github.com/user/gsd-tele-go/internal/config"
	bothandlers "github.com/user/gsd-tele-go/internal/handlers"
	"github.com/user/gsd-tele-go/internal/project"
	"github.com/user/gsd-tele-go/internal/security"
	"github.com/user/gsd-tele-go/internal/session"
)

// Bot owns the Telegram bot lifecycle: polling, middleware, session management, and shutdown.
type Bot struct {
	bot              *gotgbot.Bot
	updater          *ext.Updater
	cfg              *config.Config
	store            *session.SessionStore
	persist          *session.PersistenceManager
	rateLimiter      *security.ChannelRateLimiter
	auditLog         *audit.Logger
	cancelFunc       context.CancelFunc
	wg               sync.WaitGroup // tracks active session worker goroutines
	mappings         *project.MappingStore
	awaitingPath     *bothandlers.AwaitingPathState
	globalAPILimiter *rate.Limiter // global Telegram API rate limiter (25/sec, burst 5)
}

// New creates and initialises a Bot from the given Config.
// It creates the gotgbot client, session store, persistence manager, rate limiter, and audit log.
func New(cfg *config.Config) (*Bot, error) {
	// Create the Telegram bot client.
	tgBot, err := gotgbot.NewBot(cfg.TelegramToken, nil)
	if err != nil {
		return nil, err
	}

	// Session store.
	store := session.NewSessionStore()

	// Persistence manager: session-history.json in DataDir.
	persist := session.NewPersistenceManager(
		filepath.Join(cfg.DataDir, "session-history.json"),
		config.MaxSessionHistory,
	)

	// Rate limiter.
	rateLimiter := security.NewChannelRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)

	// Audit log.
	auditLog, err := audit.New(cfg.AuditLogPath)
	if err != nil {
		return nil, err
	}

	// MappingStore: load channel-to-project mappings from disk.
	mappings := project.NewMappingStore(filepath.Join(cfg.DataDir, "mappings.json"))
	if err := mappings.Load(); err != nil {
		log.Warn().Err(err).Msg("Failed to load mappings; starting with empty")
	}

	// Global API rate limiter: 25 edits/sec with burst of 5 (Telegram hard limit is ~30/sec).
	globalAPILimiter := rate.NewLimiter(rate.Limit(25), 5)

	b := &Bot{
		bot:              tgBot,
		cfg:              cfg,
		store:            store,
		persist:          persist,
		rateLimiter:      rateLimiter,
		auditLog:         auditLog,
		mappings:         mappings,
		awaitingPath:     bothandlers.NewAwaitingPathState(),
		globalAPILimiter: globalAPILimiter,
	}

	log.Info().
		Str("username", tgBot.User.Username).
		Str("claude_path", cfg.ClaudeCLIPath).
		Msg("Bot initialized")

	return b, nil
}

// Start begins polling for Telegram updates. It blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	b.cancelFunc = cancel

	// Restore any sessions that were active before the bot last stopped.
	if err := b.restoreSessions(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to restore sessions from persistence; starting fresh")
	}

	// Create dispatcher and updater.
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		// Handle panics in handlers so one bad update doesn't bring down the bot.
		Panic: func(_ *gotgbot.Bot, _ *ext.Context, r any) {
			log.Error().Interface("panic", r).Msg("panic recovered in handler")
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})

	b.updater = ext.NewUpdater(dispatcher, nil)

	// Register all handlers on the dispatcher.
	b.registerHandlers(dispatcher)

	// Start long polling.
	if err := b.updater.StartPolling(b.bot, &ext.PollingOpts{
		DropPendingUpdates: false,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 10,
		},
	}); err != nil {
		cancel()
		return err
	}

	log.Info().Str("username", b.bot.User.Username).Msg("Bot started, polling for updates")

	// Block until context is cancelled.
	<-ctx.Done()
	return nil
}

// Stop signals all session workers and waits for them to drain, then stops the updater.
func (b *Bot) Stop() {
	log.Info().Msg("Shutting down...")

	// Signal all session workers to stop.
	if b.cancelFunc != nil {
		b.cancelFunc()
	}

	// Stop the Telegram updater (stops polling).
	if b.updater != nil {
		if err := b.updater.Stop(); err != nil {
			log.Error().Err(err).Msg("Error stopping updater")
		}
	}

	// Wait for all session worker goroutines to exit.
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("All session workers stopped")
	case <-time.After(30 * time.Second):
		log.Warn().Msg("Timed out waiting for session workers to stop")
	}

	// Close audit log.
	if b.auditLog != nil {
		if err := b.auditLog.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing audit log")
		}
	}

	log.Info().Msg("Shutdown complete")
}

// WaitGroup returns a pointer to the bot's WaitGroup so that HandleText can
// register worker goroutines for graceful shutdown tracking.
func (b *Bot) WaitGroup() *sync.WaitGroup {
	return &b.wg
}

// GlobalAPILimiter returns the global Telegram API rate limiter.
func (b *Bot) GlobalAPILimiter() *rate.Limiter {
	return b.globalAPILimiter
}

// restoreSessions loads saved session history and recreates in-memory sessions
// for each unique channel that has a saved entry. Worker goroutines are started
// immediately so the sessions are ready when the first message arrives.
//
// For each channel, the mapping is checked to get the per-project AllowedPaths and
// SafetyPrompt. Falls back to saved.WorkingDir if no mapping exists.
func (b *Bot) restoreSessions(ctx context.Context) error {
	history, err := b.persist.Load()
	if err != nil {
		return err
	}

	// Collect the most recent session per channel.
	latest := make(map[int64]session.SavedSession)
	for _, s := range history.Sessions {
		existing, ok := latest[s.ChannelID]
		if !ok || s.SavedAt > existing.SavedAt {
			latest[s.ChannelID] = s
		}
	}

	for channelID, saved := range latest {
		sess := b.store.GetOrCreate(channelID, saved.WorkingDir)
		sess.SetSessionID(saved.SessionID)

		// Determine per-project AllowedPaths: prefer mapping, fall back to saved.WorkingDir.
		allowedPaths := b.cfg.AllowedPaths
		safetyPrompt := b.cfg.SafetyPrompt
		if mapping, ok := b.mappings.Get(channelID); ok {
			allowedPaths = []string{mapping.Path}
			safetyPrompt = config.BuildSafetyPrompt([]string{mapping.Path})
		} else if saved.WorkingDir != "" {
			allowedPaths = []string{saved.WorkingDir}
			safetyPrompt = config.BuildSafetyPrompt([]string{saved.WorkingDir})
		}

		// Start the worker goroutine exactly once, tracking with workerStarted.
		sess.SetWorkerStarted()
		b.wg.Add(1)
		wCfg := session.WorkerConfig{
			AllowedPaths: allowedPaths,
			SafetyPrompt: safetyPrompt,
			FilteredEnv:  config.FilteredEnv(),
		}
		go func(s *session.Session, c session.WorkerConfig) {
			defer b.wg.Done()
			s.Worker(ctx, b.cfg.ClaudeCLIPath, c)
		}(sess, wCfg)

		log.Info().
			Int64("channel_id", channelID).
			Str("session_id", saved.SessionID).
			Str("working_dir", saved.WorkingDir).
			Msg("Restored session from persistence")
	}

	return nil
}
