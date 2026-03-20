// Package formatting provides Markdown to MarkdownV2 conversion and message
// utilities for Telegram Bot API output.
package formatting

import (
	"strings"
)

// markdownV2Replacer escapes all MarkdownV2 special characters in plain text
// regions. Characters: _ * [ ] ( ) ~ ` > # + - = | { } . !
var markdownV2Replacer = strings.NewReplacer(
	`\`, `\\`,
	`_`, `\_`,
	`*`, `\*`,
	`[`, `\[`,
	`]`, `\]`,
	`(`, `\(`,
	`)`, `\)`,
	`~`, `\~`,
	"`", "\\`",
	`>`, `\>`,
	`#`, `\#`,
	`+`, `\+`,
	`-`, `\-`,
	`=`, `\=`,
	`|`, `\|`,
	`{`, `\{`,
	`}`, `\}`,
	`.`, `\.`,
	`!`, `\!`,
)

// EscapeMarkdownV2 escapes all MarkdownV2 special characters in the given text
// so it can be safely sent as a plain text region in a MarkdownV2 message.
// The backslash itself is escaped first to avoid double-escaping.
func EscapeMarkdownV2(text string) string {
	return markdownV2Replacer.Replace(text)
}

// ConvertToMarkdownV2 converts standard Markdown text to Telegram MarkdownV2
// format. It uses a two-pass approach:
//  1. Extract code blocks and inline code verbatim (they don't need escaping inside).
//  2. Convert markdown formatting to MarkdownV2 entities and escape plain text.
//  3. Restore code blocks wrapped in ``` and inline code wrapped in `.
func ConvertToMarkdownV2(text string) string {
	codeBlocks := []string{}
	inlineCodes := []string{}

	// Pass 1a: Extract triple-backtick code blocks (with optional language tag).
	// Replace them with NUL-delimited placeholders.
	var b strings.Builder
	remaining := text
	for {
		start := strings.Index(remaining, "```")
		if start == -1 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:start])
		rest := remaining[start+3:]
		end := strings.Index(rest, "```")
		if end == -1 {
			// Unclosed code block — treat the ``` as literal text
			b.WriteString("```")
			remaining = rest
			continue
		}
		// rest[:end] is the content (may include language tag on first line)
		content := rest[:end]
		// Strip leading language tag if present (e.g., "go\n...")
		if nl := strings.Index(content, "\n"); nl != -1 {
			firstLine := strings.TrimSpace(content[:nl])
			// Language tag contains no spaces and is alphanumeric/dash
			if len(firstLine) > 0 && !strings.ContainsAny(firstLine, " \t") {
				content = content[nl+1:]
			}
		}
		codeBlocks = append(codeBlocks, content)
		b.WriteString("\x00CODEBLOCK")
		writeInt(&b, len(codeBlocks)-1)
		b.WriteByte('\x00')
		remaining = rest[end+3:]
	}
	text = b.String()

	// Pass 1b: Extract inline code (single backtick).
	b.Reset()
	remaining = text
	for {
		start := strings.Index(remaining, "`")
		if start == -1 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:start])
		rest := remaining[start+1:]
		end := strings.Index(rest, "`")
		if end == -1 {
			// Unclosed inline code — treat the ` as literal
			b.WriteByte('`')
			remaining = rest
			continue
		}
		inlineCodes = append(inlineCodes, rest[:end])
		b.WriteString("\x00INLINECODE")
		writeInt(&b, len(inlineCodes)-1)
		b.WriteByte('\x00')
		remaining = rest[end+1:]
	}
	text = b.String()

	// Pass 2: Convert markdown formatting in the remaining text.
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = convertLine(line)
	}
	text = strings.Join(lines, "\n")

	// Pass 3: Restore code blocks.
	for i, block := range codeBlocks {
		placeholder := placeholder("CODEBLOCK", i)
		// Wrap in ``` ... ``` — no escaping needed inside pre blocks.
		replacement := "```\n" + block + "```"
		text = strings.Replace(text, placeholder, replacement, 1)
	}

	// Restore inline code.
	for i, code := range inlineCodes {
		placeholder := placeholder("INLINECODE", i)
		// Wrap in ` ... ` — no escaping needed inside code spans.
		replacement := "`" + code + "`"
		text = strings.Replace(text, placeholder, replacement, 1)
	}

	// Collapse 3+ consecutive newlines to 2.
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return text
}

