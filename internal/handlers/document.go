package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// classifyDocument determines the document type from the filename extension.
// Returns "pdf", "text", or "unsupported".
func classifyDocument(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".pdf" {
		return "pdf"
	}
	if isTextFile(filename) {
		return "text"
	}
	return "unsupported"
}

// supportedExtensionsList returns a sorted, comma-separated string of all
// supported file extensions (PDF + text extensions from helpers.go).
func supportedExtensionsList() string {
	exts := make([]string, 0, len(textExtensions)+1)
	exts = append(exts, ".pdf")
	for ext := range textExtensions {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return strings.Join(exts, ", ")
}

// buildDocumentPrompt constructs the prompt text for a document with optional caption.
func buildDocumentPrompt(filename string, content string, caption string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Document: %s]\n", filename))
	sb.WriteString(content)
	if caption != "" {
		sb.WriteString("\n\n" + caption)
	}
	return sb.String()
}

// truncateText truncates s to maxChars characters, appending "..." if truncated.
func truncateText(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "..."
}

// docGroupBuf is the lazily-initialized MediaGroupBuffer for document albums.
var docGroupBuf *MediaGroupBuffer
var docGroupOnce sync.Once

// HandleDocument processes a document message, extracting content from PDFs and
// text files and routing the result to the channel's Claude session.
//
// Follows the same enqueue pattern as HandleText: mapping check -> GetOrCreate ->
// worker -> StreamingState -> enqueue.
func HandleDocument(
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

	doc := ctx.EffectiveMessage.Document
	if doc == nil {
		return nil
	}

	chatID := ctx.EffectiveChat.Id
	var userID int64
	if ctx.EffectiveSender != nil {
		userID = ctx.EffectiveSender.Id()
	}
	caption := ctx.EffectiveMessage.Caption

	// Audit log the incoming document.
	if auditLog != nil {
		ev := audit.NewEvent("document", userID, chatID)
		ev.Message = doc.FileName
		_ = auditLog.Log(ev)
	}

	// --- Mapping check ---
	mapping, hasMapped := mappings.Get(chatID)
	if !hasMapped {
		_, err := tgBot.SendMessage(chatID,
			"This channel has no linked project. Use /project to link one first.", nil)
		return err
	}

	// --- File size check ---
	if doc.FileSize > int64(maxFileSize) {
		_, err := tgBot.SendMessage(chatID,
			fmt.Sprintf("File too large (%.1f MB). Maximum size is 10 MB.",
				float64(doc.FileSize)/(1024*1024)), nil)
		return err
	}

	// --- Classify document ---
	docType := classifyDocument(doc.FileName)
	if docType == "unsupported" {
		_, err := tgBot.SendMessage(chatID,
			fmt.Sprintf("Unsupported file type. Supported: %s", supportedExtensionsList()), nil)
		return err
	}

	// --- Download file ---
	docPath, err := downloadToTemp(tgBot, doc.FileId, filepath.Ext(doc.FileName))
	if err != nil {
		log.Error().Err(err).Str("file_id", doc.FileId).Msg("Failed to download document")
		_, _ = tgBot.SendMessage(chatID, "Failed to download file.", nil)
		return nil
	}

	// --- Extract content ---
	var content string
	switch docType {
	case "pdf":
		if cfg.PdfToTextPath == "" {
			os.Remove(docPath)
			_, err := tgBot.SendMessage(chatID,
				"PDF extraction not configured. Set PDFTOTEXT_PATH.", nil)
			return err
		}
		pdfCtx, pdfCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer pdfCancel()
		content, err = extractPDF(pdfCtx, cfg.PdfToTextPath, docPath)
		if err != nil {
			os.Remove(docPath)
			errMsg := err.Error()
			if len(errMsg) > 200 {
				errMsg = errMsg[:200]
			}
			_, _ = tgBot.SendMessage(chatID,
				fmt.Sprintf("Failed to extract PDF text: %s", errMsg), nil)
			return nil
		}

	case "text":
		raw, readErr := os.ReadFile(docPath)
		if readErr != nil {
			os.Remove(docPath)
			_, _ = tgBot.SendMessage(chatID, "Failed to read file.", nil)
			return nil
		}
		content = truncateText(string(raw), maxTextChars)
	}

	// --- Check for document album ---
	groupID := ctx.EffectiveMessage.MediaGroupId
	if groupID != "" {
		// Document album: build per-doc snippet, pass to buffer.
		snippet := buildDocumentPrompt(doc.FileName, content, "")
		os.Remove(docPath) // content already extracted

		docGroupOnce.Do(func() {
			docGroupBuf = NewMediaGroupBuffer(1*time.Second,
				makeDocAlbumProcessor(tgBot, store, cfg, auditLog, persist, wg, mappings, globalLimiter))
		})

		docGroupBuf.Add(groupID, snippet, chatID, userID, caption)
		return nil
	}

	// --- Single document ---
	defer os.Remove(docPath)

	promptText := buildDocumentPrompt(doc.FileName, content, caption)
	return sendDocToSession(tgBot, chatID, userID, promptText, store, cfg, auditLog, persist, wg, mapping, globalLimiter)
}

// makeDocAlbumProcessor returns the callback for the document album MediaGroupBuffer.
// When the buffer fires (1s after last document in group), it joins all document
// snippets and routes the combined prompt to the Claude session.
func makeDocAlbumProcessor(
	tgBot *gotgbot.Bot,
	store *session.SessionStore,
	cfg *config.Config,
	auditLog *audit.Logger,
	persist *session.PersistenceManager,
	wg *sync.WaitGroup,
	mappings *project.MappingStore,
	globalLimiter *rate.Limiter,
) func(chatID int64, userID int64, snippets []string, caption string) {
	return func(chatID int64, userID int64, snippets []string, caption string) {
		// snippets are already formatted "[Document: ...]\n..." strings.
		promptText := strings.Join(snippets, "\n\n---\n\n")
		if caption != "" {
			promptText += "\n\n" + caption
		}

		mapping, ok := mappings.Get(chatID)
		if !ok {
			log.Warn().Int64("chat_id", chatID).Msg("No mapping for document album channel")
			return
		}

		if err := sendDocToSession(tgBot, chatID, userID, promptText, store, cfg, auditLog, persist, wg, mapping, globalLimiter); err != nil {
			log.Error().Err(err).Int64("chat_id", chatID).Msg("Failed to send document album to session")
		}
	}
}

// sendDocToSession enqueues the document prompt text to the channel's Claude session.
// Follows the same pattern as HandleText: GetOrCreate -> ensure worker -> StreamingState -> enqueue.
func sendDocToSession(
	tgBot *gotgbot.Bot,
	chatID int64,
	userID int64,
	promptText string,
	store *session.SessionStore,
	cfg *config.Config,
	auditLog *audit.Logger,
	persist *session.PersistenceManager,
	wg *sync.WaitGroup,
	mapping project.ProjectMapping,
	globalLimiter *rate.Limiter,
) error {
	sess := store.GetOrCreate(chatID, mapping.Path)

	// --- Ensure session worker is running ---
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

	// --- Create streaming state and callback ---
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
		_, _ = tgBot.SendMessage(chatID,
			"Queue full, please wait for the current query to finish.", nil)
		return nil
	}

	// Drain error channel asynchronously.
	go func() {
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
