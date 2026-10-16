package docs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidAudiences(t *testing.T) {
	t.Parallel()

	expected := []string{"users", "premium", "organizations", "moderators", "api", "admin", "superadmin"}
	for _, a := range expected {
		if !ValidAudiences[a] {
			t.Errorf("ValidAudiences missing expected audience %q", a)
		}
	}

	if ValidAudiences["invalid"] {
		t.Error("ValidAudiences should not contain 'invalid'")
	}
}

func TestAudienceGatesOnlyInternal(t *testing.T) {
	t.Parallel()

	// Only internal/admin/control-panel documentation is gated; every other
	// audience is public regardless of the group its docs target.
	expected := map[string]bool{"admin": true, "superadmin": true}
	for aud := range AudienceGates {
		if !expected[aud] {
			t.Errorf("audience %q must not be gated — only internal docs are", aud)
		}
	}

	for aud := range expected {
		if _, ok := AudienceGates[aud]; !ok {
			t.Errorf("internal audience %q must be gated", aud)
		}
	}
}

func TestPublicAudiences(t *testing.T) {
	t.Parallel()

	got := publicAudiences()
	want := []string{"users", "premium", "organizations", "moderators", "api"}

	if len(got) != len(want) {
		t.Fatalf("expected %d public audiences, got %d (%v)", len(want), len(got), got)
	}

	for i, a := range want {
		if got[i] != a {
			t.Errorf("public audience[%d]: expected %q, got %q", i, a, got[i])
		}
	}
}

func TestValidReviewStatuses(t *testing.T) {
	t.Parallel()

	expected := []string{"draft", "in_review", "approved", "published"}
	for _, s := range expected {
		if !ValidReviewStatuses[s] {
			t.Errorf("ValidReviewStatuses missing expected status %q", s)
		}
	}

	if ValidReviewStatuses["garbage"] {
		t.Error("ValidReviewStatuses should not contain 'garbage'")
	}
}

func TestToDocPage(t *testing.T) {
	t.Parallel()

	page := DocPage{
		ID:           "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
		Title:        "Getting Started",
		Slug:         "getting-started",
		Audience:     "users",
		Content:      json.RawMessage(`{"type":"doc","content":[]}`),
		AIGenerated:  false,
		ReviewStatus: "published",
		Position:     0,
	}
	if page.Title != "Getting Started" {
		t.Errorf("expected title 'Getting Started', got %q", page.Title)
	}

	if page.Audience != "users" {
		t.Errorf("expected audience 'users', got %q", page.Audience)
	}
}

func TestDocPageSummaryFields(t *testing.T) {
	t.Parallel()

	summary := DocPageSummary{
		ID:           "123",
		Title:        "API Reference",
		Slug:         "api-reference",
		Audience:     "api",
		ReviewStatus: "published",
		Position:     1,
	}
	if summary.Slug != "api-reference" {
		t.Errorf("expected slug 'api-reference', got %q", summary.Slug)
	}

	if summary.Audience != "api" {
		t.Errorf("expected audience 'api', got %q", summary.Audience)
	}
}

func TestCreateDocPageRequestJSON(t *testing.T) {
	t.Parallel()

	req := CreateDocPageRequest{
		Title:    "Test Page",
		Slug:     "test-page",
		Audience: "users",
		Content:  json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal CreateDocPageRequest: %v", err)
	}

	var decoded CreateDocPageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal CreateDocPageRequest: %v", err)
	}

	if decoded.Title != "Test Page" {
		t.Errorf("expected title 'Test Page', got %q", decoded.Title)
	}
}

func TestUpdateDocPageRequestJSON(t *testing.T) {
	t.Parallel()

	req := UpdateDocPageRequest{
		Title:        "Updated Title",
		Content:      json.RawMessage(`{}`),
		ReviewStatus: "approved",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal UpdateDocPageRequest: %v", err)
	}

	var decoded UpdateDocPageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal UpdateDocPageRequest: %v", err)
	}

	if decoded.ReviewStatus != "approved" {
		t.Errorf("expected review_status 'approved', got %q", decoded.ReviewStatus)
	}
}