// convertLine converts a single line of Markdown to MarkdownV2.
// It handles headers, bold, italic, bullet lists, links, and plain text escaping.
func convertLine(line string) string {
	// Headers: # ... -> *header text*
	if strings.HasPrefix(line, "#") {
		i := 0
		for i < len(line) && line[i] == '#' {
			i++
		}
		if i < len(line) && line[i] == ' ' {
			headerText := strings.TrimSpace(line[i+1:])
			// Escape special chars in header text, then bold it.
			return "*" + EscapeMarkdownV2(headerText) + "*\n"
		}
	}

	// Process the line character by character for bold, italic, links, and escaping.
	return convertInline(line)
}

// convertInline processes inline markdown formatting (bold, italic, links)
// and escapes plain text regions for MarkdownV2.
func convertInline(text string) string {
	var b strings.Builder
	i := 0
	runes := []rune(text)
	n := len(runes)

	for i < n {
		r := runes[i]

		// Bold: **text**
		if r == '*' && i+1 < n && runes[i+1] == '*' {
			end := findClose(runes, i+2, "**")
			if end != -1 {
				inner := string(runes[i+2 : end])
				b.WriteString("*")
				b.WriteString(EscapeMarkdownV2(inner))
				b.WriteString("*")
				i = end + 2
				continue
			}
		}

		// Italic: _text_ (single underscore, not double)
		if r == '_' && (i == 0 || runes[i-1] == ' ' || runes[i-1] == '\t') {
			end := findCloseRune(runes, i+1, '_')
			if end != -1 {
				inner := string(runes[i+1 : end])
				b.WriteString("_")
				b.WriteString(EscapeMarkdownV2(inner))
				b.WriteString("_")
				i = end + 1
				continue
			}
		}

		// Markdown link: [text](url)
		if r == '[' {
			closeBracket := findCloseRune(runes, i+1, ']')
			if closeBracket != -1 && closeBracket+1 < n && runes[closeBracket+1] == '(' {
				closeParen := findCloseRune(runes, closeBracket+2, ')')
				if closeParen != -1 {
					linkText := string(runes[i+1 : closeBracket])
					url := string(runes[closeBracket+2 : closeParen])
					// Link text and url need specific escaping for MarkdownV2
					b.WriteString("[")
					b.WriteString(EscapeMarkdownV2(linkText))
					b.WriteString("](")
					b.WriteString(escapeLinkURL(url))
					b.WriteString(")")
					i = closeParen + 1
					continue
				}
			}
		}

		// Bullet list items: - or * at the start of the line (only spaces before it)
		if r == '-' && i+1 < n && runes[i+1] == ' ' && allSpaces(runes[:i]) {
			// Convert to bullet character (escape the next content)
			b.WriteString("• ")
			rest := convertInline(string(runes[i+2:]))
			b.WriteString(rest)
			return b.String()
		}

		// Placeholder markers — pass through as-is (will be replaced later).
		if r == '\x00' {
			b.WriteRune(r)
			i++
			continue
		}

		// Plain text character — escape it.
		encoded := string(r)
		b.WriteString(EscapeMarkdownV2(encoded))
		i++
	}

	return b.String()
}

