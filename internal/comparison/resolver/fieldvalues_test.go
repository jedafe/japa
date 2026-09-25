package resolver

import (
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/university_courses"
	"github.com/jedafe/japa/internal/visaroutes"
)

func scoreInput(t *testing.T, item ResolvedItem, key string) any {
	t.Helper()

	si, ok := item.Metadata["score_inputs"].(map[string]any)
	if !ok {
		t.Fatalf("score_inputs missing or wrong type: %v", item.Metadata["score_inputs"])
	}

	return si[key]
}

func TestVisaRouteCFVOverlay(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")

	route := visaroutes.RouteDTO{
		ID:                 id.String(),
		Name:               "Work",
		Slug:               "work",
		DestinationSpaceID: geoID.String(),
		Requirements:       map[string]any{"minimum_funds": 5000.0, "occupations": []any{"nurse"}},
	}

	cfv := map[string]any{
		"minimum_funds":     7500.0, // community value wins per key
		"fee_usd":           1200.0, // fills the absent typed fee
		"processing_months": 6.0,
	}

	item := routeToResolvedItem(route, cfv)

	fv, ok := item.Metadata["field_values"].(map[string]any)
	if !ok {
		t.Fatalf("field_values missing: %v", item.Metadata)
	}

	if fv["minimum_funds"] != 7500.0 {
		t.Fatalf("expected cfv minimum_funds to shadow, got %v", fv["minimum_funds"])
	}

	if fv["fee_usd"] != 1200.0 {
		t.Fatalf("expected cfv fee_usd in field_values, got %v", fv["fee_usd"])
	}

	if got := scoreInput(t, item, "cost"); got != 1200.0 {
		t.Fatalf("expected score_inputs.cost from cfv fee_usd, got %v", got)
	}

	if got := scoreInput(t, item, "minimum_funds"); got != 7500.0 {
		t.Fatalf("expected score_inputs.minimum_funds 7500, got %v", got)
	}

	if got := scoreInput(t, item, "speed"); got != 6.0 {
		t.Fatalf("expected score_inputs.speed from cfv processing_months, got %v", got)
	}
}

func TestVisaRouteTypedFeeStaysAuthoritative(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")

	fee := 900.0
	route := visaroutes.RouteDTO{ID: id.String(), Name: "Work", Slug: "work", TypicalFeeUSD: fee}

	item := routeToResolvedItem(route, map[string]any{"fee_usd": 1200.0})

	if got := scoreInput(t, item, "cost"); got != 900.0 {
		t.Fatalf("expected typed typical_fee_usd to stay authoritative, got %v", got)
	}
}

func TestLanguageTestCFVProjection(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")

	test := languagetesthub.TestDTO{ID: id, Name: "IELTS", Slug: "ielts", ValidityMonths: 24}
	cfv := map[string]any{"fee_usd": 250.0, "result_days": 5.0, "minimum_score": "7.0"}

	item := languageTestToResolvedItem(test, cfv)

	fv, ok := item.Metadata["field_values"].(map[string]any)
	if !ok {
		t.Fatalf("field_values missing: %v", item.Metadata)
	}

	if fv["fee_usd"] != 250.0 {
		t.Fatalf("expected cfv fee_usd projected, got %v", fv["fee_usd"])
	}

	if got := scoreInput(t, item, "cost"); got != 250.0 {
		t.Fatalf("expected score_inputs.cost from cfv fee_usd, got %v", got)
	}

	if got := scoreInput(t, item, "speed"); got != 5.0 {
		t.Fatalf("expected score_inputs.speed from cfv result_days, got %v", got)
	}

	if got := scoreInput(t, item, "score_required"); got != "7.0" {
		t.Fatalf("expected score_inputs.score_required from cfv minimum_score, got %v", got)
	}

	// Typed validity stays authoritative.
	if got := scoreInput(t, item, "validity_months"); got != int32(24) {
		t.Fatalf("expected typed validity_months, got %v", got)
	}
}

func TestCourseCFVProjection(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")

	item := offeringToResolvedItem(university_courses.CourseOffering{
		ID: id, CourseTitle: "Go", Slug: "go", Language: "English",
		UniversityID: id, UniversityName: "Japa University",
	}, map[string]any{"application_fee_usd": 75.0, "intake_months": []any{"Sep"}})

	fv, ok := item.Metadata["field_values"].(map[string]any)
	if !ok {
		t.Fatalf("field_values missing: %v", item.Metadata)
	}

	if fv["application_fee_usd"] != 75.0 {
		t.Fatalf("expected cfv projected, got %v", fv["application_fee_usd"])
	}
}

func TestMentorCFVProjection(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")

	item := mentorToResolvedItem(
		mentorship.MentorSummary{UserID: id, Username: "mentor", MentorAreas: []string{"career"}},
		map[string]any{"session_rate_usd": 40.0},
	)

	fv, ok := item.Metadata["field_values"].(map[string]any)
	if !ok {
		t.Fatalf("field_values missing: %v", item.Metadata)
	}

	if fv["session_rate_usd"] != 40.0 {
		t.Fatalf("expected cfv projected, got %v", fv["session_rate_usd"])
	}
}

func TestHousingGuideCFVOverlay(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")

	guide := housingguide.GuideDTO{
		ID:                 id.String(),
		DestinationSpaceID: geoID.String(),
		AccommodationType:  "flatshare",
		RentRanges:         []byte(`{"rent_monthly":800,"deposit_months":2}`),
	}

	cfv := map[string]any{"rent_monthly": 950.0, "utilities_monthly": 120.0}

	item := housingGuideToResolvedItem(guide, cfv)

	fv, ok := item.Metadata["field_values"].(map[string]any)
	if !ok {
		t.Fatalf("field_values missing: %v", item.Metadata)
	}

	if fv["rent_monthly"] != 950.0 {
		t.Fatalf("expected cfv rent_monthly to shadow, got %v", fv["rent_monthly"])
	}

	if fv["deposit_months"] != 2.0 {
		t.Fatalf("expected stored deposit_months preserved, got %v", fv["deposit_months"])
	}

	if got := scoreInput(t, item, "cost"); got != 950.0 {
		t.Fatalf("expected score_inputs.cost from cfv rent_monthly, got %v", got)
	}
}

func TestCityIntelCostDataFieldValues(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")

	row := gen.ListCOLCitiesRow{
		ID:            id,
		Title:         "Lagos",
		GeoSpaceID:    geoID,
		IntelCostData: []byte(`{"rent_1br_city_centre":1200,"groceries_monthly":300}`),
	}

	// Nil cfv → intel_cost_data still flows to field_values.
	item := cityRowToResolvedItem(row, nil)

	fv, ok := item.Metadata["field_values"].(map[string]any)
	if !ok {
		t.Fatalf("field_values missing: %v", item.Metadata)
	}

	if fv["rent_1br_city_centre"] != 1200.0 {
		t.Fatalf("expected intel cost parsed, got %v", fv["rent_1br_city_centre"])
	}
}
