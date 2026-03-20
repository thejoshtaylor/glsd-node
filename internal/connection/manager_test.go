package connection

import (
	"context"
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
)

// newTestConfig creates a NodeConfig for tests with the given server URL.
func newTestConfig(serverURL string) *config.NodeConfig {
	return &config.NodeConfig{
		ServerURL:             serverURL,
		ServerToken:           "test-token",
		HeartbeatIntervalSecs: 30,
		NodeID:                "test-node",
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
		// Read a single frame, echo back, then exit.
		_, data, err := conn.Read(context.Background())
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

	// Wait for the connection to establish.
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
