package organizations

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

func withAuthUser(r *http.Request) *http.Request {
	ctx := r.Context()

	var uid, tid pgtype.UUID

	uid.Scan("22222222-2222-2222-2222-222222222222")
	tid.Scan("11111111-1111-1111-1111-111111111111")

	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID:   uid,
		TenantID: tid,
	})

	return r.WithContext(ctx)
}

func withTenant(r *http.Request) *http.Request {
	ctx := r.Context()

	var tid pgtype.UUID

	tid.Scan("11111111-1111-1111-1111-111111111111")

	ctx = context.WithValue(ctx, tenant.TenantContextKey, &tenant.Tenant{
		ID: tid,
	})

	return r.WithContext(ctx)
}

func writeJSONBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}

	return bytes.NewBuffer(b)
}

func testHandlers() *Handlers {
	return NewHandlers(&Service{}, nil, nil)
}

// ---------------------------------------------------------------------------
// Auth validation
// ---------------------------------------------------------------------------

func TestCreateOrganization_Unauthorized(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	req := httptest.NewRequest(http.MethodPost, "/organizations", nil)

	rr := httptest.NewRecorder()
	h.CreateOrganization(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Input validation
// ---------------------------------------------------------------------------

func TestCreateOrganization_MissingName(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	body := writeJSONBody(t, map[string]string{})

	req := httptest.NewRequest(http.MethodPost, "/organizations", body)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.CreateOrganization(rr, req)

	// Service will fail with "name is required" — returns 500 since the service error is caught generically.
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestCreateOrganization_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBufferString("not json"))
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.CreateOrganization(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUpdateOrganization_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Patch("/organizations/{id}", h.UpdateOrganization)

	req := httptest.NewRequest(http.MethodPatch, "/organizations/not-a-uuid", bytes.NewBufferString("not json"))
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestDeleteOrganization_InvalidUUID(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Delete("/organizations/{id}", h.DeleteOrganization)

	req := httptest.NewRequest(http.MethodDelete, "/organizations/not-a-uuid", nil)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestAddMember_InvalidBody(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Post("/organizations/{id}/members", h.AddMember)

	req := httptest.NewRequest(http.MethodPost, "/organizations/not-a-uuid/members", bytes.NewBufferString("not json"))
	req = withAuthUser(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestInviteMember_MissingEmail(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	body := writeJSONBody(t, map[string]string{
		"role": "member",
	})

	router := chi.NewRouter()
	router.Post("/organizations/{id}/invitations", h.InviteMember)

	req := httptest.NewRequest(http.MethodPost, "/organizations/00000000-0000-0000-0000-000000000000/invitations", body)
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Service will fail with "email is required".
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestClaimOwnership_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	req := httptest.NewRequest(http.MethodPost, "/claim-ownership", bytes.NewBufferString("not json"))
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	h.ClaimOwnership(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestReviewClaim_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Patch("/claim-ownership/{id}", h.ReviewClaim)

	req := httptest.NewRequest(http.MethodPatch, "/claim-ownership/not-a-uuid", bytes.NewBufferString("not json"))
	req = withAuthUser(req)
	req = withTenant(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// AcceptInvitation — unauthorized
// ---------------------------------------------------------------------------

func TestAcceptInvitation_Unauthorized(t *testing.T) {
	t.Parallel()

	h := testHandlers()

	req := httptest.NewRequest(http.MethodPost, "/organizations/invitations/some-token/accept", nil)

	rr := httptest.NewRecorder()
	h.AcceptInvitation(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
