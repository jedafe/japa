//go:build integration

package jce_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/jce"
	_ "github.com/jedafe/japa/internal/jce/scoring"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/testdb"
)

func mustUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		t.Fatalf("mustUUID: %v", err)
	}
	return id
}

func toUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("toUUID %q: %v", s, err)
	}
	return id
}

func strPtr(s string) *string { return &s }

// testResolver resolves synthetic "test" item_refs for the scoring tests.
type testResolver struct{}

func (testResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (resolver.ResolvedItem, bool, error) {
	id, _ := ref["id"].(string)
	return resolver.ResolvedItem{
		Type:  "test",
		ID:    id,
		Title: "Test " + id,
		Metadata: map[string]any{
			"score_inputs": map[string]any{
				"cost":         1000,
				"income":       80000,
				"speed":        6,
				"language":     "English",
				"pr_path_ease": 70,
			},
		},
	}, true, nil
}

func (testResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit, offset int32) ([]resolver.ResolvedItem, error) {
	return nil, nil
}

type testEnv struct {
	svc  *jce.Service
	comp *comparison.Service
}

func newTestService(t *testing.T, d *db.DB) testEnv {
	t.Helper()

	settings := config.NewSettings(d, nil)
	t.Cleanup(settings.Close)

	r := resolver.NewRegistry()
	r.Register("test", testResolver{})

	comp := comparison.NewService(d)
	comp.SetResolver(r)
	comp.SetSettings(settings)
	comp.SetContentService(content.NewService(d))
	comp.SetValkey(nil)

	fc := features.New(d, nil)
	t.Cleanup(fc.Close)

	svc := jce.NewService(d, comp, r, settings, nil)
	svc.SetFeatureChecker(fc)
	t.Cleanup(svc.Close)

	return testEnv{svc: svc, comp: comp}
}

func withAuth(ctx context.Context, userID, tenantID pgtype.UUID) context.Context {
	return context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: userID,
	})
}

func withRBAC(ctx context.Context, r *rbac.RBAC) context.Context {
	return context.WithValue(ctx, rbac.ResolverKey, r)
}

func newTestContext(t *testing.T, d *db.DB, userID, tenantID pgtype.UUID) context.Context {
	t.Helper()
	ctx := t.Context()
	r := rbac.New(d, nil)
	ctx = withAuth(ctx, userID, tenantID)
	return withRBAC(ctx, r)
}

func enableJCEFeatures(t *testing.T, d *db.DB, tenantID pgtype.UUID) {
	t.Helper()
	ctx := t.Context()
	for _, key := range []string{"feature.jce.profiles", "feature.jce.smart_compare", "feature.jce.scoring"} {
		_, err := d.Writer.Exec(ctx, `
			INSERT INTO feature_flags (key, enabled, config) VALUES ($1, $2, true, '{}')
			ON CONFLICT (key) DO UPDATE SET enabled = true, updated_at = now()
		`, key, tenantID)
		if err != nil {
			t.Fatalf("enableJCEFeatures %s: %v", key, err)
		}
	}
}

