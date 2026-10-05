package roadmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// withUser injects an AuthenticatedUser into the request context the same
// way auth.RequireAuth middleware does.
func withUser(r *http.Request) *http.Request {
	var uid pgtype.UUID
	if err := uid.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		panic(err)
	}

	return r.WithContext(context.WithValue(r.Context(), auth.UserContextKey,
		&auth.AuthenticatedUser{UserID: uid}))
}

// withChiParam injects a chi route parameter into the request context.
func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func newHandlers() *Handlers {
	return NewHandlers(&Service{})
}

// ---------------------------------------------------------------------------
// Query/path parsing helpers
// ---------------------------------------------------------------------------

func TestParsePageSize(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/roadmap", nil)
	if ps := parsePageSize(r); ps != DefaultPageSize {
		t.Errorf("expected default page size %d, got %d", DefaultPageSize, ps)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/roadmap?page_size=50", nil)
	if ps := parsePageSize(r2); ps != 50 {
		t.Errorf("expected page size 50, got %d", ps)
	}

	r3 := httptest.NewRequest(http.MethodGet, "/roadmap?page_size=200", nil)
	if ps := parsePageSize(r3); ps != MaxPageSize {
		t.Errorf("expected max page size %d, got %d", MaxPageSize, ps)
	}

	r4 := httptest.NewRequest(http.MethodGet, "/roadmap?page_size=abc", nil)
	if ps := parsePageSize(r4); ps != DefaultPageSize {
		t.Errorf("expected default page size for invalid input, got %d", ps)
	}
}

func TestParsePageSizeOffset(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/roadmap", nil)
	if po := parsePageSizeOffset(r); po != 0 {
		t.Errorf("expected default offset 0, got %d", po)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/roadmap?page_offset=20", nil)
	if po := parsePageSizeOffset(r2); po != 20 {
		t.Errorf("expected offset 20, got %d", po)
	}

	r3 := httptest.NewRequest(http.MethodGet, "/roadmap?page_offset=-5", nil)
	if po := parsePageSizeOffset(r3); po != 0 {
		t.Errorf("expected 0 for negative offset, got %d", po)
	}
}

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	r := withChiParam(httptest.NewRequest(http.MethodGet, "/x", nil), "id", "11111111-1111-1111-1111-111111111111")

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		t.Fatalf("expected valid UUID parse, got %v", err)
	}

	if !id.Valid {
		t.Error("expected valid UUID")
	}

	r2 := withChiParam(httptest.NewRequest(http.MethodGet, "/x", nil), "id", "not-a-uuid")
	if _, err := parseUUIDParam(r2, "id"); err == nil {
		t.Error("expected error for invalid UUID")
	}
}

// ---------------------------------------------------------------------------
// Error envelope mapping
// ---------------------------------------------------------------------------

func TestHandleServiceErrorRoadmapSentinels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantCode int
		wantErr  string
	}{
		{"already voted", ErrAlreadyVoted, http.StatusConflict, "already_voted"},
		{"not voted", ErrNotVoted, http.StatusConflict, "not_voted"},
		{"not found", errpkg.ErrNotFound, http.StatusNotFound, "not_found"},
		{"invalid transition", ErrInvalidTransition, http.StatusConflict, "invalid_status_transition"},
		{"submissions disabled", ErrSubmissionsDisabled, http.StatusNotFound, "feature_disabled"},
		{"not deleted", errpkg.ErrNotDeleted, http.StatusConflict, "not_deleted"},
		{"required", errpkg.ErrRequired, http.StatusBadRequest, "validation_error"},
		{"unknown", errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		handleServiceError(w, tc.err)

		if w.Code != tc.wantCode {
			t.Errorf("%s: expected status %d, got %d", tc.name, tc.wantCode, w.Code)
			continue
		}

		var body map[string]map[string]string
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("%s: failed to decode error response: %v", tc.name, err)
		}

		if body["error"]["code"] != tc.wantErr {
			t.Errorf("%s: expected error code %q, got %q", tc.name, tc.wantErr, body["error"]["code"])
		}
	}
}

// ---------------------------------------------------------------------------
// Handler unit tests — unauthenticated requests (401 before service calls)
// ---------------------------------------------------------------------------

