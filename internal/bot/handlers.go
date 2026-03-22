package bot

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mymmrac/telego"
	"github.com/rs/zerolog/log"
	"github.com/ccmux/ccmux/internal/state"
)

func (b *Bot) handleText(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	topicID := msg.MessageThreadID

	binding, ok := b.state.GetBinding(topicID)
	if !ok {
		b.sendToTopic(ctx, topicID, //nolint
			"No session bound to this topic.\nUse `/new <path>` to start a Claude session.")
		return
	}

	text := msg.Text
	if text == "" && msg.Caption != "" {
		text = msg.Caption
	}
	if text == "" {
		return
	}

	if err := b.tmux.SendText(ctx, binding.WindowID, text); err != nil {
		log.Error().Err(err).Str("window", binding.WindowID).Msg("sending text to tmux")
		b.sendToTopic(ctx, topicID, "❌ Failed to send to Claude: "+err.Error()) //nolint
	}
}

func (b *Bot) handleNew(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	topicID := msg.MessageThreadID

	// Parse path from command
	parts := strings.Fields(msg.Text)
	workDir := ""
	if len(parts) >= 2 {
		workDir = parts[1]
	} else {
		home := "/root"
		workDir = home
	}

	// Resolve to absolute path
	if !filepath.IsAbs(workDir) {
		workDir = filepath.Join("/root", workDir)
	}

	b.sendToTopic(ctx, topicID, fmt.Sprintf("🚀 Starting Claude in `%s`…", workDir)) //nolint

	// Create tmux window with directory name
	name := filepath.Base(workDir)
	if name == "." || name == "/" {
		name = "claude"
	}

	windowID, err := b.tmux.CreateWindow(ctx, name, workDir)
	if err != nil {
		b.sendToTopic(ctx, topicID, "❌ Failed to create session: "+err.Error()) //nolint
		return
	}

	// Bind topic to window
	b.state.SetBinding(topicID, state.TopicBinding{
		WindowID:   windowID,
		WindowName: name,
	})
	if err := b.state.Save(); err != nil {
		log.Error().Err(err).Msg("saving state after /new")
	}

	b.sendToTopic(ctx, topicID, fmt.Sprintf("✅ Session started (window %s)\nType your message to interact with Claude.", windowID)) //nolint
}

func (b *Bot) handleKill(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	topicID := msg.MessageThreadID

	binding, ok := b.state.GetBinding(topicID)
	if !ok {
		b.sendToTopic(ctx, topicID, "No session bound to this topic.") //nolint
		return
	}

	if err := b.tmux.KillWindow(ctx, binding.WindowID); err != nil {
		log.Warn().Err(err).Str("window", binding.WindowID).Msg("killing window")
	}

	b.state.DeleteBinding(topicID)
	b.state.DeleteWindowState(binding.WindowID)
	if err := b.state.Save(); err != nil {
		log.Error().Err(err).Msg("saving state after /kill")
	}

	b.sendToTopic(ctx, topicID, "🛑 Session killed.") //nolint
}

func (b *Bot) handleSessions(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	topicID := msg.MessageThreadID

	bindings := b.state.AllBindings()
	if len(bindings) == 0 {
		b.sendToTopic(ctx, topicID, "No active sessions.") //nolint
		return
	}

	var lines []string
	lines = append(lines, "<b>Active sessions:</b>")
	for tid, binding := range bindings {
		ws, _ := b.state.GetWindowState(binding.WindowID)
		cwd := ws.CWD
		if cwd == "" {
			cwd = "unknown"
		}
		lines = append(lines, fmt.Sprintf("• Topic %s → window %s (<code>%s</code>)",
			tid, binding.WindowID, cwd))
	}
	b.sendToTopic(ctx, topicID, strings.Join(lines, "\n")) //nolint
}

func (b *Bot) handleEsc(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	topicID := msg.MessageThreadID

	binding, ok := b.state.GetBinding(topicID)
	if !ok {
		b.sendToTopic(ctx, topicID, "No session bound to this topic.") //nolint
		return
	}

	if err := b.tmux.SendKey(ctx, binding.WindowID, "Escape"); err != nil {
		log.Error().Err(err).Msg("sending Escape")
	}
}

func (b *Bot) handleScreenshot(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	topicID := msg.MessageThreadID

	binding, ok := b.state.GetBinding(topicID)
	if !ok {
		b.sendToTopic(ctx, topicID, "No session bound to this topic.") //nolint
		return
	}

	content, err := b.tmux.CapturePlain(ctx, binding.WindowID)
	if err != nil {
		b.sendToTopic(ctx, topicID, "❌ Failed to capture pane: "+err.Error()) //nolint
		return
	}

	// Send as a code block (plain text screenshot)
	text := "```\n" + content + "\n```"
	b.sendToTopic(ctx, topicID, text) //nolint
}
