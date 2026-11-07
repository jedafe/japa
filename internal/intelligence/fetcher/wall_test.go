package fetcher

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDetectWall covers the shared bot-wall classifier: vendor challenge
// pages (Cloudflare, DataDome, PerimeterX, Incapsula, Akamai, Sucuri),
// deceptive 200s, decisive headers, and the negative cases — clean content,
// non-HTML bodies, and prose that merely mentions vendors must never fire.
func TestDetectWall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		headers     http.Header
		body        string
		contentType string
		wantWall    bool
		wantSigPart string
	}{
		{
			name:   "cloudflare wait page on 200",
			status: 200,
			body: `<!DOCTYPE html><html><head><title>Just a moment...</title></head>
				<body>Checking your connection — cloudflare</body></html>`,
			contentType: "text/html",
			wantWall:    true,
			wantSigPart: "cloudflare",
		},
		{
			name:        "cloudflare challenge internals on 403",
			status:      403,
			body:        `<html><body><script>window._cf_chl_opt={cvId:'2'};</script></body></html>`,
			contentType: "text/html",
			wantWall:    true,
			wantSigPart: "cloudflare",
		},
		{
			name:     "cf-mitigated header alone",
			status:   403,
			headers:  http.Header{"Cf-Mitigated": {"challenge"}},
			wantWall: true, wantSigPart: "cloudflare",
		},
		{
			name:     "datadome header on 403",
			status:   403,
			headers:  http.Header{"X-Datadome": {"protected"}},
			wantWall: true, wantSigPart: "datadome",
		},
		{
			name:        "datadome captcha body",
			status:      403,
			body:        `<html><body><script src="https://geo.captcha-delivery.com/captcha/?initialCid=x"></script></body></html>`,
			contentType: "text/html",
			wantWall:    true,
			wantSigPart: "datadome",
		},
		{
			name:        "perimeterx block text",
			status:      403,
			body:        `<html><body><h1>Pardon Our Interruption</h1><script src="/px/captcha.js"></script></body></html>`,
			contentType: "text/html",
			wantWall:    true,
			wantSigPart: "perimeterx",
		},
		{
			name:        "akamai reference deny page",
			status:      403,
			body:        `<html><body><h1>Access Denied</h1><p>Reference #18.4e2cb8af.1719876543.1a2b3c4</p></body></html>`,
			contentType: "text/html",
			wantWall:    true,
			wantSigPart: "akamai",
		},
		{
			name:        "incapsula incident page",
			status:      403,
			body:        `<html><body>Request unsuccessful. Incapsula incident ID: 12345</body></html>`,
			contentType: "text/html",
			wantWall:    true,
			wantSigPart: "incapsula",
		},
		{
			name:        "sucuri firewall page",
			status:      403,
			body:        `<html><body><h1>Access Denied - Sucuri Website Firewall</h1></body></html>`,
			contentType: "text/html",
			wantWall:    true,
			wantSigPart: "sucuri",
		},
		{
			name:        "generic verify-human page on 503",
			status:      503,
			body:        `<html><body><h1>Verify you are human</h1><form></form></body></html>`,
			contentType: "text/html; charset=utf-8",
			wantWall:    true,
			wantSigPart: "verify-human",
		},
		{
			name:        "wall body mislabeled as octet-stream is sniffed",
			status:      200,
			body:        `<!DOCTYPE html><html><body>Checking your browser before accessing</body></html>`,
			contentType: "application/octet-stream",
			wantWall:    true,
			wantSigPart: "cloudflare",
		},
		// Negative cases — a wall false-positive would wrongly classify a
		// healthy fetch as blocked, so these matter as much as the hits.
		{
			name:        "clean article page",
			status:      200,
			body:        `<!DOCTYPE html><html><body><article><h1>Chevening Scholarships 2026</h1><p>Applications open for the human challenge of relocating abroad.</p></article></body></html>`,
			contentType: "text/html",
			wantWall:    false,
		},
		{
			name:        "prose mentioning vendors is not a wall",
			status:      200,
			body:        `<html><body><p>Cloudflare, DataDome and PerimeterX are bot-mitigation vendors.</p></body></html>`,
			contentType: "text/html",
			wantWall:    false,
		},
		{
			name:        "json body with challenge word is not scanned",
			status:      200,
			body:        `{"challenge": "accepted", "captcha": false, "perimeterx": 1}`,
			contentType: "application/json",
			wantWall:    false,
		},
		{
			name:        "empty body",
			status:      200,
			body:        "",
			contentType: "text/html",
			wantWall:    false,
		},
		{
			name:     "x-datadome on a 200 is not decisive",
			status:   200,
			headers:  http.Header{"X-Datadome": {"protected"}},
			body:     `<html><body><p>real content</p></body></html>`,
			wantWall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sig, ok := DetectWall(tt.status, tt.headers, []byte(tt.body), tt.contentType)
			if ok != tt.wantWall {
				t.Fatalf("DetectWall = (%q, %v), want wall=%v", sig, ok, tt.wantWall)
			}

			if ok && tt.wantSigPart != "" && !strings.Contains(sig, tt.wantSigPart) {
				t.Errorf("signature = %q, want containing %q", sig, tt.wantSigPart)
			}
		})
	}
}

