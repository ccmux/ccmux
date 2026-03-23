package state

import "fmt"

// ChatRef uniquely identifies a conversation: a Telegram chat + optional thread.
// ThreadID is 0 for DMs and plain groups.
type ChatRef struct {
	ChatID   int64 `json:"chat_id"`
	ThreadID int   `json:"thread_id"`
}

// Key returns a stable string key for use in maps.
func (r ChatRef) Key() string {
	return fmt.Sprintf("%d/%d", r.ChatID, r.ThreadID)
}

// ConvBinding maps a ChatRef to a tmux window.
type ConvBinding struct {
	ChatRef    ChatRef `json:"chat_ref"`
	WindowID   string  `json:"window_id"`   // e.g. "@12" (tmux internal ID)
	WindowName string  `json:"window_name"` // e.g. "U123456789", "G100123_42"
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
	// ConvBindings: ChatRef.Key() → ConvBinding
	ConvBindings map[string]ConvBinding `json:"conv_bindings"`

	// WindowStates: window_id → WindowState (from session_map.json, merged at startup)
	WindowStates map[string]WindowState `json:"window_states"`

	// TrackedSessions: session_id → TrackedSession (monitor byte offsets)
	TrackedSessions map[string]TrackedSession `json:"tracked_sessions"`

	// Aliases: human-readable name → window name (for ccmux attach)
	Aliases map[string]string `json:"aliases"`

	// TopicNames: window name → Telegram topic name (cached)
	TopicNames map[string]string `json:"topic_names"`
}

func New() *BotState {
	return &BotState{
		ConvBindings:    make(map[string]ConvBinding),
		WindowStates:    make(map[string]WindowState),
		TrackedSessions: make(map[string]TrackedSession),
		Aliases:         make(map[string]string),
		TopicNames:      make(map[string]string),
	}
}
