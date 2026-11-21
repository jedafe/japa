// Package email — message.go renders the RFC 5322 message shared by the
// SMTP provider and the SES raw-MIME provider so both emit identical
// bytes: sanitized base headers in a stable order, extra headers sorted
// alphabetically, and a body that is single-part text/plain or
// multipart/alternative carrying a text/plain part ahead of the
// text/html part (the plain part must come first per RFC 2046 §5.1.4).
package email

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sort"
	"strings"
)

// buildRFC822 renders the outgoing message. from/to are RFC 5322 header
// values (a display name is preserved — the caller passes the parsed
// address's .String() form); envelope addresses are a separate concern of
// the caller. Every interpolated header value is CRLF-sanitized (CWE-113).
func buildRFC822(from, to, subject string, req EmailRequest) []byte {
	var hdr strings.Builder

	hdr.WriteString("From: " + from + "\r\n")
	hdr.WriteString("To: " + to + "\r\n")
	hdr.WriteString("Subject: " + sanitizeHeaderValue(subject) + "\r\n")
	hdr.WriteString("MIME-Version: 1.0\r\n")

	var body string

	if req.IsHTML {
		boundary := multipartBoundary(from, to, subject, req)
		hdr.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")

		var parts strings.Builder

		writePart := func(contentType, content string) {
			parts.WriteString("--" + boundary + "\r\n")
			parts.WriteString("Content-Type: " + contentType + "\r\n")
			parts.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
			parts.WriteString(content + "\r\n")
		}

		// Plain part first: MIME readers pick the LAST part they can
		// render, so text/plain must precede text/html.
		writePart("text/plain; charset=UTF-8", strings.TrimSpace(req.TextAlternative()))
		writePart("text/html; charset=UTF-8", req.Body)

		parts.WriteString("--" + boundary + "--\r\n")
		body = parts.String()
	} else {
		hdr.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		hdr.WriteString("Content-Transfer-Encoding: 8bit\r\n")
		body = req.Body
	}

	// Merge optional extra headers (e.g. List-Unsubscribe) in a stable
	// (alphabetical) order so the rendered message is deterministic.
	// Header names are restricted to token chars; values are sanitized.
	if len(req.Headers) > 0 {
		keys := make([]string, 0, len(req.Headers))
		for k := range req.Headers {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			name := sanitizeHeaderValue(k)
			if name == "" || strings.ContainsAny(name, " :") {
				slog.Warn("email_send: dropping invalid header name")
				continue
			}

			hdr.WriteString(name + ": " + sanitizeHeaderValue(req.Headers[k]) + "\r\n")
		}
	}

	return []byte(hdr.String() + "\r\n" + body)
}

// multipartBoundary derives a deterministic per-message boundary from the
// message content — randomness would break deterministic output for tests
// and debugging, and a fixed string could collide with boundary text
// appearing inside a quoted body. A content hash is collision-safe
// within the single message it scopes.
func multipartBoundary(from, to, subject string, req EmailRequest) string {
	h := sha256.New()
	h.Write([]byte(from + "|" + to + "|" + subject + "|" + req.Body + "|" + req.TextAlternative()))

	return "=_japa_" + hex.EncodeToString(h.Sum(nil))[:24]
}
