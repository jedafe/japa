package onboarding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Types — JSON serialization
// ---------------------------------------------------------------------------

func TestOnboardingStatusResponseJSONTags(t *testing.T) {
	t.Parallel()

	r := OnboardingStatusResponse{
		CurrentStep:   2,
		Completed:     false,
		StepData:      json.RawMessage(`{"name":"Chidinma"}`),
		TourDismissed: false,
		CreatedAt:     "2026-01-01T00:00:00Z",
		UpdatedAt:     "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal OnboardingStatusResponse: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["current_step"] != float64(2) {
		t.Errorf("current_step = %v, want 2", got["current_step"])
	}

	if got["completed"] != false {
		t.Errorf("completed = %v, want false", got["completed"])
	}
}

func TestOnboardingRecommendationJSONTags(t *testing.T) {
	t.Parallel()

	r := OnboardingRecommendation{
		Type:      "space",
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Name:      "Canada Immigrants",
		Slug:      "canada-immigrants",
		AvatarURL: "https://example.com/avatar.jpg",
		Reason:    "geo_match",
		Score:     0.85,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal OnboardingRecommendation: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["type"] != "space" {
		t.Errorf("type = %v, want space", got["type"])
	}

	if got["reason"] != "geo_match" {
		t.Errorf("reason = %v, want geo_match", got["reason"])
	}
}

func TestUpdateStepRequestUnmarshal(t *testing.T) {
	t.Parallel()

	body := `{"step": 3, "step_data": {"interests": ["immigration", "career"]}}`

	var req UpdateStepRequest

	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.Step != 3 {
		t.Errorf("step = %d, want 3", req.Step)
	}

	if len(req.StepData) == 0 {
		t.Error("step_data should not be empty")
	}
}

func TestDismissTourRequestUnmarshal(t *testing.T) {
	t.Parallel()

	var req DismissTourRequest
	if err := json.Unmarshal([]byte(`{"surface":"docs"}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.Surface != "docs" {
		t.Errorf("surface = %q, want docs", req.Surface)
	}

	// Empty body decodes to the zero value — the service defaults it to "main".
	var empty DismissTourRequest
	if err := json.Unmarshal([]byte(`{}`), &empty); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}

	if empty.Surface != "" {
		t.Errorf("surface = %q, want empty", empty.Surface)
	}
}

func TestTourDismissalsJSONTags(t *testing.T) {
	t.Parallel()

	r := OnboardingStatusResponse{
		TourDismissed:  true,
		TourDismissals: map[string]bool{"main": true, "docs": true},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	dismissals, ok := got["tour_dismissals"].(map[string]any)
	if !ok {
		t.Fatalf("tour_dismissals = %v, want object", got["tour_dismissals"])
	}

	if dismissals["docs"] != true {
		t.Errorf("tour_dismissals[docs] = %v, want true", dismissals["docs"])
	}
}

// ---------------------------------------------------------------------------
// tour dismissal helpers
// ---------------------------------------------------------------------------

func TestNormalizeTourSurface(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"", "main"},
		{"   ", "main"},
		{"main", "main"},
		{"Docs", "docs"},
		{"  LEARN  ", "academy"},
		{"admin-panel", "admin-panel"},
		{"main'; DROP TABLE users;--", "maindroptableusers--"},
		{strings.Repeat("a", 64), strings.Repeat("a", 32)},
		{"!!!", ""}, // no valid characters — caller rejects with 400
	}

	for _, tc := range cases {
		if got := normalizeTourSurface(tc.in); got != tc.want {
			t.Errorf("normalizeTourSurface(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsTourStepKey(t *testing.T) {
	t.Parallel()

	valid := []string{"primary-nav", "docs-search", "a", "step1", "academy-account"}
	for _, k := range valid {
		if !isTourStepKey(k) {
			t.Errorf("isTourStepKey(%q) = false, want true", k)
		}
	}

	invalid := []string{"", "Primary-Nav", "space switcher", "a_b", `<x>`, strings.Repeat("k", 65)}
	for _, k := range invalid {
		if isTourStepKey(k) {
			t.Errorf("isTourStepKey(%q) = true, want false", k)
		}
	}
}

func TestParseTourDismissals(t *testing.T) {
	t.Parallel()

	// Empty/absent jsonb decodes to an empty map, not nil.
	got := parseTourDismissals(nil)
	if len(got) != 0 {
		t.Errorf("parseTourDismissals(nil) = %v, want empty map", got)
	}

	if got == nil {
		t.Error("parseTourDismissals(nil) should return a non-nil map")
	}

	got = parseTourDismissals([]byte(`{"main":true,"docs":true}`))
	if !got["main"] || !got["docs"] {
		t.Errorf("parseTourDismissals valid = %v, want main+docs", got)
	}

	if got["academy"] {
		t.Error("parseTourDismissals valid: academy should be absent")
	}

	// Corrupted payload degrades to an empty map rather than failing.
	got = parseTourDismissals([]byte(`{not json`))
	if len(got) != 0 {
		t.Errorf("parseTourDismissals corrupted = %v, want empty map", got)
	}
}

// ---------------------------------------------------------------------------
// mergeJSON
// ---------------------------------------------------------------------------

func TestMergeJSON(t *testing.T) {
	t.Parallel()

	base := json.RawMessage(`{"name":"Alice","step":1}`)
	overlay := json.RawMessage(`{"step":2,"location":"Lagos"}`)
	merged := mergeJSON(base, overlay)

	var result map[string]any
	if err := json.Unmarshal(merged, &result); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}

	if result["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", result["name"])
	}

	if result["step"] != float64(2) {
		t.Errorf("step = %v, want 2", result["step"])
	}

	if result["location"] != "Lagos" {
		t.Errorf("location = %v, want Lagos", result["location"])
	}
}

func TestMergeJSONEmptyOverlay(t *testing.T) {
	t.Parallel()

	base := json.RawMessage(`{"key":"value"}`)
	merged := mergeJSON(base, nil)

	var result map[string]any
	if err := json.Unmarshal(merged, &result); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("key = %v, want value", result["key"])
	}
}

func TestMergeJSONEmptyBase(t *testing.T) {
	t.Parallel()

	overlay := json.RawMessage(`{"new":"data"}`)
	merged := mergeJSON(nil, overlay)

	var result map[string]any
	if err := json.Unmarshal(merged, &result); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}

	if result["new"] != "data" {
		t.Errorf("new = %v, want data", result["new"])
	}
}

// ---------------------------------------------------------------------------
// tagOverlapScore
// ---------------------------------------------------------------------------

func TestTagOverlapScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
		tags        []string
		minScore    float64
	}{
		{
			name:        "perfect match",
			description: "A community for immigration and career advice",
			tags:        []string{"immigration", "career"},
			minScore:    0.5,
		},
		{
			name:        "partial match",
			description: "Education and housing discussions",
			tags:        []string{"immigration", "career", "education"},
			minScore:    0.2,
		},
		{
			name:        "no match",
			description: "General chat about movies",
			tags:        []string{"immigration", "career"},
			minScore:    0.0,
		},
		{
			name:        "empty description",
			description: "",
			tags:        []string{"immigration"},
			minScore:    0.0,
		},
		{
			name:        "empty tags",
			description: "Some description",
			tags:        []string{},
			minScore:    0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			score := tagOverlapScore(tt.description, tt.tags)
			if score < tt.minScore {
				t.Errorf("tagOverlapScore = %f, want >= %f", score, tt.minScore)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// writeJSON / writeError (copied pattern from users_test.go)
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteJSON(rr, http.StatusOK, map[string]string{"status": "ok"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
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
}

// ---------------------------------------------------------------------------
// NewHandlers constructor
// ---------------------------------------------------------------------------

func TestNewHandlersNotNil(t *testing.T) {
	t.Parallel()

	svc := &Service{}

	h := NewHandlers(svc)
	if h == nil {
		t.Fatal("NewHandlers returned nil")
	}

	if h.Service != svc {
		t.Error("handlers.Service does not match provided service")
	}
}

// ---------------------------------------------------------------------------
// parseStepData
// ---------------------------------------------------------------------------

func TestParseStepData(t *testing.T) {
	t.Parallel()

	data := json.RawMessage(`{"interests":["career","housing"],"current_location":"Lagos","name":"Chidinma"}`)
	fields := parseStepData(data)

	if fields.Name != "Chidinma" {
		t.Errorf("name = %q, want Chidinma", fields.Name)
	}

	if fields.CurrentLocation != "Lagos" {
		t.Errorf("current_location = %q, want Lagos", fields.CurrentLocation)
	}

	if len(fields.Interests) != 2 {
		t.Errorf("interests length = %d, want 2", len(fields.Interests))
	}
}

func TestParseStepDataEmpty(t *testing.T) {
	t.Parallel()

	fields := parseStepData(nil)
	if fields.Name != "" {
		t.Errorf("name = %q, want empty", fields.Name)
	}

	if len(fields.Interests) != 0 {
		t.Errorf("interests should be empty")
	}
}
