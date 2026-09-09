package assessment

import (
	"sort"
	"testing"

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
