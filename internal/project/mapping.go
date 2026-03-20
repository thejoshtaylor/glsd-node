// Package project provides channel-to-project mapping with JSON persistence.
package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// ProjectMapping holds the project path and metadata for a channel-to-project link.
type ProjectMapping struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	LinkedAt string `json:"linked_at"` // RFC3339
}

// mappingsFile is the JSON serialization format.
// JSON object keys must be strings, so channelIDs are stored as string keys.
type mappingsFile struct {
	Mappings map[string]ProjectMapping `json:"mappings"`
}

// MappingStore is a thread-safe map from channel ID to ProjectMapping,
// with atomic JSON persistence.
//
// Persistence uses the same atomic write-rename pattern as session/persist.go:
//  1. Marshal to a temp file in the same directory (same filesystem).
//  2. os.Rename(tmpPath, filePath) — atomic on POSIX; best-effort on Windows.
type MappingStore struct {
	mu       sync.RWMutex
	mappings map[int64]ProjectMapping
	filePath string
}

// NewMappingStore creates a MappingStore targeting filePath.
// Call Load() to populate from an existing file before use.
func NewMappingStore(filePath string) *MappingStore {
	return &MappingStore{
		mappings: make(map[int64]ProjectMapping),
		filePath: filePath,
	}
}

// Get returns the ProjectMapping for channelID, or (ProjectMapping{}, false) if not found.
// Safe for concurrent use.
func (ms *MappingStore) Get(channelID int64) (ProjectMapping, bool) {
	ms.mu.RLock()
	m, ok := ms.mappings[channelID]
	ms.mu.RUnlock()
	return m, ok
}

// Set stores the mapping for channelID and persists the change to disk.
// Returns an error only if persistence fails.
func (ms *MappingStore) Set(channelID int64, m ProjectMapping) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.mappings[channelID] = m
	return ms.saveLocked()
}

// Remove deletes the mapping for channelID and persists the change to disk.
// Returns an error only if persistence fails.
func (ms *MappingStore) Remove(channelID int64) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.mappings, channelID)
	return ms.saveLocked()
}

// All returns a shallow copy of all mappings.
// Safe for iteration without holding the store lock.
func (ms *MappingStore) All() map[int64]ProjectMapping {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	out := make(map[int64]ProjectMapping, len(ms.mappings))
	for k, v := range ms.mappings {
		out[k] = v
	}
	return out
}

// Load reads the mappings file from disk and populates the in-memory map.
// If the file does not exist, the store is initialized empty (not an error).
// Replaces any previously loaded data.
func (ms *MappingStore) Load() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	data, err := os.ReadFile(ms.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ms.mappings = make(map[int64]ProjectMapping)
			return nil
		}
		return err
	}

	var f mappingsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}

	ms.mappings = make(map[int64]ProjectMapping, len(f.Mappings))
	for key, m := range f.Mappings {
		channelID, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			// Skip malformed keys rather than failing the entire load.
			continue
		}
		ms.mappings[channelID] = m
	}
	return nil
}

// saveLocked serialises the current mappings to a temp file and atomically renames
// it to ms.filePath. Must be called with ms.mu held (write lock).
func (ms *MappingStore) saveLocked() error {
	f := mappingsFile{
		Mappings: make(map[string]ProjectMapping, len(ms.mappings)),
	}
	for channelID, m := range ms.mappings {
		f.Mappings[channelKey(channelID)] = m
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}

	// Create temp file in the same directory so rename stays on the same filesystem.
	dir := filepath.Dir(ms.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "project-mappings-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	// Write and close before rename.
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()

	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, ms.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

// channelKey converts a channelID int64 to a JSON-safe string key.
func channelKey(channelID int64) string {
	return strconv.FormatInt(channelID, 10)
}
