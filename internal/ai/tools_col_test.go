package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

type stubCOLReader struct {
	city      gen.GetCOLCityByIDRow
	cityByGeo gen.GetCOLByGeoSpaceIDRow
	values    []gen.GetCOLValuesForCitiesRow
	spaceCFV  map[pgtype.UUID]map[string]any
	cityErr   error
	geoErr    error
}

func (s *stubCOLReader) GetCityByID(_ context.Context, _ pgtype.UUID) (gen.GetCOLCityByIDRow, error) {
	if s.cityErr != nil {
		return gen.GetCOLCityByIDRow{}, s.cityErr
	}

	return s.city, nil
}

func (s *stubCOLReader) GetCityByGeoSpaceID(_ context.Context, _ pgtype.UUID) (gen.GetCOLByGeoSpaceIDRow, error) {
	if s.geoErr != nil {
		return gen.GetCOLByGeoSpaceIDRow{}, s.geoErr
	}

	return s.cityByGeo, nil
}

func (s *stubCOLReader) GetValuesForCities(_ context.Context, _ []pgtype.UUID) ([]gen.GetCOLValuesForCitiesRow, error) {
	return s.values, nil
}

func (s *stubCOLReader) ListSpaceFieldValues(_ context.Context, _ []pgtype.UUID) map[pgtype.UUID]map[string]any {
	return s.spaceCFV
}

func TestCOLCityGetHandler_ByID(t *testing.T) {
	t.Parallel()

	spaceID := mustUUID(t, "00000000-0000-0000-0000-000000000002")
	name := "Lagos"
	slug := "lagos"
	geo := &stubCOLReader{
		city: gen.GetCOLCityByIDRow{
			ID:            mustUUID(t, "00000000-0000-0000-0000-000000000001"),
			Title:         "Lagos",
			GeoSpaceID:    spaceID,
			GeoSpaceName:  &name,
			GeoSpaceSlug:  &slug,
			IntelCostData: []byte(`{"rent_1br": 500, "transit_monthly": 40}`),
		},
		values: []gen.GetCOLValuesForCitiesRow{
			{DimensionLabel: "Rent (1BR)", DimensionType: "number", Value: strPtr("500")},
		},
		// §80 overlay: a community edit wins over the intel value per key.
		spaceCFV: map[pgtype.UUID]map[string]any{
			spaceID: {"rent_1br": 550},
		},
	}
	deps := &ToolDeps{COL: geo}

	tr, err := COLCityGetHandler(deps)(t.Context(), json.RawMessage(`{"city_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success: %s", tr.Status, tr.Content)
	}
	if !strings.Contains(tr.Content, `"550"`) && !strings.Contains(tr.Content, `550`) {
		t.Fatalf("expected cfv overlay to override intel_cost_data, got %s", tr.Content)
	}
	if !strings.Contains(tr.Content, "Rent (1BR)") {
		t.Fatalf("expected dimension matrix in result, got %s", tr.Content)
	}
}

func TestCOLCityGetHandler_ByGeoSpace(t *testing.T) {
	t.Parallel()

	geo := &stubCOLReader{
		cityByGeo: gen.GetCOLByGeoSpaceIDRow{Title: "Accra"},
	}
	deps := &ToolDeps{COL: geo}

	tr, err := COLCityGetHandler(deps)(t.Context(), json.RawMessage(`{"geo_space_id":"00000000-0000-0000-0000-000000000002"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess || !strings.Contains(tr.Content, "Accra") {
		t.Fatalf("status = %q, want success with Accra: %s", tr.Status, tr.Content)
	}
}

func TestCOLCityGetHandler_MissingRef(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{COL: &stubCOLReader{}}

	tr, _ := COLCityGetHandler(deps)(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}