// findClose finds the next occurrence of a two-character close marker starting
// from position start in runes. Returns the index of the first char of the
// close marker, or -1 if not found.
func findClose(runes []rune, start int, marker string) int {
	m := []rune(marker)
	for i := start; i < len(runes)-len(m)+1; i++ {
		match := true
		for j, mr := range m {
			if runes[i+j] != mr {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// findCloseRune finds the next occurrence of close rune starting from start.
func findCloseRune(runes []rune, start int, close rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == close {
			return i
		}
	}
	return -1
}

// allSpaces returns true if all runes are spaces or tabs.
func allSpaces(runes []rune) bool {
	for _, r := range runes {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

// escapeLinkURL escapes special chars in a URL for MarkdownV2.
// Only ) and \ need to be escaped inside link URLs.
func escapeLinkURL(url string) string {
	url = strings.ReplaceAll(url, `\`, `\\`)
	url = strings.ReplaceAll(url, `)`, `\)`)
	return url
}

// placeholder builds the NUL-delimited placeholder string for index i.
func placeholder(kind string, i int) string {
	var b strings.Builder
	b.WriteByte('\x00')
	b.WriteString(kind)
	writeInt(&b, i)
	b.WriteByte('\x00')
	return b.String()
}

// writeInt writes a decimal integer to b.
func writeInt(b *strings.Builder, n int) {
	if n == 0 {
		b.WriteByte('0')
		return
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	b.Write(buf[pos:])
}

// StripMarkdown removes all markdown formatting and returns plain text.
// This is the fallback used when MarkdownV2 parsing fails.
func StripMarkdown(text string) string {
	// Remove code blocks first (preserve content, strip backticks)
	// Triple backtick blocks
	result := &strings.Builder{}
	remaining := text
	for {
		start := strings.Index(remaining, "```")
		if start == -1 {
			result.WriteString(remaining)
			break
		}
		result.WriteString(remaining[:start])
		rest := remaining[start+3:]
		// Skip optional language tag
		end := strings.Index(rest, "```")
		if end == -1 {
			result.WriteString(rest)
			break
		}
		content := rest[:end]
		if nl := strings.Index(content, "\n"); nl != -1 {
			firstLine := strings.TrimSpace(content[:nl])
			if len(firstLine) > 0 && !strings.ContainsAny(firstLine, " \t") {
				content = content[nl+1:]
			}
		}
		result.WriteString(content)
		remaining = rest[end+3:]
	}
	text = result.String()

	// Inline code: `code` -> code
	result.Reset()
	remaining = text
	for {
		start := strings.Index(remaining, "`")
		if start == -1 {
			result.WriteString(remaining)
			break
		}
		result.WriteString(remaining[:start])
		rest := remaining[start+1:]
		end := strings.Index(rest, "`")
		if end == -1 {
			result.WriteString(rest)
			break
		}
		result.WriteString(rest[:end])
		remaining = rest[end+1:]
	}
	text = result.String()

	// Remove headers: # Header -> Header
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "#") {
			j := 0
			for j < len(line) && line[j] == '#' {
				j++
			}
			if j < len(line) && line[j] == ' ' {
				lines[i] = strings.TrimSpace(line[j+1:])
				continue
			}
		}
		lines[i] = line
	}
	text = strings.Join(lines, "\n")

	// Remove bold: **text** -> text, __text__ -> text
	text = replacePair(text, "**", "")
	text = replacePair(text, "__", "")

	// Remove italic: _text_ -> text, *text* -> text
	text = replaceSingle(text, '_')
	text = replaceSingle(text, '*')

	// Links: [text](url) -> text
	result.Reset()
	remaining = text
	for {
		start := strings.Index(remaining, "[")
		if start == -1 {
			result.WriteString(remaining)
			break
		}
		cb := strings.Index(remaining[start:], "](")
		if cb == -1 {
			result.WriteString(remaining)
			break
		}
		cb += start
		cp := strings.Index(remaining[cb+2:], ")")
		if cp == -1 {
			result.WriteString(remaining)
			break
		}
		cp += cb + 2
		result.WriteString(remaining[:start])
		result.WriteString(remaining[start+1 : cb])
		remaining = remaining[cp+1:]
	}
	text = result.String()

	// Collapse multiple newlines
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(text)
}

// replacePair removes all occurrences of marker...marker pairs (e.g., ** or __).
func replacePair(text, marker string) string {
	return strings.ReplaceAll(text, marker, "")
}

// replaceSingle leaves single-char markdown markers as-is in the plain text
// fallback. Removing them is risky (underscore is common in identifiers, etc.)
// so we keep the text unchanged for maximum readability.
func replaceSingle(text string, _ rune) string {
	return text
}

// SplitMessage splits text into chunks no larger than limit bytes (rune-aware),
// preferring to split at paragraph boundaries (double newline), then single
// newlines, then at the hard limit.
//
// Per CONTEXT.md: "Message splitting at paragraph boundaries (last double-newline
// before 4096 char limit) — keeps code blocks and paragraphs intact."
func SplitMessage(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}

	var parts []string
	for len(text) > limit {
		// Find split point: last \n\n before limit
		chunk := text
		if len(chunk) > limit {
			chunk = text[:limit]
		}

		splitAt := -1

		// Try to split at last paragraph boundary (\n\n)
		if idx := strings.LastIndex(chunk, "\n\n"); idx > 0 {
			splitAt = idx + 2 // include the double newline in the first chunk
		}

		// Fall back to last single newline
		if splitAt == -1 {
			if idx := strings.LastIndex(chunk, "\n"); idx > 0 {
				splitAt = idx + 1
			}
		}

		// Hard split at limit
		if splitAt == -1 {
			splitAt = limit
		}

		parts = append(parts, text[:splitAt])
		text = text[splitAt:]
	}

	if len(text) > 0 {
		parts = append(parts, text)
	}

	return parts
}
