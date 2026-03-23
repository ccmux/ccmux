package bot

import (
	"fmt"

	"github.com/ccmux/ccmux/internal/state"
	"github.com/mymmrac/telego"
)

// chatRefFromMessage builds a ChatRef from an incoming Telegram message.
func chatRefFromMessage(msg telego.Message) state.ChatRef {
	ref := state.ChatRef{ChatID: msg.Chat.ID}
	if msg.Chat.Type == telego.ChatTypeSupergroup && msg.Chat.IsForum {
		ref.ThreadID = msg.MessageThreadID
	}
	return ref
}

// chatRefFromCallbackQuery extracts the ChatRef from a callback query's message.
func chatRefFromCallbackQuery(query telego.CallbackQuery) state.ChatRef {
	if query.Message == nil {
		return state.ChatRef{}
	}
	msg, ok := query.Message.(*telego.Message)
	if !ok {
		return state.ChatRef{}
	}
	ref := state.ChatRef{ChatID: msg.Chat.ID}
	if msg.Chat.Type == telego.ChatTypeSupergroup && msg.Chat.IsForum {
		ref.ThreadID = msg.MessageThreadID
	}
	return ref
}

// windowNameFromRef returns the stable, ID-based tmux window name for a ChatRef.
//
//	DM:          U<userID>             (Chat.ID is positive for DMs)
//	Plain group: G<abs(chatID)>
//	Forum topic: G<abs(chatID)>_<threadID>
func windowNameFromRef(ref state.ChatRef) string {
	if ref.ChatID > 0 {
		// Private chat — Chat.ID equals the user's ID
		return fmt.Sprintf("U%d", ref.ChatID)
	}
	absID := -ref.ChatID
	if ref.ThreadID != 0 {
		return fmt.Sprintf("G%d_%d", absID, ref.ThreadID)
	}
	return fmt.Sprintf("G%d", absID)
}
