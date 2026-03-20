package handlers

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/user/gsd-tele-go/internal/config"
)

// GsdOperation represents a single GSD operation with its key, button label, and command.
type GsdOperation struct {
	Key     string // callback suffix, e.g. "execute"
	Label   string // button text, e.g. "Execute Phase"
	Command string // slash command, e.g. "/gsd:execute-phase"
}

// GSDOperations is the ordered table of all 20 GSD operations.
var GSDOperations = []GsdOperation{
	{"next", "Next", "/gsd:next"},
	{"progress", "Progress", "/gsd:progress"},
	{"quick", "Quick Task", "/gsd:quick"},
	{"plan", "Plan Phase", "/gsd:plan-phase"},
	{"execute", "Execute Phase", "/gsd:execute-phase"},
	{"discuss", "Discuss Phase", "/gsd:discuss-phase"},
	{"research", "Research Phase", "/gsd:research-phase"},
	{"verify", "Verify Work", "/gsd:verify-work"},
	{"audit", "Audit Milestone", "/gsd:audit-milestone"},
	{"pause", "Pause Work", "/gsd:pause-work"},
	{"resume-work", "Resume Work", "/gsd:resume-work"},
	{"todos", "Check Todos", "/gsd:check-todos"},
	{"add-todo", "Add Todo", "/gsd:add-todo"},
	{"add-phase", "Add Phase", "/gsd:add-phase"},
	{"remove-phase", "Remove Phase", "/gsd:remove-phase"},
	{"new-project", "New Project", "/gsd:new-project"},
	{"new-milestone", "New Milestone", "/gsd:new-milestone"},
	{"settings", "Settings", "/gsd:settings"},
	{"debug", "Debug", "/gsd:debug"},
	{"help", "Help", "/gsd:help"},
}

// PhasePickerOps maps operation keys that require a phase picker before execution
// to their callback prefix.
var PhasePickerOps = map[string]string{
	"plan":         "gsd-plan",
	"execute":      "gsd-exec",
	"discuss":      "gsd-discuss",
	"research":     "gsd-research",
	"verify":       "gsd-verify",
	"remove-phase": "gsd-remove",
}

// Compile-once regex patterns for extraction and parsing.
var (
	gsdCmdRE      = regexp.MustCompile(`/gsd:([a-z-]+)(?:\s+([\d.]+))?`)
	numberedOptRE = regexp.MustCompile(`(?m)^(\d+)\.\s+(.+)`)
	letteredOptRE = regexp.MustCompile(`(?m)^([A-Z])[.)]\s+(.+)`)
	roadmapRE     = regexp.MustCompile(`^- \[(.)\] \*\*Phase ([\d.]+): ([^*]+)\*\* - (.+)$`)
)

// GsdSuggestion is a suggested GSD command found in response text.
type GsdSuggestion struct {
	Command string // e.g. "/gsd:execute-phase 2"
	Label   string // e.g. "Execute Phase 2"
}

// OptionButton is a numbered or lettered option extracted from response text.
type OptionButton struct {
	Key   string // "1", "A"
	Label string // truncated to ButtonLabelMaxLength
}

// RoadmapPhase is a parsed phase entry from ROADMAP.md.
type RoadmapPhase struct {
	Number string // "1", "2.1"
	Name   string // "Core Bot Infrastructure"
	Status string // "done", "pending", "skipped"
}

// gsdOpIndex is a lookup from operation key to GsdOperation for O(1) label lookup.
var gsdOpIndex = func() map[string]GsdOperation {
	m := make(map[string]GsdOperation, len(GSDOperations))
	for _, op := range GSDOperations {
		m[op.Key] = op
	}
	return m
}()