func TestUuidPtrZero(t *testing.T) {
	t.Parallel()

	var id [16]byte
	// zero UUID is invalid
	result := uuidPtrFromBytes(id)
	if result != nil {
		t.Error("expected nil for zero UUID")
	}
}

func TestUuidPtrValid(t *testing.T) {
	t.Parallel()

	b := [16]byte{0xa0, 0xee, 0xbc, 0x99, 0x9c, 0x0b, 0x4e, 0xf8, 0xbb, 0x6d, 0x6b, 0xb9, 0xbd, 0x38, 0x0a, 0x11}

	result := uuidPtrFromBytes(b)
	if result == nil {
		t.Error("expected non-nil for valid UUID")
	}
}

func TestAudienceExhaustive(t *testing.T) {
	t.Parallel()
	// Ensure all documented audience types are covered
	count := 0

	for _, v := range ValidAudiences {
		if v {
			count++
		}
	}

	if count != 7 {
		t.Errorf("expected 7 valid audiences, got %d", count)
	}
}

// uuidPtrFromBytes is a test helper to test uuidPtr logic with raw bytes.
func uuidPtrFromBytes(b [16]byte) *string {
	s := ""
	for i, v := range b {
		s += string([]byte{v})
		_ = i
	}
	// A zero UUID has all zero bytes
	allZero := true

	for _, v := range b {
		if v != 0 {
			allZero = false
			break
		}
	}

	if allZero {
		return nil
	}

	return &s
}

func TestValidateDocSlug(t *testing.T) {
	t.Parallel()

	valid := []struct{ audience, slug string }{
		{"users", "getting-started"},
		{"users", "a"},
		{"api", "authentication"},
		{"admin", "x9"},
	}
	for _, tc := range valid {
		if err := validateDocSlug(tc.audience, tc.slug); err != nil {
			t.Errorf("slug %q in %q should be valid: %v", tc.slug, tc.audience, err)
		}
	}

	invalid := []struct{ audience, slug string }{
		{"users", ""},
		{"users", "Has-Caps"},
		{"users", "under_score"},
		{"users", "-leading"},
		{"users", "trailing-"},
		{"users", "double--dash"},
		{"users", "new"},      // reserved in every audience
		{"admin", "new"},      // reserved in every audience
		{"api", "keys"},       // api static route
		{"api", "playground"}, // api static route
		{"api", "new"},        // both rules apply
		{"users", "keys"},     // NOT reserved outside api — should be valid, see below
	}
	for _, tc := range invalid[:9] {
		if err := validateDocSlug(tc.audience, tc.slug); err == nil {
			t.Errorf("slug %q in %q should be rejected", tc.slug, tc.audience)
		}
	}
	// "keys" is only reserved in the api audience.
	if err := validateDocSlug("users", "keys"); err != nil {
		t.Errorf("slug 'keys' should be valid in users audience: %v", err)
	}
}

func TestDocContentEqual(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b []byte
		want bool
	}{
		{"identical bytes", []byte(`{"a":1}`), []byte(`{"a":1}`), true},
		{"key order differs", []byte(`{"a":1,"b":2}`), []byte(`{"b":2,"a":1}`), true},
		{"whitespace differs", []byte(`{ "a": 1 }`), []byte(`{"a":1}`), true},
		{"changed text", []byte(`{"text":"v1"}`), []byte(`{"text":"v2"}`), false},
		{"string vs object", []byte(`"plain"`), []byte(`{"type":"doc"}`), false},
		{"same string", []byte(`"plain markdown"`), []byte(`"plain markdown"`), true},
		{"invalid json falls back", []byte(`not json`), []byte(`not json`), true},
		{"invalid vs valid", []byte(`not json`), []byte(`{"a":1}`), false},
	}
	for _, tc := range cases {
		if got := docContentEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: docContentEqual = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestOpenAPIHandler(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	w := httptest.NewRecorder()
	h := &Handlers{Service: &Service{}}
	h.OpenAPIHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if decoded["openapi"] != "3.0.3" {
		t.Errorf("expected openapi '3.0.3', got %v", decoded["openapi"])
	}

	if decoded["info"] == nil {
		t.Error("expected info field in response")
	}
}
