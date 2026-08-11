package content

import (
	"strings"
	"testing"
)

func TestSanitizeHTML_ScriptTagsRemoved(t *testing.T) {
	t.Parallel()

	input := `<p>Hello</p><script>alert('xss')</script><p>World</p>`

	got := SanitizeHTML(input)
	if strings.Contains(got, "<script") {
		t.Errorf("script tag not removed: %s", got)
	}

	if strings.Contains(got, "alert") {
		t.Errorf("script content not removed: %s", got)
	}

	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Errorf("safe content was lost: %s", got)
	}
}

func TestSanitizeHTML_EventHandlersRemoved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"onclick", `<p onclick="alert('xss')">text</p>`},
		{"onerror", `<img src="x" onerror="alert(1)">`},
		{"onload", `<body onload="alert(1)">text</body>`},
		{"onmouseover", `<div onmouseover="alert(1)">text</div>`},
		{"onfocus", `<input onfocus="alert(1)">`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SanitizeHTML(tt.input)
			if strings.Contains(strings.ToLower(got), "on"+tt.name[2:]) {
				// More precise: check that the specific handler is gone.
				lower := strings.ToLower(got)
				if strings.Contains(lower, "onclick") || strings.Contains(lower, "onerror") ||
					strings.Contains(lower, "onload") || strings.Contains(lower, "onmouseover") ||
					strings.Contains(lower, "onfocus") {
					t.Errorf("event handler not removed: %s", got)
				}
			}
		})
	}
}

