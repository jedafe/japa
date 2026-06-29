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
	"github.com/jedafe/japa/internal/tenant"
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
func withAuthAndTenant(r *http.Request, t *testing.T) *http.Request {
	user := &auth.AuthenticatedUser{
		UserID:   mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
		TenantID: mustUUID(t, "00000000-0000-0000-0000-000000000001"),
	}
	ten := &tenant.Tenant{
		ID:   mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Name: "Test Tenant",
		Slug: "test",
	}
	return r.WithContext(&multiValueCtx{
		Context: r.Context(),
		values: map[any]any{
			auth.UserContextKey:    user,
			tenant.TenantContextKey: ten,
		},
	})
}

// withAuthOnly injects only an auth user (no tenant) into the request context.
func withAuthOnly(r *http.Request, t *testing.T) *http.Request {
	user := &auth.AuthenticatedUser{
		UserID:   mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
		TenantID: mustUUID(t, "00000000-0000-0000-0000-000000000001"),
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
	h := newHandlers()
	if h == nil {
		t.Fatal("newHandlers returned nil")
	}
}

// ---------------------------------------------------------------------------
// ListSettings — unauthorized
// ---------------------------------------------------------------------------

func TestListSettings_Unauthorized(t *testing.T) {
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

func TestListSettings_TenantUnavailable(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req = withAuthOnly(req, t)
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
	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req = withAuthAndTenant(req, t)
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

func TestUpdateSettings_TenantUnavailable(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general", nil)
	req = withAuthOnly(req, t)
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
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/", nil)
	req = withAuthAndTenant(req, t)
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
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/nonexistent", strings.NewReader(`{"key":"val"}`))
	req = withAuthAndTenant(req, t)
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
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general", strings.NewReader("not json"))
	req = withAuthAndTenant(req, t)
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
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general", strings.NewReader(`{}`))
	req = withAuthAndTenant(req, t)
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
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general",
		strings.NewReader(`{"nonexistent_key": "value"}`))
	req = withAuthAndTenant(req, t)
	req = withChiURLParam(req, "category", "general")
	rr := httptest.NewRecorder()

	// Key validation passes, then h.DB.Writer.Begin panics on nil DB.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from nil DB, but none occurred")
		}
	}()

	h.UpdateSettings(rr, req)
}

// ---------------------------------------------------------------------------
// UpdateSettings — valid key but DB nil causes panic (nil pointer on h.DB.Writer)
// ---------------------------------------------------------------------------

func TestUpdateSettings_ValidKeyDBError(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPut, "/admin/settings/general",
		strings.NewReader(`{"site_name": "New Name"}`))
	req = withAuthAndTenant(req, t)
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

func TestListModeration_TenantUnavailable(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/moderation", nil)
	req = withAuthOnly(req, t)
	rr := httptest.NewRecorder()

	h.ListModeration(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// ListModeration — stub returns 200
// ---------------------------------------------------------------------------

func TestListModeration_StubOK(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodGet, "/admin/moderation", nil)
	req = withAuthAndTenant(req, t)
	rr := httptest.NewRecorder()

	h.ListModeration(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", body)
	}
	items, ok := data["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %v", data)
	}
	if len(items) != 0 {
		t.Errorf("expected empty items array, got %d", len(items))
	}
	if data["page"] != float64(0) {
		t.Errorf("page = %v, want 0", data["page"])
	}
	if data["total_pages"] != float64(0) {
		t.Errorf("total_pages = %v, want 0", data["total_pages"])
	}
}

// ---------------------------------------------------------------------------
// BulkModeration — unauthorized
// ---------------------------------------------------------------------------

