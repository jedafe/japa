// Package email — tracking.go provides the native open/click tracking
// primitives used when the marketing transport is a self-hosted MTA over
// plain SMTP (§62 self-hosted transport, ADR-015/ADR-080). Provider APIs
// emit engagement webhooks; a bare MTA does not, so the sender rewrites
// campaign HTML at send time (links wrapped through the click endpoint,
// 1px open pixel appended) and signs every tracking URL with HMAC-SHA256
// so the endpoints can't be forged or turned into open redirects.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// SignTracking produces the hex HMAC-SHA256 signature over the joined
// fields. Callers pass a scope discriminator first (e.g. "open", "click")
// so a signature minted for one purpose can never verify for another.
func SignTracking(secret string, fields ...string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join(fields, "|")))

	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyTracking reports whether sig matches SignTracking over the same
// fields — constant-time so the compare leaks no timing side channel.
func VerifyTracking(secret, sig string, fields ...string) bool {
	if secret == "" || sig == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(sig), []byte(SignTracking(secret, fields...))) == 1
}

// InjectEmailTracking rewrites an HTML email body for native engagement
// tracking: every <a href> whose scheme is http(s) is passed through
// rewriteURL (which returns the wrapped tracking URL, or the input
// unchanged to leave the link alone), and a 1px transparent <img> open
// pixel is appended. Fragment bodies are preserved as fragments —
// ParseFragment is used so a partial template doesn't gain an
// <html><head><body> wrapper.
//
// The caller's rewriteURL is responsible for exclusions: the one-click
// unsubscribe URL must NOT be wrapped (a POSTing mail client would hit
// the tracker instead of the unsubscribe endpoint), so callers inject
// tracking before appending the unsubscribe footer.
func InjectEmailTracking(htmlBody, pixelURL string, rewriteURL func(dest string) string) string {
	if strings.TrimSpace(htmlBody) == "" || rewriteURL == nil {
		return htmlBody
	}

	ctx := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: atom.Body.String()}

	nodes, err := html.ParseFragment(strings.NewReader(htmlBody), ctx)
	if err != nil || len(nodes) == 0 {
		return htmlBody
	}

	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.A {
			for i, attr := range n.Attr {
				if attr.Key == "href" && isTrackableLink(attr.Val) {
					n.Attr[i].Val = rewriteURL(attr.Val)
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	var out strings.Builder

	for _, n := range nodes {
		walk(n)

		if err := html.Render(&out, n); err != nil {
			return htmlBody
		}
	}

	body := out.String()

	if pixelURL != "" {
		body += `<img src="` + pixelURL + `" width="1" height="1" alt="" style="display:none" />`
	}

	return body
}

// isTrackableLink reports whether an href value is a remote link worth
// wrapping — http(s) only, case-insensitive with surrounding whitespace
// tolerated (mail clients normalize both). mailto/tel/anchors and
// scheme-relative URLs are left alone.
func isTrackableLink(href string) bool {
	l := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}
