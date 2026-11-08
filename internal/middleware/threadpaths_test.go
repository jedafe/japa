package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLegacyForumPathRewrite(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"exact legacy path", "/api/v1/forum", "/api/v1/thread"},
		{"legacy subpath", "/api/v1/forum/my-thread", "/api/v1/thread/my-thread"},
		{"legacy nested", "/api/v1/forum/abc/replies", "/api/v1/thread/abc/replies"},
		{"canonical untouched", "/api/v1/thread/my-thread", "/api/v1/thread/my-thread"},
		{"threads anchor untouched", "/api/v1/threads/anchor/tool/travel", "/api/v1/threads/anchor/tool/travel"},
		{"unrelated path", "/api/v1/content/abc", "/api/v1/content/abc"},
		{"prefix lookalike", "/api/v1/forums/x", "/api/v1/forums/x"},
		{"top-level forum feed untouched", "/forum/feed.xml", "/forum/feed.xml"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Path
			})

			req := httptest.NewRequest(http.MethodGet, tc.in+"?sort=latest", nil)
			LegacyForumPathRewrite(next).ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.want {
				t.Fatalf("path %q rewritten to %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
