package worker

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
)

func TestReadBounceIMAPConfigUnset(t *testing.T) {
	t.Setenv("JC_BOUNCE_IMAP_HOST", "")

	if cfg := readBounceIMAPConfig(); cfg != nil {
		t.Error("config must be nil when JC_BOUNCE_IMAP_HOST is unset")
	}
}

func TestReadBounceIMAPConfigDefaults(t *testing.T) {
	t.Setenv("JC_BOUNCE_IMAP_HOST", "imap.japa.test")
	t.Setenv("JC_BOUNCE_IMAP_PORT", "")
	t.Setenv("JC_BOUNCE_IMAP_MAILBOX", "")
	t.Setenv("JC_BOUNCE_IMAP_TLS", "")
	t.Setenv("JC_BOUNCE_IMAP_BATCH", "")

	cfg := readBounceIMAPConfig()
	if cfg == nil {
		t.Fatal("config nil")
	}

	if cfg.port != "993" || cfg.mailbox != "INBOX" || !cfg.tls || cfg.batch != 200 {
		t.Errorf("defaults wrong: %+v", cfg)
	}
}

const dsnMessage = "From: MAILER-DAEMON@mta.example\r\n" +
	"Content-Type: multipart/report; report-type=delivery-status; boundary=\"b1\"\r\n" +
	"\r\n" +
	"--b1\r\n" +
	"Content-Type: text/plain; charset=us-ascii\r\n" +
	"\r\n" +
	"This is a delivery status notification.\r\n" +
	"\r\n" +
	"--b1\r\n" +
	"Content-Type: message/delivery-status\r\n" +
	"\r\n" +
	"Reporting-MTA: dns; mta.example\r\n" +
	"\r\n" +
	"Final-Recipient: rfc822;Gone@Example.com\r\n" +
	"Action: failed\r\n" +
	"Status: 5.1.1\r\n" +
	"\r\n" +
	"Final-Recipient: rfc822;deferred@example.com\r\n" +
	"Action: delayed\r\n" +
	"Status: 4.4.1\r\n" +
	"\r\n" +
	"--b1--\r\n"

func TestParseBounceEventsDSN(t *testing.T) {
	t.Parallel()

	events := parseBounceEvents([]byte(dsnMessage))
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (soft bounce must be ignored): %+v", len(events), events)
	}

	if events[0].email != "gone@example.com" || events[0].kind != "bounced" {
		t.Errorf("event = %+v, want gone@example.com bounced", events[0])
	}
}

const arfMessage = "From: fbl@isp.example\r\n" +
	"Content-Type: multipart/report; report-type=feedback-report; boundary=\"b2\"\r\n" +
	"\r\n" +
	"--b2\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"Complaint.\r\n" +
	"\r\n" +
	"--b2\r\n" +
	"Content-Type: message/feedback-report\r\n" +
	"\r\n" +
	"Feedback-Type: abuse\r\n" +
	"Original-Rcpt-To: victim@example.com\r\n" +
	"\r\n" +
	"--b2--\r\n"

func TestParseBounceEventsARF(t *testing.T) {
	t.Parallel()

	events := parseBounceEvents([]byte(arfMessage))
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %+v", len(events), events)
	}

	if events[0].email != "victim@example.com" || events[0].kind != "complained" {
		t.Errorf("event = %+v, want victim@example.com complained", events[0])
	}
}

func TestParseBounceEventsXFailedRecipients(t *testing.T) {
	t.Parallel()

	raw := "From: bounce@mta.example\r\n" +
		"X-Failed-Recipients: a@example.com, B@Example.com\r\n" +
		"\r\nbody\r\n"

	events := parseBounceEvents([]byte(raw))
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2: %+v", len(events), events)
	}

	for _, ev := range events {
		if ev.kind != "bounced" {
			t.Errorf("kind = %q, want bounced", ev.kind)
		}
	}

	if events[1].email != "b@example.com" {
		t.Errorf("address not normalized: %q", events[1].email)
	}
}

func TestParseBounceEventsDedupes(t *testing.T) {
	t.Parallel()

	raw := "From: bounce@mta.example\r\n" +
		"X-Failed-Recipients: dup@example.com\r\n" +
		"Content-Type: multipart/report; report-type=delivery-status; boundary=\"b3\"\r\n" +
		"\r\n" +
		"--b3\r\n" +
		"Content-Type: message/delivery-status\r\n" +
		"\r\n" +
		"Reporting-MTA: dns; mta\r\n" +
		"\r\n" +
		"Final-Recipient: rfc822;dup@example.com\r\n" +
		"Action: failed\r\n" +
		"Status: 5.0.0\r\n" +
		"\r\n" +
		"--b3--\r\n"

	events := parseBounceEvents([]byte(raw))
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (dedup): %+v", len(events), events)
	}
}

func TestParseBounceEventsGarbage(t *testing.T) {
	t.Parallel()

	if got := parseBounceEvents(nil); got != nil {
		t.Errorf("nil input: got %v", got)
	}

	if got := parseBounceEvents([]byte("not a message")); got != nil {
		t.Errorf("garbage input: got %v", got)
	}
}

func TestHandleNewsletterPollBouncesNoDB(t *testing.T) {
	old := dbPool
	dbPool = nil
	t.Cleanup(func() { dbPool = old })

	// Two runs must both no-op cleanly — the handler is safe to retry.
	task := asynq.NewTask(TypeNewsletterPollBounces, nil)

	for i := 0; i < 2; i++ {
		if err := HandleNewsletterPollBounces(context.Background(), task); err != nil {
			t.Fatalf("run %d returned error: %v", i, err)
		}
	}
}
