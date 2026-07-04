package linkpreview

import (
	"strings"
	"testing"
)

func TestParseOGTags(t *testing.T) {
	t.Parallel()

	htmlContent := `
		<html>
		<head>
			<meta property="og:title" content="Test Title">
			<meta property="og:description" content="Test Description">
			<meta property="og:image" content="https://example.com/image.jpg">
			<meta property="og:site_name" content="Example Site">
			<link rel="icon" href="/favicon.ico">
		</head>
		<body></body>
		</html>
	`

	preview, err := parseOGTags(strings.NewReader(htmlContent), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	if preview.Title != "Test Title" {
		t.Errorf("Title = %q, want 'Test Title'", preview.Title)
	}

	if preview.Description != "Test Description" {
		t.Errorf("Description = %q, want 'Test Description'", preview.Description)
	}

	if preview.Image != "https://example.com/image.jpg" {
		t.Errorf("Image = %q, want 'https://example.com/image.jpg'", preview.Image)
	}

	if preview.SiteName != "Example Site" {
		t.Errorf("SiteName = %q, want 'Example Site'", preview.SiteName)
	}

	if preview.Favicon != "/favicon.ico" {
		t.Errorf("Favicon = %q, want '/favicon.ico'", preview.Favicon)
	}
}

func TestCacheKey(t *testing.T) {
	t.Parallel()

	f := NewFetcher(nil, nil)

	key := f.cacheKey("https://example.com")
	if !strings.HasPrefix(key, "jc:link_preview:") {
		t.Errorf("cache key prefix wrong: %s", key)
	}
}

func TestGetHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/path", "example.com"},
		{"http://sub.example.com:8080/path", "sub.example.com"},
		{"example.com", "example.com"},
	}
	for _, tt := range tests {
		got := getHost(tt.url)
		if got != tt.want {
			t.Errorf("getHost(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
