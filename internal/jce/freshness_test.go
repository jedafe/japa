//go:build integration

package jce

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/testdb"
)

// testFreshnessService builds a minimal wired JCE service for integration tests.
func testFreshnessService(t *testing.T, d *db.DB) *Service {
	t.Helper()

	settings := config.NewSettings(d, nil)
	t.Cleanup(settings.Close)

	comp := comparison.NewService(d)
	comp.SetSettings(settings)
	comp.SetValkey(nil)
	comp.SetContentService(content.NewService(d))

	r := resolver.NewRegistry()
	svc := NewService(d, comp, r, settings, nil)
	t.Cleanup(svc.Close)

	return svc
}

func makeValue(t *testing.T, updatedAt time.Time, metadata string) gen.ComparisonValue {
	t.Helper()
	return gen.ComparisonValue{
		ItemID:      pgUUID(t, uuid.New().String()),
		DimensionID: pgUUID(t, uuid.New().String()),
		TenantID:    pgUUID(t, uuid.New().String()),
		Metadata:    []byte(metadata),
		UpdatedAt:   pgtype.Timestamptz{Time: updatedAt, Valid: !updatedAt.IsZero()},
		CreatedAt:   pgtype.Timestamptz{Time: updatedAt, Valid: !updatedAt.IsZero()},
	}
}

func TestFreshness_Cell_MissingMetadata(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := defaultFreshnessSettings()
	v := makeValue(t, time.Now().UTC(), "{}")

	score, isStale, err := s.cellFreshness(t.Context(), v, nil, settings)
	if err != nil {
		t.Fatalf("cellFreshness: %v", err)
	}
	if isStale {
		t.Fatalf("expected fresh value")
	}

	// Missing source defaults to official (weight 100, trust 3). Missing confidence is 0.75.
	// (100 - 0) * 1.0 * 0.6 * 0.75 * 1.0 = 45
	want := 45
	if score != want {
		t.Fatalf("score = %d, want %d", score, want)
	}
}

func TestFreshness_Cell_IsEstimatePenalty(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := defaultFreshnessSettings()

	source := &DataSource{SourceKind: "official", TrustLevel: 5}
	meta, _ := json.Marshal(map[string]any{"confidence": "high", "is_estimate": true})
	v := makeValue(t, time.Now().UTC(), string(meta))

	score, _, err := s.cellFreshness(t.Context(), v, source, settings)
	if err != nil {
		t.Fatalf("cellFreshness: %v", err)
	}

	// (100) * 1.0 * 1.0 * 1.0 * 0.85 = 85
	want := 85
	if score != want {
		t.Fatalf("score = %d, want %d", score, want)
	}

	// Same source/value without is_estimate should be 100.
	metaNoEstimate, _ := json.Marshal(map[string]any{"confidence": "high"})
	v2 := makeValue(t, time.Now().UTC(), string(metaNoEstimate))
	score2, _, err := s.cellFreshness(t.Context(), v2, source, settings)
	if err != nil {
		t.Fatalf("cellFreshness: %v", err)
	}
	if score2 != 100 {
		t.Fatalf("score without estimate penalty = %d, want 100", score2)
	}
}

func TestFreshness_Cell_StaleThreshold(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := freshnessSettings{
		staleHours:       1,
		decayPerHour:     10,
		officialWeight:   100,
		thirdPartyWeight: 75,
		aiEstimateWeight: 50,
	}
	source := &DataSource{SourceKind: "official", TrustLevel: 5}
	meta, _ := json.Marshal(map[string]any{"confidence": "high"})

	// Just over stale threshold: age = 2h, decay applies to 1h past threshold.
	v := makeValue(t, time.Now().UTC().Add(-2*time.Hour), string(meta))
	score, isStale, err := s.cellFreshness(t.Context(), v, source, settings)
	if err != nil {
		t.Fatalf("cellFreshness: %v", err)
	}
	if !isStale {
		t.Fatalf("expected stale value")
	}
	// (100 - 10) * 1.0 * 1.0 * 1.0 * 1.0 = 90
	want := 90
	if score != want {
		t.Fatalf("score = %d, want %d", score, want)
	}

	// Well under threshold: no decay, not stale.
	v2 := makeValue(t, time.Now().UTC().Add(-30*time.Minute), string(meta))
	score2, isStale2, err := s.cellFreshness(t.Context(), v2, source, settings)
	if err != nil {
		t.Fatalf("cellFreshness: %v", err)
	}
	if isStale2 {
		t.Fatalf("expected fresh value")
	}
	if score2 != 100 {
		t.Fatalf("score = %d, want 100", score2)
	}
}

