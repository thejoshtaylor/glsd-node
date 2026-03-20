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
	"github.com/user/gsd-tele-go/internal/session"
)

// buildSinglePhotoPrompt constructs the prompt text for a single photo.
func buildSinglePhotoPrompt(path string, caption string) string {
	prompt := fmt.Sprintf("[Photo: %s]", path)
	if caption != "" {
		prompt += "\n\n" + caption
	}
	return prompt
}

// buildAlbumPrompt constructs the prompt text for a photo album.
func buildAlbumPrompt(paths []string, caption string) string {
	var sb strings.Builder
	sb.WriteString("[Photos:\n")
	for i, p := range paths {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, p))
	}
	sb.WriteString("]")
	if caption != "" {
		sb.WriteString("\n\n" + caption)
	}
	return sb.String()
}

// photoGroupBuf is the lazily-initialized MediaGroupBuffer for photo albums.
var photoGroupBuf *MediaGroupBuffer
var photoGroupOnce sync.Once

// HandlePhoto processes a Telegram photo message, supporting both single photos
// and albums (media groups).
//
// For single photos: downloads the largest resolution, builds a prompt with the
// file path and optional caption, and enqueues to the Claude session.
//
// For albums: adds the photo to the MediaGroupBuffer which fires after 1 second
// of inactivity, then sends all collected photos as a single batched prompt.
//
// Follows the same mapping-check -> worker-start -> enqueue pattern as HandleText.
func HandlePhoto(
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

	photos := ctx.EffectiveMessage.Photo
	if len(photos) == 0 {
		return nil
	}

	chatID := ctx.EffectiveChat.Id
	var userID int64
	if ctx.EffectiveSender != nil {
		userID = ctx.EffectiveSender.Id()
	}

	// Mapping check (same as HandleText).
	mapping, hasMapped := mappings.Get(chatID)
	if !hasMapped {
		_, err := tgBot.SendMessage(chatID, "No project linked. Send /project to link one.", nil)
		return err
	}

	// Select largest photo (last in array, sorted ascending by size).
	largest := photos[len(photos)-1]

	// Download photo from Telegram.
	photoPath, err := downloadToTemp(tgBot, largest.FileId, ".jpg")
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", chatID).Msg("Failed to download photo")
		_, _ = tgBot.SendMessage(chatID, "Failed to download photo.", nil)
		return nil
	}

	caption := ctx.EffectiveMessage.Caption
	groupID := ctx.EffectiveMessage.MediaGroupId

	// Audit log the photo.
	if auditLog != nil {
		ev := audit.NewEvent("photo", userID, chatID)
		if caption != "" {
			ev.Message = truncateTranscript(caption, 100)
		} else {
			ev.Message = "photo"
		}
		_ = auditLog.Log(ev)
	}

	// --- Album handling ---
	if groupID != "" {
		// Initialize photo buffer lazily with process callback.
		photoGroupOnce.Do(func() {
			photoGroupBuf = NewMediaGroupBuffer(time.Second,
				makePhotoAlbumProcessor(tgBot, store, cfg, auditLog, persist, wg, mappings, globalLimiter))
		})
		photoGroupBuf.Add(groupID, photoPath, chatID, userID, caption)
		return nil
	}

	// --- Single photo ---
	promptText := buildSinglePhotoPrompt(photoPath, caption)
	return sendPhotoToSession(tgBot, chatID, userID, promptText, photoPath, store, cfg, auditLog, persist, wg, mapping, globalLimiter)
}

