package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	"github.com/user/gsd-tele-go/internal/claude"
	"github.com/user/gsd-tele-go/internal/config"
	"github.com/user/gsd-tele-go/internal/project"
	"github.com/user/gsd-tele-go/internal/session"
)

// callbackAction is an enum for the action to take after parsing callback data.
// Using a pure enum lets us test routing logic without gotgbot types.
type callbackAction int

const (
	callbackActionUnknown       callbackAction = iota
	callbackActionResume                       // data = "resume:<session_id>"
	callbackActionStop                         // data = "action:stop"
	callbackActionNew                          // data = "action:new"
	callbackActionRetry                        // data = "action:retry"
	callbackActionGsd                          // data = "gsd:{operation_key}"
	callbackActionGsdRun                       // data = "gsd-run:{command}"
	callbackActionGsdFresh                     // data = "gsd-fresh:{command}"
	callbackActionGsdPhase                     // data = "gsd-exec:{N}", "gsd-plan:{N}", etc.
	callbackActionOption                       // data = "option:{key}"
	callbackActionAskUser                      // data = "askuser:{request_id}:{option_index}"
	callbackActionProjectChange                // data = "project:change"
	callbackActionProjectUnlink                // data = "project:unlink"
)

// parseCallbackData parses the callback data string and returns the action and
// optional payload (e.g. session ID for resume).
// Pure function — no Telegram types — fully testable.
//
// IMPORTANT: gsd-run:, gsd-fresh:, gsd-exec:, gsd-plan:, gsd-discuss:, gsd-research:,
// gsd-verify:, gsd-remove: are all checked BEFORE "gsd:" to prevent the shorter prefix
// from matching first.
func parseCallbackData(data string) (callbackAction, string) {
	switch {
	case strings.HasPrefix(data, "resume:"):
		return callbackActionResume, strings.TrimPrefix(data, "resume:")
	case data == "action:stop":
		return callbackActionStop, ""
	case data == "action:new":
		return callbackActionNew, ""
	case data == "action:retry":
		return callbackActionRetry, ""

	// GSD sub-prefixes MUST come before "gsd:" to avoid premature match.
	case strings.HasPrefix(data, "gsd-run:"):
		return callbackActionGsdRun, strings.TrimPrefix(data, "gsd-run:")
	case strings.HasPrefix(data, "gsd-fresh:"):
		return callbackActionGsdFresh, strings.TrimPrefix(data, "gsd-fresh:")
	case strings.HasPrefix(data, "gsd-exec:"),
		strings.HasPrefix(data, "gsd-plan:"),
		strings.HasPrefix(data, "gsd-discuss:"),
		strings.HasPrefix(data, "gsd-research:"),
		strings.HasPrefix(data, "gsd-verify:"),
		strings.HasPrefix(data, "gsd-remove:"):
		return callbackActionGsdPhase, data // keep full data for prefix+number parsing

	case strings.HasPrefix(data, "gsd:"):
		return callbackActionGsd, strings.TrimPrefix(data, "gsd:")

	case strings.HasPrefix(data, "option:"):
		return callbackActionOption, strings.TrimPrefix(data, "option:")
	case strings.HasPrefix(data, "askuser:"):
		return callbackActionAskUser, strings.TrimPrefix(data, "askuser:")
	case data == "project:change":
		return callbackActionProjectChange, ""
	case data == "project:unlink":
		return callbackActionProjectUnlink, ""

	default:
		return callbackActionUnknown, ""
	}
}