func TestFreshness_Cell_SourceKindWeights(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := defaultFreshnessSettings()
	meta, _ := json.Marshal(map[string]any{"confidence": "high"})
	updatedAt := time.Now().UTC()

	tests := []struct {
		kind string
		want int
	}{
		{"official", 100},
		{"third_party", 75},
		{"ai_estimate", 50},
	}

	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			source := &DataSource{SourceKind: tc.kind, TrustLevel: 5}
			v := makeValue(t, updatedAt, string(meta))
			score, _, err := s.cellFreshness(t.Context(), v, source, settings)
			if err != nil {
				t.Fatalf("cellFreshness: %v", err)
			}
			if score != tc.want {
				t.Fatalf("score = %d, want %d", score, tc.want)
			}
		})
	}
}

func TestFreshness_Cell_TrustScaling(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := defaultFreshnessSettings()
	meta, _ := json.Marshal(map[string]any{"confidence": "high"})
	updatedAt := time.Now().UTC()

	tests := []struct {
		trust int
		want  int
	}{
		{1, 20},
		{3, 60},
		{5, 100},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("trust-%d", tc.trust), func(t *testing.T) {
			t.Parallel()
			source := &DataSource{SourceKind: "official", TrustLevel: tc.trust}
			v := makeValue(t, updatedAt, string(meta))
			score, _, err := s.cellFreshness(t.Context(), v, source, settings)
			if err != nil {
				t.Fatalf("cellFreshness: %v", err)
			}
			if score != tc.want {
				t.Fatalf("score = %d, want %d", score, tc.want)
			}
		})
	}
}

func TestFreshness_Cell_MissingSourceAndUpdatedAt(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := defaultFreshnessSettings()
	v := gen.ComparisonValue{
		ItemID:      pgUUID(t, uuid.New().String()),
		DimensionID: pgUUID(t, uuid.New().String()),
		Metadata:    []byte("{}"),
	}

	score, isStale, err := s.cellFreshness(t.Context(), v, nil, settings)
	if err != nil {
		t.Fatalf("cellFreshness: %v", err)
	}
	if !isStale {
		t.Fatalf("expected stale value")
	}
	if score != 0 {
		t.Fatalf("score = %d, want 0", score)
	}
}

func TestFreshness_Cell_ConfidenceLevels(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := defaultFreshnessSettings()
	source := &DataSource{SourceKind: "official", TrustLevel: 5}
	updatedAt := time.Now().UTC()

	tests := []struct {
		confidence string
		want       int
	}{
		{"high", 100},
		{"medium", 75},
		{"low", 50},
	}

	for _, tc := range tests {
		t.Run(tc.confidence, func(t *testing.T) {
			t.Parallel()
			meta, _ := json.Marshal(map[string]any{"confidence": tc.confidence})
			v := makeValue(t, updatedAt, string(meta))
			score, _, err := s.cellFreshness(t.Context(), v, source, settings)
			if err != nil {
				t.Fatalf("cellFreshness: %v", err)
			}
			if score != tc.want {
				t.Fatalf("score = %d, want %d", score, tc.want)
			}
		})
	}
}