// ExtractGsdCommands finds all /gsd:command patterns in text and returns
// deduplicated GsdSuggestion values. Duplicate full matches (command + arg) are skipped.
func ExtractGsdCommands(text string) []GsdSuggestion {
	matches := gsdCmdRE.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool, len(matches))
	var out []GsdSuggestion

	for _, m := range matches {
		fullMatch := m[0]
		if seen[fullMatch] {
			continue
		}
		seen[fullMatch] = true

		key := m[1]
		phaseArg := m[2] // may be empty string

		// Build label: look up the operation key in the table.
		label := key // fallback: use key verbatim if not found
		if op, ok := gsdOpIndex[key]; ok {
			label = op.Label
		}
		if phaseArg != "" {
			label = label + " " + phaseArg
		}

		out = append(out, GsdSuggestion{
			Command: fullMatch,
			Label:   label,
		})
	}
	return out
}

// ExtractNumberedOptions finds numbered list items (1. 2. 3.) in text.
// Requires at least 2 consecutive items. Non-empty non-matching lines reset
// the sequence. Truncates labels to ButtonLabelMaxLength.
func ExtractNumberedOptions(text string) []OptionButton {
	lines := strings.Split(text, "\n")
	var result []OptionButton
	lastNum := 0

	for _, line := range lines {
		m := numberedOptRE.FindStringSubmatch(line)
		if m != nil {
			numStr := m[1]
			label := strings.TrimSpace(m[2])

			num := 0
			for _, ch := range numStr {
				num = num*10 + int(ch-'0')
			}

			if num == lastNum+1 {
				// Continue sequence.
				result = append(result, OptionButton{
					Key:   numStr,
					Label: truncate(label, config.ButtonLabelMaxLength),
				})
				lastNum = num
			} else if num == 1 {
				// New sequence — restart.
				result = []OptionButton{{
					Key:   numStr,
					Label: truncate(label, config.ButtonLabelMaxLength),
				}}
				lastNum = 1
			}
			// Out-of-sequence non-1 item: ignore.
		} else if strings.TrimSpace(line) != "" {
			// Non-empty non-matching line resets the sequence.
			if len(result) < 2 {
				result = nil
				lastNum = 0
			} else {
				// Already have 2+: stop scanning (sequence is complete).
				break
			}
		}
	}

	if len(result) < 2 {
		return nil
	}
	return result
}

// ExtractLetteredOptions finds lettered list items (A. B. C.) in text.
// Requires at least 2 consecutive items where letters are sequential (A→B, B→C).
// Uppercase only. Truncates labels to ButtonLabelMaxLength.
func ExtractLetteredOptions(text string) []OptionButton {
	lines := strings.Split(text, "\n")
	var result []OptionButton
	lastLetter := byte(0) // 0 = no previous letter

	for _, line := range lines {
		m := letteredOptRE.FindStringSubmatch(line)
		if m != nil {
			letter := m[1][0] // single uppercase ASCII byte
			label := strings.TrimSpace(m[2])

			if lastLetter != 0 && letter == lastLetter+1 {
				// Sequential continuation.
				result = append(result, OptionButton{
					Key:   string(rune(letter)),
					Label: truncate(label, config.ButtonLabelMaxLength),
				})
				lastLetter = letter
			} else if lastLetter == 0 || letter != lastLetter+1 {
				// New sequence or non-sequential: restart from this letter.
				result = []OptionButton{{
					Key:   string(rune(letter)),
					Label: truncate(label, config.ButtonLabelMaxLength),
				}}
				lastLetter = letter
			}
		} else if strings.TrimSpace(line) != "" {
			// Non-empty non-matching line resets the sequence.
			if len(result) < 2 {
				result = nil
				lastLetter = 0
			} else {
				break
			}
		}
	}

	if len(result) < 2 {
		return nil
	}
	return result
}

// ParseRoadmap reads .planning/ROADMAP.md from projectDir and returns all
// parsed phases. Returns an empty slice (not an error) if the file is missing.
func ParseRoadmap(projectDir string) []RoadmapPhase {
	path := filepath.Join(projectDir, ".planning", "ROADMAP.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var phases []RoadmapPhase
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		m := roadmapRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		statusChar := m[1]
		var status string
		switch statusChar {
		case "x":
			status = "done"
		case "~":
			status = "skipped"
		default:
			status = "pending"
		}
		phases = append(phases, RoadmapPhase{
			Number: m[2],
			Name:   strings.TrimSpace(m[3]),
			Status: status,
		})
	}
	return phases
}

