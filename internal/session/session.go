// Package session provides per-channel session management for the gsd-tele-go bot.
//
// Each channel gets its own independent Session with a worker goroutine that
// processes messages serially from a buffered queue. Sessions can be stopped
// mid-query using the Stop method, which cancels the underlying claude.Process.
package session

import (
	"context"
	"sync"
	"time"

	"github.com/user/gsd-tele-go/internal/claude"
	"github.com/user/gsd-tele-go/internal/config"
)

// SessionState represents the current state of a session.
type SessionState int

const (
	// StateIdle means the session is waiting for a message.
	StateIdle SessionState = iota
	// StateRunning means the session is currently processing a Claude query.
	StateRunning
	// StateStopping means a stop was requested and the session is shutting down.
	StateStopping
)

// StatusCallbackFactory creates a StatusCallback for a given chat ID.
// It is called by the worker goroutine immediately before sending a message to Claude,
// so the callback can reference the correct Telegram message for live updates.
type StatusCallbackFactory func(chatID int64) claude.StatusCallback

// QueuedMessage is a single message waiting to be processed by a Session worker.
type QueuedMessage struct {
	// Text is the raw message text to send to Claude.
	Text string

	// ChatID is the Telegram chat ID — used to create the status callback.
	ChatID int64

	// UserID is the Telegram user ID (for audit logging).
	UserID int64

	// Callback creates a StatusCallback for streaming updates to Telegram.
	Callback StatusCallbackFactory

	// ErrCh receives the result when the query completes (nil on success).
	ErrCh chan error
}

// WorkerConfig carries per-worker dependencies injected by the bot layer.
// Kept as a struct so new fields can be added without changing Worker's signature.
type WorkerConfig struct {
	// AllowedPaths is the list of directories Claude is permitted to access.
	AllowedPaths []string

	// SafetyPrompt is appended to the system prompt for every query.
	SafetyPrompt string

	// FilteredEnv is os.Environ() with CLAUDECODE= removed, passed to claude.NewProcess.
	FilteredEnv []string

	// OnQueryComplete is called with the new sessionID after each successful query.
	// Used by the persistence layer to save session state to disk.
	OnQueryComplete func(sessionID string)
}

// Session owns the Claude session lifecycle for a single Telegram channel.
//
// Messages are enqueued via Enqueue and processed serially by the Worker goroutine.
// All mutable fields are protected by mu; workingDir is immutable after construction.
type Session struct {
	mu sync.Mutex

	// sessionID is the Claude CLI session identifier (--resume flag value).
	// Empty string means a new session will be started on the next query.
	sessionID string

	// workingDir is the Claude working directory; set at construction, never changed.
	workingDir string

	// state is the current processing state.
	state SessionState

	// queue is a buffered channel of incoming messages.
	// Capacity is config.SessionQueueSize (5).
	queue chan QueuedMessage

	// stopCh receives a signal when Stop() is called.
	stopCh chan struct{}

	// cancelQuery cancels the context passed to claude.NewProcess.
	// nil when no query is running.
	cancelQuery context.CancelFunc

	// lastUsage is the token usage from the most recent result event.
	lastUsage *claude.UsageData

	// contextPercent is the context window utilisation (0–100) from the last result.
	contextPercent *int

	// queryStarted is the time the current (or last) query was dispatched.
	queryStarted *time.Time

	// lastActivity is the time of the last completed query.
	lastActivity time.Time

	// lastError is the error message from the most recent failed query (empty if none).
	lastError string

	// currentTool is the name of the tool currently executing (empty if none).
	currentTool string

	// startedAt is when this Session struct was created.
	startedAt time.Time

	// interruptPending is set by MarkInterrupt() so the worker re-processes after stop.
	interruptPending bool

	// workerStarted is true once a Worker goroutine has been launched for this session.
	// Prevents double-start (Pitfall 3 from RESEARCH.md).
	workerStarted bool
}

// NewSession creates a new idle Session for the given working directory.
// The Worker goroutine is NOT started here — the caller must call Worker in a goroutine
// after injecting the claude path and config.
func NewSession(workingDir string) *Session {
	return &Session{
		workingDir: workingDir,
		state:      StateIdle,
		queue:      make(chan QueuedMessage, config.SessionQueueSize),
		stopCh:     make(chan struct{}, 1),
		startedAt:  time.Now(),
	}
}

// Enqueue adds a message to the session's work queue.
// Returns false if the queue is full (capacity config.SessionQueueSize).
// This is a non-blocking send — the caller should handle the false case gracefully.
func (s *Session) Enqueue(msg QueuedMessage) bool {
	select {
	case s.queue <- msg:
		return true
	default:
		return false
	}
}

// Stop signals the worker to stop the currently running query.
// Non-blocking: if the stop channel is already signaled, this is a no-op.
// Also cancels the in-flight query context if one is active.
func (s *Session) Stop() {
	select {
	case s.stopCh <- struct{}{}:
	default:
	}

	s.mu.Lock()
	cancel := s.cancelQuery
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// MarkInterrupt sets the interrupt flag so the Worker knows the stop was caused
// by a new message (! prefix), not a user-initiated /stop command.
func (s *Session) MarkInterrupt() {
	s.mu.Lock()
	s.interruptPending = true
	s.mu.Unlock()
}

// IsRunning reports whether the session is currently processing a query.
func (s *Session) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == StateRunning
}

// SessionID returns the current Claude session ID (empty if no session started yet).
func (s *Session) SessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

// SetSessionID sets the Claude session ID (used when resuming a persisted session).
func (s *Session) SetSessionID(id string) {
	s.mu.Lock()
	s.sessionID = id
	s.mu.Unlock()
}

