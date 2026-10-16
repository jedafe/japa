package publicapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/visaroutes"
)

// stubSpaceReader implements SpaceReader; only the methods a test needs
// are populated.
type stubSpaceReader struct {
	getBySlugFn    func(ctx context.Context, slug string) (gen.GetPublicSpaceBySlugRow, error)
	getByGeoCodeFn func(ctx context.Context, geoCode string) (gen.GetPublicSpaceByGeoCodeRow, error)
}

func (s stubSpaceReader) GetSpaceTypeBySlug(_ context.Context, _ string) (gen.SpaceType, error) {
	return gen.SpaceType{}, nil
}

func (s stubSpaceReader) ListSpacesByTypePaginated(_ context.Context, _ pgtype.UUID, _, _, _, _, _, _ string, _, _ int32) ([]gen.ListSpacesByTypePaginatedRow, error) {
	return nil, nil
}

func (s stubSpaceReader) CountSpacesByType(_ context.Context, _ pgtype.UUID, _, _, _, _, _ string) (int64, error) {
	return 0, nil
}

func (s stubSpaceReader) GetPublicSpaceBySlug(ctx context.Context, slug string) (gen.GetPublicSpaceBySlugRow, error) {
	return s.getBySlugFn(ctx, slug)
}

func (s stubSpaceReader) GetPublicSpaceByGeoCode(ctx context.Context, geoCode string) (gen.GetPublicSpaceByGeoCodeRow, error) {
	if s.getByGeoCodeFn == nil {
		return gen.GetPublicSpaceByGeoCodeRow{}, pgx.ErrNoRows
	}

	return s.getByGeoCodeFn(ctx, geoCode)
}

func TestGetDestinationBriefUnknownSlug(t *testing.T) {
	svc := NewService(nil)
	svc.SetMoatReaders(&MoatReaders{
		Spaces: stubSpaceReader{getBySlugFn: func(_ context.Context, _ string) (gen.GetPublicSpaceBySlugRow, error) {
			return gen.GetPublicSpaceBySlugRow{}, pgx.ErrNoRows
		}},
	})

	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/public/destinations/nowhere/brief", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "nowhere")
	r = withAPIKey(r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
	h.GetDestinationBrief(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDestinationBriefPartialFailure(t *testing.T) {
	svc := NewService(nil)
	svc.SetMoatReaders(&MoatReaders{
		Spaces: stubSpaceReader{getBySlugFn: func(_ context.Context, _ string) (gen.GetPublicSpaceBySlugRow, error) {
			return gen.GetPublicSpaceBySlugRow{Visibility: "public", Status: "active", Name: "Canada"}, nil
		}},
		VisaRoutes: stubVisaRoutes{listFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, _, _ int32) ([]visaroutes.RouteDTO, int64, error) {
			return nil, 0, errors.New("boom")
		}},
	})

	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/public/destinations/canada/brief?include=visa", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "canada")
	r = withAPIKey(r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx)))
	h.GetDestinationBrief(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), `"visa"`) ||
		!strings.Contains(rec.Body.String(), "sections_unavailable") {
		t.Fatalf("expected visa in sections_unavailable; body: %s", rec.Body.String())
	}
}
