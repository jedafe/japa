package jce

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// nullable helpers used by the stores.

// uuidPtr returns a *string for a valid pgtype.UUID, or nil when invalid.
func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}

	s := u.String()

	return &s
}

// timestamptzPtr returns a *string RFC3339 timestamp for a valid
// pgtype.Timestamptz, or nil when invalid.
func timestamptzPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}

	s := t.Time.Format(time.RFC3339)

	return &s
}

// ptrString dereferences a *string safely, returning "" for nil.
func ptrString(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}

// ptrToNullString returns a non-empty string as a pointer, or nil.
func ptrToNullString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// DataSource is a platform-wide registry of authoritative comparison value feeds.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §7.1.
type DataSource struct {
	ID string `json:"id"`

	EntityType     *string        `json:"entity_type,omitempty"`
	EntityID       *string        `json:"entity_id,omitempty"`
	DimensionSlug  *string        `json:"dimension_slug,omitempty"`
	SourceName     string         `json:"source_name"`
	SourceURL      *string        `json:"source_url,omitempty"`
	SourceKind     string         `json:"source_kind"` // official|third_party|ai_estimate
	TrustLevel     int            `json:"trust_level"`
	FreshnessScore int            `json:"freshness_score"`
	LastVerifiedAt *string        `json:"last_verified_at,omitempty"`
	Config         map[string]any `json:"config"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

// DataSourceInput is used to create or update a comparison data source.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §7.4.
type DataSourceInput struct {
	EntityType    *string        `json:"entity_type,omitempty"`
	EntityID      *string        `json:"entity_id,omitempty"`
	DimensionSlug *string        `json:"dimension_slug,omitempty"`
	SourceName    string         `json:"source_name"`
	SourceURL     *string        `json:"source_url,omitempty"`
	SourceKind    string         `json:"source_kind"`
	TrustLevel    int            `json:"trust_level"`
	Config        map[string]any `json:"config"`
}

// IngestionJob tracks one refresh run for a comparison.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §7.5.
type IngestionJob struct {
	ID string `json:"id"`

	ContentID      string  `json:"content_id"`
	Status         string  `json:"status"`
	SourceKind     *string `json:"source_kind,omitempty"`
	RefreshedCount int     `json:"refreshed_count"`
	Error          *string `json:"error,omitempty"`
	StartedAt      *string `json:"started_at,omitempty"`
	CompletedAt    *string `json:"completed_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// FreshnessCell is the freshness status for a single comparison value.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §8.4.
type FreshnessCell struct {
	DimensionID    string  `json:"dimension_id"`
	Label          string  `json:"label"`
	FreshnessScore int     `json:"freshness_score"`
	SourceName     string  `json:"source_name,omitempty"`
	SourceKind     string  `json:"source_kind,omitempty"`
	RetrievedAt    *string `json:"retrieved_at,omitempty"`
	IsStale        bool    `json:"is_stale"`
}

// FreshnessItem is the per-item freshness aggregation.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §8.4.
type FreshnessItem struct {
	ItemID         string          `json:"item_id"`
	ItemTitle      string          `json:"item_title"`
	FreshnessScore int             `json:"freshness_score"`
	Cells          []FreshnessCell `json:"cells"`
}

// FreshnessSummary is the top-level freshness report for a comparison.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §8.4.
type FreshnessSummary struct {
	ContentID             string          `json:"content_id"`
	OverallFreshnessScore int             `json:"overall_freshness_score"`
	StaleCount            int             `json:"stale_count"`
	TotalCount            int             `json:"total_count"`
	ByItem                []FreshnessItem `json:"by_item"`
}
