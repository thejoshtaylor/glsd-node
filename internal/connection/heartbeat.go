package connection

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

// runHeartbeat sends periodic ping frames to the server. If a pong is not
// received within 3x the heartbeat interval, the connection context is
// cancelled (triggering a reconnect).
//
// runHeartbeat is run as a goroutine inside handleConn alongside reader and
// writer. It exits when connCtx is done or a ping times out.
func (m *ConnectionManager) runHeartbeat(ctx context.Context, conn *websocket.Conn, cancel context.CancelFunc) {
	interval := m.heartbeatInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pingCtx, pcancel := context.WithTimeout(ctx, 3*interval)
			err := conn.Ping(pingCtx)
			pcancel()
			if err != nil {
				m.log.Warn().Err(err).Msg("heartbeat ping failed, triggering reconnect")
				cancel()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
