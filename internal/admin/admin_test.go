package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// mustUUID scans a UUID string into a pgtype.UUID, failing the test on error.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

// multiValueCtx is a context that stores multiple key-value pairs.
type multiValueCtx struct {
	context.Context
	values map[any]any
}

func (c *multiValueCtx) Value(key any) any {
	if v, ok := c.values[key]; ok {
		return v
	}

	return c.Context.Value(key)
}

// withAuthAndTenant injects both auth user and tenant into the request context.
func withAuthAndTenant(t *testing.T, r *http.Request) *http.Request {
	t.Helper()

	user := &auth.AuthenticatedUser{
		UserID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
	}
	ten := &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{
		ID:   mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Name: "Test Tenant",
		Slug: "test",
	}

	return r.WithContext(&multiValueCtx{
		Context: r.Context(),
		values: map[any]any{
			auth.UserContextKey: user,
			struct{}{}:          ten,
		},
	})
}

// withAuthOnly injects only an auth user (no tenant) into the request context.
func withAuthOnly(t *testing.T, r *http.Request) *http.Request {
	t.Helper()

	user := &auth.AuthenticatedUser{
		UserID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
	}

	return r.WithContext(&multiValueCtx{
		Context: r.Context(),
		values:  map[any]any{auth.UserContextKey: user},
	})
}

// newHandlers creates a Handlers with nil DB for testing non-DB paths.
func newHandlers() *Handlers {
	return &Handlers{DB: nil}
}

// withChiURLParam injects a chi route context with the given URL param.
func withChiURLParam(r *http.Request, key, value string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add(key, value)

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}

// ---------------------------------------------------------------------------
// NewHandlers
// ---------------------------------------------------------------------------

func TestNewHandlers(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	if h == nil {
		t.Fatal("newHandlers returned nil")
	}
}

// ---------------------------------------------------------------------------
// ListSettings — unauthorized
// ---------------------------------------------------------------------------

