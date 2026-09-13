package jobs

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
	"github.com/jedafe/japa/internal/config"
)

type fakeJobUpdater struct {
	enabled bool
	detail  JobDetail
	err     error
}

func (f *fakeJobUpdater) IsEnabled(ctx context.Context, tenantID pgtype.UUID) bool { return f.enabled }

func (f *fakeJobUpdater) UpdateJobListing(ctx context.Context, tenantID, orgID, jobID, callerID pgtype.UUID, input UpdateJobListingInput) (JobDetail, error) {
	return f.detail, f.err
}

func newJobUpdateRequest(t *testing.T, body map[string]any, withUser bool, orgID, jobID string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	var payload []byte

	if body != nil {
		var err error

		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

	tenantID := pgtype.UUID{}
	if err := tenantID.Scan(uuid.New().String()); err != nil {
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
	rctx.URLParams.Add("org_id", orgID)
	rctx.URLParams.Add("id", jobID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodPatch, "/org/"+orgID+"/jobs/"+jobID, bytes.NewReader(payload))
	req = req.WithContext(ctx)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return httptest.NewRecorder(), req
}

func TestHandleUpdateJob_Unauthorized(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeJobUpdater{enabled: true}

	body := map[string]any{
		"title":        "Updated title",
		"body":         "Updated body",
		"field_values": map[string]string{"company_name": "Updated Co"},
	}
	rr, req := newJobUpdateRequest(t, body, false, uuid.New().String(), uuid.New().String())

	h.HandleUpdateJob(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdateJob_InvalidOrgID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeJobUpdater{enabled: true}

	body := map[string]any{
		"title": "Updated title",
		"body":  "Updated body",
	}
	rr, req := newJobUpdateRequest(t, body, true, "not-a-uuid", uuid.New().String())

	h.HandleUpdateJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdateJob_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeJobUpdater{enabled: true}

	rr, req := newJobUpdateRequest(t, nil, true, uuid.New().String(), uuid.New().String())
	req.Body = io.NopCloser(bytes.NewReader([]byte("not json")))

	h.HandleUpdateJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdateJob_NotFound(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeJobUpdater{enabled: true, err: pgx.ErrNoRows}

	body := map[string]any{
		"title": "Updated title",
		"body":  "Updated body",
	}
	rr, req := newJobUpdateRequest(t, body, true, uuid.New().String(), uuid.New().String())

	h.HandleUpdateJob(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdateJob_NotOrgMember(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	h.updater = &fakeJobUpdater{enabled: true, err: ErrNotOrgMember}

	body := map[string]any{
		"title": "Updated title",
		"body":  "Updated body",
	}
	rr, req := newJobUpdateRequest(t, body, true, uuid.New().String(), uuid.New().String())

	h.HandleUpdateJob(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdateJob_Valid(t *testing.T) {
	t.Parallel()

	want := JobDetail{
		ID:      uuid.New().String(),
		Title:   "Updated title",
		Slug:    "updated-title",
		Body:    "Updated body",
		Summary: "Updated body",
		FieldValues: map[string]string{
			"company_name": "Updated Co",
		},
	}

	h := NewHandlers(nil, &config.Settings{})
	h.updater = &fakeJobUpdater{enabled: true, detail: want}

	body := map[string]any{
		"title": "Updated title",
		"body":  "Updated body",
		"field_values": map[string]string{
			"company_name": "Updated Co",
		},
	}
	rr, req := newJobUpdateRequest(t, body, true, uuid.New().String(), want.ID)

	h.HandleUpdateJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got struct {
		Data JobDetail `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Data.ID != want.ID {
		t.Fatalf("expected job id %s, got %s", want.ID, got.Data.ID)
	}

	if got.Data.Title != want.Title {
		t.Fatalf("expected title %q, got %q", want.Title, got.Data.Title)
	}

	if got.Data.Body != want.Body {
		t.Fatalf("expected body %q, got %q", want.Body, got.Data.Body)
	}
}

var _ listingUpdater = (*Service)(nil)