func TestFreshness_Cell_RetrievedAtFallback(t *testing.T) {
	t.Parallel()

	s := &Service{}
	settings := freshnessSettings{
		staleHours:       1,
		decayPerHour:     10,
		officialWeight:   100,
		thirdPartyWeight: 75,
		aiEstimateWeight: 50,
	}
	source := &DataSource{SourceKind: "official", TrustLevel: 5}

	retrievedAt := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	meta, _ := json.Marshal(map[string]any{"retrieved_at": retrievedAt, "confidence": "high"})
	v := gen.ComparisonValue{
		ItemID:      pgUUID(t, uuid.New().String()),
		DimensionID: pgUUID(t, uuid.New().String()),
		Metadata:    []byte(meta),
	}

	score, isStale, err := s.cellFreshness(t.Context(), v, source, settings)
	if err != nil {
		t.Fatalf("cellFreshness: %v", err)
	}
	if isStale {
		t.Fatalf("expected fresh value from retrieved_at")
	}
	// Within stale threshold, so score should be 100.
	if score != 100 {
		t.Fatalf("score = %d, want 100", score)
	}
}

func TestFreshnessForComparison(t *testing.T) {
	t.Parallel()

	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})

	svc := testFreshnessService(t, d)
	ctx := t.Context()

	res, err := svc.compSvc.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "Freshness Test",
		Body:       "A test body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		Depth:      comparison.DepthDetailed,
		Items: []comparison.JCEItemInput{
			{Title: "Item A", Position: 0, ItemRef: []byte("{}")},
			{Title: "Item B", Position: 1, ItemRef: []byte("{}")},
		},
		Dimensions: []string{"Cost", "Duration"},
	})
	if err != nil {
		t.Fatalf("CreateJCEComparison: %v", err)
	}

	matrix, err := svc.compSvc.GetJCEComparisonMatrix(ctx, res.ID, tenantID)
	if err != nil {
		t.Fatalf("GetJCEComparisonMatrix: %v", err)
	}
	if len(matrix) != 2 {
		t.Fatalf("want 2 matrix items, got %d", len(matrix))
	}
	if len(matrix[0].Dimensions) != 2 {
		t.Fatalf("want 2 dimensions, got %d", len(matrix[0].Dimensions))
	}

	// Create a high-trust official data source.
	dsStore := newDataSourceStore(d)
	ds, err := dsStore.create(ctx, tenantID, DataSourceInput{
		SourceName: "Official Registry",
		SourceKind: "official",
		TrustLevel: 5,
	})
	if err != nil {
		t.Fatalf("create data source: %v", err)
	}

	// Fill one cell with a value linked to the data source and high-confidence metadata.
	item0 := matrix[0].Item
	dim0 := matrix[0].Dimensions[0]
	meta, _ := json.Marshal(map[string]any{"source": "Official Registry", "confidence": "high"})
	dsID := stringToUUID(ds.ID)
	if _, err := svc.compSvc.UpsertComparisonValueWithMetadata(ctx, tenantID, comparison.UpsertComparisonValueInput{
		DimensionID:  dim0.DimensionID,
		ItemID:       item0.ID,
		Value:        "1000",
		DataSourceID: &dsID,
	}, meta); err != nil {
		t.Fatalf("UpsertComparisonValueWithMetadata: %v", err)
	}

	summary, err := svc.FreshnessForComparison(ctx, res.ID, tenantID, userID)
	if err != nil {
		t.Fatalf("FreshnessForComparison: %v", err)
	}

	if summary.ContentID != res.ID.String() {
		t.Fatalf("content_id = %q, want %q", summary.ContentID, res.ID.String())
	}
	if len(summary.ByItem) != 2 {
		t.Fatalf("by_item length = %d, want 2", len(summary.ByItem))
	}
	if summary.TotalCount != 4 {
		t.Fatalf("total_count = %d, want 4", summary.TotalCount)
	}
	if summary.StaleCount != 3 {
		t.Fatalf("stale_count = %d, want 3", summary.StaleCount)
	}

	// Overall should be the average of 100, 0, 0, 0 = 25.
	if summary.OverallFreshnessScore != 25 {
		t.Fatalf("overall score = %d, want 25", summary.OverallFreshnessScore)
	}

	// Item A has one filled cell (score 100) and one missing cell (score 0), average 50.
	itemA := summary.ByItem[0]
	if itemA.FreshnessScore != 50 {
		t.Fatalf("item A freshness score = %d, want 50", itemA.FreshnessScore)
	}
	if len(itemA.Cells) != 2 {
		t.Fatalf("item A cells = %d, want 2", len(itemA.Cells))
	}

	filled := itemA.Cells[0]
	if filled.FreshnessScore != 100 {
		t.Fatalf("filled cell score = %d, want 100", filled.FreshnessScore)
	}
	if filled.SourceKind != "official" {
		t.Fatalf("filled cell source_kind = %q, want official", filled.SourceKind)
	}
	if filled.SourceName != "Official Registry" {
		t.Fatalf("filled cell source_name = %q, want %q", filled.SourceName, "Official Registry")
	}
	if filled.IsStale {
		t.Fatalf("expected filled cell to be fresh")
	}

	missing := itemA.Cells[1]
	if missing.FreshnessScore != 0 {
		t.Fatalf("missing cell score = %d, want 0", missing.FreshnessScore)
	}
	if !missing.IsStale {
		t.Fatalf("expected missing cell to be stale")
	}

	// Item B has two missing cells, so its average should be 0.
	itemB := summary.ByItem[1]
	if itemB.FreshnessScore != 0 {
		t.Fatalf("item B freshness score = %d, want 0", itemB.FreshnessScore)
	}

	// Run again to ensure the cache path returns an equivalent summary.
	summary2, err := svc.FreshnessForComparison(ctx, res.ID, tenantID, userID)
	if err != nil {
		t.Fatalf("FreshnessForComparison second call: %v", err)
	}
	if summary2.OverallFreshnessScore != summary.OverallFreshnessScore {
		t.Fatalf("cached summary mismatch: %d vs %d", summary2.OverallFreshnessScore, summary.OverallFreshnessScore)
	}
}

