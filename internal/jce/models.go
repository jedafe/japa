package jce

import "github.com/jackc/pgx/v5/pgtype"

// ProfileGoal is a single weighted goal inside a comparison profile.
type ProfileGoal struct {
	Key    string  `json:"key"`
	Weight float64 `json:"weight"`
}

// ProfileDemographics captures the user-specific context for scoring.
type ProfileDemographics struct {
	Age                 *int     `json:"age"`
	Occupation          string   `json:"occupation"`
	FamilyStatus        string   `json:"family_status"`
	LanguageProficiency string   `json:"language_proficiency"`
	BudgetMin           *float64 `json:"budget_min"`
	BudgetMax           *float64 `json:"budget_max"`
	BudgetCurrency      string   `json:"budget_currency"`
}

// ComparisonProfile is the domain representation of a comparison_profiles row.
type ComparisonProfile struct {
	ID             string `json:"id"`
	TenantID       string
	UserID         *string             `json:"user_id"`
	Title          string              `json:"title"`
	SourceSpaceID  *string             `json:"source_space_id"`
	TargetSpaceIDs []string            `json:"target_space_ids"`
	Goals          []ProfileGoal       `json:"goals"`
	Preferences    map[string]float64  `json:"preferences"`
	Demographics   ProfileDemographics `json:"demographics"`
	IsDefault      bool                `json:"is_default"`
	IsTemplate     bool                `json:"is_template"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

// ComparisonProfileInput is used to create or update a comparison profile.
type ComparisonProfileInput struct {
	Title          string              `json:"title"`
	SourceSpaceID  *string             `json:"source_space_id"`
	TargetSpaceIDs []string            `json:"target_space_ids"`
	Goals          []ProfileGoal       `json:"goals"`
	Preferences    map[string]float64  `json:"preferences"`
	Demographics   ProfileDemographics `json:"demographics"`
	IsDefault      bool                `json:"is_default"`
	IsTemplate     bool                `json:"is_template"`
}

// ScoreFactorConfig describes how a single factor is evaluated by the scoring engine.
// Ref: docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md §7.1
// Only a subset of fields applies to each data_source; unused fields are left zero.
type ScoreFactorConfig struct {
	Kind              string   `json:"kind"`
	Direction         string   `json:"direction"`
	Min               float64  `json:"min"`
	Max               float64  `json:"max"`
	Ideal             float64  `json:"ideal"`
	ValueKey          string   `json:"value_key"`
	ProfileKey        string   `json:"profile_key"`
	ItemKey           string   `json:"item_key"`
	MatchStrategy     string   `json:"match_strategy"`
	IdealTexts        []string `json:"ideal_texts"`
	MissingScore      float64  `json:"missing_score"`
	IsOutput          bool     `json:"is_output"`
	PenaltyIneligible *float64 `json:"penalty_ineligible,omitempty"`
	PenaltyLimited    *float64 `json:"penalty_limited,omitempty"`
}

// ScoreFactor is the domain representation of a comparison_score_factors row.
type ScoreFactor struct {
	ID               string `json:"id"`
	TenantID         string
	ComparisonTypeID *string           `json:"comparison_type_id"`
	FactorKey        string            `json:"factor_key"`
	Label            string            `json:"label"`
	Description      string            `json:"description"`
	DimensionSlug    *string           `json:"dimension_slug"`
	DataSource       string            `json:"data_source"`
	DefaultWeight    float64           `json:"default_weight"`
	Config           ScoreFactorConfig `json:"config"`
	Version          int               `json:"version"`
	IsSystem         bool              `json:"is_system"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

// ScoreFactorInput is used to create or update a score factor.
type ScoreFactorInput struct {
	ComparisonTypeID *string           `json:"comparison_type_id"`
	FactorKey        string            `json:"factor_key"`
	Label            string            `json:"label"`
	Description      string            `json:"description"`
	DimensionSlug    *string           `json:"dimension_slug"`
	DataSource       string            `json:"data_source"`
	DefaultWeight    float64           `json:"default_weight"`
	Config           ScoreFactorConfig `json:"config"`
	IsSystem         bool              `json:"is_system"`
}

// FactorScore is a single factor's contribution to a JapaScore.
type FactorScore struct {
	FactorKey    string  `json:"factor_key"`
	Label        string  `json:"label"`
	ScoreValue   float64 `json:"score_value"`
	MaxValue     float64 `json:"max_value"`
	Weight       float64 `json:"weight"`
	Explanation  string  `json:"explanation"`
	Missing      bool    `json:"missing"`
	MissingScore float64 `json:"missing_score"`
}

// JapaScore is the per-item result of the scoring engine.
type JapaScore struct {
	ItemID         string        `json:"item_id"`
	ItemTitle      string        `json:"item_title"`
	JapaScore      float64       `json:"japa_score"`
	MaxValue       float64       `json:"max_value"`
	EligibleStatus string        `json:"eligible_status"`
	FactorScores   []FactorScore `json:"factor_scores"`
	Explanation    string        `json:"explanation"`
}

// ComparisonRecommendation is a user-facing recommendation produced after scoring.
type ComparisonRecommendation struct {
	ID         string  `json:"id"`
	ItemID     *string `json:"item_id"`
	FactorID   *string `json:"factor_id"`
	Text       string  `json:"text"`
	ActionType *string `json:"action_type"`
	ActionURL  *string `json:"action_url"`
	Priority   int     `json:"priority"`
}

// ScoreResult is the cached output of a full comparison scoring run.
type ScoreResult struct {
	ContentID             string                     `json:"content_id"`
	ProfileID             *string                    `json:"profile_id"`
	SmartMode             string                     `json:"smart_mode"`
	Scores                []JapaScore                `json:"scores"`
	Recommendations       []ComparisonRecommendation `json:"recommendations"`
	WeightsSnapshot       map[string]float64         `json:"weights_snapshot"`
	RecommendationSummary map[string]any             `json:"recommendation_summary"`
	Version               int                        `json:"version"`
}

// ScoreInput triggers a scoring run for a comparison.
type ScoreInput struct {
	ContentID string `json:"content_id"`
	TenantID  string
	UserID    *string `json:"user_id"`
	ProfileID *string `json:"profile_id"`
	SmartMode string  `json:"smart_mode"`
	Force     bool    `json:"force"`
	// PublicPerm is the permission to use when checking whether a non-owner
	// can trigger or view this score. It defaults to jce.score.recalculate.
	PublicPerm string `json:"-"`
	// PermissionChecker is populated by the service/handler to enforce
	// jce.score.recalculate and comparison.edit_any. A nil checker is treated
	// as a deny-all fallback.
	PermissionChecker func(perm string) bool `json:"-"`
}

// GetScoreInput fetches a previously computed score for a comparison.
type GetScoreInput struct {
	ContentID string `json:"content_id"`
	TenantID  string
	UserID    *string `json:"user_id"`
	ProfileID *string `json:"profile_id"`
	// PublicPerm is the permission to use when checking whether a non-owner
	// can view this score on public content. It defaults to jce.score.view.
	PublicPerm string `json:"-"`
	// PermissionChecker is populated by the service/handler to enforce
	// jce.score.view and comparison.edit_any. A nil checker is treated as a
	// deny-all fallback.
	PermissionChecker func(perm string) bool `json:"-"`
}

// ScorePreviewInput scores a list of item refs without creating a comparison.
// TenantID and UserID are not exposed to JSON clients; the handler/service must
// set them from the request context before calling the engine.
type ScorePreviewInput struct {
	TenantID     string                  `json:"-"`
	UserID       *string                 `json:"-"`
	TypeID       string                  `json:"type_id"`
	SmartMode    string                  `json:"smart_mode"`
	ItemRefs     []map[string]any        `json:"item_refs"`
	ProfileInput *ComparisonProfileInput `json:"profile"`
	// PermissionChecker is populated by the service/handler to enforce
	// jce.score.view and comparison.edit_any. A nil checker is treated as a
	// deny-all fallback.
	PermissionChecker func(perm string) bool `json:"-"`
}

// ProfileStat is a per-source-space count of non-template profiles.
type ProfileStat struct {
	SourceSpaceID   string `json:"source_space_id"`
	ProfileCount    int32  `json:"profile_count"`
	SourceSpaceName string `json:"source_space_name"`
	SourceSpaceSlug string `json:"source_space_slug"`
}

// nullable helpers used by the stores.

// uuidPtr returns a *string for a valid pgtype.UUID, or nil when invalid.
func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}

	s := u.String()

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

