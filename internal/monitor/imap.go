package monitor

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"

	"open-mail/internal/model"
)

// MessageSummary is the notification payload produced by an IMAP poll.
type MessageSummary struct {
	UID      uint32
	Subject  string
	From     string
	Preview  string
	Received time.Time
}

// IMAPClient validates mailbox credentials and fetches newly arrived mail.
type IMAPClient struct{}

// NewIMAPClient creates an IMAP client helper.
func NewIMAPClient() *IMAPClient {
	return &IMAPClient{}
}

// Validate checks whether a mailbox can authenticate to IMAP.
func (c *IMAPClient) Validate(ctx context.Context, mailbox model.Mailbox, password string) error {
	client, err := c.login(ctx, mailbox, password)
	if err != nil {
		return err
	}
	defer client.Logout()

	if _, err := client.Select("INBOX", false); err != nil {
		return fmt.Errorf("select inbox: %w", err)
	}
	return nil
}

// FetchNewMessages returns unseen messages newer than the persisted last UID.
func (c *IMAPClient) FetchNewMessages(ctx context.Context, mailbox model.Mailbox, password string) ([]MessageSummary, uint32, error) {
	client, err := c.login(ctx, mailbox, password)
	if err != nil {
		return nil, mailbox.LastSeenUID, err
	}
	defer client.Logout()

	status, err := client.Select("INBOX", false)
	if err != nil {
		return nil, mailbox.LastSeenUID, fmt.Errorf("select inbox: %w", err)
	}
	if status.Messages == 0 {
		return nil, mailbox.LastSeenUID, nil
	}

	allUIDs, err := client.UidSearch(imap.NewSearchCriteria())
	if err != nil {
		return nil, mailbox.LastSeenUID, fmt.Errorf("search inbox: %w", err)
	}
	if len(allUIDs) == 0 {
		return nil, mailbox.LastSeenUID, nil
	}
	sort.Slice(allUIDs, func(i int, j int) bool { return allUIDs[i] < allUIDs[j] })
	latestUID := allUIDs[len(allUIDs)-1]
	if mailbox.LastSeenUID == 0 {
		return nil, latestUID, nil
	}

	newUIDs := make([]uint32, 0)
	for _, uid := range allUIDs {
		if uid > mailbox.LastSeenUID {
			newUIDs = append(newUIDs, uid)
		}
	}
	if len(newUIDs) == 0 {
		return nil, latestUID, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(newUIDs...)
	section := &imap.BodySectionName{}
	messages := make(chan *imap.Message, len(newUIDs))
	done := make(chan error, 1)
	go func() {
		done <- client.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, section.FetchItem()}, messages)
	}()

	summaries := make([]MessageSummary, 0, len(newUIDs))
	for msg := range messages {
		summary, err := buildSummary(msg, section)
		if err != nil {
			return nil, mailbox.LastSeenUID, err
		}
		summaries = append(summaries, summary)
	}
	if err := <-done; err != nil {
		return nil, mailbox.LastSeenUID, fmt.Errorf("fetch messages: %w", err)
	}

	sort.Slice(summaries, func(i int, j int) bool { return summaries[i].UID < summaries[j].UID })
	return summaries, latestUID, nil
}

func (c *IMAPClient) login(ctx context.Context, mailbox model.Mailbox, password string) (*client.Client, error) {
	address := fmt.Sprintf("%s:%d", mailbox.IMAPHost, mailbox.IMAPPort)
	var imapClient *client.Client
	var err error

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", address, err)
	}

	if mailbox.UseTLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: mailbox.IMAPHost, MinVersion: tls.VersionTLS12})
		imapClient, err = client.New(tlsConn)
	} else {
		imapClient, err = client.New(conn)
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create imap client: %w", err)
	}

	if err := imapClient.Login(mailbox.Email, password); err != nil {
		return nil, fmt.Errorf("login %s: %w", mailbox.Email, err)
	}
	return imapClient, nil
}

func buildSummary(msg *imap.Message, section *imap.BodySectionName) (MessageSummary, error) {
	summary := MessageSummary{UID: msg.Uid}
	if msg.Envelope != nil {
		summary.Subject = msg.Envelope.Subject
		summary.Received = msg.Envelope.Date
		if len(msg.Envelope.From) > 0 {
			from := msg.Envelope.From[0]
			summary.From = strings.TrimSpace(strings.TrimSpace(from.PersonalName) + " <" + from.MailboxName + "@" + from.HostName + ">")
		}
	}
	if summary.Subject == "" {
		summary.Subject = "(无主题)"
	}
	if summary.From == "" {
		summary.From = "未知发件人"
	}

	body := msg.GetBody(section)
	if body == nil {
		return summary, nil
	}
	reader, err := mail.CreateReader(body)
	if err != nil {
		return summary, nil
	}

	var preview strings.Builder
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return summary, nil
		}

		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			mediaType, _, _ := header.ContentType()
			if mediaType != "text/plain" && mediaType != "text/html" {
				continue
			}
			buffer := new(bytes.Buffer)
			if _, err := io.Copy(buffer, part.Body); err != nil {
				continue
			}
			preview.WriteString(buffer.String())
			if preview.Len() > 400 {
				break
			}
		}
	}

	summary.Preview = compactPreview(preview.String())
	return summary, nil
}

func compactPreview(value string) string {
	replacer := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ")
	clean := strings.TrimSpace(replacer.Replace(value))
	clean = strings.Join(strings.Fields(clean), " ")
	if clean == "" {
		return "邮件正文为空或无法解析。"
	}
	if len(clean) > 280 {
		return clean[:280] + "..."
	}
	return clean
}
