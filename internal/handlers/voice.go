package handlers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	"github.com/user/gsd-tele-go/internal/audit"
	"github.com/user/gsd-tele-go/internal/claude"
	"github.com/user/gsd-tele-go/internal/config"
	"github.com/user/gsd-tele-go/internal/project"
	"github.com/user/gsd-tele-go/internal/security"
	"github.com/user/gsd-tele-go/internal/session"
)

// truncateTranscript truncates s to maxLen characters, appending "..." if truncated.
func truncateTranscript(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// HandleVoice processes a Telegram voice message:
// 1. Check OpenAI API key is configured
// 2. Check channel has project mapping
// 3. Download OGG file from Telegram
// 4. Transcribe via OpenAI Whisper API
// 5. Show transcript to user
// 6. Send transcript as text to Claude session
//
// Follows the same mapping-check -> worker-start -> enqueue pattern as HandleText.
func HandleVoice(
	tgBot *gotgbot.Bot,
	ctx *ext.Context,
	store *session.SessionStore,
	cfg *config.Config,
	auditLog *audit.Logger,
	persist *session.PersistenceManager,
	wg *sync.WaitGroup,
	mappings *project.MappingStore,
	globalLimiter *rate.Limiter,
) error {
	if ctx.EffectiveMessage == nil {
		return nil
	}

	chatID := ctx.EffectiveChat.Id

	var userID int64
	if ctx.EffectiveSender != nil {
		userID = ctx.EffectiveSender.Id()
	}

	// Guard: OpenAI API key must be configured for voice transcription.
	if cfg.OpenAIAPIKey == "" {
		_, err := tgBot.SendMessage(chatID, "Voice transcription not configured. Set OPENAI_API_KEY.", nil)
		return err
	}

	// Mapping check (same as HandleText).
	mapping, hasMapped := mappings.Get(chatID)
	if !hasMapped {
		_, err := tgBot.SendMessage(chatID, "No project linked. Send /project to link one.", nil)
		return err
	}

	// Extract voice message.
	voice := ctx.EffectiveMessage.Voice
	if voice == nil {
		return nil
	}

	// Download OGG file from Telegram.
	voicePath, err := downloadToTemp(tgBot, voice.FileId, ".ogg")
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", chatID).Msg("Failed to download voice message")
		_, _ = tgBot.SendMessage(chatID, "Failed to download voice message.", nil)
		return nil
	}
	defer os.Remove(voicePath)

	// Send "Transcribing..." status message.
	statusMsg, _ := tgBot.SendMessage(chatID, "Transcribing...", &gotgbot.SendMessageOpts{
		DisableNotification: true,
	})

	// Transcribe via OpenAI Whisper API with 60-second timeout.
	transcribeCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	transcript, err := transcribeVoice(transcribeCtx, cfg.OpenAIAPIKey, voicePath)
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", chatID).Msg("Voice transcription failed")
		errMsg := truncateTranscript(err.Error(), 200)
		if statusMsg != nil {
			_, _, _ = tgBot.EditMessageText(fmt.Sprintf("Transcription failed: %s", errMsg),
				&gotgbot.EditMessageTextOpts{ChatId: chatID, MessageId: statusMsg.MessageId})
		}
		return nil
	}

	// Show transcript to user.
	displayTranscript := truncateTranscript(transcript, 200)
	if statusMsg != nil {
		_, _, _ = tgBot.EditMessageText(fmt.Sprintf("Transcribed: \"%s\"", displayTranscript),
			&gotgbot.EditMessageTextOpts{ChatId: chatID, MessageId: statusMsg.MessageId})
	}

	// Audit log the voice message.
	if auditLog != nil {
		ev := audit.NewEvent("voice_message", userID, chatID)
		ev.Message = truncateTranscript(transcript, 100)
		_ = auditLog.Log(ev)
	}

	// Command safety check on transcript before sending to Claude.
	safe, blockedPattern := security.CheckCommandSafety(transcript, config.BlockedPatterns)
	if !safe {
		log.Warn().
			Int64("chat_id", chatID).
			Int64("user_id", userID).
			Str("pattern", blockedPattern).
			Msg("Blocked voice transcript due to safety pattern")
		_, err := tgBot.SendMessage(chatID, "Voice transcript blocked for safety: "+blockedPattern, nil)
		return err
	}

	// --- Enqueue to Claude session (same pattern as HandleText) ---
	sess := store.GetOrCreate(chatID, mapping.Path)

	capturedTranscript := transcript
	capturedChatID := chatID
	capturedUserID := userID
	capturedMapping := mapping

	workerCfg := session.WorkerConfig{
		AllowedPaths: []string{mapping.Path},
		SafetyPrompt: config.BuildSafetyPrompt([]string{mapping.Path}),
		FilteredEnv:  config.FilteredEnv(),
		OnQueryComplete: func(sessionID string) {
			title := capturedTranscript
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
				log.Warn().Err(err).Msg("Failed to persist session")
			}
		},
	}

	// Start the worker goroutine exactly once per session.
	if !sess.WorkerStarted() {
		sess.SetWorkerStarted()
		wg.Add(1)
		go func(s *session.Session, c session.WorkerConfig) {
			defer wg.Done()
			s.Worker(context.Background(), cfg.ClaudeCLIPath, c)
		}(sess, workerCfg)
	}

	// Create streaming state and callback.
	ss := NewStreamingState(tgBot, chatID, globalLimiter)

	// Start typing indicator.
	typingCtl := StartTypingIndicator(tgBot, chatID)

	// Build the queued message with a callback factory.
	qMsg := session.QueuedMessage{
		Text:   transcript,
		ChatID: chatID,
		UserID: userID,
		Callback: func(_ int64) claude.StatusCallback {
			typingCtl.Stop()
			return CreateStatusCallback(ss)
		},
		ErrCh: make(chan error, 1),
	}

	// Enqueue the message (non-blocking).
	if !sess.Enqueue(qMsg) {
		_, err := tgBot.SendMessage(chatID,
			"Queue full, please wait for the current query to finish.", nil)
		typingCtl.Stop()
		return err
	}

	// Process the error channel asynchronously.
	go func() {
		err, ok := <-qMsg.ErrCh
		if !ok || err == nil {
			// Success -- check for response buttons.
			fullText := ss.AccumulatedText()
			if fullText != "" {
				maybeAttachActionKeyboard(tgBot, chatID, fullText)
			}
			return
		}

		errStr := err.Error()

		// Context limit: session was auto-cleared, notify user.
		if strings.Contains(strings.ToLower(errStr), "context limit") {
			_, sendErr := tgBot.SendMessage(chatID,
				"Session hit hard context limit and was cleared. Use /resume to restore a previous session.",
				nil)
			if sendErr != nil {
				log.Warn().Err(sendErr).Msg("Failed to send context limit message")
			}
			return
		}

		// General Claude error: truncate stderr to 200 chars.
		truncated := errStr
		if len(truncated) > 200 {
			truncated = truncated[:200]
		}
		_, sendErr := tgBot.SendMessage(chatID, "Claude error: "+truncated, nil)
		if sendErr != nil {
			log.Warn().Err(sendErr).Str("original_error", errStr).Msg("Failed to send error message")
		}

		// Log the error.
		if auditLog != nil {
			ev := audit.NewEvent("claude_error", capturedUserID, capturedChatID)
			ev.Error = truncated
			ev.Message = capturedTranscript
			_ = auditLog.Log(ev)
		}
	}()

	return nil
}