// DataSource is a tenant-wide registry of authoritative comparison value feeds.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §7.1.
type DataSource struct {
	ID             string `json:"id"`
	TenantID       string
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
	ID             string `json:"id"`
	TenantID       string
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

// AdvisorInput is the common request shape for the JCE AI advisor.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §10.2.
type AdvisorInput struct {
	ContentID  pgtype.UUID `json:"content_id"`
	TenantID   pgtype.UUID
	UserID     pgtype.UUID  `json:"user_id"`
	ProfileID  *pgtype.UUID `json:"profile_id,omitempty"`
	Question   string       `json:"question"`
	ItemID     *pgtype.UUID `json:"item_id,omitempty"`
	FactorKey  string       `json:"factor_key"`
	SourceKind string       `json:"source_kind"`
}

// AdvisorResult is the output of the JCE AI advisor.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §10.2.
type AdvisorResult struct {
	ContentID      string            `json:"content_id"`
	ProfileID      *string           `json:"profile_id,omitempty"`
	Summary        string            `json:"summary"`
	Ranking        []AdvisorRankItem `json:"ranking,omitempty"`
	NextActions    []NextAction      `json:"next_actions,omitempty"`
	Citations      []string          `json:"citations,omitempty"`
	ProviderName   string            `json:"provider_name"`
	ProviderRef    string            `json:"provider_ref"`
	FreshnessScore int               `json:"freshness_score"`
}

// AdvisorRankItem is a single ranked item in an advisor result.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §10.2.
type AdvisorRankItem struct {
	ItemID    string  `json:"item_id"`
	ItemTitle string  `json:"item_title"`
	Rank      int     `json:"rank"`
	JapaScore float64 `json:"japa_score"`
	Reasoning string  `json:"reasoning"`
	Eligible  bool    `json:"eligible"`
}

// NextAction is a user-facing recommended next step.
// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §10.2.
type NextAction struct {
	ItemID     string `json:"item_id,omitempty"`
	Text       string `json:"text"`
	ActionType string `json:"action_type,omitempty"`
	ActionURL  string `json:"action_url,omitempty"`
	Priority   int    `json:"priority"`
}
