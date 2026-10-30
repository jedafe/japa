// Package content — HTML sanitization for user-generated content.
//
// Uses a whitelist-based approach: only explicitly allowed tags and
// attributes survive. All <script>, event handlers (onclick, onerror,
// etc.), javascript: URLs, and <iframe> tags are stripped unless the
// iframe source matches an approved embed platform.
package content

import (
	"bytes"
	"io"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// allowedTags is the set of HTML tags permitted in user content.
var allowedTags = map[string]bool{
	"p": true, "br": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"strong": true, "em": true, "u": true, "s": true,
	"a":  true,
	"ul": true, "ol": true, "li": true,
	"blockquote": true,
	"code":       true, "pre": true,
	"img":   true,
	"table": true, "tr": true, "td": true, "th": true, "thead": true, "tbody": true,
	"div": true, "span": true,
	// §45 FAQ accordion block — native <details>/<summary> markup.
	"details": true, "summary": true,
}

// allowedAttributes is the set of attributes permitted per tag.
// The "*" key applies to all tags; specific keys override for that tag.
var allowedAttributes = map[string]map[string]bool{
	"*": {
		"class": true, "id": true,
	},
	"a": {
		"href": true, "title": true, "target": true, "rel": true,
		"data-affiliate-provider-id": true,
	},
	"img": {
		"src": true, "alt": true, "title": true, "width": true, "height": true,
	},
	"div": {
		"data-type": true, "data-url": true, "data-coupon-id": true,
		"data-display-mode": true,
		// §14 list amendment: internal content embed placeholder target.
		"data-content-slug": true,
		// §106 video: hydrateVideoEmbeds selects .video-embed[data-video-source-id]
		// — without this the attr is stripped and video embeds in rich text
		// bodies (including list items) render as dead placeholders.
		"data-video-source-id": true,
	},
	"span": {
		// Mention nodes (Section 15): data-type, data-id, data-label,
		// data-handle (username for profile link), data-entity-type.
		// data-url carries the link target for hashtag/interlink spans —
		// it is scheme-validated below the same as href/src.
		"data-type":        true,
		"data-id":          true,
		"data-label":       true,
		"data-handle":      true,
		"data-entity-type": true,
		"data-url":         true,
	},
	// §45 FAQ accordion block — data-type="faq-item" lets the TipTap
	// faqItem node parse stored HTML back into the document.
	"details": {
		// data-type marks faq-item nodes; name="jc-faq" gives the published
		// accordion native single-open semantics; open is harmless.
		"data-type": true, "open": true, "name": true,
	},
	"summary": {
		"data-type": true,
	},
}

// approvedIframeHosts is the default allowlist of domains permitted in
// <iframe> src. It can be overridden at runtime via SetAllowedIframeHosts
// (BL-014: sanitization rules configurable via system_settings).
var approvedIframeHosts = []string{
	"youtube.com", "www.youtube.com", "youtu.be",
	"vimeo.com", "www.vimeo.com",
	"codepen.io", "www.codepen.io",
	"jsfiddle.net",
	"embed.ted.com",
}

// iframeHostsMu guards the runtime-overridable iframe host allowlist
// (BL-014). Reads in isApprovedIframe acquire a read lock; SetAllowedIframeHosts
// acquires a write lock.
var iframeHostsMu sync.RWMutex

// SetAllowedIframeHosts replaces the iframe embed allowlist at runtime
// (BL-014 / Section 16: sanitization rules configurable via system_settings).
// Pass nil/empty to restore the compiled-in defaults. The list is matched
// case-insensitively as a substring of the iframe src URL, matching the
// existing isApprovedIframe semantics.
func SetAllowedIframeHosts(hosts []string) {
	iframeHostsMu.Lock()
	defer iframeHostsMu.Unlock()

	if len(hosts) == 0 {
		approvedIframeHosts = []string{
			"youtube.com", "www.youtube.com", "youtu.be",
			"vimeo.com", "www.vimeo.com",
			"codepen.io", "www.codepen.io",
			"jsfiddle.net",
			"embed.ted.com",
		}

		return
	}

	approvedIframeHosts = hosts
}

// currentIframeHosts returns a snapshot of the active iframe host allowlist
// under the read lock (BL-014).
func currentIframeHosts() []string {
	iframeHostsMu.RLock()
	defer iframeHostsMu.RUnlock()

	return approvedIframeHosts
}

// SanitizeHTML processes user-generated HTML through a whitelist-based
// sanitizer. It strips disallowed tags, removes event handlers and
// javascript: URLs, and validates href/src against safe protocols.
func SanitizeHTML(raw string) string {
	if raw == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return SanitizeText(raw)
	}

	sanitizeNode(doc)

	var buf bytes.Buffer

	_ = html.Render(&buf, doc)

	return buf.String()
}

// SanitizeText strips all HTML tags, returning only the text content.
func SanitizeText(text string) string {
	if text == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return text
	}

	var buf bytes.Buffer

	extractText(&buf, doc)

	return buf.String()
}

