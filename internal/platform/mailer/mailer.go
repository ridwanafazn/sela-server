// Package mailer sends account verification and password-reset messages.
package mailer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ridwanafazn/sela-server/internal/config"
)

// Message is one outbound email.
type Message struct {
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Text    string    `json:"text"`
	SentAt  time.Time `json:"sent_at"`
}

// Sender sends one message.
type Sender interface {
	Send(ctx context.Context, message Message) error
}

// New creates the configured sender.
func New(cfg config.Email) (Sender, error) {
	switch cfg.Provider {
	case "noop":
		return NoopSender{}, nil
	case "file":
		return NewFileSender(cfg.OutboxDir)
	case "smtp":
		return &SMTPSender{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported email provider %q", cfg.Provider)
	}
}

// NoopSender discards messages.
type NoopSender struct{}

// Send implements Sender.
func (NoopSender) Send(context.Context, Message) error { return nil }

// FileSender appends messages to a JSON-lines outbox for local development.
type FileSender struct {
	path string
	mu   sync.Mutex
}

// NewFileSender creates a file-backed sender.
func NewFileSender(dir string) (*FileSender, error) {
	if dir == "" {
		return nil, fmt.Errorf("email outbox directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create email outbox: %w", err)
	}
	return &FileSender{path: filepath.Join(dir, "outbox.jsonl")}, nil
}

// Send implements Sender.
func (s *FileSender) Send(_ context.Context, message Message) error {
	if message.SentAt.IsZero() {
		message.SentAt = time.Now().UTC()
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open email outbox: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write email outbox: %w", err)
	}
	return nil
}

// SMTPSender sends messages through an SMTP relay.
type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Send implements Sender.
func (s *SMTPSender) Send(_ context.Context, message Message) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	body := []byte(
		"From: " + s.From + "\r\n" +
			"To: " + message.To + "\r\n" +
			"Subject: " + message.Subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			message.Text + "\r\n",
	)
	if err := smtp.SendMail(addr, auth, s.From, []string{message.To}, body); err != nil {
		return fmt.Errorf("send smtp email: %w", err)
	}
	return nil
}
