package assessment

import (
	"sort"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/stretchr/testify/assert"
)

func TestAssessmentDimensions(t *testing.T) {
	form := &AssessmentFormDTO{
		Fields: []AssessmentField{
			{Config: FieldScoringConfig{Dimension: "financial"}},
			{Config: FieldScoringConfig{Dimension: "logistical"}},
			{Config: FieldScoringConfig{Dimension: ""}},
		},
		// Empty CuratedRoadmap falls back to the default library.
		ScoringConfig: ScoringConfig{},
	}

	got := assessmentDimensions(form)
	sort.Strings(got)

	assert.Contains(t, got, "financial")
	assert.Contains(t, got, "logistical")
	assert.Contains(t, got, "language") // from default library
	assert.NotContains(t, got, "")      // empty dimensions are skipped
}

func ptr(s string) *string { return &s }

// TestParseJourneyFieldsVisaRouteFallback verifies the purpose-derived
// visa route used when visa_type goes unanswered — the field only renders
// when purpose is ambiguous (migration 001103), so the determined intents
// must still produce a route for roadmap matching.
func TestParseJourneyFieldsVisaRouteFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		answers map[string]any
		want    string
	}{
		{"explicit visa_type wins", map[string]any{tfVisaType: tfInvestment, tfPurpose: tfWork}, tfInvestment},
		{"purpose study derives student route", map[string]any{tfPurpose: tfStudy}, tfStudent},
		{"purpose work derives work route", map[string]any{tfPurpose: tfWork}, tfWork},
		{"purpose business derives business route", map[string]any{tfPurpose: tfBusiness}, tfBusiness},
		{"purpose tourism derives tourist route", map[string]any{tfPurpose: "tourism"}, "tourist"},
		{"purpose family derives family route", map[string]any{tfPurpose: tfFamily}, tfFamily},
		{"purpose relocation falls back to other", map[string]any{tfPurpose: "relocation"}, "other"},
		{"both blank stays blank", map[string]any{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseJourneyFields(tc.answers)
			assert.Equal(t, tc.want, got.VisaRoute)
		})
	}
}

// TestParseJourneyFieldsPurposeFallback verifies the route-derived purpose
// used when purpose goes unanswered — the field is hidden at
// relocated/settled (migration 001108), so the visa_type those users still
// answer must produce a purpose for roadmap matching.
func TestParseJourneyFieldsPurposeFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		answers map[string]any
		want    string
	}{
		{"explicit purpose wins", map[string]any{tfVisaType: tfStudent, tfPurpose: tfWork}, tfWork},
		{"route student derives study", map[string]any{tfVisaType: tfStudent}, tfStudy},
		{"route work derives work", map[string]any{tfVisaType: tfWork}, tfWork},
		{"route business derives business", map[string]any{tfVisaType: tfBusiness}, tfBusiness},
		{"route tourist derives tourism", map[string]any{tfVisaType: "tourist"}, "tourism"},
		{"route spouse derives family", map[string]any{tfVisaType: "spouse"}, tfFamily},
		{"route investment derives business", map[string]any{tfVisaType: tfInvestment}, tfBusiness},
		{"route transit has no purpose", map[string]any{tfVisaType: "transit"}, ""},
		{"route other has no purpose", map[string]any{tfVisaType: "other"}, ""},
		{"both blank stays blank", map[string]any{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseJourneyFields(tc.answers)
			assert.Equal(t, tc.want, got.Purpose)
		})
	}
}

func TestPickBestRoadmapContent_PrefersStrongestContextMatch(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: base},
		{Slug: "canada-study-finance", TargetCountry: ptr("CA"), VisaRoute: ptr(tfStudent)},
	}
	journey := JourneyContext{
		DestinationCountry: "CA",
		VisaRoute:          tfStudent,
		JourneyStage:       tfPlanning,
	}

	best, score := pickBestRoadmapContent(rows, base, map[string]bool{}, journey)

	assert.Equal(t, "canada-study-finance", best.Slug)
	assert.Equal(t, 6, score, "target match (4) + visa match (2)")
}

func TestPickBestRoadmapContent_KeepsOriginalOnTie(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: "another-planning-finance", JourneyStage: ptr(tfPlanning)},
		{Slug: base, JourneyStage: ptr(tfPlanning)},
	}
	journey := JourneyContext{JourneyStage: tfPlanning}

	best, _ := pickBestRoadmapContent(rows, base, map[string]bool{}, journey)
	assert.Equal(t, base, best.Slug, "when scores tie, the original curated slug wins")
}

func TestPickBestRoadmapContent_SkipsUsedSlugs(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: "used-already", TargetCountry: ptr("CA")},
		{Slug: base, TargetCountry: ptr("CA")},
	}
	journey := JourneyContext{DestinationCountry: "CA"}

	best, _ := pickBestRoadmapContent(rows, base, map[string]bool{"used-already": true}, journey)
	assert.Equal(t, base, best.Slug, "already-used slugs are skipped")
}

func TestPickBestRoadmapContent_BaseWinsTieEvenWhenUsed(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: "another-planning-finance", JourneyStage: ptr(tfPlanning)},
		{Slug: base, JourneyStage: ptr(tfPlanning)},
	}
	journey := JourneyContext{JourneyStage: tfPlanning}

	// base is in used (as it would be at runtime from the seed map), but it is
	// still allowed to compete and win on a tie.
	best, _ := pickBestRoadmapContent(rows, base, map[string]bool{base: true}, journey)
	assert.Equal(t, base, best.Slug, "the original base slug wins ties even if it is in the used set")
}
