package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// OldSavedSession represents a persisted session entry in the old format,
// where sessions were keyed by Telegram channel ID (int64).
type OldSavedSession struct {
	SessionID  string `json:"session_id"`
	SavedAt    string `json:"saved_at"`
	WorkingDir string `json:"working_dir"`
	Title      string `json:"title"`
	ChannelID  int64  `json:"channel_id"`
}

// OldSessionHistory is the top-level structure of the old sessions.json format.
type OldSessionHistory struct {
	Sessions []OldSavedSession `json:"sessions"`
}

// ProjectMapping holds the project information from the old mappings.json format.
type ProjectMapping struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// MappingsFile is the top-level structure of the old mappings.json format.
// Keys are channel IDs as strings (e.g., "123456789").
type MappingsFile struct {
	Mappings map[string]ProjectMapping `json:"mappings"`
}

// MigrationResult reports the outcome of a MigrateSessionHistory call.
type MigrationResult struct {
	// Migrated is the count of sessions successfully mapped to a project name.
	Migrated int

	// Unmapped is the count of sessions that had no mapping match.
	Unmapped int

	// UnmappedEntries contains human-readable descriptions of each unmapped entry,
	// useful for logging. Format: "channel_id=NNN session_id=XXX working_dir=/path"
	UnmappedEntries []string
}

// MigrateSessionHistory reads the old channel-ID-keyed sessions.json format and
// rewrites it using instance IDs (project names) derived from mappings.json.
//
// Algorithm:
//  1. Read mappingsPath to build a channel-ID -> project-name lookup.
//     If the file is missing, treat as empty (all sessions unmappable).
//  2. Read sessionsPath as OldSessionHistory.
//     If the file is missing, return a zero-value result with no error.
//  3. For each old session:
//     - If its ChannelID maps to a project name: create a new SavedSession with
//       InstanceID set to the project name, copy all other fields.
//     - If not found: increment Unmapped, append a description to UnmappedEntries.
//  4. Write the new SessionHistory (containing only migrated sessions) to sessionsPath
//     using atomic write-rename.
//  5. Return MigrationResult.
func MigrateSessionHistory(sessionsPath, mappingsPath string) (*MigrationResult, error) {
	result := &MigrationResult{
		UnmappedEntries: []string{},
	}

	// Step 1: Load mappings (missing file = empty mappings).
	channelToProject, err := loadMappings(mappingsPath)
	if err != nil {
		return nil, fmt.Errorf("loading mappings from %s: %w", mappingsPath, err)
	}

	// Step 2: Load old sessions (missing file = return zeros).
	oldHistory, err := loadOldSessionHistory(sessionsPath)
	if err != nil {
		return nil, fmt.Errorf("loading sessions from %s: %w", sessionsPath, err)
	}
	if oldHistory == nil {
		// File did not exist — nothing to migrate.
		return result, nil
	}

	// Step 3: Map each old session.
	var newSessions []SavedSession
	for _, old := range oldHistory.Sessions {
		projectName, found := channelToProject[old.ChannelID]
		if !found {
			result.Unmapped++
			result.UnmappedEntries = append(result.UnmappedEntries,
				fmt.Sprintf("channel_id=%d session_id=%s working_dir=%s",
					old.ChannelID, old.SessionID, old.WorkingDir))
			continue
		}
		newSessions = append(newSessions, SavedSession{
			SessionID:  old.SessionID,
			SavedAt:    old.SavedAt,
			WorkingDir: old.WorkingDir,
			Title:      old.Title,
			InstanceID: projectName,
		})
		result.Migrated++
	}

	// Ensure sessions slice is non-nil for JSON marshalling ([] not null).
	if newSessions == nil {
		newSessions = []SavedSession{}
	}

	// Step 4: Write new format to sessionsPath atomically.
	newHistory := &SessionHistory{Sessions: newSessions}
	if err := writeSessionHistoryAtomic(sessionsPath, newHistory); err != nil {
		return nil, fmt.Errorf("writing migrated sessions to %s: %w", sessionsPath, err)
	}

	return result, nil
}

// loadMappings reads and parses mappingsPath, returning a map of channel ID -> project name.
// If the file does not exist, an empty map is returned (not an error).
func loadMappings(mappingsPath string) (map[int64]string, error) {
	data, err := os.ReadFile(mappingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[int64]string), nil
		}
		return nil, err
	}

	var mf MappingsFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parsing mappings JSON: %w", err)
	}

	lookup := make(map[int64]string, len(mf.Mappings))
	for key, mapping := range mf.Mappings {
		channelID, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			// Skip malformed keys rather than failing the whole migration.
			continue
		}
		lookup[channelID] = mapping.Name
	}
	return lookup, nil
}

// loadOldSessionHistory reads and parses sessionsPath as OldSessionHistory.
// Returns nil (no error) if the file does not exist.
func loadOldSessionHistory(sessionsPath string) (*OldSessionHistory, error) {
	data, err := os.ReadFile(sessionsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var history OldSessionHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("parsing sessions JSON: %w", err)
	}
	return &history, nil
}

// writeSessionHistoryAtomic writes history to path using atomic write-rename.
func writeSessionHistoryAtomic(path string, history *SessionHistory) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "session-history-migrate-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

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

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
