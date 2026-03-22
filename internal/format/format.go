package format

import (
	"strings"
	"unicode/utf8"
)

const maxMessageLen = 4000

// ToHTML converts simple markdown to Telegram HTML.
// Falls back gracefully — any unrecognized input is escaped and returned as plain.
func ToHTML(text string) string {
	var b strings.Builder
	lines := strings.Split(text, "\n")
	inCode := false
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCode {
				b.WriteString("</code></pre>\n")
				inCode = false
			} else {
				lang := strings.TrimPrefix(line, "```")
				lang = strings.TrimSpace(lang)
				b.WriteString("<pre><code")
				if lang != "" {
					b.WriteString(` class="language-`)
					b.WriteString(EscapeHTML(lang))
					b.WriteString(`"`)
				}
				b.WriteString(">")
				inCode = true
			}
			continue
		}
		if inCode {
			b.WriteString(EscapeHTML(line))
			b.WriteString("\n")
			continue
		}
		b.WriteString(formatLine(line))
		b.WriteString("\n")
	}
	if inCode {
		b.WriteString("</code></pre>\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatLine(line string) string {
	// Process inline: **bold**, `code`
	var b strings.Builder
	i := 0
	runes := []rune(line)
	for i < len(runes) {
		// Bold: **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := indexRune(runes, i+2, "**")
			if end >= 0 {
				b.WriteString("<b>")
				b.WriteString(EscapeHTML(string(runes[i+2 : end])))
				b.WriteString("</b>")
				i = end + 2
				continue
			}
		}
		// Italic: *text* (single)
		if runes[i] == '*' {
			end := indexRuneSingle(runes, i+1, '*')
			if end >= 0 {
				b.WriteString("<i>")
				b.WriteString(EscapeHTML(string(runes[i+1 : end])))
				b.WriteString("</i>")
				i = end + 1
				continue
			}
		}
		// Inline code: `code`
		if runes[i] == '`' {
			end := indexRuneSingle(runes, i+1, '`')
			if end >= 0 {
				b.WriteString("<code>")
				b.WriteString(EscapeHTML(string(runes[i+1 : end])))
				b.WriteString("</code>")
				i = end + 1
				continue
			}
		}
		// Escape and emit
		ch := string(runes[i])
		b.WriteString(EscapeHTML(ch))
		i++
	}
	return b.String()
}

func indexRune(runes []rune, start int, seq string) int {
	s := []rune(seq)
	for i := start; i+len(s) <= len(runes); i++ {
		match := true
		for j, r := range s {
			if runes[i+j] != r {
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

func indexRuneSingle(runes []rune, start int, r rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == r {
			return i
		}
	}
	return -1
}

// EscapeHTML escapes characters that break Telegram HTML mode.
func EscapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ThinkingBlock wraps a thinking block in an expandable blockquote.
func ThinkingBlock(text string) string {
	truncated := truncateStr(text, 500)
	return "<blockquote expandable>💭 " + EscapeHTML(truncated) + "</blockquote>"
}

// SplitMessage splits text into chunks of at most maxMessageLen bytes,
// breaking at paragraph boundaries when possible.
func SplitMessage(text string) []string {
	if utf8.RuneCountInString(text) <= maxMessageLen {
		return []string{text}
	}
	var chunks []string
	for len(text) > 0 {
		if utf8.RuneCountInString(text) <= maxMessageLen {
			chunks = append(chunks, text)
			break
		}
		// Try to split at a paragraph boundary
		cut := findSplitPoint(text, maxMessageLen)
		chunks = append(chunks, text[:cut])
		text = strings.TrimLeft(text[cut:], "\n")
	}
	return chunks
}

func findSplitPoint(text string, maxLen int) int {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return len(text)
	}
	limit := string(runes[:maxLen])

	// Find last paragraph break
	if i := strings.LastIndex(limit, "\n\n"); i > maxLen/2 {
		return i
	}
	// Find last newline
	if i := strings.LastIndex(limit, "\n"); i > maxLen/2 {
		return i
	}
	// Hard cut at maxLen
	return len(limit)
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
