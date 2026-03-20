package handlers

import (
	"strings"
	"testing"
)

// TestBuildSinglePhotoPrompt verifies prompt construction for single photos.
func TestBuildSinglePhotoPrompt(t *testing.T) {
	// With caption.
	got := buildSinglePhotoPrompt("/tmp/photo.jpg", "describe this")
	want := "[Photo: /tmp/photo.jpg]\n\ndescribe this"
	if got != want {
		t.Errorf("buildSinglePhotoPrompt with caption:\ngot:  %q\nwant: %q", got, want)
	}

	// Without caption.
	got = buildSinglePhotoPrompt("/tmp/photo.jpg", "")
	want = "[Photo: /tmp/photo.jpg]"
	if got != want {
		t.Errorf("buildSinglePhotoPrompt without caption:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestBuildAlbumPrompt verifies prompt construction for photo albums.
func TestBuildAlbumPrompt(t *testing.T) {
	// Multiple paths with caption.
	got := buildAlbumPrompt([]string{"/tmp/a.jpg", "/tmp/b.jpg"}, "my album")
	if !strings.Contains(got, "[Photos:") {
		t.Errorf("buildAlbumPrompt should contain '[Photos:', got: %q", got)
	}
	if !strings.Contains(got, "1. /tmp/a.jpg") {
		t.Errorf("buildAlbumPrompt should contain '1. /tmp/a.jpg', got: %q", got)
	}
	if !strings.Contains(got, "2. /tmp/b.jpg") {
		t.Errorf("buildAlbumPrompt should contain '2. /tmp/b.jpg', got: %q", got)
	}
	if !strings.Contains(got, "my album") {
		t.Errorf("buildAlbumPrompt should contain caption 'my album', got: %q", got)
	}

	// Single path without caption.
	got = buildAlbumPrompt([]string{"/tmp/a.jpg"}, "")
	if !strings.Contains(got, "[Photos:") {
		t.Errorf("buildAlbumPrompt should contain '[Photos:', got: %q", got)
	}
	if !strings.Contains(got, "1. /tmp/a.jpg") {
		t.Errorf("buildAlbumPrompt should contain '1. /tmp/a.jpg', got: %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("buildAlbumPrompt without caption should not have double newline, got: %q", got)
	}
}

// TestHandlePhoto_FunctionExists verifies the HandlePhoto function exists.
func TestHandlePhoto_FunctionExists(t *testing.T) {
	var fn interface{} = HandlePhoto
	if fn == nil {
		t.Fatal("HandlePhoto should not be nil")
	}
}
