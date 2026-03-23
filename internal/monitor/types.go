package monitor

// EntryKind classifies a parsed JSONL entry for the bot.
type EntryKind string

const (
	KindText       EntryKind = "text"
	KindThinking   EntryKind = "thinking"
	KindToolUse    EntryKind = "tool_use"
	KindToolResult EntryKind = "tool_result"
	KindDone       EntryKind = "done" // Claude finished (Stop hook or transcript end)
)

// ParsedEntry is a single display-ready message from the monitor.
type ParsedEntry struct {
	SessionID string
	Kind      EntryKind
	Text      string // formatted display text
	ToolUseID string // non-empty for tool_use and tool_result
	ToolName  string // e.g. "Read", "Bash", "AskUserQuestion"
	Role      string // "user" or "assistant"
}

// PendingTool holds tool_use info until the matching tool_result arrives.
type PendingTool struct {
	ToolName string
	Summary  string // formatted display e.g. "**Read** `/path/to/file`"
}

// SessionMapEntry matches what the hook binary writes.
type SessionMapEntry struct {
	SessionID  string `json:"session_id"`
	CWD        string `json:"cwd"`
	WindowName string `json:"window_name"`
}
