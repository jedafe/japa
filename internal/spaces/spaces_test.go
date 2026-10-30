package spaces

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// requestWithURLParam builds a request whose chi route context carries the
// given URL parameter, mimicking what the chi router injects during routing.
// The body may be empty for handlers that do not read the request body.
func requestWithURLParam(method, target, param, value, body string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(param, value)

	r := httptest.NewRequest(method, target, strings.NewReader(body))

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// errorResponse mirrors the canonical error envelope for test decoding.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// decodeErrorResponse unmarshals an error envelope from the response body,
// failing the test when the body is not valid JSON.
func decodeErrorResponse(t *testing.T, body []byte) errorResponse {
	t.Helper()

	var got errorResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}

	return got
}

// mustUUID scans a UUID string into a pgtype.UUID, failing the test on error.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

func TestIsValidLevel(t *testing.T) {
	t.Parallel()

	for _, level := range []string{LevelCountry, LevelState, LevelCity, LevelCustom, LevelUniversity} {
		if !IsValidLevel(level) {
			t.Errorf("IsValidLevel(%q) = false, want true", level)
		}
	}

	for _, level := range []string{"foo", "", "planet"} {
		if IsValidLevel(level) {
			t.Errorf("IsValidLevel(%q) = true, want false", level)
		}
	}
}

func TestExtractSpaceKeywords(t *testing.T) {
	t.Parallel()

	got := extractSpaceKeywords("Frontend Devs in Lagos, Nigeria")
	want := []string{"frontend", "devs", "lagos", "nigeria", "frontend devs", "devs in", "in lagos", "lagos nigeria"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractSpaceKeywords() = %v, want %v", got, want)
	}

	if extractSpaceKeywords("") != nil {
		t.Error("extractSpaceKeywords(\"\") should return nil")
	}

	if extractSpaceKeywords("a") != nil {
		t.Error("extractSpaceKeywords(\"a\") should return nil")
	}
}

func TestValidPermissionAxes(t *testing.T) {
	t.Parallel()

	valid := []SpacePermAxes{
		{VisibilityPublic, PermOpen, PermMembers, PermModerators, JoiningOpen},
		{VisibilityMembers, PermAdmins, PermVerified, PermMembers, JoiningRequest},
		{VisibilityPrivate, PermModerators, PermOpen, PermAdmins, JoiningInvite},
	}
	for _, axes := range valid {
		if !ValidPermissionAxes(axes) {
			t.Errorf("ValidPermissionAxes(%+v) = false, want true", axes)
		}
	}

	invalid := []SpacePermAxes{
		{Visibility: "garbage", Posting: PermOpen, Replying: PermOpen, Commenting: PermOpen, Joining: JoiningOpen},
		{Visibility: VisibilityPublic, Posting: "nope", Replying: PermOpen, Commenting: PermOpen, Joining: JoiningOpen},
		{Visibility: VisibilityPublic, Posting: PermOpen, Replying: "nope", Commenting: PermOpen, Joining: JoiningOpen},
		{Visibility: VisibilityPublic, Posting: PermOpen, Replying: PermOpen, Commenting: "nope", Joining: JoiningOpen},
		{Visibility: VisibilityPublic, Posting: PermOpen, Replying: PermOpen, Commenting: PermOpen, Joining: "nope"},
		{Visibility: "", Posting: PermOpen, Replying: PermOpen, Commenting: PermOpen, Joining: JoiningOpen},
	}
	for _, axes := range invalid {
		if ValidPermissionAxes(axes) {
			t.Errorf("ValidPermissionAxes(%+v) = true, want false", axes)
		}
	}
}

func TestSpacePermAxes_Constants(t *testing.T) {
	t.Parallel()

	wantLevels := map[string]string{
		LevelCountry: "country", LevelState: "state", LevelCity: "city", LevelCustom: "custom",
	}
	for got, want := range wantLevels {
		if got != want {
			t.Errorf("level constant mismatch: got %q want %q", got, want)
		}
	}

	wantVisibility := map[string]string{
		VisibilityPublic: "public", VisibilityMembers: "members", VisibilityPrivate: "private",
	}
	for got, want := range wantVisibility {
		if got != want {
			t.Errorf("visibility constant mismatch: got %q want %q", got, want)
		}
	}

	wantPerms := map[string]string{
		PermOpen: "open", PermMembers: "members", PermVerified: "verified",
		PermModerators: "moderators", PermAdmins: "admins",
	}
	for got, want := range wantPerms {
		if got != want {
			t.Errorf("perm constant mismatch: got %q want %q", got, want)
		}
	}

	wantJoining := map[string]string{
		JoiningOpen: "open", JoiningRequest: "request", JoiningInvite: "invite",
	}
	for got, want := range wantJoining {
		if got != want {
			t.Errorf("joining constant mismatch: got %q want %q", got, want)
		}
	}

	wantStatus := map[string]string{
		StatusActive: "active", StatusArchived: "archived", StatusDraft: "draft",
	}
	for got, want := range wantStatus {
		if got != want {
			t.Errorf("status constant mismatch: got %q want %q", got, want)
		}
	}
}

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
	errpkg.WriteError(rr, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	got := decodeErrorResponse(t, rr.Body.Bytes())
	if got.Error.Code != "invalid_body" {
		t.Errorf("error.code = %q, want invalid_body", got.Error.Code)
	}

	if got.Error.Message != "Request body is not valid JSON" {
		t.Errorf("error.message = %q, want Request body is not valid JSON", got.Error.Message)
	}
}

