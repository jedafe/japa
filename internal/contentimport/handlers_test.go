// Package contentimport — handlers_test.go tests the HTTP handlers using
// httptest. Includes auth denial tests, source type validation, and
// HTML-to-TipTap conversion tests per the testing expectations.
//
// Ref: japaconnect-engineering/sections/65-content-import.md
//      japaconnect-engineering/adr/ADR-083-content-import.md
package contentimport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Auth denial tests — no auth context set → 401.
// The RBAC middleware is applied at the route registration site, so these
// tests verify the handler's own auth check via requireUser.
// ---------------------------------------------------------------------------

// TestHandleStartImport_Unauthorized verifies that the start import
// endpoint requires authentication.
func TestHandleStartImport_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/me/imports", nil)
	rr := httptest.NewRecorder()
	h.HandleStartImport(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleStartImport without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleListImports_Unauthorized verifies that the list imports
// endpoint requires authentication.
func TestHandleListImports_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/me/imports", nil)
	rr := httptest.NewRecorder()
	h.HandleListImports(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleListImports without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleGetImport_Unauthorized verifies that the get import endpoint
// requires authentication.
func TestHandleGetImport_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/me/imports/123", nil)
	rr := httptest.NewRecorder()
	h.HandleGetImport(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleGetImport without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminListImports_Unauthorized verifies that the admin list
// imports endpoint requires authentication.
func TestHandleAdminListImports_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/imports", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminListImports(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminListImports without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminRetryImport_Unauthorized verifies that the admin retry
// import endpoint requires authentication.
func TestHandleAdminRetryImport_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/imports/123/retry", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminRetryImport(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminRetryImport without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Source type validation tests (table-driven).
// ---------------------------------------------------------------------------

// TestSourceTypeValidation tests the isValidSourceType function with valid
// and invalid source types. The content_imports CHECK constraint allows
// only 'medium', 'substack', 'rss', and 'zip'.
func TestSourceTypeValidation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValid bool
	}{
		{"medium is valid", "medium", true},
		{"substack is valid", "substack", true},
		{"rss is valid", "rss", true},
		{"zip is valid", "zip", true},
		{"empty is invalid", "", false},
		{"wordpress is invalid", "wordpress", false},
		{"ghost is invalid", "ghost", false},
		{"blogger is invalid", "blogger", false},
		{"uppercase medium is invalid", "MEDIUM", false},
		{"mixed case is invalid", "Medium", false},
		{"random string is invalid", "foobar", false},
		{"medium with spaces is invalid", "medium ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidSourceType(tt.input)
			if got != tt.wantValid {
				t.Errorf("isValidSourceType(%q) = %v, want %v",
					tt.input, got, tt.wantValid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ConvertHTMLToTipTap tests.
// ---------------------------------------------------------------------------

// TestConvertHTMLToTipTap tests the basic HTML-to-TipTap conversion for
// paragraphs, headings, lists, and images.
func TestConvertHTMLToTipTap(t *testing.T) {
	t.Run("empty input returns empty doc", func(t *testing.T) {
		result, err := ConvertHTMLToTipTap("")
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap(\"\") error: %v", err)
		}

		var doc map[string]interface{}
		if err := json.Unmarshal(result, &doc); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}

		if doc["type"] != "doc" {
			t.Errorf("type = %v, want doc", doc["type"])
		}

		content, ok := doc["content"].([]interface{})
		if !ok {
			t.Fatal("content is not an array")
		}
		if len(content) != 0 {
			t.Errorf("content length = %d, want 0", len(content))
		}
	})

	t.Run("paragraph", func(t *testing.T) {
		html := "<p>Hello world</p>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 1 {
			t.Fatalf("content length = %d, want 1", len(content))
		}

		node := content[0].(map[string]interface{})
		if node["type"] != "paragraph" {
			t.Errorf("node type = %v, want paragraph", node["type"])
		}

		text := extractTextFromNode(node)
		if text != "Hello world" {
			t.Errorf("text = %q, want %q", text, "Hello world")
		}
	})

	t.Run("multiple paragraphs", func(t *testing.T) {
		html := "<p>First paragraph</p><p>Second paragraph</p>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 2 {
			t.Fatalf("content length = %d, want 2", len(content))
		}

		for i, expected := range []string{"First paragraph", "Second paragraph"} {
			node := content[i].(map[string]interface{})
			if node["type"] != "paragraph" {
				t.Errorf("node %d type = %v, want paragraph", i, node["type"])
			}
			text := extractTextFromNode(node)
			if text != expected {
				t.Errorf("node %d text = %q, want %q", i, text, expected)
			}
		}
	})

	t.Run("heading h1", func(t *testing.T) {
		html := "<h1>Main Title</h1>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 1 {
			t.Fatalf("content length = %d, want 1", len(content))
		}

		node := content[0].(map[string]interface{})
		if node["type"] != "heading" {
			t.Errorf("node type = %v, want heading", node["type"])
		}

		attrs, ok := node["attrs"].(map[string]interface{})
		if !ok {
			t.Fatal("attrs is not a map")
		}
		if level, ok := attrs["level"].(float64); !ok || int(level) != 1 {
			t.Errorf("heading level = %v, want 1", attrs["level"])
		}

		text := extractTextFromNode(node)
		if text != "Main Title" {
			t.Errorf("text = %q, want %q", text, "Main Title")
		}
	})

	t.Run("heading h2", func(t *testing.T) {
		html := "<h2>Subtitle</h2>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 1 {
			t.Fatalf("content length = %d, want 1", len(content))
		}

		node := content[0].(map[string]interface{})
		if node["type"] != "heading" {
			t.Errorf("node type = %v, want heading", node["type"])
		}

		attrs := node["attrs"].(map[string]interface{})
		if level, ok := attrs["level"].(float64); !ok || int(level) != 2 {
			t.Errorf("heading level = %v, want 2", attrs["level"])
		}
	})

	t.Run("heading h3", func(t *testing.T) {
		html := "<h3>Section Title</h3>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		node := content[0].(map[string]interface{})
		attrs := node["attrs"].(map[string]interface{})
		if level, ok := attrs["level"].(float64); !ok || int(level) != 3 {
			t.Errorf("heading level = %v, want 3", attrs["level"])
		}
	})

	t.Run("unordered list", func(t *testing.T) {
		html := "<ul><li>First item</li><li>Second item</li></ul>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 1 {
			t.Fatalf("content length = %d, want 1", len(content))
		}

		node := content[0].(map[string]interface{})
		if node["type"] != "bulletList" {
			t.Errorf("node type = %v, want bulletList", node["type"])
		}

		items, ok := node["content"].([]interface{})
		if !ok {
			t.Fatal("list content is not an array")
		}
		if len(items) != 2 {
			t.Fatalf("list items length = %d, want 2", len(items))
		}

		expected := []string{"First item", "Second item"}
		for i, want := range expected {
			item := items[i].(map[string]interface{})
			if item["type"] != "listItem" {
				t.Errorf("item %d type = %v, want listItem", i, item["type"])
			}
			text := extractTextFromListItem(item)
			if text != want {
				t.Errorf("item %d text = %q, want %q", i, text, want)
			}
		}
	})

	t.Run("ordered list", func(t *testing.T) {
		html := "<ol><li>Step one</li><li>Step two</li></ol>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		node := content[0].(map[string]interface{})
		if node["type"] != "orderedList" {
			t.Errorf("node type = %v, want orderedList", node["type"])
		}
	})

	t.Run("image", func(t *testing.T) {
		html := `<img src="https://example.com/image.jpg" alt="Test image">`
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 1 {
			t.Fatalf("content length = %d, want 1", len(content))
		}

		node := content[0].(map[string]interface{})
		if node["type"] != "image" {
			t.Errorf("node type = %v, want image", node["type"])
		}

		attrs := node["attrs"].(map[string]interface{})
		if attrs["src"] != "https://example.com/image.jpg" {
			t.Errorf("src = %v, want https://example.com/image.jpg", attrs["src"])
		}
		if attrs["alt"] != "Test image" {
			t.Errorf("alt = %v, want Test image", attrs["alt"])
		}
	})

	t.Run("bold text", func(t *testing.T) {
		html := "<p>This is <strong>bold</strong> text</p>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		node := content[0].(map[string]interface{})
		if node["type"] != "paragraph" {
			t.Errorf("node type = %v, want paragraph", node["type"])
		}

		inlineNodes, ok := node["content"].([]interface{})
		if !ok {
			t.Fatal("paragraph content is not an array")
		}
		if len(inlineNodes) < 2 {
			t.Fatalf("inline nodes length = %d, want at least 2", len(inlineNodes))
		}

		// The "bold" text node should have a bold mark.
		foundBold := false
		for _, in := range inlineNodes {
			n := in.(map[string]interface{})
			if n["type"] == "text" && n["text"] == "bold" {
				if marks, ok := n["marks"].([]interface{}); ok && len(marks) > 0 {
					mark := marks[0].(map[string]interface{})
					if mark["type"] == "bold" {
						foundBold = true
					}
				}
			}
		}
		if !foundBold {
			t.Error("expected a text node with bold mark")
		}
	})

	t.Run("italic text", func(t *testing.T) {
		html := "<p>This is <em>italic</em> text</p>"
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		node := content[0].(map[string]interface{})
		inlineNodes := node["content"].([]interface{})

		foundItalic := false
		for _, in := range inlineNodes {
			n := in.(map[string]interface{})
			if n["type"] == "text" && n["text"] == "italic" {
				if marks, ok := n["marks"].([]interface{}); ok && len(marks) > 0 {
					mark := marks[0].(map[string]interface{})
					if mark["type"] == "italic" {
						foundItalic = true
					}
				}
			}
		}
		if !foundItalic {
			t.Error("expected a text node with italic mark")
		}
	})

	t.Run("mixed content", func(t *testing.T) {
		html := `<h1>Title</h1><p>First paragraph</p><h2>Section</h2><p>Second paragraph</p>`
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 4 {
			t.Fatalf("content length = %d, want 4", len(content))
		}

		expectedTypes := []string{"heading", "paragraph", "heading", "paragraph"}
		for i, want := range expectedTypes {
			node := content[i].(map[string]interface{})
			if node["type"] != want {
				t.Errorf("node %d type = %v, want %v", i, node["type"], want)
			}
		}
	})

	t.Run("link", func(t *testing.T) {
		html := `<p><a href="https://example.com">click here</a></p>`
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		node := content[0].(map[string]interface{})
		inlineNodes := node["content"].([]interface{})

		foundLink := false
		for _, in := range inlineNodes {
			n := in.(map[string]interface{})
			if n["type"] == "text" && n["text"] == "click here" {
				if marks, ok := n["marks"].([]interface{}); ok && len(marks) > 0 {
					mark := marks[0].(map[string]interface{})
					if mark["type"] == "link" {
						foundLink = true
						attrs := mark["attrs"].(map[string]interface{})
						if attrs["href"] != "https://example.com" {
							t.Errorf("link href = %v, want https://example.com", attrs["href"])
						}
					}
				}
			}
		}
		if !foundLink {
			t.Error("expected a text node with link mark")
		}
	})

	t.Run("blockquote", func(t *testing.T) {
		html := `<blockquote><p>Quoted text</p></blockquote>`
		result, err := ConvertHTMLToTipTap(html)
		if err != nil {
			t.Fatalf("ConvertHTMLToTipTap error: %v", err)
		}

		content := extractContent(t, result)
		if len(content) != 1 {
			t.Fatalf("content length = %d, want 1", len(content))
		}

		node := content[0].(map[string]interface{})
		if node["type"] != "blockquote" {
			t.Errorf("node type = %v, want blockquote", node["type"])
		}
	})
}

