package search

import "testing"

// Regression: doc_pages.content is jsonb and can hold a quoted JSON string
// (e.g. `""` or a markdown blob). Before the '"'-case fix, tiptapPlainText
// returned the literal quotes, so search excerpts rendered as `""`.
func TestTiptapPlainTextJSONStringBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty quoted string", `""`, ""},
		{"quoted markdown", `"# Hello World"`, "# Hello World"},
		{"quoted plain text", `"plain body text"`, "plain body text"},
		{"unquoted plain text", `plain body text`, "plain body text"},
		{"tiptap doc", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]}`, "Hello"},
		{"html body", `<p>Hello <b>world</b></p>`, "Hello world"},
		{"empty body", ``, ""},
		{"bare quotes no close", `"unterminated`, `"unterminated`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tiptapPlainText(tc.body); got != tc.want {
				t.Fatalf("tiptapPlainText(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}
