package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ccmux/ccmux/internal/state"
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

var numberedOptionRe = regexp.MustCompile(`^\s*(\d{1,2})[\)\.\:]\s+(.+?)\s*$`)

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
	ref := chatRefFromCallbackQuery(query)

	switch prefix {
	case cbApproval:
		b.handleApproval(ctx, query, action, windowID, ref)
	case cbInteractive:
		b.handleInteractiveKey(ctx, query, action, windowID, ref)
	case cbRateLimit:
		b.handleRateLimitOption(ctx, query, action, windowID, ref)
	}
}

func (b *Bot) handleApproval(ctx context.Context, query telego.CallbackQuery, action, windowID string, ref state.ChatRef) {
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
		b.removeKeyboard(ctx, ref, msgID)
		return
	default:
		return
	}

	if err := b.tmux.SendText(ctx, windowID, key); err != nil {
		log.Error().Err(err).Str("window", windowID).Msg("sending approval key")
	}
	b.removeKeyboard(ctx, ref, msgID)
}

func (b *Bot) handleInteractiveKey(ctx context.Context, query telego.CallbackQuery, action, windowID string, ref state.ChatRef) {
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
			ChatID:      telego.ChatID{ID: ref.ChatID},
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
			ChatID:      telego.ChatID{ID: ref.ChatID},
			MessageID:   msgID,
			Text:        "<pre><code>" + content + "</code></pre>",
			ParseMode:   telego.ModeHTML,
			ReplyMarkup: &kb,
		})
	}
}

func (b *Bot) handleRateLimitOption(ctx context.Context, query telego.CallbackQuery, action, windowID string, ref state.ChatRef) {
	msgID := callbackMsgID(query)
	if action == "" {
		return
	}
	if err := b.tmux.SendText(ctx, windowID, action); err != nil {
		log.Error().Err(err).Str("window", windowID).Str("option", action).Msg("sending rate-limit option")
		return
	}
	b.removeKeyboard(ctx, ref, msgID)
	b.clearRateLimitPrompt(ref, windowID)
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
func (b *Bot) maybeShowApproval(ctx context.Context, windowID string, ref state.ChatRef) {
	content, err := b.tmux.CapturePlain(ctx, windowID)
	if err != nil {
		return
	}
	if !isPermissionPrompt(content) {
		return
	}
	kb := approvalKeyboard(windowID)
	b.sendToRefWithKeyboard(ctx, ref, //nolint
		"<b>⚠️ Approval required</b>\n<pre><code>"+content+"</code></pre>", kb)
}

// maybeShowInteractive checks the pane for an interactive UI and sends the navigation keyboard.
func (b *Bot) maybeShowInteractive(ctx context.Context, windowID string, ref state.ChatRef) {
	content, err := b.tmux.CapturePlain(ctx, windowID)
	if err != nil {
		return
	}
	kb := interactiveKeyboard(windowID)
	b.sendToRefWithKeyboard(ctx, ref, //nolint
		"<pre><code>"+content+"</code></pre>", kb)
}

func (b *Bot) maybeShowRateLimitPrompt(ctx context.Context, windowID string, ref state.ChatRef) {
	content, err := b.tmux.CapturePlain(ctx, windowID)
	if err != nil {
		return
	}
	promptText, options, sig, ok := parseRateLimitPrompt(content)
	if !ok {
		b.clearRateLimitPrompt(ref, windowID)
		return
	}
	if b.rateLimitPromptSeen(ref, windowID, sig) {
		return
	}

	kb := rateLimitKeyboard(windowID, options)
	b.sendToRefWithKeyboard(ctx, ref, promptText, kb) //nolint
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

func parseRateLimitPrompt(pane string) (string, []rateLimitOption, string, bool) {
	lower := strings.ToLower(pane)
	hasLimitLanguage := strings.Contains(lower, "limit") &&
		(strings.Contains(lower, "reached") ||
			strings.Contains(lower, "hit") ||
			strings.Contains(lower, "usage") ||
			strings.Contains(lower, "rate"))
	if !hasLimitLanguage {
		return "", nil, "", false
	}

	var opts []rateLimitOption
	var messageLines []string
	lines := strings.Split(pane, "\n")
	for _, line := range lines {
		m := numberedOptionRe.FindStringSubmatch(line)
		if len(m) != 3 {
			if isLimitMessageLine(line) {
				messageLines = append(messageLines, strings.TrimSpace(line))
			}
			continue
		}
		num := strings.TrimSpace(m[1])
		label := strings.TrimSpace(m[2])
		if num == "" || label == "" {
			continue
		}
		opts = append(opts, rateLimitOption{
			Number: num,
			Label:  fmt.Sprintf("%s) %s", num, truncateLabel(label, 48)),
		})
	}
	if len(opts) == 0 {
		return "", nil, "", false
	}

	promptText := "⚠️ Claude hit a usage limit. Choose an option:"
	if len(messageLines) > 0 {
		unique := dedupeLines(messageLines, 2)
		promptText = "⚠️ " + strings.Join(unique, "\n")
	}

	return promptText, opts, rateLimitPromptSignature(promptText, opts), true
}

func rateLimitPromptSignature(promptText string, opts []rateLimitOption) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(promptText))
	b.WriteString("|")
	for _, opt := range opts {
		b.WriteString(opt.Number)
		b.WriteString(":")
		b.WriteString(opt.Label)
		b.WriteString(";")
	}
	return b.String()
}

func (b *Bot) rateLimitPromptSeen(ref state.ChatRef, windowID, sig string) bool {
	key := ref.Key() + ":" + windowID
	b.rateLimitMu.Lock()
	defer b.rateLimitMu.Unlock()
	if b.rateLimitSig == nil {
		b.rateLimitSig = make(map[string]string)
	}
	if prev, ok := b.rateLimitSig[key]; ok && prev == sig {
		return true
	}
	b.rateLimitSig[key] = sig
	return false
}

func (b *Bot) clearRateLimitPrompt(ref state.ChatRef, windowID string) {
	key := ref.Key() + ":" + windowID
	b.rateLimitMu.Lock()
	if b.rateLimitSig != nil {
		delete(b.rateLimitSig, key)
	}
	b.rateLimitMu.Unlock()
}

func isLimitMessageLine(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return false
	}
	// Ignore box drawing and generic chrome lines.
	if strings.HasPrefix(s, "╭") || strings.HasPrefix(s, "│") || strings.HasPrefix(s, "╰") {
		return false
	}
	lower := strings.ToLower(s)
	return strings.Contains(lower, "limit") ||
		strings.Contains(lower, "usage") ||
		strings.Contains(lower, "rate") ||
		strings.Contains(lower, "try again") ||
		strings.Contains(lower, "upgrade") ||
		strings.Contains(lower, "plan")
}

func dedupeLines(lines []string, max int) []string {
	seen := make(map[string]bool, len(lines))
	out := make([]string, 0, max)
	for _, line := range lines {
		if seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
		if len(out) >= max {
			break
		}
	}
	return out
}

func truncateLabel(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
