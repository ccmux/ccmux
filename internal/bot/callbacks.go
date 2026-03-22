package bot

import (
	"context"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"github.com/rs/zerolog/log"
)

// Approval tools that require user confirmation.
var approvalTools = map[string]bool{
	"mcp__permissions__request": true,
	"Bash":                      true,
}

// Interactive tools that need navigation UI.
var interactiveTools = map[string]bool{
	"AskUserQuestion": true,
	"ExitPlanMode":    true,
}

func (b *Bot) handleCallback(ctx context.Context, query telego.CallbackQuery) {
	// Always answer immediately to clear the "loading" state on the button
	b.tg.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{ //nolint
		CallbackQueryID: query.ID,
	})

	data := query.Data
	parts := strings.SplitN(data, ":", 3)
	if len(parts) < 3 {
		return
	}

	prefix, action, windowID := parts[0], parts[1], parts[2]
	switch prefix {
	case cbApproval:
		b.handleApproval(ctx, query, action, windowID)
	case cbInteractive:
		b.handleInteractiveKey(ctx, query, action, windowID)
	}
}

func (b *Bot) handleApproval(ctx context.Context, query telego.CallbackQuery, action, windowID string) {
	msgID := callbackMsgID(query)
	var key string
	switch action {
	case "allow":
		key = "1" // Select option 1 (Yes) in Claude's numbered menu
	case "always":
		key = "2" // Select option 2 (Yes, always)
	case "deny":
		if err := b.tmux.SendKey(ctx, windowID, "Escape"); err != nil {
			log.Error().Err(err).Str("window", windowID).Msg("sending Escape for deny")
		}
		b.removeKeyboard(ctx, msgID)
		return
	default:
		return
	}

	if err := b.tmux.SendText(ctx, windowID, key); err != nil {
		log.Error().Err(err).Str("window", windowID).Msg("sending approval key")
	}
	b.removeKeyboard(ctx, msgID)
}

func (b *Bot) handleInteractiveKey(ctx context.Context, query telego.CallbackQuery, action, windowID string) {
	msgID := callbackMsgID(query)
	keyMap := map[string]string{
		"up":    "Up",
		"down":  "Down",
		"left":  "Left",
		"right": "Right",
		"enter": "Enter",
		"esc":   "Escape",
		"space": "Space",
		"tab":   "Tab",
	}

	if action == "refresh" {
		content, err := b.tmux.CapturePlain(ctx, windowID)
		if err != nil {
			log.Warn().Err(err).Msg("capturing pane for refresh")
			return
		}
		kb := interactiveKeyboard(windowID)
		_, err = b.tg.EditMessageText(ctx, &telego.EditMessageTextParams{
			ChatID:      telego.ChatID{ID: b.cfg.GroupChatID},
			MessageID:   msgID,
			Text:        "<pre><code>" + content + "</code></pre>",
			ParseMode:   telego.ModeHTML,
			ReplyMarkup: &kb,
		})
		if err != nil {
			log.Warn().Err(err).Msg("editing interactive message on refresh")
		}
		return
	}

	key, ok := keyMap[action]
	if !ok {
		return
	}
	if err := b.tmux.SendKey(ctx, windowID, key); err != nil {
		log.Error().Err(err).Str("key", key).Msg("sending interactive key")
	}
	time.Sleep(150 * time.Millisecond)
	content, err := b.tmux.CapturePlain(ctx, windowID)
	if err == nil {
		kb := interactiveKeyboard(windowID)
		_, _ = b.tg.EditMessageText(ctx, &telego.EditMessageTextParams{
			ChatID:      telego.ChatID{ID: b.cfg.GroupChatID},
			MessageID:   msgID,
			Text:        "<pre><code>" + content + "</code></pre>",
			ParseMode:   telego.ModeHTML,
			ReplyMarkup: &kb,
		})
	}
}

// callbackMsgID safely extracts the message ID from a callback query.
func callbackMsgID(query telego.CallbackQuery) int {
	if query.Message == nil {
		return 0
	}
	if msg, ok := query.Message.(*telego.Message); ok {
		return msg.MessageID
	}
	return query.Message.GetMessageID()
}

// maybeShowApproval checks the pane for a permission prompt and sends the keyboard.
func (b *Bot) maybeShowApproval(ctx context.Context, windowID string, topicID int) {
	content, err := b.tmux.CapturePlain(ctx, windowID)
	if err != nil {
		return
	}
	if !isPermissionPrompt(content) {
		return
	}
	kb := approvalKeyboard(windowID)
	b.sendToTopicWithKeyboard(ctx, topicID, //nolint
		"<b>⚠️ Approval required</b>\n<pre><code>"+content+"</code></pre>", kb)
}

// maybeShowInteractive checks the pane for an interactive UI and sends the navigation keyboard.
func (b *Bot) maybeShowInteractive(ctx context.Context, windowID string, topicID int) {
	content, err := b.tmux.CapturePlain(ctx, windowID)
	if err != nil {
		return
	}
	kb := interactiveKeyboard(windowID)
	b.sendToTopicWithKeyboard(ctx, topicID, //nolint
		"<pre><code>"+content+"</code></pre>", kb)
}

func isPermissionPrompt(pane string) bool {
	patterns := []string{
		"Do you want to proceed",
		"Do you want to make this edit",
		"This command requires approval",
		"Bash command",
		"Esc to cancel",
		"Allow once",
		"Allow always",
	}
	for _, p := range patterns {
		if strings.Contains(pane, p) {
			return true
		}
	}
	return false
}
