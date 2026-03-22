package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/rs/zerolog/log"
	"github.com/ccmux/ccmux/internal/config"
	"github.com/ccmux/ccmux/internal/format"
	"github.com/ccmux/ccmux/internal/monitor"
	"github.com/ccmux/ccmux/internal/state"
	"github.com/ccmux/ccmux/internal/tmux"
)

// Bot is the main Telegram bot instance.
type Bot struct {
	cfg     *config.Config
	state   *state.Manager
	tmux    *tmux.Manager
	monitor *monitor.Monitor
	tg      *telego.Bot
	bh      *th.BotHandler

	// toolMsgIDs: "<topicID>:<toolUseID>" → telegram message ID
	// Used to edit tool_use messages in-place when tool_result arrives.
	toolMsgIDs map[string]int
	toolMsgMu  sync.Mutex
}

func New(cfg *config.Config, stateMgr *state.Manager, tmuxMgr *tmux.Manager, mon *monitor.Monitor) (*Bot, error) {
	tg, err := telego.NewBot(cfg.TelegramBotToken)
	if err != nil {
		return nil, err
	}

	b := &Bot{
		cfg:        cfg,
		state:      stateMgr,
		tmux:       tmuxMgr,
		monitor:    mon,
		tg:         tg,
		toolMsgIDs: make(map[string]int),
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

	// Register command handlers
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

	// Start bot handler (blocks until ctx cancelled)
	go bh.Start()

	log.Info().Int64("group", b.cfg.GroupChatID).Msg("ccmux bot started")

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

	switch {
	case strings.HasPrefix(text, "/new"):
		b.handleNew(ctx, *msg)
	case strings.HasPrefix(text, "/kill"):
		b.handleKill(ctx, *msg)
	case strings.HasPrefix(text, "/sessions"):
		b.handleSessions(ctx, *msg)
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

func (b *Bot) deliverEntry(ctx context.Context, entry monitor.ParsedEntry) {
	// Resolve session → topic
	topicID, ok := b.state.FindTopicForSession(entry.SessionID)
	if !ok {
		return
	}

	// Quiet mode: skip tool calls but still show approvals
	if b.cfg.QuietMode && (entry.Kind == monitor.KindToolUse || entry.Kind == monitor.KindToolResult) {
		if entry.Kind == monitor.KindToolUse && approvalTools[entry.ToolName] {
			binding, ok2 := b.state.GetBinding(topicID)
			if ok2 {
				time.AfterFunc(300*time.Millisecond, func() {
					b.maybeShowApproval(ctx, binding.WindowID, topicID)
				})
			}
		}
		return
	}

	switch entry.Kind {
	case monitor.KindText:
		b.sendToTopic(ctx, topicID, entry.Text) //nolint

	case monitor.KindThinking:
		b.sendToTopic(ctx, topicID, format.ThinkingBlock(entry.Text)) //nolint

	case monitor.KindToolUse:
		// Send tool_use message, record ID for later editing
		if !b.cfg.ShowToolCalls && !approvalTools[entry.ToolName] && !interactiveTools[entry.ToolName] {
			return
		}
		msgID, err := b.sendToTopic(ctx, topicID, entry.Text)
		if err == nil && entry.ToolUseID != "" {
			b.storeToolMsgID(topicID, entry.ToolUseID, msgID)
		}
		// Check if this needs approval or interactive UI
		if approvalTools[entry.ToolName] {
			binding, ok2 := b.state.GetBinding(topicID)
			if ok2 {
				time.AfterFunc(300*time.Millisecond, func() {
					b.maybeShowApproval(ctx, binding.WindowID, topicID)
				})
			}
		}
		if interactiveTools[entry.ToolName] {
			binding, ok2 := b.state.GetBinding(topicID)
			if ok2 {
				time.AfterFunc(300*time.Millisecond, func() {
					b.maybeShowInteractive(ctx, binding.WindowID, topicID)
				})
			}
		}

	case monitor.KindToolResult:
		// Edit the tool_use message in-place
		if msgID, ok := b.consumeToolMsgID(topicID, entry.ToolUseID); ok {
			b.editTopicMsg(ctx, msgID, entry.Text)
		} else {
			// No pending message to edit — send as new
			b.sendToTopic(ctx, topicID, entry.Text) //nolint
		}
	}
}

func (b *Bot) storeToolMsgID(topicID int, toolUseID string, msgID int) {
	b.toolMsgMu.Lock()
	b.toolMsgIDs[toolMsgKey(topicID, toolUseID)] = msgID
	b.toolMsgMu.Unlock()
}

func (b *Bot) consumeToolMsgID(topicID int, toolUseID string) (int, bool) {
	b.toolMsgMu.Lock()
	defer b.toolMsgMu.Unlock()
	key := toolMsgKey(topicID, toolUseID)
	id, ok := b.toolMsgIDs[key]
	if ok {
		delete(b.toolMsgIDs, key)
	}
	return id, ok
}

func toolMsgKey(topicID int, toolUseID string) string {
	return fmt.Sprintf("%d:%s", topicID, toolUseID)
}
