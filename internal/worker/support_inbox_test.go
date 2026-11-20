package worker

import (
	"bytes"
	"context"
	"errors"
	"net/mail"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	valkey "github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// fakeSupportBridge records bridge calls so filing outcomes can be
// asserted without a live ticket service.
type fakeSupportBridge struct {
	typeErr  error
	fileErr  error
	tickets  []SupportTicketInput
	comments []fakeComment
}

type fakeComment struct {
	ref    string
	userID pgtype.UUID
	body   string
}

func (f *fakeSupportBridge) TicketTypeID(_ context.Context, _ string) (pgtype.UUID, error) {
	if f.typeErr != nil {
		return pgtype.UUID{}, f.typeErr
	}
	var id pgtype.UUID
	_ = id.Scan("01900000-0000-7000-8000-000000000001")
	return id, nil
}

func (f *fakeSupportBridge) FileTicket(_ context.Context, in SupportTicketInput) error {
	if f.fileErr != nil {
		return f.fileErr
	}
	f.tickets = append(f.tickets, in)
	return nil
}

func (f *fakeSupportBridge) FileComment(_ context.Context, ref string, userID pgtype.UUID, body string) error {
	f.comments = append(f.comments, fakeComment{ref: ref, userID: userID, body: body})
	return nil
}

func rawMessage(headers, body string) []byte {
	return []byte(headers + "\r\n\r\n" + body)
}

func TestReadSupportIMAPConfig(t *testing.T) {
	t.Run("unset host disables", func(t *testing.T) {
		t.Setenv("JC_SUPPORT_IMAP_HOST", "")
		if cfg := readSupportIMAPConfig(); cfg != nil {
			t.Fatalf("expected nil config, got %+v", cfg)
		}
	})

	t.Run("defaults", func(t *testing.T) {
		t.Setenv("JC_SUPPORT_IMAP_HOST", "mail.japa.to")
		for _, k := range []string{"JC_SUPPORT_IMAP_PORT", "JC_SUPPORT_IMAP_MAILBOX",
			"JC_SUPPORT_IMAP_TLS", "JC_SUPPORT_IMAP_BATCH", "JC_SUPPORT_TICKET_TYPE"} {
			t.Setenv(k, "")
		}
		cfg := readSupportIMAPConfig()
		if cfg == nil {
			t.Fatal("expected config")
		}
		if cfg.port != "993" || cfg.mailbox != "INBOX" || !cfg.tls ||
			cfg.batch != 200 || cfg.ticketType != "demo-ticket-type-general" {
			t.Fatalf("bad defaults: %+v", cfg)
		}
	})

	t.Run("explicit values", func(t *testing.T) {
		t.Setenv("JC_SUPPORT_IMAP_HOST", "imap.example")
		t.Setenv("JC_SUPPORT_IMAP_PORT", "143")
		t.Setenv("JC_SUPPORT_IMAP_MAILBOX", "Support")
		t.Setenv("JC_SUPPORT_IMAP_TLS", "false")
		t.Setenv("JC_SUPPORT_IMAP_BATCH", "25")
		t.Setenv("JC_SUPPORT_TICKET_TYPE", "general")
		cfg := readSupportIMAPConfig()
		if cfg.port != "143" || cfg.mailbox != "Support" || cfg.tls ||
			cfg.batch != 25 || cfg.ticketType != "general" {
			t.Fatalf("bad config: %+v", cfg)
		}
	})
}

func TestTicketRefRegex(t *testing.T) {
	cases := map[string]string{
		"Re: [Ticket JC-1042] help":    "1042",
		"Re: JC-7":                     "7",
		"about jc-33 please":           "33",
		"no ref here":                  "",
		"JCX-9 not a ref":              "",
		"ABC-1042 wrong prefix":        "",
		"JC-":                          "",
		"ticket JC-12 and JC-99 first": "12",
	}
	for subj, want := range cases {
		m := ticketRefRe.FindStringSubmatch(subj)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("subject %q: got ref %q, want %q", subj, got, want)
		}
	}
}

func TestIsAutoSubmitted(t *testing.T) {
	mk := func(headers string) []byte {
		return rawMessage("From: User <u@example.com>\r\n"+headers, "hi")
	}
	auto := map[string][]byte{
		"auto-submitted header": mk("Auto-Submitted: auto-replied"),
		"list-id":               mk("List-Id: <list.example.com>"),
		"precedence bulk":       mk("Precedence: bulk"),
		"precedence list":       mk("Precedence: list"),
		"empty return-path":     mk("Return-Path: <>"),
		"x-autorespond":         mk("X-Autorespond: yes"),
		"mailer-daemon sender":  rawMessage("From: Mailer Daemon <mailer-daemon@example.com>", "bounce"),
		"postmaster sender":     rawMessage("From: <postmaster@example.com>", "bounce"),
		"noreply sender":        rawMessage("From: <noreply@example.com>", "auto"),
	}
	for name, raw := range auto {
		msg, err := readMail(raw)
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if !isAutoSubmitted(msg) {
			t.Errorf("%s: expected auto-submitted", name)
		}
	}

	human := map[string][]byte{
		"plain":                 mk(""),
		"auto-submitted no":     mk("Auto-Submitted: no"),
		"non-empty return-path": mk("Return-Path: <u@example.com>"),
		"precedence normal":     mk("Precedence: normal"),
	}
	for name, raw := range human {
		msg, err := readMail(raw)
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if isAutoSubmitted(msg) {
			t.Errorf("%s: human message flagged auto-submitted", name)
		}
	}
}