// WorkingDir returns the working directory. Immutable after construction.
func (s *Session) WorkingDir() string {
	return s.workingDir
}

// LastUsage returns a copy of the most recent token usage data, or nil if none.
func (s *Session) LastUsage() *claude.UsageData {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastUsage == nil {
		return nil
	}
	copy := *s.lastUsage
	return &copy
}

// ContextPercent returns a copy of the most recent context window utilisation, or nil.
func (s *Session) ContextPercent() *int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.contextPercent == nil {
		return nil
	}
	copy := *s.contextPercent
	return &copy
}

// QueryStarted returns a copy of the query start time, or nil if not running.
func (s *Session) QueryStarted() *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queryStarted == nil {
		return nil
	}
	copy := *s.queryStarted
	return &copy
}

// CurrentTool returns the name of the tool currently executing, or empty string.
func (s *Session) CurrentTool() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentTool
}

// SetCurrentTool sets the name of the tool currently executing.
func (s *Session) SetCurrentTool(name string) {
	s.mu.Lock()
	s.currentTool = name
	s.mu.Unlock()
}

// LastError returns the error message from the most recent failed query.
func (s *Session) LastError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastError
}

// StartedAt returns the time this Session was created.
func (s *Session) StartedAt() time.Time {
	return s.startedAt
}

// WorkerStarted reports whether a Worker goroutine has been started.
func (s *Session) WorkerStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workerStarted
}

// SetWorkerStarted marks that a Worker goroutine has been started.
func (s *Session) SetWorkerStarted() {
	s.mu.Lock()
	s.workerStarted = true
	s.mu.Unlock()
}

// Worker is the session's main processing loop. It must be called in a goroutine.
//
// The worker reads messages from s.queue and dispatches each to the Claude CLI via
// claude.NewProcess + Process.Stream. It runs until ctx is cancelled (bot shutdown).
//
// claudePath is the resolved path to the claude CLI binary (from config.ClaudeCLIPath).
func (s *Session) Worker(ctx context.Context, claudePath string, cfg WorkerConfig) {
	for {
		// Drain the stop channel before waiting for the next message, so a Stop()
		// call that arrived while idle doesn't cause the next message to be skipped.
		select {
		case <-s.stopCh:
		default:
		}

		select {
		case <-ctx.Done():
			// Bot is shutting down — drain the queue with errors.
			s.drainQueueWithError(ctx.Err())
			return

		case msg := <-s.queue:
			s.processMessage(ctx, claudePath, cfg, msg)
		}
	}
}

// processMessage executes a single queued message against the Claude CLI.
func (s *Session) processMessage(ctx context.Context, claudePath string, cfg WorkerConfig, msg QueuedMessage) {
	// Mark state as running.
	now := time.Now()
	s.mu.Lock()
	s.state = StateRunning
	s.queryStarted = &now
	s.interruptPending = false
	currentSessionID := s.sessionID
	s.mu.Unlock()

	// Create a cancellable child context so Stop() can abort the query.
	queryCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancelQuery = cancel
	s.mu.Unlock()

	// Build CLI args.
	args := claude.BuildArgs(currentSessionID, cfg.AllowedPaths, "", cfg.SafetyPrompt)

	// Spawn the Claude subprocess.
	proc, err := claude.NewProcess(queryCtx, claudePath, s.workingDir, msg.Text, args, cfg.FilteredEnv)
	if err != nil {
		cancel()
		s.mu.Lock()
		s.state = StateIdle
		s.queryStarted = nil
		s.cancelQuery = nil
		s.lastError = err.Error()
		s.mu.Unlock()
		if msg.ErrCh != nil {
			msg.ErrCh <- err
		}
		return
	}

	// Create status callback using the factory.
	var cb claude.StatusCallback
	if msg.Callback != nil {
		cb = msg.Callback(msg.ChatID)
	} else {
		cb = func(_ claude.ClaudeEvent) error { return nil }
	}

	// Stream events from the process.
	streamErr := proc.Stream(queryCtx, cb)

	// Capture the session ID from the completed process.
	newSessionID := proc.SessionID()

	cancel()

	s.mu.Lock()
	s.state = StateIdle
	s.queryStarted = nil
	s.cancelQuery = nil
	s.currentTool = ""

	if streamErr == claude.ErrContextLimit {
		// Context limit: clear session so next message starts fresh.
		s.sessionID = ""
		s.lastError = "Context limit reached — session cleared. Send a new message to start fresh."
	} else if streamErr != nil && ctx.Err() == nil {
		// Real error (not cancellation from Stop or shutdown).
		s.lastError = streamErr.Error()
	} else {
		// Success: update session ID and clear any previous error.
		if newSessionID != "" {
			s.sessionID = newSessionID
		}
		s.lastError = ""

		// Capture usage from the last result event.
		// (Callback layer may have already set these via SetCurrentTool etc.,
		//  but we also read directly from the process for completeness.)
	}
	s.mu.Unlock()

	// Fire persistence callback on success.
	if streamErr == nil && newSessionID != "" && cfg.OnQueryComplete != nil {
		cfg.OnQueryComplete(newSessionID)
	}

	// Notify caller.
	if msg.ErrCh != nil {
		msg.ErrCh <- streamErr
	}
}

// drainQueueWithError sends the given error to all queued messages and returns.
func (s *Session) drainQueueWithError(err error) {
	for {
		select {
		case msg := <-s.queue:
			if msg.ErrCh != nil {
				msg.ErrCh <- err
			}
		default:
			return
		}
	}
}
