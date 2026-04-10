package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"open-mail/internal/model"
)

// FileStore persists mailbox configuration into a JSON file under DATA_DIR.
type FileStore struct {
	path string
	mu   sync.Mutex
}

// NewFileStore creates a JSON-backed mailbox store.
func NewFileStore(dataDir string) (*FileStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &FileStore{path: filepath.Join(dataDir, "mailboxes.json")}, nil
}

// Load retrieves all persisted mailboxes.
func (s *FileStore) Load() ([]model.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

// Save writes the full mailbox list atomically.
func (s *FileStore) Save(mailboxes []model.Mailbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.MarshalIndent(mailboxes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mailboxes: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return fmt.Errorf("write temp mailbox store: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace mailbox store: %w", err)
	}

	return nil
}

func (s *FileStore) loadUnlocked() ([]model.Mailbox, error) {
	payload, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Mailbox{}, nil
		}
		return nil, fmt.Errorf("read mailbox store: %w", err)
	}

	var mailboxes []model.Mailbox
	if err := json.Unmarshal(payload, &mailboxes); err != nil {
		return nil, fmt.Errorf("unmarshal mailbox store: %w", err)
	}

	return mailboxes, nil
}
