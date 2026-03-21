package connection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"

	"github.com/user/gsd-tele-go/internal/protocol"
)

// runWriter is the single writer goroutine (XPORT-06). It drains sendCh and
// calls conn.Write for each frame. This is the ONLY code path that calls
// conn.Write — all outbound frames must be submitted via m.Send().
//
// On clean shutdown (m.stopCh closed), runWriter sends a NodeDisconnect frame
// and then sends a WebSocket close frame to stop the reader goroutine. This
// ordering ensures the disconnect frame is sent while the connection is healthy,
// before any context cancellation closes the underlying transport.
//
// The cancel arg is connCtx's cancel — used only as a fallback to unblock the
// reader if conn.Close fails.
//
// runWriter exits when connCtx is cancelled or a write error occurs.
func (m *ConnectionManager) runWriter(connCtx context.Context, conn *websocket.Conn, cancel context.CancelFunc) {
	for {
		// Priority check: if stopCh is already closed, handle clean shutdown
		// immediately without entering the main select (avoids race with
		// connCtx.Done() firing simultaneously when Stop() cancels the context).
		select {
		case <-m.stopCh:
			m.sendDisconnectFrame(conn)
			conn.Close(websocket.StatusNormalClosure, "shutdown")
			return
		default:
		}

		select {
		case data := <-m.sendCh:
			if err := conn.Write(connCtx, websocket.MessageText, data); err != nil {
				if connCtx.Err() == nil {
					m.log.Debug().Err(err).Msg("write error")
				}
				return
			}
		case <-m.stopCh:
			// Clean shutdown — send NodeDisconnect, then close the WebSocket.
			m.sendDisconnectFrame(conn)
			conn.Close(websocket.StatusNormalClosure, "shutdown")
			return
		case <-connCtx.Done():
			return
		}
	}
}

// sendDisconnectFrame writes a NodeDisconnect envelope to conn. It uses a
// fresh context with a short timeout to ensure delivery without blocking.
// Called only from runWriter as its final action before clean shutdown.
func (m *ConnectionManager) sendDisconnectFrame(conn *websocket.Conn) {
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutCancel()

	disconnectEnv, err := protocol.Encode(protocol.TypeNodeDisconnect, generateMsgID(), protocol.NodeDisconnect{Reason: "shutdown"})
	if err != nil {
		return
	}
	data, err := json.Marshal(disconnectEnv)
	if err != nil {
		return
	}
	_ = conn.Write(shutCtx, websocket.MessageText, data)
}
