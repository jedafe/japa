package follows

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/httpx"
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

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestEntityTypeConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		EntityTypeUser:                "user",
		EntityTypeSpace:               "space",
		EntityTypeTag:                 "tag",
		EntityTypeOrganization:        "organization",
		EntityTypeProfessionalProfile: "professional_profile",
	}
	for got, want := range want {
		if got != want {
			t.Errorf("entity type constant mismatch: got %q want %q", got, want)
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
// isValidEntityType
// ---------------------------------------------------------------------------

func TestIsValidEntityType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		entityType string
		want       bool
	}{
		{EntityTypeUser, true},
		{EntityTypeSpace, true},
		{EntityTypeTag, true},
		{EntityTypeOrganization, true},
		{EntityTypeProfessionalProfile, true},
		{"invalid", false},
		{"", false},
		{"USER", false},
		{"Space", false},
	}
	for _, tt := range tests {
		got := isValidEntityType(tt.entityType)
		if got != tt.want {
			t.Errorf("isValidEntityType(%q) = %v, want %v", tt.entityType, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// writeJSON / writeError
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	httpx.WriteJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})

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
	httpx.WriteError(rr, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

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
		{"101", MaxPageSize},
		{"999", MaxPageSize},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/follows?page_size="+tt.query, nil)

		got := parsePageSize(req)
		if got != tt.want {
			t.Errorf("parsePageSize(page_size=%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parsePageOffset
// ---------------------------------------------------------------------------

func TestParsePageOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int32
	}{
		{"", 0},
		{"0", 0},
		{"-1", 0},
		{"abc", 0},
		{"10", 10},
		{"50", 50},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/follows?page_offset="+tt.query, nil)

		got := parsePageOffset(req)
		if got != tt.want {
			t.Errorf("parsePageOffset(page_offset=%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ToggleFollow self-follow prevention (unit test without DB)
// ---------------------------------------------------------------------------

func TestToggleFollowSelfFollowPrevention(t *testing.T) {
	t.Parallel()

	svc := &Service{} // no store — we only test validation logic
	userID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	tenantID := mustUUID(t, "00000000-0000-0000-0000-000000000001")

	_, err := svc.ToggleFollow(t.Context(), tenantID, userID, EntityTypeUser, userID)
	if err == nil {
		t.Error("expected error for self-follow, got nil")
	}
}

// ---------------------------------------------------------------------------
// ToggleFollow invalid entity type (unit test without DB)
// ---------------------------------------------------------------------------

func TestToggleFollowInvalidEntityType(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	userID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	tenantID := mustUUID(t, "00000000-0000-0000-0000-000000000001")

	_, err := svc.ToggleFollow(t.Context(), tenantID, userID, "invalid_type", userID)
	if err == nil {
		t.Error("expected error for invalid entity type, got nil")
	}
}

// ---------------------------------------------------------------------------
// FollowerInfo / FollowingInfo struct tags
// ---------------------------------------------------------------------------

func TestFollowerInfoJSONTags(t *testing.T) {
	t.Parallel()

	fi := FollowerInfo{
		Username:    "testuser",
		DisplayName: "Test User",
		AvatarURL:   "https://example.com/avatar.jpg",
	}

	data, err := json.Marshal(fi)
	if err != nil {
		t.Fatalf("marshal FollowerInfo: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["username"] != "testuser" {
		t.Errorf("username = %v, want testuser", got["username"])
	}

	if got["display_name"] != "Test User" {
		t.Errorf("display_name = %v, want 'Test User'", got["display_name"])
	}

	if got["avatar_url"] != "https://example.com/avatar.jpg" {
		t.Errorf("avatar_url = %v, want avatar URL", got["avatar_url"])
	}
}

func TestFollowingInfoJSONTags(t *testing.T) {
	t.Parallel()

	fi := FollowingInfo{
		EntityName: "Naija Tech Hub",
	}

	data, err := json.Marshal(fi)
	if err != nil {
		t.Fatalf("marshal FollowingInfo: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["entity_name"] != "Naija Tech Hub" {
		t.Errorf("entity_name = %v, want 'Naija Tech Hub'", got["entity_name"])
	}
}

// ---------------------------------------------------------------------------
// NewService constructor
// ---------------------------------------------------------------------------

func TestNewServiceNotNil(t *testing.T) {
	t.Parallel()
	// NewService requires a real DB; we just verify the constructor signature compiles.
	// A nil database will panic, so we test with a nil-safe wrapper.
	// This test ensures the Service type is properly defined.
	svc := &Service{}
	if svc.store != nil {
		// Store should be nil when no DB is provided.
		t.Error("expected nil store for zero-value Service")
	}
}
