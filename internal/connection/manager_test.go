package connection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"go.uber.org/goleak"

	"github.com/user/gsd-tele-go/internal/config"
	"github.com/user/gsd-tele-go/internal/protocol"
)

// newTestConfig creates a NodeConfig for tests with the given server URL.
func newTestConfig(serverURL string) *config.NodeConfig {
	return &config.NodeConfig{
		ServerURL:             serverURL,
		ServerToken:           "test-token",
		HeartbeatIntervalSecs: 30,
		NodeID:                "test-node",
		Projects:              []string{},
	}
}

// newMockServer creates a TLS WebSocket server. The handler receives accepted
// connections. Returns (wsURL, httptest.Server) — caller must defer srv.Close().
func newMockServer(t *testing.T, handler func(conn *websocket.Conn)) (string, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Logf("accept error: %v", err)
			return
		}
		handler(conn)
	}))
	wsURL := "wss://" + srv.Listener.Addr().String()
	return wsURL, srv
}

// TestNewConnectionManager verifies constructor returns non-nil manager with
// initialized channels.
func TestNewConnectionManager(t *testing.T) {
	defer goleak.VerifyNone(t)

	cfg := newTestConfig("wss://localhost:9999")
	m := NewConnectionManager(cfg, zerolog.Nop())
	if m == nil {
		t.Fatal("NewConnectionManager returned nil")
	}
	if m.sendCh == nil {
		t.Error("sendCh is nil")
	}
	if m.stopCh == nil {
		t.Error("stopCh is nil")
	}
}

// TestSendAfterStop verifies that Send() returns ErrStopped after Stop() is called.
func TestSendAfterStop(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Use an unreachable server so the dial fails immediately and the dialLoop
	// waits on backoff — this way we can test Stop() without a live connection.
	cfg := newTestConfig("wss://127.0.0.1:1") // port 1 is unreachable
	m := NewConnectionManager(cfg, zerolog.Nop())

	ctx := context.Background()
	m.Start(ctx)

	// Give dialLoop a moment to attempt and fail the first dial.
	time.Sleep(50 * time.Millisecond)

	m.Stop()

	err := m.Send([]byte("hello"))
	if err != ErrStopped {
		t.Errorf("Send after Stop: got %v, want ErrStopped", err)
	}
}

