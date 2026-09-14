package scamalert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
)

type fakeScamUpdater struct {
	enabled bool
	report  ScamReportDTO
	err     error
}

func (f *fakeScamUpdater) Enabled(ctx context.Context, _ pgtype.UUID) bool { return f.enabled }

func (f *fakeScamUpdater) Update(ctx context.Context, id, callerID pgtype.UUID, input UpdateReportInput) (ScamReportDTO, error) {
	return f.report, f.err
}

func newScamUpdateRequest(t *testing.T, body map[string]any, withUser bool, id string) (*httptest.ResponseRecorder, *http.Request) {
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

	req := httptest.NewRequest(http.MethodPatch, "/scam-alerts/"+id, bytes.NewReader(payload))
	req = req.WithContext(ctx)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return httptest.NewRecorder(), req
}

func TestHandleUpdate_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeScamUpdater{enabled: true}

	body := map[string]any{
		"scam_type":            "fake_job",
		"destination_space_id": uuid.New().String(),
		"description":          "Updated description",
	}
	rr, req := newScamUpdateRequest(t, body, false, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_InvalidID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeScamUpdater{enabled: true}

	body := map[string]any{
		"scam_type":            "fake_job",
		"destination_space_id": uuid.New().String(),
		"description":          "Updated description",
	}
	rr, req := newScamUpdateRequest(t, body, true, "not-a-uuid")

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeScamUpdater{enabled: true}

	rr, req := newScamUpdateRequest(t, nil, true, uuid.New().String())
	req.Body = io.NopCloser(bytes.NewReader([]byte("not json")))

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeScamUpdater{enabled: true}

	body := map[string]any{
		"scam_type": "fake_job",
	}
	rr, req := newScamUpdateRequest(t, body, true, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_NotFound(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeScamUpdater{enabled: true, err: pgx.ErrNoRows}

	body := map[string]any{
		"scam_type":            "fake_job",
		"destination_space_id": uuid.New().String(),
		"description":          "Updated description",
	}
	rr, req := newScamUpdateRequest(t, body, true, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_Forbidden(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeScamUpdater{enabled: true, err: ErrEditForbidden}

	body := map[string]any{
		"scam_type":            "fake_job",
		"destination_space_id": uuid.New().String(),
		"description":          "Updated description",
	}
	rr, req := newScamUpdateRequest(t, body, true, uuid.New().String())

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdate_Valid(t *testing.T) {
	t.Parallel()

	want := ScamReportDTO{
		ID:                    uuid.New().String(),
		ScamType:              "fake_job",
		DestinationSpaceID:    uuid.New().String(),
		Description:           "Updated description",
		ReportedToAuthorities: true,
		Status:                "under_review",
	}

	h := NewHandlers(nil, &config.Settings{})
	h.updater = &fakeScamUpdater{enabled: true, report: want}

	body := map[string]any{
		"scam_type":               "fake_job",
		"destination_space_id":    want.DestinationSpaceID,
		"description":             want.Description,
		"reported_to_authorities": true,
	}
	rr, req := newScamUpdateRequest(t, body, true, want.ID)

	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got struct {
		Data ScamReportDTO `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Data.ID != want.ID {
		t.Fatalf("expected report id %s, got %s", want.ID, got.Data.ID)
	}

	if got.Data.Description != want.Description {
		t.Fatalf("expected description %q, got %q", want.Description, got.Data.Description)
	}
}

// Compile-time guard: *Service implements updater.
var _ updater = (*Service)(nil)

// Ensure err identity for tests.
var _ = errors.New