// makePhotoAlbumProcessor returns the callback for the photo album MediaGroupBuffer.
// When the buffer fires (1s after last photo in group), it joins all photo paths
// and routes the combined prompt to the Claude session.
func makePhotoAlbumProcessor(
	tgBot *gotgbot.Bot,
	store *session.SessionStore,
	cfg *config.Config,
	auditLog *audit.Logger,
	persist *session.PersistenceManager,
	wg *sync.WaitGroup,
	mappings *project.MappingStore,
	globalLimiter *rate.Limiter,
) func(chatID int64, userID int64, paths []string, caption string) {
	return func(chatID int64, userID int64, paths []string, caption string) {
		promptText := buildAlbumPrompt(paths, caption)

		mapping, ok := mappings.Get(chatID)
		if !ok {
			log.Warn().Int64("chat_id", chatID).Msg("No mapping for photo album channel")
			// Clean up downloaded files.
			for _, p := range paths {
				os.Remove(p)
			}
			return
		}

		// sendPhotoToSession will handle cleanup of a single path, but for albums
		// we need to clean up all paths after the session processes the message.
		// Pass empty photoPath since cleanup is handled here via the ErrCh goroutine.
		err := sendAlbumToSession(tgBot, chatID, userID, promptText, paths, store, cfg, auditLog, persist, wg, mapping, globalLimiter)
		if err != nil {
			log.Error().Err(err).Int64("chat_id", chatID).Msg("Failed to send photo album to session")
			// Clean up on error.
			for _, p := range paths {
				os.Remove(p)
			}
		}
	}
}

// sendPhotoToSession enqueues a single photo prompt to the channel's Claude session.
// Cleans up photoPath in the async ErrCh goroutine after the session processes it.
func sendPhotoToSession(
	tgBot *gotgbot.Bot,
	chatID int64,
	userID int64,
	promptText string,
	photoPath string,
	store *session.SessionStore,
	cfg *config.Config,
	auditLog *audit.Logger,
	persist *session.PersistenceManager,
	wg *sync.WaitGroup,
	mapping project.ProjectMapping,
	globalLimiter *rate.Limiter,
) error {
	sess := store.GetOrCreate(chatID, mapping.Path)

	workerCfg := session.WorkerConfig{
		AllowedPaths: []string{mapping.Path},
		SafetyPrompt: config.BuildSafetyPrompt([]string{mapping.Path}),
		FilteredEnv:  config.FilteredEnv(),
		OnQueryComplete: func(sessionID string) {
			title := promptText
			if len(title) > 50 {
				title = title[:50]
			}
			saved := session.SavedSession{
				SessionID:  sessionID,
				SavedAt:    time.Now().UTC().Format(time.RFC3339),
				WorkingDir: mapping.Path,
				Title:      title,
				ChannelID:  chatID,
			}
			if err := persist.Save(saved); err != nil {
				log.Warn().Err(err).Msg("Failed to persist session")
			}
		},
	}

	if !sess.WorkerStarted() {
		sess.SetWorkerStarted()
		wg.Add(1)
		go func(s *session.Session, c session.WorkerConfig) {
			defer wg.Done()
			s.Worker(context.Background(), cfg.ClaudeCLIPath, c)
		}(sess, workerCfg)
	}

	ss := NewStreamingState(tgBot, chatID, globalLimiter)
	typingCtl := StartTypingIndicator(tgBot, chatID)

	qMsg := session.QueuedMessage{
		Text:   promptText,
		ChatID: chatID,
		UserID: userID,
		Callback: func(_ int64) claude.StatusCallback {
			typingCtl.Stop()
			return CreateStatusCallback(ss)
		},
		ErrCh: make(chan error, 1),
	}

	if !sess.Enqueue(qMsg) {
		typingCtl.Stop()
		os.Remove(photoPath)
		_, _ = tgBot.SendMessage(chatID,
			"Queue full, please wait for the current query to finish.", nil)
		return nil
	}

	// Drain error channel asynchronously; clean up photo file when done.
	go func() {
		defer os.Remove(photoPath)

		err, ok := <-qMsg.ErrCh
		if !ok || err == nil {
			fullText := ss.AccumulatedText()
			if fullText != "" {
				maybeAttachActionKeyboard(tgBot, chatID, fullText)
			}
			return
		}

		errStr := err.Error()
		if strings.Contains(strings.ToLower(errStr), "context limit") {
			_, _ = tgBot.SendMessage(chatID,
				"Session hit hard context limit and was cleared. Use /resume to restore a previous session.", nil)
			return
		}

		truncated := errStr
		if len(truncated) > 200 {
			truncated = truncated[:200]
		}
		_, _ = tgBot.SendMessage(chatID, "Claude error: "+truncated, nil)

		if auditLog != nil {
			ev := audit.NewEvent("claude_error", userID, chatID)
			ev.Error = truncated
			_ = auditLog.Log(ev)
		}
	}()

	return nil
}

