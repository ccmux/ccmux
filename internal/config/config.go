package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Required
	TelegramBotToken string
	AllowedUserIDs   []int64
	GroupChatID      int64

	// Tmux
	TmuxSessionName string
	ClaudeCommand   string

	// Paths
	StateDir          string
	StateFile         string
	SessionMapFile    string
	ClaudeProjectsDir string

	// Behavior
	PollInterval  time.Duration
	QuietMode     bool
	ShowToolCalls bool
}

func Load() (*Config, error) {
	home, _ := os.UserHomeDir()
	stateDir := filepath.Join(home, ".ccmux")
	if v := os.Getenv("CCMUX_DIR"); v != "" {
		stateDir = v
	}

	// Load .env file if present (ignore error if missing)
	_ = godotenv.Load(filepath.Join(stateDir, ".env"))

	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TmuxSessionName:  getEnvOr("TMUX_SESSION_NAME", "ccmux"),
		ClaudeCommand:    getEnvOr("CLAUDE_COMMAND", "claude"),
		StateDir:         stateDir,
		PollInterval:     getDurationOr("POLL_INTERVAL", 2*time.Second),
		QuietMode:        getBool("CCMUX_QUIET_MODE"),
		ShowToolCalls:    getBoolOr("CCMUX_SHOW_TOOL_CALLS", true),
	}

	cfg.StateFile = filepath.Join(stateDir, "state.json")
	cfg.SessionMapFile = filepath.Join(stateDir, "session_map.json")

	claudeConfigDir := getEnvOr("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	cfg.ClaudeProjectsDir = filepath.Join(claudeConfigDir, "projects")

	// Parse required fields
	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	if v := os.Getenv("TELEGRAM_GROUP_ID"); v != "" {
		groupID, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("TELEGRAM_GROUP_ID must be a number, got %q", v)
		}
		cfg.GroupChatID = groupID
	}

	users := os.Getenv("ALLOWED_USERS")
	if users == "" {
		return nil, fmt.Errorf("ALLOWED_USERS is required")
	}
	for _, s := range strings.Split(users, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user ID in ALLOWED_USERS: %q", s)
		}
		cfg.AllowedUserIDs = append(cfg.AllowedUserIDs, id)
	}
	if len(cfg.AllowedUserIDs) == 0 {
		return nil, fmt.Errorf("ALLOWED_USERS must contain at least one user ID")
	}

	// Ensure state dir exists
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("creating state dir: %w", err)
	}

	return cfg, nil
}

func (c *Config) IsAllowed(userID int64) bool {
	for _, id := range c.AllowedUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func getEnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "true" || v == "1" || v == "yes"
}

func getBoolOr(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	v = strings.ToLower(v)
	return v == "true" || v == "1" || v == "yes"
}

func getDurationOr(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
