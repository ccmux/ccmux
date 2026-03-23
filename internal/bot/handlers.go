package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ccmux/ccmux/internal/state"
	"github.com/mymmrac/telego"
	"github.com/rs/zerolog/log"
)

func (b *Bot) handleText(ctx context.Context, msg telego.Message) {
	if msg.From == nil {
		log.Warn().Int64("chat_id", msg.Chat.ID).Msg("handleText: msg.From is nil, skipping")
		return
	}
	if !b.cfg.IsAllowed(msg.From.ID) {
		log.Warn().Int64("from_id", msg.From.ID).Msg("handleText: user not allowed")
		return
	}
	ref := chatRefFromMessage(msg)

	text := msg.Text
	if text == "" && msg.Caption != "" {
		text = msg.Caption
	}
	if text == "" {
		return
	}

	binding, ok := b.state.GetBinding(ref)
	log.Debug().Str("ref", ref.Key()).Bool("has_binding", ok).Str("window_id", binding.WindowID).Msg("handleText: binding lookup")
	if !ok {
		// Auto-start: create window and start Claude in $HOME
		home, _ := os.UserHomeDir()
		windowName := windowNameFromRef(ref)
		windowID, err := b.tmux.CreateWindow(ctx, windowName, home)
		if err != nil {
			b.sendToRef(ctx, ref, "❌ Failed to create session: "+err.Error()) //nolint
			return
		}
		binding = state.ConvBinding{
			ChatRef:    ref,
			WindowID:   windowID,
			WindowName: windowName,
		}
		b.state.SetBinding(ref, binding)
		if err := b.state.Save(); err != nil {
			log.Error().Err(err).Msg("saving state after auto-start")
		}
		// Give Claude time to initialise
		time.Sleep(2 * time.Second)
	}

	log.Debug().Str("window_id", binding.WindowID).Str("text", text).Msg("handleText: sending to tmux")
	if err := b.tmux.SendText(ctx, binding.WindowID, text); err != nil {
		log.Error().Err(err).Str("window", binding.WindowID).Msg("sending text to tmux")
		b.sendToRef(ctx, ref, "❌ Failed to send to Claude: "+err.Error()) //nolint
	}
}

func (b *Bot) handleNew(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	ref := chatRefFromMessage(msg)

	// Kill existing window if any
	if binding, ok := b.state.GetBinding(ref); ok {
		if err := b.tmux.KillWindow(ctx, binding.WindowID); err != nil {
			log.Warn().Err(err).Str("window", binding.WindowID).Msg("killing window for /new")
		}
		b.state.DeleteWindowState(binding.WindowID)
		b.state.DeleteBinding(ref)
	}

	home, _ := os.UserHomeDir()
	windowName := windowNameFromRef(ref)
	windowID, err := b.tmux.CreateWindow(ctx, windowName, home)
	if err != nil {
		b.sendToRef(ctx, ref, "❌ Failed to create session: "+err.Error()) //nolint
		return
	}

	b.state.SetBinding(ref, state.ConvBinding{
		ChatRef:    ref,
		WindowID:   windowID,
		WindowName: windowName,
	})
	if err := b.state.Save(); err != nil {
		log.Error().Err(err).Msg("saving state after /new")
	}

	b.sendToRef(ctx, ref, fmt.Sprintf("✅ Session started (window %s). Type your message to interact with Claude.", windowID)) //nolint
}

func (b *Bot) handleKill(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	ref := chatRefFromMessage(msg)

	binding, ok := b.state.GetBinding(ref)
	if !ok {
		b.sendToRef(ctx, ref, "No session bound to this chat.") //nolint
		return
	}

	if err := b.tmux.KillWindow(ctx, binding.WindowID); err != nil {
		log.Warn().Err(err).Str("window", binding.WindowID).Msg("killing window")
	}

	b.state.DeleteBinding(ref)
	b.state.DeleteWindowState(binding.WindowID)
	if err := b.state.Save(); err != nil {
		log.Error().Err(err).Msg("saving state after /kill")
	}

	b.sendToRef(ctx, ref, "🛑 Session killed.") //nolint
}

