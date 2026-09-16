// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §8
package jce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// freshnessSettings groups the freshness-related system_settings with safe defaults.
type freshnessSettings struct {
	staleHours       int
	decayPerHour     int
	officialWeight   int
	thirdPartyWeight int
	aiEstimateWeight int
	cacheTTLMinutes  int
}

// loadFreshnessSettings loads the freshness-related system_settings for the platform.
func (s *Service) loadFreshnessSettings(ctx context.Context) freshnessSettings {
	if s.settings == nil {
		return defaultFreshnessSettings()
	}

	return freshnessSettings{
		staleHours:       s.settings.GetIntOr(ctx, "jce.freshness_stale_hours", 168),
		decayPerHour:     s.settings.GetIntOr(ctx, "jce.freshness_decay_per_hour", 2),
		officialWeight:   s.settings.GetIntOr(ctx, "jce.freshness_official_weight", 100),
		thirdPartyWeight: s.settings.GetIntOr(ctx, "jce.freshness_third_party_weight", 75),
		aiEstimateWeight: s.settings.GetIntOr(ctx, "jce.freshness_ai_estimate_weight", 50),
		cacheTTLMinutes:  s.settings.GetIntOr(ctx, "jce.freshness_cache_ttl_minutes", 10),
	}
}

func defaultFreshnessSettings() freshnessSettings {
	return freshnessSettings{
		staleHours:       168,
		decayPerHour:     2,
		officialWeight:   100,
		thirdPartyWeight: 75,
		aiEstimateWeight: 50,
		cacheTTLMinutes:  10,
	}
}

// freshnessCacheKey returns the Valkey cache key for a comparison freshness summary.
func freshnessCacheKey(contentID pgtype.UUID) string {
	return fmt.Sprintf("jc:jce:freshness::%s", contentID.String())
}

// cellFreshness computes a 0–100 freshness score and stale flag for a single comparison value.
// // The score is based on the Sub-spec 4 formula:
// //	base = source_kind_weight / 100
// trust = trust_level / 5
// age_hours = max(0, now - updated_at).hours - stale_hours
// decay = max(0, age_hours) * decay_per_hour
// confidence = 1.0 (high), 0.75 (medium or missing), 0.5 (low)
// estimate_penalty = 0.85 if is_estimate else 1.0
// freshness_score = clamp(0, 100, (100 - decay) * base * trust * confidence * estimate_penalty)
func (s *Service) cellFreshness(
	ctx context.Context,
	v gen.ComparisonValue,
	source *DataSource,
	settings freshnessSettings,
) (int, bool, error) {
	_ = ctx

	now := time.Now().UTC()

	kindWeight := settings.officialWeight

	trustLevel := 3
	if source != nil {
		trustLevel = source.TrustLevel

		switch source.SourceKind {
		case "third_party":
			kindWeight = settings.thirdPartyWeight
		case "ai_estimate":
			kindWeight = settings.aiEstimateWeight
		}
	}

	// Effective update time. Prefer updated_at, then created_at, then metadata.retrieved_at.
	updatedAt := time.Time{}

	switch {
	case v.UpdatedAt.Valid && !v.UpdatedAt.Time.IsZero():
		updatedAt = v.UpdatedAt.Time
	case v.CreatedAt.Valid && !v.CreatedAt.Time.IsZero():
		updatedAt = v.CreatedAt.Time
	}

	var md map[string]any
	if len(v.Metadata) > 0 && string(v.Metadata) != "null" {
		_ = json.Unmarshal(v.Metadata, &md)
	}

	if !v.UpdatedAt.Valid && updatedAt.IsZero() && md != nil {
		if r, ok := md["retrieved_at"].(string); ok && r != "" {
			if t, err := time.Parse(time.RFC3339, r); err == nil && !t.IsZero() {
				updatedAt = t
			}
		}
	}

	ageHours := now.Sub(updatedAt).Hours() - float64(settings.staleHours)
	if ageHours < 0 {
		ageHours = 0
	}

	decay := ageHours * float64(settings.decayPerHour)

	confidence := 0.75

	if c, ok := md["confidence"].(string); ok {
		switch c {
		case "high":
			confidence = 1.0
		case "medium":
			confidence = 0.75
		case "low":
			confidence = 0.5
		}
	}

	penalty := 1.0
	if est, ok := md["is_estimate"].(bool); ok && est {
		penalty = 0.85
	}

	base := float64(kindWeight) / 100.0
	trust := float64(trustLevel) / 5.0
	score := (100.0 - decay) * base * trust * confidence * penalty

	if score < 0 {
		score = 0
	}

	if score > 100 {
		score = 100
	}

	isStale := now.Sub(updatedAt).Hours() > float64(settings.staleHours)

	return int(math.Round(score)), isStale, nil
}