// sendAlbumToSession enqueues an album prompt to the channel's Claude session.
// Cleans up all photo paths in the async ErrCh goroutine after processing.
func sendAlbumToSession(
	tgBot *gotgbot.Bot,
	chatID int64,
	userID int64,
	promptText string,
	paths []string,
	store *session.SessionStore,
	cfg *config.Config,
	auditLog *audit.Logger,
	persist *session.PersistenceManager,
	wg *sync.WaitGroup,
	mapping project.ProjectMapping,
	globalLimiter *rate.Limiter,
) error {
	sess := store.GetOrCreate(chatID, mapping.Path)

	workerCfg := session.WorkerConfig{
		AllowedPaths: []string{mapping.Path},
		SafetyPrompt: config.BuildSafetyPrompt([]string{mapping.Path}),
		FilteredEnv:  config.FilteredEnv(),
		OnQueryComplete: func(sessionID string) {
			title := promptText
			if len(title) > 50 {
				title = title[:50]
			}
			saved := session.SavedSession{
				SessionID:  sessionID,
				SavedAt:    time.Now().UTC().Format(time.RFC3339),
				WorkingDir: mapping.Path,
				Title:      title,
				ChannelID:  chatID,
			}
			if err := persist.Save(saved); err != nil {
				log.Warn().Err(err).Msg("Failed to persist session")
			}
		},
	}

	if !sess.WorkerStarted() {
		sess.SetWorkerStarted()
		wg.Add(1)
		go func(s *session.Session, c session.WorkerConfig) {
			defer wg.Done()
			s.Worker(context.Background(), cfg.ClaudeCLIPath, c)
		}(sess, workerCfg)
	}

	ss := NewStreamingState(tgBot, chatID, globalLimiter)
	typingCtl := StartTypingIndicator(tgBot, chatID)

	qMsg := session.QueuedMessage{
		Text:   promptText,
		ChatID: chatID,
		UserID: userID,
		Callback: func(_ int64) claude.StatusCallback {
			typingCtl.Stop()
			return CreateStatusCallback(ss)
		},
		ErrCh: make(chan error, 1),
	}

	if !sess.Enqueue(qMsg) {
		typingCtl.Stop()
		for _, p := range paths {
			os.Remove(p)
		}
		_, _ = tgBot.SendMessage(chatID,
			"Queue full, please wait for the current query to finish.", nil)
		return nil
	}

	// Drain error channel asynchronously; clean up all album files when done.
	go func() {
		defer func() {
			for _, p := range paths {
				os.Remove(p)
			}
		}()

		err, ok := <-qMsg.ErrCh
		if !ok || err == nil {
			fullText := ss.AccumulatedText()
			if fullText != "" {
				maybeAttachActionKeyboard(tgBot, chatID, fullText)
			}
			return
		}

		errStr := err.Error()
		if strings.Contains(strings.ToLower(errStr), "context limit") {
			_, _ = tgBot.SendMessage(chatID,
				"Session hit hard context limit and was cleared. Use /resume to restore a previous session.", nil)
			return
		}

		truncated := errStr
		if len(truncated) > 200 {
			truncated = truncated[:200]
		}
		_, _ = tgBot.SendMessage(chatID, "Claude error: "+truncated, nil)

		if auditLog != nil {
			ev := audit.NewEvent("claude_error", userID, chatID)
			ev.Error = truncated
			_ = auditLog.Log(ev)
		}
	}()

	return nil
}
