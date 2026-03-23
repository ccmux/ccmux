package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ccmux/ccmux/internal/config"
	"github.com/ccmux/ccmux/internal/format"
	"github.com/ccmux/ccmux/internal/monitor"
	"github.com/ccmux/ccmux/internal/state"
	"github.com/ccmux/ccmux/internal/tmux"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/rs/zerolog/log"
)

// Bot is the main Telegram bot instance.
type Bot struct {
	cfg     *config.Config
	state   *state.Manager
	tmux    *tmux.Manager
	monitor *monitor.Monitor
	tg      *telego.Bot
	bh      *th.BotHandler

	// toolMsgIDs: "<chatRef.Key()>:<toolUseID>" → telegram message ID
	// Used to edit tool_use messages in-place when tool_result arrives.
	toolMsgIDs map[string]int
	toolMsgMu  sync.Mutex

	rateLimitSig map[string]string // "<chatRef.Key()>:<windowID>" → pane signature
	rateLimitMu  sync.Mutex
}

func New(cfg *config.Config, stateMgr *state.Manager, tmuxMgr *tmux.Manager, mon *monitor.Monitor) (*Bot, error) {
	tg, err := telego.NewBot(cfg.TelegramBotToken)
	if err != nil {
		return nil, err
	}

	b := &Bot{
		cfg:          cfg,
		state:        stateMgr,
		tmux:         tmuxMgr,
		monitor:      mon,
		tg:           tg,
		toolMsgIDs:   make(map[string]int),
		rateLimitSig: make(map[string]string),
	}
	return b, nil
}

func (b *Bot) Start(ctx context.Context) error {
	// Set up update channel via long polling
	updates, err := b.tg.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return err
	}

	bh, err := th.NewBotHandler(b.tg, updates)
	if err != nil {
		return err
	}
	b.bh = bh

	// Register handlers
	bh.Handle(func(ctx *th.Context, update telego.Update) error {
		b.onMessage(update)
		return nil
	}, th.AnyMessage())
	bh.Handle(func(ctx *th.Context, update telego.Update) error {
		b.onCallback(update)
		return nil
	}, th.AnyCallbackQuery())

	// Start monitoring
	b.monitor.Start()

	// Start delivery goroutine
	go b.deliveryLoop(ctx)
	go b.promptScanLoop(ctx)

	// Start bot handler (blocks until ctx cancelled)
	go bh.Start()

	log.Info().Str("session", b.cfg.TmuxSessionName).Msg("ccmux bot started")

	<-ctx.Done()
	bh.Stop()
	b.monitor.Stop()
	return nil
}

func (b *Bot) onMessage(update telego.Update) {
	msg := update.Message
	if msg == nil {
		return
	}
	ctx := context.Background()

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}

	var fromID int64
	if msg.From != nil {
		fromID = msg.From.ID
	}
	log.Debug().
		Int64("chat_id", msg.Chat.ID).
		Str("chat_type", string(msg.Chat.Type)).
		Int("thread_id", msg.MessageThreadID).
		Int64("from_id", fromID).
		Str("text", text).
		Msg("message received")

	switch {
	case strings.HasPrefix(text, "/new"):
		b.handleNew(ctx, *msg)
	case strings.HasPrefix(text, "/kill"):
		b.handleKill(ctx, *msg)
	case strings.HasPrefix(text, "/sessions"):
		b.handleSessions(ctx, *msg)
	case strings.HasPrefix(text, "/bind"):
		b.handleBind(ctx, *msg)
	case strings.HasPrefix(text, "/cd"):
		b.handleCd(ctx, *msg)
	case strings.HasPrefix(text, "/esc"):
		b.handleEsc(ctx, *msg)
	case strings.HasPrefix(text, "/screenshot"):
		b.handleScreenshot(ctx, *msg)
	default:
		b.handleText(ctx, *msg)
	}
}

func (b *Bot) onCallback(update telego.Update) {
	if update.CallbackQuery == nil {
		return
	}
	b.handleCallback(context.Background(), *update.CallbackQuery)
}

// deliveryLoop reads ParsedEntry values from the monitor and sends them to Telegram.
func (b *Bot) deliveryLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-b.monitor.Entries:
			b.deliverEntry(ctx, entry)
		}
	}
}

