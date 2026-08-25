package scoring

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/jce"
)

func TestEngine_NumberFactorHigherIsBetter(t *testing.T) {
	e := &Engine{}
	factor := jce.ScoreFactor{
		FactorKey:  "income",
		DataSource: "resolver_metadata",
		Config: jce.ScoreFactorConfig{
			Kind:      "number",
			Direction: "higher_is_better",
			ValueKey:  "income",
			Min:       0,
			Max:       100,
		},
	}
	item := scoredItem{
		id: "item-1",
		resolved: resolver.ResolvedItem{
			Metadata: map[string]any{
				"score_inputs": map[string]any{"income": 75.0},
			},
		},
	}

	sf := e.computeFactorScore(jce.ComparisonProfile{}, item, factor, typeConfig{}, map[string]float64{"income": 1})
	if sf.score != 75.0 {
		t.Fatalf("expected score 75.0 for higher_is_better, got %v", sf.score)
	}
}

func TestEngine_NumberFactorLowerIsBetter(t *testing.T) {
	e := &Engine{}
	factor := jce.ScoreFactor{
		FactorKey:  "cost",
		DataSource: "resolver_metadata",
		Config: jce.ScoreFactorConfig{
			Kind:      "number",
			Direction: "lower_is_better",
			ValueKey:  "cost",
			Min:       0,
			Max:       100,
		},
	}
	item := scoredItem{
		id: "item-1",
		resolved: resolver.ResolvedItem{
			Metadata: map[string]any{
				"score_inputs": map[string]any{"cost": 25.0},
			},
		},
	}

	sf := e.computeFactorScore(jce.ComparisonProfile{}, item, factor, typeConfig{}, map[string]float64{"cost": 1})
	if sf.score != 75.0 {
		t.Fatalf("expected score 75.0 for lower_is_better, got %v", sf.score)
	}
}

func TestEngine_MissingInputUsesMissingScore(t *testing.T) {
	e := &Engine{}
	factor := jce.ScoreFactor{
		FactorKey:  "cost",
		DataSource: "resolver_metadata",
		Config: jce.ScoreFactorConfig{
			Kind:         "number",
			Direction:    "lower_is_better",
			ValueKey:     "cost",
			MissingScore: 15,
			Min:          0,
			Max:          100,
		},
	}
	item := scoredItem{
		id: "item-1",
		resolved: resolver.ResolvedItem{
			Metadata: map[string]any{
				"score_inputs": map[string]any{},
			},
		},
	}

	sf := e.computeFactorScore(jce.ComparisonProfile{}, item, factor, typeConfig{}, map[string]float64{"cost": 1})
	if sf.score != 15.0 {
		t.Fatalf("expected missing score 15.0, got %v", sf.score)
	}
}

func TestEngine_ProfileMatchExact(t *testing.T) {
	e := &Engine{}
	factor := jce.ScoreFactor{
		FactorKey:  "language_match",
		DataSource: "profile",
		Config: jce.ScoreFactorConfig{
			Kind:          "text",
			MatchStrategy: "exact",
			ProfileKey:    "demographics.language_proficiency",
			ItemKey:       "language",
		},
	}
	profile := jce.ComparisonProfile{
		Demographics: jce.ProfileDemographics{
			LanguageProficiency: "English",
		},
	}
	item := scoredItem{
		id: "item-1",
		resolved: resolver.ResolvedItem{
			Metadata: map[string]any{
				"score_inputs": map[string]any{"language": "English"},
			},
		},
	}

	sf := e.computeFactorScore(profile, item, factor, typeConfig{}, map[string]float64{"language_match": 1})
	if sf.score != 100.0 {
		t.Fatalf("expected exact match score 100.0, got %v", sf.score)
	}
}

func TestEngine_EligibilityRuleIneligible(t *testing.T) {
	e := &Engine{}
	cfg := typeConfig{
		EligibilityRules: []map[string]any{
			{
				"profile_key": "demographics.budget_max",
				"item_key":    "minimum_funds",
				"operator":    "gte",
				"on_fail":     "ineligible",
				"explanation": "required funds exceed your budget",
			},
		},
	}
	factor := jce.ScoreFactor{
		FactorKey:  "eligibility",
		DataSource: "eligibility",
		Config: jce.ScoreFactorConfig{
			Kind: "eligibility",
		},
	}
	budget := 1000.0
	profile := jce.ComparisonProfile{
		Demographics: jce.ProfileDemographics{
			BudgetMax: &budget,
		},
	}
	item := scoredItem{
		id: "item-1",
		resolved: resolver.ResolvedItem{
			Metadata: map[string]any{
				"score_inputs": map[string]any{"minimum_funds": 2000.0},
			},
		},
	}

	sf := e.computeFactorScore(profile, item, factor, cfg, map[string]float64{"eligibility": 1})
	if sf.score != 0.0 {
		t.Fatalf("expected ineligible score 0.0, got %v", sf.score)
	}
	if sf.eligibleStatus != "ineligible" {
		t.Fatalf("expected ineligible status, got %v", sf.eligibleStatus)
	}
}

