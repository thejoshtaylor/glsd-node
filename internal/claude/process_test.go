package claude

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"
)

// testEchoCommand returns the command and args for a process that echoes a fixed string to stdout.
// On Windows, uses cmd.exe /c echo; on Unix, uses echo.
func testEchoArgs(text string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", "echo " + text}
	}
	return "echo", []string{text}
}

// testSleepArgs returns a long-running command for kill tests.
// On Windows: ping -n 100 127.0.0.1; on Unix: sleep 100.
func testSleepArgs() (string, []string) {
	if runtime.GOOS == "windows" {
		return "ping", []string{"-n", "100", "127.0.0.1"}
	}
	return "sleep", []string{"100"}
}

// TestNewProcessSetsWaitDelay verifies that NewProcess sets WaitDelay to 5 seconds.
func TestNewProcessSetsWaitDelay(t *testing.T) {
	bin, args := testEchoArgs("{}")
	ctx := context.Background()
	env := os.Environ()

	p, err := NewProcess(ctx, bin, ".", "test", args, env)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	defer p.cmd.Wait() //nolint:errcheck

	if p.cmd.WaitDelay != 5*time.Second {
		t.Errorf("WaitDelay = %v, want %v", p.cmd.WaitDelay, 5*time.Second)
	}

	// Let the process complete.
	_ = p.Stream(ctx, func(ClaudeEvent) error { return nil })
}

// TestKillProcessWindows verifies that Kill uses taskkill on Windows.
func TestKillProcessWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	bin, args := testSleepArgs()
	ctx := context.Background()
	env := os.Environ()

	p, err := NewProcess(ctx, bin, ".", "", args, env)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	if err := p.Kill(); err != nil {
		t.Logf("Kill returned error (may be normal if process already exited): %v", err)
	}

	// Give taskkill a moment to act.
	time.Sleep(500 * time.Millisecond)

	// Process should be dead: cmd.Wait() should not block.
	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-done:
		// Good: process terminated.
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate within 3 seconds after Kill()")
	}
}

// TestKillProcessUnix verifies that Kill sends SIGTERM on Unix.
func TestKillProcessUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	bin, args := testSleepArgs()
	ctx := context.Background()
	env := os.Environ()

	p, err := NewProcess(ctx, bin, ".", "", args, env)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	if err := p.Kill(); err != nil {
		t.Logf("Kill returned error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-done:
		// Good.
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate within 3 seconds after Kill()")
	}
}

// TestStreamParsesNDJSON verifies that Stream parses NDJSON lines and delivers events to cb.
func TestStreamParsesNDJSON(t *testing.T) {
	// Build a small NDJSON payload.
	events := []ClaudeEvent{
		{Type: "assistant", SessionID: "s1"},
		{Type: "result", SessionID: "s1", Result: "done"},
	}

	// Write NDJSON to a temp file; use 'type' (Windows) or 'cat' (Unix) to emit it.
	// This avoids cmd.exe echo corruption of JSON special characters.
	tmpFile, err := os.CreateTemp("", "ndjson-test-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	for _, evt := range events {
		b, _ := json.Marshal(evt)
		tmpFile.Write(b)
		tmpFile.WriteString("\n")
	}
	tmpFile.Close()

	var bin string
	var args []string
	if runtime.GOOS == "windows" {
		bin = "cmd.exe"
		args = []string{"/c", "type", tmpFile.Name()}
	} else {
		bin = "cat"
		args = []string{tmpFile.Name()}
	}

	ctx := context.Background()
	env := os.Environ()

	p, err := NewProcess(ctx, bin, ".", "", args, env)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	var received []ClaudeEvent
	streamErr := p.Stream(ctx, func(evt ClaudeEvent) error {
		received = append(received, evt)
		return nil
	})
	// Exit code may be non-zero on some platforms; ignore.
	_ = streamErr

	if len(received) < 1 {
		t.Fatalf("expected at least 1 event, got %d — stderr: %q", len(received), p.Stderr())
	}

	// Verify at least one event was parsed with a recognized type.
	foundRecognized := false
	for _, e := range received {
		if e.Type == "assistant" || e.Type == "result" {
			foundRecognized = true
		}
	}
	if !foundRecognized {
		t.Errorf("no assistant or result events parsed from NDJSON stream; got: %+v", received)
	}
}

// TestStreamCapturesSessionID verifies that p.SessionID() returns the session_id from a result event.
func TestStreamCapturesSessionID(t *testing.T) {
	evt := ClaudeEvent{Type: "result", SessionID: "captured-session-id-123"}
	b, _ := json.Marshal(evt)

	// Write to temp file to avoid echo/shell corruption of JSON.
	tmpFile, err := os.CreateTemp("", "session-id-test-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write(b)
	tmpFile.WriteString("\n")
	tmpFile.Close()

	var bin string
	var args []string
	if runtime.GOOS == "windows" {
		bin = "cmd.exe"
		args = []string{"/c", "type", tmpFile.Name()}
	} else {
		bin = "cat"
		args = []string{tmpFile.Name()}
	}

	ctx := context.Background()
	env := os.Environ()

	p, err := NewProcess(ctx, bin, ".", "", args, env)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	_ = p.Stream(ctx, func(ClaudeEvent) error { return nil })

	if p.SessionID() != "captured-session-id-123" {
		t.Errorf("SessionID() = %q, want %q — stderr: %q", p.SessionID(), "captured-session-id-123", p.Stderr())
	}
}

// TestStreamDetectsContextLimit verifies that ContextLimitHit() returns true when
// stderr contains a context limit error pattern.
func TestStreamDetectsContextLimit(t *testing.T) {
	// We want stderr to contain the context limit pattern.
	// On Windows, use cmd.exe to write to stderr via 1>&2.
	// On Unix, use sh -c.
	var bin string
	var args []string

	contextLimitMsg := "prompt too long for this request"

	if runtime.GOOS == "windows" {
		bin = "cmd.exe"
		// echo to stderr in cmd.exe
		args = []string{"/c", "echo " + contextLimitMsg + " 1>&2"}
	} else {
		bin = "sh"
		args = []string{"-c", "echo '" + contextLimitMsg + "' >&2"}
	}

	ctx := context.Background()
	env := os.Environ()

	p, err := NewProcess(ctx, bin, ".", "", args, env)
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	_ = p.Stream(ctx, func(ClaudeEvent) error { return nil })

	if !p.ContextLimitHit() {
		t.Errorf("ContextLimitHit() = false, want true after stderr: %q", p.Stderr())
	}
}
