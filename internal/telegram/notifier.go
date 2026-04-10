package telegram

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Notifier delivers messages to all allowed Telegram chats.
type Notifier struct {
	bot            *tgbotapi.BotAPI
	allowedChatIDs map[int64]struct{}
}

// NewNotifier creates a Telegram notifier.
func NewNotifier(botToken string, allowedChatIDs map[int64]struct{}) (*Notifier, error) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	bot.Debug = false
	return &Notifier{bot: bot, allowedChatIDs: allowedChatIDs}, nil
}

// Bot returns the underlying Telegram bot client.
func (n *Notifier) Bot() *tgbotapi.BotAPI {
	return n.bot
}

// IsAllowedChat reports whether a chat id can interact with the service.
func (n *Notifier) IsAllowedChat(chatID int64) bool {
	_, ok := n.allowedChatIDs[chatID]
	return ok
}

// AllowedChatIDs returns the configured chat ids in stable order.
func (n *Notifier) AllowedChatIDs() []int64 {
	result := make([]int64, 0, len(n.allowedChatIDs))
	for chatID := range n.allowedChatIDs {
		result = append(result, chatID)
	}
	sort.Slice(result, func(i int, j int) bool { return result[i] < result[j] })
	return result
}

// Broadcast sends one message to every allowed chat.
func (n *Notifier) Broadcast(ctx context.Context, message string) error {
	for chatID := range n.allowedChatIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg := tgbotapi.NewMessage(chatID, message)
		if _, err := n.bot.Send(msg); err != nil {
			return fmt.Errorf("send telegram message to %d: %w", chatID, err)
		}
	}
	return nil
}

// FormatMailboxList renders mailboxes for a Telegram reply.
func FormatMailboxList(mailboxes []string) string {
	if len(mailboxes) == 0 {
		return "当前还没有托管任何邮箱。"
	}
	return "当前邮箱列表:\n" + strings.Join(mailboxes, "\n")
}