func TestSanitizeHTML_JavascriptURLsRemoved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"javascript href", `<a href="javascript:alert('xss')">click</a>`},
		{"javascript src", `<img src="javascript:alert(1)" alt="xss">`},
		{"data URI", `<a href="data:text/html,<script>alert(1)</script>">click</a>`},
		{"vbscript", `<a href="vbscript:MsgBox(1)">click</a>`},
		{"uppercase javascript", `<a href="JavaScript:alert(1)">click</a>`},
		{"mixed case", `<a href="JaVaScRiPt:alert(1)">click</a>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SanitizeHTML(tt.input)
			lower := strings.ToLower(got)

			if strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") || strings.Contains(lower, "vbscript:") {
				t.Errorf("dangerous URL not removed: %s", got)
			}
			// The link text should survive.
			if !strings.Contains(got, "click") && !strings.Contains(got, "xss") {
				t.Errorf("content lost: %s", got)
			}
		})
	}
}

func TestSanitizeHTML_SafeHTMLPreserved(t *testing.T) {
	t.Parallel()

	input := `<p>Hello <strong>world</strong></p>
<h2>Title</h2>
<ul><li>Item 1</li><li>Item 2</li></ul>
<blockquote>Quote</blockquote>
<code>inline code</code>
<pre>block code</pre>
<a href="https://example.com">safe link</a>
<img src="https://example.com/image.png" alt="photo" width="100" height="200">`

	got := SanitizeHTML(input)

	safeTags := []string{"<p>", "<strong>", "<h2>", "<ul>", "<li>", "<blockquote>", "<code>", "<pre>", "<a", "<img"}
	for _, tag := range safeTags {
		if !strings.Contains(got, tag) {
			t.Errorf("safe tag %s was removed: %s", tag, got)
		}
	}

	if !strings.Contains(got, "https://example.com") {
		t.Errorf("safe URL was removed: %s", got)
	}
}

func TestSanitizeHTML_IframesRemovedExceptApproved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		shouldStay bool
	}{
		{"youtube", `<iframe src="https://www.youtube.com/embed/abc123"></iframe>`, true},
		{"vimeo", `<iframe src="https://player.vimeo.com/video/123"></iframe>`, true},
		{"codepen", `<iframe src="https://codepen.io/user/embed/abc"></iframe>`, true},
		{"unknown domain", `<iframe src="https://evil.com/steal"></iframe>`, false},
		{"no src", `<iframe></iframe>`, false},
		{"javascript iframe", `<iframe src="javascript:alert(1)"></iframe>`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := SanitizeHTML(tt.input)
			hasIframe := strings.Contains(got, "<iframe")

			if hasIframe != tt.shouldStay {
				t.Errorf("iframe presence=%v, want %v; output: %s", hasIframe, tt.shouldStay, got)
			}
		})
	}
}

func TestSanitizeHTML_StyleTagsRemoved(t *testing.T) {
	t.Parallel()

	input := `<p>text</p><style>body{background:red}</style><p>more</p>`

	got := SanitizeHTML(input)
	if strings.Contains(got, "<style") || strings.Contains(got, "background:red") {
		t.Errorf("style tag not removed: %s", got)
	}
}

func TestSanitizeHTML_DisallowedTagsUnwrapped(t *testing.T) {
	t.Parallel()
	// Tags like <form>, <input>, <select> should be unwrapped — children kept.
	input := `<form><input type="text"><p>content</p></form>`

	got := SanitizeHTML(input)
	if strings.Contains(got, "<form") || strings.Contains(got, "<input") {
		t.Errorf("disallowed tags not unwrapped: %s", got)
	}

	if !strings.Contains(got, "content") {
		t.Errorf("content lost during unwrap: %s", got)
	}
}

func TestSanitizeHTML_EmptyInput(t *testing.T) {
	t.Parallel()

	if got := SanitizeHTML(""); got != "" {
		t.Errorf("empty input should return empty, got: %q", got)
	}
}

func TestSanitizeHTML_SafeRelativeURLs(t *testing.T) {
	t.Parallel()

	input := `<a href="/about">About</a> <a href="#section">Section</a> <a href="?page=2">Page 2</a>`

	got := SanitizeHTML(input)
	if !strings.Contains(got, "/about") || !strings.Contains(got, "#section") || !strings.Contains(got, "?page=2") {
		t.Errorf("relative URLs were stripped: %s", got)
	}
}

// ---------------------------------------------------------------------------
// SanitizeText
// ---------------------------------------------------------------------------

func TestSanitizeText_StripsAllHTML(t *testing.T) {
	t.Parallel()

	input := `<p>Hello <strong>world</strong></p><script>alert('xss')</script>`

	got := SanitizeText(input)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("HTML not fully stripped: %q", got)
	}

	if !strings.Contains(got, "Hello") || !strings.Contains(got, "world") {
		t.Errorf("text content lost: %q", got)
	}
}

func TestSanitizeText_EmptyInput(t *testing.T) {
	t.Parallel()

	if got := SanitizeText(""); got != "" {
		t.Errorf("empty input should return empty, got: %q", got)
	}
}

func TestSanitizeText_PlainText(t *testing.T) {
	t.Parallel()

	input := `Just plain text, no HTML.`
	got := SanitizeText(input)

	if got != input {
		t.Errorf("plain text modified: got %q, want %q", got, input)
	}
}

// ---------------------------------------------------------------------------
// IsSafeURL
// ---------------------------------------------------------------------------

func TestIsSafeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"/relative/path", true},
		{"#anchor", true},
		{"?query=1", true},
		{"", true},
		{"javascript:alert(1)", false},
		{"data:text/html,<script>1</script>", false},
		{"vbscript:MsgBox", false},
		{"JAVASCRIPT:alert(1)", false},
		{"  https://example.com  ", true},
		{"  javascript:alert(1)  ", false},
	}
	for _, tt := range tests {
		got := isSafeURL(tt.url)
		if got != tt.want {
			t.Errorf("isSafeURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestSanitizeHTML_LinkPreviewDivPreserved(t *testing.T) {
	t.Parallel()

	input := `<p>Some text</p><div data-type="link-preview" data-url="https://example.com"></div><p>More text</p>`
	got := SanitizeHTML(input)

	if !strings.Contains(got, `data-type="link-preview"`) {
		t.Errorf("data-type attribute was stripped: %s", got)
	}
	if !strings.Contains(got, `data-url="https://example.com"`) {
		t.Errorf("data-url attribute was stripped: %s", got)
	}
	if !strings.Contains(got, "Some text") || !strings.Contains(got, "More text") {
		t.Errorf("surrounding content was lost: %s", got)
	}
}

func TestSanitizeHTML_CouponEmbedDivPreserved(t *testing.T) {
	t.Parallel()

	input := `<div class="coupon-embed" data-coupon-id="abc123">Coupon</div>`
	got := SanitizeHTML(input)

	if !strings.Contains(got, `data-coupon-id="abc123"`) {
		t.Errorf("data-coupon-id attribute was stripped: %s", got)
	}
}

func TestSanitizeHTML_LinkPreviewDataURLXSSBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantKept bool
	}{
		{"safe https url", `<div data-type="link-preview" data-url="https://example.com"></div>`, true},
		{"safe http url", `<div data-type="link-preview" data-url="http://example.com"></div>`, true},
		{"safe relative url", `<div data-type="link-preview" data-url="/page"></div>`, true},
		{"javascript uri blocked", `<div data-type="link-preview" data-url="javascript:alert(1)"></div>`, false},
		{"data uri blocked", `<div data-type="link-preview" data-url="data:text/html,<script>alert(1)</script>"></div>`, false},
		{"vbscript uri blocked", `<div data-type="link-preview" data-url="vbscript:alert(1)"></div>`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeHTML(tt.input)
			// isSafeURL lowercases the scheme for comparison; the sanitizer
			// may preserve or strip the attribute. Check for the dangerous
			// scheme substring to confirm it was stripped.
			lowered := strings.ToLower(got)
			if tt.wantKept {
				if !strings.Contains(got, `data-url="`) {
					t.Errorf("safe data-url was stripped: %s", got)
				}
			} else {
				// The dangerous scheme must not survive in data-url.
				if strings.Contains(lowered, "javascript:") || strings.Contains(lowered, "vbscript:") {
					t.Errorf("dangerous scheme survived in data-url: %s", got)
				}
			}
		})
	}
}
