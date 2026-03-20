package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrContextLimit is returned by Stream when the Claude CLI reports that the
// context limit has been exceeded (matched by isContextLimitError).
var ErrContextLimit = errors.New("claude: context limit exceeded")

// StatusCallback is called for each parsed ClaudeEvent during streaming.
// Returning a non-nil error aborts the stream (the error is propagated to Stream's caller).
type StatusCallback func(event ClaudeEvent) error

// Process wraps an exec.Cmd running the Claude CLI and provides methods
// for streaming its NDJSON output and killing the process tree.
type Process struct {
	cmd             *exec.Cmd
	stdout          io.ReadCloser
	stderr          io.ReadCloser
	stderrBuf       strings.Builder
	contextLimitHit bool
	sessionID       string
}

// NewProcess creates and starts a Claude CLI subprocess.
//
// The prompt is written to stdin and stdin is closed so Claude starts processing.
// The caller must pass the appropriate CLI args (from BuildArgs) and a filtered
// environment (from config.FilteredEnv) to prevent nested session errors.
//
// cmd.WaitDelay is set to 5 seconds to prevent goroutine leaks when the process
// is killed (Go 1.20+ feature).
func NewProcess(ctx context.Context, claudePath, workingDir, prompt string, args []string, env []string) (*Process, error) {
	cmd := exec.CommandContext(ctx, claudePath, args...)
	cmd.Dir = workingDir
	cmd.Env = env

	// WaitDelay caps how long cmd.Wait() blocks for I/O goroutines after the
	// process exits. Without this, killed subprocesses can leave goroutines
	// blocked on pipe reads indefinitely (Pitfall 3 from research).
	cmd.WaitDelay = 5 * time.Second

	// Set up stdin pipe — we write the prompt then close it.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	// Set up stdout and stderr pipes.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Write prompt and close stdin so Claude starts processing.
	if _, err := stdin.Write([]byte(prompt)); err != nil {
		// Non-fatal: process may have already consumed stdin or exited.
		_ = stdin.Close()
	} else {
		_ = stdin.Close()
	}

	p := &Process{cmd: cmd}

	// Attach scanner goroutines. Store references to pipes so Stream can use them.
	// We save the pipes on cmd via the already-set Stdout/Stderr fields — but since
	// we used StdoutPipe/StderrPipe, cmd.Stdout/cmd.Stderr are nil. Instead, pass
	// them to the Process via unexported fields.
	p.stdout = stdout
	p.stderr = stderr

	return p, nil
}

// Stream reads NDJSON events from stdout and dispatches them to cb.
// A separate goroutine reads stderr and checks for context limit errors.
// Stream blocks until stdout is exhausted, then waits for stderr and calls cmd.Wait().
//
// If the context limit is hit (detected from stderr), Stream returns ErrContextLimit.
// Any error from cmd.Wait() is returned as-is (unless context limit takes precedence).
func (p *Process) Stream(ctx context.Context, cb StatusCallback) error {
	scanner := bufio.NewScanner(p.stdout)
	// 1 MB buffer to handle large tool outputs.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Stderr goroutine: collect stderr lines and check for context limit patterns.
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		s := bufio.NewScanner(p.stderr)
		for s.Scan() {
			line := s.Text()
			if strings.TrimSpace(line) != "" {
				p.stderrBuf.WriteString(line + "\n")
				if isContextLimitError(line) {
					p.contextLimitHit = true
				}
			}
		}
	}()

	// Main NDJSON scanning loop.
	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		var event ClaudeEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			// Skip non-JSON or partial lines (e.g. debug output).
			continue
		}

		// Capture session ID from result events.
		if event.Type == "result" && event.SessionID != "" {
			p.sessionID = event.SessionID
		}

		// Check for context limit in result text.
		if event.Type == "result" && isContextLimitError(event.Result) {
			p.contextLimitHit = true
		}

		if err := cb(event); err != nil {
			// Caller can abort streaming by returning an error from the callback.
			break
		}
	}

	// Wait for stderr goroutine to drain before calling cmd.Wait().
	<-stderrDone

	waitErr := p.cmd.Wait()

	if p.contextLimitHit {
		return ErrContextLimit
	}
	return waitErr
}

// Kill terminates the subprocess process tree.
//
// On Windows, taskkill /T /F is used to kill the entire process tree including
// any cmd.exe wrapper and Claude child processes (Pitfall 1 from research).
// On Unix, SIGTERM is sent to the process.
func (p *Process) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		pid := strconv.Itoa(p.cmd.Process.Pid)
		return exec.Command("taskkill", "/pid", pid, "/T", "/F").Run()
	}
	return p.cmd.Process.Signal(syscall.SIGTERM)
}

// SessionID returns the Claude session ID captured from result events.
// Returns empty string if no result event has been received yet.
func (p *Process) SessionID() string {
	return p.sessionID
}

// Stderr returns the accumulated stderr output.
func (p *Process) Stderr() string {
	return p.stderrBuf.String()
}

// ContextLimitHit reports whether a context limit error was detected in the output.
func (p *Process) ContextLimitHit() bool {
	return p.contextLimitHit
}