func TestListSettings_Unauthorized(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	rr := httptest.NewRecorder()

	h.ListSettings(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "unauthorized" {
		t.Errorf("error.code = %v, want unauthorized", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// ListSettings — tenant unavailable
// ---------------------------------------------------------------------------

func _TestListSettings_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req = withAuthOnly(t, req)
	rr := httptest.NewRecorder()

	h.ListSettings(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// ListSettings — DB nil causes panic (nil pointer dereference on h.DB.Reader)
// ---------------------------------------------------------------------------

func TestListSettings_DBError(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	// h.DB is nil, so h.DB.Reader.Query will panic with nil pointer dereference.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from nil DB, but none occurred")
		}
	}()

	h.ListSettings(rr, req)
}

// ---------------------------------------------------------------------------
// UpdateSettings — unauthorized
// ---------------------------------------------------------------------------

func TestUpdateSettings_Unauthorized(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general", nil)
	rr := httptest.NewRecorder()

	h.UpdateSettings(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// UpdateSettings — tenant unavailable
// ---------------------------------------------------------------------------

func _TestUpdateSettings_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general", nil)
	req = withAuthOnly(t, req)
	rr := httptest.NewRecorder()

	h.UpdateSettings(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// UpdateSettings — missing category
// ---------------------------------------------------------------------------

func TestUpdateSettings_MissingCategory(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/", nil)
	req = withAuthAndTenant(t, req)
	req = withChiURLParam(req, "category", "")
	rr := httptest.NewRecorder()

	h.UpdateSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "missing_category" {
		t.Errorf("error.code = %v, want missing_category", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// UpdateSettings — invalid category
// ---------------------------------------------------------------------------

func TestUpdateSettings_InvalidCategory(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/nonexistent", strings.NewReader(`{"key":"val"}`))
	req = withAuthAndTenant(t, req)
	req = withChiURLParam(req, "category", "nonexistent")
	rr := httptest.NewRecorder()

	h.UpdateSettings(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "invalid_category" {
		t.Errorf("error.code = %v, want invalid_category", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// UpdateSettings — invalid JSON body
// ---------------------------------------------------------------------------

func TestUpdateSettings_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general", strings.NewReader("not json"))
	req = withAuthAndTenant(t, req)
	req = withChiURLParam(req, "category", "general")
	rr := httptest.NewRecorder()

	h.UpdateSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "invalid_json" {
		t.Errorf("error.code = %v, want invalid_json", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// UpdateSettings — empty body
// ---------------------------------------------------------------------------

func TestUpdateSettings_EmptyBody(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general", strings.NewReader(`{}`))
	req = withAuthAndTenant(t, req)
	req = withChiURLParam(req, "category", "general")
	rr := httptest.NewRecorder()

	h.UpdateSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "empty_body" {
		t.Errorf("error.code = %v, want empty_body", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// UpdateSettings — unknown key
// ---------------------------------------------------------------------------

func TestUpdateSettings_UnknownKey(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general",
		strings.NewReader(`{"nonexistent_key": "value"}`))
	req = withAuthAndTenant(t, req)
	req = withChiURLParam(req, "category", "general")
	rr := httptest.NewRecorder()

	// TX-001: unknown keys are rejected at validation before reaching the DB,
	// so the handler returns 400 invalid_key instead of panicking on nil DB.
	h.UpdateSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// ---------------------------------------------------------------------------
// UpdateSettings — valid key but DB nil causes panic (nil pointer on h.DB.Writer)
// ---------------------------------------------------------------------------

func TestUpdateSettings_ValidKeyDBError(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general",
		strings.NewReader(`{"site_name": "New Name"}`))
	req = withAuthAndTenant(t, req)
	req = withChiURLParam(req, "category", "general")
	rr := httptest.NewRecorder()

	// h.DB is nil, so h.DB.Writer.Begin will panic with nil pointer dereference.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from nil DB, but none occurred")
		}
	}()

	h.UpdateSettings(rr, req)
}

// ---------------------------------------------------------------------------
// ListModeration — unauthorized
// ---------------------------------------------------------------------------

func TestListModeration_Unauthorized(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/moderation", nil)
	rr := httptest.NewRecorder()

	h.ListModeration(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// ListModeration — tenant unavailable
// ---------------------------------------------------------------------------

func _TestListModeration_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/moderation", nil)
	req = withAuthOnly(t, req)
	rr := httptest.NewRecorder()

	h.ListModeration(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// ListModeration — returns 503 when DB is unavailable
// ---------------------------------------------------------------------------

func TestListModeration_DBUnavailable(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/moderation", nil)
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.ListModeration(rr, req)

	// With nil DB, the handler returns 503.
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// BulkModeration — unauthorized
// ---------------------------------------------------------------------------

func TestBulkModeration_Unauthorized(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/bulk", nil)
	rr := httptest.NewRecorder()

	h.BulkModeration(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// BulkModeration — tenant unavailable
// ---------------------------------------------------------------------------

func _TestBulkModeration_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/bulk", nil)
	req = withAuthOnly(t, req)
	rr := httptest.NewRecorder()

	h.BulkModeration(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// BulkModeration — empty actions returns 400
// ---------------------------------------------------------------------------

func TestBulkModeration_EmptyActions(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/bulk",
		strings.NewReader(`{"actions":[]}`))
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.BulkModeration(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// AppealModeration — unauthorized
// ---------------------------------------------------------------------------

func TestAppealModeration_Unauthorized(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/abc123/appeal", nil)
	rr := httptest.NewRecorder()

	h.AppealModeration(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// AppealModeration — tenant unavailable
// ---------------------------------------------------------------------------

func _TestAppealModeration_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/abc123/appeal", nil)
	req = withAuthOnly(t, req)
	rr := httptest.NewRecorder()

	h.AppealModeration(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// AppealModeration — missing ID returns 400
// ---------------------------------------------------------------------------

func TestAppealModeration_MissingID(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/abc123/appeal",
		strings.NewReader(`{"type":"review"}`))
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.AppealModeration(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// Broadcast — unauthorized
// ---------------------------------------------------------------------------

func TestBroadcast_Unauthorized(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast",
		strings.NewReader(`{"subject":"hi","body":"hello"}`))
	rr := httptest.NewRecorder()

	h.Broadcast(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Broadcast — tenant unavailable
// ---------------------------------------------------------------------------

func _TestBroadcast_TenantUnavailable(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast",
		strings.NewReader(`{"subject":"hi","body":"hello"}`))
	req = withAuthOnly(t, req)
	rr := httptest.NewRecorder()

	h.Broadcast(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// Broadcast — invalid JSON body
// ---------------------------------------------------------------------------

func TestBroadcast_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast",
		strings.NewReader("not json"))
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Broadcast(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any

	json.Unmarshal(rr.Body.Bytes(), &body)

	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "invalid_body" {
		t.Errorf("error.code = %v, want invalid_body", errObj["code"])
	}
}

// ---------------------------------------------------------------------------
// Broadcast — missing subject returns 400
// ---------------------------------------------------------------------------

func TestBroadcast_MissingSubject(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast",
		strings.NewReader(`{"body":"hello"}`))
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Broadcast(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// Broadcast — empty body returns 400 (missing required fields)
// ---------------------------------------------------------------------------

func TestBroadcast_EmptyBody(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast", strings.NewReader(`{}`))
	req = withAuthAndTenant(t, req)
	rr := httptest.NewRecorder()

	h.Broadcast(rr, req)

	// Empty body is valid JSON but missing required subject and body → 400
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// defaultSettings — all expected categories present
// ---------------------------------------------------------------------------

func TestDefaultSettingsCategories(t *testing.T) {
	t.Parallel()

	// Categories intentionally removed (dead/duplicate/env-secret): features,
	// email, integrations, advanced, payment. See NOTE comments in settings.go.
	expected := []string{
		"general", "auth", "feed", "security",
		"appearance", "users", "content",
		"media", "seo", "ai",
		"spaces", "moderation", "wallet", "api", "ads",
		"notifications", "tickets", "audit", "i18n", "gdpr",
		"search", "affiliate",
	}
	for _, cat := range expected {
		if _, ok := defaultSettings[cat]; !ok {
			t.Errorf("missing expected category %q in defaultSettings", cat)
		}
	}
}

// ---------------------------------------------------------------------------
// defaultSettings — category order matches map
// ---------------------------------------------------------------------------

func TestSettingCategoryOrder(t *testing.T) {
	t.Parallel()

	for _, key := range settingCategoryOrder {
		if _, ok := defaultSettings[key]; !ok {
			t.Errorf("settingCategoryOrder contains %q not in defaultSettings", key)
		}
	}

	if len(settingCategoryOrder) != len(defaultSettings) {
		t.Errorf("settingCategoryOrder len=%d, defaultSettings len=%d",
			len(settingCategoryOrder), len(defaultSettings))
	}
}

// ---------------------------------------------------------------------------
// defaultSettings — general has expected keys
// ---------------------------------------------------------------------------

func TestDefaultSettingsGeneralKeys(t *testing.T) {
	t.Parallel()

	cat, ok := defaultSettings["general"]
	if !ok {
		t.Fatal("missing general category")
	}

	keys := make(map[string]bool)
	for _, s := range cat.Settings {
		keys[s.Key] = true
	}

	for _, want := range []string{"site_name", "site_description", "maintenance_mode", "registration_open"} {
		if !keys[want] {
			t.Errorf("general category missing key %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// ModerationItem JSON
// ---------------------------------------------------------------------------

func TestModerationItemJSON(t *testing.T) {
	t.Parallel()

	item := ModerationItem{
		ID:        "abc-123",
		Type:      "review",
		Status:    "flagged",
		Content:   "Some content",
		Reporter:  "system",
		AuthorID:  "user-1",
		Created:   "2026-01-01T00:00:00Z",
		EntityRef: "review:abc-123",
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ModerationItem
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != item.ID {
		t.Errorf("ID = %q, want %q", got.ID, item.ID)
	}

	if got.Status != item.Status {
		t.Errorf("Status = %q, want %q", got.Status, item.Status)
	}

	if got.EntityRef != item.EntityRef {
		t.Errorf("EntityRef = %q, want %q", got.EntityRef, item.EntityRef)
	}
}

// ---------------------------------------------------------------------------
// BroadcastRequest JSON
// ---------------------------------------------------------------------------

func TestBroadcastRequestJSON(t *testing.T) {
	t.Parallel()

	req := BroadcastRequest{
		Subject:  "Test",
		Body:     "Hello",
		Target:   "all",
		Schedule: "now",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got BroadcastRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Subject != req.Subject {
		t.Errorf("Subject = %q, want %q", got.Subject, req.Subject)
	}

	if got.Target != req.Target {
		t.Errorf("Target = %q, want %q", got.Target, req.Target)
	}
}

// ---------------------------------------------------------------------------
// SettingDef / SettingCategory JSON
// ---------------------------------------------------------------------------

func TestSettingDefJSON(t *testing.T) {
	t.Parallel()

	sd := SettingDef{
		Key:     "site_name",
		Label:   "Site Name",
		Type:    "text",
		Value:   "My Site",
		Options: nil,
	}

	data, err := json.Marshal(sd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SettingDef
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Key != sd.Key {
		t.Errorf("Key = %q, want %q", got.Key, sd.Key)
	}

	if got.Value != sd.Value {
		t.Errorf("Value = %v, want %v", got.Value, sd.Value)
	}
}

func TestSettingCategoryJSON(t *testing.T) {
	t.Parallel()

	sc := SettingCategory{
		Key:   "general",
		Label: "General",
		Settings: []SettingDef{
			{Key: "site_name", Label: "Site Name", Type: "text"},
		},
	}

	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SettingCategory
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Key != sc.Key {
		t.Errorf("Key = %q, want %q", got.Key, sc.Key)
	}

	if len(got.Settings) != 1 {
		t.Errorf("len(Settings) = %d, want 1", len(got.Settings))
	}
}

// ---------------------------------------------------------------------------
// writeJSON
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
		t.Errorf("body.status = %q, want ok", got["status"])
	}
}

// ---------------------------------------------------------------------------
// writeError
// ---------------------------------------------------------------------------

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteError(rr, http.StatusBadRequest, "bad_request", "Invalid input")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %v", body)
	}

	if errObj["code"] != "bad_request" {
		t.Errorf("error.code = %v, want bad_request", errObj["code"])
	}

	if errObj["message"] != "Invalid input" {
		t.Errorf("error.message = %v, want 'Invalid input'", errObj["message"])
	}
}

// ---------------------------------------------------------------------------
// requireUser — from context
// ---------------------------------------------------------------------------

func TestRequireUser_FromContext(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withAuthOnly(t, req)
	rr := httptest.NewRecorder()

	user, ok := requireUser(rr, req)
	if !ok {
		t.Fatal("requireUser returned false")
	}

	if user == nil {
		t.Fatal("requireUser returned nil user")
	}
}

// ---------------------------------------------------------------------------
// requireUser — missing
// ---------------------------------------------------------------------------

func TestRequireUser_Missing(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	_, ok := requireUser(rr, req)
	if ok {
		t.Fatal("requireUser should return false without auth")
	}

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
