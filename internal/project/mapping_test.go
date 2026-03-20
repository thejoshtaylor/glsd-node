package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTempMapping creates a MappingStore with a file path in a temp directory.
func newTempMapping(t *testing.T) (*MappingStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mappings.json")
	return NewMappingStore(path), path
}

// sampleMapping returns a test ProjectMapping.
func sampleMapping(path, name string) ProjectMapping {
	return ProjectMapping{
		Path:     path,
		Name:     name,
		LinkedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// TestMappingGetEmpty verifies that Get on a fresh store returns false.
func TestMappingGetEmpty(t *testing.T) {
	ms, _ := newTempMapping(t)
	_, ok := ms.Get(12345)
	if ok {
		t.Fatal("expected false for Get on empty store")
	}
}

// TestMappingSetAndGet verifies that Set then Get returns the correct mapping.
func TestMappingSetAndGet(t *testing.T) {
	ms, _ := newTempMapping(t)
	m := sampleMapping("/home/user/project", "My Project")
	if err := ms.Set(100, m); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	got, ok := ms.Get(100)
	if !ok {
		t.Fatal("expected mapping to be found after Set")
	}
	if got.Path != m.Path {
		t.Errorf("path mismatch: got %q, want %q", got.Path, m.Path)
	}
	if got.Name != m.Name {
		t.Errorf("name mismatch: got %q, want %q", got.Name, m.Name)
	}
}

// TestMappingRemove verifies that Remove deletes the mapping.
func TestMappingRemove(t *testing.T) {
	ms, _ := newTempMapping(t)
	m := sampleMapping("/home/user/project", "My Project")
	if err := ms.Set(200, m); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := ms.Remove(200); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	_, ok := ms.Get(200)
	if ok {
		t.Fatal("expected false for Get after Remove")
	}
}

// TestMappingAll verifies that All returns all stored mappings.
func TestMappingAll(t *testing.T) {
	ms, _ := newTempMapping(t)
	ms.Set(1, sampleMapping("/a", "Alpha"))
	ms.Set(2, sampleMapping("/b", "Beta"))
	ms.Set(3, sampleMapping("/c", "Gamma"))

	all := ms.All()
	if len(all) != 3 {
		t.Errorf("expected 3 mappings, got %d", len(all))
	}
	for _, id := range []int64{1, 2, 3} {
		if _, ok := all[id]; !ok {
			t.Errorf("expected mapping for channelID %d", id)
		}
	}
}

// TestMappingPersistence verifies that Set persists data that Load can recover.
func TestMappingPersistence(t *testing.T) {
	ms, path := newTempMapping(t)
	want := sampleMapping("/home/user/myproject", "Persisted Project")
	if err := ms.Set(999, want); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Create a new store pointing at the same file.
	ms2 := NewMappingStore(path)
	if err := ms2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got, ok := ms2.Get(999)
	if !ok {
		t.Fatal("expected mapping to survive persist/load cycle")
	}
	if got.Path != want.Path {
		t.Errorf("path mismatch after load: got %q, want %q", got.Path, want.Path)
	}
	if got.Name != want.Name {
		t.Errorf("name mismatch after load: got %q, want %q", got.Name, want.Name)
	}
}

// TestMappingReassign verifies that Set on an existing channel replaces the mapping.
func TestMappingReassign(t *testing.T) {
	ms, path := newTempMapping(t)
	ms.Set(42, sampleMapping("/path/a", "Project A"))
	ms.Set(42, sampleMapping("/path/b", "Project B"))

	got, ok := ms.Get(42)
	if !ok {
		t.Fatal("expected mapping after reassign")
	}
	if got.Path != "/path/b" {
		t.Errorf("expected /path/b after reassign, got %q", got.Path)
	}

	// Verify persistence reflects the new value.
	ms2 := NewMappingStore(path)
	if err := ms2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	got2, ok := ms2.Get(42)
	if !ok {
		t.Fatal("expected reassigned mapping to persist")
	}
	if got2.Path != "/path/b" {
		t.Errorf("expected /path/b after load, got %q", got2.Path)
	}
}

// TestMappingLoadMissingFile verifies that Load with a nonexistent file is not an error.
func TestMappingLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	ms := NewMappingStore(path)

	if err := ms.Load(); err != nil {
		t.Fatalf("Load of missing file should not return error, got: %v", err)
	}

	all := ms.All()
	if len(all) != 0 {
		t.Errorf("expected empty map after loading missing file, got %d entries", len(all))
	}

	// Confirm the file was not created by Load.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Load should not create the file if it does not exist")
	}
}
