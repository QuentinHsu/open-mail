package mailbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"open-mail/internal/config"
	"open-mail/internal/cryptox"
	"open-mail/internal/model"
	"open-mail/internal/store"
)

var (
	// ErrMailboxNotFound is returned when a mailbox id does not exist.
	ErrMailboxNotFound = errors.New("mailbox not found")
	// ErrMailboxExists is returned when a mailbox with the same email already exists.
	ErrMailboxExists = errors.New("mailbox already exists")
)

// CredentialValidator checks whether mailbox credentials are valid against the IMAP server.
type CredentialValidator interface {
	Validate(ctx context.Context, mailbox model.Mailbox, password string) error
}

// RefreshTrigger refreshes background monitors after mailbox changes.
type RefreshTrigger interface {
	Refresh(ctx context.Context) error
}

// Service manages mailbox CRUD, encryption, and persistence.
type Service struct {
	cfg       config.Config
	store     *store.FileStore
	crypto    *cryptox.Service
	validator CredentialValidator
	refresh   RefreshTrigger
	mu        sync.RWMutex
	mailboxes []model.Mailbox
}

// NewService creates a mailbox service with persisted state.
func NewService(cfg config.Config, fileStore *store.FileStore, crypto *cryptox.Service, validator CredentialValidator, refresh RefreshTrigger) (*Service, error) {
	mailboxes, err := fileStore.Load()
	if err != nil {
		return nil, err
	}

	return &Service{
		cfg:       cfg,
		store:     fileStore,
		crypto:    crypto,
		validator: validator,
		refresh:   refresh,
		mailboxes: mailboxes,
	}, nil
}

// SetRefreshTrigger wires the monitor refresh callback after service construction.
func (s *Service) SetRefreshTrigger(refresh RefreshTrigger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh = refresh
}

// List returns a copy of all managed mailboxes.
func (s *Service) List() []model.Mailbox {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Mailbox, len(s.mailboxes))
	copy(result, s.mailboxes)
	return result
}

// Get returns one mailbox by id.
func (s *Service) Get(id string) (model.Mailbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, mailbox := range s.mailboxes {
		if mailbox.ID == id {
			return mailbox, nil
		}
	}
	return model.Mailbox{}, ErrMailboxNotFound
}

