package bots

import (
	"testing"
)

func TestIsSpaceIndexable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		members    int
		posts      int
		minMembers int
		minPosts   int
		want       bool
	}{
		{"meets defaults", 10, 5, 0, 0, true},
		{"below min members", 2, 5, 0, 0, false},
		{"below min posts", 10, 1, 0, 0, false},
		{"both below", 1, 1, 0, 0, false},
		{"custom thresholds meet", 20, 10, 15, 8, true},
		{"custom thresholds fail members", 10, 10, 15, 8, false},
		{"custom thresholds fail posts", 20, 5, 15, 8, false},
		{"zero thresholds use defaults", 10, 5, 0, 0, true},
		{"exact default threshold members", 5, 3, 0, 0, true},
		{"just below default threshold members", 4, 3, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsSpaceIndexable(tt.members, tt.posts, tt.minMembers, tt.minPosts)
			if got != tt.want {
				t.Errorf("IsSpaceIndexable(%d, %d, %d, %d) = %v, want %v",
					tt.members, tt.posts, tt.minMembers, tt.minPosts, got, tt.want)
			}
		})
	}
}

func TestIsContentIndexable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		status              string
		visibility          string
		wordCount           int
		minWords            int
		allowSearchIndexing bool
		want                bool
	}{
		{"published public with indexing", "published", "public", 200, 0, true, true},
		{"published private", "published", "private", 200, 0, true, false},
		{"draft published", "draft", "public", 200, 0, true, false},
		{"no indexing allowed", "published", "public", 200, 0, false, false},
		{"below min words", "published", "public", 50, 100, true, false},
		{"exact min words", "published", "public", 100, 100, true, true},
		{"above min words", "published", "public", 150, 100, true, true},
		{"empty visibility treated as public", "published", "", 200, 0, true, true},
		{"custom min words", "published", "public", 300, 250, true, true},
		{"default min words threshold", "published", "public", 100, 0, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsContentIndexable(tt.status, tt.visibility, tt.wordCount, tt.minWords, tt.allowSearchIndexing)
			if got != tt.want {
				t.Errorf("IsContentIndexable(%q, %q, %d, %d, %v) = %v, want %v",
					tt.status, tt.visibility, tt.wordCount, tt.minWords, tt.allowSearchIndexing, got, tt.want)
			}
		})
	}
}

func TestDefaultSitemapQualityConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultSitemapQualityConfig()
	if cfg.MinMembers != 5 {
		t.Errorf("expected MinMembers 5, got %d", cfg.MinMembers)
	}

	if cfg.MinPosts != 3 {
		t.Errorf("expected MinPosts 3, got %d", cfg.MinPosts)
	}

	if cfg.MinWords != 100 {
		t.Errorf("expected MinWords 100, got %d", cfg.MinWords)
	}
}
