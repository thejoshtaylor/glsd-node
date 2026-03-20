package connection

import (
	"context"

	"github.com/coder/websocket"
)

// runWriter is the single writer goroutine (XPORT-06). It drains sendCh and
// calls conn.Write for each frame. This is the ONLY code path that calls
// conn.Write — all outbound frames must be submitted via m.Send().
//
// runWriter exits when connCtx is cancelled or a write error occurs.
func (m *ConnectionManager) runWriter(connCtx context.Context, conn *websocket.Conn) {
	for {
		select {
		case data := <-m.sendCh:
			if err := conn.Write(connCtx, websocket.MessageText, data); err != nil {
				if connCtx.Err() == nil {
					m.log.Debug().Err(err).Msg("write error")
				}
				return
			}
		case <-connCtx.Done():
			return
		}
	}
}
