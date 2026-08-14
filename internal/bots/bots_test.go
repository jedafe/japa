package bots

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Type validation tests
// ---------------------------------------------------------------------------

func TestIsBotType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"search_engine", true},
		{"ai_llm", true},
		{"social", true},
		{"monitoring", true},
		{"scraper", true},
		{"unknown", true},
		{"invalid", false},
		{"", false},
		{"SEARCH_ENGINE", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			if got := IsBotType(tt.input); got != tt.want {
				t.Errorf("IsBotType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"allow", true},
		{"block", true},
		{"limit", true},
		{"deny", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			if got := IsPolicy(tt.input); got != tt.want {
				t.Errorf("IsPolicy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAllBotTypes(t *testing.T) {
	t.Parallel()

	types := AllBotTypes()
	if len(types) != 6 {
		t.Errorf("expected 6 bot types, got %d", len(types))
	}

	seen := make(map[BotType]bool)
	for _, bt := range types {
		if seen[bt] {
			t.Errorf("duplicate bot type: %s", bt)
		}

		seen[bt] = true
	}
}

func TestAllPolicies(t *testing.T) {
	t.Parallel()

	policies := AllPolicies()
	if len(policies) != 3 {
		t.Errorf("expected 3 policies, got %d", len(policies))
	}
}

// ---------------------------------------------------------------------------
// Handler helper tests
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %s", body["status"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var body map[string]map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if body["error"]["code"] != "invalid_input" {
		t.Errorf("expected error code invalid_input, got %s", body["error"]["code"])
	}

	if body["error"]["message"] != "Bad request" {
		t.Errorf("expected error message Bad request, got %s", body["error"]["message"])
	}
}

func TestParsePageSize(t *testing.T) {
	t.Parallel()
	// Default page size
	r := httptest.NewRequest(http.MethodGet, "/bots", nil)
	if ps := parsePageSize(r); ps != DefaultPageSize {
		t.Errorf("expected default page size %d, got %d", DefaultPageSize, ps)
	}

	// Custom page size
	r2 := httptest.NewRequest(http.MethodGet, "/bots?page_size=50", nil)
	if ps := parsePageSize(r2); ps != 50 {
		t.Errorf("expected page size 50, got %d", ps)
	}

	// Page size exceeding max
	r3 := httptest.NewRequest(http.MethodGet, "/bots?page_size=200", nil)
	if ps := parsePageSize(r3); ps != MaxPageSize {
		t.Errorf("expected max page size %d, got %d", MaxPageSize, ps)
	}

	// Invalid page size
	r4 := httptest.NewRequest(http.MethodGet, "/bots?page_size=abc", nil)
	if ps := parsePageSize(r4); ps != DefaultPageSize {
		t.Errorf("expected default page size for invalid input, got %d", ps)
	}
}

func TestParseOffset(t *testing.T) {
	t.Parallel()
	// Default offset
	r := httptest.NewRequest(http.MethodGet, "/bots", nil)
	if o := parseOffset(r); o != 0 {
		t.Errorf("expected default offset 0, got %d", o)
	}

	// Custom offset
	r2 := httptest.NewRequest(http.MethodGet, "/bots?page_offset=10", nil)
	if o := parseOffset(r2); o != 10 {
		t.Errorf("expected offset 10, got %d", o)
	}

	// Invalid offset
	r3 := httptest.NewRequest(http.MethodGet, "/bots?page_offset=abc", nil)
	if o := parseOffset(r3); o != 0 {
		t.Errorf("expected default offset for invalid input, got %d", o)
	}
}
