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
	ID             string              `json:"id"`
	TenantID       string              `json:"tenant_id"`
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
	PenaltyIneligible float64  `json:"penalty_ineligible"`
	PenaltyLimited    float64  `json:"penalty_limited"`
}

// ScoreFactor is the domain representation of a comparison_score_factors row.
type ScoreFactor struct {
	ID               string            `json:"id"`
	TenantID         string            `json:"tenant_id"`
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
	FactorKey   string  `json:"factor_key"`
	Label       string  `json:"label"`
	ScoreValue  float64 `json:"score_value"`
	MaxValue    float64 `json:"max_value"`
	Weight      float64 `json:"weight"`
	Explanation string  `json:"explanation"`
	Missing     bool    `json:"missing"`
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
	ContentID string  `json:"content_id"`
	TenantID  string  `json:"tenant_id"`
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
	ContentID string  `json:"content_id"`
	TenantID  string  `json:"tenant_id"`
	UserID    *string `json:"user_id"`
	ProfileID *string `json:"profile_id"`
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
	SourceSpaceID string `json:"source_space_id"`
	ProfileCount  int32  `json:"profile_count"`
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