// BuildGsdKeyboard builds the inline keyboard for the /gsd command.
// Layout: quick-actions row (Next + Progress) at top, then remaining 18 operations
// in 2-column rows (9 rows), for 10 rows total.
// Callback data format: "gsd:{key}".
func BuildGsdKeyboard(statusHeader string) gotgbot.InlineKeyboardMarkup {
	_ = statusHeader // reserved for future header row

	var rows [][]gotgbot.InlineKeyboardButton

	// Row 0: quick-actions (Next + Progress, first two operations).
	quickRow := []gotgbot.InlineKeyboardButton{
		{Text: GSDOperations[0].Label, CallbackData: "gsd:" + GSDOperations[0].Key},
		{Text: GSDOperations[1].Label, CallbackData: "gsd:" + GSDOperations[1].Key},
	}
	rows = append(rows, quickRow)

	// Remaining 18 operations in 2-column rows.
	rest := GSDOperations[2:]
	for i := 0; i < len(rest); i += 2 {
		row := []gotgbot.InlineKeyboardButton{
			{Text: rest[i].Label, CallbackData: "gsd:" + rest[i].Key},
		}
		if i+1 < len(rest) {
			row = append(row, gotgbot.InlineKeyboardButton{
				Text:         rest[i+1].Label,
				CallbackData: "gsd:" + rest[i+1].Key,
			})
		}
		rows = append(rows, row)
	}

	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// BuildResponseKeyboard builds the inline keyboard shown below GSD response messages.
// - GSD commands: two buttons per command: "Run" (gsd-run:{cmd}) and "Fresh" (gsd-fresh:{cmd})
// - Numbered options: one button per option (option:{key})
// - Lettered options: one button per option (option:{key})
func BuildResponseKeyboard(cmds []GsdSuggestion, numbered []OptionButton, lettered []OptionButton) gotgbot.InlineKeyboardMarkup {
	var rows [][]gotgbot.InlineKeyboardButton

	for _, cmd := range cmds {
		row := []gotgbot.InlineKeyboardButton{
			{Text: "Run", CallbackData: "gsd-run:" + cmd.Command},
			{Text: "Fresh", CallbackData: "gsd-fresh:" + cmd.Command},
		}
		rows = append(rows, row)
	}

	for _, opt := range numbered {
		row := []gotgbot.InlineKeyboardButton{
			{Text: opt.Key + ". " + opt.Label, CallbackData: "option:" + opt.Key},
		}
		rows = append(rows, row)
	}

	for _, opt := range lettered {
		row := []gotgbot.InlineKeyboardButton{
			{Text: opt.Key + ". " + opt.Label, CallbackData: "option:" + opt.Key},
		}
		rows = append(rows, row)
	}

	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// BuildPhasePickerKeyboard builds an inline keyboard with one button per non-skipped phase.
// Button label: status emoji + phase number + name.
// Callback data: "{callbackPrefix}:{phase.Number}".
func BuildPhasePickerKeyboard(phases []RoadmapPhase, callbackPrefix string) gotgbot.InlineKeyboardMarkup {
	var rows [][]gotgbot.InlineKeyboardButton

	for _, p := range phases {
		if p.Status == "skipped" {
			continue
		}
		emoji := "⏳"
		if p.Status == "done" {
			emoji = "✓"
		}
		label := emoji + " Phase " + p.Number + ": " + p.Name
		row := []gotgbot.InlineKeyboardButton{
			{Text: label, CallbackData: callbackPrefix + ":" + p.Number},
		}
		rows = append(rows, row)
	}

	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// truncate returns s truncated to max runes. Does not add ellipsis.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
