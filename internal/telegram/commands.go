package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"open-mail/internal/mailbox"
	"open-mail/internal/model"
)

// CommandHandler processes mailbox commands from Telegram.
type CommandHandler struct {
	logger    *slog.Logger
	notifier  *Notifier
	mailboxes *mailbox.Service
}

// NewCommandHandler creates a Telegram command controller.
func NewCommandHandler(logger *slog.Logger, notifier *Notifier, mailboxes *mailbox.Service) *CommandHandler {
	return &CommandHandler{logger: logger, notifier: notifier, mailboxes: mailboxes}
}

// Run consumes Telegram updates until the context is canceled.
func (h *CommandHandler) Run(ctx context.Context) error {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := h.notifier.Bot().GetUpdatesChan(updateConfig)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			if update.Message == nil {
				continue
			}
			h.handleMessage(ctx, update.Message)
		}
	}
}

func (h *CommandHandler) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	if !h.notifier.IsAllowedChat(message.Chat.ID) {
		h.reply(message.Chat.ID, "这个 Telegram 会话没有权限访问该服务。")
		return
	}

	fields := strings.Fields(message.Text)
	if len(fields) == 0 {
		h.reply(message.Chat.ID, helpText())
		return
	}

	command := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	switch command {
	case "start", "help":
		h.reply(message.Chat.ID, helpText())
	case "mailboxes":
		h.handleList(message.Chat.ID)
	case "add":
		h.handleAdd(ctx, message.Chat.ID, fields)
	case "update":
		h.handleUpdate(ctx, message.Chat.ID, fields)
	case "remove":
		h.handleRemove(ctx, message.Chat.ID, fields)
	default:
		h.reply(message.Chat.ID, helpText())
	}
}

func (h *CommandHandler) handleList(chatID int64) {
	mailboxes := h.mailboxes.List()
	lines := make([]string, 0, len(mailboxes))
	for _, mailboxValue := range mailboxes {
		status := "正常"
		if mailboxValue.LastError != "" {
			status = "异常: " + mailboxValue.LastError
		}
		lines = append(lines, fmt.Sprintf("- %s %s (%s)", mailboxValue.ID, mailboxValue.Email, status))
	}
	h.reply(chatID, FormatMailboxList(lines))
}

func (h *CommandHandler) handleAdd(ctx context.Context, chatID int64, fields []string) {
	if len(fields) < 3 {
		h.reply(chatID, "用法: /add 邮箱 密码 [显示名]")
		return
	}
	displayName := ""
	if len(fields) > 3 {
		displayName = strings.Join(fields[3:], " ")
	}

	created, err := h.mailboxes.Create(ctx, model.MailboxInput{Email: fields[1], Password: fields[2], DisplayName: displayName})
	if err != nil {
		h.reply(chatID, "添加邮箱失败: "+err.Error())
		return
	}
	h.reply(chatID, fmt.Sprintf("邮箱已接入: %s (%s)", created.ID, created.Email))
}

func (h *CommandHandler) handleUpdate(ctx context.Context, chatID int64, fields []string) {
	if len(fields) < 4 {
		h.reply(chatID, "用法: /update ID 邮箱 密码 [显示名]")
		return
	}
	displayName := ""
	if len(fields) > 4 {
		displayName = strings.Join(fields[4:], " ")
	}

	updated, err := h.mailboxes.Update(ctx, fields[1], model.MailboxInput{Email: fields[2], Password: fields[3], DisplayName: displayName})
	if err != nil {
		h.reply(chatID, "更新邮箱失败: "+err.Error())
		return
	}
	h.reply(chatID, fmt.Sprintf("邮箱已更新: %s (%s)", updated.ID, updated.Email))
}

func (h *CommandHandler) handleRemove(ctx context.Context, chatID int64, fields []string) {
	if len(fields) != 2 {
		h.reply(chatID, "用法: /remove ID")
		return
	}
	if err := h.mailboxes.Delete(ctx, fields[1]); err != nil {
		h.reply(chatID, "删除邮箱失败: "+err.Error())
		return
	}
	h.reply(chatID, "邮箱已删除。")
}

func (h *CommandHandler) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.notifier.Bot().Send(msg); err != nil {
		h.logger.Error("reply telegram message", "chat_id", chatID, "error", err)
	}
}

func helpText() string {
	return strings.Join([]string{
		"可用命令:",
		"/mailboxes 查看当前托管邮箱",
		"/add 邮箱 密码 [显示名]",
		"/update ID 邮箱 密码 [显示名]",
		"/remove ID",
	}, "\n")
}
