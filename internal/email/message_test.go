package email

import (
	"strings"
	"testing"
)

func TestBuildRFC822PlainTextSinglePart(t *testing.T) {
	t.Parallel()

	msg := string(buildRFC822("f@x.com", "t@x.com", "hi", EmailRequest{
		Body: "hello",
	}))

	if !strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8") {
		t.Fatalf("missing plain content type:\n%s", msg)
	}

	if strings.Contains(msg, "multipart/alternative") {
		t.Fatalf("plain request must stay single-part:\n%s", msg)
	}
}

func TestBuildRFC822HTMLCarriesTextAlternative(t *testing.T) {
	t.Parallel()

	msg := string(buildRFC822("f@x.com", "t@x.com", "hi", EmailRequest{
		IsHTML: true,
		Body:   `<p>Hi Ada,</p><p><a href="https://x.com/v">Verify</a></p>`,
	}))

	if !strings.Contains(msg, "Content-Type: multipart/alternative; boundary=") {
		t.Fatalf("HTML message must be multipart/alternative:\n%s", msg)
	}

	textIdx := strings.Index(msg, "Content-Type: text/plain; charset=UTF-8")
	htmlIdx := strings.Index(msg, "Content-Type: text/html; charset=UTF-8")

	if textIdx < 0 || htmlIdx < 0 {
		t.Fatalf("missing alternative parts:\n%s", msg)
	}

	// RFC 2046 §5.1.4: plain part precedes HTML part so readers pick the
	// last type they can render.
	if textIdx > htmlIdx {
		t.Fatalf("text/plain must precede text/html:\n%s", msg)
	}

	// The derived text part carries readable content, not markup.
	if !strings.Contains(msg, "Verify <https://x.com/v>") {
		t.Fatalf("derived text part missing link destination:\n%s", msg)
	}
}

func TestBuildRFC822PrefersExplicitTextBody(t *testing.T) {
	t.Parallel()

	msg := string(buildRFC822("f@x.com", "t@x.com", "hi", EmailRequest{
		IsHTML:   true,
		Body:     `<p>ignored for text</p>`,
		TextBody: "explicit text part",
	}))

	if !strings.Contains(msg, "explicit text part") {
		t.Fatalf("explicit TextBody not used:\n%s", msg)
	}

	if strings.Contains(msg, "ignored for text") && !strings.Contains(msg, "<p>ignored for text</p>") {
		t.Fatalf("unexpected content:\n%s", msg)
	}
}

func TestParseAddressRepresentations(t *testing.T) {
	t.Parallel()

	parsed, err := parseAddress("JapaConnect <noreply@japa.to>")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Header form preserves the display name.
	if parsed.String() != `"JapaConnect" <noreply@japa.to>` {
		t.Fatalf("String() = %q", parsed.String())
	}

	// Envelope form is the bare mailbox.
	if parsed.Address != "noreply@japa.to" {
		t.Fatalf("Address = %q", parsed.Address)
	}

	// A bare address renders identically in both forms — no regression for
	// existing configs without a display name.
	parsed, err = parseAddress("noreply@japa.to")
	if err != nil {
		t.Fatalf("parse bare: %v", err)
	}

	if headerAddress(parsed) != "noreply@japa.to" {
		t.Fatalf("bare headerAddress() = %q", headerAddress(parsed))
	}

	if _, err := parseAddress("not an address"); err == nil {
		t.Fatal("invalid address must error")
	}
}