// FreshnessForComparison computes the freshness summary for a comparison.
// // It loads the comparison matrix, the raw comparison_values rows, and the platform's
// data sources, resolves a source for each cell, and aggregates per-item and overall
// scores. The result is cached in Valkey under jc:jce:freshness:{}:{content_id}.
func (s *Service) FreshnessForComparison(ctx context.Context, contentID, userID pgtype.UUID) (FreshnessSummary, error) {
	// Load the comparison and enforce visibility before reading cache or matrix.
	cmp, err := s.compSvc.GetJCEComparison(ctx, contentID.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FreshnessSummary{}, fmt.Errorf("%w: comparison not found", errpkg.ErrNotFound)
		}

		return FreshnessSummary{}, fmt.Errorf("loading comparison: %w", err)
	}

	if !s.canViewJCEComparison(ctx, cmp, userID) {
		return FreshnessSummary{}, fmt.Errorf("%w: cannot access comparison", errpkg.ErrForbidden)
	}

	// Try Valkey cache next.
	if s.vk != nil {
		data, err := s.vk.Get(ctx, freshnessCacheKey(contentID)).Bytes()
		if err == nil && len(data) > 0 {
			var cached FreshnessSummary
			if err := json.Unmarshal(data, &cached); err == nil {
				return cached, nil
			}
		}
	}

	settings := s.loadFreshnessSettings(ctx)

	matrix, err := s.compSvc.GetJCEComparisonMatrix(ctx, contentID)
	if err != nil {
		return FreshnessSummary{}, fmt.Errorf("loading comparison matrix: %w", err)
	}

	values, err := gen.New(s.db.Reader).GetComparisonValuesByContent(ctx,
		contentID,
	)
	if err != nil {
		return FreshnessSummary{}, fmt.Errorf("loading comparison values: %w", err)
	}

	type valueKey struct {
		itemID      [16]byte
		dimensionID [16]byte
	}

	valueIndex := make(map[valueKey]gen.ComparisonValue, len(values))
	for _, v := range values {
		valueIndex[valueKey{v.ItemID.Bytes, v.DimensionID.Bytes}] = v
	}

	sources, _, err := s.dataSources.list(ctx, nil, nil, 0, 0)
	if err != nil {
		return FreshnessSummary{}, fmt.Errorf("loading data sources: %w", err)
	}

	sourceByID := make(map[string]DataSource, len(sources))
	for _, ds := range sources {
		sourceByID[ds.ID] = ds
	}

	summary := FreshnessSummary{
		ContentID: contentID.String(),
	}

	var totalScore int

	var totalCells int

	for _, item := range matrix {
		entityType, entityID, _ := resolveItemEntityFromRef(item.Item.ItemRef)
		freshItem := FreshnessItem{
			ItemID:    item.Item.ID.String(),
			ItemTitle: item.Item.Title,
		}

		for _, dim := range item.Dimensions {
			key := valueKey{item.Item.ID.Bytes, dim.DimensionID.Bytes}

			v, ok := valueIndex[key]
			if !ok {
				v = gen.ComparisonValue{
					ItemID:      item.Item.ID,
					DimensionID: dim.DimensionID,

					Metadata:     []byte("{}"),
					UpdatedAt:    pgtype.Timestamptz{},
					CreatedAt:    pgtype.Timestamptz{},
					DataSourceID: pgtype.UUID{},
				}
			}

			var md map[string]any
			if len(v.Metadata) > 0 && string(v.Metadata) != "null" {
				_ = json.Unmarshal(v.Metadata, &md)
			}

			sourceName := ""
			if sn, ok := md["source"].(string); ok {
				sourceName = sn
			}

			var source *DataSource

			if v.DataSourceID.Valid {
				if ds, ok := sourceByID[v.DataSourceID.String()]; ok {
					source = &ds
				}
			}

			if source == nil {
				// The matrix projection does not include comparison_dimensions.slug,
				// so we use the dimension label as the slug for source matching.
				// MatchDataSource falls back through scoped to global passes.
				matched, err := s.MatchDataSource(ctx, entityType, entityID, dim.Label, sourceName, nil)
				if err == nil && matched != nil {
					source = matched
				}
			}

			score, isStale, err := s.cellFreshness(ctx, v, source, settings)
			if err != nil {
				return FreshnessSummary{}, fmt.Errorf("computing cell freshness: %w", err)
			}

			if isStale {
				summary.StaleCount++
			}

			cell := FreshnessCell{
				DimensionID:    dim.DimensionID.String(),
				Label:          dim.Label,
				FreshnessScore: score,
				SourceName:     sourceName,
				SourceKind:     "",
				RetrievedAt:    valueRetrievedAt(v, md),
				IsStale:        isStale,
			}
			if source != nil {
				cell.SourceName = source.SourceName
				cell.SourceKind = source.SourceKind
			} else if sk, ok := md["source_kind"].(string); ok && cell.SourceKind == "" {
				cell.SourceKind = sk
			}

			freshItem.Cells = append(freshItem.Cells, cell)
			freshItem.FreshnessScore += score
			totalScore += score
			totalCells++
		}

		if len(freshItem.Cells) > 0 {
			freshItem.FreshnessScore = int(math.Round(float64(freshItem.FreshnessScore) / float64(len(freshItem.Cells))))
		}

		summary.ByItem = append(summary.ByItem, freshItem)
	}

	if totalCells > 0 {
		summary.OverallFreshnessScore = int(math.Round(float64(totalScore) / float64(totalCells)))
	}

	summary.TotalCount = totalCells

	// Cache the computed summary.
	if s.vk != nil {
		if b, err := json.Marshal(summary); err == nil {
			ttl := time.Duration(settings.cacheTTLMinutes) * time.Minute
			_ = s.vk.Set(ctx, freshnessCacheKey(contentID), b, ttl).Err()
		}
	}

	return summary, nil
}

// valueRetrievedAt returns a display timestamp for a cell, preferring updated_at
// then metadata.retrieved_at then created_at.
func valueRetrievedAt(v gen.ComparisonValue, md map[string]any) *string {
	if v.UpdatedAt.Valid && !v.UpdatedAt.Time.IsZero() {
		s := v.UpdatedAt.Time.UTC().Format(time.RFC3339)
		return &s
	}

	if r, ok := md["retrieved_at"].(string); ok && r != "" {
		return &r
	}

	if v.CreatedAt.Valid && !v.CreatedAt.Time.IsZero() {
		s := v.CreatedAt.Time.UTC().Format(time.RFC3339)
		return &s
	}

	return nil
}
