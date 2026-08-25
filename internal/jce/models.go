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
	ID             string
	TenantID       string
	UserID         *string
	Title          string
	SourceSpaceID  *string
	TargetSpaceIDs []string
	Goals          []ProfileGoal
	Preferences    map[string]float64
	Demographics   ProfileDemographics
	IsDefault      bool
	IsTemplate     bool
	CreatedAt      string
	UpdatedAt      string
}

// ComparisonProfileInput is used to create or update a comparison profile.
type ComparisonProfileInput struct {
	Title          string
	SourceSpaceID  *string
	TargetSpaceIDs []string
	Goals          []ProfileGoal
	Preferences    map[string]float64
	Demographics   ProfileDemographics
	IsDefault      bool
	IsTemplate     bool
}

// ScoreFactorConfig describes how a single factor is evaluated by the scoring engine.
// Ref: docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md §7.1
// Only a subset of fields applies to each data_source; unused fields are left zero.
type ScoreFactorConfig struct {
	Kind          string   `json:"kind"`
	Direction     string   `json:"direction"`
	Min           float64  `json:"min"`
	Max           float64  `json:"max"`
	ValueKey      string   `json:"value_key"`
	ProfileKey    string   `json:"profile_key"`
	ItemKey       string   `json:"item_key"`
	MatchStrategy string   `json:"match_strategy"`
	IdealTexts    []string `json:"ideal_texts"`
	MissingScore  float64  `json:"missing_score"`
	IsOutput      bool     `json:"is_output"`
}

// ScoreFactor is the domain representation of a comparison_score_factors row.
type ScoreFactor struct {
	ID               string
	TenantID         string
	ComparisonTypeID *string
	FactorKey        string
	Label            string
	Description      string
	DimensionSlug    *string
	DataSource       string
	DefaultWeight    float64
	Config           ScoreFactorConfig
	Version          int
	IsSystem         bool
	CreatedAt        string
	UpdatedAt        string
}

// ScoreFactorInput is used to create or update a score factor.
type ScoreFactorInput struct {
	ComparisonTypeID *string
	FactorKey        string
	Label            string
	Description      string
	DimensionSlug    *string
	DataSource       string
	DefaultWeight    float64
	Config           ScoreFactorConfig
	IsSystem         bool
}

// FactorScore is a single factor's contribution to a JapaScore.
type FactorScore struct {
	FactorKey   string
	Label       string
	ScoreValue  float64
	MaxValue    float64
	Weight      float64
	Explanation string
}

// JapaScore is the per-item result of the scoring engine.
type JapaScore struct {
	ItemID         string
	ItemTitle      string
	JapaScore      float64
	MaxValue       float64
	EligibleStatus string
	FactorScores   []FactorScore
	Explanation    string
}

// ComparisonRecommendation is a user-facing recommendation produced after scoring.
type ComparisonRecommendation struct {
	ID         string
	ItemID     *string
	Text       string
	ActionType *string
	ActionURL  *string
	Priority   int
}

// ScoreResult is the cached output of a full comparison scoring run.
type ScoreResult struct {
	ContentID             string
	ProfileID             *string
	SmartMode             string
	Scores                []JapaScore
	Recommendations       []ComparisonRecommendation
	WeightsSnapshot       map[string]float64
	RecommendationSummary map[string]any
	Version               int
}

// ScoreInput triggers a scoring run for a comparison.
type ScoreInput struct {
	ContentID string
	TenantID  string
	UserID    *string
	ProfileID *string
	SmartMode string
	Force     bool
}

// GetScoreInput fetches a previously computed score for a comparison.
type GetScoreInput struct {
	ContentID string
	TenantID  string
	UserID    *string
	ProfileID *string
}

// ScorePreviewInput scores a list of item refs without creating a comparison.
type ScorePreviewInput struct {
	TypeID       string
	SmartMode    string
	ItemRefs     []map[string]any
	ProfileInput *ComparisonProfileInput
}

// ProfileStat is a per-source-space count of non-template profiles.
type ProfileStat struct {
	SourceSpaceID string
	ProfileCount  int32
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
