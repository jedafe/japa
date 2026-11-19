package worker

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/email"
)

const testTrackingSecret = "test-tracking-secret"

func setTrackingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JC_NEWSLETTER_TRACKING_SECRET", testTrackingSecret)
	t.Setenv("JC_API_PUBLIC_URL", "https://api.japa.test/api/v1")
}

func testCampaignID(t *testing.T) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan("11111111-2222-3333-4444-555555555555"); err != nil {
		t.Fatalf("campaign id scan: %v", err)
	}

	return id
}

func TestNewsletterTrackingSecretResolution(t *testing.T) {
	// JC_NEWSLETTER_TRACKING_SECRET wins when both are set.
	t.Setenv("JC_NEWSLETTER_TRACKING_SECRET", "primary")
	t.Setenv("JC_NEWSLETTER_WEBHOOK_SECRET", "fallback")

	if got := newsletterTrackingSecret(); got != "primary" {
		t.Errorf("primary secret not preferred, got %q", got)
	}
}

func TestNewsletterTrackingSecretFallsBackToWebhook(t *testing.T) {
	t.Setenv("JC_NEWSLETTER_TRACKING_SECRET", "")
	t.Setenv("JC_NEWSLETTER_WEBHOOK_SECRET", "fallback")

	if got := newsletterTrackingSecret(); got != "fallback" {
		t.Errorf("webhook secret fallback not used, got %q", got)
	}
}

func TestInjectNewsletterTrackingWrapsLinksAndPixel(t *testing.T) {
	setTrackingEnv(t)

	cid := testCampaignID(t)
	recipient := "reader@example.com"
	body := `<p>Hi <a href="https://japa.to/a">link</a></p>`

	out := injectNewsletterTracking(context.Background(), cid, recipient, body)

	// The link is wrapped through the click endpoint with a valid signature.
	idx := strings.Index(out, `href="`)
	if idx < 0 {
		t.Fatalf("no link in output: %s", out)
	}

	rest := out[idx+len(`href="`):]
	wrapped := rest[:strings.Index(rest, `"`)]

	u, err := url.Parse(strings.ReplaceAll(wrapped, "&amp;", "&"))
	if err != nil {
		t.Fatalf("wrapped href not a URL: %v", err)
	}

	if u.Path != "/api/v1/newsletter/track/click" {
		t.Errorf("click endpoint path = %q", u.Path)
	}

	q := u.Query()
	if q.Get("c") != cid.String() || q.Get("e") != recipient || q.Get("u") != "https://japa.to/a" {
		t.Errorf("click params wrong: %s", q.Encode())
	}

	if !email.VerifyTracking(testTrackingSecret, q.Get("s"), "click", cid.String(), recipient, "https://japa.to/a") {
		t.Error("click signature does not verify")
	}

	// The open pixel is appended with a valid open signature.
	pi := strings.Index(out, `<img src="`)
	if pi < 0 {
		t.Fatalf("open pixel missing: %s", out)
	}

	prest := out[pi+len(`<img src="`):]
	pixelURL := prest[:strings.Index(prest, `"`)]

	pu, err := url.Parse(pixelURL)
	if err != nil {
		t.Fatalf("pixel src not a URL: %v", err)
	}

	if pu.Path != "/api/v1/newsletter/track/open" {
		t.Errorf("open endpoint path = %q", pu.Path)
	}

	pq := pu.Query()
	if !email.VerifyTracking(testTrackingSecret, pq.Get("s"), "open", cid.String(), recipient) {
		t.Error("open signature does not verify")
	}
}

func TestInjectNewsletterTrackingSkipsWithoutSecret(t *testing.T) {
	t.Setenv("JC_NEWSLETTER_TRACKING_SECRET", "")
	t.Setenv("JC_NEWSLETTER_WEBHOOK_SECRET", "")
	t.Setenv("JC_API_PUBLIC_URL", "https://api.japa.test/api/v1")

	body := `<p>Hi <a href="https://japa.to/a">link</a></p>`
	if got := injectNewsletterTracking(context.Background(), testCampaignID(t), "r@x.test", body); got != body {
		t.Error("body must pass through unchanged when no secret is configured")
	}
}

func TestNewsletterPollBouncesQueueTier(t *testing.T) {
	t.Parallel()

	if got := QueueForTask(TypeNewsletterPollBounces); got != QueueLow {
		t.Errorf("QueueForTask(%s) = %q, want %q", TypeNewsletterPollBounces, got, QueueLow)
	}
}
