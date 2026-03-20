package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============ ExtractGsdCommands tests ============

func TestExtractGsdCommands_Single(t *testing.T) {
	text := "You should run /gsd:execute-phase 2 to continue."
	cmds := ExtractGsdCommands(text)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Command != "/gsd:execute-phase 2" {
		t.Errorf("unexpected command: %q", cmds[0].Command)
	}
	if !strings.Contains(cmds[0].Label, "2") {
		t.Errorf("label should include phase number, got: %q", cmds[0].Label)
	}
}

func TestExtractGsdCommands_Multiple(t *testing.T) {
	text := "Options: /gsd:next, /gsd:progress, or /gsd:execute-phase 3"
	cmds := ExtractGsdCommands(text)
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(cmds), cmds)
	}
}

func TestExtractGsdCommands_Dedup(t *testing.T) {
	text := "Run /gsd:next and then /gsd:next again."
	cmds := ExtractGsdCommands(text)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 (deduplicated), got %d", len(cmds))
	}
}

func TestExtractGsdCommands_None(t *testing.T) {
	text := "No commands here. Just plain text."
	cmds := ExtractGsdCommands(text)
	if len(cmds) != 0 {
		t.Fatalf("expected 0 commands, got %d", len(cmds))
	}
}

// ============ ExtractNumberedOptions tests ============

func TestExtractNumberedOptions_Basic(t *testing.T) {
	text := "Choose an option:\n1. Option A\n2. Option B\n"
	opts := ExtractNumberedOptions(text)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d: %v", len(opts), opts)
	}
	if opts[0].Key != "1" {
		t.Errorf("expected key '1', got %q", opts[0].Key)
	}
	if opts[1].Key != "2" {
		t.Errorf("expected key '2', got %q", opts[1].Key)
	}
}

func TestExtractNumberedOptions_NonConsecutive(t *testing.T) {
	text := "Some header text.\n1. Only one item\nSome footer text."
	opts := ExtractNumberedOptions(text)
	if len(opts) != 0 {
		t.Fatalf("expected 0 (need 2+ consecutive), got %d: %v", len(opts), opts)
	}
}

func TestExtractNumberedOptions_Three(t *testing.T) {
	text := "1. Alpha\n2. Beta\n3. Gamma"
	opts := ExtractNumberedOptions(text)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options, got %d", len(opts))
	}
}

// ============ ExtractLetteredOptions tests ============

func TestExtractLetteredOptions_Basic(t *testing.T) {
	text := "A. First option\nB. Second option"
	opts := ExtractLetteredOptions(text)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d: %v", len(opts), opts)
	}
	if opts[0].Key != "A" {
		t.Errorf("expected key 'A', got %q", opts[0].Key)
	}
	if opts[1].Key != "B" {
		t.Errorf("expected key 'B', got %q", opts[1].Key)
	}
}

func TestExtractLetteredOptions_NonSequential(t *testing.T) {
	text := "A. First\nC. Third"
	opts := ExtractLetteredOptions(text)
	if len(opts) != 0 {
		t.Fatalf("expected 0 (must be sequential A→B→C), got %d: %v", len(opts), opts)
	}
}

func TestExtractLetteredOptions_MixedCase(t *testing.T) {
	text := "a) first option\nb) second option"
	opts := ExtractLetteredOptions(text)
	if len(opts) != 0 {
		t.Fatalf("expected 0 (lowercase not matched), got %d: %v", len(opts), opts)
	}
}

// ============ ParseRoadmap tests ============

func TestParseRoadmap_AllStatuses(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, ".planning")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := strings.Join([]string{
		"# ROADMAP",
		"",
		"- [x] **Phase 1: Done Phase** - It is done",
		"- [ ] **Phase 2: Pending Phase** - Work needed",
		"- [~] **Phase 3: Skipped Phase** - Not needed",
		"",
	}, "\n")

	roadmapPath := filepath.Join(planDir, "ROADMAP.md")
	if err := os.WriteFile(roadmapPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	phases := ParseRoadmap(dir)
	if len(phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(phases))
	}

	if phases[0].Status != "done" {
		t.Errorf("phase 1 status: got %q, want %q", phases[0].Status, "done")
	}
	if phases[1].Status != "pending" {
		t.Errorf("phase 2 status: got %q, want %q", phases[1].Status, "pending")
	}
	if phases[2].Status != "skipped" {
		t.Errorf("phase 3 status: got %q, want %q", phases[2].Status, "skipped")
	}
	if phases[0].Number != "1" {
		t.Errorf("phase 1 number: got %q, want %q", phases[0].Number, "1")
	}
	if phases[1].Name != "Pending Phase" {
		t.Errorf("phase 2 name: got %q, want %q", phases[1].Name, "Pending Phase")
	}
}

