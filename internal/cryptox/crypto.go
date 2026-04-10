package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// Service encrypts and decrypts mailbox passwords for local persistence.
type Service struct {
	key []byte
}

// NewService derives a stable AES-256 key from the configured secret.
func NewService(secret string) *Service {
	sum := sha256.Sum256([]byte(secret))
	return &Service{key: sum[:]}
}

// Encrypt secures plaintext using AES-GCM and returns a base64 payload.
func (s *Service) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt restores plaintext from a base64 AES-GCM payload.
func (s *Service) Decrypt(ciphertext string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return "", fmt.Errorf("ciphertext is too short")
	}

	nonce := payload[:nonceSize]
	data := payload[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt payload: %w", err)
	}

	return string(plaintext), nil
}
