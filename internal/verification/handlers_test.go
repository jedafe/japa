package verification

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
)

func withAuthUser(r *http.Request) *http.Request {
	ctx := r.Context()

	var uid pgtype.UUID

	uid.Scan("22222222-2222-2222-2222-222222222222")

	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: uid,
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
	return NewHandlers(&Service{})
}

// ---------------------------------------------------------------------------
// Auth validation
// ---------------------------------------------------------------------------

func TestSubmitRequest_Unauthorized(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	req := httptest.NewRequest(http.MethodPost, "/verification-requests", nil)

	rr := httptest.NewRecorder()
	h.SubmitRequest(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestListMyRequests_Unauthorized(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	req := httptest.NewRequest(http.MethodGet, "/me/verification-requests", nil)

	rr := httptest.NewRecorder()
	h.ListMyRequests(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestReviewRequest_Unauthorized(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Patch("/admin/verification-requests/{id}", h.ReviewRequest)

	req := httptest.NewRequest(http.MethodPatch, "/admin/verification-requests/33333333-3333-3333-3333-333333333333", nil)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Input validation
// ---------------------------------------------------------------------------

func TestSubmitRequest_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	req := httptest.NewRequest(http.MethodPost, "/verification-requests", bytes.NewBufferString("not json"))
	req = withAuthUser(req)

	rr := httptest.NewRecorder()
	h.SubmitRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSubmitRequest_MissingFormSubmission(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	body := writeJSONBody(t, map[string]string{"entity_type": "user"})
	req := httptest.NewRequest(http.MethodPost, "/verification-requests", body)
	req = withAuthUser(req)

	rr := httptest.NewRecorder()
	h.SubmitRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetRequest_InvalidUUID(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Get("/verification-requests/{id}", h.GetRequest)

	req := httptest.NewRequest(http.MethodGet, "/verification-requests/not-a-uuid", nil)
	req = withAuthUser(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWithdrawRequest_InvalidUUID(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Delete("/verification-requests/{id}", h.WithdrawRequest)

	req := httptest.NewRequest(http.MethodDelete, "/verification-requests/not-a-uuid", nil)
	req = withAuthUser(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestReviewRequest_InvalidUUID(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	router := chi.NewRouter()
	router.Patch("/admin/verification-requests/{id}", h.ReviewRequest)

	body := writeJSONBody(t, map[string]string{"status": "approved"})
	req := httptest.NewRequest(http.MethodPatch, "/admin/verification-requests/not-a-uuid", body)
	req = withAuthUser(req)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// Types endpoint (public)
// ---------------------------------------------------------------------------

func TestListTypes(t *testing.T) {
	t.Parallel()

	h := testHandlers()
	req := httptest.NewRequest(http.MethodGet, "/verification-requests/types", nil)

	rr := httptest.NewRecorder()
	h.ListTypes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		Data struct {
			Types []string `json:"types"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Types) != 5 {
		t.Errorf("types len = %d, want 5", len(body.Data.Types))
	}
}

// ---------------------------------------------------------------------------
// Type validation helper
// ---------------------------------------------------------------------------

func TestIsValidType(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"business", "embassy", "notable", "immigration_lawyer", "professional_body"} {
		if !IsValidType(valid) {
			t.Errorf("IsValidType(%q) = false, want true", valid)
		}
	}
	for _, invalid := range []string{"", "admin", "vip", "blue"} {
		if IsValidType(invalid) {
			t.Errorf("IsValidType(%q) = true, want false", invalid)
		}
	}
}
