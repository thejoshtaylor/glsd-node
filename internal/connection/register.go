package connection

import (
	"context"
	"encoding/json"
	"runtime"

	"github.com/coder/websocket"

	"github.com/user/gsd-tele-go/internal/protocol"
)

// sendRegister builds and sends a NodeRegister envelope as the first frame on
// a new connection. It writes directly to conn (not via sendCh) to guarantee
// NodeRegister is the first frame sent, before the writer goroutine starts.
func (m *ConnectionManager) sendRegister(ctx context.Context, conn *websocket.Conn) error {
	reg := protocol.NodeRegister{
		NodeID:           m.cfg.NodeID,
		Platform:         runtime.GOOS,
		Version:          protocol.Version,
		Projects:         []string{},
		RunningInstances: make([]protocol.InstanceSummary, 0),
	}

	env, err := protocol.Encode(protocol.TypeNodeRegister, generateMsgID(), reg)
	if err != nil {
		return err
	}

	data, err := json.Marshal(env)
	if err != nil {
		return err
	}

	return conn.Write(ctx, websocket.MessageText, data)
}