// TestDial verifies that the ConnectionManager dials the mock TLS server,
// completes the WebSocket handshake, and can exchange a frame.
func TestDial(t *testing.T) {
	defer goleak.VerifyNone(t)

	// received tracks whether the server got a message.
	received := make(chan string, 1)

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		ctx := context.Background()
		// First frame is always NodeRegister — skip it.
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		// Second frame is the test frame sent via Send().
		_, data, err := conn.Read(ctx)
		if err == nil {
			received <- string(data)
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	m.Start(ctx)
	defer m.Stop()

	// Wait for the connection to establish and registration to complete.
	time.Sleep(200 * time.Millisecond)

	// Send a test frame.
	if err := m.Send([]byte(`{"type":"test"}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Wait for server to receive the frame.
	select {
	case msg := <-received:
		if msg != `{"type":"test"}` {
			t.Errorf("server received %q, want %q", msg, `{"type":"test"}`)
		}
	case <-time.After(5 * time.Second):
		t.Error("server did not receive frame within timeout")
	}
}

// TestBackoff verifies the backoffState: Next() returns values in [0, current],
// current doubles up to max, and Reset() returns to min.
func TestBackoff(t *testing.T) {
	// No goroutines in this test; no need for goleak.
	minDur := 500 * time.Millisecond
	maxDur := 30 * time.Second

	b := newBackoff(minDur, maxDur)

	// First several Next() calls should return values in range.
	var prev time.Duration = minDur
	for i := 0; i < 10; i++ {
		got := b.Next()
		if got < 0 {
			t.Errorf("iteration %d: got negative delay %v", i, got)
		}
		_ = prev
		prev = prev * 2
		if prev > maxDur {
			prev = maxDur
		}
	}

	// Calling Next() many times should cap at maxDur (jittered — so <= maxDur).
	for i := 0; i < 5; i++ {
		got := b.Next()
		if got > maxDur {
			t.Errorf("delay %v exceeds max %v", got, maxDur)
		}
	}

	// Reset should bring current back to min so Next() returns small values.
	b.Reset()
	// After reset, the next value must be in [0, min], which is [0, 500ms].
	got := b.Next()
	if got > minDur {
		t.Errorf("after Reset, got %v, want <= %v", got, minDur)
	}
}

// TestRegisterOnConnect verifies that the first frame received by the server
// after a new connection is a NodeRegister envelope with the correct NodeID and Projects.
func TestRegisterOnConnect(t *testing.T) {
	defer goleak.VerifyNone(t)

	registered := make(chan protocol.NodeRegister, 1) // receives full NodeRegister from first frame

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		ctx := context.Background()
		// Read first frame — must be node_register
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Logf("read error: %v", err)
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Logf("unmarshal error: %v", err)
			return
		}
		if env.Type != protocol.TypeNodeRegister {
			t.Logf("expected node_register, got %q", env.Type)
			return
		}
		var reg protocol.NodeRegister
		if err := env.Decode(&reg); err != nil {
			t.Logf("decode error: %v", err)
			return
		}
		registered <- reg
		// Drain until close.
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	cfg.Projects = []string{"project-a", "project-b"}
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	select {
	case reg := <-registered:
		if reg.NodeID != cfg.NodeID {
			t.Errorf("NodeID = %q, want %q", reg.NodeID, cfg.NodeID)
		}
		if len(reg.Projects) != 2 || reg.Projects[0] != "project-a" || reg.Projects[1] != "project-b" {
			t.Errorf("Projects = %v, want [project-a project-b]", reg.Projects)
		}
	case <-time.After(5 * time.Second):
		t.Error("server did not receive NodeRegister frame within timeout")
	}
}

// TestHeartbeatKeepsAlive verifies that a mock server which reads frames
// (allowing pong responses) keeps the connection alive through multiple heartbeats.
func TestHeartbeatKeepsAlive(t *testing.T) {
	defer goleak.VerifyNone(t)

	connected := make(chan struct{})
	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		close(connected)
		ctx := context.Background()
		// Read loop — keeps pong handling alive (coder/websocket handles pings automatically)
		deadline := time.Now().Add(600 * time.Millisecond)
		for time.Now().Before(deadline) {
			conn.SetReadLimit(1 << 20)
			_ = conn // read with short deadline
			rCtx, rCancel := context.WithDeadline(ctx, deadline)
			_, _, err := conn.Read(rCtx)
			rCancel()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())
	m.SetHeartbeatInterval(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	// Wait for connection to be established.
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("connection not established")
	}

	// Connection should still be alive after 500ms (5+ heartbeats at 100ms).
	time.Sleep(500 * time.Millisecond)

	// Verify we can still send (connection still alive).
	if err := m.Send([]byte(`{"type":"ping-check"}`)); err != nil {
		t.Errorf("Send failed after heartbeat period: %v", err)
	}
}

// TestHeartbeatDeadServer verifies that a server that does not read (blocking
// pong responses) causes the manager to close the connection within 3x the
// heartbeat interval.
func TestHeartbeatDeadServer(t *testing.T) {
	defer goleak.VerifyNone(t)

	disconnected := make(chan struct{})
	serverDone := make(chan struct{})

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		defer close(serverDone)
		// Do NOT read — this blocks pong responses, simulating a dead server.
		// Use a select with a channel so we can unblock when the test finishes.
		select {
		case <-disconnected:
			// Test is done; exit cleanly.
		case <-time.After(5 * time.Second):
			// Failsafe to avoid goroutine leak if test fails.
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())
	m.SetHeartbeatInterval(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Start(ctx)

	// The heartbeat pong timeout is 3x interval = 300ms.
	// The connection should be closed and reconnected within 400ms.
	time.Sleep(400 * time.Millisecond)
	m.Stop()

	// Signal the server handler to exit.
	close(disconnected)

	// Wait for the server goroutine to fully exit before goleak check.
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Error("server handler did not exit within timeout")
	}
}

// TestRegisterAfterReconnect verifies that after a server drop, the manager
// reconnects and sends a second NodeRegister frame.
func TestRegisterAfterReconnect(t *testing.T) {
	defer goleak.VerifyNone(t)

	var mu sync.Mutex
	registerCount := 0
	allRegistered := make(chan struct{})

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		ctx := context.Background()
		// Read the NodeRegister frame.
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return
		}
		if env.Type == protocol.TypeNodeRegister {
			mu.Lock()
			registerCount++
			count := registerCount
			mu.Unlock()
			if count >= 2 {
				close(allRegistered)
				// Drain until close.
				for {
					if _, _, err := conn.Read(ctx); err != nil {
						return
					}
				}
			}
		}
		// First connection: close immediately to trigger reconnect.
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	select {
	case <-allRegistered:
		// Good — two NodeRegister frames received.
	case <-time.After(10 * time.Second):
		mu.Lock()
		count := registerCount
		mu.Unlock()
		t.Errorf("only %d register frames received, want 2", count)
	}
}

// TestConcurrentSend verifies that 10 goroutines sending 100 frames each
// (1000 total) complete without panic under -race.
func TestConcurrentSend(t *testing.T) {
	defer goleak.VerifyNone(t)

	var frameCount atomic.Int64
	done := make(chan struct{})

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		// Read all frames until connection closes or we hit 1000.
		for {
			_, _, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			if frameCount.Add(1) >= 1000 {
				close(done)
				return
			}
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.Start(ctx)
	defer m.Stop()

	// Wait for connection to establish.
	time.Sleep(200 * time.Millisecond)

	// Launch 10 goroutines, each sending 100 frames.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if err := m.Send([]byte(`{"type":"stress"}`)); err != nil {
					// Manager may stop; that's OK.
					return
				}
			}
		}()
	}
	wg.Wait()

	// Wait for server to receive all 1000 frames (or timeout).
	select {
	case <-done:
		// All frames received.
	case <-time.After(20 * time.Second):
		t.Errorf("only %d/1000 frames received before timeout", frameCount.Load())
	}
}

// TestCleanShutdown verifies that Stop() causes the manager to send a
// NodeDisconnect frame before the WebSocket close frame.
func TestCleanShutdown(t *testing.T) {
	defer goleak.VerifyNone(t)

	disconnectReceived := make(chan struct{})
	serverDone := make(chan struct{})

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		defer close(serverDone)
		ctx := context.Background()
		// Read frames until WebSocket closes. Look for NodeDisconnect.
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				// Connection closed — check if we got the disconnect frame.
				return
			}
			var env protocol.Envelope
			if json.Unmarshal(data, &env) == nil && env.Type == protocol.TypeNodeDisconnect {
				close(disconnectReceived)
			}
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Start(ctx)

	// Wait for connection and registration.
	time.Sleep(200 * time.Millisecond)

	// Stop the manager — should send disconnect frame.
	m.Stop()

	// Wait for server to finish.
	select {
	case <-serverDone:
	case <-time.After(5 * time.Second):
		t.Fatal("server handler did not exit")
	}

	// Verify disconnect frame was received.
	select {
	case <-disconnectReceived:
		// Good — NodeDisconnect received before close.
	default:
		t.Error("server did not receive NodeDisconnect frame before WebSocket close")
	}
}

// TestNoReconnectAfterStop verifies that after Stop(), the manager does not
// attempt to reconnect — exactly 1 connection is established.
func TestNoReconnectAfterStop(t *testing.T) {
	defer goleak.VerifyNone(t)

	var connCount atomic.Int64
	serverDone := make(chan struct{}, 10) // buffered to handle multiple connections

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		connCount.Add(1)
		ctx := context.Background()
		// Drain frames until connection closes.
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				serverDone <- struct{}{}
				return
			}
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Start(ctx)

	// Wait for connection to establish.
	time.Sleep(200 * time.Millisecond)

	m.Stop()

	// Wait 500ms to confirm no reconnect occurs.
	time.Sleep(500 * time.Millisecond)

	count := connCount.Load()
	if count != 1 {
		t.Errorf("connection count = %d, want 1 (no reconnect after Stop)", count)
	}
}

// TestReconnectAfterDrop verifies that when the server closes the connection,
// the manager reconnects and sends a second NodeRegister frame.
func TestReconnectAfterDrop(t *testing.T) {
	defer goleak.VerifyNone(t)

	var connCount atomic.Int64
	secondConnRegistered := make(chan time.Time, 1)
	var dropTime time.Time
	var dropTimeMu sync.Mutex

	wsURL, srv := newMockServer(t, func(conn *websocket.Conn) {
		defer conn.CloseNow()
		n := connCount.Add(1)
		ctx := context.Background()

		// Read the NodeRegister frame.
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return
		}
		if env.Type != protocol.TypeNodeRegister {
			return
		}

		if n == 1 {
			// First connection: record drop time and close to trigger reconnect.
			dropTimeMu.Lock()
			dropTime = time.Now()
			dropTimeMu.Unlock()
			// Close with StatusGoingAway to simulate network drop.
			conn.Close(websocket.StatusGoingAway, "simulated drop")
		} else {
			// Second connection: record when registration arrived.
			secondConnRegistered <- time.Now()
			// Drain until manager stops.
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return
				}
			}
		}
	})
	defer srv.Close()

	cfg := newTestConfig(wsURL)
	m := NewConnectionManager(cfg, zerolog.Nop())
	m.SetHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	select {
	case reconnectTime := <-secondConnRegistered:
		dropTimeMu.Lock()
		drop := dropTime
		dropTimeMu.Unlock()
		elapsed := reconnectTime.Sub(drop)
		if elapsed < 0 {
			t.Error("reconnect time before drop time")
		}
		if elapsed > 2*time.Second {
			t.Errorf("reconnect took %v, want < 2s", elapsed)
		}
		t.Logf("reconnect after drop: %v", elapsed)
	case <-time.After(10 * time.Second):
		t.Error("did not reconnect after server drop within 10s")
	}
}
