package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ccmux/ccmux/internal/bot"
	"github.com/ccmux/ccmux/internal/config"
	"github.com/ccmux/ccmux/internal/hook"
	"github.com/ccmux/ccmux/internal/monitor"
	"github.com/ccmux/ccmux/internal/state"
	"github.com/ccmux/ccmux/internal/tmux"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hook":
			if len(os.Args) > 2 && os.Args[2] == "--install" {
				exe, err := os.Executable()
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
				if err := hook.Install(exe); err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
				return
			}
			if err := hook.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "hook error: %v\n", err)
				os.Exit(1)
			}
			return
		case "version", "--version", "-v":
			fmt.Println("ccmux v0.1.0")
			return
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	if err := serve(); err != nil {
		log.Fatal().Err(err).Msg("ccmux failed")
	}
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	log.Info().
		Str("session", cfg.TmuxSessionName).
		Str("state_dir", cfg.StateDir).
		Msg("starting ccmux")

	// Load state
	stateMgr, err := state.NewManager(cfg.StateFile)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	// Load session map from hook output
	if err := stateMgr.LoadSessionMap(cfg.SessionMapFile, cfg.TmuxSessionName); err != nil {
		log.Warn().Err(err).Msg("loading session map (continuing without it)")
	}

	// Set up tmux
	tmuxMgr := tmux.New(cfg.TmuxSessionName, cfg.ClaudeCommand)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := tmuxMgr.GetOrCreateSession(ctx); err != nil {
		return fmt.Errorf("setting up tmux session: %w", err)
	}

	// Resolve stale window IDs at startup
	resolveStaleWindowIDs(ctx, stateMgr, tmuxMgr)

	// Set up monitor
	mon := monitor.New(monitor.MonitorConfig{
		PollInterval:   cfg.PollInterval,
		SessionMapFile: cfg.SessionMapFile,
		TmuxSession:    cfg.TmuxSessionName,
	}, stateMgr)

	// Set up bot
	b, err := bot.New(cfg, stateMgr, tmuxMgr, mon)
	if err != nil {
		return fmt.Errorf("creating bot: %w", err)
	}

	return b.Start(ctx)
}

// resolveStaleWindowIDs remaps window IDs that may have changed after tmux server restart.
func resolveStaleWindowIDs(ctx context.Context, stateMgr *state.Manager, tmuxMgr *tmux.Manager) {
	liveWindows, err := tmuxMgr.ListWindows(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("listing windows for stale ID resolution")
		return
	}

	liveByID := make(map[string]bool)
	liveByName := make(map[string]string) // name → ID
	for _, w := range liveWindows {
		liveByID[w.ID] = true
		liveByName[w.Name] = w.ID
	}

	bindings := stateMgr.AllBindings()
	for topicStr, binding := range bindings {
		if liveByID[binding.WindowID] {
			continue // still valid
		}
		// Try to find by name
		if newID, ok := liveByName[binding.WindowName]; ok {
			log.Info().Str("old", binding.WindowID).Str("new", newID).Msg("remapped stale window ID")
			topicID := 0
			fmt.Sscanf(topicStr, "%d", &topicID)
			if topicID > 0 {
				stateMgr.SetBinding(topicID, state.TopicBinding{
					WindowID:   newID,
					WindowName: binding.WindowName,
				})
			}
		} else {
			log.Warn().Str("window", binding.WindowID).Str("name", binding.WindowName).
				Msg("window no longer exists, removing binding")
			topicID := 0
			fmt.Sscanf(topicStr, "%d", &topicID)
			if topicID > 0 {
				stateMgr.DeleteBinding(topicID)
			}
		}
	}

	if err := stateMgr.Save(); err != nil {
		log.Warn().Err(err).Msg("saving state after stale ID resolution")
	}
}

func printHelp() {
	fmt.Print(`ccmux - Telegram ↔ Claude Code tmux bridge

Usage:
  ccmux              Start the bot (reads ~/.ccmux/.env)
  ccmux hook         Process a Claude Code hook event (called by Claude)
  ccmux hook --install  Install the hook into ~/.claude/settings.json
  ccmux version      Show version

Telegram commands (in a forum topic):
  /new <path>        Start a Claude session in the given directory
  /kill              Kill the session bound to this topic
  /sessions          List all active sessions
  /esc               Send Escape key to Claude
  /screenshot        Capture current terminal as text

Config (~/.ccmux/.env):
  TELEGRAM_BOT_TOKEN     required
  ALLOWED_USERS          required (comma-separated user IDs)
  TELEGRAM_GROUP_ID      required (supergroup chat ID)
  TMUX_SESSION_NAME      default: ccmux
  CLAUDE_COMMAND         default: claude
  CCMUX_DIR              default: ~/.ccmux
  POLL_INTERVAL          default: 2s
  CCMUX_QUIET_MODE       default: false
  CCMUX_SHOW_TOOL_CALLS  default: true
`)
}
