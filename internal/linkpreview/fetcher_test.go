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

	if preview.Favicon != "https://example.com/favicon.ico" {
		t.Errorf("Favicon = %q, want 'https://example.com/favicon.ico' (relative URL resolved against base)", preview.Favicon)
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

func TestParseOGTagsRelativeURLResolution(t *testing.T) {
	t.Parallel()

	// BUG-001: relative URLs for og:image and favicon must be resolved
	// against the page base URL.
	htmlContent := `
		<html>
		<head>
			<meta property="og:image" content="/images/og.jpg">
			<link rel="icon" href="/favicon.ico">
		</head>
		<body></body>
		</html>
	`
	preview, err := parseOGTags(strings.NewReader(htmlContent), "https://example.com/page")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Image != "https://example.com/images/og.jpg" {
		t.Errorf("Image = %q, want 'https://example.com/images/og.jpg'", preview.Image)
	}
	if preview.Favicon != "https://example.com/favicon.ico" {
		t.Errorf("Favicon = %q, want 'https://example.com/favicon.ico'", preview.Favicon)
	}
}

func TestParseOGTagsTitleFallback(t *testing.T) {
	t.Parallel()

	// BUG-006: fall back to <title> when og:title is missing.
	htmlContent := `
		<html>
		<head>
			<title>Page Title From Title Tag</title>
			<meta name="description" content="A description">
		</head>
		<body></body>
		</html>
	`
	preview, err := parseOGTags(strings.NewReader(htmlContent), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Title != "Page Title From Title Tag" {
		t.Errorf("Title = %q, want 'Page Title From Title Tag'", preview.Title)
	}
	if preview.Description != "A description" {
		t.Errorf("Description = %q, want 'A description'", preview.Description)
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
