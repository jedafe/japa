package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
)

type fakeCalendarUpdater struct {
	enabled bool
	entry   CalendarEntryDTO
	err     error
}

func (f *fakeCalendarUpdater) Enabled(ctx context.Context) bool {
	return f.enabled
}

func (f *fakeCalendarUpdater) Update(ctx context.Context, id, callerID pgtype.UUID, input UpdateEntryInput) (CalendarEntryDTO, error) {
	return f.entry, f.err
}

func newCalendarUpdateRequest(t *testing.T, body map[string]any, withUser bool, id string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	var payload []byte

	if body != nil {
		var err error

		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

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

	req := httptest.NewRequest(http.MethodPatch, "/calendar/"+id, bytes.NewReader(payload))
	req = req.WithContext(ctx)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return httptest.NewRecorder(), req
}

func TestHandleUpdate_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeCalendarUpdater{enabled: true}

	body := map[string]any{
		"title":       "Updated title",
		"category":    "visa_window",
		"occurs_at":   time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"source_url":  "https://example.com/source",
		"description": "Updated description",
	}
	rr, req := newCalendarUpdateRequest(t, body, false, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_InvalidID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeCalendarUpdater{enabled: true}

	body := map[string]any{
		"title":       "Updated title",
		"category":    "visa_window",
		"occurs_at":   time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"source_url":  "https://example.com/source",
		"description": "Updated description",
	}
	rr, req := newCalendarUpdateRequest(t, body, true, "not-a-uuid")

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeCalendarUpdater{enabled: true}

	rr, req := newCalendarUpdateRequest(t, nil, true, uuid.New().String())
	req.Body = io.NopCloser(bytes.NewReader([]byte("not json")))

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeCalendarUpdater{enabled: true}

	body := map[string]any{
		"title": "Updated title",
	}
	rr, req := newCalendarUpdateRequest(t, body, true, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_NotFound(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeCalendarUpdater{enabled: true, err: pgx.ErrNoRows}

	body := map[string]any{
		"title":       "Updated title",
		"category":    "visa_window",
		"occurs_at":   time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"source_url":  "https://example.com/source",
		"description": "Updated description",
	}
	rr, req := newCalendarUpdateRequest(t, body, true, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_Forbidden(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeCalendarUpdater{enabled: true, err: ErrEditForbidden}

	body := map[string]any{
		"title":       "Updated title",
		"category":    "visa_window",
		"occurs_at":   time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"source_url":  "https://example.com/source",
		"description": "Updated description",
	}
	rr, req := newCalendarUpdateRequest(t, body, true, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_Valid(t *testing.T) {
	t.Parallel()

	want := CalendarEntryDTO{
		ID:          uuid.New().String(),
		Title:       "Updated title",
		Category:    "visa_window",
		SourceURL:   "https://example.com/source",
		Description: "Updated description",
		Status:      "pending",
	}

	h := NewHandlers(nil, &config.Settings{})
	h.updater = &fakeCalendarUpdater{enabled: true, entry: want}

	body := map[string]any{
		"title":       want.Title,
		"category":    want.Category,
		"occurs_at":   time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"source_url":  want.SourceURL,
		"description": want.Description,
	}
	rr, req := newCalendarUpdateRequest(t, body, true, want.ID)

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got struct {
		Data CalendarEntryDTO `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Data.ID != want.ID {
		t.Fatalf("expected entry id %s, got %s", want.ID, got.Data.ID)
	}

	if got.Data.Title != want.Title {
		t.Fatalf("expected title %q, got %q", want.Title, got.Data.Title)
	}
}

var _ updater = (*Service)(nil)
