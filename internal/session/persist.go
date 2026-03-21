package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// SavedSession represents a persisted Claude session entry in the history file.
type SavedSession struct {
	// SessionID is the Claude CLI session identifier used with --resume.
	SessionID string `json:"session_id"`

	// SavedAt is the ISO 8601 timestamp when the session was last saved.
	SavedAt string `json:"saved_at"`

	// WorkingDir is the directory the session was created in.
	WorkingDir string `json:"working_dir"`

	// Title is the first ~50 characters of the first message sent in this session.
	Title string `json:"title"`

	// InstanceID is the instance identifier (project name or UUID) that owns this session.
	InstanceID string `json:"instance_id"`
}

// SessionHistory is the top-level structure stored in the persistence JSON file.
type SessionHistory struct {
	Sessions []SavedSession `json:"sessions"`
}

// PersistenceManager handles atomic read-modify-write of the session history JSON file.
//
// All methods are safe for concurrent use. The mutex serialises concurrent saves
// through the atomic write-rename pattern:
//  1. Marshal to a temp file in the same directory (same filesystem).
//  2. os.Rename(tmpPath, filePath) — atomic on POSIX; best-effort on Windows.
//
// On Windows, os.Rename is documented as atomic for files on the same volume.
type PersistenceManager struct {
	mu            sync.Mutex
	filePath      string
	maxPerProject int
}

// NewPersistenceManager creates a PersistenceManager targeting filePath.
// maxPerProject is the maximum number of sessions to retain per WorkingDir;
// the oldest entries are trimmed when the limit is exceeded.
// Passing 0 uses the default of 5 (matching config.MaxSessionHistory).
func NewPersistenceManager(filePath string, maxPerProject int) *PersistenceManager {
	if maxPerProject <= 0 {
		maxPerProject = 5
	}
	return &PersistenceManager{
		filePath:      filePath,
		maxPerProject: maxPerProject,
	}
}

// Save atomically appends or updates a session in the history file.
//
// Algorithm:
//  1. Load existing history (empty if file not found).
//  2. Update existing entry with matching SessionID, or append new entry.
//  3. Sort each WorkingDir group by SavedAt descending; keep only maxPerProject.
//  4. Write to temp file + rename.
func (pm *PersistenceManager) Save(sess SavedSession) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	history, err := pm.loadLocked()
	if err != nil {
		return err
	}

	// Update in-place if same SessionID already present, otherwise append.
	updated := false
	for i, s := range history.Sessions {
		if s.SessionID == sess.SessionID {
			history.Sessions[i] = sess
			updated = true
			break
		}
	}
	if !updated {
		history.Sessions = append(history.Sessions, sess)
	}

	// Trim each project to maxPerProject entries, keeping the most recent.
	history.Sessions = trimPerProject(history.Sessions, pm.maxPerProject)

	return pm.writeLocked(history)
}

// Load reads and returns the full session history.
// Returns an empty SessionHistory (no error) if the file does not exist.
func (pm *PersistenceManager) Load() (*SessionHistory, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.loadLocked()
}

// LoadForInstance returns all saved sessions for the given instance ID.
func (pm *PersistenceManager) LoadForInstance(instanceID string) ([]SavedSession, error) {
	history, err := pm.Load()
	if err != nil {
		return nil, err
	}

	var out []SavedSession
	for _, s := range history.Sessions {
		if s.InstanceID == instanceID {
			out = append(out, s)
		}
	}
	return out, nil
}

// GetLatestForInstance returns the most recently saved session for instanceID,
// or nil if none exist.  "Most recent" is determined by SavedAt string comparison
// (ISO 8601 timestamps sort lexicographically).
func (pm *PersistenceManager) GetLatestForInstance(instanceID string) (*SavedSession, error) {
	sessions, err := pm.LoadForInstance(instanceID)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}

	latest := sessions[0]
	for _, s := range sessions[1:] {
		if s.SavedAt > latest.SavedAt {
			latest = s
		}
	}
	return &latest, nil
}

// --- internal helpers ---

// loadLocked reads the file and returns its contents.
// Must be called with pm.mu held.
func (pm *PersistenceManager) loadLocked() (*SessionHistory, error) {
	data, err := os.ReadFile(pm.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &SessionHistory{Sessions: []SavedSession{}}, nil
		}
		return nil, err
	}

	var history SessionHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	if history.Sessions == nil {
		history.Sessions = []SavedSession{}
	}
	return &history, nil
}

// writeLocked serialises history to a temp file then atomically renames it to pm.filePath.
// Must be called with pm.mu held.
func (pm *PersistenceManager) writeLocked(history *SessionHistory) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	// Create temp file in the same directory so rename stays on the same filesystem.
	dir := filepath.Dir(pm.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "session-history-*.json.tmp")
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
	if err := os.Rename(tmpPath, pm.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

// trimPerProject keeps at most maxPerProject sessions per unique WorkingDir,
// retaining the most recent by SavedAt timestamp.
func trimPerProject(sessions []SavedSession, maxPerProject int) []SavedSession {
	// Group by working dir.
	type group struct {
		sessions []SavedSession
	}
	groups := make(map[string]*group)
	order := []string{} // preserve original project order

	for _, s := range sessions {
		if _, ok := groups[s.WorkingDir]; !ok {
			groups[s.WorkingDir] = &group{}
			order = append(order, s.WorkingDir)
		}
		groups[s.WorkingDir].sessions = append(groups[s.WorkingDir].sessions, s)
	}

	// Sort each group by SavedAt descending, trim.
	var out []SavedSession
	for _, dir := range order {
		g := groups[dir]
		sort.Slice(g.sessions, func(i, j int) bool {
			return g.sessions[i].SavedAt > g.sessions[j].SavedAt
		})
		if len(g.sessions) > maxPerProject {
			g.sessions = g.sessions[:maxPerProject]
		}
		out = append(out, g.sessions...)
	}
	return out
}

// nowISO8601 returns the current UTC time in ISO 8601 format.
// Provided as a helper for callers building SavedSession values.
func nowISO8601() string {
	return time.Now().UTC().Format(time.RFC3339)
}