func TestBulkModeration_Unauthorized(t *testing.T) {
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

func TestBulkModeration_TenantUnavailable(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/bulk", nil)
	req = withAuthOnly(req, t)
	rr := httptest.NewRecorder()

	h.BulkModeration(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// BulkModeration — stub returns 200
// ---------------------------------------------------------------------------

func TestBulkModeration_StubOK(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/bulk", nil)
	req = withAuthAndTenant(req, t)
	rr := httptest.NewRecorder()

	h.BulkModeration(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	data := body["data"].(map[string]any)
	if data["processed"] != float64(0) {
		t.Errorf("processed = %v, want 0", data["processed"])
	}
}

// ---------------------------------------------------------------------------
// AppealModeration — unauthorized
// ---------------------------------------------------------------------------

func TestAppealModeration_Unauthorized(t *testing.T) {
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

func TestAppealModeration_TenantUnavailable(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/abc123/appeal", nil)
	req = withAuthOnly(req, t)
	rr := httptest.NewRecorder()

	h.AppealModeration(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// AppealModeration — stub returns 200
// ---------------------------------------------------------------------------

func TestAppealModeration_StubOK(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/moderation/abc123/appeal", nil)
	req = withAuthAndTenant(req, t)
	rr := httptest.NewRecorder()

	h.AppealModeration(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	data := body["data"].(map[string]any)
	if data["status"] != "submitted" {
		t.Errorf("status = %v, want submitted", data["status"])
	}
}

// ---------------------------------------------------------------------------
// Broadcast — unauthorized
// ---------------------------------------------------------------------------

func TestBroadcast_Unauthorized(t *testing.T) {
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

func TestBroadcast_TenantUnavailable(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast",
		strings.NewReader(`{"subject":"hi","body":"hello"}`))
	req = withAuthOnly(req, t)
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
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast",
		strings.NewReader("not json"))
	req = withAuthAndTenant(req, t)
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
// Broadcast — valid stub returns 200
// ---------------------------------------------------------------------------

func TestBroadcast_StubOK(t *testing.T) {
	h := newHandlers()
	payload := `{"subject":"Update","body":"New features available!","target":"all","schedule":"now"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast", strings.NewReader(payload))
	req = withAuthAndTenant(req, t)
	rr := httptest.NewRecorder()

	h.Broadcast(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	data := body["data"].(map[string]any)
	if data["sent"] != float64(0) {
		t.Errorf("sent = %v, want 0", data["sent"])
	}
	if data["scheduled"] != false {
		t.Errorf("scheduled = %v, want false", data["scheduled"])
	}
}

// ---------------------------------------------------------------------------
// Broadcast — empty body
// ---------------------------------------------------------------------------

func TestBroadcast_EmptyBody(t *testing.T) {
	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast", strings.NewReader(`{}`))
	req = withAuthAndTenant(req, t)
	rr := httptest.NewRecorder()

	h.Broadcast(rr, req)

	// Empty body is valid JSON with zero-value fields → stub 200
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// defaultSettings — all expected categories present
// ---------------------------------------------------------------------------

func TestDefaultSettingsCategories(t *testing.T) {
	expected := []string{
		"general", "features", "email", "security",
		"appearance", "users", "content", "payment",
		"media", "seo", "integrations", "ai", "advanced",
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
	item := ModerationItem{
		ID:       "abc-123",
		Type:     "content",
		Status:   "pending",
		Content:  "Some content",
		Reporter: "user-1",
		Created:  "2026-01-01T00:00:00Z",
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
}

// ---------------------------------------------------------------------------
// BroadcastRequest JSON
// ---------------------------------------------------------------------------

func TestBroadcastRequestJSON(t *testing.T) {
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
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})

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
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "bad_request", "Invalid input")

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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withAuthOnly(req, t)
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

// ---------------------------------------------------------------------------
// requireTenant — from context
// ---------------------------------------------------------------------------

func TestRequireTenant_FromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withAuthAndTenant(req, t)
	rr := httptest.NewRecorder()

	tenantID, ok := requireTenant(rr, req)
	if !ok {
		t.Fatal("requireTenant returned false")
	}
	if tenantID == (pgtype.UUID{}) {
		t.Fatal("requireTenant returned zero UUID")
	}
}

// ---------------------------------------------------------------------------
// requireTenant — missing
// ---------------------------------------------------------------------------

func TestRequireTenant_Missing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	_, ok := requireTenant(rr, req)
	if ok {
		t.Fatal("requireTenant should return false without tenant")
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}
