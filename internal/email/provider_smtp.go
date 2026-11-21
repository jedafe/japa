package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// SMTPConfig carries the connection settings for an SMTP provider. It is
// the non-env constructor path used by DB-backed email_providers rows
// (ADR-015 admin-addable generic SMTP).
type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string // secret — stored encrypted at rest, decrypted at load
	From string
	// EnvelopeFrom overrides the SMTP MAIL FROM (return-path) while From
	// stays the header address — bounces land in a dedicated mailbox the
	// newsletter:poll_bounces task ingests (§62 self-hosted transport).
	EnvelopeFrom string
	Name         string // optional display name for logs/dashboards
}

// SMTPProvider implements EmailProvider using raw SMTP via net/smtp.
// Configuration is read from JC_SMTP_* environment variables, or supplied
// directly via NewSMTPProviderFromConfig for DB-backed providers.
type SMTPProvider struct {
	host         string
	port         string
	user         string
	pass         string
	from         string
	envelopeFrom string
	addr         string
	name         string
}

// NewSMTPProvider creates an SMTPProvider from environment variables.
// Returns an error if JC_SMTP_HOST is not set (SMTP not configured).
func NewSMTPProvider() (*SMTPProvider, error) {
	host := os.Getenv("JC_SMTP_HOST")
	if host == "" {
		return nil, errors.New("SMTP not configured: JC_SMTP_HOST is empty")
	}

	return NewSMTPProviderFromConfig(SMTPConfig{
		Host:         host,
		Port:         os.Getenv("JC_SMTP_PORT"),
		User:         os.Getenv("JC_SMTP_USER"),
		Pass:         os.Getenv("JC_SMTP_PASS"),
		From:         os.Getenv("JC_SMTP_FROM"),
		EnvelopeFrom: os.Getenv("JC_SMTP_ENVELOPE_FROM"),
	})
}

// NewSMTPProviderFromConfig creates an SMTPProvider from explicit settings.
// Port defaults to 587; From defaults to User. Host is required.
func NewSMTPProviderFromConfig(cfg SMTPConfig) (*SMTPProvider, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("SMTP not configured: host is empty")
	}

	port := cfg.Port
	if port == "" {
		port = "587"
	}

	from := cfg.From
	if from == "" {
		from = cfg.User
	}

	name := cfg.Name
	if name == "" {
		name = "smtp"
	}

	return &SMTPProvider{
		host:         cfg.Host,
		port:         port,
		user:         cfg.User,
		pass:         cfg.Pass,
		from:         from,
		envelopeFrom: cfg.EnvelopeFrom,
		addr:         cfg.Host + ":" + port,
		name:         name,
	}, nil
}

// Name returns the provider identifier.
func (p *SMTPProvider) Name() string {
	if p.name != "" {
		return p.name
	}

	return "smtp"
}

// sanitizeHeaderValue strips CR/LF from a header field to block SMTP header
// injection (CWE-113): a To/Subject/extra-header value containing "\r\nBcc:
// attacker@x" would otherwise smuggle attacker headers into the message.
func sanitizeHeaderValue(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")

	return strings.TrimSpace(v)
}

// parseAddress validates and parses an address, returning the mailbox so
// callers can pick the representation each context needs: .Address is the
// bare mailbox for envelope MAIL FROM/RCPT TO and vendor email fields,
// while .String() is the display-name-preserving RFC 5322 form for
// From:/To: headers and vendor name fields.
func parseAddress(addr string) (*mail.Address, error) {
	addr = sanitizeHeaderValue(addr)

	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid email address: %w", err)
	}

	return parsed, nil
}

// sanitizeAddress returns the bare mailbox of an address — the form
// envelope commands and vendor email fields require.
func sanitizeAddress(addr string) (string, error) {
	parsed, err := parseAddress(addr)
	if err != nil {
		return "", err
	}

	return parsed.Address, nil
}

// headerAddress returns the RFC 5322 From:/To: header value for a parsed
// mailbox: the display-name-preserving "Name <addr>" form when a name is
// configured, the bare address otherwise (mail.Address.String renders
// nameless addresses as "<addr>" — a cosmetic regression worth avoiding).
func headerAddress(a *mail.Address) string {
	if a.Name == "" {
		return a.Address
	}

	return a.String()
}

// Send dispatches an email via SMTP.
func (p *SMTPProvider) Send(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	to, err := parseAddress(req.To)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("smtp send: %w", err)
	}

	from, err := parseAddress(p.from)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("smtp misconfigured (JC_SMTP_FROM): %w", err)
	}

	// Envelope MAIL FROM may differ from the header From (bounce mailbox
	// routing, §62 self-hosted transport); default to the header address.
	mailFrom := from.Address

	if p.envelopeFrom != "" {
		if mailFrom, err = sanitizeAddress(p.envelopeFrom); err != nil {
			return EmailResponse{}, fmt.Errorf("smtp misconfigured (envelope_from): %w", err)
		}
	}

	// Headers carry the display-name-preserving form; the envelope
	// commands below use the bare mailbox.
	msg := buildRFC822(headerAddress(from), headerAddress(to), req.Subject, req)

	var auth smtp.Auth
	if p.user != "" && p.pass != "" {
		auth = smtp.PlainAuth("", p.user, p.pass, p.host)
	}

	client, err := p.dial(ctx)
	if err != nil {
		return EmailResponse{}, err
	}

	defer func() { _ = client.Close() }()

	// Port 465 is already wrapped in TLS by dial. For other ports use
	// STARTTLS when the server offers it (net/smtp.SendMail semantics) —
	// in production a server that doesn't offer it is refused rather than
	// sending credentials and content in cleartext.
	if p.port != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: p.host, MinVersion: tls.VersionTLS12}); err != nil {
				return EmailResponse{}, fmt.Errorf("smtp starttls: %w", err)
			}
		} else if strings.EqualFold(os.Getenv("JC_ENV"), "production") {
			_ = client.Quit()

			return EmailResponse{}, fmt.Errorf("smtp server %s does not offer STARTTLS (refusing plaintext in production)", p.addr)
		}
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return EmailResponse{}, fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(mailFrom); err != nil {
		return EmailResponse{}, fmt.Errorf("smtp mail: %w", err)
	}

	if err := client.Rcpt(to.Address); err != nil {
		return EmailResponse{}, fmt.Errorf("smtp rcpt: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return EmailResponse{}, fmt.Errorf("smtp data: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
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

// dial opens the SMTP session with a context-aware dialer (net/smtp's
// Dial/SendMail can't take ctx — a hung server would block the worker
// past task cancellation). The returned conn's deadline bounds all
// subsequent session I/O: the ctx deadline when present, else a fixed
// 60s budget. Port 465 gets the TLS handshake here; other ports upgrade
// via STARTTLS in Send.
func (p *SMTPProvider) dial(ctx context.Context) (*smtp.Client, error) {
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}

	conn, err := d.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial %s: %w", p.addr, err)
	}

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	}

	if p.port == "465" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: p.host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()

			return nil, fmt.Errorf("TLS dial %s: %w", p.addr, err)
		}

		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, p.host)
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("smtp client: %w", err)
	}

	return client, nil
}