func TestFreshnessForComparison_WithSourceMatch(t *testing.T) {
	t.Parallel()

	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})

	svc := testFreshnessService(t, d)
	ctx := t.Context()

	res, err := svc.compSvc.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "Freshness Match Test",
		Body:       "A test body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		Depth:      comparison.DepthDetailed,
		Items: []comparison.JCEItemInput{
			{Title: "Item A", Position: 0, ItemRef: []byte("{}")},
		},
		Dimensions: []string{"Cost"},
	})
	if err != nil {
		t.Fatalf("CreateJCEComparison: %v", err)
	}

	matrix, err := svc.compSvc.GetJCEComparisonMatrix(ctx, res.ID, tenantID)
	if err != nil {
		t.Fatalf("GetJCEComparisonMatrix: %v", err)
	}

	// Create a global data source that should match by source name.
	dsStore := newDataSourceStore(d)
	if _, err := dsStore.create(ctx, tenantID, DataSourceInput{
		SourceName: "Global Registry",
		SourceKind: "third_party",
		TrustLevel: 4,
	}); err != nil {
		t.Fatalf("create data source: %v", err)
	}

	item0 := matrix[0].Item
	dim0 := matrix[0].Dimensions[0]
	// Value has no data_source_id but metadata.source matches the global source.
	meta, _ := json.Marshal(map[string]any{"source": "Global Registry", "confidence": "high"})
	if _, err := svc.compSvc.UpsertComparisonValueWithMetadata(ctx, tenantID, comparison.UpsertComparisonValueInput{
		DimensionID: dim0.DimensionID,
		ItemID:      item0.ID,
		Value:       "1000",
	}, meta); err != nil {
		t.Fatalf("UpsertComparisonValueWithMetadata: %v", err)
	}

	summary, err := svc.FreshnessForComparison(ctx, res.ID, tenantID, userID)
	if err != nil {
		t.Fatalf("FreshnessForComparison: %v", err)
	}

	if summary.TotalCount != 1 {
		t.Fatalf("total_count = %d, want 1", summary.TotalCount)
	}

	cell := summary.ByItem[0].Cells[0]
	if cell.SourceName != "Global Registry" {
		t.Fatalf("source_name = %q, want Global Registry", cell.SourceName)
	}
	if cell.SourceKind != "third_party" {
		t.Fatalf("source_kind = %q, want third_party", cell.SourceKind)
	}

	// third_party weight 75, trust 4/5 => 75 * 0.8 = 60, high confidence, no estimate.
	want := int(math.Round(75.0 * 0.8))
	if cell.FreshnessScore != want {
		t.Fatalf("cell score = %d, want %d", cell.FreshnessScore, want)
	}
}
