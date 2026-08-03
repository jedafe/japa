// Package assessment — service.go holds the business logic for the Japa
// Readiness Assessment: form retrieval, submission, the server-side scoring
// engine, roadmap generation, and admin stats. It reads assessment.* system
// settings via the hot-reloadable config.Settings and enqueues Asynq tasks
// (assessment:score_submission, wallet:award_points) fire-and-forget.
//
// Ref: japaconnect-engineering/sections/71-readiness-assessment.md
//      japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md
package assessment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Task type constants — must match the Section 11 Asynq task catalog.
const (
	TaskScoreSubmission = "assessment:score_submission"
	TaskAwardPoints     = "wallet:award_points"
)

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// The concrete enqueuer satisfies this.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// ScoringConfig is the JSONB shape stored in forms.scoring_config.
// It defines the max possible score, threshold bands for classification,
// and per-dimension weights used for aggregation.
type ScoringConfig struct {
	MaxScore         int                `json:"max_score"`
	Thresholds       []Threshold        `json:"thresholds"`
	DimensionWeights map[string]float64 `json:"dimension_weights"`
}

// Threshold is a single band in the scoring_config.thresholds array.
// A total score falling within [Min, Max] (inclusive) is classified with
// Label and rendered with Color.
type Threshold struct {
	Min   int    `json:"min"`
	Max   int    `json:"max"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// FieldScoringConfig is the JSONB shape stored in custom_fields.config for
// assessment fields. Weight scales the field's contribution, Dimension groups
// it with related fields, and OptionScores maps option values to raw scores.
type FieldScoringConfig struct {
	Weight       float64        `json:"weight"`
	Dimension    string         `json:"dimension"`
	OptionScores map[string]int `json:"option_scores"`
}

// RoadmapStep is a single recommended next step in the generated roadmap.
// ContentSlug links to a platform content item (guide, tool, learning track).
type RoadmapStep struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ContentSlug string `json:"content_slug"`
	Dimension   string `json:"dimension"`
	Priority    int    `json:"priority"`
}

// ScoreResult is the output of the scoring engine before persistence.
type ScoreResult struct {
	TotalScore      int            `json:"total_score"`
	DimensionScores map[string]int `json:"dimension_scores"`
	ThresholdLabel  string         `json:"threshold_label"`
	Roadmap         []RoadmapStep  `json:"roadmap"`
}

// AssessmentFormDTO is the active assessment form with its scoring_config and
// fields, ready for the scoring engine or for public display.
type AssessmentFormDTO struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	Description   string            `json:"description"`
	IsMultiStep   bool              `json:"is_multi_step"`
	SubmitLabel   string            `json:"submit_label"`
	Status        string            `json:"status"`
	ScoringConfig ScoringConfig     `json:"scoring_config"`
	Fields        []AssessmentField `json:"fields"`
}

// AssessmentField is a single field on the assessment form, with its scoring
// config resolved from the config JSONB column.
type AssessmentField struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	Key       string             `json:"key"`
	FieldType string             `json:"field_type"`
	Required  bool               `json:"is_required"`
	Position  int32              `json:"position"`
	Config    FieldScoringConfig `json:"config"`
}

// Service holds the business logic for the Japa Readiness Assessment.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates an assessment service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetEnqueuer injects the Asynq task enqueuer for scoring and points tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// GetActiveAssessment returns the active assessment form with its scoring_config
// and fields. Returns an error if no active assessment form exists for the
// tenant.
func (s *Service) GetActiveAssessment(ctx context.Context, tenantID pgtype.UUID) (*AssessmentFormDTO, error) {
	form, err := s.store.GetActiveAssessmentForm(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("assessment: get active: %w", err)
	}

	fields, err := s.store.GetAssessmentFormFields(ctx, tenantID, form.ID)
	if err != nil {
		return nil, fmt.Errorf("assessment: get fields: %w", err)
	}

	return formToDTO(form, fields), nil
}

// SubmitAssessment creates a form_submission for the assessment and enqueues
// the assessment:score_submission task fire-and-forget (Rule 8). For anonymous
// users (userID is nil), the submission is created without a user_id.
// Returns the submission ID.
func (s *Service) SubmitAssessment(ctx context.Context, tenantID pgtype.UUID, userID *uuid.UUID, formID pgtype.UUID, fieldValues json.RawMessage) (pgtype.UUID, error) {
	var userIDVal pgtype.UUID
	if userID != nil {
		userIDVal = pgtype.UUID{Bytes: *userID, Valid: true}
	}

	sub, err := s.store.CreateAssessmentSubmission(ctx, formID, tenantID, userIDVal, []byte(fieldValues))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: submit: %w", err)
	}

	// Enqueue scoring task fire-and-forget (Rule 8). A failure to enqueue
	// does not fail the submission — the result will be scored on a later
	// retry or admin re-score.
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"submission_id": sub.ID.String(),
			"tenant_id":     tenantID.String(),
		})
		if err != nil {
			slog.Warn("assessment: submit: marshal payload failed", "error", err)
		} else if err := s.enqueuer.Enqueue(TaskScoreSubmission, payload); err != nil {
			slog.Warn("assessment: submit: enqueue scoring task failed", "error", err)
		}
	}

	return sub.ID, nil
}

// ScoreSubmissionInput is the input to the scoring engine when the caller
// has already fetched the submission's field_values. The worker handler
// uses this to avoid a second query for the submission row.
type ScoreSubmissionInput struct {
	SubmissionID pgtype.UUID
	UserID       pgtype.UUID
	FieldValues  json.RawMessage
}

// ScoreSubmission is the scoring engine. It reads the form's scoring_config
// and fields, resolves per-answer scores from each field's config
// (option_scores map + weight), aggregates dimension scores, classifies
// against scoring_config.thresholds, generates roadmap_content, and writes
// the assessment_results row. This is called by the worker task handler.
//
// The caller provides the submission's field_values and user_id via
// ScoreSubmissionInput. The worker handler fetches the submission row from
// the shared form_submissions store and passes its field_values here.
func (s *Service) ScoreSubmission(ctx context.Context, tenantID pgtype.UUID, input ScoreSubmissionInput) (pgtype.UUID, error) {
	form, err := s.store.GetActiveAssessmentForm(ctx, tenantID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: get form: %w", err)
	}

	fields, err := s.store.GetAssessmentFormFields(ctx, tenantID, form.ID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: get fields: %w", err)
	}

	scoringConfig, err := parseScoringConfig(form.ScoringConfig)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: parse scoring config: %w", err)
	}

	result := computeScore(input.FieldValues, fields, scoringConfig)

	dimensionScoresJSON, err := json.Marshal(result.DimensionScores)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: marshal dimension scores: %w", err)
	}

	roadmapJSON, err := json.Marshal(result.Roadmap)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: marshal roadmap: %w", err)
	}

	ar, err := s.store.CreateAssessmentResult(ctx, tenantID, input.UserID, input.SubmissionID, int32(result.TotalScore), dimensionScoresJSON, result.ThresholdLabel, roadmapJSON)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: create result: %w", err)
	}

	// Award completion points if configured (Section 28 — Wallet/Credits).
	if s.settings != nil && input.UserID.Valid {
		points := s.settings.GetIntOr(ctx, tenantID, "assessment.completion_points", 50)
		if points > 0 && s.enqueuer != nil {
			payload, err := json.Marshal(map[string]any{
				"user_id":      input.UserID.String(),
				"tenant_id":    tenantID.String(),
				"points":       points,
				"reason":       "assessment_completion",
				"reference_id": ar.ID.String(),
			})
			if err != nil {
				slog.Warn("assessment: score: marshal points payload failed", "error", err)
			} else if err := s.enqueuer.Enqueue(TaskAwardPoints, payload); err != nil {
				slog.Warn("assessment: score: enqueue award points failed", "error", err)
			}
		}
	}

	slog.Info("assessment: score: complete",
		"tenant_id", tenantID.String(),
		"submission_id", input.SubmissionID.String(),
		"result_id", ar.ID.String(),
		"total_score", result.TotalScore,
		"threshold", result.ThresholdLabel)

	return ar.ID, nil
}

// GetResult returns a single assessment result with its roadmap.
func (s *Service) GetResult(ctx context.Context, tenantID, resultID pgtype.UUID) (gen.AssessmentResult, error) {
	return s.store.GetAssessmentResultByID(ctx, tenantID, resultID)
}

// ListHistory returns a user's past assessment results, paginated.
func (s *Service) ListHistory(ctx context.Context, tenantID, userID pgtype.UUID, page, limit int32) ([]gen.AssessmentResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	results, err := s.store.ListUserAssessmentResults(ctx, tenantID, userID, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: list history: %w", err)
	}

	total, err := s.store.CountUserAssessmentResults(ctx, tenantID, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: count history: %w", err)
	}

	return results, total, nil
}

// AdminListResults returns all users' assessment results, paginated and
// optionally filtered by score range. minScore/maxScore of 0 means no filter.
func (s *Service) AdminListResults(ctx context.Context, tenantID pgtype.UUID, minScore, maxScore, page, limit int32) ([]AdminListAssessmentResultsRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	results, err := s.store.AdminListAssessmentResults(ctx, tenantID, minScore, maxScore, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: admin list: %w", err)
	}

	total, err := s.store.AdminCountAssessmentResults(ctx, tenantID, minScore, maxScore)
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: admin count: %w", err)
	}

	return results, total, nil
}

// AdminScoreDistribution returns the count and average score of results
// grouped by threshold label.
func (s *Service) AdminScoreDistribution(ctx context.Context, tenantID pgtype.UUID) ([]AdminScoreDistributionRow, error) {
	return s.store.AdminScoreDistribution(ctx, tenantID)
}

// AdminCompletionStats returns total submissions, total scored results, and
// the average score across all assessment submissions.
func (s *Service) AdminCompletionStats(ctx context.Context, tenantID pgtype.UUID) (AdminCompletionStatsRow, error) {
	return s.store.AdminCompletionStats(ctx, tenantID)
}

// UpdateFormScoring updates the scoring_config JSONB on the assessment form.
// Admin-only.
func (s *Service) UpdateFormScoring(ctx context.Context, tenantID, formID pgtype.UUID, scoringConfig json.RawMessage) (gen.Form, error) {
	// Validate the JSONB is a well-formed ScoringConfig before persisting.
	var cfg ScoringConfig
	if err := json.Unmarshal(scoringConfig, &cfg); err != nil {
		return gen.Form{}, fmt.Errorf("assessment: update scoring: invalid config: %w", err)
	}
	return s.store.UpdateAssessmentFormScoring(ctx, tenantID, formID, []byte(scoringConfig))
}

// ---------------------------------------------------------------------------
// Scoring engine helpers
// ---------------------------------------------------------------------------

// computeScore resolves per-answer scores from field config, aggregates
// dimension scores, classifies against thresholds, and generates the roadmap.
// This is the pure scoring function — no I/O.
func computeScore(fieldValues json.RawMessage, fields []gen.CustomField, scoringConfig ScoringConfig) ScoreResult {
	// Parse the field_values snapshot: {"field_key": "value", ...}
	var answers map[string]any
	if err := json.Unmarshal(fieldValues, &answers); err != nil {
		slog.Warn("assessment: computeScore: unmarshal field_values failed", "error", err)
		answers = map[string]any{}
	}

	// Build a field-key → field lookup with parsed scoring config.
	fieldByKey := make(map[string]gen.CustomField, len(fields))
	for _, f := range fields {
		fieldByKey[f.Key] = f
	}

	// Aggregate raw scores per dimension and track the max possible per
	// dimension for normalisation.
	dimensionRaw := make(map[string]float64)
	dimensionMaxRaw := make(map[string]float64)

	for key, answer := range answers {
		field, ok := fieldByKey[key]
		if !ok {
			continue
		}
		fsc := parseFieldScoringConfig(field.Config)
		if fsc.Dimension == "" {
			continue
		}

		answerStr := fmt.Sprintf("%v", answer)
		rawScore, ok := fsc.OptionScores[answerStr]
		if !ok {
			continue
		}

		weighted := float64(rawScore) * fsc.Weight
		dimensionRaw[fsc.Dimension] += weighted

		// Track the max possible raw score for this dimension (the highest
		// option score × weight) for normalisation to 0–100 scale.
		maxOption := 0
		for _, s := range fsc.OptionScores {
			if s > maxOption {
				maxOption = s
			}
		}
		dimensionMaxRaw[fsc.Dimension] += float64(maxOption) * fsc.Weight
	}

	// Normalise each dimension to a 0–100 scale based on its max raw score.
	dimensionScores := make(map[string]int, len(dimensionRaw))
	for dim, raw := range dimensionRaw {
		maxRaw := dimensionMaxRaw[dim]
		if maxRaw <= 0 {
			dimensionScores[dim] = 0
			continue
		}
		normalised := (raw / maxRaw) * 100.0
		dimensionScores[dim] = int(normalised)
	}

	// Apply dimension weights to compute the total score. If no weights are
	// configured, use a simple average of dimension scores.
	totalScore := 0
	if len(scoringConfig.DimensionWeights) > 0 {
		weightedSum := 0.0
		weightTotal := 0.0
		for dim, score := range dimensionScores {
			w := scoringConfig.DimensionWeights[dim]
			weightedSum += float64(score) * w
			weightTotal += w
		}
		if weightTotal > 0 {
			totalScore = int(weightedSum / weightTotal)
		}
	} else if len(dimensionScores) > 0 {
		sum := 0
		for _, score := range dimensionScores {
			sum += score
		}
		totalScore = sum / len(dimensionScores)
	}

	// Clamp to max_score if configured.
	if scoringConfig.MaxScore > 0 && totalScore > scoringConfig.MaxScore {
		totalScore = scoringConfig.MaxScore
	}
	if totalScore < 0 {
		totalScore = 0
	}

	thresholdLabel := ClassifyScore(totalScore, scoringConfig.Thresholds)
	roadmap := GenerateRoadmap(dimensionScores, scoringConfig)

	return ScoreResult{
		TotalScore:      totalScore,
		DimensionScores: dimensionScores,
		ThresholdLabel:  thresholdLabel,
		Roadmap:         roadmap,
	}
}

// ClassifyScore returns the threshold label for a given total score. It
// iterates the thresholds in order and returns the first label whose
// [Min, Max] range contains the score. If no threshold matches, it returns
// "Unclassified".
func ClassifyScore(totalScore int, thresholds []Threshold) string {
	for _, t := range thresholds {
		if totalScore >= t.Min && totalScore <= t.Max {
			return t.Label
		}
	}
	return "Unclassified"
}

// GenerateRoadmap generates curated roadmap steps based on the user's weakest
// scoring dimensions. Dimensions are sorted ascending by score; the weakest
// dimensions get the highest-priority roadmap steps. Each step links to a
// platform content slug.
//
// The curated step library maps dimension names to recommended content. This
// is deterministic and always present (ADR-071 §3). AI-assisted supplementary
// steps are a separate layer gated by assessment.ai_roadmap_enabled.
func GenerateRoadmap(dimensionScores map[string]int, scoringConfig ScoringConfig) []RoadmapStep {
	if len(dimensionScores) == 0 {
		return []RoadmapStep{}
	}

	// Sort dimensions by score ascending (weakest first).
	type dimScore struct {
		dimension string
		score     int
	}
	sorted := make([]dimScore, 0, len(dimensionScores))
	for dim, score := range dimensionScores {
		sorted = append(sorted, dimScore{dimension: dim, score: score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score < sorted[j].score
	})

	// Curated step library — maps dimension to recommended next steps.
	// Priority is assigned based on the order of weakest dimensions.
	// Content slugs link to platform guides/tools (Section 14 content).
	curatedLibrary := curatedRoadmapLibrary()

	roadmap := make([]RoadmapStep, 0)
	priority := 1
	for _, ds := range sorted {
		steps, ok := curatedLibrary[ds.dimension]
		if !ok {
			continue
		}
		for _, step := range steps {
			step.Priority = priority
			step.Dimension = ds.dimension
			roadmap = append(roadmap, step)
			priority++
		}
	}

	return roadmap
}

// curatedRoadmapLibrary returns the deterministic mapping of dimension names
// to recommended roadmap steps. Each step links to a platform content slug.
// Admin can override these via scoring_config in a future iteration; for now
// this is the default curated library (ADR-071 §3).
func curatedRoadmapLibrary() map[string][]RoadmapStep {
	return map[string][]RoadmapStep{
		"financial": {
			{
				Title:       "Build your relocation savings fund",
				Description: "Calculate your target savings based on your destination's cost of living and create a monthly savings plan.",
				ContentSlug: "guide/banking-finance-relocation",
			},
			{
				Title:       "Use the Cost of Living Calculator",
				Description: "Compare living costs between your current city and target destination to set a realistic budget.",
				ContentSlug: "tool/cost-of-living-calculator",
			},
		},
		"language": {
			{
				Title:       "Start a language learning track",
				Description: "Begin structured language lessons for your destination country's primary language.",
				ContentSlug: "track/language-proficiency",
			},
		},
		"qualifications": {
			{
				Title:       "Get your credentials evaluated",
				Description: "Research credential recognition requirements for your profession in your target country.",
				ContentSlug: "guide/credential-evaluation",
			},
			{
				Title:       "Upgrade your professional certifications",
				Description: "Identify and pursue certifications that are in demand in your destination's job market.",
				ContentSlug: "guide/professional-certifications",
			},
		},
		"visa_eligibility": {
			{
				Title:       "Determine your visa pathway",
				Description: "Research visa categories you qualify for based on your skills, experience, and destination.",
				ContentSlug: "guide/visa-pathways-overview",
			},
			{
				Title:       "Prepare your visa application documents",
				Description: "Gather and organise the documents required for your target visa category.",
				ContentSlug: "guide/visa-document-checklist",
			},
		},
		"destination_knowledge": {
			{
				Title:       "Research your destination city",
				Description: "Learn about housing, healthcare, transportation, and community in your target city.",
				ContentSlug: "guide/destination-research-guide",
			},
			{
				Title:       "Connect with the diaspora community",
				Description: "Join spaces and forums for Nigerians in your destination country for on-the-ground insights.",
				ContentSlug: "guide/connecting-with-diaspora",
			},
		},
	}
}

// ---------------------------------------------------------------------------
// JSONB parsing helpers
// ---------------------------------------------------------------------------

// parseScoringConfig unmarshals the scoring_config JSONB from forms.
// Returns a zero-value config if the JSONB is empty or malformed.
func parseScoringConfig(raw []byte) (ScoringConfig, error) {
	if len(raw) == 0 {
		return ScoringConfig{}, nil
	}
	var cfg ScoringConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ScoringConfig{}, err
	}
	return cfg, nil
}

// parseFieldScoringConfig unmarshals the config JSONB from custom_fields.
// Returns a zero-value config if the JSONB is empty or malformed.
func parseFieldScoringConfig(raw []byte) FieldScoringConfig {
	if len(raw) == 0 {
		return FieldScoringConfig{}
	}
	var cfg FieldScoringConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return FieldScoringConfig{}
	}
	return cfg
}

// formToDTO converts a gen.Form + gen.CustomField slice into the
// AssessmentFormDTO used by the service and handlers.
func formToDTO(form gen.Form, fields []gen.CustomField) *AssessmentFormDTO {
	description := ""
	if form.Description != nil {
		description = *form.Description
	}

	scoringConfig, _ := parseScoringConfig(form.ScoringConfig)

	fieldDTOs := make([]AssessmentField, 0, len(fields))
	for _, f := range fields {
		fieldDTOs = append(fieldDTOs, AssessmentField{
			ID:        f.ID.String(),
			Label:     f.Label,
			Key:       f.Key,
			FieldType: f.FieldType,
			Required:  f.IsRequired,
			Position:  f.Position,
			Config:    parseFieldScoringConfig(f.Config),
		})
	}

	return &AssessmentFormDTO{
		ID:            form.ID.String(),
		TenantID:      form.TenantID.String(),
		Name:          form.Name,
		Slug:          form.Slug,
		Description:   description,
		IsMultiStep:   form.IsMultiStep,
		SubmitLabel:   form.SubmitLabel,
		Status:        form.Status,
		ScoringConfig: scoringConfig,
		Fields:        fieldDTOs,
	}
}
