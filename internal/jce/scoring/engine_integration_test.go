//go:build integration

package scoring

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/jce"
	"github.com/jedafe/japa/internal/testdb"
)

// TestMain changes the working directory to the repo root so the shared
// integration-test harness can find the migrations directory. go test sets
// the working directory to the package under test, which is too deep for
// the harness's default relative search.
func TestMain(m *testing.M) {
	cwd, err := os.Getwd()
	if err == nil {
		for d := cwd; d != "/" && d != "."; d = filepath.Dir(d) {
			if info, err := os.Stat(filepath.Join(d, "migrations")); err == nil && info.IsDir() {
				_ = os.Chdir(d)
				break
			}
		}
	}
	os.Exit(m.Run())
}

// stubResolver resolves item_refs of type "test" for integration tests.
type stubResolver struct {
	inputs map[string]map[string]any
}

func (s *stubResolver) Resolve(_ context.Context, ref map[string]any, _ pgtype.UUID) (resolver.ResolvedItem, bool, error) {
	typ, ok := ref["type"].(string)
	if !ok || typ != "test" {
		return resolver.ResolvedItem{}, false, nil
	}
	id, _ := ref["id"].(string)
	inputs, ok := s.inputs[id]
	if !ok {
		inputs = map[string]any{}
	}
	return resolver.ResolvedItem{
		Type:     "test",
		ID:       id,
		Title:    "Item " + id,
		Metadata: map[string]any{"score_inputs": inputs},
	}, true, nil
}

func (s *stubResolver) Search(_ context.Context, _ string, _ map[string]any, _ pgtype.UUID, _ int32) ([]resolver.ResolvedItem, error) {
	return nil, nil
}

func newTestUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(uuid.New().String()); err != nil {
		t.Fatalf("newTestUUID: %v", err)
	}
	return u
}

func seedComparisonType(t *testing.T, database *db.DB, tenantID pgtype.UUID, cfg map[string]any) pgtype.UUID {
	t.Helper()
	ctx := t.Context()

	fieldGroupID := newTestUUID(t)
	_, err := database.Writer.Exec(ctx, `INSERT INTO custom_field_groups (id, name, slug, description, is_system) VALUES ($1, $2, 'Test Fields', $3, 'Test field group', false)`, fieldGroupID, tenantID, "test-fields-"+uuid.New().String())
	if err != nil {
		t.Fatalf("seed comparison type: insert custom_field_groups: %v", err)
	}

	typeID := newTestUUID(t)
	slug := "test-type-" + uuid.New().String()

	var cfgBytes []byte
	if cfg != nil {
		cfgBytes, err = json.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal type config: %v", err)
		}
	} else {
		cfgBytes = []byte("{}")
	}

	_, err = database.Writer.Exec(ctx, `INSERT INTO comparison_type_definitions (id, name, slug, field_group_id, item_label, is_system, config) VALUES ($1, $2, 'Test Type', $3, $4, 'Item', false, $5)`, typeID, tenantID, slug, fieldGroupID, cfgBytes)
	if err != nil {
		t.Fatalf("seed comparison type: insert comparison_type_definitions: %v", err)
	}

	return typeID
}

func seedScoreFactor(t *testing.T, database *db.DB, tenantID, typeID pgtype.UUID, factorKey, dataSource string, defaultWeight float64, cfg jce.ScoreFactorConfig) pgtype.UUID {
	t.Helper()
	ctx := t.Context()

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal score factor config: %v", err)
	}

	id := newTestUUID(t)
	var typeIDArg any
	if typeID.Valid {
		typeIDArg = typeID
	}

	_, err = database.Writer.Exec(ctx, `INSERT INTO comparison_score_factors (
		    id, comparison_type_id, factor_key, label, description,
		    data_source, default_weight, config, is_system
		) VALUES ($1, $2, $3, $4, $5, 'test factor', $6, $7, $8, false)`, id, tenantID, typeIDArg, factorKey, factorKey, dataSource, defaultWeight, cfgBytes)
	if err != nil {
		t.Fatalf("seed score factor %s: %v", factorKey, err)
	}

	return id
}

