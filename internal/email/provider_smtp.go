package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"os"
	"sort"
	"strings"
)

// SMTPProvider implements EmailProvider using raw SMTP via net/smtp.
// Configuration is read from JC_SMTP_* environment variables.
type SMTPProvider struct {
	host string
	port string
	user string
	pass string
	from string
	addr string
}

// NewSMTPProvider creates an SMTPProvider from environment variables.
// Returns an error if JC_SMTP_HOST is not set (SMTP not configured).
func NewSMTPProvider() (*SMTPProvider, error) {
	host := os.Getenv("JC_SMTP_HOST")
	if host == "" {
		return nil, fmt.Errorf("SMTP not configured: JC_SMTP_HOST is empty")
	}

	port := os.Getenv("JC_SMTP_PORT")
	if port == "" {
		port = "587"
	}

	user := os.Getenv("JC_SMTP_USER")
	pass := os.Getenv("JC_SMTP_PASS")

	from := os.Getenv("JC_SMTP_FROM")
	if from == "" {
		from = user
	}

	return &SMTPProvider{
		host: host,
		port: port,
		user: user,
		pass: pass,
		from: from,
		addr: host + ":" + port,
	}, nil
}

// Name returns the provider identifier.
func (p *SMTPProvider) Name() string {
	return "smtp"
}

// Send dispatches an email via SMTP.
func (p *SMTPProvider) Send(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	// Build the message
	contentType := "text/plain; charset=UTF-8"
	contentBody := req.Body

	if req.IsHTML {
		contentType = "text/html; charset=UTF-8"
	}

	// Base headers, in a stable order for deterministic output.
	var hdr strings.Builder
	hdr.WriteString("From: " + p.from + "\r\n")
	hdr.WriteString("To: " + req.To + "\r\n")
	hdr.WriteString("Subject: " + req.Subject + "\r\n")
	hdr.WriteString("MIME-Version: 1.0\r\n")
	hdr.WriteString("Content-Type: " + contentType + "\r\n")

	// Merge optional extra headers (e.g. List-Unsubscribe) in a stable
	// (alphabetical) order so the rendered message is deterministic.
	if len(req.Headers) > 0 {
		keys := make([]string, 0, len(req.Headers))
		for k := range req.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hdr.WriteString(k + ": " + req.Headers[k] + "\r\n")
		}
	}

	msg := hdr.String() + "\r\n" + contentBody

	var auth smtp.Auth
	if p.user != "" && p.pass != "" {
		auth = smtp.PlainAuth("", p.user, p.pass, p.host)
	}

	// Try TLS first for port 465, otherwise use STARTTLS or plain.
	if p.port == "465" {
		tlsConfig := &tls.Config{ServerName: p.host, MinVersion: tls.VersionTLS12}

		conn, err := tls.Dial("tcp", p.addr, tlsConfig)
		if err != nil {
			slog.Error("email_send: TLS dial failed", "addr", p.addr, "error", err)
			return EmailResponse{}, fmt.Errorf("TLS dial %s: %w", p.addr, err)
		}

		defer func() { _ = conn.Close() }()

		client, err := smtp.NewClient(conn, p.host)
		if err != nil {
			return EmailResponse{}, fmt.Errorf("smtp client: %w", err)
		}

		defer func() { _ = client.Close() }()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return EmailResponse{}, fmt.Errorf("smtp auth: %w", err)
			}
		}

		if err := client.Mail(p.from); err != nil {
			return EmailResponse{}, fmt.Errorf("smtp mail: %w", err)
		}

		if err := client.Rcpt(req.To); err != nil {
			return EmailResponse{}, fmt.Errorf("smtp rcpt: %w", err)
		}

		w, err := client.Data()
		if err != nil {
			return EmailResponse{}, fmt.Errorf("smtp data: %w", err)
		}

		if _, err := w.Write([]byte(msg)); err != nil {
			return EmailResponse{}, fmt.Errorf("smtp write: %w", err)
		}

		if err := w.Close(); err != nil {
			return EmailResponse{}, fmt.Errorf("smtp close: %w", err)
		}

		if err := client.Quit(); err != nil {
			return EmailResponse{}, fmt.Errorf("smtp quit: %w", err)
		}

		return EmailResponse{Status: "success"}, nil
	}

	// STARTTLS or plain SMTP.
	if err := smtp.SendMail(p.addr, auth, p.from, []string{req.To}, []byte(msg)); err != nil {
		slog.Error("email_send: SMTP send failed", "to", req.To, "addr", p.addr, "error", err)
		return EmailResponse{}, fmt.Errorf("smtp send to %s: %w", req.To, err)
	}

	return EmailResponse{Status: "success"}, nil
}
