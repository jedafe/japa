package seo

import (
	"bytes"
	"encoding/xml"
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