func TestSenderAddress(t *testing.T) {
	cases := []struct {
		headers string
		want    string
	}{
		{"From: User <u@example.com>", "u@example.com"},
		{"From: u@example.com", "u@example.com"},
		{"From: =?UTF-8?Q?Jos=C3=A9?= <jose@example.com>", "jose@example.com"},
		{"Reply-To: r@example.com", "r@example.com"},
		{"From: \r\nReply-To: r@example.com", "r@example.com"},
		{"From: not-an-address", ""},
		{"", ""},
	}
	for _, c := range cases {
		msg, err := readMail(rawMessage(c.headers, "x"))
		if err != nil {
			t.Fatalf("%q: parse: %v", c.headers, err)
		}
		if got := senderAddress(msg); got != c.want {
			t.Errorf("%q: got %q, want %q", c.headers, got, c.want)
		}
	}
}

func TestDecodeHeader(t *testing.T) {
	if got := decodeHeader(""); got != "" {
		t.Errorf("empty: %q", got)
	}
	if got := decodeHeader("=?UTF-8?Q?Re=3A_=5BTicket_JC-1042=5D?="); !strings.Contains(got, "JC-1042") {
		t.Errorf("encoded: %q", got)
	}
	if got := decodeHeader("plain subject"); got != "plain subject" {
		t.Errorf("plain: %q", got)
	}
}

