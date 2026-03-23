package bot

import (
	"fmt"

	"github.com/mymmrac/telego"
)

// Callback data prefixes (all must fit in 64 bytes total)
const (
	cbApproval    = "ap"
	cbInteractive = "ui"
	cbRateLimit   = "rl"
)

func approvalKeyboard(windowID string) telego.InlineKeyboardMarkup {
	return telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				btn("✅ Allow", fmt.Sprintf("%s:allow:%s", cbApproval, windowID)),
				btn("🔒 Always", fmt.Sprintf("%s:always:%s", cbApproval, windowID)),
				btn("❌ Deny", fmt.Sprintf("%s:deny:%s", cbApproval, windowID)),
			},
		},
	}
}

func interactiveKeyboard(windowID string) telego.InlineKeyboardMarkup {
	return telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				btn("↑", fmt.Sprintf("%s:up:%s", cbInteractive, windowID)),
				btn("↓", fmt.Sprintf("%s:down:%s", cbInteractive, windowID)),
				btn("←", fmt.Sprintf("%s:left:%s", cbInteractive, windowID)),
				btn("→", fmt.Sprintf("%s:right:%s", cbInteractive, windowID)),
			},
			{
				btn("Space", fmt.Sprintf("%s:space:%s", cbInteractive, windowID)),
				btn("Tab", fmt.Sprintf("%s:tab:%s", cbInteractive, windowID)),
				btn("Enter ↩", fmt.Sprintf("%s:enter:%s", cbInteractive, windowID)),
				btn("Esc", fmt.Sprintf("%s:esc:%s", cbInteractive, windowID)),
			},
			{
				btn("↻ Refresh", fmt.Sprintf("%s:refresh:%s", cbInteractive, windowID)),
			},
		},
	}
}

type rateLimitOption struct {
	Number string
	Label  string
}

func rateLimitKeyboard(windowID string, options []rateLimitOption) telego.InlineKeyboardMarkup {
	rows := make([][]telego.InlineKeyboardButton, 0, len(options))
	for _, opt := range options {
		rows = append(rows, []telego.InlineKeyboardButton{
			btn(opt.Label, fmt.Sprintf("%s:%s:%s", cbRateLimit, opt.Number, windowID)),
		})
	}
	return telego.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func btn(text, data string) telego.InlineKeyboardButton {
	return telego.InlineKeyboardButton{Text: text, CallbackData: data}
}