// HandleCallback handles all inline keyboard callback queries.
//
// Routes by callback data prefix:
//   - "resume:<session_id>"     — restore a persisted Claude session
//   - "action:stop"             — stop the current running query
//   - "action:new"              — start a fresh session
//   - "action:retry"            — placeholder (deferred to v2)
//   - "gsd:{key}"               — run or pick a GSD operation
//   - "gsd-run:{cmd}"           — run a GSD command in current session
//   - "gsd-fresh:{cmd}"         — run a GSD command in a new session
//   - "gsd-exec:{N}" etc.       — run a phase-specific GSD command
//   - "option:{key}"            — send a numbered/lettered option to Claude
//   - "askuser:{id}:{idx}"      — respond to an ask_user MCP request
//   - "project:change"          — prompt user for new project path
//   - "project:unlink"          — unlink current project
func HandleCallback(b *gotgbot.Bot, ctx *ext.Context, store *session.SessionStore,
	persist *session.PersistenceManager, cfg *config.Config,
	mappings *project.MappingStore, awaitingPath *AwaitingPathState,
	wg *sync.WaitGroup, globalLimiter *rate.Limiter) error {

	if ctx.CallbackQuery == nil {
		return nil
	}

	cq := ctx.CallbackQuery

	// Answer the callback query immediately to remove the loading spinner.
	if _, err := cq.Answer(b, nil); err != nil {
		// Non-fatal: the user experience degrades slightly but we continue.
		_ = err
	}

	// Determine the chat ID from the message attached to the callback query.
	var chatID int64
	var msgID int64
	if cq.Message != nil {
		chat := cq.Message.GetChat()
		chatID = chat.Id
		msgID = cq.Message.GetMessageId()
	}

	if chatID == 0 {
		return nil
	}

	action, payload := parseCallbackData(cq.Data)

	switch action {
	case callbackActionResume:
		return handleCallbackResume(b, ctx, store, cfg, mappings, chatID, msgID, payload)

	case callbackActionStop:
		return handleCallbackStop(b, store, chatID, msgID)

	case callbackActionNew:
		return handleCallbackNew(b, store, cfg, mappings, chatID, msgID)

	case callbackActionRetry:
		// Deferred to v2.
		_, err := b.SendMessage(chatID, "Retry not available yet.", nil)
		return err

	case callbackActionGsd:
		return handleCallbackGsd(b, chatID, msgID, payload, store, mappings, cfg, persist, wg, globalLimiter)

	case callbackActionGsdRun:
		if msgID != 0 {
			_, _, _ = b.EditMessageText("Running: "+payload+"...", &gotgbot.EditMessageTextOpts{
				ChatId:    chatID,
				MessageId: msgID,
			})
		}
		return enqueueGsdCommand(b, chatID, payload, store, mappings, cfg, persist, wg, globalLimiter)

	case callbackActionGsdFresh:
		// Fresh session: clear session ID before enqueueing.
		if sess, ok := store.Get(chatID); ok {
			sess.SetSessionID("")
		}
		if msgID != 0 {
			_, _, _ = b.EditMessageText("Fresh session: "+payload+"...", &gotgbot.EditMessageTextOpts{
				ChatId:    chatID,
				MessageId: msgID,
			})
		}
		return enqueueGsdCommand(b, chatID, payload, store, mappings, cfg, persist, wg, globalLimiter)

	case callbackActionGsdPhase:
		return handleCallbackGsdPhase(b, chatID, msgID, payload, store, mappings, cfg, persist, wg, globalLimiter)

	case callbackActionOption:
		// Send the option key (e.g. "1", "A") directly to Claude.
		if msgID != 0 {
			_, _, _ = b.EditMessageText("Selected: "+payload, &gotgbot.EditMessageTextOpts{
				ChatId:    chatID,
				MessageId: msgID,
			})
		}
		return enqueueGsdCommand(b, chatID, payload, store, mappings, cfg, persist, wg, globalLimiter)

	case callbackActionAskUser:
		return handleCallbackAskUser(b, chatID, msgID, payload, store, mappings, cfg, persist, wg, globalLimiter)

	case callbackActionProjectChange:
		awaitingPath.Set(chatID)
		_, err := b.SendMessage(chatID, "Send the new project path:", nil)
		return err

	case callbackActionProjectUnlink:
		_ = mappings.Remove(chatID)
		// Stop the session if running.
		if sess, ok := store.Get(chatID); ok && sess.IsRunning() {
			sess.Stop()
		}
		if msgID != 0 {
			_, _, _ = b.EditMessageText("Project unlinked.", &gotgbot.EditMessageTextOpts{
				ChatId:    chatID,
				MessageId: msgID,
			})
		} else {
			_, _ = b.SendMessage(chatID, "Project unlinked.", nil)
		}
		return nil

	default:
		// Unknown callback data — log and ignore gracefully.
		return nil
	}
}

