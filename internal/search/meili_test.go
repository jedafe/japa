package search

import (
	"testing"
)

// TestRenderMeiliFilter verifies the FilterExpr AST translates to correct
// Meilisearch filter syntax. These cases mirror the exact filter shapes the
// handlers and adapters build — a regression here silently breaks tenant
// scoping, geo filtering, or doc-page audience filtering.
func TestRenderMeiliFilter(t *testing.T) {
	cases := []struct {
		name string
		in   FilterExpr
		want string
	}{
		{
			name: "nil filter",
			in:   nil,
			want: "",
		},
		{
			name: "tenant scoping (content adapter shape)",
			in:   
			want: ``,
		},
		{
			name: "space-scoped content search",
			in: andAll(
				
				Eq{Field: "space_id", Value: "s1"},
			),
			want: `(space_id = "s1")`,
		},
		{
			name: "geo boost (ADR-064)",
			in: andAll(
				
				orAny(
					IsEmpty{Field: "relevant_countries"},
					Eq{Field: "relevant_countries", Value: "NG"},
				),
			),
			want: `((relevant_countries IS EMPTY) OR (relevant_countries = "NG"))`,
		},
		{
			name: "posts extra filter via buildPostFilter",
			in: andAll(
				
				buildPostFilter("post", "s1", "visa"),
			),
			want: `((content_type = "post") AND (space_id = "s1") AND (tag_slugs = "visa"))`,
		},
		{
			name: "doc pages tenant+status+audience",
			in: andAll(
				
				Eq{Field: "review_status", Value: "published"},
				In{Field: "audience", Values: []string{"member", "public"}},
			),
			want: `(review_status = "published") AND (audience IN ["member", "public"])`,
		},
		{
			name: "empty In renders empty",
			in:   In{Field: "audience", Values: nil},
			want: "",
		},
		{
			name: "quote escaping",
			in:   Eq{Field: "title", Value: `say "hi"`},
			want: `title = "say \"hi\""`,
		},
		{
			name: "backslash escaping",
			in:   Eq{Field: "slug", Value: `a\b`},
			want: `slug = "a\\b"`,
		},
		{
			name: "andAll skips nil children",
			in:   andAll(Eq{Field: "a", Value: "1"}, nil, Eq{Field: "b", Value: "2"}),
			want: `(a = "1") AND (b = "2")`,
		},
		{
			name: "andAll collapses to nil when empty",
			in:   ,
			want: "",
		},
		{
			name: "single child unwraps",
			in:   andAll(Eq{Field: "a", Value: "1"}),
			want: `a = "1"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderMeiliFilter(tc.in)
			if got != tc.want {
				t.Errorf("renderMeiliFilter() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHitDecodeInto verifies the engine-neutral Hit decodes into typed
// structs the same way downstream adapters rely on.
func TestHitDecodeInto(t *testing.T) {
	hit := Hit{
		"id":       []byte(`"abc-123"`),
		"title":    []byte(`"Hello"`),
		"audience": []byte(`"member"`),
	}

	var out docPageHit
	if err := hit.DecodeInto(&out); err != nil {
		t.Fatalf("DecodeInto: %v", err)
	}

	if out.ID != "abc-123" || out.Title != "Hello" || out.Audience != "member" {
		t.Errorf("unexpected decode result: %+v", out)
	}
}
