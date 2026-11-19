package email

import (
	"strings"
	"testing"
)

func TestSignTrackingRoundTrip(t *testing.T) {
	t.Parallel()

	sig := SignTracking("secret", "open", "camp-1", "user@example.com")
	if sig == "" {
		t.Fatal("SignTracking returned empty signature")
	}

	if !VerifyTracking("secret", sig, "open", "camp-1", "user@example.com") {
		t.Error("VerifyTracking rejected a valid signature")
	}
}

func TestVerifyTrackingRejectsMismatches(t *testing.T) {
	t.Parallel()

	sig := SignTracking("secret", "click", "camp-1", "user@example.com", "https://x.test")

	cases := []struct {
		name   string
		secret string
		sig    string
		fields []string
	}{
		{"wrong secret", "other", sig, []string{"click", "camp-1", "user@example.com", "https://x.test"}},
		{"wrong scope", "secret", sig, []string{"open", "camp-1", "user@example.com", "https://x.test"}},
		{"different url", "secret", sig, []string{"click", "camp-1", "user@example.com", "https://evil.test"}},
		{"empty sig", "secret", "", []string{"click", "camp-1"}},
		{"empty secret", "", sig, []string{"click", "camp-1"}},
	}

	for _, tc := range cases {
		if VerifyTracking(tc.secret, tc.sig, tc.fields...) {
			t.Errorf("VerifyTracking accepted %s", tc.name)
		}
	}
}

func TestInjectEmailTrackingRewritesLinksAndAppendsPixel(t *testing.T) {
	t.Parallel()

	body := `<p>Hello <a href="https://japa.to/post/1">read</a> ` +
		`<a href="mailto:a@b.c">mail</a> ` +
		`<a href="#frag">anchor</a></p>`

	out := InjectEmailTracking(body, "https://api.test/track/open?x=1", func(dest string) string {
		return "https://api.test/track/click?u=" + dest
	})

	if !strings.Contains(out, `href="https://api.test/track/click?u=https://japa.to/post/1"`) {
		t.Errorf("http link not wrapped: %s", out)
	}

	if !strings.Contains(out, `href="mailto:a@b.c"`) {
		t.Errorf("mailto link was rewritten: %s", out)
	}

	if !strings.Contains(out, `href="#frag"`) {
		t.Errorf("fragment link was rewritten: %s", out)
	}

	if !strings.Contains(out, `<img src="https://api.test/track/open?x=1"`) {
		t.Errorf("open pixel missing: %s", out)
	}
}

func TestInjectEmailTrackingPreservesFragment(t *testing.T) {
	t.Parallel()

	body := `<p>text</p>`
	out := InjectEmailTracking(body, "", func(dest string) string { return dest })

	if strings.Contains(out, "<html>") || strings.Contains(out, "<body>") {
		t.Errorf("fragment gained a document wrapper: %s", out)
	}
}

func TestInjectEmailTrackingSkips(t *testing.T) {
	t.Parallel()

	if got := InjectEmailTracking("", "px", func(d string) string { return d }); got != "" {
		t.Error("empty body should pass through")
	}

	if got := InjectEmailTracking("<p>x</p>", "px", nil); got != "<p>x</p>" {
		t.Error("nil rewrite should pass through")
	}
}