// TestConvertHTMLToTipTap_InvalidHTML verifies that malformed HTML does not
// cause a panic — the parser is lenient and produces a best-effort result.
func TestConvertHTMLToTipTap_InvalidHTML(t *testing.T) {
	html := `<p>unclosed paragraph`
	result, err := ConvertHTMLToTipTap(html)
	if err != nil {
		t.Fatalf("ConvertHTMLToTipTap error: %v", err)
	}

	content := extractContent(t, result)
	if len(content) == 0 {
		t.Error("expected at least one node for unclosed paragraph")
	}
}

// TestParseRSSFeed_EmptyURL verifies that ParseRSSFeed returns an error
// for an empty URL.
func TestParseRSSFeed_EmptyURL(t *testing.T) {
	svc := &Service{}
	_, err := svc.ParseRSSFeed(context.Background(), "")
	if err == nil {
		t.Error("ParseRSSFeed(\"\") expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty feed URL") {
		t.Errorf("ParseRSSFeed(\"\") error = %q, want it to contain 'empty feed URL'", err.Error())
	}
}

// TestParseRSSFeed_BlockedIP verifies that ParseRSSFeed rejects loopback
// addresses (SSRF protection).
func TestParseRSSFeed_BlockedIP(t *testing.T) {
	svc := &Service{}
	_, err := svc.ParseRSSFeed(context.Background(), "http://127.0.0.1/feed")
	if err == nil {
		t.Error("ParseRSSFeed(loopback) expected error, got nil")
	}
}

// TestParseRSSFeed_PrivateIP verifies that ParseRSSFeed rejects private
// IP ranges (SSRF protection).
func TestParseRSSFeed_PrivateIP(t *testing.T) {
	svc := &Service{}
	_, err := svc.ParseRSSFeed(context.Background(), "http://10.0.0.1/feed")
	if err == nil {
		t.Error("ParseRSSFeed(private IP) expected error, got nil")
	}
}

// TestParseRSSFeed_BadScheme verifies that ParseRSSFeed rejects non-http
// schemes.
func TestParseRSSFeed_BadScheme(t *testing.T) {
	svc := &Service{}
	_, err := svc.ParseRSSFeed(context.Background(), "ftp://example.com/feed")
	if err == nil {
		t.Error("ParseRSSFeed(ftp) expected error, got nil")
	}
}

// TestValidateSourceURL verifies the handler-level URL validation.
func TestValidateSourceURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty", "", false},
		{"valid https", "https://medium.com/@user", false},
		{"valid http", "http://example.com/feed.xml", false},
		{"ftp scheme", "ftp://example.com/feed", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"no host", "https:///feed", true},
		{"too long", "https://example.com/" + strings.Repeat("a", 2100), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := validateSourceURL(tt.url)
			if tt.wantErr && msg == "" {
				t.Errorf("validateSourceURL(%q) expected error, got empty", tt.url)
			}
			if !tt.wantErr && msg != "" {
				t.Errorf("validateSourceURL(%q) unexpected error: %s", tt.url, msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// extractContent unmarshals a TipTap JSON doc and returns the content
// array, failing the test if the structure is invalid.
func extractContent(t *testing.T, result json.RawMessage) []interface{} {
	t.Helper()
	var doc map[string]interface{}
	if err := json.Unmarshal(result, &doc); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if doc["type"] != "doc" {
		t.Fatalf("doc type = %v, want doc", doc["type"])
	}
	content, ok := doc["content"].([]interface{})
	if !ok {
		t.Fatal("content is not an array")
	}
	return content
}

// extractTextFromNode extracts the concatenated text from a TipTap node's
// inline content.
func extractTextFromNode(node map[string]interface{}) string {
	content, ok := node["content"].([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, c := range content {
		n := c.(map[string]interface{})
		if n["type"] == "text" {
			if text, ok := n["text"].(string); ok {
				sb.WriteString(text)
			}
		}
	}
	return sb.String()
}

// extractTextFromListItem extracts the text from a listItem node.
func extractTextFromListItem(item map[string]interface{}) string {
	content, ok := item["content"].([]interface{})
	if !ok {
		return ""
	}
	if len(content) == 0 {
		return ""
	}
	para := content[0].(map[string]interface{})
	return extractTextFromNode(para)
}
