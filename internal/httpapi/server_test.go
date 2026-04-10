package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"open-mail/internal/config"
	"open-mail/internal/cryptox"
	"open-mail/internal/mailbox"
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

func TestServerCreateMailbox(t *testing.T) {
	cfg := config.Config{
		DefaultIMAPHost:      "outlook.office365.com",
		DefaultIMAPPort:      993,
		DefaultIMAPTLS:       true,
		MailboxEncryptionKey: "super-secret",
	}
	fileStore, err := store.NewFileStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	service, err := mailbox.NewService(cfg, fileStore, cryptox.NewService(cfg.MailboxEncryptionKey), fakeValidator{}, fakeRefresh{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	server := NewServer(service, "")
	body, _ := json.Marshal(model.MailboxInput{Email: "user@example.com", Password: "pw"})
	request := httptest.NewRequest(http.MethodPost, "/v1/mailboxes", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
}
