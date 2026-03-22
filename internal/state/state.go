package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

type Manager struct {
	mu       sync.RWMutex
	path     string
	state    *BotState
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
	if s.TopicBindings == nil {
		s.TopicBindings = make(map[string]TopicBinding)
	}
	if s.WindowStates == nil {
		s.WindowStates = make(map[string]WindowState)
	}
	if s.TrackedSessions == nil {
		s.TrackedSessions = make(map[string]TrackedSession)
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

// --- TopicBindings ---

func (m *Manager) GetBinding(topicID int) (TopicBinding, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.state.TopicBindings[strconv.Itoa(topicID)]
	return b, ok
}

func (m *Manager) SetBinding(topicID int, b TopicBinding) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.TopicBindings[strconv.Itoa(topicID)] = b
}

func (m *Manager) DeleteBinding(topicID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.TopicBindings, strconv.Itoa(topicID))
}

func (m *Manager) AllBindings() map[string]TopicBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]TopicBinding, len(m.state.TopicBindings))
	for k, v := range m.state.TopicBindings {
		out[k] = v
	}
	return out
}

// FindTopicForWindow returns the topic ID bound to a given window ID, if any.
func (m *Manager) FindTopicForWindow(windowID string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, b := range m.state.TopicBindings {
		if b.WindowID == windowID {
			id, err := strconv.Atoi(k)
			if err == nil {
				return id, true
			}
		}
	}
	return 0, false
}

// FindTopicForSession resolves session_id → window_id → topic_id.
func (m *Manager) FindTopicForSession(sessionID string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Find window with this session
	windowID := ""
	for wid, ws := range m.state.WindowStates {
		if ws.SessionID == sessionID {
			windowID = wid
			break
		}
	}
	if windowID == "" {
		return 0, false
	}
	for k, b := range m.state.TopicBindings {
		if b.WindowID == windowID {
			id, err := strconv.Atoi(k)
			if err == nil {
				return id, true
			}
		}
	}
	return 0, false
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
