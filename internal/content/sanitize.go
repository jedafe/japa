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
	"strings"

	"golang.org/x/net/html"
)

// allowedTags is the set of HTML tags permitted in user content.
var allowedTags = map[string]bool{
	"p": true, "br": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"strong": true, "em": true, "u": true, "s": true,
	"a":      true,
	"ul":     true, "ol": true, "li": true,
	"blockquote": true,
	"code":       true, "pre": true,
	"img":        true,
	"table":      true, "tr": true, "td": true, "th": true, "thead": true, "tbody": true,
	"div":        true, "span": true,
}

// allowedAttributes is the set of attributes permitted per tag.
// The "*" key applies to all tags; specific keys override for that tag.
var allowedAttributes = map[string]map[string]bool{
	"*": {
		"class": true, "id": true,
	},
	"a": {
		"href": true, "title": true, "target": true, "rel": true,
	},
	"img": {
		"src": true, "alt": true, "title": true, "width": true, "height": true,
	},
}

// approvedIframeHosts is the allowlist of domains permitted in <iframe> src.
var approvedIframeHosts = []string{
	"youtube.com", "www.youtube.com", "youtu.be",
	"vimeo.com", "www.vimeo.com",
	"codepen.io", "www.codepen.io",
	"jsfiddle.net",
	"embed.ted.com",
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
	html.Render(&buf, doc)
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

		// Validate URL attributes.
		if attrName == "href" || attrName == "src" {
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
// approved embed platform.
func isApprovedIframe(n *html.Node) bool {
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "src" {
			src := strings.ToLower(strings.TrimSpace(attr.Val))
			for _, host := range approvedIframeHosts {
				if strings.Contains(src, host) {
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
		io.WriteString(w, n.Data)
	}
	child := n.FirstChild
	for child != nil {
		extractText(w, child)
		child = child.NextSibling
	}
}