// promptScanLoop periodically checks active panes for prompts that may not appear in JSONL.
func (b *Bot) promptScanLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.scanRateLimitPrompts(ctx)
		}
	}
}

func (b *Bot) scanRateLimitPrompts(ctx context.Context) {
	for _, binding := range b.state.AllBindings() {
		if binding.WindowID == "" {
			continue
		}
		b.maybeShowRateLimitPrompt(ctx, binding.WindowID, binding.ChatRef)
	}
}

func (b *Bot) deliverEntry(ctx context.Context, entry monitor.ParsedEntry) {
	// Resolve session → ChatRef
	ref, ok := b.state.FindConvForSession(entry.SessionID)
	if !ok {
		return
	}

	// Quiet mode: skip tool calls but still show approvals
	if b.cfg.QuietMode && (entry.Kind == monitor.KindToolUse || entry.Kind == monitor.KindToolResult) {
		if entry.Kind == monitor.KindToolUse && approvalTools[entry.ToolName] {
			binding, ok2 := b.state.GetBinding(ref)
			if ok2 {
				time.AfterFunc(300*time.Millisecond, func() {
					b.maybeShowApproval(ctx, binding.WindowID, ref)
				})
			}
		}
		return
	}

	switch entry.Kind {
	case monitor.KindText:
		b.sendToRef(ctx, ref, renderEntry(entry)) //nolint
		if binding, ok := b.state.GetBinding(ref); ok {
			time.AfterFunc(300*time.Millisecond, func() {
				b.maybeShowRateLimitPrompt(ctx, binding.WindowID, ref)
			})
		}

	case monitor.KindThinking:
		b.sendToRef(ctx, ref, format.ThinkingBlock(renderEntry(entry))) //nolint

	case monitor.KindToolUse:
		if !b.cfg.ShowToolCalls && !approvalTools[entry.ToolName] && !interactiveTools[entry.ToolName] {
			return
		}
		msgID, err := b.sendToRef(ctx, ref, renderEntry(entry))
		if err == nil && entry.ToolUseID != "" {
			b.storeToolMsgID(ref, entry.ToolUseID, msgID)
		}
		if approvalTools[entry.ToolName] {
			binding, ok2 := b.state.GetBinding(ref)
			if ok2 {
				time.AfterFunc(300*time.Millisecond, func() {
					b.maybeShowApproval(ctx, binding.WindowID, ref)
				})
			}
		}
		if interactiveTools[entry.ToolName] {
			binding, ok2 := b.state.GetBinding(ref)
			if ok2 {
				time.AfterFunc(300*time.Millisecond, func() {
					b.maybeShowInteractive(ctx, binding.WindowID, ref)
				})
			}
		}

	case monitor.KindToolResult:
		if msgID, ok := b.consumeToolMsgID(ref, entry.ToolUseID); ok {
			b.editTopicMsg(ctx, ref, msgID, renderEntry(entry))
		} else {
			b.sendToRef(ctx, ref, renderEntry(entry)) //nolint
		}
		if binding, ok := b.state.GetBinding(ref); ok {
			time.AfterFunc(300*time.Millisecond, func() {
				b.maybeShowRateLimitPrompt(ctx, binding.WindowID, ref)
			})
		}
	}
}

func (b *Bot) storeToolMsgID(ref state.ChatRef, toolUseID string, msgID int) {
	b.toolMsgMu.Lock()
	b.toolMsgIDs[toolMsgKey(ref, toolUseID)] = msgID
	b.toolMsgMu.Unlock()
}

func (b *Bot) consumeToolMsgID(ref state.ChatRef, toolUseID string) (int, bool) {
	b.toolMsgMu.Lock()
	defer b.toolMsgMu.Unlock()
	key := toolMsgKey(ref, toolUseID)
	id, ok := b.toolMsgIDs[key]
	if ok {
		delete(b.toolMsgIDs, key)
	}
	return id, ok
}

func toolMsgKey(ref state.ChatRef, toolUseID string) string {
	return fmt.Sprintf("%s:%s", ref.Key(), toolUseID)
}