func seedComparison(t *testing.T, database *db.DB, tenantID, authorID, typeID pgtype.UUID, visibility string) (pgtype.UUID, []pgtype.UUID) {
	t.Helper()
	ctx := t.Context()

	contentID := newTestUUID(t)
	slug := "test-comparison-" + uuid.New().String()

	_, err := database.Writer.Exec(ctx, `INSERT INTO contents (id, author_id, content_type, title, slug, body, body_html, status, visibility) VALUES ($1, $2, $3, 'comparison', 'Test Comparison', $4, '', '', 'published', $5)`, contentID, tenantID, authorID, slug, visibility)
	if err != nil {
		t.Fatalf("seed comparison: insert contents: %v", err)
	}

	_, err = database.Writer.Exec(ctx, `INSERT INTO comparison_content_metadata (content_id, depth, score_version, config) VALUES ($1, $2, 'quick', 1, '{}')`, contentID, tenantID)
	if err != nil {
		t.Fatalf("seed comparison: insert comparison_content_metadata: %v", err)
	}

	items := []struct {
		id    pgtype.UUID
		ref   string
		title string
	}{
		{newTestUUID(t), `{"type":"test","id":"a"}`, "Item A"},
		{newTestUUID(t), `{"type":"test","id":"b"}`, "Item B"},
	}

	var itemIDs []pgtype.UUID
	for i, it := range items {
		_, err := database.Writer.Exec(ctx, `INSERT INTO comparison_items (id, content_id, type_id, title, position, item_ref) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`, it.id, contentID, tenantID, typeID, it.title, i, it.ref)
		if err != nil {
			t.Fatalf("seed comparison: insert comparison_items: %v", err)
		}
		itemIDs = append(itemIDs, it.id)
	}

	return contentID, itemIDs
}

func seedComparisonProfile(t *testing.T, database *db.DB, tenantID, userID pgtype.UUID, in jce.ComparisonProfileInput) pgtype.UUID {
	t.Helper()
	ctx := t.Context()

	profileID := newTestUUID(t)
	demographics, err := json.Marshal(in.Demographics)
	if err != nil {
		t.Fatalf("marshal demographics: %v", err)
	}
	preferences, err := json.Marshal(in.Preferences)
	if err != nil {
		t.Fatalf("marshal preferences: %v", err)
	}
	goals, err := json.Marshal(in.Goals)
	if err != nil {
		t.Fatalf("marshal goals: %v", err)
	}

	targetSpaceIDs := make([]pgtype.UUID, 0, len(in.TargetSpaceIDs))
	for _, s := range in.TargetSpaceIDs {
		var u pgtype.UUID
		_ = u.Scan(s)
		targetSpaceIDs = append(targetSpaceIDs, u)
	}

	_, err = database.Writer.Exec(ctx, `INSERT INTO comparison_profiles (
		    id, user_id, title, target_space_ids, goals, preferences, demographics, is_default, is_template
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, false)`, profileID, tenantID, userID, in.Title, targetSpaceIDs, goals, preferences, demographics)
	if err != nil {
		t.Fatalf("seed comparison profile: %v", err)
	}

	return profileID
}

func newTestEngine(t *testing.T, database *db.DB, r *resolver.Registry) *Engine {
	t.Helper()
	e := NewEngine(database, nil, r, nil, nil)
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	return e
}

func TestEngine_NewEngine(t *testing.T) {
	t.Parallel()
	database := testdb.Connect(t)
	e := newTestEngine(t, database, nil)
	if e.read == nil || e.write == nil || e.database == nil {
		t.Fatal("NewEngine did not initialize database queries")
	}
}

