package email

import (
	"strings"
	"testing"
)

func TestHTMLToTextStripsMarkupAndKeepsLinkDestinations(t *testing.T) {
	t.Parallel()

	body := `<!DOCTYPE html><html><head><title>hidden title</title>
<style>p{color:red}</style></head><body>
<p>Hi Ada,</p>
<p>Click <a href="https://example.com/verify?token=abc">Verify Email</a> to continue.</p>
<p>Raw link: <a href="https://example.com/raw">https://example.com/raw</a></p>
<script>alert("nope")</script>
</body></html>`

	text := HTMLToText(body)

	for _, want := range []string{"Hi Ada,", "Verify Email <https://example.com/verify?token=abc>"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}

	// A bare-URL link renders once, not "url <url>".
	if strings.Contains(text, "example.com/raw>") || strings.Count(text, "example.com/raw") != 1 {
		t.Errorf("bare-URL link should render the URL once, got:\n%s", text)
	}

	for _, banned := range []string{"hidden title", "alert(", "color:red", "<p>"} {
		if strings.Contains(text, banned) {
			t.Errorf("non-text content leaked %q in:\n%s", banned, text)
		}
	}
}

func TestHTMLToTextBulletsAndWhitespace(t *testing.T) {
	t.Parallel()

	text := HTMLToText(`<div><ul><li>one</li><li>two</li></ul><p>a   b</p><br><p>c</p></div>`)

	if !strings.Contains(text, "- one") || !strings.Contains(text, "- two") {
		t.Fatalf("list items not bulleted:\n%s", text)
	}

	if !strings.Contains(text, "a b") {
		t.Fatalf("intra-line whitespace not collapsed:\n%s", text)
	}

	if strings.Contains(text, "\n\n\n") {
		t.Fatalf("blank runs not collapsed:\n%q", text)
	}
}

func TestHTMLToTextImageAltAndEmpty(t *testing.T) {
	t.Parallel()

	if got := HTMLToText(""); got != "" {
		t.Fatalf("empty input: got %q", got)
	}

	text := HTMLToText(`<p><img src="logo.png" alt="JapaConnect"></p>`)
	if !strings.Contains(text, "JapaConnect") {
		t.Fatalf("img alt text missing:\n%s", text)
	}
}

func TestTextAlternativePrecedence(t *testing.T) {
	t.Parallel()

	html := `<p>Hello <b>world</b></p>`

	// Non-HTML request: no alternative part exists.
	if got := (EmailRequest{Body: "plain"}).TextAlternative(); got != "" {
		t.Fatalf("plain request: got %q", got)
	}

	// Explicit TextBody wins over derivation.
	got := (EmailRequest{IsHTML: true, Body: html, TextBody: "explicit"}).TextAlternative()
	if got != "explicit" {
		t.Fatalf("explicit TextBody: got %q", got)
	}

	// Empty TextBody falls back to HTML derivation.
	got = (EmailRequest{IsHTML: true, Body: html}).TextAlternative()
	if got != "Hello world" {
		t.Fatalf("derived text: got %q", got)
	}
}
