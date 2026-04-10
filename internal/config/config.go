package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains runtime configuration for the service.
type Config struct {
	Port                  string
	TelegramBotToken      string
	TelegramAllowedChatID map[int64]struct{}
	MailboxEncryptionKey  string
	DataDir               string
	PollInterval          time.Duration
	DefaultIMAPHost       string
	DefaultIMAPPort       int
	DefaultIMAPTLS        bool
	APIToken              string
}

// Load parses environment variables into a validated config value.
func Load() (Config, error) {
	pollInterval := getEnv("POLL_INTERVAL", "1m")
	pollDuration, err := time.ParseDuration(pollInterval)
	if err != nil {
		return Config{}, fmt.Errorf("parse POLL_INTERVAL: %w", err)
	}

	port := getEnv("PORT", "3000")
	defaultPort, err := strconv.Atoi(getEnv("DEFAULT_IMAP_PORT", "993"))
	if err != nil {
		return Config{}, fmt.Errorf("parse DEFAULT_IMAP_PORT: %w", err)
	}

	defaultTLS, err := strconv.ParseBool(getEnv("DEFAULT_IMAP_TLS", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("parse DEFAULT_IMAP_TLS: %w", err)
	}

	allowedChatIDs, err := parseChatIDs(os.Getenv("TELEGRAM_ALLOWED_CHAT_IDS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse TELEGRAM_ALLOWED_CHAT_IDS: %w", err)
	}

	cfg := Config{
		Port:                  port,
		TelegramBotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramAllowedChatID: allowedChatIDs,
		MailboxEncryptionKey:  os.Getenv("MAILBOX_ENCRYPTION_KEY"),
		DataDir:               getEnv("DATA_DIR", ".data"),
		PollInterval:          pollDuration,
		DefaultIMAPHost:       getEnv("DEFAULT_IMAP_HOST", "outlook.office365.com"),
		DefaultIMAPPort:       defaultPort,
		DefaultIMAPTLS:        defaultTLS,
		APIToken:              os.Getenv("API_TOKEN"),
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.MailboxEncryptionKey == "" {
		return Config{}, fmt.Errorf("MAILBOX_ENCRYPTION_KEY is required")
	}
	if len(cfg.TelegramAllowedChatID) == 0 {
		return Config{}, fmt.Errorf("TELEGRAM_ALLOWED_CHAT_IDS must include at least one chat id")
	}

	return cfg, nil
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseChatIDs(raw string) (map[int64]struct{}, error) {
	result := make(map[int64]struct{})
	for _, part := range strings.Split(strings.TrimSpace(raw), ",") {
		if part == "" {
			continue
		}
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return nil, err
		}
		result[value] = struct{}{}
	}
	return result, nil
}