func seedJCEPermissions(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID) {
	t.Helper()
	ctx := t.Context()

	groupID := mustUUID(t)
	groupSlug := fmt.Sprintf("jce-test-%s", uuid.New().String())
	_, err := d.Writer.Exec(ctx, `
		INSERT INTO user_groups (id, name, slug, is_system) VALUES ($1, $2, 'JCE Test', $3, false)
	`, groupID, tenantID, groupSlug)
	if err != nil {
		t.Fatalf("seedJCEPermissions insert group: %v", err)
	}

	roleID := mustUUID(t)
	roleSlug := fmt.Sprintf("jce-test-role-%s", uuid.New().String())
	_, err = d.Writer.Exec(ctx, `
		INSERT INTO user_roles (id, name, slug) VALUES ($1, $2, 'JCE Test', $3)
	`, roleID, tenantID, roleSlug)
	if err != nil {
		t.Fatalf("seedJCEPermissions insert role: %v", err)
	}

	_, err = d.Writer.Exec(ctx, `INSERT INTO group_roles (group_id, role_id) VALUES ($1, $2)`, groupID, roleID)
	if err != nil {
		t.Fatalf("seedJCEPermissions insert group_roles: %v", err)
	}

	_, err = d.Writer.Exec(ctx, `
		INSERT INTO user_group_memberships (id, user_id, group_id) VALUES ($1, $2, $3, $4)
	`, mustUUID(t), tenantID, userID, groupID)
	if err != nil {
		t.Fatalf("seedJCEPermissions insert membership: %v", err)
	}

	keys := []string{
		"jce.profile.list",
		"jce.profile.view",
		"jce.profile.create",
		"jce.profile.update",
		"jce.profile.delete",
		"jce.score.view",
		"jce.score.recalculate",
		"jce.recommendation.view",
		"jce.factor.manage",
		"comparison.edit_any",
	}
	for _, key := range keys {
		var permID pgtype.UUID
		err := d.Reader.QueryRow(ctx, `SELECT id FROM permissions WHERE key = $1`, key).Scan(&permID)
		if err != nil {
			t.Fatalf("seedJCEPermissions lookup %s: %v", key, err)
		}
		_, err = d.Writer.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)`, roleID, permID)
		if err != nil {
			t.Fatalf("seedJCEPermissions insert %s: %v", key, err)
		}
	}
}

func seedComparisonTypeForScoring(t *testing.T, d *db.DB, tenantID pgtype.UUID) pgtype.UUID {
	t.Helper()
	ctx := t.Context()

	fieldGroupID := mustUUID(t)
	groupSlug := fmt.Sprintf("test-fields-%s", uuid.New().String())
	_, err := d.Writer.Exec(ctx, `
		INSERT INTO custom_field_groups (id, name, slug, is_system) VALUES ($1, $2, 'Test Fields', $3, false)
	`, fieldGroupID, tenantID, groupSlug)
	if err != nil {
		t.Fatalf("seedComparisonTypeForScoring insert field group: %v", err)
	}

	typeID := mustUUID(t)
	typeSlug := fmt.Sprintf("test-type-%s", uuid.New().String())
	cfg := map[string]any{
		"item_ref_types":  []string{"test"},
		"scoring_enabled": true,
		"default_weights": map[string]float64{
			"cost":           0.25,
			"income":         0.25,
			"speed":          0.25,
			"language_match": 0.25,
			"pr_path_ease":   0.25,
		},
		"smart_modes": map[string]any{
			"cost": map[string]any{"factor_multipliers": map[string]float64{"cost": 2.0}},
		},
		"allowed_goals":   []map[string]any{{"key": "low_cost", "default_weight": 0.25}},
		"goal_factor_map": map[string][]string{"low_cost": {"cost"}},
		"recommendation_templates": []map[string]any{
			{"factor_key": "cost", "condition": "score_lt", "threshold": 50, "text": "Consider a lower-cost option.", "priority": 10},
			{"factor_key": "income", "condition": "score_gt", "threshold": 50, "text": "Strong income potential.", "priority": 5},
		},
	}
	cfgBytes, _ := json.Marshal(cfg)
	_, err = d.Writer.Exec(ctx, `
		INSERT INTO comparison_type_definitions (id, name, slug, description, field_group_id, item_label, is_system, config) VALUES ($1, $2, 'Test Type', $3, '', $4, 'Item', false, $5::jsonb)
	`, typeID, tenantID, typeSlug, fieldGroupID, cfgBytes)
	if err != nil {
		t.Fatalf("seedComparisonTypeForScoring insert type: %v", err)
	}

	return typeID
}

func seedScoreFactors(t *testing.T, d *db.DB, tenantID pgtype.UUID) {
	t.Helper()
	ctx := t.Context()

	factors := []struct {
		key    string
		label  string
		source string
		weight float64
		cfg    map[string]any
	}{
		{"cost", "Cost", "resolver_metadata", 0.25, map[string]any{"kind": "number", "direction": "lower_is_better", "value_key": "cost", "min": 0, "max": 5000, "missing_score": 0}},
		{"income", "Income", "resolver_metadata", 0.25, map[string]any{"kind": "number", "direction": "higher_is_better", "value_key": "income", "min": 0, "max": 100000, "missing_score": 0}},
		{"speed", "Speed", "resolver_metadata", 0.25, map[string]any{"kind": "number", "direction": "lower_is_better", "value_key": "speed", "min": 0, "max": 12, "missing_score": 0}},
		{"language_match", "Language Match", "resolver_metadata", 0.25, map[string]any{"kind": "text", "value_key": "language", "ideal_texts": []string{"English"}, "missing_score": 0}},
		{"pr_path_ease", "PR Path Ease", "resolver_metadata", 0.25, map[string]any{"kind": "number", "direction": "higher_is_better", "value_key": "pr_path_ease", "min": 0, "max": 100, "missing_score": 0}},
	}

	for _, f := range factors {
		cfgBytes, _ := json.Marshal(f.cfg)
		_, err := d.Writer.Exec(ctx, `
			INSERT INTO comparison_score_factors (id, comparison_type_id, factor_key, label, data_source, default_weight, config, is_system) VALUES ($1, $2, NULL, $3, $4, $5, $6, $7::jsonb, false)
		`, mustUUID(t), tenantID, f.key, f.label, f.source, f.weight, cfgBytes)
		if err != nil {
			t.Fatalf("seedScoreFactors %s: %v", f.key, err)
		}
	}
}

func createComparison(t *testing.T, ctx context.Context, comp *comparison.Service, tenantID, userID, spaceID, typeID pgtype.UUID) pgtype.UUID {
	t.Helper()

	res, err := comp.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "Test Comparison",
		Body:       "A test body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		TypeID:     typeID,
		Depth:      comparison.DepthDetailed,
		Items: []comparison.JCEItemInput{{
			Title:    "Test A",
			TypeID:   typeID,
			Position: 1,
			ItemRef:  []byte(`{"type":"test","id":"a"}`),
		}},
	})
	if err != nil {
		t.Fatalf("createComparison: %v", err)
	}
	return res.ID
}

func createProfile(t *testing.T, ctx context.Context, svc *jce.Service, tenantID, userID, spaceID pgtype.UUID, goals []jce.ProfileGoal) jce.ComparisonProfile {
	t.Helper()

	spaceStr := spaceID.String()
	in := jce.ComparisonProfileInput{
		Title:          "My Profile",
		SourceSpaceID:  &spaceStr,
		TargetSpaceIDs: []string{spaceStr},
		Goals:          goals,
		Preferences:    map[string]float64{},
		Demographics:   jce.ProfileDemographics{LanguageProficiency: "English"},
	}
	p, err := svc.CreateComparisonProfile(ctx, tenantID, userID, in)
	if err != nil {
		t.Fatalf("createProfile: %v", err)
	}
	return p
}

func TestService_ProfileCRUDAndDefault(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	spaceStr := spaceID.String()
	in := jce.ComparisonProfileInput{
		Title:          "My Profile",
		SourceSpaceID:  &spaceStr,
		TargetSpaceIDs: []string{spaceStr},
		Goals:          []jce.ProfileGoal{{Key: "low_cost", Weight: 0.25}},
	}

	p, err := env.svc.CreateComparisonProfile(ctx, tenantID, userID, in)
	if err != nil {
		t.Fatalf("CreateComparisonProfile: %v", err)
	}
	if p.Title != "My Profile" {
		t.Errorf("created profile title = %q, want %q", p.Title, "My Profile")
	}

	list, err := env.svc.ListComparisonProfiles(ctx, tenantID, userID, false, false)
	if err != nil {
		t.Fatalf("ListComparisonProfiles: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListComparisonProfiles returned %d profiles, want 1", len(list))
	}

	got, err := env.svc.GetComparisonProfile(ctx, toUUID(t, p.ID), tenantID, userID)
	if err != nil {
		t.Fatalf("GetComparisonProfile: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("GetComparisonProfile ID = %q, want %q", got.ID, p.ID)
	}

	upd, err := env.svc.UpdateComparisonProfile(ctx, toUUID(t, p.ID), tenantID, userID, jce.ComparisonProfileInput{Title: "Updated Profile"})
	if err != nil {
		t.Fatalf("UpdateComparisonProfile: %v", err)
	}
	if upd.Title != "Updated Profile" {
		t.Errorf("updated title = %q, want %q", upd.Title, "Updated Profile")
	}

	if err := env.svc.SetDefaultComparisonProfile(ctx, toUUID(t, p.ID), tenantID, userID); err != nil {
		t.Fatalf("SetDefaultComparisonProfile: %v", err)
	}
	def, err := env.svc.GetComparisonProfile(ctx, toUUID(t, p.ID), tenantID, userID)
	if err != nil {
		t.Fatalf("GetComparisonProfile after default: %v", err)
	}
	if !def.IsDefault {
		t.Errorf("profile.IsDefault = false after SetDefault")
	}

	if err := env.svc.DeleteComparisonProfile(ctx, toUUID(t, p.ID), tenantID, userID); err != nil {
		t.Fatalf("DeleteComparisonProfile: %v", err)
	}
	if _, err := env.svc.GetComparisonProfile(ctx, toUUID(t, p.ID), tenantID, userID); !errors.Is(err, errpkg.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestService_ScoreFactorCRUD(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	_ = testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	in := jce.ScoreFactorInput{
		FactorKey:     "cost",
		Label:         "Cost",
		DataSource:    "resolver_metadata",
		DefaultWeight: 0.25,
		Config: jce.ScoreFactorConfig{
			Kind:      "number",
			Direction: "lower_is_better",
			ValueKey:  "cost",
			Min:       0,
			Max:       5000,
		},
	}

	f, err := env.svc.CreateScoreFactor(ctx, tenantID, userID, in)
	if err != nil {
		t.Fatalf("CreateScoreFactor: %v", err)
	}
	if f.FactorKey != "cost" {
		t.Errorf("created factor key = %q, want %q", f.FactorKey, "cost")
	}

	factors, err := env.svc.ListScoreFactors(ctx, tenantID, pgtype.UUID{}, true)
	if err != nil {
		t.Fatalf("ListScoreFactors: %v", err)
	}
	if len(factors) != 1 {
		t.Fatalf("ListScoreFactors returned %d factors, want 1", len(factors))
	}

	got, err := env.svc.GetScoreFactor(ctx, toUUID(t, f.ID), tenantID)
	if err != nil {
		t.Fatalf("GetScoreFactor: %v", err)
	}
	if got.Label != "Cost" {
		t.Errorf("GetScoreFactor label = %q, want %q", got.Label, "Cost")
	}

	upd, err := env.svc.UpdateScoreFactor(ctx, toUUID(t, f.ID), tenantID, userID, jce.ScoreFactorInput{
		FactorKey:     f.FactorKey,
		Label:         "Cost Updated",
		DataSource:    f.DataSource,
		DefaultWeight: 0.30,
		Config:        f.Config,
	})
	if err != nil {
		t.Fatalf("UpdateScoreFactor: %v", err)
	}
	if upd.Label != "Cost Updated" {
		t.Errorf("updated label = %q, want %q", upd.Label, "Cost Updated")
	}

	if err := env.svc.DeleteScoreFactor(ctx, toUUID(t, f.ID), tenantID, userID); err != nil {
		t.Fatalf("DeleteScoreFactor: %v", err)
	}
	if _, err := env.svc.GetScoreFactor(ctx, toUUID(t, f.ID), tenantID); !errors.Is(err, errpkg.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestService_ScoreComparison(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)
	seedScoreFactors(t, d, tenantID)
	typeID := seedComparisonTypeForScoring(t, d, tenantID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	cmpID := createComparison(t, ctx, env.comp, tenantID, userID, spaceID, typeID)
	p := createProfile(t, ctx, env.svc, tenantID, userID, spaceID, []jce.ProfileGoal{{Key: "low_cost", Weight: 0.25}})
	pid := toUUID(t, p.ID)

	result, err := env.svc.ScoreComparison(ctx, cmpID, tenantID, userID, &pid, "", false)
	if err != nil {
		t.Fatalf("ScoreComparison: %v", err)
	}
	if len(result.Scores) != 1 {
		t.Fatalf("ScoreComparison returned %d scores, want 1", len(result.Scores))
	}
	if result.Scores[0].JapaScore <= 0 {
		t.Errorf("JapaScore = %f, want > 0", result.Scores[0].JapaScore)
	}
	if result.Version < 2 {
		t.Errorf("score version = %d, want >= 2", result.Version)
	}

	got, err := env.svc.GetComparisonScore(ctx, cmpID, tenantID, userID, &pid)
	if err != nil {
		t.Fatalf("GetComparisonScore: %v", err)
	}
	if got.Version != result.Version {
		t.Errorf("GetComparisonScore version = %d, want %d", got.Version, result.Version)
	}
}

func TestService_PreviewScoreForItems(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)
	seedScoreFactors(t, d, tenantID)
	typeID := seedComparisonTypeForScoring(t, d, tenantID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	spaceStr := spaceID.String()
	in := jce.ScorePreviewInput{
		TypeID:   typeID.String(),
		ItemRefs: []map[string]any{{"type": "test", "id": "a"}},
		ProfileInput: &jce.ComparisonProfileInput{
			Title:          "Preview Profile",
			SourceSpaceID:  &spaceStr,
			TargetSpaceIDs: []string{spaceStr},
			Goals:          []jce.ProfileGoal{{Key: "low_cost", Weight: 0.25}},
			Demographics:   jce.ProfileDemographics{LanguageProficiency: "English"},
		},
	}

	result, err := env.svc.PreviewScoreForItems(ctx, tenantID, userID, in)
	if err != nil {
		t.Fatalf("PreviewScoreForItems: %v", err)
	}
	if len(result.Scores) != 1 {
		t.Fatalf("PreviewScoreForItems returned %d scores, want 1", len(result.Scores))
	}
	if result.Scores[0].JapaScore <= 0 {
		t.Errorf("JapaScore = %f, want > 0", result.Scores[0].JapaScore)
	}
}

func TestService_ListRecommendations(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)
	seedScoreFactors(t, d, tenantID)
	typeID := seedComparisonTypeForScoring(t, d, tenantID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	cmpID := createComparison(t, ctx, env.comp, tenantID, userID, spaceID, typeID)
	p := createProfile(t, ctx, env.svc, tenantID, userID, spaceID, []jce.ProfileGoal{{Key: "low_cost", Weight: 0.25}})
	pid := toUUID(t, p.ID)

	if _, err := env.svc.ScoreComparison(ctx, cmpID, tenantID, userID, &pid, "", false); err != nil {
		t.Fatalf("ScoreComparison: %v", err)
	}

	recs, err := env.svc.ListRecommendations(ctx, cmpID, tenantID, userID, &pid, 10)
	if err != nil {
		t.Fatalf("ListRecommendations: %v", err)
	}
	if len(recs) == 0 {
		t.Fatalf("ListRecommendations returned no recommendations")
	}
}
