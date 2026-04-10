package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"open-mail/internal/model"
)

// MailboxReader provides read and write access required by the monitor manager.
type MailboxReader interface {
	List() []model.Mailbox
	DecryptPassword(mailbox model.Mailbox) (string, error)
	UpdateState(ctx context.Context, id string, lastSeenUID uint32, lastCheckedAt time.Time, lastError string) error
}

// Broadcaster delivers notifications to Telegram.
type Broadcaster interface {
	Broadcast(ctx context.Context, message string) error
}

// Manager keeps one polling worker per mailbox.
type Manager struct {
	logger     *slog.Logger
	interval   time.Duration
	mailClient *IMAPClient
	mailboxes  MailboxReader
	notifier   Broadcaster
	mu         sync.Mutex
	running    map[string]context.CancelFunc
}

// NewManager creates a polling manager for all configured mailboxes.
func NewManager(logger *slog.Logger, interval time.Duration, mailClient *IMAPClient, mailboxes MailboxReader, notifier Broadcaster) *Manager {
	return &Manager{
		logger:     logger,
		interval:   interval,
		mailClient: mailClient,
		mailboxes:  mailboxes,
		notifier:   notifier,
		running:    make(map[string]context.CancelFunc),
	}
}

// Refresh reconciles goroutines with the latest mailbox list.
func (m *Manager) Refresh(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := make(map[string]model.Mailbox)
	for _, mailbox := range m.mailboxes.List() {
		current[mailbox.ID] = mailbox
		if _, exists := m.running[mailbox.ID]; exists {
			continue
		}

		workerCtx, cancel := context.WithCancel(context.Background())
		m.running[mailbox.ID] = cancel
		go m.runMailbox(workerCtx, mailbox.ID)
	}

	for id, cancel := range m.running {
		if _, exists := current[id]; exists {
			continue
		}
		cancel()
		delete(m.running, id)
	}

	return nil
}

// Stop shuts down all active mailbox workers.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, cancel := range m.running {
		cancel()
		delete(m.running, id)
	}
}

func (m *Manager) runMailbox(ctx context.Context, mailboxID string) {
	m.pollOnce(ctx, mailboxID)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollOnce(ctx, mailboxID)
		}
	}
}

func (m *Manager) pollOnce(ctx context.Context, mailboxID string) {
	var mailbox model.Mailbox
	found := false
	for _, candidate := range m.mailboxes.List() {
		if candidate.ID == mailboxID {
			mailbox = candidate
			found = true
			break
		}
	}
	if !found {
		return
	}

	password, err := m.mailboxes.DecryptPassword(mailbox)
	if err != nil {
		m.logger.Error("decrypt mailbox password", "mailbox_id", mailbox.ID, "error", err)
		_ = m.mailboxes.UpdateState(context.Background(), mailbox.ID, mailbox.LastSeenUID, time.Now().UTC(), err.Error())
		return
	}

	messages, lastUID, err := m.mailClient.FetchNewMessages(ctx, mailbox, password)
	if err != nil {
		m.logger.Error("poll mailbox", "mailbox_id", mailbox.ID, "email", mailbox.Email, "error", err)
		_ = m.mailboxes.UpdateState(context.Background(), mailbox.ID, mailbox.LastSeenUID, time.Now().UTC(), err.Error())
		return
	}

	for _, message := range messages {
		body := fmt.Sprintf("新邮件提醒\n邮箱: %s\n主题: %s\n发件人: %s\n时间: %s\n\n%s", mailbox.Email, message.Subject, message.From, message.Received.Local().Format(time.RFC3339), message.Preview)
		if err := m.notifier.Broadcast(ctx, body); err != nil {
			m.logger.Error("broadcast mail notification", "mailbox_id", mailbox.ID, "error", err)
		}
	}

	if err := m.mailboxes.UpdateState(context.Background(), mailbox.ID, lastUID, time.Now().UTC(), ""); err != nil {
		m.logger.Error("persist mailbox poll state", "mailbox_id", mailbox.ID, "error", err)
	}
}
