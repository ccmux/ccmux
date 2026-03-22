package state

// TopicBinding maps a Telegram forum topic to a tmux window.
type TopicBinding struct {
	WindowID   string `json:"window_id"`   // e.g. "@12"
	WindowName string `json:"window_name"` // display name
}

// WindowState is per-window metadata written by the hook.
type WindowState struct {
	SessionID  string `json:"session_id"`
	CWD        string `json:"cwd"`
	WindowName string `json:"window_name"`
}

// TrackedSession holds the JSONL monitor read progress for one session.
type TrackedSession struct {
	SessionID      string `json:"session_id"`
	FilePath       string `json:"file_path"`
	LastByteOffset int64  `json:"last_byte_offset"`
}

// BotState is the top-level persisted state document.
type BotState struct {
	// GroupChatID is the Telegram supergroup chat ID (negative number).
	// Stored here as a convenience; also set in config.
	GroupChatID int64 `json:"group_chat_id,omitempty"`

	// TopicBindings: topic_id (string) → TopicBinding
	TopicBindings map[string]TopicBinding `json:"topic_bindings"`

	// WindowStates: window_id → WindowState (from session_map.json, merged at startup)
	WindowStates map[string]WindowState `json:"window_states"`

	// TrackedSessions: session_id → TrackedSession (monitor byte offsets)
	TrackedSessions map[string]TrackedSession `json:"tracked_sessions"`
}

func New() *BotState {
	return &BotState{
		TopicBindings:   make(map[string]TopicBinding),
		WindowStates:    make(map[string]WindowState),
		TrackedSessions: make(map[string]TrackedSession),
	}
}
