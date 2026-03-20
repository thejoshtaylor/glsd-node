package connection

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/user/gsd-tele-go/internal/protocol"
)


// dialLoop is the reconnect loop. It runs until ctx is cancelled.
// On each iteration it attempts a WebSocket dial; on failure it sleeps a
// jittered backoff duration; on success it calls handleConn (blocking until
// the connection dies) then loops back to reconnect.
// When dialLoop exits it closes m.stopped.
func (m *ConnectionManager) dialLoop(ctx context.Context) {
	defer close(m.stopped)

	backoff := newBackoff(500*time.Millisecond, 30*time.Second)

	for {
		// Check for cancellation before attempting a new dial.
		select {
		case <-ctx.Done():
			return
		default:
		}

		opts := &websocket.DialOptions{
			HTTPHeader: http.Header{
				"Authorization": []string{"Bearer " + m.cfg.ServerToken},
			},
		}
		if m.httpClient != nil {
			opts.HTTPClient = m.httpClient
		}

		conn, _, err := websocket.Dial(ctx, m.cfg.ServerURL, opts)
		if err != nil {
			delay := backoff.Next()
			m.log.Warn().Err(err).Dur("retry_in", delay).Msg("dial failed, retrying")
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			continue
		}

		backoff.Reset()
		m.handleConn(ctx, conn) // blocks until connection dies or ctx cancelled

		// After handleConn returns, check if we should stop or reconnect.
		select {
		case <-ctx.Done():
			return
		default:
			// Loop back to reconnect.
		}
	}
}

// handleConn manages a live connection. It:
//  1. Sends a NodeRegister frame as the first frame (before starting goroutines)
//  2. Starts reader, writer, and heartbeat goroutines
//  3. Waits for all three to exit
//  4. On clean shutdown (stopCh closed): sends NodeDisconnect frame before close
//  5. On connection drop: returns to dialLoop for reconnect
func (m *ConnectionManager) handleConn(ctx context.Context, conn *websocket.Conn) {
	defer conn.CloseNow()

	// Send NodeRegister as the first frame before starting other goroutines.
	if err := m.sendRegister(ctx, conn); err != nil {
		if ctx.Err() == nil {
			m.log.Warn().Err(err).Msg("failed to send NodeRegister, will reconnect")
		}
		return
	}

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	// Reader goroutine — required for coder/websocket Ping() to receive pongs.
	// Also delivers inbound envelopes to m.recvCh for Phase 13.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel() // signal writer/heartbeat to stop when reader exits
		for {
			_, data, err := conn.Read(connCtx)
			if err != nil {
				if connCtx.Err() == nil {
					m.log.Debug().Err(err).Msg("read error")
				}
				return
			}
			// Decode envelope and forward to receive channel.
			var env protocol.Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				m.log.Warn().Err(err).Msg("failed to decode inbound envelope")
				continue
			}
			// Non-blocking send to avoid stalling the reader.
			select {
			case m.recvCh <- &env:
			default:
				m.log.Warn().Str("type", env.Type).Msg("recvCh full, dropping inbound envelope")
			}
		}
	}()

	// Writer goroutine (XPORT-06) — sole caller of conn.Write for normal frames.
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.runWriter(connCtx, conn)
	}()

	// Heartbeat goroutine — sends periodic pings; triggers reconnect on pong timeout.
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.runHeartbeat(connCtx, conn, cancel)
	}()

	wg.Wait()

	// All goroutines have exited. Check if this is a clean shutdown.
	select {
	case <-m.stopCh:
		// Clean shutdown — send NodeDisconnect frame before closing.
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		disconnectEnv, err := protocol.Encode(protocol.TypeNodeDisconnect, generateMsgID(), protocol.NodeDisconnect{Reason: "shutdown"})
		if err == nil {
			data, merr := json.Marshal(disconnectEnv)
			if merr == nil {
				_ = conn.Write(shutCtx, websocket.MessageText, data)
			}
		}
		conn.Close(websocket.StatusNormalClosure, "shutdown")
	default:
		// Connection dropped — dialLoop will reconnect.
	}
}
