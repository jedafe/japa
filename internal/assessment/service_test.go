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

func TestPickBestRoadmapContent_PrefersStrongestContextMatch(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: base},
		{Slug: "canada-study-finance", TargetCountry: ptr("CA"), VisaRoute: ptr("student")},
	}
	journey := JourneyContext{
		DestinationCountry: "CA",
		VisaRoute:          "student",
		JourneyStage:       "planning",
	}

	best, score := pickBestRoadmapContent(rows, base, map[string]bool{}, journey)

	assert.Equal(t, "canada-study-finance", best.Slug)
	assert.Equal(t, 6, score, "target match (4) + visa match (2)")
}

func TestPickBestRoadmapContent_KeepsOriginalOnTie(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: "another-planning-finance", JourneyStage: ptr("planning")},
		{Slug: base, JourneyStage: ptr("planning")},
	}
	journey := JourneyContext{JourneyStage: "planning"}

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
		{Slug: "another-planning-finance", JourneyStage: ptr("planning")},
		{Slug: base, JourneyStage: ptr("planning")},
	}
	journey := JourneyContext{JourneyStage: "planning"}

	// base is in used (as it would be at runtime from the seed map), but it is
	// still allowed to compete and win on a tie.
	best, _ := pickBestRoadmapContent(rows, base, map[string]bool{base: true}, journey)
	assert.Equal(t, base, best.Slug, "the original base slug wins ties even if it is in the used set")
}
