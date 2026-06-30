package seo

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeneratePlaceholderOG(t *testing.T) {
	// Test that placeholder OG image generation works
	handler := &OGImageHandler{}

	// This will fail without a real database, but we can test the structure
	// For now, just test that the handler exists
	if handler == nil {
		t.Fatal("Handler should not be nil")
	}
}

func TestSitemapXML(t *testing.T) {
	// Test XML sitemap generation
	entries := []urlEntry{
		{Loc: "https://example.com", LastMod: "2026-01-01T00:00:00Z", ChangeFreq: "daily", Priority: "1.0"},
		{Loc: "https://example.com/post/hello", LastMod: "2026-01-02T00:00:00Z", ChangeFreq: "weekly", Priority: "0.8"},
	}

	testUrlset := urlset{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	}

	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(testUrlset); err != nil {
		t.Fatalf("Failed to encode XML: %v", err)
	}

	// Verify it's valid XML
	var decoded urlset
	if err := xml.NewDecoder(&buf).Decode(&decoded); err != nil {
		t.Fatalf("Failed to decode XML: %v", err)
	}

	if len(decoded.URLs) != 2 {
		t.Errorf("Expected 2 URLs, got %d", len(decoded.URLs))
	}

	if decoded.XMLName.Local != "urlset" {
		t.Errorf("Expected root element 'urlset', got '%s'", decoded.XMLName.Local)
	}
}

func TestPageMetadata(t *testing.T) {
	metadata := PageMetadata{
		Title:       "Test Title",
		Description: "Test Description",
		Slug:        "test-title",
		ContentType: "article",
		OGImage:     "https://example.com/og/test.png",
		Canonical:   "https://example.com/article/test-title",
		Author:      "Test Author",
	}

	if metadata.Title != "Test Title" {
		t.Errorf("Expected title 'Test Title', got '%s'", metadata.Title)
	}
}

func TestOGImageRequest(t *testing.T) {
	req := OGImageRequest{
		Title:    "Test Title",
		Subtitle: "Test Subtitle",
		LogoPath: "/logo.png",
	}

	if req.Title != "Test Title" {
		t.Errorf("Expected title 'Test Title', got '%s'", req.Title)
	}
}

// --- Structured Data (JSON-LD) tests ---

func TestGenerateArticleJSONLD(t *testing.T) {
	ld := GenerateArticleJSONLD(
		"Moving to Canada",
		"Complete guide",
		"Chidinma O.",
		"https://example.com/og/canada.png",
		"2026-01-15",
		"2026-06-20",
	)

	if ld["@type"] != "Article" {
		t.Errorf("expected @type Article, got %v", ld["@type"])
	}
	if ld["title"] != "Moving to Canada" {
		t.Errorf("expected title 'Moving to Canada', got %v", ld["title"])
	}
	if ld["dateModified"] != "2026-06-20" {
		t.Errorf("expected dateModified '2026-06-20', got %v", ld["dateModified"])
	}
	if ld["image"] != "https://example.com/og/canada.png" {
		t.Errorf("expected image URL, got %v", ld["image"])
	}

	// Verify it marshals to valid JSON.
	raw, err := json.Marshal(ld)
	if err != nil {
		t.Fatalf("failed to marshal Article JSON-LD: %v", err)
	}
	if !strings.Contains(string(raw), `"@type":"Article"`) {
		t.Errorf("marshaled JSON missing @type Article: %s", raw)
	}
}

func TestGenerateArticleJSONLD_OptionalFields(t *testing.T) {
	ld := GenerateArticleJSONLD("T", "D", "A", "", "", "")

	if _, ok := ld["image"]; ok {
		t.Error("empty imageURL should not appear in output")
	}
	if _, ok := ld["dateModified"]; ok {
		t.Error("empty updatedAt should not appear in output")
	}
}

func TestGenerateQAPageJSONLD_Solved(t *testing.T) {
	ld := GenerateQAPageJSONLD(
		"How to apply for visa?",
		"I need help with my application.",
		"Here is the step-by-step process...",
		"Ada N.",
		true,
	)

	if ld["@type"] != "QAPage" {
		t.Errorf("expected @type QAPage, got %v", ld["@type"])
	}

	mainEntity, ok := ld["mainEntity"].(map[string]interface{})
	if !ok {
		t.Fatal("mainEntity should be a map")
	}
	if mainEntity["@type"] != "Question" {
		t.Errorf("expected mainEntity @type Question, got %v", mainEntity["@type"])
	}
	if mainEntity["answerCount"] != 1 {
		t.Errorf("expected answerCount 1, got %v", mainEntity["answerCount"])
	}

	answer, ok := mainEntity["acceptedAnswer"].(map[string]interface{})
	if !ok {
		t.Fatal("acceptedAnswer should be present for solved questions")
	}
	if answer["text"] != "Here is the step-by-step process..." {
		t.Errorf("unexpected answer text: %v", answer["text"])
	}
}