// handleCallbackGsd routes a "gsd:{key}" callback.
// If the key is in PhasePickerOps, show the phase picker keyboard.
// Otherwise, send the GSD command to Claude.
func handleCallbackGsd(b *gotgbot.Bot, chatID, msgID int64, key string,
	store *session.SessionStore, mappings *project.MappingStore,
	cfg *config.Config, persist *session.PersistenceManager,
	wg *sync.WaitGroup, globalLimiter *rate.Limiter) error {

	// Check if this operation needs a phase picker first.
	if prefix, ok := PhasePickerOps[key]; ok {
		mapping, hasMapped := mappings.Get(chatID)
		if !hasMapped {
			_, err := b.SendMessage(chatID, "No project linked. Use /project to link one first.", nil)
			return err
		}
		phases := ParseRoadmap(mapping.Path)
		keyboard := BuildPhasePickerKeyboard(phases, prefix)
		_, err := b.SendMessage(chatID, "Select a phase:", &gotgbot.SendMessageOpts{
			ReplyMarkup: keyboard,
		})
		return err
	}

	// Look up the operation command.
	op, ok := gsdOpIndex[key]
	if !ok {
		_, err := b.SendMessage(chatID, "Unknown GSD operation: "+key, nil)
		return err
	}

	if msgID != 0 {
		_, _, _ = b.EditMessageText("Running: "+op.Command+"...", &gotgbot.EditMessageTextOpts{
			ChatId:    chatID,
			MessageId: msgID,
		})
	}
	return enqueueGsdCommand(b, chatID, op.Command, store, mappings, cfg, persist, wg, globalLimiter)
}

// handleCallbackGsdPhase parses a phase-specific callback like "gsd-exec:2"
// and maps it to the full command "/gsd:execute-phase 2".
func handleCallbackGsdPhase(b *gotgbot.Bot, chatID, msgID int64, data string,
	store *session.SessionStore, mappings *project.MappingStore,
	cfg *config.Config, persist *session.PersistenceManager,
	wg *sync.WaitGroup, globalLimiter *rate.Limiter) error {

	// Map callback prefix to GSD command.
	prefixToCmd := map[string]string{
		"gsd-exec":     "/gsd:execute-phase",
		"gsd-plan":     "/gsd:plan-phase",
		"gsd-discuss":  "/gsd:discuss-phase",
		"gsd-research": "/gsd:research-phase",
		"gsd-verify":   "/gsd:verify-work",
		"gsd-remove":   "/gsd:remove-phase",
	}

	// Split "gsd-exec:2" into prefix="gsd-exec" and number="2".
	colonIdx := strings.Index(data, ":")
	if colonIdx < 0 {
		_, err := b.SendMessage(chatID, "Invalid phase callback: "+data, nil)
		return err
	}
	prefix := data[:colonIdx]
	phaseNum := data[colonIdx+1:]

	cmd, ok := prefixToCmd[prefix]
	if !ok {
		_, err := b.SendMessage(chatID, "Unknown phase callback prefix: "+prefix, nil)
		return err
	}

	fullCmd := cmd + " " + phaseNum
	if msgID != 0 {
		_, _, _ = b.EditMessageText("Running: "+fullCmd+"...", &gotgbot.EditMessageTextOpts{
			ChatId:    chatID,
			MessageId: msgID,
		})
	}
	return enqueueGsdCommand(b, chatID, fullCmd, store, mappings, cfg, persist, wg, globalLimiter)
}

// askUserRequest is the JSON structure written by the ask_user MCP tool.
type askUserRequest struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Status   string   `json:"status"`
}

