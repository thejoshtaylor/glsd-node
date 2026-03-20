// Package handlers provides Telegram message handlers and the streaming response layer.
package handlers

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	"github.com/user/gsd-tele-go/internal/claude"
	"github.com/user/gsd-tele-go/internal/config"
	"github.com/user/gsd-tele-go/internal/formatting"
)

// TextSegment tracks a single Telegram message that accumulates streaming text.
type TextSegment struct {
	MessageID int64
	Text      string
}

// StreamingState manages throttled edit-in-place Telegram message updates during
// a streaming Claude response.  All fields are protected by mu.
type StreamingState struct {
	bot    *gotgbot.Bot
	chatID int64

	mu            sync.Mutex
	statusMsgID   int64                // ephemeral tool/thinking status message ID (0 if none)
	textSegments  map[int]*TextSegment // segment index -> Telegram message + accumulated text
	lastEditTime  time.Time
	pendingText   string       // text buffered waiting for the 500ms throttle
	pendingTimer  *time.Timer  // fires flush when throttle expires
	nextSegment   int          // index of the current (latest) text segment
	globalLimiter *rate.Limiter // optional; nil means no global limiting
}

// NewStreamingState creates a StreamingState for the given bot and chat.
// globalLimiter is optional — pass nil to disable global API rate limiting.
func NewStreamingState(bot *gotgbot.Bot, chatID int64, globalLimiter *rate.Limiter) *StreamingState {
	return &StreamingState{
		bot:           bot,
		chatID:        chatID,
		textSegments:  make(map[int]*TextSegment),
		globalLimiter: globalLimiter,
	}
}

// AccumulatedText returns the full concatenated text from all segments.
// Called after streaming completes to extract response for button detection.
func (ss *StreamingState) AccumulatedText() string {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if len(ss.textSegments) == 0 {
		return ""
	}

	var sb strings.Builder
	for i := 0; i <= ss.nextSegment; i++ {
		seg, ok := ss.textSegments[i]
		if ok {
			sb.WriteString(seg.Text)
		}
	}
	return sb.String()
}

// SendThinkingMessage sends a "Thinking..." placeholder message and returns its
// message ID.  The caller may delete or replace this once real text arrives.
func (ss *StreamingState) SendThinkingMessage() int64 {
	msg, err := ss.bot.SendMessage(ss.chatID, "Thinking...", &gotgbot.SendMessageOpts{
		DisableNotification: true,
	})
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", ss.chatID).Msg("Failed to send thinking message")
		return 0
	}
	return msg.MessageId
}

// CreateStatusCallback returns a claude.StatusCallback that drives live Telegram
// message updates from the streaming NDJSON events.
//
// Event handling:
//   - "assistant" events with text blocks: throttled edit-in-place (500ms minimum).
//   - "assistant" events with thinking blocks: update status message with "Thinking...".
//   - "assistant" events with tool_use blocks: update status message with tool emoji.
//   - "result" events: flush pending text, delete status message, final segment edit.
func CreateStatusCallback(ss *StreamingState) claude.StatusCallback {
	return func(event claude.ClaudeEvent) error {
		switch event.Type {
		case "assistant":
			if event.Message == nil {
				return nil
			}
			return ss.handleAssistantEvent(event.Message)

		case "result":
			ss.flushPending()
			ss.deleteStatusMessage()
			return nil
		}
		return nil
	}
}

// handleAssistantEvent processes the content blocks of an assistant message event.
func (ss *StreamingState) handleAssistantEvent(msg *claude.AssistantMsg) error {
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			ss.accumulateText(block.Text)

		case "thinking":
			ss.updateStatusMessage("Thinking...")

		case "tool_use":
			status := formatting.FormatToolStatus(block.Name, block.Input)
			ss.updateStatusMessage(status)
		}
	}
	return nil
}

