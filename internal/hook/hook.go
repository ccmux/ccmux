package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// HookPayload is the JSON Claude Code sends on stdin for hook events.
type HookPayload struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
}

type sessionMapEntry struct {
	SessionID  string `json:"session_id"`
	CWD        string `json:"cwd"`
	WindowName string `json:"window_name"`
}

// Run processes a hook event from stdin. Called by `ccmux hook`.
func Run() error {
	var payload HookPayload
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		return fmt.Errorf("reading hook payload: %w", err)
	}
	if payload.HookEventName != "SessionStart" {
		// Only handle SessionStart for now
		return nil
	}
	if payload.SessionID == "" {
		return fmt.Errorf("empty session_id in hook payload")
	}

	// Get tmux pane info
	tmuxPane := os.Getenv("TMUX_PANE")
	if tmuxPane == "" {
		return fmt.Errorf("TMUX_PANE not set — hook must run inside tmux")
	}

	out, err := exec.Command("tmux", "display-message", "-t", tmuxPane, "-p",
		"#{session_name}\t#{window_id}\t#{window_name}").Output()
	if err != nil {
		return fmt.Errorf("getting tmux window info: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(parts) < 3 {
		return fmt.Errorf("unexpected tmux output: %q", string(out))
	}
	sessionName, windowID, windowName := parts[0], parts[1], parts[2]
	mapKey := sessionName + ":" + windowID

	// Determine state dir
	stateDir := stateDirectory()
	mapFile := filepath.Join(stateDir, "session_map.json")

	// Load existing map with exclusive file lock
	f, err := os.OpenFile(mapFile, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("opening session_map: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("locking session_map: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	var entries map[string]sessionMapEntry
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&entries); err != nil {
		entries = make(map[string]sessionMapEntry)
	}

	entries[mapKey] = sessionMapEntry{
		SessionID:  payload.SessionID,
		CWD:        payload.CWD,
		WindowName: windowName,
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session_map: %w", err)
	}

	// Truncate and rewrite
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("truncating session_map: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seeking session_map: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing session_map: %w", err)
	}
	return f.Sync()
}

// Install adds the ccmux hook to ~/.claude/settings.json.
func Install(binaryPath string) error {
	settingsFile := claudeSettingsFile()

	data, err := os.ReadFile(settingsFile)
	if os.IsNotExist(err) {
		data = []byte("{}")
	} else if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}

	hookCmd := binaryPath + " hook"
	hookEntry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookCmd,
				"timeout": 5,
			},
		},
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}

	// Check if already installed
	if existing, ok := hooks["SessionStart"]; ok {
		if arr, ok := existing.([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					if hs, ok := m["hooks"].([]any); ok {
						for _, h := range hs {
							if hm, ok := h.(map[string]any); ok {
								if cmd, _ := hm["command"].(string); strings.Contains(cmd, "ccmux hook") {
									fmt.Println("ccmux hook already installed")
									return nil
								}
							}
						}
					}
				}
			}
			hooks["SessionStart"] = append(arr, hookEntry)
		}
	} else {
		hooks["SessionStart"] = []any{hookEntry}
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}

	// Ensure parent dir exists
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0700); err != nil {
		return err
	}

	if err := atomicWrite(settingsFile, out); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	fmt.Printf("ccmux hook installed in %s\n", settingsFile)
	return nil
}

func stateDirectory() string {
	if v := os.Getenv("CCMUX_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ccmux")
}

func claudeSettingsFile() string {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return filepath.Join(v, "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ccmux-hook-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), path)
}