// handleCallbackAskUser handles an "askuser:{request_id}:{option_index}" callback.
// It reads the temp file, validates the selection, edits the message, deletes the file,
// and sends the selected option text to Claude.
func handleCallbackAskUser(b *gotgbot.Bot, chatID, msgID int64, payload string,
	store *session.SessionStore, mappings *project.MappingStore,
	cfg *config.Config, persist *session.PersistenceManager,
	wg *sync.WaitGroup, globalLimiter *rate.Limiter) error {

	// Payload format: "{request_id}:{option_index}"
	colonIdx := strings.Index(payload, ":")
	if colonIdx < 0 {
		_, err := b.SendMessage(chatID, "Invalid askuser payload.", nil)
		return err
	}
	requestID := payload[:colonIdx]
	optionIndexStr := payload[colonIdx+1:]

	// Parse option index.
	optionIndex := 0
	for _, ch := range optionIndexStr {
		if ch < '0' || ch > '9' {
			_, err := b.SendMessage(chatID, "Invalid option index.", nil)
			return err
		}
		optionIndex = optionIndex*10 + int(ch-'0')
	}

	// Read the temp file.
	tmpFile := filepath.Join(os.TempDir(), "ask-user-"+requestID+".json")
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		_, sendErr := b.SendMessage(chatID, "Request expired or not found.", nil)
		return sendErr
	}

	var req askUserRequest
	if err := json.Unmarshal(data, &req); err != nil {
		_, sendErr := b.SendMessage(chatID, "Failed to parse request.", nil)
		return sendErr
	}

	if optionIndex < 0 || optionIndex >= len(req.Options) {
		_, sendErr := b.SendMessage(chatID, "Option index out of range.", nil)
		return sendErr
	}

	selectedText := req.Options[optionIndex]

	// Edit the message to confirm selection.
	if msgID != 0 {
		_, _, _ = b.EditMessageText("Selected: "+selectedText, &gotgbot.EditMessageTextOpts{
			ChatId:    chatID,
			MessageId: msgID,
		})
	}

	// Delete the temp file.
	_ = os.Remove(tmpFile)

	// Send the selected option to Claude.
	return enqueueGsdCommand(b, chatID, selectedText, store, mappings, cfg, persist, wg, globalLimiter)
}

// enqueueGsdCommand looks up the project mapping for the channel, gets or creates
// the session, ensures the worker is started, and enqueues the message with a
// streaming callback.
//
// This is used by all GSD/option/askuser callbacks to send text to Claude.
func enqueueGsdCommand(b *gotgbot.Bot, chatID int64, text string,
	store *session.SessionStore, mappings *project.MappingStore,
	cfg *config.Config, persist *session.PersistenceManager,
	wg *sync.WaitGroup, globalLimiter *rate.Limiter) error {

	mapping, hasMapped := mappings.Get(chatID)
	if !hasMapped {
		_, err := b.SendMessage(chatID, "No project linked. Use /project to link one first.", nil)
		return err
	}

	sess := store.GetOrCreate(chatID, mapping.Path)

	capturedText := text
	capturedChatID := chatID
	capturedMapping := mapping

	// Start worker if not already started.
	if !sess.WorkerStarted() {
		sess.SetWorkerStarted()
		wg.Add(1)
		go func(s *session.Session) {
			defer wg.Done()
			wCfg := session.WorkerConfig{
				AllowedPaths: []string{mapping.Path},
				SafetyPrompt: config.BuildSafetyPrompt([]string{mapping.Path}),
				FilteredEnv:  config.FilteredEnv(),
				OnQueryComplete: func(sessionID string) {
					title := capturedText
					if len(title) > 50 {
						title = title[:50]
					}
					saved := session.SavedSession{
						SessionID:  sessionID,
						SavedAt:    time.Now().UTC().Format(time.RFC3339),
						WorkingDir: capturedMapping.Path,
						Title:      title,
						ChannelID:  capturedChatID,
					}
					if err := persist.Save(saved); err != nil {
						log.Warn().Err(err).Msg("Failed to persist GSD session")
					}
				},
			}
			s.Worker(context.Background(), cfg.ClaudeCLIPath, wCfg)
		}(sess)
	}

	// Create streaming state and callback using the global API rate limiter.
	ss := NewStreamingState(b, chatID, globalLimiter)

	qMsg := session.QueuedMessage{
		Text:   text,
		ChatID: chatID,
		Callback: func(_ int64) claude.StatusCallback {
			return CreateStatusCallback(ss)
		},
		ErrCh: make(chan error, 1),
	}

	if !sess.Enqueue(qMsg) {
		_, err := b.SendMessage(chatID, "Queue full, please wait for the current query to finish.", nil)
		return err
	}

	// Drain the error channel asynchronously.
	go func() {
		err, ok := <-qMsg.ErrCh
		if !ok || err == nil {
			// Success — check for response buttons.
			fullText := ss.AccumulatedText()
			if fullText != "" {
				maybeAttachActionKeyboard(b, chatID, fullText)
			}
			return
		}
		truncated := err.Error()
		if len(truncated) > 200 {
			truncated = truncated[:200]
		}
		_, _ = b.SendMessage(chatID, "Claude error: "+truncated, nil)
	}()

	return nil
}