// TestDetectWall_Fixtures runs the detector over the captured vendor wall
// pages in testdata/ — full-page HTML, not signature snippets — plus the
// thin-but-legitimate page that must never fire. Vendor markup evolves;
// a fixture that stops matching is a regression in the signature set.
func TestDetectWall_Fixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file        string
		status      int
		wantWall    bool
		wantSigPart string
	}{
		{"wall_cloudflare.html", 200, true, "cloudflare"},
		{"wall_datadome.html", 403, true, "datadome"},
		{"wall_perimeterx.html", 403, true, "perimeterx"},
		{"wall_incapsula.html", 403, true, "incapsula"},
		{"wall_akamai.html", 403, true, "akamai"},
		{"legit_thin.html", 200, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			sig, ok := DetectWall(tt.status, nil, body, "text/html")
			if ok != tt.wantWall {
				t.Fatalf("DetectWall(%s) = (%q, %v), want wall=%v", tt.file, sig, ok, tt.wantWall)
			}

			if ok && tt.wantSigPart != "" && !strings.Contains(sig, tt.wantSigPart) {
				t.Errorf("signature = %q, want containing %q", sig, tt.wantSigPart)
			}
		})
	}
}

// TestCollapseConfig_LooksCollapsed covers the poison guard's bounds: the
// prior capture must be substantial, the new one both proportionally thin
// and absolutely small, and the body must be HTML-ish.
func TestCollapseConfig_LooksCollapsed(t *testing.T) {
	t.Parallel()

	cfg := DefaultCollapseConfig // 20KB min prior, 15% ratio, 8KB cap

	html := func(n int) []byte {
		b := make([]byte, n)
		copy(b, "<html><body>")
		return b
	}

	tests := []struct {
		name        string
		prevLen     int64
		contentType string
		body        []byte
		want        bool
	}{
		{"wall stub vs 100KB page", 100 * 1024, "text/html", html(3 * 1024), true},
		{"octet-stream mislabel sniffs html", 100 * 1024, "application/octet-stream", html(3 * 1024), true},
		{"boundary: under ratio", 20480, "text/html", html(2900), true},
		{"boundary: at ratio", 20480, "text/html", html(3072), false},
		{"slim but not collapsed (10KB)", 100 * 1024, "text/html", html(10 * 1024), false},
		{"no substantial baseline", 5 * 1024, "text/html", html(1024), false},
		{"json body never collapses", 100 * 1024, "application/json", []byte(`{"a":1}`), false},
		{"manifest body never collapses", 100 * 1024, "application/x-intel-manifest", []byte("http://a\t\n"), false},
		{"empty body", 100 * 1024, "text/html", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := cfg.LooksCollapsed(tt.prevLen, tt.contentType, tt.body); got != tt.want {
				t.Errorf("LooksCollapsed(%d, %q, %d bytes) = %v, want %v",
					tt.prevLen, tt.contentType, len(tt.body), got, tt.want)
			}
		})
	}
}
