package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/user/gsd-tele-go/internal/bot"
	"github.com/user/gsd-tele-go/internal/config"
)

func main() {
	// Configure zerolog with a human-readable console writer for development.
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load configuration from environment (and .env if present).
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Log startup info (mask token: show first 5 chars only).
	maskedToken := cfg.TelegramToken
	if len(maskedToken) > 5 {
		maskedToken = maskedToken[:5] + "..."
	}
	log.Info().
		Str("token_prefix", maskedToken).
		Str("working_dir", cfg.WorkingDir).
		Str("claude_path", cfg.ClaudeCLIPath).
		Int("allowed_users", len(cfg.AllowedUsers)).
		Msg("Starting gsd-tele-go")

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatal().Err(err).Str("data_dir", cfg.DataDir).Msg("Failed to create data directory")
	}

	// Create bot instance.
	b, err := bot.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create bot")
	}

	// Create a cancellable context for the bot and its session workers.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up OS signal handling for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start the bot in a background goroutine.
	go func() {
		if err := b.Start(ctx); err != nil {
			log.Fatal().Err(err).Msg("Bot failed")
		}
	}()

	// Block until a shutdown signal is received.
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Cancel the context to propagate shutdown to all session workers.
	cancel()

	// Stop the bot (waits for worker goroutines to drain, closes audit log).
	b.Stop()
}
