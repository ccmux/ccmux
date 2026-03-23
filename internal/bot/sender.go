package bot

import (
	"context"
	"strings"
	"time"

	"github.com/ccmux/ccmux/internal/format"
	"github.com/ccmux/ccmux/internal/state"
	"github.com/mymmrac/telego"
	"github.com/rs/zerolog/log"
)

// sendToRef sends a message to the Telegram chat identified by ref, splitting if needed.
// Returns the message ID of the last sent message.
func (b *Bot) sendToRef(ctx context.Context, ref state.ChatRef, text string) (int, error) {
	chunks := format.SplitMessage(text)
	var lastID int
	for _, chunk := range chunks {
		id, err := b.sendChunk(ctx, ref, chunk, nil)
		if err != nil {
			return lastID, err
		}
		lastID = id
	}
	return lastID, nil
}

// sendToRefWithKeyboard sends a message with an inline keyboard.
func (b *Bot) sendToRefWithKeyboard(ctx context.Context, ref state.ChatRef, text string, kb telego.InlineKeyboardMarkup) (int, error) {
	return b.sendChunk(ctx, ref, text, &kb)
}

func (b *Bot) sendChunk(ctx context.Context, ref state.ChatRef, text string, kb *telego.InlineKeyboardMarkup) (int, error) {
	html := format.ToHTML(text)
	params := &telego.SendMessageParams{
		ChatID:          telego.ChatID{ID: ref.ChatID},
		MessageThreadID: ref.ThreadID,
		Text:            html,
		ParseMode:       telego.ModeHTML,
	}
	if kb != nil {
		params.ReplyMarkup = kb
	}

	msg, err := b.tg.SendMessage(ctx, params)
	if err != nil {
		// Retry with plain text on parse error
		if strings.Contains(err.Error(), "can't parse") || strings.Contains(err.Error(), "Bad Request") {
			plain := &telego.SendMessageParams{
				ChatID:          telego.ChatID{ID: ref.ChatID},
				MessageThreadID: ref.ThreadID,
				Text:            text,
			}
			if kb != nil {
				plain.ReplyMarkup = kb
			}
			msg, err = b.tg.SendMessage(ctx, plain)
			if err != nil {
				return 0, err
			}
		} else if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "Too Many Requests") {
			time.Sleep(2 * time.Second)
			msg, err = b.tg.SendMessage(ctx, params)
			if err != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}
	return msg.MessageID, nil
}

// editTopicMsg edits a previously sent message.
func (b *Bot) editTopicMsg(ctx context.Context, ref state.ChatRef, msgID int, text string) {
	html := format.ToHTML(text)
	_, err := b.tg.EditMessageText(ctx, &telego.EditMessageTextParams{
		ChatID:    telego.ChatID{ID: ref.ChatID},
		MessageID: msgID,
		Text:      html,
		ParseMode: telego.ModeHTML,
	})
	if err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Msg("editing message")
	}
}

// removeKeyboard removes the inline keyboard from a message.
func (b *Bot) removeKeyboard(ctx context.Context, ref state.ChatRef, msgID int) {
	empty := telego.InlineKeyboardMarkup{}
	_, err := b.tg.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
		ChatID:      telego.ChatID{ID: ref.ChatID},
		MessageID:   msgID,
		ReplyMarkup: &empty,
	})
	if err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Msg("removing keyboard")
	}
}
