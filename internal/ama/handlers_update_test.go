package ama

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
)

type fakeAMAPatcher struct {
	updated     AMAThreadDTO
	closeErr    error
	updateErr   error
	lastInput   UpdateAMAInput
	closeCalled bool
}

func (f *fakeAMAPatcher) CloseAMA(ctx context.Context, threadID, callerUserID pgtype.UUID) error {
	f.closeCalled = true
	return f.closeErr
}

func (f *fakeAMAPatcher) UpdateAMA(ctx context.Context, threadID, callerUserID pgtype.UUID, in UpdateAMAInput) (AMAThreadDTO, error) {
	f.lastInput = in
	return f.updated, f.updateErr
}

func newAMAPatchRequest(t *testing.T, body []byte, withUser bool, id string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	if err := (error)(nil); err != nil {
		t.Fatalf("scan tenant id: %v", err)
	}

	ctx := t.Context()
	ctx = context.WithValue(ctx, struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID})

	if withUser {
		userID := pgtype.UUID{}
		if err := userID.Scan(uuid.New().String()); err != nil {
			t.Fatalf("scan user id: %v", err)
		}

		ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{UserID: userID})
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodPatch, "/forum/ama/"+id, bytes.NewReader(body))
	req = req.WithContext(ctx)

	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	return httptest.NewRecorder(), req
}

func TestHandlePatch_Close_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	h.patcher = &fakeAMAPatcher{}

	rr, req := newAMAPatchRequest(t, []byte(`{"closed":true}`), false, uuid.New().String())
	h.HandlePatch(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatch_Close_LegacyEmptyBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	h.patcher = &fakeAMAPatcher{}

	rr, req := newAMAPatchRequest(t, nil, true, uuid.New().String())
	h.HandlePatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for legacy close, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatch_Update_Valid(t *testing.T) {
	t.Parallel()

	want := AMAThreadDTO{
		ID:    uuid.New().String(),
		Title: "Updated AMA title",
		Body:  []byte(`{"type":"doc"}`),
	}

	fake := &fakeAMAPatcher{updated: want}
	h := NewHandlers(nil)
	h.patcher = fake

	body, _ := json.Marshal(map[string]any{
		"title": "Updated AMA title",
		"body":  json.RawMessage(`{"type":"doc"}`),
	})
	rr, req := newAMAPatchRequest(t, body, true, want.ID)
	h.HandlePatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got struct {
		Data AMAThreadDTO `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Data.ID != want.ID {
		t.Fatalf("expected ama id %s, got %s", want.ID, got.Data.ID)
	}

	if got.Data.Title != want.Title {
		t.Fatalf("expected title %q, got %q", want.Title, got.Data.Title)
	}
}

func TestHandlePatch_Update_NotFound(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	h.patcher = &fakeAMAPatcher{updateErr: pgx.ErrNoRows}

	body, _ := json.Marshal(map[string]any{
		"title": "Updated AMA title",
		"body":  json.RawMessage(`{"type":"doc"}`),
	})
	rr, req := newAMAPatchRequest(t, body, true, uuid.New().String())
	h.HandlePatch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatch_Update_Forbidden(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	h.patcher = &fakeAMAPatcher{updateErr: ErrEditForbidden}

	body, _ := json.Marshal(map[string]any{
		"title": "Updated AMA title",
		"body":  json.RawMessage(`{"type":"doc"}`),
	})
	rr, req := newAMAPatchRequest(t, body, true, uuid.New().String())
	h.HandlePatch(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatch_Update_MissingFields(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	h.patcher = &fakeAMAPatcher{}

	rr, req := newAMAPatchRequest(t, []byte(`{"is_anonymous":true}`), true, uuid.New().String())
	h.HandlePatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatch_Update_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil)
	h.patcher = &fakeAMAPatcher{}

	rr, req := newAMAPatchRequest(t, []byte(`{"title"`), true, uuid.New().String())
	req.Body = io.NopCloser(bytes.NewReader([]byte("not json")))
	h.HandlePatch(rr, req)

	if rr.Code != http.StatusOK {
		// Invalid JSON falls through to close (legacy behaviour).
		// With a fake closeErr=nil we get 200.
		t.Fatalf("expected status 200 (legacy close fallback), got %d: %s", rr.Code, rr.Body.String())
	}
}

var _ patcher = (*Service)(nil)