func TestEngine_SmartModeCostMultiplier(t *testing.T) {
	cfg := typeConfig{
		DefaultWeights: map[string]float64{
			"cost":   0.2,
			"income": 0.3,
		},
		SmartModes: map[string]smartModeConfig{
			"cost": {
				FactorMultipliers: map[string]float64{"cost": 2.0},
			},
		},
	}

	weights := buildWeightMap(cfg, jce.ComparisonProfile{}, "cost")
	if weights["cost"] != 0.4 {
		t.Fatalf("expected cost weight 0.4 after smart-mode multiplier, got %v", weights["cost"])
	}
	if weights["income"] != 0.3 {
		t.Fatalf("expected income weight unchanged at 0.3, got %v", weights["income"])
	}
}

func TestEngine_JapaScoreWeightedAverage(t *testing.T) {
	e := &Engine{}
	cfg := typeConfig{
		DefaultWeights: map[string]float64{
			"cost":   0.4,
			"income": 0.6,
		},
	}
	factors := []jce.ScoreFactor{
		{
			FactorKey:  "cost",
			DataSource: "resolver_metadata",
			Config: jce.ScoreFactorConfig{
				Kind:      "number",
				Direction: "higher_is_better",
				ValueKey:  "cost",
				Min:       0,
				Max:       100,
			},
		},
		{
			FactorKey:  "income",
			DataSource: "resolver_metadata",
			Config: jce.ScoreFactorConfig{
				Kind:      "number",
				Direction: "higher_is_better",
				ValueKey:  "income",
				Min:       0,
				Max:       100,
			},
		},
		{
			FactorKey:  "japa_score",
			DataSource: "aggregate",
			Config: jce.ScoreFactorConfig{
				IsOutput: true,
				Max:      100,
			},
		},
	}

	items := []scoredItem{
		{
			id: "item-1",
			resolved: resolver.ResolvedItem{
				Metadata: map[string]any{
					"score_inputs": map[string]any{
						"cost":   80.0,
						"income": 60.0,
					},
				},
			},
		},
		{
			id: "item-2",
			resolved: resolver.ResolvedItem{
				Metadata: map[string]any{
					"score_inputs": map[string]any{
						"cost":   50.0,
						"income": 90.0,
					},
				},
			},
		},
	}

	res, err := e.scoreRun(context.Background(), pgtype.UUID{}, cfg, factors, jce.ComparisonProfile{}, items, comparison.SmartModeNone)
	if err != nil {
		t.Fatalf("scoreRun failed: %v", err)
	}

	if len(res.scoreResult.Scores) != 2 {
		t.Fatalf("expected 2 JapaScores, got %d", len(res.scoreResult.Scores))
	}

	score1 := res.scoreResult.Scores[0].JapaScore
	score2 := res.scoreResult.Scores[1].JapaScore

	expected1 := 80.0*0.4 + 60.0*0.6
	expected2 := 50.0*0.4 + 90.0*0.6

	if diff := score1 - expected1; diff < -0.001 || diff > 0.001 {
		t.Fatalf("expected item-1 JapaScore %.1f, got %v", expected1, score1)
	}
	if diff := score2 - expected2; diff < -0.001 || diff > 0.001 {
		t.Fatalf("expected item-2 JapaScore %.1f, got %v", expected2, score2)
	}
}

func TestEngine_RecommendationTemplateScoreLt(t *testing.T) {
	templates := []map[string]any{
		{
			"factor_key": "cost",
			"condition":  "score_lt",
			"threshold":  50.0,
			"text":       "Review your budget before applying.",
			"priority":   20,
		},
	}

	recs := generateRecommendations("item-1", "cost-id", "cost", 40.0, "eligible", templates)
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation for score 40, got %d", len(recs))
	}
	if recs[0].Text != "Review your budget before applying." {
		t.Fatalf("unexpected recommendation text: %v", recs[0].Text)
	}
	if recs[0].Priority != 20 {
		t.Fatalf("expected priority 20, got %d", recs[0].Priority)
	}
	if recs[0].FactorID == nil || *recs[0].FactorID != "cost-id" {
		t.Fatalf("expected factor id cost-id, got %v", recs[0].FactorID)
	}

	recs = generateRecommendations("item-1", "cost-id", "cost", 60.0, "eligible", templates)
	if len(recs) != 0 {
		t.Fatalf("expected 0 recommendations for score 60, got %d", len(recs))
	}
}