func TestEngine_PreviewScoreForItems(t *testing.T) {
	t.Parallel()
	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "preview-user"}).String()

	checker := func(perm string) bool {
		return perm == "jce.score.view" || perm == "comparison.edit_any"
	}

	typeID := seedComparisonType(t, database, tenantID, map[string]any{
		"scoring_enabled": true,
		"default_weights": map[string]any{"cost": 0.5, "income": 0.5},
	})
	seedScoreFactor(t, database, tenantID, typeID, "cost", "resolver_metadata", 0.5, jce.ScoreFactorConfig{
		Kind:      "number",
		Direction: "lower_is_better",
		ValueKey:  "cost",
		Min:       0,
		Max:       100,
	})
	seedScoreFactor(t, database, tenantID, typeID, "income", "resolver_metadata", 0.5, jce.ScoreFactorConfig{
		Kind:      "number",
		Direction: "higher_is_better",
		ValueKey:  "income",
		Min:       0,
		Max:       100,
	})

	r := resolver.NewRegistry()
	r.Register("test", &stubResolver{inputs: map[string]map[string]any{
		"a": {"cost": 25.0, "income": 80.0},
		"b": {"cost": 75.0, "income": 60.0},
	}})

	e := newTestEngine(t, database, r)
	ctx := context.Background()

	result, err := e.PreviewScoreForItems(ctx, jce.ScorePreviewInput{

		UserID:            &userID,
		TypeID:            typeID.String(),
		ItemRefs:          []map[string]any{{"type": "test", "id": "a"}, {"type": "test", "id": "b"}},
		SmartMode:         comparison.SmartModeNone,
		PermissionChecker: checker,
	})
	if err != nil {
		t.Fatalf("PreviewScoreForItems: %v", err)
	}

	if result.ContentID != "" {
		t.Fatalf("preview ContentID = %q, want empty", result.ContentID)
	}
	if result.Version != 0 {
		t.Fatalf("preview Version = %d, want 0", result.Version)
	}
	if len(result.Scores) != 2 {
		t.Fatalf("preview Scores length = %d, want 2", len(result.Scores))
	}

	// Cost 25 -> score 75; income 80 -> score 80. Weighted average = 77.5.
	if result.Scores[0].JapaScore < 77.0 || result.Scores[0].JapaScore > 78.0 {
		t.Fatalf("unexpected JapaScore for item a: %v", result.Scores[0].JapaScore)
	}
}

func TestEngine_ScoreAndGetScore(t *testing.T) {
	t.Parallel()
	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	authorID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "score-author"})

	typeID := seedComparisonType(t, database, tenantID, map[string]any{
		"scoring_enabled": true,
		"default_weights": map[string]any{"cost": 0.5, "income": 0.5},
		"recommendation_templates": []map[string]any{
			{"factor_key": "cost", "condition": "score_lt", "threshold": 50.0, "text": "Budget check needed.", "priority": 20},
		},
	})
	seedScoreFactor(t, database, tenantID, typeID, "cost", "resolver_metadata", 0.5, jce.ScoreFactorConfig{
		Kind:      "number",
		Direction: "lower_is_better",
		ValueKey:  "cost",
		Min:       0,
		Max:       100,
	})
	seedScoreFactor(t, database, tenantID, typeID, "income", "resolver_metadata", 0.5, jce.ScoreFactorConfig{
		Kind:      "number",
		Direction: "higher_is_better",
		ValueKey:  "income",
		Min:       0,
		Max:       100,
	})

	contentID, _ := seedComparison(t, database, tenantID, authorID, typeID, "public")

	r := resolver.NewRegistry()
	r.Register("test", &stubResolver{inputs: map[string]map[string]any{
		"a": {"cost": 25.0, "income": 80.0},
		"b": {"cost": 75.0, "income": 60.0},
	}})

	e := newTestEngine(t, database, r)
	ctx := context.Background()
	authorIDStr := authorID.String()
	contentIDStr := contentID.String()

	checker := func(perm string) bool {
		return perm == "jce.score.recalculate" || perm == "jce.score.view" || perm == "comparison.edit_any"
	}

	result, err := e.Score(ctx, jce.ScoreInput{
		ContentID: contentIDStr,

		UserID:            &authorIDStr,
		PermissionChecker: checker,
	})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if result.ContentID != contentIDStr {
		t.Fatalf("Score ContentID = %q, want %q", result.ContentID, contentIDStr)
	}
	if result.Version != 2 {
		t.Fatalf("Score Version = %d, want 2", result.Version)
	}
	if len(result.Scores) != 2 {
		t.Fatalf("Score Scores length = %d, want 2", len(result.Scores))
	}

	// The low-cost item should get the best-fit recommendation plus a cost template rec.
	if len(result.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
	hasFactorID := false
	for _, rec := range result.Recommendations {
		if rec.FactorID != nil && *rec.FactorID != "" {
			hasFactorID = true
			break
		}
	}
	if !hasFactorID {
		t.Fatal("expected at least one recommendation with a non-nil FactorID")
	}

	// GetScore should return the persisted result.
	got, err := e.GetScore(ctx, jce.GetScoreInput{
		ContentID: contentIDStr,

		UserID:            &authorIDStr,
		PermissionChecker: checker,
	})
	if err != nil {
		t.Fatalf("GetScore: %v", err)
	}
	if got.Version != result.Version {
		t.Fatalf("GetScore Version = %d, want %d", got.Version, result.Version)
	}
	if len(got.Scores) != len(result.Scores) {
		t.Fatalf("GetScore Scores length = %d, want %d", len(got.Scores), len(result.Scores))
	}

	// Scoring disabled for the type should block Score.
	_, err = database.Writer.Exec(ctx, `UPDATE comparison_type_definitions SET config = $1 WHERE id = $2`, []byte(`{"scoring_enabled": false}`), typeID)
	if err != nil {
		t.Fatalf("disable scoring: %v", err)
	}

	contentID2, _ := seedComparison(t, database, tenantID, authorID, typeID, "public")
	_, err = e.Score(ctx, jce.ScoreInput{
		ContentID: contentID2.String(),

		UserID:            &authorIDStr,
		PermissionChecker: checker,
	})
	if !errors.Is(err, errpkg.ErrScoringDisabled) {
		t.Fatalf("Score disabled type: got %v, want ErrScoringDisabled", err)
	}
}