func TestExtractPlainText(t *testing.T) {
	t.Run("7bit plain", func(t *testing.T) {
		msg, _ := readMail(rawMessage(
			"From: u@x.com\r\nContent-Type: text/plain; charset=utf-8",
			"hello there"))
		if got := extractPlainText(msg); got != "hello there" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("base64", func(t *testing.T) {
		msg, _ := readMail(rawMessage(
			"From: u@x.com\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: base64",
			"aGVsbG8gd29ybGQ="))
		if got := extractPlainText(msg); got != "hello world" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("quoted-printable", func(t *testing.T) {
		msg, _ := readMail(rawMessage(
			"From: u@x.com\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable",
			"hello=20qp"))
		if got := extractPlainText(msg); got != "hello qp" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("multipart prefers text/plain", func(t *testing.T) {
		raw := "From: u@x.com\r\nContent-Type: multipart/alternative; boundary=BND\r\n\r\n" +
			"--BND\r\nContent-Type: text/plain\r\n\r\nplain body\r\n" +
			"--BND\r\nContent-Type: text/html\r\n\r\n<p>html body</p>\r\n" +
			"--BND--\r\n"
		msg, _ := readMail([]byte(raw))
		if got := extractPlainText(msg); got != "plain body" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("html-only stripped", func(t *testing.T) {
		raw := "From: u@x.com\r\nContent-Type: multipart/alternative; boundary=BND\r\n\r\n" +
			"--BND\r\nContent-Type: text/html\r\n\r\n<p>para one</p><p>two</p>\r\n" +
			"--BND--\r\n"
		msg, _ := readMail([]byte(raw))
		got := extractPlainText(msg)
		if !strings.Contains(got, "para one") || !strings.Contains(got, "two") {
			t.Errorf("got %q", got)
		}
		if strings.Contains(got, "<p>") {
			t.Errorf("tags not stripped: %q", got)
		}
	})

	t.Run("nested multipart", func(t *testing.T) {
		raw := "From: u@x.com\r\nContent-Type: multipart/mixed; boundary=OUT\r\n\r\n" +
			"--OUT\r\nContent-Type: multipart/alternative; boundary=IN\r\n\r\n" +
			"--IN\r\nContent-Type: text/plain\r\n\r\nnested plain\r\n" +
			"--IN--\r\n" +
			"--OUT\r\nContent-Type: application/pdf\r\n\r\nPDFDATA\r\n" +
			"--OUT--\r\n"
		msg, _ := readMail([]byte(raw))
		if got := extractPlainText(msg); got != "nested plain" {
			t.Errorf("got %q", got)
		}
	})
}

func TestFileSupportMessage(t *testing.T) {
	ctx := context.Background()
	var typeID pgtype.UUID
	_ = typeID.Scan("01900000-0000-7000-8000-000000000001")

	// No dbPool/rdb registered — submitter lookup is skipped, dedup off.
	simple := "From: Someone <someone@example.com>\r\nSubject: Need help\r\n" +
		"Message-ID: <m1@x>\r\nContent-Type: text/plain"

	t.Run("files new ticket", func(t *testing.T) {
		fb := &fakeSupportBridge{}
		out, err := fileSupportMessage(ctx, fb, typeID, "support@japa.to",
			rawMessage(simple, "my site is broken"))
		if err != nil || out != fileTicket {
			t.Fatalf("out=%v err=%v", out, err)
		}
		if len(fb.tickets) != 1 {
			t.Fatalf("expected 1 ticket, got %d", len(fb.tickets))
		}
		in := fb.tickets[0]
		if in.Subject != "Need help" || in.Description != "my site is broken" ||
			in.SubmitterEmail != "someone@example.com" || in.TypeID != typeID {
			t.Fatalf("bad input: %+v", in)
		}
		if in.SubmitterUserID.Valid {
			t.Error("submitter id set without db")
		}
	})

	t.Run("jc-ref without registered sender still files", func(t *testing.T) {
		fb := &fakeSupportBridge{}
		out, err := fileSupportMessage(ctx, fb, typeID, "support@japa.to",
			rawMessage("From: someone@example.com\r\nSubject: Re: JC-1042", "more info"))
		if err != nil || out != fileTicket || len(fb.comments) != 0 {
			t.Fatalf("out=%v err=%v comments=%d", out, err, len(fb.comments))
		}
	})

	t.Run("self-address skipped", func(t *testing.T) {
		fb := &fakeSupportBridge{}
		out, err := fileSupportMessage(ctx, fb, typeID, "support@japa.to",
			rawMessage("From: support@japa.to\r\nSubject: loop", "x"))
		if err != nil || out != fileSkip || len(fb.tickets) != 0 {
			t.Fatalf("out=%v err=%v tickets=%d", out, err, len(fb.tickets))
		}
	})

	t.Run("auto-submitted skipped", func(t *testing.T) {
		fb := &fakeSupportBridge{}
		out, err := fileSupportMessage(ctx, fb, typeID, "support@japa.to",
			rawMessage("From: u@x.com\r\nAuto-Submitted: auto-generated", "x"))
		if err != nil || out != fileSkip || len(fb.tickets) != 0 {
			t.Fatalf("out=%v err=%v tickets=%d", out, err, len(fb.tickets))
		}
	})

	t.Run("malformed skipped", func(t *testing.T) {
		fb := &fakeSupportBridge{}
		out, err := fileSupportMessage(ctx, fb, typeID, "support@japa.to",
			[]byte("\x00\x01\x02"))
		if err != nil || out != fileSkip {
			t.Fatalf("out=%v err=%v", out, err)
		}
	})

	t.Run("bridge error propagates for retry", func(t *testing.T) {
		fb := &fakeSupportBridge{fileErr: errors.New("db down")}
		out, err := fileSupportMessage(ctx, fb, typeID, "support@japa.to",
			rawMessage(simple, "body"))
		if err == nil || out != fileTicket {
			t.Fatalf("out=%v err=%v", out, err)
		}
	})
}

func TestFileSupportMessageDedup(t *testing.T) {
	ctx := context.Background()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey client: %v", err)
	}
	t.Cleanup(vk.Close)

	RegisterRedis(valkeycompat.NewAdapter(vk))
	t.Cleanup(func() { RegisterRedis(nil) })

	var typeID pgtype.UUID
	_ = typeID.Scan("01900000-0000-7000-8000-000000000001")

	raw := rawMessage("From: u@x.com\r\nSubject: dup test\r\nMessage-ID: <dup@x>", "body")

	t.Run("same message-id skipped on refetch", func(t *testing.T) {
		fb := &fakeSupportBridge{}
		out, err := fileSupportMessage(ctx, fb, typeID, "", raw)
		if err != nil || out != fileTicket {
			t.Fatalf("first: out=%v err=%v", out, err)
		}
		out, err = fileSupportMessage(ctx, fb, typeID, "", raw)
		if err != nil || out != fileSkip {
			t.Fatalf("second: out=%v err=%v", out, err)
		}
		if len(fb.tickets) != 1 {
			t.Fatalf("filed %d tickets for one message", len(fb.tickets))
		}
	})

	t.Run("filing error clears marker for retry", func(t *testing.T) {
		bad := rawMessage("From: u@x.com\r\nSubject: retry\r\nMessage-ID: <retry@x>", "body")
		fb := &fakeSupportBridge{fileErr: errors.New("db down")}
		if _, err := fileSupportMessage(ctx, fb, typeID, "", bad); err == nil {
			t.Fatal("expected error")
		}
		fb.fileErr = nil
		out, err := fileSupportMessage(ctx, fb, typeID, "", bad)
		if err != nil || out != fileTicket || len(fb.tickets) != 1 {
			t.Fatalf("retry: out=%v err=%v tickets=%d", out, err, len(fb.tickets))
		}
	})
}

// readMail parses a raw message for tests.
func readMail(raw []byte) (*mail.Message, error) {
	return mail.ReadMessage(bytes.NewReader(raw))
}
