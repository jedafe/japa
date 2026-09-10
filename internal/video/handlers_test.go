//go:build integration

package video

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
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
	"github.com/jedafe/japa/internal/users"
)

func testAuthMiddleware(userID, tenantID pgtype.UUID) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID})
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
				UserID:    userID,
				TenantID:  tenantID,
				SessionID: "test-session",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func setupTestHandler(t *testing.T, resolver Resolver) (*Handlers, *db.DB, pgtype.UUID, pgtype.UUID) {
	t.Helper()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "video-tester"})

	userSvc := users.NewService(database)
	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, resolver)
	svc.SetSettings(newFakeSettings())

	h := NewHandlers(svc)
	h.SetAuthMiddleware(testAuthMiddleware(userID, tenantID))
	h.SetRBAC(nil)

	return h, database, tenantID, userID
}

func TestHandlers_Resolve_RequiresAuth(t *testing.T) {
	t.Parallel()

	h, _, _, _ := setupTestHandler(t, &fakeResolver{})
	h.SetAuthMiddleware(nil)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ"})
	req := httptest.NewRequest(http.MethodPost, "/videos/resolve", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandlers_Resolve_FeatureDisabled(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "video-disabled"})

	userSvc := users.NewService(database)
	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, &fakeResolver{})
	svc.SetSettings(&fakeSettings{bools: map[string]bool{"video.enabled": false}})

	h := NewHandlers(svc)
	h.SetAuthMiddleware(testAuthMiddleware(userID, tenantID))
	h.SetRBAC(nil)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ"})
	req := httptest.NewRequest(http.MethodPost, "/videos/resolve", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when feature disabled, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlers_Resolve_UnsupportedPlatform(t *testing.T) {
	t.Parallel()

	h, _, _, _ := setupTestHandler(t, &fakeResolver{err: ErrUnsupportedPlatform})

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"url": "https://example.com/video"})
	req := httptest.NewRequest(http.MethodPost, "/videos/resolve", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlers_GetByID_RequiresTenant(t *testing.T) {
	t.Parallel()

	h, _, _, _ := setupTestHandler(t, &fakeResolver{})
	h.SetAuthMiddleware(nil)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/videos/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no tenant, got %d", rec.Code)
	}
}