// sanitizeNode processes a single node and recursively processes children.
// Children are snapshot into a slice before iteration so tree mutations
// (removal, unwrapping) don't cause skipped or double-visited nodes.
func sanitizeNode(n *html.Node) {
	// Snapshot children before any mutations.
	children := collectChildren(n)

	// Process this node if it's an element.
	if n.Type == html.ElementNode {
		tagName := strings.ToLower(n.Data)

		if !allowedTags[tagName] {
			switch tagName {
			case "script", "style":
				// Remove entirely — no children survive.
				removeNode(n)
				return
			case "iframe":
				if !isApprovedIframe(n) {
					removeNode(n)
					return
				}
				// Approved: keep, fall through to sanitize attributes.
				sanitizeAttributes(n)
			default:
				// Unwrap: replace node with its children in-place.
				unwrapNode(n)
				// After unwrap, n's children are now siblings in the parent.
				// The snapshot `children` still holds the original children;
				// we process them below. Since they're now children of n's
				// parent, they'll be visited when the parent re-iterates.
				// But to avoid double-processing, just recurse directly.
				for _, child := range children {
					sanitizeNode(child)
				}

				return
			}
		} else {
			// Allowed tag: sanitize its attributes.
			sanitizeAttributes(n)
		}
	}

	// Process children (snapshot prevents mutation issues).
	for _, child := range children {
		sanitizeNode(child)
	}
}

// collectChildren returns a snapshot of a node's children as a slice.
func collectChildren(n *html.Node) []*html.Node {
	var children []*html.Node
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		children = append(children, child)
	}

	return children
}

// sanitizeAttributes removes disallowed attributes from a node and
// validates URLs in href and src attributes.
func sanitizeAttributes(n *html.Node) {
	tagName := strings.ToLower(n.Data)
	globalAttrs := allowedAttributes["*"]
	tagAttrs := allowedAttributes[tagName]

	var filtered []html.Attribute

	for _, attr := range n.Attr {
		attrName := strings.ToLower(attr.Key)

		// Reject any event handler (onclick, onerror, etc.).
		if strings.HasPrefix(attrName, "on") {
			continue
		}

		// Check if allowed globally or for this specific tag.
		allowed := globalAttrs[attrName]
		if !allowed && tagAttrs != nil {
			allowed = tagAttrs[attrName]
		}

		if !allowed {
			continue
		}

		// Validate URL attributes. data-url is used by the link-preview
		// TipTap node (Section 15) and is rendered as <a href> on view
		// pages, so it must be scheme-validated the same as href/src to
		// prevent stored XSS via javascript: URIs.
		if attrName == "href" || attrName == "src" || attrName == "data-url" {
			if !isSafeURL(attr.Val) {
				continue
			}
		}

		filtered = append(filtered, attr)
	}

	n.Attr = filtered
}

// isSafeURL returns true if the URL uses http, https, or is a relative path.
// It rejects javascript:, data:, vbscript: and other dangerous schemes.
func isSafeURL(url string) bool {
	url = strings.TrimSpace(strings.ToLower(url))

	// Relative URLs are safe.
	if url == "" || strings.HasPrefix(url, "/") || strings.HasPrefix(url, "#") || strings.HasPrefix(url, "?") {
		return true
	}

	// Only allow http and https schemes.
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return true
	}

	return false
}

// isApprovedIframe checks whether an <iframe> node's src points to an
// approved embed platform. The allowlist is runtime-configurable via
// SetAllowedIframeHosts (BL-014). The src host must exactly match an
// allowed host or be a direct subdomain of it; substring matching is
// not sufficient to prevent `notyoutube.com` bypasses (BUG-016).
func isApprovedIframe(n *html.Node) bool {
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "src" {
			src := strings.TrimSpace(attr.Val)

			u, err := url.Parse(src)
			if err != nil {
				return false
			}

			host := strings.ToLower(u.Host)

			for _, allowed := range currentIframeHosts() {
				allowed = strings.ToLower(strings.TrimSpace(allowed))
				if allowed == "" {
					continue
				}

				if host == allowed {
					return true
				}
				// Allow direct subdomains, e.g. www.youtube.com for youtube.com.
				if strings.HasSuffix(host, "."+allowed) {
					return true
				}
			}

			return false
		}
	}

	return false
}

// removeNode detaches a node from the tree. Its children are discarded.
func removeNode(n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
}

// unwrapNode replaces a node with its children, preserving their content.
func unwrapNode(n *html.Node) {
	if n.Parent == nil {
		return
	}

	parent := n.Parent
	nextSibling := n.NextSibling

	// Collect children first (to avoid mutation during iteration).
	var children []*html.Node

	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		children = append(children, child)
		child = next
	}

	// Detach all children from the current node, then insert into parent.
	for _, child := range children {
		n.RemoveChild(child)
		parent.InsertBefore(child, nextSibling)
	}

	parent.RemoveChild(n)
}

// extractText recursively extracts only text content from an HTML tree,
// discarding all markup.
func extractText(w io.Writer, n *html.Node) {
	if n.Type == html.TextNode {
		_, _ = io.WriteString(w, n.Data)
	}

	child := n.FirstChild
	for child != nil {
		extractText(w, child)
		child = child.NextSibling
	}
}
