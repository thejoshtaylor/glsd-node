// Package protocol defines all wire message types for the WebSocket protocol
// between nodes and the central server. Every WebSocket message in both
// directions is framed as an Envelope, with the Payload decoded based on Type.
package protocol

import "encoding/json"

// Version is the current node protocol version.
const Version = "1.2.0"

// Message type constants — used as the Envelope.Type discriminator.
const (
	TypeNodeRegister     = "node_register"
	TypeExecute          = "execute"
	TypeKill             = "kill"
	TypeStatusRequest    = "status_request"
	TypeStreamEvent      = "stream_event"
	TypeInstanceStarted  = "instance_started"
	TypeInstanceFinished = "instance_finished"
	TypeInstanceError    = "instance_error"
	TypeNodeDisconnect   = "node_disconnect"
)

// Envelope is the outer frame for every WebSocket message in both directions.
// Payload is held as raw JSON until the dispatcher inspects Type and calls Decode.
type Envelope struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Decode unmarshals the Payload into dst. Call after inspecting Type to
// determine the correct destination struct.
func (e *Envelope) Decode(dst any) error {
	return json.Unmarshal(e.Payload, dst)
}

// Encode builds an Envelope from a typed payload. Returns an error if payload
// cannot be marshaled to JSON (e.g. contains channel or function types).
func Encode(msgType, msgID string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Type: msgType, ID: msgID, Payload: raw}, nil
}

// --- Inbound (server-to-node) command structs ---

// ExecuteCmd instructs the node to start a new Claude CLI instance.
type ExecuteCmd struct {
	// InstanceID is the server-assigned unique identifier for this execution.
	InstanceID string `json:"instance_id"`
	// Project is the project name configured on the node.
	Project string `json:"project"`
	// WorkDir is the working directory for the Claude CLI subprocess.
	WorkDir string `json:"work_dir"`
	// Prompt is the user message to send to Claude.
	Prompt string `json:"prompt"`
	// SessionID is the Claude session to resume (empty string starts a new session).
	SessionID string `json:"session_id,omitempty"`
}

// KillCmd instructs the node to terminate a running Claude CLI instance.
type KillCmd struct {
	// InstanceID identifies the running instance to kill.
	InstanceID string `json:"instance_id"`
}

// StatusRequest asks the node to send a fresh NodeRegister with current state.
// Empty struct — carries no additional fields.
type StatusRequest struct{}

// --- Outbound (node-to-server) event structs ---

// NodeRegister is sent by the node on WebSocket connect (and in response to
// StatusRequest) to announce its identity and current running state.
//
// NOTE: RunningInstances must NOT use omitempty — it must marshal as [] (not
// null) when no instances are running. Callers must initialize with
// make([]InstanceSummary, 0) rather than leaving the field nil.
type NodeRegister struct {
	// NodeID is the stable identifier for this node (e.g. hostname or UUID).
	NodeID string `json:"node_id"`
	// Platform is the OS/arch string (e.g. "windows", "linux").
	Platform string `json:"platform"`
	// Version is the node software version (should match protocol.Version).
	Version string `json:"version"`
	// Projects is the list of project names this node has configured.
	Projects []string `json:"projects"`
	// RunningInstances is the list of currently active Claude CLI instances.
	// Always marshals as [] rather than null — omitempty is intentionally absent.
	RunningInstances []InstanceSummary `json:"running_instances"`
}

// InstanceSummary is a brief description of a running Claude CLI instance,
// included in NodeRegister payloads.
type InstanceSummary struct {
	// InstanceID is the server-assigned unique identifier for the instance.
	InstanceID string `json:"instance_id"`
	// Project is the project name the instance is running under.
	Project string `json:"project"`
	// SessionID is the Claude session identifier for this instance (if known).
	SessionID string `json:"session_id,omitempty"`
}

// StreamEvent carries a chunk of output from a running Claude CLI instance.
type StreamEvent struct {
	// InstanceID identifies which instance produced the output.
	InstanceID string `json:"instance_id"`
	// Data is the NDJSON line or text chunk from the Claude CLI.
	Data string `json:"data"`
}

// InstanceStarted is sent when a Claude CLI subprocess has been launched.
type InstanceStarted struct {
	// InstanceID identifies the newly started instance.
	InstanceID string `json:"instance_id"`
	// Project is the project name the instance is running under.
	Project string `json:"project"`
	// SessionID is the Claude session identifier (set after the first response).
	SessionID string `json:"session_id,omitempty"`
}

// InstanceFinished is sent when a Claude CLI subprocess exits cleanly.
type InstanceFinished struct {
	// InstanceID identifies the instance that finished.
	InstanceID string `json:"instance_id"`
	// ExitCode is the OS exit code from the Claude CLI process.
	ExitCode int `json:"exit_code"`
}

// InstanceError is sent when a Claude CLI subprocess exits with an error
// or the node encounters a fatal error managing the instance.
type InstanceError struct {
	// InstanceID identifies the instance that errored.
	InstanceID string `json:"instance_id"`
	// Error is a human-readable description of the failure.
	Error string `json:"error"`
}

// NodeDisconnect is sent by the node before a clean WebSocket close.
// Reason is optional — empty string means normal shutdown.
type NodeDisconnect struct {
	Reason string `json:"reason,omitempty"`
}