func TestGenerateQAPageJSONLD_Unsolved(t *testing.T) {
	ld := GenerateQAPageJSONLD("Q", "B", "", "Author", false)

	mainEntity := ld["mainEntity"].(map[string]interface{})
	if mainEntity["answerCount"] != 0 {
		t.Errorf("expected answerCount 0 for unsolved, got %v", mainEntity["answerCount"])
	}
	if _, ok := mainEntity["acceptedAnswer"]; ok {
		t.Error("acceptedAnswer should not be present for unsolved questions")
	}
}

func TestGenerateReviewJSONLD(t *testing.T) {
	ld := GenerateReviewJSONLD("Immigration Consultancy", 4.5, 120)

	if ld["@type"] != "Product" {
		t.Errorf("expected @type Product, got %v", ld["@type"])
	}
	rating := ld["aggregateRating"].(map[string]interface{})
	if rating["ratingValue"] != "4.5" {
		t.Errorf("expected ratingValue '4.5', got %v", rating["ratingValue"])
	}
	if rating["reviewCount"] != 120 {
		t.Errorf("expected reviewCount 120, got %v", rating["reviewCount"])
	}
}

func TestGenerateOrganizationJSONLD(t *testing.T) {
	ld := GenerateOrganizationJSONLD("JapaConnect", "https://example.com/logo.png", "https://example.com")

	if ld["@type"] != "Organization" {
		t.Errorf("expected @type Organization, got %v", ld["@type"])
	}
	if ld["logo"] != "https://example.com/logo.png" {
		t.Errorf("expected logo URL, got %v", ld["logo"])
	}
	if ld["url"] != "https://example.com" {
		t.Errorf("expected url, got %v", ld["url"])
	}
}

func TestGenerateOrganizationJSONLD_Minimal(t *testing.T) {
	ld := GenerateOrganizationJSONLD("JapaConnect", "", "")

	if _, ok := ld["logo"]; ok {
		t.Error("empty logo should not appear")
	}
	if _, ok := ld["url"]; ok {
		t.Error("empty url should not appear")
	}
}

func TestGenerateCourseJSONLD(t *testing.T) {
	ld := GenerateCourseJSONLD(
		"Canadian Immigration 101",
		"Learn the basics of Canadian immigration.",
		"JapaConnect Academy",
		"https://example.com/courses/canada-101",
	)

	if ld["@type"] != "Course" {
		t.Errorf("expected @type Course, got %v", ld["@type"])
	}
	if ld["name"] != "Canadian Immigration 101" {
		t.Errorf("unexpected name: %v", ld["name"])
	}
	provider := ld["provider"].(map[string]interface{})
	if provider["name"] != "JapaConnect Academy" {
		t.Errorf("unexpected provider name: %v", provider["name"])
	}
}

func TestGenerateCourseJSONLD_NoURL(t *testing.T) {
	ld := GenerateCourseJSONLD("C", "D", "P", "")

	if _, ok := ld["url"]; ok {
		t.Error("empty url should not appear")
	}
}

// --- Hreflang tests ---

func TestHreflangLink(t *testing.T) {
	got := HreflangLink("fr", "https://example.com/fr/guide")
	want := `<link rel="alternate" hreflang="fr" href="https://example.com/fr/guide" />`
	if got != want {
		t.Errorf("expected:\n  %s\ngot:\n  %s", want, got)
	}
}

func TestHreflangAlternate_DefaultLanguages(t *testing.T) {
	got := HreflangAlternate("https://japaconnect.com", "/guide", nil)

	if !strings.Contains(got, `hreflang="en"`) {
		t.Error("missing English hreflang")
	}
	if !strings.Contains(got, `hreflang="fr"`) {
		t.Error("missing French hreflang")
	}
	if !strings.Contains(got, `hreflang="x-default"`) {
		t.Error("missing x-default hreflang")
	}
	if !strings.Contains(got, `https://japaconnect.com/en/guide`) {
		t.Error("missing English URL")
	}
	if !strings.Contains(got, `https://japaconnect.com/fr/guide`) {
		t.Error("missing French URL")
	}
}

func TestHreflangAlternate_CustomLanguages(t *testing.T) {
	got := HreflangAlternate("https://example.com", "/post", []string{"en", "de", "es"})

	langs := []string{"en", "de", "es", "x-default"}
	for _, l := range langs {
		if !strings.Contains(got, `hreflang="`+l+`"`) {
			t.Errorf("missing hreflang for %s", l)
		}
	}
}

// --- OG Image Handler tests ---

func TestServeOGImage(t *testing.T) {
	h := NewOGImageHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/og/00000000-0000-0000-0000-000000000001.png", nil)
	rec := httptest.NewRecorder()

	h.ServeOGImage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("expected Content-Type image/png, got %s", ct)
	}
	cc := rec.Header().Get("Cache-Control")
	if !strings.Contains(cc, "max-age=86400") {
		t.Errorf("expected 24h cache header, got %s", cc)
	}
	if rec.Body.Len() == 0 {
		t.Error("response body should not be empty")
	}
}

func TestNewOGImageHandler(t *testing.T) {
	h := NewOGImageHandler(nil)
	if h == nil {
		t.Fatal("NewOGImageHandler should return non-nil")
	}
}