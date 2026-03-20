package handlers

import (
	"sync"
	"time"
)

// MediaGroupBuffer collects media items arriving in the same Telegram media group
// and fires a batch callback after a configurable timeout of inactivity.
//
// Usage: call Add() for each incoming media group message. The buffer collects
// items with the same groupID and fires the process callback once no new items
// arrive within the timeout window. The callback receives the full slice of file
// paths, the chat and user IDs from the first Add call, and the first non-empty
// caption seen in the group.
//
// MediaGroupBuffer is safe for concurrent use.
type pendingGroup struct {
	paths   []string
	chatID  int64
	userID  int64
	caption string
	timer   *time.Timer
}

// MediaGroupBuffer manages in-flight media group batches.
type MediaGroupBuffer struct {
	mu      sync.Mutex
	groups  map[string]*pendingGroup
	timeout time.Duration
	process func(chatID int64, userID int64, paths []string, caption string)
}

// NewMediaGroupBuffer creates a MediaGroupBuffer that fires the process callback
// after timeout of inactivity within any given group.
//
// process is called at most once per groupID, after no new Add calls arrive
// within the timeout window. It is invoked from a goroutine created by
// time.AfterFunc, so it must be safe to run concurrently.
func NewMediaGroupBuffer(timeout time.Duration,
	process func(chatID int64, userID int64, paths []string, caption string)) *MediaGroupBuffer {
	return &MediaGroupBuffer{
		groups:  make(map[string]*pendingGroup),
		timeout: timeout,
		process: process,
	}
}

// Add records a new item for groupID, restarting the inactivity timer.
// chatID and userID are taken from the first Add call for each groupID.
// caption follows "first non-empty wins" semantics.
func (b *MediaGroupBuffer) Add(groupID string, path string, chatID int64, userID int64, caption string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	g, ok := b.groups[groupID]
	if !ok {
		g = &pendingGroup{chatID: chatID, userID: userID}
		b.groups[groupID] = g
	}

	g.paths = append(g.paths, path)

	// First non-empty caption wins.
	if caption != "" && g.caption == "" {
		g.caption = caption
	}

	// Reset the inactivity timer so the batch window extends with each new item.
	if g.timer != nil {
		g.timer.Stop()
	}
	captured := groupID
	g.timer = time.AfterFunc(b.timeout, func() {
		b.fire(captured)
	})
}

// fire removes the group from the buffer and invokes the process callback.
// Called from a time.AfterFunc goroutine — safe to call process outside the mutex.
func (b *MediaGroupBuffer) fire(groupID string) {
	b.mu.Lock()
	g, ok := b.groups[groupID]
	if !ok {
		b.mu.Unlock()
		return
	}
	delete(b.groups, groupID)
	b.mu.Unlock()

	b.process(g.chatID, g.userID, g.paths, g.caption)
}
