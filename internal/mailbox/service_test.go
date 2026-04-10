package mailbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"open-mail/internal/config"
	"open-mail/internal/cryptox"
	"open-mail/internal/model"
	"open-mail/internal/store"
)

type fakeValidator struct{}

func (fakeValidator) Validate(_ context.Context, _ model.Mailbox, _ string) error {
	return nil
}

type fakeRefresh struct{}

func (fakeRefresh) Refresh(_ context.Context) error {
	return nil
}

func TestServiceCreateAndDeleteMailbox(t *testing.T) {
	cfg := config.Config{
		DefaultIMAPHost:      "outlook.office365.com",
		DefaultIMAPPort:      993,
		DefaultIMAPTLS:       true,
		MailboxEncryptionKey: "super-secret",
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	fileStore, err := store.NewFileStore(dataDir)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	service, err := NewService(cfg, fileStore, cryptox.NewService(cfg.MailboxEncryptionKey), fakeValidator{}, fakeRefresh{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := service.Create(context.Background(), model.MailboxInput{Email: "user@example.com", Password: "pw"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Email != "user@example.com" {
		t.Fatalf("Create() email = %q, want user@example.com", created.Email)
	}
	if len(service.List()) != 1 {
		t.Fatalf("List() length = %d, want 1", len(service.List()))
	}

	if err := service.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(service.List()) != 0 {
		t.Fatalf("List() length after delete = %d, want 0", len(service.List()))
	}

	if _, err := os.Stat(filepath.Join(dataDir, "mailboxes.json")); err != nil {
		t.Fatalf("mailboxes.json should exist: %v", err)
	}
}
