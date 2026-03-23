package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/ccmux/ccmux/internal/bot"
	"github.com/ccmux/ccmux/internal/config"
	"github.com/ccmux/ccmux/internal/monitor"
	"github.com/ccmux/ccmux/internal/state"
	"github.com/ccmux/ccmux/internal/tmux"
)

const (
	colorLeft  = "\033[1;38;2;213;70;70m"
	colorRight = "\033[1;38;2;55;90;150m"
	colorReset = "\033[0m"

	banner = "\n" +
		colorLeft + " ██████╗ ██████╗" + colorRight + "███╗   ███╗██╗   ██╗██╗  ██╗" + colorReset + "\n" +
		colorLeft + "██╔════╝██╔════╝" + colorRight + "████╗ ████║██║   ██║╚██╗██╔╝" + colorReset + "\n" +
		colorLeft + "██║     ██║     " + colorRight + "██╔████╔██║██║   ██║ ╚███╔╝ " + colorReset + "\n" +
		colorLeft + "██║     ██║     " + colorRight + "██║╚██╔╝██║██║   ██║ ██╔██╗ " + colorReset + "\n" +
		colorLeft + "╚██████╗╚██████╗" + colorRight + "██║ ╚═╝ ██║╚██████╔╝██╔╝ ██╗" + colorReset + "\n" +
		colorLeft + " ╚═════╝ ╚═════╝" + colorRight + "╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═╝" + colorReset + "\n"
)

func newGatewayCommand() *cobra.Command {
	var debug bool

	cmd := &cobra.Command{
		Use:     "gateway",
		Aliases: []string{"g"},
		Short:   "Start the ccmux gateway",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Print(banner)
			if debug {
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			}
			return serve()
		},
	}

	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")

	return cmd
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

	stateMgr, err := state.NewManager(cfg.StateFile)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if err := stateMgr.LoadSessionMap(cfg.SessionMapFile, cfg.TmuxSessionName); err != nil {
		log.Warn().Err(err).Msg("loading session map (continuing without it)")
	}

	tmuxMgr := tmux.New(cfg.TmuxSessionName, cfg.ClaudeCommand)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := tmuxMgr.GetOrCreateSession(ctx); err != nil {
		return fmt.Errorf("setting up tmux session: %w", err)
	}

	resolveStaleWindowIDs(ctx, stateMgr, tmuxMgr)

	mon := monitor.New(monitor.MonitorConfig{
		PollInterval:   cfg.PollInterval,
		SessionMapFile: cfg.SessionMapFile,
		TmuxSession:    cfg.TmuxSessionName,
	}, stateMgr)

	b, err := bot.New(cfg, stateMgr, tmuxMgr, mon)
	if err != nil {
		return fmt.Errorf("creating bot: %w", err)
	}

	return b.Start(ctx)
}

func resolveStaleWindowIDs(ctx context.Context, stateMgr *state.Manager, tmuxMgr *tmux.Manager) {
	liveWindows, err := tmuxMgr.ListWindows(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("listing windows for stale ID resolution")
		return
	}

	liveByID := make(map[string]bool)
	liveByName := make(map[string]string)
	for _, w := range liveWindows {
		liveByID[w.ID] = true
		liveByName[w.Name] = w.ID
	}

	bindings := stateMgr.AllBindings()
	for _, binding := range bindings {
		if liveByID[binding.WindowID] {
			continue
		}
		ref := binding.ChatRef
		if newID, ok := liveByName[binding.WindowName]; ok {
			log.Info().Str("old", binding.WindowID).Str("new", newID).Msg("remapped stale window ID")
			if ref.ChatID != 0 {
				stateMgr.SetBinding(ref, state.ConvBinding{
					ChatRef:    ref,
					WindowID:   newID,
					WindowName: binding.WindowName,
				})
			}
		} else {
			log.Warn().Str("window", binding.WindowID).Str("name", binding.WindowName).
				Msg("window no longer exists, removing binding")
			if ref.ChatID != 0 {
				stateMgr.DeleteBinding(ref)
			}
		}
	}

	if err := stateMgr.Save(); err != nil {
		log.Warn().Err(err).Msg("saving state after stale ID resolution")
	}
}
