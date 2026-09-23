package publicapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/visaroutes"
)

func withAPIKey(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), apiKeyContextKey,
		&APIKeyContext{UserID: pgtype.UUID{Valid: true}, Scopes: []string{"api.read.visa"}}))
}

func TestListVisaRoutesHandler(t *testing.T) {
	svc := NewService(nil)
	svc.SetMoatReaders(&MoatReaders{
		VisaRoutes: stubVisaRoutes{listFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, _, _ int32) ([]visaroutes.RouteDTO, int64, error) {
			return []visaroutes.RouteDTO{{}, {}}, 2, nil
		}},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.ListVisaRoutes(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/visa-routes", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestListVisaRoutesHandlerNoKey(t *testing.T) {
	h := NewHandlers(NewService(nil))
	rec := httptest.NewRecorder()
	h.ListVisaRoutes(rec, httptest.NewRequest(http.MethodGet, "/public/visa-routes", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