// accumulateText appends text to the current segment and either sends/edits
// immediately (if the 500ms throttle allows) or buffers it.
func (ss *StreamingState) accumulateText(text string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	seg := ss.currentSegmentLocked()
	seg.Text += text

	now := time.Now()
	if now.Sub(ss.lastEditTime) >= config.StreamingThrottleMs*time.Millisecond {
		// Throttle allows — send/edit immediately.
		ss.lastEditTime = now
		// Cancel any pending timer.
		if ss.pendingTimer != nil {
			ss.pendingTimer.Stop()
			ss.pendingTimer = nil
		}
		ss.pendingText = ""
		go ss.editOrSendSegment(ss.nextSegment, seg.Text)
	} else {
		// Buffer and schedule a deferred flush.
		ss.pendingText = seg.Text
		if ss.pendingTimer == nil {
			segIdx := ss.nextSegment
			ss.pendingTimer = time.AfterFunc(config.StreamingThrottleMs*time.Millisecond, func() {
				ss.mu.Lock()
				text := ss.pendingText
				ss.pendingTimer = nil
				ss.pendingText = ""
				ss.lastEditTime = time.Now()
				ss.mu.Unlock()
				if text != "" {
					ss.editOrSendSegment(segIdx, text)
				}
			})
		}
	}
}

// flushPending sends any pending buffered text immediately, ignoring the throttle.
func (ss *StreamingState) flushPending() {
	ss.mu.Lock()
	if ss.pendingTimer != nil {
		ss.pendingTimer.Stop()
		ss.pendingTimer = nil
	}
	text := ss.pendingText
	ss.pendingText = ""
	segIdx := ss.nextSegment
	seg := ss.currentSegmentLocked()
	if text == "" {
		text = seg.Text
	}
	ss.mu.Unlock()

	if text != "" {
		ss.editOrSendSegment(segIdx, text)
	}
}

// currentSegmentLocked returns the current TextSegment, creating it if needed.
// Must be called with ss.mu held.
func (ss *StreamingState) currentSegmentLocked() *TextSegment {
	seg, ok := ss.textSegments[ss.nextSegment]
	if !ok {
		seg = &TextSegment{}
		ss.textSegments[ss.nextSegment] = seg
	}
	return seg
}

// editOrSendSegment sends or edits the Telegram message for segmentIdx with text.
// If the text exceeds TelegramSafeLimit, the remainder becomes a new segment.
func (ss *StreamingState) editOrSendSegment(segmentIdx int, text string) {
	// Split if text exceeds the safe limit.
	parts := formatting.SplitMessage(text, config.TelegramSafeLimit)
	if len(parts) == 0 {
		return
	}

	// Send/edit the primary part.
	ss.sendOrEditWithFallback(segmentIdx, parts[0])

	// Any overflow parts become new segments.
	for _, extra := range parts[1:] {
		ss.mu.Lock()
		ss.nextSegment++
		nextIdx := ss.nextSegment
		seg := &TextSegment{Text: extra}
		ss.textSegments[nextIdx] = seg
		ss.mu.Unlock()
		ss.sendOrEditWithFallback(nextIdx, extra)
	}
}

// waitForRateLimit waits on the global API rate limiter if set.
// Uses a 5-second timeout context to avoid blocking indefinitely during shutdown.
func (ss *StreamingState) waitForRateLimit() {
	if ss.globalLimiter == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := ss.globalLimiter.Wait(ctx); err != nil {
		// Rate limited during shutdown — drop this edit.
		cancel()
		return
	}
	cancel()
}