func TestParseRoadmap_MissingFile(t *testing.T) {
	dir := t.TempDir()
	// No ROADMAP.md created.
	phases := ParseRoadmap(dir)
	if phases != nil && len(phases) != 0 {
		t.Fatalf("expected empty slice for missing file, got %d phases", len(phases))
	}
}

// ============ BuildGsdKeyboard tests ============

func TestBuildGsdKeyboard_RowCount(t *testing.T) {
	kb := BuildGsdKeyboard("")
	rows := kb.InlineKeyboard
	// 1 quick row + 9 pairs from 18 remaining = 10 rows total
	if len(rows) != 10 {
		t.Errorf("expected 10 rows, got %d", len(rows))
	}
	// First row (quick-actions) must have exactly 2 buttons.
	if len(rows[0]) != 2 {
		t.Errorf("expected 2 buttons in quick row, got %d", len(rows[0]))
	}
}

// ============ Callback data length tests ============

func TestCallbackDataLength(t *testing.T) {
	const maxLen = 64

	// GSD keyboard callback data.
	kb := BuildGsdKeyboard("")
	for i, row := range kb.InlineKeyboard {
		for j, btn := range row {
			if len(btn.CallbackData) > maxLen {
				t.Errorf("GSDKeyboard row[%d][%d] callback data too long (%d > %d): %q",
					i, j, len(btn.CallbackData), maxLen, btn.CallbackData)
			}
		}
	}

	// Phase picker callback data — use a long prefix to stress-test.
	phases := []RoadmapPhase{
		{Number: "1", Name: "Core Infrastructure", Status: "done"},
		{Number: "2.1", Name: "Feature Work", Status: "pending"},
	}
	for _, prefix := range []string{"gsd-plan", "gsd-exec", "gsd-discuss", "gsd-research", "gsd-verify", "gsd-remove"} {
		kb2 := BuildPhasePickerKeyboard(phases, prefix)
		for i, row := range kb2.InlineKeyboard {
			for j, btn := range row {
				if len(btn.CallbackData) > maxLen {
					t.Errorf("PhasePickerKeyboard prefix=%q row[%d][%d] callback data too long (%d > %d): %q",
						prefix, i, j, len(btn.CallbackData), maxLen, btn.CallbackData)
				}
			}
		}
	}

	// Response keyboard callback data.
	cmds := []GsdSuggestion{
		{Command: "/gsd:execute-phase 2", Label: "Execute Phase 2"},
	}
	numbered := []OptionButton{{Key: "1", Label: "Option A"}}
	lettered := []OptionButton{{Key: "A", Label: "Choice X"}}
	kb3 := BuildResponseKeyboard(cmds, numbered, lettered)
	for i, row := range kb3.InlineKeyboard {
		for j, btn := range row {
			if len(btn.CallbackData) > maxLen {
				t.Errorf("ResponseKeyboard row[%d][%d] callback data too long (%d > %d): %q",
					i, j, len(btn.CallbackData), maxLen, btn.CallbackData)
			}
		}
	}
}

// TestGSDOperationsCount verifies the table has exactly 20 entries.
func TestGSDOperationsCount(t *testing.T) {
	if len(GSDOperations) != 20 {
		t.Errorf("expected 20 GSD operations, got %d", len(GSDOperations))
	}
}

// TestPhasePickerKeyboard_SkipsSkipped verifies skipped phases are excluded.
func TestPhasePickerKeyboard_SkipsSkipped(t *testing.T) {
	phases := []RoadmapPhase{
		{Number: "1", Name: "Phase One", Status: "done"},
		{Number: "2", Name: "Phase Two", Status: "skipped"},
		{Number: "3", Name: "Phase Three", Status: "pending"},
	}
	kb := BuildPhasePickerKeyboard(phases, "gsd-exec")
	rows := kb.InlineKeyboard
	if len(rows) != 2 {
		t.Errorf("expected 2 rows (skipped excluded), got %d", len(rows))
	}
	for _, row := range rows {
		for _, btn := range row {
			if strings.Contains(btn.CallbackData, ":2") {
				t.Errorf("skipped phase 2 should not appear in keyboard: %q", btn.CallbackData)
			}
			_ = fmt.Sprintf("btn: %s", btn.CallbackData)
		}
	}
}