// Create validates credentials, encrypts the password, persists the mailbox, and refreshes monitoring.
func (s *Service) Create(ctx context.Context, input model.MailboxInput) (model.Mailbox, error) {
	normalized, password, err := s.normalizeInput(input)
	if err != nil {
		return model.Mailbox{}, err
	}

	s.mu.Lock()
	for _, mailbox := range s.mailboxes {
		if strings.EqualFold(mailbox.Email, normalized.Email) {
			s.mu.Unlock()
			return model.Mailbox{}, ErrMailboxExists
		}
	}
	s.mu.Unlock()

	candidate := model.Mailbox{
		ID:          generateID(),
		Email:       normalized.Email,
		DisplayName: normalized.DisplayName,
		IMAPHost:    normalized.IMAPHost,
		IMAPPort:    normalized.IMAPPort,
		UseTLS:      normalized.UseTLS != nil && *normalized.UseTLS,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if normalized.UseTLS == nil {
		candidate.UseTLS = s.cfg.DefaultIMAPTLS
	}

	if err := s.validator.Validate(ctx, candidate, password); err != nil {
		return model.Mailbox{}, err
	}

	encryptedPassword, err := s.crypto.Encrypt(password)
	if err != nil {
		return model.Mailbox{}, fmt.Errorf("encrypt password: %w", err)
	}
	candidate.EncryptedPassword = encryptedPassword

	s.mu.Lock()
	s.mailboxes = append(s.mailboxes, candidate)
	mailboxes := append([]model.Mailbox(nil), s.mailboxes...)
	s.mu.Unlock()

	if err := s.store.Save(mailboxes); err != nil {
		return model.Mailbox{}, err
	}
	if s.refresh != nil {
		if err := s.refresh.Refresh(ctx); err != nil {
			return model.Mailbox{}, err
		}
	}

	return candidate, nil
}

// Update replaces credentials or metadata for one mailbox.
func (s *Service) Update(ctx context.Context, id string, input model.MailboxInput) (model.Mailbox, error) {
	normalized, password, err := s.normalizeInput(input)
	if err != nil {
		return model.Mailbox{}, err
	}

	s.mu.Lock()
	index := -1
	current := model.Mailbox{}
	for i, mailbox := range s.mailboxes {
		if mailbox.ID == id {
			index = i
			current = mailbox
			break
		}
	}
	if index == -1 {
		s.mu.Unlock()
		return model.Mailbox{}, ErrMailboxNotFound
	}
	for _, mailbox := range s.mailboxes {
		if mailbox.ID != id && strings.EqualFold(mailbox.Email, normalized.Email) {
			s.mu.Unlock()
			return model.Mailbox{}, ErrMailboxExists
		}
	}
	s.mu.Unlock()

	updated := current
	updated.Email = normalized.Email
	updated.DisplayName = normalized.DisplayName
	updated.IMAPHost = normalized.IMAPHost
	updated.IMAPPort = normalized.IMAPPort
	if normalized.UseTLS == nil {
		updated.UseTLS = s.cfg.DefaultIMAPTLS
	} else {
		updated.UseTLS = *normalized.UseTLS
	}
	updated.UpdatedAt = time.Now().UTC()

	if err := s.validator.Validate(ctx, updated, password); err != nil {
		return model.Mailbox{}, err
	}

	encryptedPassword, err := s.crypto.Encrypt(password)
	if err != nil {
		return model.Mailbox{}, fmt.Errorf("encrypt password: %w", err)
	}
	updated.EncryptedPassword = encryptedPassword
	updated.LastError = ""

	s.mu.Lock()
	s.mailboxes[index] = updated
	mailboxes := append([]model.Mailbox(nil), s.mailboxes...)
	s.mu.Unlock()

	if err := s.store.Save(mailboxes); err != nil {
		return model.Mailbox{}, err
	}
	if s.refresh != nil {
		if err := s.refresh.Refresh(ctx); err != nil {
			return model.Mailbox{}, err
		}
	}

	return updated, nil
}

// Delete removes a mailbox by id and refreshes monitoring.
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	index := -1
	for i, mailbox := range s.mailboxes {
		if mailbox.ID == id {
			index = i
			break
		}
	}
	if index == -1 {
		s.mu.Unlock()
		return ErrMailboxNotFound
	}
	s.mailboxes = append(s.mailboxes[:index], s.mailboxes[index+1:]...)
	mailboxes := append([]model.Mailbox(nil), s.mailboxes...)
	s.mu.Unlock()

	if err := s.store.Save(mailboxes); err != nil {
		return err
	}
	if s.refresh != nil {
		if err := s.refresh.Refresh(ctx); err != nil {
			return err
		}
	}
	return nil
}

// UpdateState updates polling state for one mailbox after a monitor pass.
func (s *Service) UpdateState(ctx context.Context, id string, lastSeenUID uint32, lastCheckedAt time.Time, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for index, mailbox := range s.mailboxes {
		if mailbox.ID != id {
			continue
		}
		mailbox.LastSeenUID = lastSeenUID
		mailbox.LastCheckedAt = lastCheckedAt.UTC()
		mailbox.LastError = lastError
		mailbox.UpdatedAt = time.Now().UTC()
		s.mailboxes[index] = mailbox
		return s.store.Save(append([]model.Mailbox(nil), s.mailboxes...))
	}
	return ErrMailboxNotFound
}

// DecryptPassword restores a mailbox password for IMAP use.
func (s *Service) DecryptPassword(mailbox model.Mailbox) (string, error) {
	return s.crypto.Decrypt(mailbox.EncryptedPassword)
}

func (s *Service) normalizeInput(input model.MailboxInput) (model.MailboxInput, string, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	password := strings.TrimSpace(input.Password)
	if email == "" {
		return model.MailboxInput{}, "", fmt.Errorf("email is required")
	}
	if password == "" {
		return model.MailboxInput{}, "", fmt.Errorf("password is required")
	}

	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = email
	}

	host := strings.TrimSpace(input.IMAPHost)
	if host == "" {
		host = s.cfg.DefaultIMAPHost
	}

	port := input.IMAPPort
	if port == 0 {
		port = s.cfg.DefaultIMAPPort
	}

	return model.MailboxInput{
		Email:       email,
		Password:    password,
		DisplayName: displayName,
		IMAPHost:    host,
		IMAPPort:    port,
		UseTLS:      input.UseTLS,
	}, password, nil
}

func generateID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
