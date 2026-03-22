package monitor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// jsonlLine is the top-level structure of a Claude JSONL entry.
// Claude uses type "user" | "assistant" with a "message" field directly.
type jsonlLine struct {
	Type    string        `json:"type"`
	Message *jsonlMessage `json:"message,omitempty"`
}

type jsonlMessage struct {
	Role    string           `json:"role"`
	Content []contentBlock   `json:"content"`
}

type contentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	// thinking
	Thinking string         `json:"thinking,omitempty"`
	// tool_use
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

// Parser parses JSONL lines and maintains pending tool state across poll cycles.
type Parser struct {
	pending map[string]PendingTool // tool_use_id → PendingTool
}

func NewParser() *Parser {
	return &Parser{pending: make(map[string]PendingTool)}
}

// ParseLine parses one JSONL line and returns zero or more ParsedEntry values.
// sessionID is passed through for context.
func (p *Parser) ParseLine(sessionID string, line []byte) []ParsedEntry {
	var row jsonlLine
	if err := json.Unmarshal(line, &row); err != nil {
		return nil
	}
	// Only process user/assistant entries that have a message payload
	if (row.Type != "user" && row.Type != "assistant") || row.Message == nil {
		return nil
	}

	msg := row.Message
	var entries []ParsedEntry

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			entries = append(entries, ParsedEntry{
				SessionID: sessionID,
				Kind:      KindText,
				Text:      text,
				Role:      msg.Role,
			})

		case "thinking":
			text := strings.TrimSpace(block.Thinking)
			if text == "" {
				continue
			}
			entries = append(entries, ParsedEntry{
				SessionID: sessionID,
				Kind:      KindThinking,
				Text:      text,
				Role:      msg.Role,
			})

		case "tool_use":
			summary := formatToolUse(block.Name, block.Input)
			p.pending[block.ID] = PendingTool{
				ToolName: block.Name,
				Summary:  summary,
			}
			entries = append(entries, ParsedEntry{
				SessionID: sessionID,
				Kind:      KindToolUse,
				Text:      summary,
				ToolUseID: block.ID,
				ToolName:  block.Name,
				Role:      msg.Role,
			})

		case "tool_result":
			toolUseID := block.ToolUseID
			result := extractToolResult(block.Content, block.IsError)
			pending, hasPending := p.pending[toolUseID]
			if hasPending {
				delete(p.pending, toolUseID)
			}
			toolName := pending.ToolName

			var displayText string
			if block.IsError {
				displayText = "❌ " + pending.Summary + "\n" + truncate(result, 300)
			} else {
				displayText = "✓ " + pending.Summary + "\n" + truncate(result, 200)
			}

			entries = append(entries, ParsedEntry{
				SessionID: sessionID,
				Kind:      KindToolResult,
				Text:      displayText,
				ToolUseID: toolUseID,
				ToolName:  toolName,
				Role:      msg.Role,
			})
		}
	}
	return entries
}

func formatToolUse(name string, input json.RawMessage) string {
	if len(input) == 0 {
		return fmt.Sprintf("⚙️ **%s**", name)
	}
	var args map[string]any
	if err := json.Unmarshal(input, &args); err != nil {
		return fmt.Sprintf("⚙️ **%s**", name)
	}

	// Extract the most relevant argument for display
	switch name {
	case "Read", "Write", "Edit", "MultiEdit":
		if p, ok := args["file_path"].(string); ok {
			return fmt.Sprintf("⚙️ **%s** `%s`", name, shortenPath(p))
		}
	case "Bash":
		if cmd, ok := args["command"].(string); ok {
			return fmt.Sprintf("⚙️ **Bash** `%s`", truncate(cmd, 80))
		}
	case "Glob":
		if pat, ok := args["pattern"].(string); ok {
			return fmt.Sprintf("⚙️ **Glob** `%s`", pat)
		}
	case "Grep":
		if pat, ok := args["pattern"].(string); ok {
			return fmt.Sprintf("⚙️ **Grep** `%s`", truncate(pat, 60))
		}
	case "AskUserQuestion":
		if q, ok := args["question"].(string); ok {
			return fmt.Sprintf("❓ %s", truncate(q, 200))
		}
	}
	return fmt.Sprintf("⚙️ **%s**", name)
}

func extractToolResult(content json.RawMessage, isError bool) string {
	if len(content) == 0 {
		return ""
	}
	// content can be a string or array of content blocks
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(content)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func shortenPath(p string) string {
	// Show last 2 path components
	parts := strings.Split(strings.TrimRight(p, "/"), "/")
	if len(parts) <= 2 {
		return p
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}