// sendOrEditWithFallback sends or edits a message with MarkdownV2 formatting,
// falling back to plain text if Telegram rejects the MarkdownV2 parse.
func (ss *StreamingState) sendOrEditWithFallback(segmentIdx int, text string) {
	ss.mu.Lock()
	seg := ss.textSegments[segmentIdx]
	if seg == nil {
		seg = &TextSegment{Text: text}
		ss.textSegments[segmentIdx] = seg
	}
	msgID := seg.MessageID
	ss.mu.Unlock()

	formatted := formatting.ConvertToMarkdownV2(text)

	// Rate limit API calls if global limiter is set.
	ss.waitForRateLimit()

	if msgID == 0 {
		// Send new message.
		msg, err := ss.bot.SendMessage(ss.chatID, formatted, &gotgbot.SendMessageOpts{
			ParseMode:           "MarkdownV2",
			DisableNotification: true,
		})
		if err != nil {
			// Fallback to plain text.
			log.Debug().Err(err).Msg("MarkdownV2 send failed, retrying as plain text")
			plain := formatting.StripMarkdown(text)
			msg, err = ss.bot.SendMessage(ss.chatID, plain, &gotgbot.SendMessageOpts{
				DisableNotification: true,
			})
			if err != nil {
				log.Warn().Err(err).Int64("chat_id", ss.chatID).Msg("Failed to send message")
				return
			}
		}
		ss.mu.Lock()
		if ss.textSegments[segmentIdx] != nil {
			ss.textSegments[segmentIdx].MessageID = msg.MessageId
		}
		ss.mu.Unlock()
	} else {
		// Edit existing message.
		_, _, err := ss.bot.EditMessageText(formatted, &gotgbot.EditMessageTextOpts{
			ChatId:    ss.chatID,
			MessageId: msgID,
			ParseMode: "MarkdownV2",
		})
		if err != nil {
			// Fallback to plain text.
			log.Debug().Err(err).Msg("MarkdownV2 edit failed, retrying as plain text")
			plain := formatting.StripMarkdown(text)
			_, _, err = ss.bot.EditMessageText(plain, &gotgbot.EditMessageTextOpts{
				ChatId:    ss.chatID,
				MessageId: msgID,
			})
			if err != nil {
				log.Warn().Err(err).Int64("chat_id", ss.chatID).Msg("Failed to edit message")
			}
		}
	}
}

// updateStatusMessage edits the ephemeral status message (or sends a new one).
// Status messages use plain text (no parse mode) — they contain emojis and tool names.
func (ss *StreamingState) updateStatusMessage(text string) {
	ss.mu.Lock()
	statusMsgID := ss.statusMsgID
	ss.mu.Unlock()

	// Rate limit API calls if global limiter is set.
	ss.waitForRateLimit()

	if statusMsgID == 0 {
		msg, err := ss.bot.SendMessage(ss.chatID, text, &gotgbot.SendMessageOpts{
			DisableNotification: true,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Failed to send status message")
			return
		}
		ss.mu.Lock()
		ss.statusMsgID = msg.MessageId
		ss.mu.Unlock()
	} else {
		_, _, err := ss.bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
			ChatId:    ss.chatID,
			MessageId: statusMsgID,
		})
		if err != nil {
			log.Debug().Err(err).Msg("Failed to edit status message")
		}
	}
}

// deleteStatusMessage deletes the ephemeral status message if one exists.
func (ss *StreamingState) deleteStatusMessage() {
	ss.mu.Lock()
	statusMsgID := ss.statusMsgID
	ss.statusMsgID = 0
	ss.mu.Unlock()

	if statusMsgID != 0 {
		_, err := ss.bot.DeleteMessage(ss.chatID, statusMsgID, nil)
		if err != nil {
			log.Debug().Err(err).Msg("Failed to delete status message")
		}
	}
}

// TypingController manages a goroutine that periodically sends the "typing"
// chat action to Telegram while Claude is processing.
type TypingController struct {
	stop chan struct{}
}

// StartTypingIndicator starts a goroutine that sends "typing" every 4 seconds.
// Call Stop() to cancel it.
func StartTypingIndicator(bot *gotgbot.Bot, chatID int64) *TypingController {
	tc := &TypingController{stop: make(chan struct{})}
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		// Send immediately on start.
		_, _ = bot.SendChatAction(chatID, "typing", nil)
		for {
			select {
			case <-ticker.C:
				_, _ = bot.SendChatAction(chatID, "typing", nil)
			case <-tc.stop:
				return
			}
		}
	}()
	return tc
}

// Stop cancels the typing indicator goroutine.
func (tc *TypingController) Stop() {
	select {
	case <-tc.stop:
		// already stopped
	default:
		close(tc.stop)
	}
}
