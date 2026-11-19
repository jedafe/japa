package worker

import (
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/email"
)

const testTrackingSecret = "test-tracking-secret"

const testAPIBase = "https://api.japa.test/api/v1"

func setTrackingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JC_NEWSLETTER_TRACKING_SECRET", testTrackingSecret)
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

	out := injectNewsletterTracking(testAPIBase, cid, recipient, body)

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
	if q.Get("c") != cid.String() || q.Get("u") != "https://japa.to/a" {
		t.Errorf("click params wrong: %s", q.Encode())
	}

	// e is the opaque recipient tag — it must carry no raw address and
	// the signature must verify over the tag as transmitted.
	if rid := q.Get("e"); rid == "" || rid == recipient || len(rid) != 32 {
		t.Errorf("e param = %q, want 32-char opaque tag (not %q)", rid, recipient)
	}

	if !email.VerifyTracking(testTrackingSecret, q.Get("s"), "click", cid.String(), q.Get("e"), "https://japa.to/a") {
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
	if pq.Get("e") == recipient {
		t.Error("open pixel e param leaks the raw recipient address")
	}

	if !email.VerifyTracking(testTrackingSecret, pq.Get("s"), "open", cid.String(), pq.Get("e")) {
		t.Error("open signature does not verify")
	}
}

func TestInjectNewsletterTrackingSkipsWithoutSecret(t *testing.T) {
	t.Setenv("JC_NEWSLETTER_TRACKING_SECRET", "")
	t.Setenv("JC_NEWSLETTER_WEBHOOK_SECRET", "")

	body := `<p>Hi <a href="https://japa.to/a">link</a></p>`
	if got := injectNewsletterTracking(testAPIBase, testCampaignID(t), "r@x.test", body); got != body {
		t.Error("body must pass through unchanged when no secret is configured")
	}
}

func TestInjectNewsletterTrackingSkipsInvalidCampaign(t *testing.T) {
	setTrackingEnv(t)

	body := `<p>Hi <a href="https://japa.to/a">link</a></p>`
	if got := injectNewsletterTracking(testAPIBase, pgtype.UUID{}, "r@x.test", body); got != body {
		t.Error("body must pass through unchanged when campaign ID is invalid")
	}
}

func TestNewsletterAPIBaseAppendsAPIPrefix(t *testing.T) {
	// JC_API_PUBLIC_URL holds the API origin only (same convention as
	// media.ServeURL) — the routes base must gain /api/v1 or every
	// machine-targeted URL (List-Unsubscribe header, tracking links)
	// 404s.
	t.Setenv("JC_API_PUBLIC_URL", "https://api.japa.to")

	if got := newsletterAPIBase("https://japa.to"); got != "https://api.japa.to/api/v1" {
		t.Errorf("env branch = %q, want origin + /api/v1", got)
	}
}

func TestNewsletterAPIBaseSiteFallback(t *testing.T) {
	t.Setenv("JC_API_PUBLIC_URL", "")

	if got := newsletterAPIBase("https://japa.to/"); got != "https://japa.to/api/v1" {
		t.Errorf("site fallback = %q, want site base + /api/v1", got)
	}

	if got := newsletterAPIBase(""); got != "" {
		t.Errorf("no site base = %q, want empty", got)
	}
}

func TestNewsletterPollBouncesQueueTier(t *testing.T) {
	t.Parallel()

	if got := QueueForTask(TypeNewsletterPollBounces); got != QueueLow {
		t.Errorf("QueueForTask(%s) = %q, want %q", TypeNewsletterPollBounces, got, QueueLow)
	}
}