func TestCastVoteHandlerNoAuth(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/roadmap/items/123/vote", nil)
	w := httptest.NewRecorder()

	h.CastVote(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRemoveVoteHandlerNoAuth(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/roadmap/items/123/vote", nil)
	w := httptest.NewRecorder()

	h.RemoveVote(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreateSubmissionHandlerNoAuth(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/roadmap/submissions",
		bytes.NewBuffer([]byte(`{"title":"x"}`)))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	h.CreateSubmission(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminHandlersNoAuth(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	cases := []struct {
		name    string
		method  string
		path    string
		body    string
		handler http.HandlerFunc
	}{
		{"list items", http.MethodGet, "/api/v1/admin/roadmap/items", "", h.AdminListItems},
		{"list submissions", http.MethodGet, "/api/v1/admin/roadmap/submissions", "", h.AdminListSubmissions},
		{"get item", http.MethodGet, "/api/v1/admin/roadmap/items/123", "", h.AdminGetItem},
		{"create item", http.MethodPost, "/api/v1/admin/roadmap/items", `{"title":"x"}`, h.AdminCreateItem},
		{"update item", http.MethodPatch, "/api/v1/admin/roadmap/items/123", `{"title":"x"}`, h.AdminUpdateItem},
		{"set status", http.MethodPatch, "/api/v1/admin/roadmap/items/123/status", `{"status":"planned"}`, h.AdminSetStatus},
		{"delete item", http.MethodDelete, "/api/v1/admin/roadmap/items/123", "", h.AdminDeleteItem},
		{"restore item", http.MethodPost, "/api/v1/admin/roadmap/items/123/restore", "", h.AdminRestoreItem},
		{"reorder", http.MethodPost, "/api/v1/admin/roadmap/items/reorder", `{"items":[]}`, h.AdminReorderItems},
		{"list categories", http.MethodGet, "/api/v1/admin/roadmap/categories", "", h.AdminListCategories},
		{"create category", http.MethodPost, "/api/v1/admin/roadmap/categories", `{"name":"x"}`, h.AdminCreateCategory},
		{"update category", http.MethodPatch, "/api/v1/admin/roadmap/categories/123", `{"name":"x"}`, h.AdminUpdateCategory},
		{"delete category", http.MethodDelete, "/api/v1/admin/roadmap/categories/123", "", h.AdminDeleteCategory},
	}

	for _, tc := range cases {
		var req *http.Request
		if tc.body != "" {
			req = httptest.NewRequest(tc.method, tc.path, bytes.NewBuffer([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(tc.method, tc.path, nil)
		}

		w := httptest.NewRecorder()
		tc.handler(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", tc.name, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Handler unit tests — authenticated requests that fail validation before
// reaching the Service (so a nil-dependency &Service{} is safe).
// ---------------------------------------------------------------------------

func TestCastVoteHandlerInvalidID(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := withChiParam(httptest.NewRequest(http.MethodPost, "/x", nil), "id", "not-a-uuid")
	req = withUser(req)
	w := httptest.NewRecorder()

	h.CastVote(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateSubmissionHandlerValidation(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	cases := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{"missing title", `{}`, http.StatusBadRequest, "missing_title"},
		{"invalid body", `not-json`, http.StatusBadRequest, "invalid_body"},
		{"bad category", `{"title":"x","category_id":"zzz"}`, http.StatusBadRequest, "invalid_category_id"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/roadmap/submissions", bytes.NewBuffer([]byte(tc.body)))
		req = withUser(req)
		w := httptest.NewRecorder()

		h.CreateSubmission(w, req)

		if w.Code != tc.wantCode {
			t.Errorf("%s: expected %d, got %d", tc.name, tc.wantCode, w.Code)
			continue
		}

		var body map[string]map[string]string
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("%s: decode failed: %v", tc.name, err)
		}

		if body["error"]["code"] != tc.wantErr {
			t.Errorf("%s: expected error code %q, got %q", tc.name, tc.wantErr, body["error"]["code"])
		}
	}
}

func TestAdminSetStatusHandlerValidation(t *testing.T) {
	t.Parallel()

	h := newHandlers()

	// Missing status → 400.
	req := withUser(withChiParam(
		httptest.NewRequest(http.MethodPatch, "/x", bytes.NewBuffer([]byte(`{}`))),
		"id", "11111111-1111-1111-1111-111111111111"))
	w := httptest.NewRecorder()

	h.AdminSetStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing status, got %d", w.Code)
	}

	// Invalid item ID → 400.
	req2 := withUser(withChiParam(
		httptest.NewRequest(http.MethodPatch, "/x", bytes.NewBuffer([]byte(`{"status":"planned"}`))),
		"id", "bogus"))
	w2 := httptest.NewRecorder()

	h.AdminSetStatus(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid id, got %d", w2.Code)
	}
}

func TestAdminCreateItemHandlerMissingTitle(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := withUser(httptest.NewRequest(http.MethodPost, "/x", bytes.NewBuffer([]byte(`{"summary":"s"}`))))
	w := httptest.NewRecorder()

	h.AdminCreateItem(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing title, got %d", w.Code)
	}
}

func TestAdminReorderItemsHandlerValidation(t *testing.T) {
	t.Parallel()

	h := newHandlers()

	// Empty items array → 400.
	req := withUser(httptest.NewRequest(http.MethodPost, "/x", bytes.NewBuffer([]byte(`{"items":[]}`))))
	w := httptest.NewRecorder()

	h.AdminReorderItems(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty items, got %d", w.Code)
	}

	// Bad UUID inside items → 400.
	req2 := withUser(httptest.NewRequest(http.MethodPost, "/x",
		bytes.NewBuffer([]byte(`{"items":[{"id":"nope","sort_order":1}]}`))))
	w2 := httptest.NewRecorder()

	h.AdminReorderItems(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad item id, got %d", w2.Code)
	}
}

func TestAdminListItemsHandlerBadCategoryID(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := withUser(httptest.NewRequest(http.MethodGet, "/x?category_id=not-a-uuid", nil))
	w := httptest.NewRecorder()

	h.AdminListItems(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad category_id, got %d", w.Code)
	}
}

func TestAdminCreateCategoryHandlerMissingName(t *testing.T) {
	t.Parallel()

	h := newHandlers()
	req := withUser(httptest.NewRequest(http.MethodPost, "/x", bytes.NewBuffer([]byte(`{"slug":"x"}`))))
	w := httptest.NewRecorder()

	h.AdminCreateCategory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", w.Code)
	}
}
