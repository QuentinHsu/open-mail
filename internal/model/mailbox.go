package model

import "time"

// Mailbox stores the persisted configuration and state for one Outlook account.
type Mailbox struct {
	ID                string    `json:"id"`
	Email             string    `json:"email"`
	DisplayName       string    `json:"display_name"`
	IMAPHost          string    `json:"imap_host"`
	IMAPPort          int       `json:"imap_port"`
	UseTLS            bool      `json:"use_tls"`
	EncryptedPassword string    `json:"encrypted_password"`
	LastSeenUID       uint32    `json:"last_seen_uid"`
	LastCheckedAt     time.Time `json:"last_checked_at"`
	LastError         string    `json:"last_error"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// MailboxInput is the client payload used for create and update operations.
type MailboxInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	IMAPHost    string `json:"imap_host"`
	IMAPPort    int    `json:"imap_port"`
	UseTLS      *bool  `json:"use_tls"`
}
