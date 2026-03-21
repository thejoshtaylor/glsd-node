package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

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

	log.Info().
		Str("working_dir", cfg.WorkingDir).
		Str("claude_path", cfg.ClaudeCLIPath).
		Msg("Starting gsd-tele-go node")

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatal().Err(err).Str("data_dir", cfg.DataDir).Msg("Failed to create data directory")
	}

	// Set up OS signal handling for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// TODO(phase-13): Wire ConnectionManager and dispatch loop here
	// The context below will be passed to the ConnectionManager when wired up.
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Block until a shutdown signal is received.
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Cancel the context to propagate shutdown.
	cancel()

	log.Info().Msg("gsd-tele-go node stopped")
}
