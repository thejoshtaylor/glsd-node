package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/user/gsd-tele-go/internal/audit"
	"github.com/user/gsd-tele-go/internal/config"
	"github.com/user/gsd-tele-go/internal/connection"
	"github.com/user/gsd-tele-go/internal/dispatch"
	"github.com/user/gsd-tele-go/internal/security"
)

func main() {
	// Configure zerolog with a human-readable console writer for development.
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load configuration from environment (and .env if present).
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	nodeCfg, err := config.LoadNodeConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load node configuration")
	}

	nodeLog := log.With().Str("node_id", nodeCfg.NodeID).Logger()
	nodeLog.Info().
		Str("working_dir", cfg.WorkingDir).
		Str("server_url", nodeCfg.ServerURL).
		Msg("starting gsd node")

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatal().Err(err).Str("data_dir", cfg.DataDir).Msg("failed to create data directory")
	}

	// Open audit logger.
	auditLog, err := audit.New(cfg.AuditLogPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", cfg.AuditLogPath).Msg("failed to open audit log")
	}
	defer auditLog.Close()

	// Create rate limiter (uses cfg settings; always created, checked inside dispatcher only if enabled).
	limiter := security.NewProjectRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)

	// Set up OS signal handling for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create context for the node lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and start ConnectionManager.
	connMgr := connection.NewConnectionManager(nodeCfg, nodeLog)
	connMgr.Start(ctx)

	// Create and start Dispatcher.
	dispatcher := dispatch.New(connMgr, cfg, nodeCfg, auditLog, limiter, nodeLog)
	go dispatcher.Run(ctx)

	nodeLog.Info().Msg("gsd node running — waiting for commands")

	// Block until a shutdown signal is received.
	sig := <-sigCh
	nodeLog.Info().Str("signal", sig.String()).Msg("received shutdown signal")

	// Phase 1: Cancel context — propagates to all instance goroutines.
	cancel()

	// Phase 2: Stop dispatcher and wait for instances to drain (up to 10s).
	dispatcher.Stop()
	done := make(chan struct{})
	go func() {
		dispatcher.Wait()
		close(done)
	}()

	select {
	case <-done:
		nodeLog.Info().Msg("all instances drained")
	case <-time.After(10 * time.Second):
		nodeLog.Warn().Msg("shutdown timeout: forcing exit after 10s")
	}

	// Phase 3: Stop ConnectionManager (sends disconnect frame, closes WebSocket).
	connMgr.Stop()

	nodeLog.Info().Msg("gsd node stopped")
}