func TestEngine_ScoreFilterIneligible(t *testing.T) {
	t.Parallel()
	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	authorID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "filter-author"})

	typeID := seedComparisonType(t, database, tenantID, map[string]any{
		"scoring_enabled": true,
		"default_weights": map[string]any{"eligibility": 1.0},
		"eligibility_rules": []map[string]any{
			{"profile_key": "demographics.budget_max", "item_key": "minimum_funds", "operator": "gte", "on_fail": "ineligible"},
		},
		"smart_modes": map[string]any{
			"eligibility": map[string]any{"filter_ineligible": true},
		},
	})
	seedScoreFactor(t, database, tenantID, typeID, "eligibility", "eligibility", 1.0, jce.ScoreFactorConfig{Kind: "eligibility"})

	contentID, _ := seedComparison(t, database, tenantID, authorID, typeID, "public")

	r := resolver.NewRegistry()
	r.Register("test", &stubResolver{inputs: map[string]map[string]any{
		"a": {"minimum_funds": 500.0},
		"b": {"minimum_funds": 2000.0},
	}})

	e := newTestEngine(t, database, r)
	ctx := context.Background()
	authorIDStr := authorID.String()

	checker := func(perm string) bool {
		return perm == "jce.score.recalculate" || perm == "jce.score.view" || perm == "comparison.edit_any"
	}

	budget := 1000.0
	profileID := seedComparisonProfile(t, database, tenantID, authorID, jce.ComparisonProfileInput{
		Title:        "Filter profile",
		Demographics: jce.ProfileDemographics{BudgetMax: &budget},
	})
	profileIDStr := profileID.String()

	result, err := e.Score(ctx, jce.ScoreInput{
		ContentID: contentID.String(),

		UserID:            &authorIDStr,
		SmartMode:         "eligibility",
		ProfileID:         &profileIDStr,
		PermissionChecker: checker,
	})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	// With filter_ineligible true, only the affordable item (a) should remain.
	if len(result.Scores) != 1 {
		t.Fatalf("filter_ineligible: expected 1 score, got %d", len(result.Scores))
	}
	if result.Scores[0].EligibleStatus != "eligible" {
		t.Fatalf("filter_ineligible: expected eligible item, got %q", result.Scores[0].EligibleStatus)
	}
	if result.Scores[0].JapaScore != 100 {
		t.Fatalf("filter_ineligible: expected JapaScore 100, got %v", result.Scores[0].JapaScore)
	}
}
