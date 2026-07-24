package media

import (
	"path/filepath"
	"testing"
)

const svgContentType = "image/svg+xml"

func TestResolveMediaPath_RejectsTraversal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		bucket string
		path   string
	}{
		{"avatars", "../../../etc/passwd"},
		{"avatars", "nested/../../../../etc/passwd"},
		{"../..", "etc/passwd"},
		{"../avatars", "pic.png"},
	}

	for _, tc := range cases {
		if got, err := resolveMediaPath(tc.bucket, tc.path); err == nil {
			t.Errorf("resolveMediaPath(%q, %q) = %q, want error", tc.bucket, tc.path, got)
		}
	}
}

func TestResolveMediaPath_AllowsNormalPath(t *testing.T) {
	t.Parallel()

	got, err := resolveMediaPath("avatars", "2024/06/pic.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want, _ := filepath.Abs(filepath.Join(mediaRoot, "avatars", "2024", "06", "pic.png"))
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestIsActiveContentType(t *testing.T) {
	t.Parallel()

	for _, ct := range []string{svgContentType, "text/html; charset=utf-8", "TEXT/HTML"} {
		if !isActiveContentType(ct) {
			t.Errorf("expected %q to be treated as active content", ct)
		}
	}

	if isActiveContentType("image/png") {
		t.Error("image/png should not be treated as active content")
	}
}
