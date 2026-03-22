package bot

import (
	"context"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"github.com/rs/zerolog/log"
	"github.com/ccmux/ccmux/internal/format"
)

// sendToTopic sends a message to a Telegram forum topic, splitting if needed.
// Returns the message ID of the last sent message.
func (b *Bot) sendToTopic(ctx context.Context, topicID int, text string) (int, error) {
	chunks := format.SplitMessage(text)
	var lastID int
	for _, chunk := range chunks {
		id, err := b.sendChunk(ctx, topicID, chunk, nil)
		if err != nil {
			return lastID, err
		}
		lastID = id
	}
	return lastID, nil
}

// sendToTopicWithKeyboard sends a message with an inline keyboard.
func (b *Bot) sendToTopicWithKeyboard(ctx context.Context, topicID int, text string, kb telego.InlineKeyboardMarkup) (int, error) {
	return b.sendChunk(ctx, topicID, text, &kb)
}

func (b *Bot) sendChunk(ctx context.Context, topicID int, text string, kb *telego.InlineKeyboardMarkup) (int, error) {
	html := format.ToHTML(text)
	params := &telego.SendMessageParams{
		ChatID:          telego.ChatID{ID: b.cfg.GroupChatID},
		MessageThreadID: topicID,
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
				ChatID:          telego.ChatID{ID: b.cfg.GroupChatID},
				MessageThreadID: topicID,
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
func (b *Bot) editTopicMsg(ctx context.Context, msgID int, text string) {
	html := format.ToHTML(text)
	_, err := b.tg.EditMessageText(ctx, &telego.EditMessageTextParams{
		ChatID:    telego.ChatID{ID: b.cfg.GroupChatID},
		MessageID: msgID,
		Text:      html,
		ParseMode: telego.ModeHTML,
	})
	if err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Msg("editing message")
	}
}

// removeKeyboard removes the inline keyboard from a message.
func (b *Bot) removeKeyboard(ctx context.Context, msgID int) {
	empty := telego.InlineKeyboardMarkup{}
	_, err := b.tg.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
		ChatID:      telego.ChatID{ID: b.cfg.GroupChatID},
		MessageID:   msgID,
		ReplyMarkup: &empty,
	})
	if err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Msg("removing keyboard")
	}
}
