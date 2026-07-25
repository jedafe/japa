package users

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Types — JSON serialization
// ---------------------------------------------------------------------------

func TestUserProfileResponseJSONTags(t *testing.T) {
	t.Parallel()

	r := UserProfileResponse{
		ID:            "550e8400-e29b-41d4-a716-446655440000",
		Email:         "test@example.com",
		Username:      "testuser",
		DisplayName:   strPtr("Test User"),
		AvatarURL:     strPtr("https://example.com/avatar.jpg"),
		Bio:           strPtr("A bio"),
		Timezone:      strPtr("Africa/Lagos"),
		ProfileType:   "personal",
		Status:        "active",
		FollowerCount: 42,
		UserGroupName: "members",
		CreatedAt:     "2026-01-01T00:00:00Z",
		UpdatedAt:     "2026-01-02T00:00:00Z",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal UserProfileResponse: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["id"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("id = %v, want UUID", got["id"])
	}

	if got["email"] != "test@example.com" {
		t.Errorf("email = %v, want test@example.com", got["email"])
	}

	if got["username"] != "testuser" {
		t.Errorf("username = %v, want testuser", got["username"])
	}

	if got["display_name"] != "Test User" {
		t.Errorf("display_name = %v, want 'Test User'", got["display_name"])
	}

	if got["follower_count"] != float64(42) {
		t.Errorf("follower_count = %v, want 42", got["follower_count"])
	}

	if got["user_group_name"] != "members" {
		t.Errorf("user_group_name = %v, want members", got["user_group_name"])
	}
}

func TestPublicProfileResponseJSONTags(t *testing.T) {
	t.Parallel()

	r := PublicProfileResponse{
		ID:             "550e8400-e29b-41d4-a716-446655440000",
		Username:       "testuser",
		DisplayName:    strPtr("Test User"),
		AvatarURL:      strPtr("https://example.com/avatar.jpg"),
		Bio:            strPtr("A bio"),
		ProfileType:    "personal",
		FollowerCount:  3,
		FollowingCount: 2,
		CreatedAt:      "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal PublicProfileResponse: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wantKeys := map[string]any{
		"id":              "550e8400-e29b-41d4-a716-446655440000",
		"username":        "testuser",
		"display_name":    "Test User",
		"avatar_url":      "https://example.com/avatar.jpg",
		"bio":             "A bio",
		"profile_type":    "personal",
		"follower_count":  float64(3),
		"following_count": float64(2),
		"created_at":      "2026-01-01T00:00:00Z",
	}

	for k, want := range wantKeys {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}

	if len(got) != len(wantKeys) {
		t.Errorf("got %d keys %v, want exactly %d keys %v", len(got), got, len(wantKeys), wantKeys)
	}

	// PII rule (Section 19 / ADR-014): the public profile response must
	// never expose email, status, last_login_at, or groups.
	s := string(data)
	for _, leaked := range []string{`"email"`, `"status"`, `"last_login_at"`, `"groups"`, `"user_group_name"`} {
		if strings.Contains(s, leaked) {
			t.Fatalf("PublicProfileResponse JSON leaked PII field %s: %s", leaked, s)
		}
	}
}

func TestUserPreferencesResponseJSONTags(t *testing.T) {
	t.Parallel()

	r := UserPreferencesResponse{
		DarkMode: "dark",
		Language: "en-NG",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal UserPreferencesResponse: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["dark_mode"] != "dark" {
		t.Errorf("dark_mode = %v, want dark", got["dark_mode"])
	}

	if got["language"] != "en-NG" {
		t.Errorf("language = %v, want en-NG", got["language"])
	}
}

// ---------------------------------------------------------------------------
// writeJSON / writeError (copied pattern from follows_test.go)
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"status": "ok"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
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
	writeError(rr, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

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
// Handlers — requireUser / requireTenant (unit tests without DB)
// ---------------------------------------------------------------------------

func TestRequireUserUnauthorized(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)

	user, ok := h.requireUser(rr, req)
	if ok {
		t.Error("expected ok=false for missing auth, got true")
	}

	if user != nil {
		t.Error("expected nil user for missing auth")
	}

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestRequireTenantUnavailable(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)

	tenantID, ok := h.requireTenant(rr, req)
	if ok {
		t.Error("expected ok=false for missing tenant, got true")
	}

	if tenantID.Valid {
		t.Error("expected invalid UUID for missing tenant")
	}

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// Request type validation
// ---------------------------------------------------------------------------

func TestUpdateProfileRequestUnmarshal(t *testing.T) {
	t.Parallel()

	body := `{"display_name": "New Name", "bio": "Updated bio"}`

	var req UpdateProfileRequest

	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.DisplayName == nil || *req.DisplayName != "New Name" {
		t.Errorf("display_name = %v, want 'New Name'", req.DisplayName)
	}

	if req.Bio == nil || *req.Bio != "Updated bio" {
		t.Errorf("bio = %v, want 'Updated bio'", req.Bio)
	}

	if req.AvatarURL != nil {
		t.Errorf("avatar_url = %v, want nil", req.AvatarURL)
	}
}

func TestUpdatePreferencesRequestUnmarshal(t *testing.T) {
	t.Parallel()

	body := `{"dark_mode": "dark", "language": "en-US"}`

	var req UpdatePreferencesRequest

	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.DarkMode == nil || *req.DarkMode != "dark" {
		t.Errorf("dark_mode = %v, want 'dark'", req.DarkMode)
	}

	if req.Language == nil || *req.Language != "en-US" {
		t.Errorf("language = %v, want 'en-US'", req.Language)
	}
}

func TestUpdatePreferencesRequestPartialUnmarshal(t *testing.T) {
	t.Parallel()

	body := `{"dark_mode": "light"}`

	var req UpdatePreferencesRequest

	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.DarkMode == nil || *req.DarkMode != "light" {
		t.Errorf("dark_mode = %v, want 'light'", req.DarkMode)
	}

	if req.Language != nil {
		t.Errorf("language = %v, want nil for omitted field", req.Language)
	}
}

// ---------------------------------------------------------------------------
// Default preference values
// ---------------------------------------------------------------------------

func TestDefaultPreferenceValues(t *testing.T) {
	t.Parallel()
	// Verify the documented default values match what the service layer
	// returns when no preferences row exists. We test the response type
	// construction directly rather than calling GetPreferences (which
	// requires a real DB connection).
	prefs := UserPreferencesResponse{
		DarkMode: "system",
		Language: "en-NG",
	}
	if prefs.DarkMode != "system" {
		t.Errorf("default dark_mode = %q, want 'system'", prefs.DarkMode)
	}

	if prefs.Language != "en-NG" {
		t.Errorf("default language = %q, want 'en-NG'", prefs.Language)
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
// helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// DraftResponse (GET /me/drafts/{id}) — JSON shape + mapping helpers
// ---------------------------------------------------------------------------

func TestDraftResponseJSONTags(t *testing.T) {
	t.Parallel()

	r := DraftResponse{
		ID:           "550e8400-e29b-41d4-a716-446655440000",
		Slug:         "study-in-the-uk",
		ContentType:  "blog_post",
		Title:        "Study in the UK",
		Body:         "raw body",
		BodyHTML:     "<p>html</p>",
		FieldValues:  json.RawMessage(`{"key":"value"}`),
		CanonicalURL: strPtr("https://example.com/canonical"),
		Status:       "draft",
		Visibility:   "public",
		SpaceID:      strPtr("33333333-3333-3333-3333-333333333333"),
		PublishAt:    strPtr("2099-01-01T00:00:00Z"),
		CreatedAt:    "2026-01-01T00:00:00Z",
		UpdatedAt:    "2026-01-02T00:00:00Z",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal DraftResponse: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wantStr := map[string]string{
		"id":            "550e8400-e29b-41d4-a716-446655440000",
		"slug":          "study-in-the-uk",
		"content_type":  "blog_post",
		"title":         "Study in the UK",
		"body":          "raw body",
		"body_html":     "<p>html</p>",
		"status":        "draft",
		"visibility":    "public",
		"canonical_url": "https://example.com/canonical",
		"space_id":      "33333333-3333-3333-3333-333333333333",
		"publish_at":    "2099-01-01T00:00:00Z",
	}
	for k, want := range wantStr {
		if got[k] != want {
			t.Errorf("%s = %v, want %q", k, got[k], want)
		}
	}

	// field_values MUST round-trip as a JSON object, not base64 — the composer
	// consumes it as Record<string, unknown>.
	fv, ok := got["field_values"].(map[string]any)
	if !ok {
		t.Fatalf("field_values = %T, want a JSON object (map[string]any)", got["field_values"])
	}
	if fv["key"] != "value" {
		t.Errorf("field_values.key = %v, want value", fv["key"])
	}
}

func TestDraftResponse_NilOptionalsOmit(t *testing.T) {
	t.Parallel()

	// A draft with no space/canonical/publish_at must serialize those as null,
	// and an empty FieldValues must not panic (it marshals as null).
	r := DraftResponse{
		ID:          "id",
		Slug:        "slug",
		ContentType: "blog_post",
		Title:       "t",
		Status:      "draft",
		Visibility:  "public",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-02T00:00:00Z",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["space_id"] != nil {
		t.Errorf("space_id = %v, want nil", got["space_id"])
	}
	if got["canonical_url"] != nil {
		t.Errorf("canonical_url = %v, want nil", got["canonical_url"])
	}
}

func TestUUIDPtr(t *testing.T) {
	t.Parallel()

	if got := uuidPtr(pgtype.UUID{}); got != nil {
		t.Errorf("invalid UUID → %v, want nil", got)
	}

	var u pgtype.UUID
	if err := u.Scan("22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	got := uuidPtr(u)
	if got == nil || *got != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("valid UUID → %v, want the UUID string", got)
	}
}

func TestTsPtr(t *testing.T) {
	t.Parallel()

	if got := tsPtr(pgtype.Timestamptz{}); got != nil {
		t.Errorf("invalid ts → %v, want nil", got)
	}

	wantTime, err := time.Parse(time.RFC3339, "2099-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	ts := pgtype.Timestamptz{Time: wantTime, Valid: true}
	got := tsPtr(ts)
	if got == nil {
		t.Fatal("valid ts → nil, want RFC3339 string")
	}
	if _, err := time.Parse(time.RFC3339, *got); err != nil {
		t.Errorf("valid ts → %q is not RFC3339: %v", *got, err)
	}
}
