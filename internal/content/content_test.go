package content

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// mustUUID scans a UUID string into a pgtype.UUID, failing the test on error.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

// repeat builds a string by repeating s n times. Helper for word count tests.
func repeat(s string, n int) string {
	result := ""
	for range n {
		result += s
	}

	return result
}

// ---------------------------------------------------------------------------
// Slugify
// ---------------------------------------------------------------------------

func TestSlugify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"Special!@#Characters$%^", "special-characters"},
		{"UPPERCASE", "uppercase"},
		{"MixedCase123", "mixedcase123"},
		{"Trailing---Hyphens---", "trailing-hyphens"},
		{"---Leading Hyphens", "leading-hyphens"},
		{"", "untitled"},
		{"   ", "untitled"},
		{"Numbers 123 and Text", "numbers-123-and-text"},
		{"Nigerian-Job-Postings", "nigerian-job-postings"},
	}
	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ComputeReadTime
// ---------------------------------------------------------------------------

func TestComputeReadTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want int32
	}{
		{"empty", "", 0},
		{"under 200 words", "one two three four five", 1},
		{"exactly 200 words", repeat("word ", 200), 1},
		{"201 words", repeat("word ", 201), 2},
		{"400 words", repeat("word ", 400), 2},
		{"500 words", repeat("word ", 500), 3},
	}
	for _, tt := range tests {
		got := ComputeReadTime(tt.body)
		if got != tt.want {
			t.Errorf("ComputeReadTime(%s) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestContentStatusConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		StatusDraft:     "draft",
		StatusPublished: "published",
		StatusScheduled: "scheduled",
		StatusArchived:  "archived",
	}
	for got, want := range want {
		if got != want {
			t.Errorf("status constant mismatch: got %q want %q", got, want)
		}
	}
}

func TestContentTypeConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		TypeBlogPost:       "blog_post",
		TypeDiscussion:     "discussion",
		TypePage:           "page",
		TypeComparison:     "comparison",
		TypeStory:          "story",
		TypeAchievement:    "achievement",
		TypeServiceListing: "service_listing",
	}
	for got, want := range want {
		if got != want {
			t.Errorf("type constant mismatch: got %q want %q", got, want)
		}
	}
}

func TestVisibilityConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		VisibilityPublic:  "public",
		VisibilityMembers: "members",
		VisibilityPrivate: "private",
	}
	for got, want := range want {
		if got != want {
			t.Errorf("visibility constant mismatch: got %q want %q", got, want)
		}
	}
}

func TestPageSizeConstants(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// writeJSON / writeError
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, got["status"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteError(rr, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errMap, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %v", got)
	}

	if errMap["code"] != "invalid_body" {
		t.Errorf("error.code = %v, want invalid_body", errMap["code"])
	}

	if errMap["message"] != "Invalid JSON body" {
		t.Errorf("error.message = %v, want 'Invalid JSON body'", errMap["message"])
	}
}

// ---------------------------------------------------------------------------
// parsePageSize
// ---------------------------------------------------------------------------

func TestParsePageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int32
	}{
		{"", DefaultPageSize},
		{"0", DefaultPageSize},
		{"-1", DefaultPageSize},
		{"abc", DefaultPageSize},
		{"10", 10},
		{"50", 50},
		{"100", 100},
		{"101", MaxPageSize}, // clamped
		{"999", MaxPageSize}, // clamped
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/content?page_size="+tt.query, nil)

		got := parsePageSize(req)
		if got != tt.want {
			t.Errorf("parsePageSize(page_size=%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}
