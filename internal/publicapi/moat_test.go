package publicapi

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/visaroutes"
)

type stubVisaRoutes struct {
	listFn func(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) ([]visaroutes.RouteDTO, int64, error)
}

func (s stubVisaRoutes) List(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) ([]visaroutes.RouteDTO, int64, error) {
	return s.listFn(ctx, dest, prof, edu, yrs, page, limit)
}

func (s stubVisaRoutes) Compare(_ context.Context, _ []pgtype.UUID) ([]visaroutes.RouteDTO, error) {
	return nil, nil
}

func (s stubVisaRoutes) GetByID(_ context.Context, _ pgtype.UUID) (visaroutes.RouteDTO, error) {
	return visaroutes.RouteDTO{}, nil
}
func (s stubVisaRoutes) Enabled(_ context.Context) bool { return true }

func TestServiceListVisaRoutesDelegates(t *testing.T) {
	svc := NewService(nil)
	called := false

	svc.SetMoatReaders(&MoatReaders{
		VisaRoutes: stubVisaRoutes{listFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, page, limit int32) ([]visaroutes.RouteDTO, int64, error) {
			called = true
			if page != 2 || limit != 20 {
				t.Fatalf("page/limit = %d/%d, want 2/20", page, limit)
			}
			return []visaroutes.RouteDTO{{}}, 1, nil
		}},
	})

	routes, total, err := svc.ListVisaRoutes(t.Context(), pgtype.UUID{}, "", "", 0, 2, 20)
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	if !called || total != 1 || len(routes) != 1 {
		t.Fatalf("delegate not invoked correctly")
	}
}

func TestServiceListVisaRoutesUnconfigured(t *testing.T) {
	svc := NewService(nil)

	_, _, err := svc.ListVisaRoutes(t.Context(), pgtype.UUID{}, "", "", 0, 1, 20)
	if err == nil {
		t.Fatal("expected error when readers not configured")
	}
}
