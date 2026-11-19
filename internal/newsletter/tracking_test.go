package newsletter

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/email"
)

func TestIsTrackingScanner(t *testing.T) {
	t.Setenv("JC_NEWSLETTER_TRACKING_IGNORE_UA", "")

	cases := []struct {
		ua   string
		want bool
	}{
		{"GoogleImageProxy; (+http://www.google.com/ggp)", true},
		{"Mozilla/5.0 (compatible; Proofpoint Link Scanner)", true},
		{"Barracuda Networks", true},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15", false},
		{"", false},
		{"okhttp/4.12.0", false},
	}

	for _, tc := range cases {
		if got := IsTrackingScanner(tc.ua); got != tc.want {
			t.Errorf("IsTrackingScanner(%q) = %v, want %v", tc.ua, got, tc.want)
		}
	}
}

func TestIsTrackingScannerEnvExtension(t *testing.T) {
	t.Setenv("JC_NEWSLETTER_TRACKING_IGNORE_UA", " CorpGW ,, ")

	if !IsTrackingScanner("corpgw/2.1 link-check") {
		t.Error("env-supplied substring should match case-insensitively")
	}

	if IsTrackingScanner("Mozilla/5.0") {
		t.Error("normal UA must not match")
	}
}

// testHandlers builds Handlers over a zero-value Service. Enabled() fails
// closed (nil checker + nil settings → feature.newsletter off), so
// RecordTrackEvent is gated out — if it ran, the nil db would panic,
// which is exactly what the flag-off tests prove can't happen.
func testHandlers(t *testing.T) *Handlers {
	t.Helper()
	t.Setenv("JC_NEWSLETTER_TRACKING_SECRET", "test-secret")

	return NewHandlers(&Service{})
}

func TestHandleTrackOpenServesPixelOnInvalidSig(t *testing.T) {
	h := testHandlers(t)

	r := httptest.NewRequest("GET", "/newsletter/track/open?c=x&e=y&s=forged", nil)
	w := httptest.NewRecorder()

	h.HandleTrackOpen(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 — pixel must render even for forged sigs", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "image/gif" {
		t.Errorf("Content-Type = %q, want image/gif", ct)
	}

	if w.Body.Len() == 0 {
		t.Error("empty body — pixel bytes missing")
	}
}

func TestHandleTrackOpenCountsSkippedWhenFlagOff(t *testing.T) {
	h := testHandlers(t)

	cid, rid := "11111111-2222-3333-4444-555555555555", "abc123"
	sig := email.SignTracking("test-secret", TrackScopeOpen, cid, rid)

	q := url.Values{"c": {cid}, "e": {rid}, "s": {sig}}
	r := httptest.NewRequest("GET", "/newsletter/track/open?"+q.Encode(), nil)
	w := httptest.NewRecorder()

	// Valid signature but feature.newsletter off — counting is skipped
	// (a nil-db panic would fail the test), pixel still served.
	h.HandleTrackOpen(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandleTrackClickRejectsForgedSig(t *testing.T) {
	h := testHandlers(t)

	r := httptest.NewRequest("GET", "/newsletter/track/click?c=x&e=y&u=https%3A%2F%2Fevil.test&s=forged", nil)
	w := httptest.NewRecorder()

	h.HandleTrackClick(w, r)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400 — forged click must not redirect", w.Code)
	}
}

func TestHandleTrackClickRejectsNonHTTPDestination(t *testing.T) {
	h := testHandlers(t)

	cid, rid, dest := "11111111-2222-3333-4444-555555555555", "abc123", "ftp://files.test/x"
	sig := email.SignTracking("test-secret", TrackScopeClick, cid, rid, dest)

	q := url.Values{"c": {cid}, "e": {rid}, "u": {dest}, "s": {sig}}
	r := httptest.NewRequest("GET", "/newsletter/track/click?"+q.Encode(), nil)
	w := httptest.NewRecorder()

	// Signature is valid over the ftp dest — the scheme whitelist is the
	// second gate that must still reject.
	h.HandleTrackClick(w, r)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400 — signed non-http(s) dest must not redirect", w.Code)
	}
}

func TestHandleTrackClickRedirectsWhenFlagOff(t *testing.T) {
	h := testHandlers(t)

	cid, rid, dest := "11111111-2222-3333-4444-555555555555", "abc123", "https://japa.to/article"
	sig := email.SignTracking("test-secret", TrackScopeClick, cid, rid, dest)

	q := url.Values{"c": {cid}, "e": {rid}, "u": {dest}, "s": {sig}}
	r := httptest.NewRequest("GET", "/newsletter/track/click?"+q.Encode(), nil)
	w := httptest.NewRecorder()

	// feature.newsletter off → counting skipped, redirect still runs so
	// already-sent links keep working.
	h.HandleTrackClick(w, r)

	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}

	if loc := w.Header().Get("Location"); loc != dest {
		t.Errorf("Location = %q, want %q", loc, dest)
	}
}

func TestHandleUnsubscribeRedirectPassesToken(t *testing.T) {
	h := NewHandlers(&Service{})
	t.Setenv("JC_SITE_URL", "https://japa.test")

	r := httptest.NewRequest("GET", "/newsletter/unsubscribe?token=tok123", nil)
	w := httptest.NewRecorder()

	h.HandleUnsubscribeRedirect(w, r)

	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://japa.test/unsubscribe?token=tok123") {
		t.Errorf("Location = %q, want frontend page carrying the token", loc)
	}
}
