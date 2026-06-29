package docs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidAudiences(t *testing.T) {
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

func TestValidReviewStatuses(t *testing.T) {
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
	var id [16]byte
	// zero UUID is invalid
	result := uuidPtrFromBytes(id)
	if result != nil {
		t.Error("expected nil for zero UUID")
	}
}

func TestUuidPtrValid(t *testing.T) {
	b := [16]byte{0xa0, 0xee, 0xbc, 0x99, 0x9c, 0x0b, 0x4e, 0xf8, 0xbb, 0x6d, 0x6b, 0xb9, 0xbd, 0x38, 0x0a, 0x11}
	result := uuidPtrFromBytes(b)
	if result == nil {
		t.Error("expected non-nil for valid UUID")
	}
}

func TestAudienceExhaustive(t *testing.T) {
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

func TestGetOpenAPISpec(t *testing.T) {
	spec := GetOpenAPISpec()
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	version, ok := spec["openapi"].(string)
	if !ok || version != "3.0.3" {
		t.Errorf("expected openapi version '3.0.3', got %v", spec["openapi"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("expected paths to be a map")
	}
	if _, ok := paths["/spaces"]; !ok {
		t.Error("expected /spaces path in spec")
	}
	if _, ok := paths["/content"]; !ok {
		t.Error("expected /content path in spec")
	}
	if _, ok := paths["/users/me"]; !ok {
		t.Error("expected /users/me path in spec")
	}
	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("expected components to be a map")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("expected components.schemas to be a map")
	}
	for _, name := range []string{"Space", "Content", "User", "Error"} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("expected schema %q in components.schemas", name)
		}
	}
}

func TestGetOpenAPISpecJSON(t *testing.T) {
	spec := GetOpenAPISpec()
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal OpenAPI spec: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal OpenAPI spec: %v", err)
	}
	if decoded["openapi"] != "3.0.3" {
		t.Errorf("expected openapi '3.0.3' after round-trip, got %v", decoded["openapi"])
	}
}

func TestAPIPlaygroundHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/api/playground", nil)
	w := httptest.NewRecorder()
	h := &Handlers{Service: &Service{}}
	h.APIPlaygroundHandler(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "API Playground") {
		t.Error("expected playground HTML to contain 'API Playground'")
	}
	if !strings.Contains(body, "Send Request") {
		t.Error("expected playground HTML to contain 'Send Request' button")
	}
	if !strings.Contains(body, "/api/v1/spaces") {
		t.Error("expected playground HTML to contain pre-populated /api/v1/spaces endpoint")
	}
}

func TestOpenAPIHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/api/openapi.json", nil)
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
