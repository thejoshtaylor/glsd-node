package formatting

import (
	"strings"
	"testing"
)

// TestEscapeMarkdownV2 verifies basic special character escaping.
func TestEscapeMarkdownV2(t *testing.T) {
	input := "Hello! Test. (1+2)"
	want := `Hello\! Test\. \(1\+2\)`
	got := EscapeMarkdownV2(input)
	if got != want {
		t.Errorf("EscapeMarkdownV2(%q) = %q; want %q", input, got, want)
	}
}

// TestEscapeMarkdownV2AllSpecialChars verifies all 19 special chars are escaped.
func TestEscapeMarkdownV2AllSpecialChars(t *testing.T) {
	// All special chars that must be escaped in MarkdownV2 plain text regions
	cases := []struct {
		input string
		want  string
	}{
		{"_", `\_`},
		{"*", `\*`},
		{"[", `\[`},
		{"]", `\]`},
		{"(", `\(`},
		{")", `\)`},
		{"~", `\~`},
		{"`", "\\`"},
		{">", `\>`},
		{"#", `\#`},
		{"+", `\+`},
		{"-", `\-`},
		{"=", `\=`},
		{"|", `\|`},
		{"{", `\{`},
		{"}", `\}`},
		{".", `\.`},
		{"!", `\!`},
		{`\`, `\\`},
	}
	for _, c := range cases {
		got := EscapeMarkdownV2(c.input)
		if got != c.want {
			t.Errorf("EscapeMarkdownV2(%q) = %q; want %q", c.input, got, c.want)
		}
	}
}

// TestEscapeMarkdownV2PreservesText verifies plain letters are not escaped.
func TestEscapeMarkdownV2PreservesText(t *testing.T) {
	input := "Hello World"
	got := EscapeMarkdownV2(input)
	if got != input {
		t.Errorf("EscapeMarkdownV2(%q) = %q; want %q (plain text unchanged)", input, got, input)
	}
}

// TestConvertBold verifies **text** -> *text* conversion.
func TestConvertBold(t *testing.T) {
	input := "**bold text**"
	got := ConvertToMarkdownV2(input)
	if !strings.Contains(got, "*bold text*") {
		t.Errorf("ConvertToMarkdownV2(%q) = %q; want it to contain *bold text*", input, got)
	}
}

// TestConvertHeaders verifies ## Header -> *Header*\n conversion.
func TestConvertHeaders(t *testing.T) {
	input := "## Title"
	got := ConvertToMarkdownV2(input)
	// Should convert to bold header
	if !strings.Contains(got, "*Title*") {
		t.Errorf("ConvertToMarkdownV2(%q) = %q; want *Title* in output", input, got)
	}
}

// TestConvertCodeBlock verifies code blocks are preserved verbatim.
func TestConvertCodeBlock(t *testing.T) {
	input := "```go\nfunc main() {\n\tfmt.Println(\"Hello!\")\n}\n```"
	got := ConvertToMarkdownV2(input)
	// Code block should be preserved — internal special chars NOT escaped
	if !strings.Contains(got, "```") {
		t.Errorf("ConvertToMarkdownV2(%q) = %q; want ``` markers preserved", input, got)
	}
	// Special chars inside code block should NOT be escaped
	if strings.Contains(got, `\!`) {
		t.Errorf("ConvertToMarkdownV2(%q) = %q; exclamation inside code block should NOT be escaped", input, got)
	}
}

// TestConvertInlineCode verifies inline code is preserved verbatim.
func TestConvertInlineCode(t *testing.T) {
	input := "`code.here`"
	got := ConvertToMarkdownV2(input)
	// Inline code should be preserved — dot inside should NOT be escaped
	if strings.Contains(got, `\.`) {
		t.Errorf("ConvertToMarkdownV2(%q) = %q; dot inside inline code should NOT be escaped", input, got)
	}
	if !strings.Contains(got, "`") {
		t.Errorf("ConvertToMarkdownV2(%q) = %q; backtick markers should be present", input, got)
	}
}

// TestConvertMixedContent verifies a real Claude-like response doesn't crash
// and produces output without obvious corruption.
func TestConvertMixedContent(t *testing.T) {
	input := `## Summary

Here is **bold text** and some regular text.

` + "```go\nfunc hello() {\n\tfmt.Println(\"world\")\n}\n```" + `

Use ` + "`inline code`" + ` for short snippets.

- Bullet point one
- Bullet point two`

	got := ConvertToMarkdownV2(input)
	if len(got) == 0 {
		t.Error("ConvertToMarkdownV2 returned empty string for mixed content")
	}
	// Should contain code block markers
	if !strings.Contains(got, "```") {
		t.Errorf("ConvertToMarkdownV2 lost code block markers: %q", got)
	}
}

// TestStripMarkdown verifies markdown is removed leaving plain text.
func TestStripMarkdown(t *testing.T) {
	input := "**bold** and `code`"
	got := StripMarkdown(input)
	// Should not contain ** or backticks
	if strings.Contains(got, "**") {
		t.Errorf("StripMarkdown(%q) = %q; should not contain **", input, got)
	}
	if strings.Contains(got, "`") {
		t.Errorf("StripMarkdown(%q) = %q; should not contain backticks", input, got)
	}
	// Should contain the words
	if !strings.Contains(got, "bold") {
		t.Errorf("StripMarkdown(%q) = %q; should contain 'bold'", input, got)
	}
	if !strings.Contains(got, "code") {
		t.Errorf("StripMarkdown(%q) = %q; should contain 'code'", input, got)
	}
}

// TestStripMarkdownHeaders verifies header markers are removed.
func TestStripMarkdownHeaders(t *testing.T) {
	input := "# H1 Title\n## H2 Title"
	got := StripMarkdown(input)
	if strings.Contains(got, "#") {
		t.Errorf("StripMarkdown(%q) = %q; should not contain # markers", input, got)
	}
	if !strings.Contains(got, "H1 Title") {
		t.Errorf("StripMarkdown(%q) = %q; should contain 'H1 Title'", input, got)
	}
}

// TestSplitMessageShort verifies a short message is returned as a single-element slice.
func TestSplitMessageShort(t *testing.T) {
	input := "Short message"
	parts := SplitMessage(input, 4096)
	if len(parts) != 1 {
		t.Errorf("SplitMessage short text: got %d parts; want 1", len(parts))
	}
	if parts[0] != input {
		t.Errorf("SplitMessage short text: got %q; want %q", parts[0], input)
	}
}

// TestSplitMessageAtParagraph verifies splitting at paragraph boundary.
func TestSplitMessageAtParagraph(t *testing.T) {
	// Build a 5000-char text with a paragraph boundary before 4096
	// First paragraph: 2000 chars of 'a', then \n\n, then 3000 chars of 'b'
	para1 := strings.Repeat("a", 2000)
	para2 := strings.Repeat("b", 3000)
	input := para1 + "\n\n" + para2

	parts := SplitMessage(input, 4096)
	if len(parts) < 2 {
		t.Errorf("SplitMessage with paragraph boundary: got %d parts; want >= 2", len(parts))
		return
	}
	// First part should end with the paragraph separator
	first := parts[0]
	if !strings.HasSuffix(strings.TrimRight(first, "\n"), para1) {
		t.Errorf("SplitMessage first part should contain para1 block, got length %d", len(first))
	}
	// No part should exceed the limit
	for i, part := range parts {
		if len(part) > 4096 {
			t.Errorf("SplitMessage part %d has length %d > 4096", i, len(part))
		}
	}
}

// TestSplitMessageNoBreak verifies hard split when no whitespace exists.
func TestSplitMessageNoBreak(t *testing.T) {
	// 5000 chars with no whitespace
	input := strings.Repeat("x", 5000)
	parts := SplitMessage(input, 4096)
	if len(parts) < 2 {
		t.Errorf("SplitMessage no-break: got %d parts; want >= 2", len(parts))
		return
	}
	// First part should be exactly limit chars
	if len(parts[0]) != 4096 {
		t.Errorf("SplitMessage no-break: first part length %d; want 4096", len(parts[0]))
	}
}

// TestSplitMessageReassembly verifies no characters are lost during splitting.
func TestSplitMessageReassembly(t *testing.T) {
	input := strings.Repeat("Hello world. ", 400) // ~5200 chars
	parts := SplitMessage(input, 4096)
	reassembled := strings.Join(parts, "")
	if reassembled != input {
		t.Errorf("SplitMessage reassembly lost characters: input len %d, output len %d",
			len(input), len(reassembled))
	}
}
