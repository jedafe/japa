package textprep

import (
	"strings"
	"testing"
)

const realScholarshipHTML = `<!DOCTYPE html><html><head><title>DAAD Scholarships</title>
<style>.nav{display:none}</style><script>var x = 1;</script></head>
<body><nav>Menu links everywhere</nav><main><h1>DAAD Scholarship Program</h1>
<p>Funding for international students in Germany. Amount: €500/month. Deadline: 2026-12-31.</p>
<script>track();</script><style>.x{color:red}</style></main></body></html>`

func TestSourceText_StripsMarkupAndKeepsContent(t *testing.T) {
	t.Parallel()

	got := SourceText(realScholarshipHTML, "text/html", 0)
	for _, want := range []string{"DAAD Scholarship Program", "€500/month", "2026-12-31"} {
		if !strings.Contains(got, want) {
			t.Errorf("SourceText missing %q in %q", want, got)
		}
	}

	for _, unwanted := range []string{"display:none", "var x = 1", "track()", "color:red", "<script", "<style"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("SourceText leaked markup/script %q in %q", unwanted, got)
		}
	}
}

func TestSourceText_NonHTMLPassesThrough(t *testing.T) {
	t.Parallel()

	rss := `<?xml version="1.0"?><rss><channel><item><title>Item</title></item></channel></rss>`
	if got := SourceText(rss, "application/rss+xml", 0); got != rss {
		t.Errorf("RSS body was modified: %q", got)
	}

	jsonBody := `{"key": "value"}`
	if got := SourceText(jsonBody, "application/json", 0); got != jsonBody {
		t.Errorf("JSON body was modified: %q", got)
	}
}

func TestSourceText_LengthCap(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("word ", 50000) // ~250k chars

	got := SourceText(long, "text/plain", 1000)
	if len([]rune(got)) > 1000 {
		t.Errorf("SourceText length = %d, want <= 1000", len([]rune(got)))
	}
}

func TestIsSoft404(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		body   string
		ct     string
		status int
		want   bool
	}{
		{"hard 404", `<html><body>anything</body></html>`, "text/html", 404, true},
		{"nairaland soft 404", `<html><head><title>404: Page Not Found</title></head><body><h1>404: Page Not Found.</h1><p>Nairaland Forum Welcome, Guest</p></body></html>`, "text/html", 200, true},
		{"german error page", `<html><head><title>Ein Fehler ist aufgetreten</title></head><body><h1>404: Ein Fehler ist aufgetreten.</h1><p>content</p></body></html>`, "text/html", 200, true},
		{"real scholarship page", realScholarshipHTML, "text/html", 200, false},
		{"non-html rss", `<?xml version="1.0"?><rss><channel><title>404</title><item/></channel></rss>`, "application/rss+xml", 200, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := IsSoft404(tc.body, tc.ct, tc.status); got != tc.want {
				t.Errorf("IsSoft404 = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasExtractableText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		ct   string
		want bool
	}{
		{"real page", realScholarshipHTML, "text/html", true},
		{"js shell", `<html><head><title>App</title></head><body><div id="root"></div><script>boot()</script></body></html>`, "text/html", false},
		{"empty body", ``, "text/html", false},
		{"plain text ok", "Some plain text body", "text/plain", true},
		{"empty plain", "  ", "text/plain", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := HasExtractableText(tc.body, tc.ct); got != tc.want {
				t.Errorf("HasExtractableText = %v, want %v", got, tc.want)
			}
		})
	}
}
