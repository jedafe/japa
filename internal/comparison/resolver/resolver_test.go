package resolver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/university_courses"
	"github.com/jedafe/japa/internal/visaroutes"
)

type fakeResolver struct {
	resolveFunc func(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error)
	searchFunc  func(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error)
}

func (f *fakeResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	return f.resolveFunc(ctx, ref, tenantID)
}

func (f *fakeResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
	return f.searchFunc(ctx, query, filters, tenantID, limit, offset)
}

func TestRegistryResolve(t *testing.T) {
	reg := NewRegistry()
	reg.Register("test", &fakeResolver{
		resolveFunc: func(_ context.Context, ref map[string]any, _ pgtype.UUID) (ResolvedItem, bool, error) {
			id, _ := ref["id"].(string)
			return ResolvedItem{ID: id, Title: "Found"}, true, nil
		},
	})

	ctx := context.Background()
	item, err := reg.Resolve(ctx, map[string]any{"type": "test", "id": "abc"}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if item.Type != "test" {
		t.Fatalf("expected type overwritten to 'test', got %q", item.Type)
	}
	if item.Title != "Found" || item.ID != "abc" {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestRegistryResolveMissingType(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	_, err := reg.Resolve(ctx, map[string]any{"id": "abc"}, pgtype.UUID{})
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestRegistryResolveUnsupported(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	_, err := reg.Resolve(ctx, map[string]any{"type": "missing", "id": "abc"}, pgtype.UUID{})
	if !errors.Is(err, fmt.Errorf("unsupported item_ref type: missing")) && err.Error() != "unsupported item_ref type: missing" {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestRegistryResolveNotFound(t *testing.T) {
	reg := NewRegistry()
	reg.Register("test", &fakeResolver{
		resolveFunc: func(_ context.Context, _ map[string]any, _ pgtype.UUID) (ResolvedItem, bool, error) {
			return ResolvedItem{}, false, nil
		},
	})

	ctx := context.Background()
	_, err := reg.Resolve(ctx, map[string]any{"type": "test", "id": "abc"}, pgtype.UUID{})
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestRegistrySearch(t *testing.T) {
	reg := NewRegistry()
	reg.Register("test", &fakeResolver{
		searchFunc: func(_ context.Context, query string, _ map[string]any, _ pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
			return []ResolvedItem{{ID: "1", Title: query}}, nil
		},
	})

	ctx := context.Background()
	items, err := reg.Search(ctx, "test", "q", nil, pgtype.UUID{}, 10, 0)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(items) != 1 || items[0].Type != "test" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestParseUUID(t *testing.T) {
	valid := "550e8400-e29b-41d4-a716-446655440000"
	u, err := ParseUUID(valid)
	if err != nil {
		t.Fatalf("valid uuid failed: %v", err)
	}
	if !u.Valid {
		t.Fatal("expected valid uuid")
	}
	if u.String() != valid {
		t.Fatalf("expected %s, got %s", valid, u.String())
	}

	_, err = ParseUUID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid uuid")
	}
}

func TestExcerpt(t *testing.T) {
	cases := []struct {
		input string
		max   int
		want  string
	}{
		{"hello world", 5, "hello…"},
		{"<p>hello world</p>", 20, "hello world"},
		{"{\"a\":\"hello\",\"b\":\"world\"}", 20, "hello world"},
	}

	for _, tc := range cases {
		got := Excerpt(tc.input, tc.max)
		if got != tc.want {
			t.Fatalf("Excerpt(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

func TestCoerceMap(t *testing.T) {
	m, err := CoerceMap([]byte(`{"type":"test","id":"1"}`))
	if err != nil {
		t.Fatalf("CoerceMap failed: %v", err)
	}
	if m["type"] != "test" {
		t.Fatalf("unexpected map: %+v", m)
	}

	m, err = CoerceMap(nil)
	if err != nil || m != nil {
		t.Fatalf("nil input should return nil map: %v, %v", m, err)
	}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(pgx.ErrNoRows) {
		t.Fatal("expected pgx.ErrNoRows to be not found")
	}
	if IsNotFound(errors.New("other")) {
		t.Fatal("expected other errors to not be not found")
	}
}

func TestQuoteIdent(t *testing.T) {
	cases := []struct {
		input string
		ok    bool
	}{
		{"valid_table", true},
		{"also_valid_2", true},
		{"with; injection", false},
		{"", false},
	}

	for _, tc := range cases {
		got, err := quoteIdent(tc.input)
		if tc.ok {
			if err != nil || got != tc.input {
				t.Fatalf("quoteIdent(%q) = %q, %v", tc.input, got, err)
			}
		} else {
			if err == nil {
				t.Fatalf("quoteIdent(%q) should have failed", tc.input)
			}
		}
	}
}

func TestResolveScoreInputs(t *testing.T) {
	id, err := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	geoID, err := ParseUUID("550e8400-e29b-41d4-a716-446655440001")
	if err != nil {
		t.Fatalf("parse geo uuid: %v", err)
	}

	cases := []struct {
		name     string
		item     ResolvedItem
		wantKeys []string
	}{
		{
			name:     "space",
			item:     ResolvedItem{Metadata: spaceMetadata(id, "canada", "country", ptr("CA"), "", 100)},
			wantKeys: []string{"cost", "income", "speed", "pr_path_ease", "safety_index", "healthcare_index", "language", "geo_space_id"},
		},
		{
			name:     "university",
			item:     makeSpaceResolvedItem("university", id, "Uni", "uni", nil, nil, "city", pgtype.UUID{}, pgtype.UUID{}, 0),
			wantKeys: []string{"cost", "income", "acceptance_rate", "duration_years", "qs_ranking", "pr_path_ease", "post_study_work_months", "language", "accreditation", "fields_of_study", "geo_space_id"},
		},
		{
			name: "course",
			item: offeringToResolvedItem(university_courses.CourseOffering{
				ID: id, CourseTitle: "Go", Slug: "go", Language: "English",
				UniversityID: id, UniversityName: "Japa University",
			}),
			wantKeys: []string{"cost", "duration_weeks", "language", "field", "degree_type", "university_id"},
		},
		{
			name:     "visa_route",
			item:     routeToResolvedItem(visaroutes.RouteDTO{ID: id.String(), Name: "Work", Slug: "work", DestinationSpaceID: geoID.String()}),
			wantKeys: []string{"cost", "income", "speed", "pr_path_ease", "minimum_funds", "work_allowed", "pr_eligible", "family_included", "language", "age_min", "age_max", "occupation_eligible", "destination_geo_space_id", "geo_space_id"},
		},
		{
			name:     "col_city",
			item:     cityRowToResolvedItem(gen.GetCOLCityByIDRow{ID: id, Title: "Lagos", GeoSpaceID: geoID}),
			wantKeys: []string{"cost", "rent", "groceries", "transport", "utilities", "income", "language", "safety_index", "healthcare_index", "geo_space_id"},
		},
		{
			name:     "scholarship",
			item:     scholarshipToResolvedItem(scholarships.ScholarshipResult{GuideID: id.String(), Title: "Scholarship", Slug: "scholarship", FieldsOfStudy: []string{"CS"}, Country: "NG", Institution: "Uni", LevelOfStudy: "undergrad"}),
			wantKeys: []string{"cost", "award", "deadline_days", "fields_of_study", "country", "institution", "level", "language"},
		},
		{
			name:     "languagetest",
			item:     languageTestToResolvedItem(languagetesthub.TestDTO{ID: id, Name: "IELTS", Slug: "ielts", ValidityMonths: 24}),
			wantKeys: []string{"cost", "speed", "validity_months", "accepted_by", "score_required", "language"},
		},
		{
			name:     "mentor",
			item:     mentorToResolvedItem(mentorship.MentorSummary{UserID: id, Username: "mentor", MentorAreas: []string{"career"}}),
			wantKeys: []string{"cost", "rating", "sessions_count", "areas", "language", "availability_days"},
		},
		{
			name: "job_listing",
			item: jobListingFromAny(gen.ListContentsByTypeRow{
				ID: id, Title: "Engineer", ContentType: "job_listing", Slug: "engineer",
				Status: "published", Visibility: "public", GeoSpaceID: geoID,
				FieldValues: []byte(`{"salary_max": 120000, "job_type": "full_time", "visa_sponsorship": true, "location_type": "remote"}`),
			}, nil),
			wantKeys: []string{"income", "cost", "speed", "contract_type", "sponsorship", "remote", "required_experience_years", "language", "location_geo_space_id", "geo_space_id"},
		},
		{
			name:     "content",
			item:     contentItemFromAny(gen.GetContentByIDRow{ID: id, Title: "Guide", ContentType: "guide", Slug: "guide", Status: "published", Visibility: "public"}, nil),
			wantKeys: []string{"type_slug", "child_japa_score", "freshness_score"},
		},
		{
			name:     "comparison",
			item:     comparisonResolvedItem(gen.GetContentByIDRow{ID: id, Title: "Comparison", ContentType: "comparison", Slug: "comp", Status: "published", Visibility: "public"}, pgtype.UUID{}, "universities", "", nil),
			wantKeys: []string{"type_slug", "child_japa_score", "freshness_score"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, ok := tc.item.Metadata["score_inputs"]
			if !ok {
				t.Fatalf("metadata missing score_inputs")
			}
			inputs, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("score_inputs is %T, want map[string]any", raw)
			}
			if len(inputs) != len(tc.wantKeys) {
				t.Fatalf("score_inputs has %d keys, want %d: got %+v", len(inputs), len(tc.wantKeys), inputs)
			}
			for _, k := range tc.wantKeys {
				if _, ok := inputs[k]; !ok {
					t.Fatalf("score_inputs missing key %q", k)
				}
			}
		})
	}

	t.Run("organization", func(t *testing.T) {
		r := &DBResolver{cfg: DBConfig{Entity: "organization"}}
		item := r.toItem(&dbRowValues{id: id.String(), title: "Bank", country: "NG"})
		raw, ok := item.Metadata["score_inputs"]
		if !ok {
			t.Fatalf("metadata missing score_inputs")
		}
		inputs, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("score_inputs is %T, want map[string]any", raw)
		}
		want := []string{"cost", "income", "speed", "atm_fee", "min_balance", "countries"}
		if len(inputs) != len(want) {
			t.Fatalf("score_inputs has %d keys, want %d: got %+v", len(inputs), len(want), inputs)
		}
		for _, k := range want {
			if _, ok := inputs[k]; !ok {
				t.Fatalf("score_inputs missing key %q", k)
			}
		}
		if countries, ok := inputs["countries"].([]string); !ok || !slices.Equal(countries, []string{"NG"}) {
			t.Fatalf("expected countries [NG], got %v", inputs["countries"])
		}
	})

	t.Run("bank", func(t *testing.T) {
		r := &DBResolver{cfg: DBConfig{Entity: "bank"}}
		item := r.toItem(&dbRowValues{id: id.String(), title: "Bank", country: "NG"})
		raw, ok := item.Metadata["score_inputs"]
		if !ok {
			t.Fatalf("metadata missing score_inputs")
		}
		inputs, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("score_inputs is %T, want map[string]any", raw)
		}
		want := []string{"cost", "income", "speed", "atm_fee", "min_balance", "countries"}
		if len(inputs) != len(want) {
			t.Fatalf("score_inputs has %d keys, want %d: got %+v", len(inputs), len(want), inputs)
		}
		for _, k := range want {
			if _, ok := inputs[k]; !ok {
				t.Fatalf("score_inputs missing key %q", k)
			}
		}
	})
}
