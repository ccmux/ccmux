package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Manager struct {
	SessionName string
	Command     string // claude command, default "claude"
}

type WindowInfo struct {
	ID   string // e.g. "@12"
	Name string
	CWD  string
}

func New(sessionName, command string) *Manager {
	return &Manager{SessionName: sessionName, Command: command}
}

// GetOrCreateSession ensures the tmux session exists and scrubs sensitive env vars.
func (m *Manager) GetOrCreateSession(ctx context.Context) error {
	// Check if session exists (exit 0 = exists, non-zero = doesn't exist)
	_, err := m.run(ctx, "has-session", "-t", m.SessionName)
	if err == nil {
		// Session already exists
		return nil
	}
	// Create new session with a placeholder window
	if _, err := m.run(ctx, "new-session", "-d", "-s", m.SessionName, "-n", "__main__"); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}
	// Scrub sensitive vars from the session environment
	for _, v := range []string{"TELEGRAM_BOT_TOKEN", "ALLOWED_USERS", "TELEGRAM_GROUP_ID"} {
		_, _ = m.run(ctx, "setenv", "-g", "-u", v)
	}
	return nil
}

// ListWindows returns all windows in the session.
func (m *Manager) ListWindows(ctx context.Context) ([]WindowInfo, error) {
	out, err := m.run(ctx, "list-windows", "-t", m.SessionName,
		"-F", "#{window_id}|#{window_name}|#{pane_current_path}")
	if err != nil {
		return nil, err
	}
	var windows []WindowInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		windows = append(windows, WindowInfo{
			ID:   parts[0],
			Name: parts[1],
			CWD:  parts[2],
		})
	}
	return windows, nil
}

// CreateWindow creates a new tmux window running claude in the given directory.
// Returns the new window ID and name.
func (m *Manager) CreateWindow(ctx context.Context, name, workDir string) (string, error) {
	args := []string{
		"new-window", "-t", m.SessionName,
		"-n", name,
		"-c", workDir,
		"-P", "-F", "#{window_id}",
		m.Command,
	}
	out, err := m.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("creating window: %w", err)
	}
	windowID := strings.TrimSpace(string(out))
	// Prevent tmux from auto-renaming the window
	_, _ = m.run(ctx, "set-window-option", "-t",
		m.SessionName+":"+windowID, "allow-rename", "off")
	return windowID, nil
}

// KillWindow kills a tmux window by ID.
func (m *Manager) KillWindow(ctx context.Context, windowID string) error {
	_, err := m.run(ctx, "kill-window", "-t", m.SessionName+":"+windowID)
	return err
}

// SendText sends text followed by Enter to the window.
func (m *Manager) SendText(ctx context.Context, windowID, text string) error {
	target := m.SessionName + ":" + windowID
	// Send text as literal
	if _, err := m.run(ctx, "send-keys", "-t", target, "-l", text); err != nil {
		return err
	}
	// Small delay then Enter, to avoid TUI race conditions
	time.Sleep(100 * time.Millisecond)
	_, err := m.run(ctx, "send-keys", "-t", target, "Enter", "")
	return err
}

// SendKey sends a single special key (Up, Down, Enter, Escape, Space, Tab).
func (m *Manager) SendKey(ctx context.Context, windowID, key string) error {
	target := m.SessionName + ":" + windowID
	_, err := m.run(ctx, "send-keys", "-t", target, key, "")
	return err
}

// CapturePlain captures the current pane content as plain text.
func (m *Manager) CapturePlain(ctx context.Context, windowID string) (string, error) {
	target := m.SessionName + ":" + windowID
	out, err := m.run(ctx, "capture-pane", "-t", target, "-p")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// WindowExists checks if a window ID is still alive.
func (m *Manager) WindowExists(ctx context.Context, windowID string) bool {
	_, err := m.run(ctx, "has-session", "-t", m.SessionName+":"+windowID)
	return err == nil
}

func (m *Manager) run(ctx context.Context, args ...string) ([]byte, error) {
	// Filter empty trailing args (used as separators)
	var filtered []string
	for i, a := range args {
		if i == len(args)-1 && a == "" {
			break
		}
		filtered = append(filtered, a)
	}
	cmd := exec.CommandContext(ctx, "tmux", filtered...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("tmux %s: %s", args[0], string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return out, nil
}
