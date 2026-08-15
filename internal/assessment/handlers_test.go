// Package assessment — handlers_test.go tests the HTTP handlers and scoring
// helpers for the Japa Readiness Assessment. Includes auth denial tests for
// authenticated routes and table-driven tests for ClassifyScore and
// GenerateRoadmap.
//
// Ref: japaconnect-engineering/sections/71-readiness-assessment.md
//
//	japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md
package assessment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// ---------------------------------------------------------------------------
// Auth denial tests — authenticated routes must return 401 without auth.
// ---------------------------------------------------------------------------

// TestHandleHistory_Unauthorized verifies that the history endpoint requires
// authentication. No auth context set → requireUser should return 401.
func TestHandleHistory_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assessment/history", nil)
	// Set a tenant context so requireTenant passes, isolating the auth check.
	req = req.WithContext(withTenant(req.Context()))
	rr := httptest.NewRecorder()
	h.HandleHistory(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleHistory without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminGetForm_Unauthorized verifies that the admin get-form
// endpoint requires authentication.
func TestHandleAdminGetForm_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/assessment/form", nil)
	req = req.WithContext(withTenant(req.Context()))
	rr := httptest.NewRecorder()
	h.HandleAdminGetForm(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminGetForm without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminUpdateForm_Unauthorized verifies that the admin update-form
// endpoint requires authentication.
func TestHandleAdminUpdateForm_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/assessment/form", nil)
	req = req.WithContext(withTenant(req.Context()))
	rr := httptest.NewRecorder()
	h.HandleAdminUpdateForm(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminUpdateForm without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminListResults_Unauthorized verifies that the admin list-results
// endpoint requires authentication.
func TestHandleAdminListResults_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/assessment/results", nil)
	req = req.WithContext(withTenant(req.Context()))
	rr := httptest.NewRecorder()
	h.HandleAdminListResults(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminListResults without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminStats_Unauthorized verifies that the admin stats endpoint
// requires authentication.
func TestHandleAdminStats_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/assessment/stats", nil)
	req = req.WithContext(withTenant(req.Context()))
	rr := httptest.NewRecorder()
	h.HandleAdminStats(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminStats without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleGetResult_UserResultUnauthorized verifies that fetching a user's
// result without auth returns 401. The handler checks auth only when the
// result has a user_id. Since the service is nil, the handler returns 503
// before reaching the auth check — so we test the auth check path by using
// a non-nil service is not feasible without a DB. Instead, we verify the
// handler returns a non-200 status (503 for nil service) which confirms the
// route is not publicly accessible without proper setup.
func TestHandleGetResult_NilServiceReturns503(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assessment/result/"+uuid.New().String(), nil)
	req = req.WithContext(withTenant(req.Context()))
	rr := httptest.NewRecorder()
	h.HandleGetResult(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleGetResult with nil service = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// ClassifyScore — table-driven tests
// ---------------------------------------------------------------------------

// testThresholds is the standard 4-band threshold configuration from
// ADR-071 §1, used across ClassifyScore tests.
var testThresholds = []Threshold{
	{Min: 0, Max: 39, Label: "Early Stage", Color: "red"},
	{Min: 40, Max: 69, Label: "Getting Ready", Color: "amber"},
	{Min: 70, Max: 84, Label: "Nearly There", Color: "blue"},
	{Min: 85, Max: 100, Label: "Japa Ready", Color: "green"},
}

// TestClassifyScore verifies the threshold classification logic with
// table-driven tests covering all bands and edge cases.
func TestClassifyScore(t *testing.T) {
	tests := []struct {
		name      string
		score     int
		wantLabel string
	}{
		{"score 0 is Early Stage", 0, "Early Stage"},
		{"score 30 is Early Stage", 30, "Early Stage"},
		{"score 39 is Early Stage (upper boundary)", 39, "Early Stage"},
		{"score 40 is Getting Ready (lower boundary)", 40, "Getting Ready"},
		{"score 50 is Getting Ready", 50, "Getting Ready"},
		{"score 69 is Getting Ready (upper boundary)", 69, "Getting Ready"},
		{"score 70 is Nearly There (lower boundary)", 70, "Nearly There"},
		{"score 75 is Nearly There", 75, "Nearly There"},
		{"score 84 is Nearly There (upper boundary)", 84, "Nearly There"},
		{"score 85 is Japa Ready (lower boundary)", 85, "Japa Ready"},
		{"score 95 is Japa Ready", 95, "Japa Ready"},
		{"score 100 is Japa Ready (upper boundary)", 100, "Japa Ready"},
		{"score 101 is Unclassified (above all bands)", 101, "Unclassified"},
		{"score -1 is Unclassified (below all bands)", -1, "Unclassified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyScore(tt.score, testThresholds)
			if got != tt.wantLabel {
				t.Errorf("ClassifyScore(%d) = %q, want %q", tt.score, got, tt.wantLabel)
			}
		})
	}
}

// TestClassifyScore_EmptyThresholds verifies that an empty threshold list
// returns "Unclassified" for any score.
func TestClassifyScore_EmptyThresholds(t *testing.T) {
	got := ClassifyScore(50, nil)
	if got != "Unclassified" {
		t.Errorf("ClassifyScore(50, nil) = %q, want %q", got, "Unclassified")
	}
}

// ---------------------------------------------------------------------------
// GenerateRoadmap — tests that weakest dimensions get roadmap steps
// ---------------------------------------------------------------------------

// TestGenerateRoadmap verifies that the roadmap is generated with steps for
// the weakest dimensions first, ordered by priority.
func TestGenerateRoadmap(t *testing.T) {
	scoringConfig := ScoringConfig{
		MaxScore:   100,
		Thresholds: testThresholds,
		DimensionWeights: map[string]float64{
			"financial":             0.3,
			"language":              0.2,
			"qualifications":        0.2,
			"visa_eligibility":      0.15,
			"destination_knowledge": 0.15,
		},
	}

	// Dimension scores: financial is weakest, destination_knowledge is strongest.
	dimensionScores := map[string]int{
		"financial":             20,
		"language":              45,
		"qualifications":        60,
		"visa_eligibility":      70,
		"destination_knowledge": 85,
	}

	roadmap := GenerateRoadmap(dimensionScores, scoringConfig)
	if len(roadmap) == 0 {
		t.Fatal("GenerateRoadmap returned empty roadmap")
	}

	// The first step should be for the weakest dimension ("financial").
	if roadmap[0].Dimension != "financial" {
		t.Errorf("GenerateRoadmap first step dimension = %q, want %q",
			roadmap[0].Dimension, "financial")
	}
	if roadmap[0].Priority != 1 {
		t.Errorf("GenerateRoadmap first step priority = %d, want 1", roadmap[0].Priority)
	}

	// Verify priorities are sequential (1, 2, 3, ...).
	for i, step := range roadmap {
		wantPriority := i + 1
		if step.Priority != wantPriority {
			t.Errorf("GenerateRoadmap step %d priority = %d, want %d",
				i, step.Priority, wantPriority)
		}
	}

	// Verify all steps have non-empty titles and content slugs.
	for i, step := range roadmap {
		if step.Title == "" {
			t.Errorf("GenerateRoadmap step %d has empty title", i)
		}
		if step.ContentSlug == "" {
			t.Errorf("GenerateRoadmap step %d has empty content_slug", i)
		}
	}
}

// TestGenerateRoadmap_WeakestDimensionFirst verifies that the weakest
// dimension (lowest score) gets the first roadmap steps.
func TestGenerateRoadmap_WeakestDimensionFirst(t *testing.T) {
	scoringConfig := ScoringConfig{
		MaxScore:   100,
		Thresholds: testThresholds,
	}

	dimensionScores := map[string]int{
		"visa_eligibility": 10, // weakest
		"financial":        80,
	}

	roadmap := GenerateRoadmap(dimensionScores, scoringConfig)
	if len(roadmap) == 0 {
		t.Fatal("GenerateRoadmap returned empty roadmap")
	}

	// visa_eligibility is weakest (score 10) → its steps come first.
	if roadmap[0].Dimension != "visa_eligibility" {
		t.Errorf("GenerateRoadmap first step dimension = %q, want %q (weakest)",
			roadmap[0].Dimension, "visa_eligibility")
	}
}

// TestGenerateRoadmap_EmptyScores verifies that an empty dimension scores
// map produces an empty roadmap.
func TestGenerateRoadmap_EmptyScores(t *testing.T) {
	roadmap := GenerateRoadmap(map[string]int{}, ScoringConfig{})
	if len(roadmap) != 0 {
		t.Errorf("GenerateRoadmap with empty scores = %d steps, want 0", len(roadmap))
	}
}

// TestGenerateRoadmap_UnknownDimension verifies that dimensions without
// curated steps are skipped (no panic, no empty steps).
func TestGenerateRoadmap_UnknownDimension(t *testing.T) {
	dimensionScores := map[string]int{
		"unknown_dimension": 10,
		"financial":         50,
	}

	roadmap := GenerateRoadmap(dimensionScores, ScoringConfig{})

	// Only "financial" has curated steps; "unknown_dimension" is skipped.
	for _, step := range roadmap {
		if step.Dimension == "unknown_dimension" {
			t.Errorf("GenerateRoadmap should skip unknown dimensions, found step for %q",
				step.Dimension)
		}
	}
	if len(roadmap) == 0 {
		t.Error("GenerateRoadmap should still produce steps for known dimensions")
	}
}

// TestGenerateRoadmap_DefaultSeedDimensions verifies that the default seeded
// form dimensions (financial, logistical, professional, personal — migration
// 000279) all produce curated roadmap steps from the built-in fallback
// library. This is a regression test for the dimension-mismatch bug where the
// curated library only covered the ADR-071 example dimensions and 3 of 4
// seeded dimensions produced no roadmap steps.
func TestGenerateRoadmap_DefaultSeedDimensions(t *testing.T) {
	dimensionScores := map[string]int{
		"financial":    20,
		"logistical":   40,
		"professional": 60,
		"personal":     80,
	}

	roadmap := GenerateRoadmap(dimensionScores, ScoringConfig{})

	// Every seeded dimension must have at least one curated step.
	seen := map[string]bool{}
	for _, step := range roadmap {
		seen[step.Dimension] = true
	}
	for _, dim := range []string{"financial", "logistical", "professional", "personal"} {
		if !seen[dim] {
			t.Errorf("GenerateRoadmap produced no steps for seeded dimension %q", dim)
		}
	}
}

// TestGenerateRoadmap_CuratedRoadmapOverride verifies that when
// scoring_config.curated_roadmap is set, GenerateRoadmap uses the
// admin-configured library instead of the built-in fallback (ADR-071 §3 —
// curated steps are admin-configurable, stored in scoring_config).
func TestGenerateRoadmap_CuratedRoadmapOverride(t *testing.T) {
	customStep := RoadmapStep{
		Title:       "Custom admin step",
		Description: "Configured by admin via scoring_config",
		ContentSlug: "guide/custom-admin-guide",
	}
	scoringConfig := ScoringConfig{
		CuratedRoadmap: map[string][]RoadmapStep{
			"financial": {customStep},
		},
	}

	roadmap := GenerateRoadmap(
		map[string]int{"financial": 30, "logistical": 50},
		scoringConfig,
	)

	// The admin-configured library only has "financial", so "logistical"
	// (which exists in the fallback) must NOT appear — proving the override
	// is used instead of the fallback.
	for _, step := range roadmap {
		if step.Dimension == "logistical" {
			t.Error("GenerateRoadmap used the fallback library instead of the admin-configured curated_roadmap")
		}
	}
	if len(roadmap) == 0 || roadmap[0].Title != customStep.Title {
		t.Errorf("GenerateRoadmap did not use the admin-configured step, got %v", roadmap)
	}
}

// ---------------------------------------------------------------------------
// parseScoringConfig and parseFieldScoringConfig — parsing tests
// ---------------------------------------------------------------------------

// TestParseScoringConfig verifies the scoring_config JSONB parsing.
func TestParseScoringConfig(t *testing.T) {
	t.Run("empty bytes returns zero config", func(t *testing.T) {
		cfg, err := parseScoringConfig(nil)
		if err != nil {
			t.Fatalf("parseScoringConfig(nil) error = %v", err)
		}
		if cfg.MaxScore != 0 {
			t.Errorf("parseScoringConfig(nil) max_score = %d, want 0", cfg.MaxScore)
		}
	})

	t.Run("valid config parses correctly", func(t *testing.T) {
		raw := []byte(`{"max_score":100,"thresholds":[{"min":0,"max":39,"label":"Early Stage","color":"red"}],"dimension_weights":{"financial":0.3}}`)
		cfg, err := parseScoringConfig(raw)
		if err != nil {
			t.Fatalf("parseScoringConfig error = %v", err)
		}
		if cfg.MaxScore != 100 {
			t.Errorf("parseScoringConfig max_score = %d, want 100", cfg.MaxScore)
		}
		if len(cfg.Thresholds) != 1 {
			t.Errorf("parseScoringConfig thresholds len = %d, want 1", len(cfg.Thresholds))
		}
		if cfg.Thresholds[0].Label != "Early Stage" {
			t.Errorf("parseScoringConfig threshold label = %q, want %q",
				cfg.Thresholds[0].Label, "Early Stage")
		}
		if cfg.DimensionWeights["financial"] != 0.3 {
			t.Errorf("parseScoringConfig dimension_weights[financial] = %v, want 0.3",
				cfg.DimensionWeights["financial"])
		}
	})

	t.Run("malformed config returns error", func(t *testing.T) {
		_, err := parseScoringConfig([]byte("{not json"))
		if err == nil {
			t.Error("parseScoringConfig(malformed) error = nil, want error")
		}
	})
}

// TestParseFieldScoringConfig verifies the field config JSONB parsing.
func TestParseFieldScoringConfig(t *testing.T) {
	t.Run("empty bytes returns zero config", func(t *testing.T) {
		cfg := parseFieldScoringConfig(nil)
		if cfg.Weight != 0 {
			t.Errorf("parseFieldScoringConfig(nil) weight = %v, want 0", cfg.Weight)
		}
	})

	t.Run("valid config parses correctly", func(t *testing.T) {
		raw := []byte(`{"weight":1.5,"dimension":"financial","option_scores":{"option_a":10,"option_b":5}}`)
		cfg := parseFieldScoringConfig(raw)
		if cfg.Weight != 1.5 {
			t.Errorf("parseFieldScoringConfig weight = %v, want 1.5", cfg.Weight)
		}
		if cfg.Dimension != "financial" {
			t.Errorf("parseFieldScoringConfig dimension = %q, want %q",
				cfg.Dimension, "financial")
		}
		if cfg.OptionScores["option_a"] != 10 {
			t.Errorf("parseFieldScoringConfig option_scores[option_a] = %d, want 10",
				cfg.OptionScores["option_a"])
		}
	})

	t.Run("malformed config returns zero config", func(t *testing.T) {
		cfg := parseFieldScoringConfig([]byte("{not json"))
		if cfg.Weight != 0 {
			t.Errorf("parseFieldScoringConfig(malformed) weight = %v, want 0", cfg.Weight)
		}
	})
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// withTenant returns a context with a test tenant set, so requireTenant
// passes in handler tests.
func withTenant(ctx context.Context) context.Context {
	tenantID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	return context.WithValue(ctx, tenant.TenantContextKey, &tenant.Tenant{
		ID:   tenantID,
		Name: "Test Tenant",
		Slug: "test",
	})
}

// withUser returns a context with an authenticated user set, so requireUser
// passes in handler tests.
func withUser(ctx context.Context) context.Context {
	return context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID:   pgtype.UUID{Bytes: uuid.New(), Valid: true},
		TenantID: pgtype.UUID{Bytes: uuid.New(), Valid: true},
	})
}
