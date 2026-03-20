// Package connection provides the ConnectionManager, which dials outbound to
// a WebSocket server with Bearer token authentication, reconnects with
// exponential backoff, and serializes all outbound writes through a single
// writer goroutine (XPORT-06).
package connection

import (
	"context"
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/user/gsd-tele-go/internal/config"
	"github.com/user/gsd-tele-go/internal/protocol"
)

// ErrStopped is returned by Send() after Stop() has been called.
var ErrStopped = errors.New("connection manager stopped")

// ConnectionManager owns the WebSocket connection lifecycle. It dials outbound
// to cfg.ServerURL, authenticates with a Bearer token, and reconnects
// automatically with exponential backoff on failure.
//
// All outbound WebSocket writes are serialized through a single writer goroutine
// (XPORT-06). Callers submit frames via Send(); the writer goroutine drains
// sendCh and calls conn.Write exclusively.
type ConnectionManager struct {
	cfg        *config.NodeConfig
	log        zerolog.Logger
	sendCh     chan []byte            // buffered 64; writer goroutine owns draining
	recvCh     chan *protocol.Envelope // buffered 64; Phase 13 dispatcher reads from this
	stopCh     chan struct{}           // closed by Stop() to signal shutdown
	stopped    chan struct{}           // closed by dialLoop when it exits
	cancel     context.CancelFunc     // cancels the child context passed to dialLoop
	httpClient *http.Client           // nil uses default; set in tests for TLS cert trust
}

// NewConnectionManager creates and returns a ConnectionManager. Call Start()
// to begin dialing and Stop() for clean shutdown.
func NewConnectionManager(cfg *config.NodeConfig, log zerolog.Logger) *ConnectionManager {
	return &ConnectionManager{
		cfg:     cfg,
		log:     log,
		sendCh:  make(chan []byte, 64),
		recvCh:  make(chan *protocol.Envelope, 64),
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

// SetHTTPClient overrides the HTTP client used for WebSocket dialing. Use
// srv.Client() from httptest.NewTLSServer to trust the self-signed test cert.
func (m *ConnectionManager) SetHTTPClient(c *http.Client) {
	m.httpClient = c
}

// Start launches the dial loop in a background goroutine and returns
// immediately. The dial loop reconnects automatically on failure.
// ctx controls the lifetime of all background goroutines.
func (m *ConnectionManager) Start(ctx context.Context) {
	childCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	go m.dialLoop(childCtx)
}

// Stop signals the connection manager to shut down and waits for all
// background goroutines to exit. It is safe to call Stop() multiple times.
func (m *ConnectionManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	// Close stopCh to unblock any Send() calls waiting for sendCh capacity.
	// Use a select to avoid panic on double-close.
	select {
	case <-m.stopCh:
		// already closed
	default:
		close(m.stopCh)
	}
	// Wait for dialLoop to exit.
	<-m.stopped
}

// Send enqueues data for delivery to the server via the single writer goroutine.
// Returns ErrStopped if Stop() has already been called.
// Send does not block indefinitely: if the send channel is full and the manager
// is stopped, ErrStopped is returned rather than waiting.
func (m *ConnectionManager) Send(data []byte) error {
	// Check stopCh first (non-blocking) to return ErrStopped immediately if
	// the manager has been stopped, even if sendCh still has capacity.
	select {
	case <-m.stopCh:
		return ErrStopped
	case <-m.stopped:
		return ErrStopped
	default:
	}
	// Now try to enqueue, falling back to ErrStopped if manager shuts down.
	select {
	case m.sendCh <- data:
		return nil
	case <-m.stopped:
		return ErrStopped
	case <-m.stopCh:
		return ErrStopped
	}
}

// Receive returns the channel on which inbound envelopes from the server are
// delivered. Phase 13 (dispatch) reads from this channel.
func (m *ConnectionManager) Receive() <-chan *protocol.Envelope {
	return m.recvCh
}
