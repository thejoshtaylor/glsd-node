package handlers

import (
	"strings"
	"testing"
)

// TestClassifyDocument verifies document type classification by file extension.
func TestClassifyDocument(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"report.pdf", "pdf"},
		{"REPORT.PDF", "pdf"},
		{"code.py", "text"},
		{"app.ts", "text"},
		{"config.json", "text"},
		{"readme.md", "text"},
		{"script.sh", "text"},
		{"data.csv", "text"},
		{"image.png", "unsupported"},
		{"binary.exe", "unsupported"},
		{"archive.zip", "unsupported"},
		{"photo.jpg", "unsupported"},
	}
	for _, tc := range tests {
		got := classifyDocument(tc.filename)
		if got != tc.want {
			t.Errorf("classifyDocument(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

// TestBuildDocumentPrompt verifies prompt construction with and without caption.
func TestBuildDocumentPrompt(t *testing.T) {
	// With caption.
	got := buildDocumentPrompt("report.pdf", "extracted text here", "summarize")
	want := "[Document: report.pdf]\nextracted text here\n\nsummarize"
	if got != want {
		t.Errorf("buildDocumentPrompt with caption:\ngot:  %q\nwant: %q", got, want)
	}

	// Without caption.
	got = buildDocumentPrompt("code.py", "print('hello')", "")
	want = "[Document: code.py]\nprint('hello')"
	if got != want {
		t.Errorf("buildDocumentPrompt without caption:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestTruncateText verifies text truncation with ellipsis.
func TestTruncateText(t *testing.T) {
	// String longer than max — should truncate with "...".
	got := truncateText("abcdef", 3)
	if got != "abc..." {
		t.Errorf("truncateText(\"abcdef\", 3) = %q, want %q", got, "abc...")
	}

	// String shorter than max — no truncation.
	got = truncateText("ab", 3)
	if got != "ab" {
		t.Errorf("truncateText(\"ab\", 3) = %q, want %q", got, "ab")
	}

	// String exactly at max — no truncation.
	got = truncateText("abc", 3)
	if got != "abc" {
		t.Errorf("truncateText(\"abc\", 3) = %q, want %q", got, "abc")
	}

	// Empty string.
	got = truncateText("", 3)
	if got != "" {
		t.Errorf("truncateText(\"\", 3) = %q, want %q", got, "")
	}
}

// TestSupportedExtensionsList verifies the sorted extension list contains expected entries.
func TestSupportedExtensionsList(t *testing.T) {
	list := supportedExtensionsList()

	// Must contain .pdf and several text extensions.
	for _, ext := range []string{".pdf", ".py", ".md", ".json", ".ts", ".sh"} {
		if !strings.Contains(list, ext) {
			t.Errorf("supportedExtensionsList() = %q, missing %q", list, ext)
		}
	}

	// Must be sorted (check that .cfg comes before .css comes before .csv).
	cfgIdx := strings.Index(list, ".cfg")
	cssIdx := strings.Index(list, ".css")
	csvIdx := strings.Index(list, ".csv")
	if cfgIdx >= cssIdx || cssIdx >= csvIdx {
		t.Errorf("extensions not sorted: .cfg@%d, .css@%d, .csv@%d", cfgIdx, cssIdx, csvIdx)
	}
}