func TestUnreadCounter_NilClient(t *testing.T) {
	t.Parallel()

	c := NewUnreadCounter(nil)
	uid := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	sid := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")

	if err := c.Increment(t.Context(), uid, sid); err != nil {
		t.Errorf("Increment with nil client returned error: %v", err)
	}

	if err := c.Reset(t.Context(), uid, sid); err != nil {
		t.Errorf("Reset with nil client returned error: %v", err)
	}

	n, err := c.Get(t.Context(), uid, sid)
	if err != nil {
		t.Errorf("Get with nil client returned error: %v", err)
	}

	if n != 0 {
		t.Errorf("Get with nil client = %d, want 0", n)
	}

	batch, err := c.GetBatch(t.Context(), uid, []pgtype.UUID{sid})
	if err != nil {
		t.Errorf("GetBatch with nil client returned error: %v", err)
	}

	if len(batch) != 0 {
		t.Errorf("GetBatch with nil client returned %d entries, want 0", len(batch))
	}
}

func TestCreateSpace_MissingFields(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil, nil)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/spaces", strings.NewReader(`{}`))
	h.CreateSpace(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "missing_fields" {
		t.Errorf("error.code = %q, want missing_fields", got.Error.Code)
	}
}

func TestCreateSpace_InvalidLevel(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil, nil)
	rr := httptest.NewRecorder()
	body := `{"name":"Test","slug":"test","space_type_id":"550e8400-e29b-41d4-a716-446655440000","level":"planet"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/spaces", strings.NewReader(body))
	h.CreateSpace(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_level" {
		t.Errorf("error.code = %q, want invalid_level", got.Error.Code)
	}
}

func TestCreateSpaceType_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil, nil)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/space-types", strings.NewReader(`{not json`))
	h.CreateSpaceType(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_body" {
		t.Errorf("error.code = %q, want invalid_body", got.Error.Code)
	}
}

func TestIsAllowedCategory(t *testing.T) {
	t.Parallel()

	for _, cat := range []string{"education", "business", "profession", "interest", "lifestyle"} {
		if !isAllowedCategory(cat) {
			t.Errorf("isAllowedCategory(%q) = false, want true", cat)
		}
	}

	for _, cat := range []string{"community", "global", "country", "", "foo"} {
		if isAllowedCategory(cat) {
			t.Errorf("isAllowedCategory(%q) = true, want false", cat)
		}
	}
}

func _TestListSpaceTypes_NoPlatform(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil, nil)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/space-types", nil)
	h.ListSpaceTypes(rr, r)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "service_unavailable" {
		t.Errorf("error.code = %q, want service_unavailable", got.Error.Code)
	}
}

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	const valid = "550e8400-e29b-41d4-a716-446655440000"
	r := requestWithURLParam(http.MethodGet, "/", "id", valid, "")

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		t.Fatalf("parseUUIDParam valid UUID: %v", err)
	}

	if !id.Valid {
		t.Fatal("expected valid pgtype.UUID, got invalid zero value")
	}

	r2 := requestWithURLParam(http.MethodGet, "/", "id", "not-a-uuid", "")
	if _, err := parseUUIDParam(r2, "id"); err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestDefaultPermAxes(t *testing.T) {
	t.Parallel()

	axes := DefaultPermAxes()
	if axes.Visibility != VisibilityPublic {
		t.Errorf("default visibility = %q, want %q", axes.Visibility, VisibilityPublic)
	}

	if axes.Posting != PermMembers {
		t.Errorf("default posting = %q, want %q", axes.Posting, PermMembers)
	}

	if axes.Replying != PermMembers {
		t.Errorf("default replying = %q, want %q", axes.Replying, PermMembers)
	}

	if axes.Commenting != PermMembers {
		t.Errorf("default commenting = %q, want %q", axes.Commenting, PermMembers)
	}

	if axes.Joining != JoiningOpen {
		t.Errorf("default joining = %q, want %q", axes.Joining, JoiningOpen)
	}

	if !ValidPermissionAxes(axes) {
		t.Error("default axes should be valid")
	}
}
