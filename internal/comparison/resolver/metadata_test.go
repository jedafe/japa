package resolver

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/visaroutes"
)

func ptr(s string) *string { return &s }

func TestContentResolvedItemMetadata(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")

	row := gen.GetContentByIDRow{
		ID:            id,
		Title:         "Test Content",
		Body:          "body",
		ContentType:   "guide",
		Slug:          "test-content",
		Status:        "published",
		Visibility:    "public",
		GeoSpaceID:    geoID,
		SourceCountry: ptr("NG"),
		TargetCountry: ptr("CA"),
	}

	item := contentItemFromAny(row, []string{"visa", "study"})

	if item.Metadata["geo_space_id"] != geoID.String() {
		t.Fatalf("expected geo_space_id %q, got %v", geoID.String(), item.Metadata["geo_space_id"])
	}

	if item.Metadata["country"] != "CA" {
		t.Fatalf("expected target country CA, got %v", item.Metadata["country"])
	}

	if !slices.Equal(item.Metadata["tags"].([]string), []string{"visa", "study"}) {
		t.Fatalf("unexpected tags: %v", item.Metadata["tags"])
	}

	if item.Metadata["content_type"] != "guide" {
		t.Fatalf("unexpected content_type: %v", item.Metadata["content_type"])
	}
}

func TestSpaceResolvedItemMetadata(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	logoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440002")

	item := makeSpaceResolvedItem("country", id, "Canada", "canada", ptr("A country"), ptr("CA"), "country", logoID, pgtype.UUID{}, 100)

	if item.Metadata["geo_space_id"] != id.String() {
		t.Fatalf("expected geo_space_id %q, got %v", id.String(), item.Metadata["geo_space_id"])
	}

	if item.Metadata["country"] != "CA" {
		t.Fatalf("expected country CA, got %v", item.Metadata["country"])
	}

	if item.Metadata["level"] != "country" {
		t.Fatalf("unexpected level: %v", item.Metadata["level"])
	}

	// Non-country spaces should not set the country key.
	item2 := makeSpaceResolvedItem("city", id, "Toronto", "toronto", nil, ptr("toronto-ca"), "city", pgtype.UUID{}, pgtype.UUID{}, 0)
	if _, ok := item2.Metadata["country"]; ok {
		t.Fatalf("non-country space should not set country metadata")
	}
}

func TestComparisonResolvedItemMetadata(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")

	row := gen.GetContentByIDRow{
		ID:            id,
		Title:         "Comparison",
		Body:          "body",
		ContentType:   "comparison",
		Slug:          "comp",
		Status:        "published",
		Visibility:    "public",
		GeoSpaceID:    geoID,
		TargetCountry: ptr("GB"),
	}

	item := comparisonResolvedItem(row, pgtype.UUID{}, "", "", []string{"universities"})

	if item.Metadata["geo_space_id"] != geoID.String() {
		t.Fatalf("expected geo_space_id %q, got %v", geoID.String(), item.Metadata["geo_space_id"])
	}

	if item.Metadata["country"] != "GB" {
		t.Fatalf("expected country GB, got %v", item.Metadata["country"])
	}

	if !slices.Equal(item.Metadata["tags"].([]string), []string{"universities"}) {
		t.Fatalf("unexpected tags: %v", item.Metadata["tags"])
	}
}

func TestVisaRouteResolvedItemMetadata(t *testing.T) {
	route := visaroutes.RouteDTO{
		ID:                   "550e8400-e29b-41d4-a716-446655440000",
		Name:                 "Work Visa",
		Slug:                 "work-visa",
		DestinationSpaceID:   "550e8400-e29b-41d4-a716-446655440001",
		DestinationSpaceCode: "DE",
		VisaCategory:         "work",
	}

	item := routeToResolvedItem(route)

	if item.Metadata["geo_space_id"] != route.DestinationSpaceID {
		t.Fatalf("expected geo_space_id %q, got %v", route.DestinationSpaceID, item.Metadata["geo_space_id"])
	}

	if item.Metadata["country"] != "DE" {
		t.Fatalf("expected country DE, got %v", item.Metadata["country"])
	}

	if !slices.Equal(item.Metadata["tags"].([]string), []string{"work"}) {
		t.Fatalf("unexpected tags: %v", item.Metadata["tags"])
	}
}

func TestMentorResolvedItemMetadata(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	m := mentorship.MentorSummary{
		UserID:      id,
		Username:    "mentor",
		DisplayName: ptr("Mentor Name"),
		MentorAreas: []string{"career", "visa"},
	}

	item := mentorToResolvedItem(m)

	if !slices.Equal(item.Metadata["tags"].([]string), []string{"career", "visa"}) {
		t.Fatalf("unexpected tags: %v", item.Metadata["tags"])
	}
}

func TestCityResolvedItemMetadata(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")
	name := "Lagos"

	row := gen.ListCOLCitiesRow{
		ID:           id,
		Title:        "Lagos",
		GeoSpaceID:   geoID,
		GeoSpaceName: &name,
		GeoSpaceSlug: ptr("lagos"),
	}

	item := cityRowToResolvedItem(row)

	if item.Metadata["geo_space_id"] != geoID.String() {
		t.Fatalf("expected geo_space_id %q, got %v", geoID.String(), item.Metadata["geo_space_id"])
	}
}

func TestJobListingResolvedItemMetadata(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")

	row := gen.ListContentsByTypeRow{
		ID:            id,
		Title:         "Engineer",
		Body:          "job body",
		ContentType:   "job_listing",
		Slug:          "engineer",
		Status:        "published",
		Visibility:    "public",
		GeoSpaceID:    geoID,
		TargetCountry: ptr("US"),
	}

	item := jobListingFromAny(row, nil)

	if item.Metadata["geo_space_id"] != geoID.String() {
		t.Fatalf("expected geo_space_id %q, got %v", geoID.String(), item.Metadata["geo_space_id"])
	}

	if item.Metadata["country"] != "US" {
		t.Fatalf("expected country US, got %v", item.Metadata["country"])
	}
}

func TestDBResolverMetadataColumns(t *testing.T) {
	id, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	geoID, _ := ParseUUID("550e8400-e29b-41d4-a716-446655440001")

	db := &fakeDBTX{
		queryRowResults: []*fakeRow{{
			values: []any{
				"id-1", "Backend Engineer", "backend-engineer", "Build things",
				geoID, "US",
				[]string{"go", "backend"},
			},
		}},
	}

	r := NewDBResolver(db, DBConfig{
		Entity:           "job",
		Table:            "intel_jobs",
		IDColumn:         "id",
		TitleColumn:      "title",
		SlugColumn:       "",
		DescColumn:       "description",
		TenantColumn:     "the id",
		GeoSpaceIDColumn: "country_space_id",
		CountryColumn:    "country",
		TagsColumn:       "skills",
	})

	item, found, err := r.Resolve(t.Context(), map[string]any{"id": id.String()})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if !found {
		t.Fatal("expected item to be found")
	}

	if item.Metadata["geo_space_id"] != geoID.String() {
		t.Fatalf("expected geo_space_id %q, got %v", geoID.String(), item.Metadata["geo_space_id"])
	}

	if item.Metadata["country"] != "US" {
		t.Fatalf("expected country US, got %v", item.Metadata["country"])
	}

	if !slices.Equal(item.Metadata["tags"].([]string), []string{"go", "backend"}) {
		t.Fatalf("unexpected tags: %v", item.Metadata["tags"])
	}
}