func TestEngine_ProfileMatchExactIsNotSubstring(t *testing.T) {
	// Exact must not award points for a substring match.
	if got := matchExact("English", "Eng"); got != 0 {
		t.Fatalf("expected 0 for substring exact match, got %v", got)
	}
	if got := matchExact("  English  ", "english"); got != 100 {
		t.Fatalf("expected 100 for case-insensitive trimmed exact match, got %v", got)
	}

	// Array membership should still be exact.
	spaces := []any{"space-1", "space-2"}
	if got := matchExact(spaces, "space-2"); got != 100 {
		t.Fatalf("expected 100 for exact array membership, got %v", got)
	}
	if got := matchExact(spaces, "space-3"); got != 0 {
		t.Fatalf("expected 0 for missing array member, got %v", got)
	}
}

func TestEngine_MatchExactStringArrayVsAnyArray(t *testing.T) {
	// Profile target_space_ids is a []string; item geo_space_id is a []any.
	targetSpaces := []string{"space-1", "space-2"}
	itemSpaces := []any{"space-2", "space-3"}

	if got := matchExact(targetSpaces, itemSpaces); got != 100 {
		t.Fatalf("expected 100 for overlapping []string/[]any, got %v", got)
	}
	if got := matchExact(itemSpaces, targetSpaces); got != 100 {
		t.Fatalf("expected 100 for symmetric []any/[]string, got %v", got)
	}

	missing := []any{"space-3", "space-4"}
	if got := matchExact(targetSpaces, missing); got != 0 {
		t.Fatalf("expected 0 for non-overlapping []string/[]any, got %v", got)
	}

	// Scalar membership in either direction should still work.
	if got := matchExact(targetSpaces, "space-1"); got != 100 {
		t.Fatalf("expected 100 for scalar in []string, got %v", got)
	}
	if got := matchExact("space-1", targetSpaces); got != 100 {
		t.Fatalf("expected 100 for scalar in []string (reversed), got %v", got)
	}
}

func TestEngine_CanScoreContent(t *testing.T) {
	e := &Engine{}
	authorID := pgtype.UUID{}
	_ = authorID.Scan("11111111-1111-1111-1111-111111111111")
	otherID := "22222222-2222-2222-2222-222222222222"
	authorIDStr := authorID.String()

	recalc := func(perm string) bool { return perm == "jce.score.recalculate" }
	view := func(perm string) bool { return perm == "jce.score.view" }
	editAny := func(perm string) bool { return perm == "comparison.edit_any" }
	denyAll := func(perm string) bool { return false }

	tests := []struct {
		name       string
		visibility string
		userID     *string
		checker    func(string) bool
		perm       string
		want       bool
	}{
		{"nil checker denies all", content.VisibilityPublic, nil, nil, "jce.score.recalculate", false},
		{"public anonymous with view", content.VisibilityPublic, nil, view, "jce.score.view", false},
		{"public other with recalc", content.VisibilityPublic, &otherID, recalc, "jce.score.recalculate", true},
		{"public other with view for recalc", content.VisibilityPublic, &otherID, view, "jce.score.recalculate", false},
		{"public other no perms", content.VisibilityPublic, &otherID, denyAll, "jce.score.recalculate", false},
		{"members authenticated", content.VisibilityMembers, &otherID, denyAll, "jce.score.view", true},
		{"members anonymous", content.VisibilityMembers, nil, view, "jce.score.view", false},
		{"private author", content.VisibilityPrivate, &authorIDStr, denyAll, "jce.score.view", true},
		{"private other", content.VisibilityPrivate, &otherID, denyAll, "jce.score.view", false},
		{"private other edit_any", content.VisibilityPrivate, &otherID, editAny, "jce.score.view", true},
		{"public author with deny checker", content.VisibilityPublic, &authorIDStr, denyAll, "jce.score.recalculate", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmp := gen.GetComparisonBySlugOrIDRow{Visibility: tc.visibility, AuthorID: authorID}
			if got := e.canScoreContent(context.Background(), cmp, tc.userID, tc.checker, tc.perm); got != tc.want {
				t.Errorf("visibility=%q user=%v: got %v, want %v", tc.visibility, tc.userID, got, tc.want)
			}
		})
	}
}

func TestEngine_BestFitRecommendationText(t *testing.T) {
	if got := bestFitRecommendationText(nil); got != "This is your best-fit option." {
		t.Fatalf("unexpected default best-fit text: %q", got)
	}

	templates := []map[string]any{
		{"factor_key": "japa_score", "condition": "top", "text": "Top pick for you!"},
	}
	if got := bestFitRecommendationText(templates); got != "Top pick for you!" {
		t.Fatalf("unexpected best-fit text: %q", got)
	}
}