func (b *Bot) handleSessions(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	ref := chatRefFromMessage(msg)

	bindings := b.state.AllBindings()
	if len(bindings) == 0 {
		b.sendToRef(ctx, ref, "No active sessions.") //nolint
		return
	}

	var lines []string
	lines = append(lines, "<b>Active sessions:</b>")
	for _, binding := range bindings {
		ws, _ := b.state.GetWindowState(binding.WindowID)
		cwd := ws.CWD
		if cwd == "" {
			cwd = "unknown"
		}
		lines = append(lines, fmt.Sprintf("• %s → window %s (<code>%s</code>)",
			binding.WindowName, binding.WindowID, cwd))
	}
	b.sendToRef(ctx, ref, strings.Join(lines, "\n")) //nolint
}

func (b *Bot) handleBind(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	ref := chatRefFromMessage(msg)

	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		b.sendToRef(ctx, ref, "Usage: /bind &lt;name&gt;") //nolint
		return
	}
	name := parts[1]

	binding, ok := b.state.GetBinding(ref)
	if !ok {
		b.sendToRef(ctx, ref, "No session bound to this chat. Send a message first to auto-start.") //nolint
		return
	}

	if err := b.state.SetAlias(name, binding.WindowName); err != nil {
		b.sendToRef(ctx, ref, "❌ "+err.Error()) //nolint
		return
	}
	if err := b.state.Save(); err != nil {
		log.Error().Err(err).Msg("saving state after /bind")
	}

	b.sendToRef(ctx, ref, fmt.Sprintf("✅ Alias <code>%s</code> set for this chat.", name)) //nolint
}

func (b *Bot) handleCd(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	ref := chatRefFromMessage(msg)

	parts := strings.Fields(msg.Text)
	if len(parts) < 2 {
		b.sendToRef(ctx, ref, "Usage: /cd &lt;path&gt;") //nolint
		return
	}
	workDir := parts[1]
	if !filepath.IsAbs(workDir) {
		home, _ := os.UserHomeDir()
		workDir = filepath.Join(home, workDir)
	}

	// Kill existing window if any
	if binding, ok := b.state.GetBinding(ref); ok {
		if err := b.tmux.KillWindow(ctx, binding.WindowID); err != nil {
			log.Warn().Err(err).Str("window", binding.WindowID).Msg("killing window for /cd")
		}
		b.state.DeleteWindowState(binding.WindowID)
		b.state.DeleteBinding(ref)
	}

	windowName := windowNameFromRef(ref)
	windowID, err := b.tmux.CreateWindow(ctx, windowName, workDir)
	if err != nil {
		b.sendToRef(ctx, ref, "❌ Failed to create session: "+err.Error()) //nolint
		return
	}

	b.state.SetBinding(ref, state.ConvBinding{
		ChatRef:    ref,
		WindowID:   windowID,
		WindowName: windowName,
	})
	if err := b.state.Save(); err != nil {
		log.Error().Err(err).Msg("saving state after /cd")
	}

	b.sendToRef(ctx, ref, fmt.Sprintf("🔄 Restarted Claude in <code>%s</code>", workDir)) //nolint
}

func (b *Bot) handleEsc(ctx context.Context, msg telego.Message) {
	if !b.cfg.IsAllowed(msg.From.ID) {
		return
	}
	ref := chatRefFromMessage(msg)

	binding, ok := b.state.GetBinding(ref)
	if !ok {
		b.sendToRef(ctx, ref, "No session bound to this chat.") //nolint
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
	ref := chatRefFromMessage(msg)

	binding, ok := b.state.GetBinding(ref)
	if !ok {
		b.sendToRef(ctx, ref, "No session bound to this chat.") //nolint
		return
	}

	content, err := b.tmux.CapturePlain(ctx, binding.WindowID)
	if err != nil {
		b.sendToRef(ctx, ref, "❌ Failed to capture pane: "+err.Error()) //nolint
		return
	}

	b.sendToRef(ctx, ref, "```\n"+content+"\n```") //nolint
}
