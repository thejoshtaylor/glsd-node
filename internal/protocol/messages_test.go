// Package protocol provides wire message type definitions for the WebSocket
// protocol between nodes and the central server.
package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEnvelopeRoundTrip verifies that every message type survives an
// Encode → json.Marshal → json.Unmarshal → Decode round-trip without data loss.
func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		msgType string
		payload any
		verify  func(t *testing.T, env Envelope)
	}{
		{
			name:    "node_register",
			msgType: TypeNodeRegister,
			payload: NodeRegister{
				NodeID:           "node-abc",
				Platform:         "windows",
				Version:          "1.2.0",
				Projects:         []string{"proj-a", "proj-b"},
				RunningInstances: make([]InstanceSummary, 0),
			},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got NodeRegister
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.NodeID != "node-abc" {
					t.Errorf("NodeID: got %q, want %q", got.NodeID, "node-abc")
				}
				if got.Platform != "windows" {
					t.Errorf("Platform: got %q, want %q", got.Platform, "windows")
				}
				if got.Version != "1.2.0" {
					t.Errorf("Version: got %q, want %q", got.Version, "1.2.0")
				}
				if len(got.Projects) != 2 {
					t.Errorf("Projects len: got %d, want 2", len(got.Projects))
				}
			},
		},
		{
			name:    "execute",
			msgType: TypeExecute,
			payload: ExecuteCmd{
				InstanceID: "inst-1",
				Project:    "my-project",
				WorkDir:    "C:/code/my-project",
				Prompt:     "run tests",
				SessionID:  "sess-xyz",
			},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got ExecuteCmd
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.InstanceID != "inst-1" {
					t.Errorf("InstanceID: got %q, want %q", got.InstanceID, "inst-1")
				}
				if got.Project != "my-project" {
					t.Errorf("Project: got %q, want %q", got.Project, "my-project")
				}
				if got.WorkDir != "C:/code/my-project" {
					t.Errorf("WorkDir: got %q, want %q", got.WorkDir, "C:/code/my-project")
				}
				if got.Prompt != "run tests" {
					t.Errorf("Prompt: got %q, want %q", got.Prompt, "run tests")
				}
				if got.SessionID != "sess-xyz" {
					t.Errorf("SessionID: got %q, want %q", got.SessionID, "sess-xyz")
				}
			},
		},
		{
			name:    "kill",
			msgType: TypeKill,
			payload: KillCmd{InstanceID: "inst-2"},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got KillCmd
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.InstanceID != "inst-2" {
					t.Errorf("InstanceID: got %q, want %q", got.InstanceID, "inst-2")
				}
			},
		},
		{
			name:    "status_request",
			msgType: TypeStatusRequest,
			payload: StatusRequest{},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got StatusRequest
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				// StatusRequest is an empty struct; no fields to check.
			},
		},
		{
			name:    "stream_event",
			msgType: TypeStreamEvent,
			payload: StreamEvent{InstanceID: "inst-3", Data: "hello output"},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got StreamEvent
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.InstanceID != "inst-3" {
					t.Errorf("InstanceID: got %q, want %q", got.InstanceID, "inst-3")
				}
				if got.Data != "hello output" {
					t.Errorf("Data: got %q, want %q", got.Data, "hello output")
				}
			},
		},
		{
			name:    "instance_started",
			msgType: TypeInstanceStarted,
			payload: InstanceStarted{InstanceID: "inst-4", Project: "proj-x", SessionID: "sess-1"},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got InstanceStarted
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.InstanceID != "inst-4" {
					t.Errorf("InstanceID: got %q, want %q", got.InstanceID, "inst-4")
				}
				if got.Project != "proj-x" {
					t.Errorf("Project: got %q, want %q", got.Project, "proj-x")
				}
				if got.SessionID != "sess-1" {
					t.Errorf("SessionID: got %q, want %q", got.SessionID, "sess-1")
				}
			},
		},
		{
			name:    "instance_finished",
			msgType: TypeInstanceFinished,
			payload: InstanceFinished{InstanceID: "inst-5", ExitCode: 0},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got InstanceFinished
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.InstanceID != "inst-5" {
					t.Errorf("InstanceID: got %q, want %q", got.InstanceID, "inst-5")
				}
				// ExitCode 0 must be preserved — not omitted.
				if got.ExitCode != 0 {
					t.Errorf("ExitCode: got %d, want 0", got.ExitCode)
				}
			},
		},
		{
			name:    "instance_error",
			msgType: TypeInstanceError,
			payload: InstanceError{InstanceID: "inst-6", Error: "process crashed"},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got InstanceError
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.InstanceID != "inst-6" {
					t.Errorf("InstanceID: got %q, want %q", got.InstanceID, "inst-6")
				}
				if got.Error != "process crashed" {
					t.Errorf("Error: got %q, want %q", got.Error, "process crashed")
				}
			},
		},
		{
			name:    "node_disconnect",
			msgType: TypeNodeDisconnect,
			payload: NodeDisconnect{Reason: "shutting down"},
			verify: func(t *testing.T, env Envelope) {
				t.Helper()
				var got NodeDisconnect
				if err := env.Decode(&got); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if got.Reason != "shutting down" {
					t.Errorf("Reason: got %q, want %q", got.Reason, "shutting down")
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Encode payload into Envelope.
			env, err := Encode(tc.msgType, "msg-id-001", tc.payload)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			// Verify Type and ID are preserved.
			if env.Type != tc.msgType {
				t.Errorf("Type: got %q, want %q", env.Type, tc.msgType)
			}
			if env.ID != "msg-id-001" {
				t.Errorf("ID: got %q, want %q", env.ID, "msg-id-001")
			}

			// Marshal Envelope to JSON, then unmarshal back.
			raw, err := json.Marshal(env)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			var env2 Envelope
			if err := json.Unmarshal(raw, &env2); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}

			// Type and ID survive the JSON round-trip.
			if env2.Type != tc.msgType {
				t.Errorf("round-trip Type: got %q, want %q", env2.Type, tc.msgType)
			}
			if env2.ID != "msg-id-001" {
				t.Errorf("round-trip ID: got %q, want %q", env2.ID, "msg-id-001")
			}

			// Decode payload and run type-specific assertions.
			tc.verify(t, env2)
		})
	}
}

// TestNodeRegisterEmptyInstances asserts that a NodeRegister with no running
// instances marshals to JSON with "running_instances":[] (not null).
func TestNodeRegisterEmptyInstances(t *testing.T) {
	t.Parallel()

	nr := NodeRegister{
		NodeID:           "node-xyz",
		Platform:         "windows",
		Version:          Version,
		Projects:         []string{},
		RunningInstances: make([]InstanceSummary, 0),
	}

	raw, err := json.Marshal(nr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	got := string(raw)
	if !strings.Contains(got, `"running_instances":[]`) {
		t.Errorf("expected running_instances:[] in JSON, got: %s", got)
	}
	// Explicit guard: must not be null.
	if strings.Contains(got, `"running_instances":null`) {
		t.Errorf("running_instances must not be null, got: %s", got)
	}
}

// TestEncodeError verifies that Encode returns an error when the payload
// cannot be marshaled to JSON (e.g. a channel type).
func TestEncodeError(t *testing.T) {
	t.Parallel()

	_, err := Encode(TypeStreamEvent, "msg-err", make(chan int))
	if err == nil {
		t.Error("expected Encode to return an error for unmarshalable type, got nil")
	}
}
