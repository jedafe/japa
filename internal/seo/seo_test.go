package seo

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestSitemapXML(t *testing.T) {
	t.Parallel()
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

// --- Structured Data (JSON-LD) tests ---

func TestGenerateArticleJSONLD(t *testing.T) {
	t.Parallel()

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

	if ld["headline"] != "Moving to Canada" {
		t.Errorf("expected headline 'Moving to Canada', got %v", ld["headline"])
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
	t.Parallel()

	ld := GenerateArticleJSONLD("T", "D", "A", "", "", "")

	if _, ok := ld["image"]; ok {
		t.Error("empty imageURL should not appear in output")
	}

	if _, ok := ld["dateModified"]; ok {
		t.Error("empty updatedAt should not appear in output")
	}
}

func TestGenerateQAPageJSONLD_Solved(t *testing.T) {
	t.Parallel()

	ld := GenerateQAPageJSONLD(
		"How to apply for visa?",
		"I need help with my application.",
		"Here is the step-by-step process...",
		"Ada N.",
		"2025-01-15T10:00:00Z",
		true,
		"Chidi O.",
		1,
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
	t.Parallel()

	ld := GenerateQAPageJSONLD("Q", "B", "", "Author", "2025-01-15T10:00:00Z", false, "", 0)

	mainEntity := ld["mainEntity"].(map[string]interface{})
	if mainEntity["answerCount"] != 0 {
		t.Errorf("expected answerCount 0 for unsolved, got %v", mainEntity["answerCount"])
	}

	if _, ok := mainEntity["acceptedAnswer"]; ok {
		t.Error("acceptedAnswer should not be present for unsolved questions")
	}
}

func TestGenerateReviewJSONLD(t *testing.T) {
	t.Parallel()

	ld := GenerateReviewJSONLD("Immigration Consultancy", 4.5, 120, "Service")

	if ld["@type"] != "Service" {
		t.Errorf("expected @type Service, got %v", ld["@type"])
	}

	rating := ld["aggregateRating"].(map[string]interface{})
	if rating["ratingValue"] != 4.5 {
		t.Errorf("expected ratingValue 4.5, got %v", rating["ratingValue"])
	}

	if rating["reviewCount"] != 120 {
		t.Errorf("expected reviewCount 120, got %v", rating["reviewCount"])
	}
}

func TestGenerateOrganizationJSONLD(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	ld := GenerateOrganizationJSONLD("JapaConnect", "", "")

	if _, ok := ld["logo"]; ok {
		t.Error("empty logo should not appear")
	}

	if _, ok := ld["url"]; ok {
		t.Error("empty url should not appear")
	}
}

func TestGenerateCourseJSONLD(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	ld := GenerateCourseJSONLD("C", "D", "P", "")

	if _, ok := ld["url"]; ok {
		t.Error("empty url should not appear")
	}
}

func TestGenerateBreadcrumbListJSONLD(t *testing.T) {
	t.Parallel()

	ld := GenerateBreadcrumbListJSONLD([]BreadcrumbItem{
		{Name: "Home", URL: "https://example.com"},
		{Name: "Blog", URL: "https://example.com/blog"},
		{Name: "My Post", URL: "https://example.com/blog/my-post"},
	})

	if ld["@type"] != "BreadcrumbList" {
		t.Errorf("expected @type BreadcrumbList, got %v", ld["@type"])
	}

	elements, ok := ld["itemListElement"].([]map[string]interface{})
	if !ok {
		t.Fatal("itemListElement should be a slice of maps")
	}

	if len(elements) != 3 {
		t.Fatalf("expected 3 items, got %d", len(elements))
	}

	if elements[0]["position"] != 1 {
		t.Errorf("expected position 1, got %v", elements[0]["position"])
	}

	if elements[2]["name"] != "My Post" {
		t.Errorf("expected last item name 'My Post', got %v", elements[2]["name"])
	}

	if elements[1]["item"] != "https://example.com/blog" {
		t.Errorf("expected item URL, got %v", elements[1]["item"])
	}
}

// --- Hreflang tests ---

func TestHreflangLink(t *testing.T) {
	t.Parallel()

	got := HreflangLink("fr", "https://example.com/fr/guide")
	want := `<link rel="alternate" hreflang="fr" href="https://example.com/fr/guide" />`

	if got != want {
		t.Errorf("expected:\n  %s\ngot:\n  %s", want, got)
	}
}

func TestHreflangAlternate_DefaultLanguages(t *testing.T) {
	t.Parallel()

	got := HreflangAlternate("https://japa.to", "/guide", nil)

	if !strings.Contains(got, `hreflang="en"`) {
		t.Error("missing English hreflang")
	}

	if !strings.Contains(got, `hreflang="fr"`) {
		t.Error("missing French hreflang")
	}

	if !strings.Contains(got, `hreflang="x-default"`) {
		t.Error("missing x-default hreflang")
	}

	if !strings.Contains(got, `https://japa.to/en/guide`) {
		t.Error("missing English URL")
	}

	if !strings.Contains(got, `https://japa.to/fr/guide`) {
		t.Error("missing French URL")
	}
}

func TestHreflangAlternate_CustomLanguages(t *testing.T) {
	t.Parallel()

	got := HreflangAlternate("https://example.com", "/post", []string{"en", "de", "es"})

	langs := []string{"en", "de", "es", "x-default"}
	for _, l := range langs {
		if !strings.Contains(got, `hreflang="`+l+`"`) {
			t.Errorf("missing hreflang for %s", l)
		}
	}
}

// stubHreflangSettings is a minimal hreflangSettingsReader for testing
// ResolveLanguages without a real config.Settings.
type stubHreflangSettings struct {
	langs string
}

func (s stubHreflangSettings) GetStringOr(_ context.Context, _, _ string) string {
	return s.langs
}

func TestResolveLanguages(t *testing.T) {
	t.Parallel()

	// Reader returns a multi-language CSV.
	got := ResolveLanguages(t.Context(), stubHreflangSettings{langs: "en, fr , es"})
	if len(got) != 3 || got[0] != "en" || got[1] != "fr" || got[2] != "es" {
		t.Errorf("expected [en fr es], got %v", got)
	}

	// Empty setting falls back to FallbackLanguages.
	got = ResolveLanguages(t.Context(), stubHreflangSettings{langs: ""})
	if len(got) != 2 || got[0] != "en" || got[1] != "fr" {
		t.Errorf("expected fallback [en fr], got %v", got)
	}

	// Nil reader falls back to FallbackLanguages.
	got = ResolveLanguages(t.Context(), nil)
	if len(got) != 2 || got[0] != "en" || got[1] != "fr" {
		t.Errorf("expected fallback [en fr] for nil reader, got %v", got)
	}
}

// --- OG Image Handler tests ---

func TestServeOGImage(t *testing.T) {
	t.Parallel()

	h := NewOGImageHandler(nil, nil, nil, "Test Site")
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

func TestServeOGImageNamespacedKey(t *testing.T) {
	t.Parallel()

	h := NewOGImageHandler(nil, nil, nil, "Test Site")
	r := chi.NewRouter()
	r.Get("/og/{id}.png", h.ServeOGImage)

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// The colon namespaced key must survive {id} param capture and resolve
	// the registry title — a resolved card differs byte-wise from the
	// brand fallback served for unresolvable keys.
	tool := get("/og/tool:visa-routes.png")
	brand := get("/og/definitely-not-a-thing.png")
	if tool.Code != http.StatusOK || tool.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("tool key: got %d %q", tool.Code, tool.Header().Get("Content-Type"))
	}
	if bytes.Equal(tool.Body.Bytes(), brand.Body.Bytes()) {
		t.Error("tool:visa-routes rendered the brand fallback — namespaced key did not resolve")
	}
}

func TestNewOGImageHandler(t *testing.T) {
	t.Parallel()

	h := NewOGImageHandler(nil, nil, nil, "Test Site")
	if h == nil {
		t.Fatal("NewOGImageHandler should return non-nil")
	}
}

func TestLookupToolKey(t *testing.T) {
	t.Parallel()

	h := NewOGImageHandler(nil, nil, nil, "Test Site")

	// Registered tool slug resolves to its registry title card.
	meta := h.lookup(t.Context(), "tool:visa-routes")
	if meta == nil {
		t.Fatal("expected contentMeta for tool:visa-routes, got nil")
	}
	if meta.Title != "Visa Routes" {
		t.Errorf("expected title 'Visa Routes', got %q", meta.Title)
	}
	if meta.ContentType != "tool" {
		t.Errorf("expected contentType 'tool', got %q", meta.ContentType)
	}

	// Unknown tool slugs and namespaces resolve to nil → brand fallback.
	for _, key := range []string{"tool:not-a-tool", "bogus:thing", "not-a-uuid"} {
		if m := h.lookup(t.Context(), key); m != nil {
			t.Errorf("expected nil meta for %q, got %+v", key, m)
		}
	}
}

// TestLookupEntityNamespaces proves every namespaced entity key routes to
// its resolver — with a nil reader the DB-backed lookups return nil → brand
// fallback rather than panicking, and none fall through to the UUID path.
func TestLookupEntityNamespaces(t *testing.T) {
	t.Parallel()

	h := NewOGImageHandler(nil, nil, nil, "Test Site")

	for _, key := range []string{
		"user:someone", "product:widget", "tag:visa", "source:gov-ng",
		"org:acme", "scholarship:chevening", "ama:8f1c2a3b-0000-4000-8000-000000000001",
		"event:8f1c2a3b-0000-4000-8000-000000000000",
		"scam-alert:8f1c2a3b-0000-4000-8000-000000000000",
	} {
		if m := h.lookup(t.Context(), key); m != nil {
			t.Errorf("expected nil meta for %q with nil reader, got %+v", key, m)
		}
	}
}

func TestHumanizeLabel(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"fake_landlord":      "Fake Landlord",
		"romance_scam":       "Romance Scam",
		"single":             "Single",
		"":                   "",
		"multi__underscores": "Multi Underscores",
	}
	for in, want := range cases {
		if got := humanizeLabel(in); got != want {
			t.Errorf("humanizeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanizeContentType(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"blog_post":        "Blog Post",
		"tool":             "Tool",
		"wiki_page":        "Wiki Page",
		"destination_wiki": "Destination Wiki",
		"profile":          "Member Profile",
		"product":          "Product",
		"tag":              "Tag",
		"source":           "Source",
		"organization":     "Organization",
		"scholarship":      "Scholarship",
		"event":            "Event",
		"scam_alert":       "Scam Alert",
		"ama":              "AMA Session",
		"":                 "Content",
		"custom_thing":     "Custom_thing",
	}
	for ct, want := range cases {
		if got := humanizeContentType(ct); got != want {
			t.Errorf("humanizeContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}

// TestSanitizeForFace proves unrenderable runes (emoji, Arabic, CJK — none
// of which Go Regular covers) are stripped rather than painted as tofu,
// and that fully-unrenderable titles collapse to "" for brand fallback.
func TestSanitizeForFace(t *testing.T) {
	t.Parallel()

	face := loadFontFace(48)

	cases := map[string]string{
		"Ahmed Al-Farsi":   "Ahmed Al-Farsi",
		"Ahmed 🎓 MBA":      "Ahmed MBA",
		"أحمد":             "",
		"プログラム":            "",
		"Visa Guide ":      "Visa Guide",
		"  spaced   out  ": "spaced out",
	}
	for in, want := range cases {
		if got := sanitizeForFace(face, in); got != want {
			t.Errorf("sanitizeForFace(%q) = %q, want %q", in, got, want)
		}
	}

	// nil face → pass-through (renderer degrades to the bitmap fallback).
	if got := sanitizeForFace(nil, "raw"); got != "raw" {
		t.Errorf("sanitizeForFace(nil, raw) = %q, want pass-through", got)
	}
}