// handleCallbackResume restores a saved session by session ID.
// Updates the inline keyboard message to confirm the restore.
func handleCallbackResume(b *gotgbot.Bot, _ *ext.Context, store *session.SessionStore,
	cfg *config.Config, mappings *project.MappingStore,
	chatID, msgID int64, sessionID string) error {
	if sessionID == "" {
		_, err := b.SendMessage(chatID, "Invalid session ID.", nil)
		return err
	}

	workingDir := cfg.WorkingDir
	if m, ok := mappings.Get(chatID); ok {
		workingDir = m.Path
	}
	if workingDir == "" {
		_, err := b.SendMessage(chatID, "No project linked. Use /project to link one.", nil)
		return err
	}
	sess := store.GetOrCreate(chatID, workingDir)
	sess.SetSessionID(sessionID)

	// Show a short prefix of the session ID for confirmation.
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}

	confirmText := fmt.Sprintf("Session restored: %s... — send a message to continue.", short)

	// Edit the keyboard message to remove the buttons and show the confirmation.
	if msgID != 0 {
		_, _, _ = b.EditMessageText(confirmText, &gotgbot.EditMessageTextOpts{
			ChatId:    chatID,
			MessageId: msgID,
		})
	} else {
		_, _ = b.SendMessage(chatID, confirmText, nil)
	}

	return nil
}

// handleCallbackStop stops the current running query (same as /stop).
func handleCallbackStop(b *gotgbot.Bot, store *session.SessionStore, chatID, msgID int64) error {
	sess, ok := store.Get(chatID)
	if !ok || !sess.IsRunning() {
		_, err := b.SendMessage(chatID, "No query running.", nil)
		return err
	}

	sess.Stop()

	if msgID != 0 {
		_, _, _ = b.EditMessageText("Query stopped.", &gotgbot.EditMessageTextOpts{
			ChatId:    chatID,
			MessageId: msgID,
		})
	} else {
		_, _ = b.SendMessage(chatID, "Query stopped.", nil)
	}

	return nil
}

// handleCallbackNew starts a fresh session (same as /new).
func handleCallbackNew(b *gotgbot.Bot, store *session.SessionStore,
	cfg *config.Config, mappings *project.MappingStore,
	chatID, msgID int64) error {
	workingDir := cfg.WorkingDir
	if m, ok := mappings.Get(chatID); ok {
		workingDir = m.Path
	}
	if workingDir == "" {
		_, err := b.SendMessage(chatID, "No project linked. Use /project to link one.", nil)
		return err
	}
	sess := store.GetOrCreate(chatID, workingDir)

	if sess.IsRunning() {
		sess.Stop()
	}

	sess.SetSessionID("")

	if msgID != 0 {
		_, _, _ = b.EditMessageText("New session started. Previous session saved for /resume.", &gotgbot.EditMessageTextOpts{
			ChatId:    chatID,
			MessageId: msgID,
		})
	} else {
		_, _ = b.SendMessage(chatID, "New session started. Previous session saved for /resume.", nil)
	}

	return nil
}
