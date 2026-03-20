package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTranscribeVoice_Success verifies that transcribeVoice returns the transcript text
// when the mock server responds with {"text": "hello world"}.
func TestTranscribeVoice_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello world"})
	}))
	defer srv.Close()

	// Create a temporary voice file to upload.
	f, err := os.CreateTemp("", "voice_test_*.ogg")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("fake ogg data")
	f.Close()

	// Override the whisper endpoint by using the mock server URL.
	// We test the core multipart upload logic by calling a wrapped version.
	transcript, err := transcribeVoiceURL(context.Background(), "test-api-key", f.Name(), srv.URL+"/v1/audio/transcriptions")
	if err != nil {
		t.Fatalf("transcribeVoice returned error: %v", err)
	}
	if transcript != "hello world" {
		t.Errorf("transcript = %q, want %q", transcript, "hello world")
	}
}

// TestTranscribeVoice_AuthError verifies that a 401 response produces an error
// whose message contains "whisper API 401".
func TestTranscribeVoice_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	f, err := os.CreateTemp("", "voice_test_*.ogg")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	_, err = transcribeVoiceURL(context.Background(), "bad-key", f.Name(), srv.URL+"/v1/audio/transcriptions")
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	wantSubstr := "whisper API 401"
	if !contains(err.Error(), wantSubstr) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), wantSubstr)
	}
}

// TestExtractPDF_Success verifies that extractPDF captures stdout from the command.
// We use a platform-appropriate command to echo/cat the file contents.
func TestExtractPDF_Success(t *testing.T) {
	// Write a temp file with known content.
	f, err := os.CreateTemp("", "pdf_test_*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("extracted pdf content")
	f.Close()

	// Use a wrapper script/command that echoes the file contents.
	// On Windows we use cmd.exe /c type; on Unix we use cat.
	var cmdPath string
	if runtime.GOOS == "windows" {
		// We'll use a helper batch script approach -- but since extractPDF calls
		// exec.CommandContext(ctx, pdfToTextPath, "-layout", filePath, "-"),
		// we need a mock that ignores args and cats the file.
		// Create a .bat file that types the third arg (filePath = args[2]).
		bat, batErr := os.CreateTemp("", "mock_pdftotext_*.bat")
		if batErr != nil {
			t.Fatalf("CreateTemp bat: %v", batErr)
		}
		defer os.Remove(bat.Name())
		// Script: type the file passed as the 2nd argument (after -layout)
		// Args passed: -layout <filePath> -
		// %2 = filePath in batch
		_, _ = fmt.Fprintf(bat, "@echo off\r\ntype %%2\r\n")
		bat.Close()
		cmdPath = bat.Name()
	} else {
		// Unix: use a shell script that cats $2 (the filePath arg)
		sh, shErr := os.CreateTemp("", "mock_pdftotext_*.sh")
		if shErr != nil {
			t.Fatalf("CreateTemp sh: %v", shErr)
		}
		defer os.Remove(sh.Name())
		_, _ = fmt.Fprintf(sh, "#!/bin/sh\ncat \"$2\"\n")
		sh.Close()
		_ = os.Chmod(sh.Name(), 0755)
		cmdPath = sh.Name()
	}

	result, err := extractPDF(context.Background(), cmdPath, f.Name())
	if err != nil {
		t.Fatalf("extractPDF returned error: %v", err)
	}
	if !contains(result, "extracted pdf content") {
		t.Errorf("extractPDF result = %q, want to contain %q", result, "extracted pdf content")
	}
}

// TestExtractPDF_CommandError verifies that a nonexistent binary returns an error.
func TestExtractPDF_CommandError(t *testing.T) {
	f, err := os.CreateTemp("", "pdf_test_*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	_, err = extractPDF(context.Background(), "/nonexistent/pdftotext", f.Name())
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
}

// TestDownloadToTemp_Success verifies that downloadToTemp writes the server's bytes
// to a temp file and returns a valid path.
func TestDownloadToTemp_Success(t *testing.T) {
	const testContent = "telegram file bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testContent))
	}))
	defer srv.Close()

	// downloadToTemp calls bot.GetFile which requires a live bot. Instead, we test
	// the inner HTTP download logic directly via downloadFromURL helper.
	tmpPath, err := downloadFromURL(srv.URL, ".ogg")
	if err != nil {
		t.Fatalf("downloadFromURL returned error: %v", err)
	}
	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("temp file content = %q, want %q", string(data), testContent)
	}
}

// TestIsTextFile verifies that text file extensions are recognized correctly.
func TestIsTextFile(t *testing.T) {
	trueTests := []string{
		"main.py", "script.sh", "readme.md", "data.json", "config.yaml", "values.yml",
		"data.csv", "page.html", "style.css", "app.js", "types.ts", "notes.txt",
		"config.toml", "settings.ini", "app.cfg", "app.log", ".env",
	}
	for _, name := range trueTests {
		if !isTextFile(name) {
			t.Errorf("isTextFile(%q) = false, want true", name)
		}
	}

	falseTests := []string{
		"binary.exe", "archive.zip", "document.pdf", "image.png", "photo.jpg",
		"video.mp4", "audio.mp3", "compiled.bin", "package.tar.gz",
	}
	for _, name := range falseTests {
		if isTextFile(name) {
			t.Errorf("isTextFile(%q) = true, want false", name)
		}
	}
}

// TestIsTextFile_Extensions verifies the extension lookup is case-insensitive.
func TestIsTextFile_Extensions(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"FILE.PY", true},
		{"SCRIPT.SH", true},
		{"DATA.JSON", true},
		{"IMAGE.PNG", false},
		{"README", false}, // no extension
	}
	for _, tc := range tests {
		got := isTextFile(tc.name)
		if got != tc.want {
			t.Errorf("isTextFile(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestConstants_Helpers verifies that maxFileSize and maxTextChars have expected values.
func TestConstants_Helpers(t *testing.T) {
	const expectedMaxFileSize = 10 * 1024 * 1024
	const expectedMaxTextChars = 100_000

	if maxFileSize != expectedMaxFileSize {
		t.Errorf("maxFileSize = %d, want %d", maxFileSize, expectedMaxFileSize)
	}
	if maxTextChars != expectedMaxTextChars {
		t.Errorf("maxTextChars = %d, want %d", maxTextChars, expectedMaxTextChars)
	}
}

// Ensure filepath is used (referenced by test helpers).
var _ = filepath.Base
