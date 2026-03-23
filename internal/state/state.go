package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Manager struct {
	mu    sync.RWMutex
	path  string
	state *BotState
}

func NewManager(path string) (*Manager, error) {
	m := &Manager{path: path}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		m.state = New()
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}
	s := New()
	if err := json.Unmarshal(data, s); err != nil {
		return fmt.Errorf("parsing state: %w", err)
	}
	// Ensure maps are initialized
	if s.ConvBindings == nil {
		s.ConvBindings = make(map[string]ConvBinding)
	}
	if s.WindowStates == nil {
		s.WindowStates = make(map[string]WindowState)
	}
	if s.TrackedSessions == nil {
		s.TrackedSessions = make(map[string]TrackedSession)
	}
	if s.Aliases == nil {
		s.Aliases = make(map[string]string)
	}
	if s.TopicNames == nil {
		s.TopicNames = make(map[string]string)
	}
	m.state = s
	return nil
}

func (m *Manager) Save() error {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.state, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return atomicWrite(m.path, data)
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ccmux-state-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// --- ConvBindings ---

func (m *Manager) GetBinding(ref ChatRef) (ConvBinding, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.state.ConvBindings[ref.Key()]
	return b, ok
}

func (m *Manager) SetBinding(ref ChatRef, b ConvBinding) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.ConvBindings[ref.Key()] = b
}

func (m *Manager) DeleteBinding(ref ChatRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.ConvBindings, ref.Key())
}

func (m *Manager) AllBindings() map[string]ConvBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]ConvBinding, len(m.state.ConvBindings))
	for k, v := range m.state.ConvBindings {
		out[k] = v
	}
	return out
}

// FindConvForWindow returns the ChatRef bound to a tmux window ID.
func (m *Manager) FindConvForWindow(windowID string) (ChatRef, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, b := range m.state.ConvBindings {
		if b.WindowID == windowID {
			return b.ChatRef, true
		}
	}
	return ChatRef{}, false
}

// FindConvForSession resolves session ID -> window ID -> ChatRef.
func (m *Manager) FindConvForSession(sessionID string) (ChatRef, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	windowID := ""
	for wid, ws := range m.state.WindowStates {
		if ws.SessionID == sessionID {
			windowID = wid
			break
		}
	}
	if windowID == "" {
		return ChatRef{}, false
	}
	for _, b := range m.state.ConvBindings {
		if b.WindowID == windowID {
			return b.ChatRef, true
		}
	}
	return ChatRef{}, false
}

// --- Aliases ---

// SetAlias stores a human-readable alias -> window name mapping.
// Returns an error if the alias is already taken.
func (m *Manager) SetAlias(name, windowName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.state.Aliases[name]; ok && existing != windowName {
		return fmt.Errorf("alias '%s' already taken. Choose a different name", name)
	}
	m.state.Aliases[name] = windowName
	return nil
}

func (m *Manager) GetAlias(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.state.Aliases[name]
	return v, ok
}

// --- TopicNames ---

func (m *Manager) SetTopicName(windowName, topicName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.TopicNames[windowName] = topicName
}

func (m *Manager) GetTopicName(windowName string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.state.TopicNames[windowName]
	return v, ok
}

// --- WindowStates ---

func (m *Manager) GetWindowState(windowID string) (WindowState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ws, ok := m.state.WindowStates[windowID]
	return ws, ok
}

func (m *Manager) SetWindowState(windowID string, ws WindowState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.WindowStates[windowID] = ws
}

func (m *Manager) DeleteWindowState(windowID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.WindowStates, windowID)
}

func (m *Manager) AllWindowStates() map[string]WindowState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]WindowState, len(m.state.WindowStates))
	for k, v := range m.state.WindowStates {
		out[k] = v
	}
	return out
}

// --- TrackedSessions ---

func (m *Manager) GetTracked(sessionID string) (TrackedSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ts, ok := m.state.TrackedSessions[sessionID]
	return ts, ok
}

func (m *Manager) SetTracked(sessionID string, ts TrackedSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.TrackedSessions[sessionID] = ts
}

func (m *Manager) AllTracked() map[string]TrackedSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]TrackedSession, len(m.state.TrackedSessions))
	for k, v := range m.state.TrackedSessions {
		out[k] = v
	}
	return out
}

// --- Session map loading ---

// SessionMapEntry matches what the hook binary writes.
type SessionMapEntry struct {
	SessionID  string `json:"session_id"`
	CWD        string `json:"cwd"`
	WindowName string `json:"window_name"`
}

// LoadSessionMap reads session_map.json and merges into WindowStates.
// key format: "<tmux_session>:<window_id>"
func (m *Manager) LoadSessionMap(path, tmuxSession string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var entries map[string]SessionMapEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := tmuxSession + ":"
	for key, entry := range entries {
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		windowID := key[len(prefix):]
		m.state.WindowStates[windowID] = WindowState{
			SessionID:  entry.SessionID,
			CWD:        entry.CWD,
			WindowName: entry.WindowName,
		}
	}
	return nil
}
