// Package assessment — service.go holds the business logic for the Japa
// Readiness Assessment: form retrieval, submission, the server-side scoring
// engine, roadmap generation, and admin stats. It reads assessment.* system
// settings via the hot-reloadable config.Settings and enqueues Asynq tasks
// (assessment:score_submission, wallet:award_points) fire-and-forget.
// // Ref: japaconnect-engineering/sections/71-readiness-assessment.md
// //	japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md
package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/jedafe/japa/internal/wallet"
)

// Task type constants — must match the Section 11 Asynq task catalog.
const (
	TaskScoreSubmission  = "assessment:score_submission"
	TaskCleanupAnonymous = "assessment:cleanup_anonymous"
	TaskAwardPoints      = "wallet:award_points"
)

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// The concrete enqueuer satisfies this.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// AIDispatcher is the minimal interface for calling the AI gateway (§20).
// The assessment service uses it to generate AI-assisted supplementary
// roadmap steps when assessment.ai_roadmap_enabled is true (ADR-071 §3).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req AIRequest) (AIResponse, error)
	HasCategory(category string) bool
}

// AIRequest is the standard envelope sent to the AI gateway.
type AIRequest struct {
	Category string
	Payload  map[string]any
}

// AIResponse is the standard envelope returned by the AI gateway.
type AIResponse struct {
	ProviderRef string
	Status      string
	Raw         map[string]any
}

// RAGRetriever is the minimal interface for RAG grounding (§69, ADR-069).
// The assessment service uses it to ground AI-assisted roadmap steps in
// platform content.
type RAGRetriever interface {
	Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]RAGChunk, error)
	FormatContext(chunks []RAGChunk) string
}

// RAGChunk is a single retrieved passage from the RAG layer.
type RAGChunk struct {
	Content   string
	Score     float64
	Source    string
	ChunkType string
}

// ScoringConfig is the JSONB shape stored in forms.scoring_config.
// It defines the max possible score, threshold bands for classification,
// per-dimension weights used for aggregation, and an optional admin-configured
// curated roadmap library (ADR-071 §3 — curated steps are admin-configurable,
// stored in scoring_config). When CuratedRoadmap is empty, GenerateRoadmap
// falls back to the built-in default library returned by
// curatedRoadmapLibrary().
type ScoringConfig struct {
	MaxScore         int                      `json:"max_score"`
	Thresholds       []Threshold              `json:"thresholds"`
	DimensionWeights map[string]float64       `json:"dimension_weights"`
	CuratedRoadmap   map[string][]RoadmapStep `json:"curated_roadmap,omitempty"`
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
// TargetCountry, JourneyStage, VisaRoute, and SourceCountry are optional
// filtering hints so the step is only surfaced for matching journey contexts.
// Empty values mean the step applies to all visitors (global fallback).
type RoadmapStep struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ContentSlug string `json:"content_slug"`
	Dimension   string `json:"dimension"`
	Priority    int    `json:"priority"`

	TargetCountry string `json:"target_country,omitempty"`
	SourceCountry string `json:"source_country,omitempty"`
	JourneyStage  string `json:"journey_stage,omitempty"`
	VisaRoute     string `json:"visa_route,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
}

// JourneyContext captures the geo and intent signals submitted with the
// assessment. It drives destination-specific roadmap selection, RAG grounding,
// and post-submission visitor profile enrichment.
// Ref: sections/71-readiness-assessment.md §2.2
// //	docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md
// docs/superpowers/plans/2026-08-31-geo-aware-readiness-roadmap.md
// // Exported fields are persisted with the score for audit and read-time
// personalization; GeoSpaceID is resolved only at score time and is not stored.
type JourneyContext struct {
	OriginCountry      string      `json:"origin_country,omitempty"`
	DestinationCountry string      `json:"destination_country,omitempty"`
	VisaRoute          string      `json:"visa_route,omitempty"`
	JourneyStage       string      `json:"journey_stage,omitempty"`
	Purpose            string      `json:"purpose,omitempty"`
	TravelDate         string      `json:"travel_date,omitempty"`
	GeoSpaceID         pgtype.UUID `json:"-"`
}

// ScoreResult is the output of the scoring engine before persistence.
type ScoreResult struct {
	TotalScore      int            `json:"total_score"`
	DimensionScores map[string]int `json:"dimension_scores"`
	ThresholdLabel  string         `json:"threshold_label"`
	Roadmap         []RoadmapStep  `json:"roadmap"`
	JourneyContext  JourneyContext `json:"journey_context,omitempty"`
}

// AssessmentFormDTO is the active assessment form with its scoring_config and
// fields, ready for the scoring engine or for public display.
// //nolint:revive // exported: stutter naming is intentional
type AssessmentFormDTO struct {
	ID string `json:"id"`

	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	Description   string            `json:"description"`
	IsMultiStep   bool              `json:"is_multi_step"`
	SubmitLabel   string            `json:"submit_label"`
	Status        string            `json:"status"`
	ScoringConfig ScoringConfig     `json:"scoring_config"`
	Fields        []AssessmentField `json:"fields"`
}

// assessmentDimensions returns the distinct readiness dimensions that may be
// recommended by the active assessment. It is the union of dimensions declared
// on form fields and the keys of the active/default curated roadmap library.
// Ref: docs/superpowers/specs/2026-09-09-readiness-roadmap-content-journey-design.md
func assessmentDimensions(form *AssessmentFormDTO) []string {
	seen := make(map[string]bool)

	for _, f := range form.Fields {
		if d := strings.TrimSpace(f.Config.Dimension); d != "" {
			seen[d] = true
		}
	}

	library := form.ScoringConfig.CuratedRoadmap
	if len(library) == 0 {
		library = curatedRoadmapLibrary()
	}
	for d := range library {
		seen[d] = true
	}

	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// AssessmentField is a single field on the assessment form, with its scoring
// config resolved from the config JSONB column. Options (for select/multiselect
// fields) and Placeholder are exposed so the frontend can render the form
// controls without a second round-trip (Rule 16 — no hardcoded option lists).
// //nolint:revive // exported: stutter naming is intentional
type AssessmentField struct {
	ID          string             `json:"id"`
	Label       string             `json:"label"`
	Key         string             `json:"key"`
	FieldType   string             `json:"field_type"`
	Required    bool               `json:"is_required"`
	Position    int32              `json:"position"`
	Placeholder string             `json:"placeholder"`
	Options     json.RawMessage    `json:"options"`
	Config      FieldScoringConfig `json:"config"`
}

// Service holds the business logic for the Japa Readiness Assessment.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
	ai       AIDispatcher
	rag      RAGRetriever
	activity wallet.ActivityEmitter
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

// SetActivityEmitter injects the wallet activity emitter used to record
// assessment.completed events so §64 badge/challenge triggers (e.g.
// japa_ready) can fire. Optional — nil-safe.
func (s *Service) SetActivityEmitter(e wallet.ActivityEmitter) { s.activity = e }

// emitActivity records a wallet activity event and enqueues its processing.
// Fire-and-forget (Rule 8): failures are logged, never propagated. A
// duplicate reference hits ON CONFLICT DO NOTHING and returns ErrNoRows,
// which is treated as "already recorded" rather than an error.
func (s *Service) emitActivity(ctx context.Context, userID pgtype.UUID, key, reference string, payload map[string]interface{}) {
	if s.activity == nil || !userID.Valid {
		return
	}

	event, err := s.activity.EmitActivityEvent(ctx, userID, wallet.EmitActivityEventInput{
		ActivityKey: key,
		Reference:   reference,
		Payload:     payload,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("assessment: emit activity event failed", "activity_key", key, "error", err)
		}

		return
	}

	if err := s.activity.EnqueueProcessActivityEvent(ctx, event.ID); err != nil {
		slog.Warn("assessment: enqueue activity event failed", "activity_key", key, "error", err)
	}
}

// SetAIDispatcher injects the AI gateway dispatcher for AI-assisted roadmap
// generation (ADR-071 §3). Optional — if not set, only curated roadmap steps
// are generated.
func (s *Service) SetAIDispatcher(d AIDispatcher) { s.ai = d }

// SetRAGRetriever injects the RAG retriever for grounding AI-assisted roadmap
// steps in platform content (ADR-069). Optional — if not set, AI steps are
// generated without RAG grounding.
func (s *Service) SetRAGRetriever(r RAGRetriever) { s.rag = r }

// GetActiveAssessment returns the active assessment form with its scoring_config
// and fields. Returns an error if no active assessment form exists for the
// platform.
func (s *Service) GetActiveAssessment(ctx context.Context) (*AssessmentFormDTO, error) {
	form, err := s.store.GetActiveAssessmentForm(ctx)
	if err != nil {
		return nil, fmt.Errorf("assessment: get active: %w", err)
	}

	fields, err := s.store.GetAssessmentFormFields(ctx, form.ID)
	if err != nil {
		return nil, fmt.Errorf("assessment: get fields: %w", err)
	}

	return formToDTO(form, fields), nil
}

// SubmitAssessment creates a form_submission for the assessment and enqueues
// the assessment:score_submission task fire-and-forget (Rule 8). For anonymous
// users (userID is nil), the submission is created without a user_id.
// Returns the submission ID.
func (s *Service) SubmitAssessment(ctx context.Context, userID *uuid.UUID, formID pgtype.UUID, fieldValues json.RawMessage) (pgtype.UUID, error) {
	var userIDVal pgtype.UUID
	if userID != nil {
		userIDVal = pgtype.UUID{Bytes: *userID, Valid: true}
	}

	sub, err := s.store.CreateAssessmentSubmission(ctx, formID, userIDVal, []byte(fieldValues))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: submit: %w", err)
	}

	// Enqueue scoring task fire-and-forget (Rule 8). A failure to enqueue
	// does not fail the submission — the result will be scored on a later
	// retry or admin re-score.
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"submission_id": sub.ID.String(),
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
// // The caller provides the submission's field_values and user_id via
// ScoreSubmissionInput. The worker handler fetches the submission row from
// the shared form_submissions store and passes its field_values here.
func (s *Service) ScoreSubmission(ctx context.Context, input ScoreSubmissionInput) (pgtype.UUID, error) {
	form, err := s.store.GetActiveAssessmentForm(ctx)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: get form: %w", err)
	}

	// Load the submitted form ID so we can scope the completion-point award
	// to one per user per form (Section 28).
	submission, err := s.store.GetSubmissionByID(ctx, input.SubmissionID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: get submission: %w", err)
	}

	fields, err := s.store.GetAssessmentFormFields(ctx, form.ID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: get fields: %w", err)
	}

	scoringConfig, err := parseScoringConfig(form.ScoringConfig)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: parse scoring config: %w", err)
	}

	result := computeScore(input.FieldValues, fields, scoringConfig)

	// Resolve the destination country to a country-level Space for RAG
	// geo-filtering. This is best-effort; a missing Space leaves GeoSpaceID
	// invalid and RAG falls back to unfiltered retrieval (§103, ADR-069).
	if result.JourneyContext.DestinationCountry != "" {
		if space, err := s.store.GetSpaceByGeoCode(ctx, result.JourneyContext.DestinationCountry); err == nil {
			result.JourneyContext.GeoSpaceID = space.ID
		} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("assessment: score: destination space lookup failed", "destination", result.JourneyContext.DestinationCountry, "error", err)
		}
	}

	// AI-assisted supplementary roadmap steps (ADR-071 §3).
	// Gated by assessment.ai_roadmap_enabled. Falls back gracefully to
	// curated-only if AI is unavailable or the setting is off.
	if s.ai != nil && s.ai.HasCategory("content_generation") {
		aiEnabled := true
		if s.settings != nil {
			aiEnabled = s.settings.GetBoolOr(ctx, "assessment.ai_roadmap_enabled", true)
		}

		if aiEnabled {
			aiSteps := s.generateAIRoadmapSteps(ctx, input.UserID, result.JourneyContext, result.DimensionScores, result.ThresholdLabel)
			if len(aiSteps) > 0 {
				// Append AI steps after curated steps, continuing the priority
				// sequence. Skip any step that links to content already in the
				// roadmap so a guide is never listed as two different steps.
				usedSlugs := make(map[string]bool, len(result.Roadmap))

				for _, step := range result.Roadmap {
					if step.ContentSlug != "" {
						usedSlugs[step.ContentSlug] = true
					}
				}

				basePriority := len(result.Roadmap)

				for _, step := range aiSteps {
					if step.ContentSlug != "" && usedSlugs[step.ContentSlug] {
						continue
					}

					if step.ContentSlug != "" {
						usedSlugs[step.ContentSlug] = true
					}

					basePriority++
					step.Priority = basePriority
					result.Roadmap = append(result.Roadmap, step)
				}
			}
		}
	}

	// Resolve each curated step to the most specific published content for the
	// visitor's journey context. Gated by assessment.geo_content_resolution_enabled.
	result.Roadmap = s.resolveRoadmapContent(ctx, result.Roadmap, result.JourneyContext)

	dimensionScoresJSON, err := json.Marshal(result.DimensionScores)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: marshal dimension scores: %w", err)
	}

	roadmapJSON, err := json.Marshal(result.Roadmap)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: marshal roadmap: %w", err)
	}

	// Completion points are awarded only once per user per form (Section 28).
	// Count prior results before creating the new one.
	var priorResults int64
	if input.UserID.Valid {
		priorResults, err = s.store.CountAssessmentResultsByUserAndForm(ctx, input.UserID, submission.FormID)
		if err != nil {
			slog.Warn("assessment: score: count prior results failed", "error", err)

			priorResults = 0
		}
	}

	ar, err := s.store.CreateAssessmentResult(ctx, input.UserID, input.SubmissionID, int32(result.TotalScore), dimensionScoresJSON, result.ThresholdLabel, roadmapJSON) // #nosec
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("assessment: score: create result: %w", err)
	}

	// §64: record the assessment.completed activity so badge/challenge
	// triggers (e.g. japa_ready) can fire. Reference is per result so each
	// completion counts once.
	s.emitActivity(ctx, input.UserID, wallet.ActivityKeyAssessmentCompleted, "assessment:"+ar.ID.String(), map[string]interface{}{
		"form_id":   submission.FormID.String(),
		"threshold": result.ThresholdLabel,
	})

	// Award completion points if configured (Section 28 — Wallet/Credits).
	// A deterministic per-form idempotency key in the wallet ledger makes
	// concurrent first-submission races safe (Rule 9).
	if s.settings != nil && input.UserID.Valid && priorResults == 0 {
		points := s.settings.GetIntOr(ctx, "assessment.completion_points", 50)
		if points > 0 && s.enqueuer != nil {
			idempotencyKey := fmt.Sprintf("assessment:completion::%s:%s", input.UserID.String(), submission.FormID.String())

			payload, err := json.Marshal(map[string]any{
				"user_id": input.UserID.String(),

				"points":          points,
				"reason":          "assessment_completion",
				"reference_id":    ar.ID.String(),
				"idempotency_key": idempotencyKey,
			})
			if err != nil {
				slog.Warn("assessment: score: marshal points payload failed", "error", err)
			} else if err := s.enqueuer.Enqueue(TaskAwardPoints, payload); err != nil {
				slog.Warn("assessment: score: enqueue award points failed", "error", err)
			}
		}
	}

	slog.Info("assessment: score: complete", "submission_id", input.SubmissionID.String(), "result_id", ar.ID.String(), "total_score", result.TotalScore, "threshold", result.ThresholdLabel)

	return ar.ID, nil
}

// GetResult returns a single assessment result with its roadmap.
func (s *Service) GetResult(ctx context.Context, resultID pgtype.UUID) (gen.AssessmentResult, error) {
	return s.store.GetAssessmentResultByID(ctx, resultID)
}

// GetSubmissionFieldValues returns the raw field_values snapshot for the
// submission linked to an assessment result. Used by the public result roadmap
// endpoint to rebuild the visitor's journey context at read time.
func (s *Service) GetSubmissionFieldValues(ctx context.Context, submissionID pgtype.UUID) (json.RawMessage, error) {
	sub, err := s.store.GetSubmissionByID(ctx, submissionID)
	if err != nil {
		return nil, err
	}

	return sub.FieldValues, nil
}

// GetResultBySubmission returns the assessment result linked to a submission.
// Callers use this to poll for the result after the async scoring task is
// enqueued. Returns pgx.ErrNoRows when the result is not yet written.
func (s *Service) GetResultBySubmission(ctx context.Context, submissionID pgtype.UUID) (gen.AssessmentResult, error) {
	return s.store.GetAssessmentResultBySubmission(ctx, submissionID)
}

// ListHistory returns a user's past assessment results, paginated.
func (s *Service) ListHistory(ctx context.Context, userID pgtype.UUID, page, limit int32) ([]gen.AssessmentResult, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	results, err := s.store.ListUserAssessmentResults(ctx, userID, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: list history: %w", err)
	}

	total, err := s.store.CountUserAssessmentResults(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: count history: %w", err)
	}

	return results, total, nil
}

// AdminListResults returns all users' assessment results, paginated,
// optionally filtered by score range, and sorted by a whitelisted column.
// minScore/maxScore of 0 means no filter. sort is a whitelisted key
// (total_score, threshold_label, created_at, username); order is "asc"/"desc".
func (s *Service) AdminListResults(ctx context.Context, minScore, maxScore, page, limit int32, sort, order string) ([]AdminListAssessmentResultsRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	results, total, err := s.store.AdminListAssessmentResults(ctx, AdminListAssessmentResultsFilter{

		MinScore: minScore,
		MaxScore: maxScore,
		Offset:   offset,
		Limit:    limit,
		Sort:     sort,
		Order:    order,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("assessment: admin list: %w", err)
	}

	return results, total, nil
}

// AdminScoreDistribution returns the count and average score of results
// grouped by threshold label.
func (s *Service) AdminScoreDistribution(ctx context.Context) ([]AdminScoreDistributionRow, error) {
	return s.store.AdminScoreDistribution(ctx)
}

// AdminCompletionStats returns total submissions, total scored results, and
// the average score across all assessment submissions.
func (s *Service) AdminCompletionStats(ctx context.Context) (AdminCompletionStatsRow, error) {
	return s.store.AdminCompletionStats(ctx)
}

// UpdateFormScoring updates the scoring_config JSONB on the assessment form.
// Admin-only.
func (s *Service) UpdateFormScoring(ctx context.Context, formID pgtype.UUID, scoringConfig json.RawMessage) (gen.Form, error) {
	// Validate the JSONB is a well-formed ScoringConfig before persisting.
	var cfg ScoringConfig
	if err := json.Unmarshal(scoringConfig, &cfg); err != nil {
		return gen.Form{}, fmt.Errorf("assessment: update scoring: invalid config: %w", err)
	}

	return s.store.UpdateAssessmentFormScoring(ctx, formID, []byte(scoringConfig))
}

// CreateAssessment creates a new readiness assessment form (form_type='assessment').
func (s *Service) CreateAssessment(ctx context.Context, input CreateAssessmentInput) (*AssessmentFormDTO, error) {
	if input.Name == "" {
		return nil, errors.New("name is required")
	}

	if input.Slug == "" {
		return nil, errors.New("slug is required")
	}

	form, err := s.store.CreateAssessmentForm(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("assessment: create assessment: %w", err)
	}

	return formToDTO(form, []gen.CustomField{}), nil
}

// UpdateAssessment updates an existing readiness assessment form.
func (s *Service) UpdateAssessment(ctx context.Context, id pgtype.UUID, input UpdateAssessmentInput) (*AssessmentFormDTO, error) {
	if input.Name != nil && *input.Name == "" {
		return nil, errors.New("name cannot be empty")
	}

	if input.Slug != nil && *input.Slug == "" {
		return nil, errors.New("slug cannot be empty")
	}

	form, err := s.store.UpdateAssessmentForm(ctx, id, input)
	if err != nil {
		return nil, fmt.Errorf("assessment: update assessment: %w", err)
	}

	fields, err := s.store.GetAssessmentFormFields(ctx, pgtype.UUID{})
	if err != nil {
		return nil, fmt.Errorf("assessment: get form fields after update: %w", err)
	}

	return formToDTO(form, fields), nil
}

// DeleteAssessment deletes a readiness assessment form by id.
func (s *Service) DeleteAssessment(ctx context.Context, id pgtype.UUID) error {
	if err := s.store.DeleteAssessmentForm(ctx, id); err != nil {
		return fmt.Errorf("assessment: delete assessment: %w", err)
	}

	return nil
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

		rawScore, maxScore := scoreFieldAnswer(field, fsc, answer)
		if maxScore <= 0 {
			continue
		}

		dimensionRaw[fsc.Dimension] += rawScore * fsc.Weight
		dimensionMaxRaw[fsc.Dimension] += maxScore * fsc.Weight
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
	journeyCtx := parseJourneyFields(answers)
	roadmap := GenerateRoadmap(dimensionScores, scoringConfig, journeyCtx)

	return ScoreResult{
		TotalScore:      totalScore,
		DimensionScores: dimensionScores,
		ThresholdLabel:  thresholdLabel,
		Roadmap:         roadmap,
		JourneyContext:  journeyCtx,
	}
}

// scoreFieldAnswer returns the raw score and the maximum possible raw score
// for a single field answer. It supports the full Form Builder field types:
// select, multiselect, number/rating, boolean, and text (text is skipped).
// For multiselect the raw score is the sum of the selected option scores and
// the max is the sum of all configured positive option scores. For number/
// rating, the raw numeric value is used on a 0–100 scale when no explicit
// option mapping exists.
func scoreFieldAnswer(field gen.CustomField, fsc FieldScoringConfig, answer any) (float64, float64) {
	if answer == nil {
		return 0, 0
	}

	switch field.FieldType {
	case "multiselect":
		return scoreMultiSelect(fsc, answer)
	case "number", "rating":
		return scoreNumber(fsc, answer)
	case "boolean":
		return scoreBoolean(fsc, answer)
	case "select":
		return scoreSingle(fsc, fmt.Sprintf("%v", answer))
	default:
		// text and any other un-scored types contribute 0
		return 0, 0
	}
}

// scoreSingle returns the score and max score for a single string answer.
func scoreSingle(fsc FieldScoringConfig, answer string) (float64, float64) {
	raw, ok := fsc.OptionScores[answer]
	if !ok {
		return 0, 0
	}

	return float64(raw), float64(maxOptionScore(fsc.OptionScores))
}

// scoreMultiSelect returns the summed score for selected values and the sum of
// all configured positive option scores as the maximum.
func scoreMultiSelect(fsc FieldScoringConfig, answer any) (float64, float64) {
	selected, ok := answer.([]any)
	if !ok {
		return 0, 0
	}

	var raw float64

	for _, v := range selected {
		key := fmt.Sprintf("%v", v)
		if s, ok := fsc.OptionScores[key]; ok {
			raw += float64(s)
		}
	}

	var max float64

	for _, s := range fsc.OptionScores {
		if s > 0 {
			max += float64(s)
		}
	}

	return raw, max
}

// scoreNumber returns the score for a number/rating answer. If the field has
// an explicit option mapping for the numeric string, use it; otherwise treat
// the numeric value as a 0–100 score.
func scoreNumber(fsc FieldScoringConfig, answer any) (float64, float64) {
	var n float64
	switch v := answer.(type) {
	case float64:
		n = v
	case float32:
		n = float64(v)
	case int:
		n = float64(v)
	case int32:
		n = float64(v)
	case int64:
		n = float64(v)
	case json.Number:
		if parsed, err := strconv.ParseFloat(string(v), 64); err == nil {
			n = parsed
		} else {
			return 0, 0
		}
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			n = parsed
		} else {
			return 0, 0
		}
	default:
		return 0, 0
	}

	if s, ok := fsc.OptionScores[fmt.Sprintf("%v", answer)]; ok {
		return float64(s), float64(maxOptionScore(fsc.OptionScores))
	}

	return n, 100.0
}

// scoreBoolean returns the score for a boolean answer by normalising true/
// false/1/0/yes/no to "true" or "false" keys.
func scoreBoolean(fsc FieldScoringConfig, answer any) (float64, float64) {
	s := strings.ToLower(fmt.Sprintf("%v", answer))
	switch s {
	case "true", "1", "yes", "on":
		s = "true"
	case "false", "0", "no", "off":
		s = "false"
	default:
		return 0, 0
	}

	if raw, ok := fsc.OptionScores[s]; ok {
		return float64(raw), float64(maxOptionScore(fsc.OptionScores))
	}

	return 0, 0
}

// maxOptionScore returns the highest configured option score.
func maxOptionScore(scores map[string]int) int {
	max := 0
	for _, s := range scores {
		if s > max {
			max = s
		}
	}

	return max
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

// parseJourneyFields extracts the geo and intent signals the readiness form
// captured. It mirrors the parsing in visitor.parseAssessmentSubmission so the
// same field names can update the visitor's journey context after scoring.
func parseJourneyFields(answers map[string]any) JourneyContext {
	j := JourneyContext{
		OriginCountry:      visitor.NormalizeCountry(extractStringValue(answers, "origin_country")),
		DestinationCountry: visitor.NormalizeCountry(extractStringValue(answers, "destination_country")),
		VisaRoute:          visitor.NormalizeVisaRoute(extractStringValue(answers, "visa_type", "visa_route")),
		JourneyStage:       visitor.NormalizeJourneyStage(extractStringValue(answers, "journey_stage", "japa_status")),
		Purpose:            visitor.NormalizePurpose(extractStringValue(answers, "purpose", "travel_purpose")),
		TravelDate:         extractStringValue(answers, "travel_date", "planned_travel_date"),
	}
	if _, err := time.Parse("2006-01-02", j.TravelDate); err != nil {
		j.TravelDate = ""
	}

	return j
}

// extractStringValue returns the first non-empty string value for the given
// keys from a field_values map. It handles the common Form Builder encodings:
// plain string, {value: string}, {id: string}, and []any of the same.
func extractStringValue(raw map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := raw[k]
		if !ok {
			continue
		}

		switch val := v.(type) {
		case string:
			if val != "" {
				return val
			}
		case map[string]any:
			if s, ok := val["value"].(string); ok && s != "" {
				return s
			}

			if s, ok := val["id"].(string); ok && s != "" {
				return s
			}
		case []any:
			var parts []string

			for _, el := range val {
				switch e := el.(type) {
				case string:
					if e != "" {
						parts = append(parts, e)
					}
				case map[string]any:
					if s, ok := e["value"].(string); ok && s != "" {
						parts = append(parts, s)
					} else if s, ok := e["id"].(string); ok && s != "" {
						parts = append(parts, s)
					}
				}
			}

			if len(parts) > 0 {
				return strings.Join(parts, ",")
			}
		case float64:
			return strconv.FormatFloat(val, 'f', -1, 64)
		case int:
			return strconv.Itoa(val)
		}
	}

	return ""
}

// GenerateRoadmap generates curated roadmap steps based on the user's weakest
// scoring dimensions. Dimensions are sorted ascending by score; the weakest
// dimensions get the highest-priority roadmap steps. Each step links to a
// platform content slug.
// // The curated step library is sourced from scoring_config.curated_roadmap when
// the admin has configured it (ADR-071 §3 — curated steps are
// admin-configurable, stored in scoring_config). When that field is empty,
// GenerateRoadmap falls back to the built-in default library returned by
// curatedRoadmapLibrary(). This is deterministic and always present.
// AI-assisted supplementary steps are a separate layer gated by
// assessment.ai_roadmap_enabled.
func GenerateRoadmap(dimensionScores map[string]int, scoringConfig ScoringConfig, journeyCtx JourneyContext) []RoadmapStep {
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

	// Curated step library — prefer the admin-configured library in
	// scoring_config.curated_roadmap (ADR-071 §3); fall back to the built-in
	// default library when the admin has not configured one.
	curatedLibrary := scoringConfig.CuratedRoadmap
	if len(curatedLibrary) == 0 {
		curatedLibrary = curatedRoadmapLibrary()
	}

	roadmap := make([]RoadmapStep, 0)
	priority := 1

	// The curated library can reference the same content slug from more than
	// one dimension (e.g. visa_eligibility and logistical both link the visa
	// pathway guide). Deduplicate so the roadmap never lists the same content
	// as two different steps.
	usedSlugs := make(map[string]bool)

	for _, ds := range sorted {
		steps, ok := curatedLibrary[ds.dimension]
		if !ok {
			continue
		}
		// Prefer steps that match the visitor's journey context; fall back to
		// globally-applicable steps (empty target/stage/visa fields) if no
		// context-specific step exists for this dimension.
		matched := filterRoadmapSteps(steps, journeyCtx)
		if len(matched) == 0 {
			matched = steps
		}

		for _, step := range matched {
			if step.ContentSlug != "" {
				if usedSlugs[step.ContentSlug] {
					continue
				}

				usedSlugs[step.ContentSlug] = true
			}

			step.Priority = priority
			step.Dimension = ds.dimension
			roadmap = append(roadmap, step)
			priority++
		}
	}

	return roadmap
}

// pickBestRoadmapContent selects the highest-scoring, unused published guide for a
// roadmap step. It does not restrict candidates to the original step's slug family,
// so any guide with the same dimension and a better journey match can win.
// // Ref: docs/superpowers/specs/2026-09-09-readiness-roadmap-content-journey-design.md
func pickBestRoadmapContent(rows []gen.FindRoadmapContentByDimensionRow, base string, used map[string]bool, journeyCtx JourneyContext) (gen.FindRoadmapContentByDimensionRow, int) {
	best := gen.FindRoadmapContentByDimensionRow{}
	bestScore := 0

	for _, row := range rows {
		// The current step's own curated slug is allowed as a candidate even if it
		// is already in `used`, so the tie-breaker can keep the original on equal
		// context scores. Other steps' slugs remain blocked to avoid duplicates.
		if row.Slug == "" || (used[row.Slug] && row.Slug != base) {
			continue
		}

		score := 0
		if journeyCtx.DestinationCountry != "" && row.TargetCountry != nil && *row.TargetCountry == journeyCtx.DestinationCountry {
			score += 4
		}
		if journeyCtx.OriginCountry != "" && row.SourceCountry != nil && *row.SourceCountry == journeyCtx.OriginCountry {
			score += 1
		}
		if journeyCtx.JourneyStage != "" && row.JourneyStage != nil && *row.JourneyStage == journeyCtx.JourneyStage {
			score += 2
		}
		if journeyCtx.VisaRoute != "" && row.VisaRoute != nil && *row.VisaRoute == journeyCtx.VisaRoute {
			score += 2
		}
		if p := stringFromAny(row.Purpose); journeyCtx.Purpose != "" && p != "" && p == journeyCtx.Purpose {
			score += 1
		}

		if score <= 0 {
			continue
		}

		// Tie-breaker: when two guides are equally specific, keep the original.
		if score == bestScore && bestScore > 0 && row.Slug == base && best.Slug != base {
			best = row
		}

		if score > bestScore {
			best = row
			bestScore = score
		}
	}

	return best, bestScore
}

// resolveRoadmapContent resolves each curated roadmap step to the most specific
// published content for the visitor's journey context when
// assessment.geo_content_resolution_enabled is true. It mutates the provided
// steps in place and returns the same slice.
// // A step whose content_slug is empty (e.g. AI-generated prose without a link)
// is skipped. The original curated step is kept when no candidate is specific.
func (s *Service) resolveRoadmapContent(ctx context.Context, steps []RoadmapStep, journeyCtx JourneyContext) []RoadmapStep {
	if s.store == nil || s.settings == nil {
		return steps
	}

	enabled := s.settings.GetBoolOr(ctx, "assessment.geo_content_resolution_enabled", true)
	if !enabled {
		return steps
	}

	// Seed used with every step's current slug so resolution can never point a
	// step at content another step already links to.
	used := make(map[string]bool, len(steps))

	for _, step := range steps {
		if step.ContentSlug != "" {
			used[step.ContentSlug] = true
		}
	}

	for i := range steps {
		if steps[i].ContentSlug == "" {
			continue
		}

		rows, err := s.store.FindRoadmapContentByDimension(
			ctx,
			steps[i].Dimension,
			journeyCtx.DestinationCountry,
			journeyCtx.JourneyStage,
			journeyCtx.VisaRoute,
			journeyCtx.OriginCountry,
			journeyCtx.Purpose,
		)
		if err != nil {
			slog.Warn("assessment: resolve roadmap content failed", "dimension", steps[i].Dimension, "error", err)

			continue
		}

		best, bestScore := pickBestRoadmapContent(rows, steps[i].ContentSlug, used, journeyCtx)
		if bestScore == 0 {
			continue
		}

		steps[i].ContentSlug = best.Slug
		if best.Title != "" {
			steps[i].Title = best.Title
		}
		if desc := feed.CleanExcerpt(best.Description); desc != "" {
			steps[i].Description = desc
		}
		if best.TargetCountry != nil && *best.TargetCountry != "" {
			steps[i].TargetCountry = *best.TargetCountry
		}
		if best.JourneyStage != nil && *best.JourneyStage != "" {
			steps[i].JourneyStage = *best.JourneyStage
		}
		if best.VisaRoute != nil && *best.VisaRoute != "" {
			steps[i].VisaRoute = *best.VisaRoute
		}
		if best.SourceCountry != nil && *best.SourceCountry != "" {
			steps[i].SourceCountry = *best.SourceCountry
		}
		if p := stringFromAny(best.Purpose); p != "" {
			steps[i].Purpose = p
		}

		used[best.Slug] = true
	}

	return steps
}

// filterRoadmapSteps returns steps whose target_country, journey_stage,
// visa_route, and source_country filters match the supplied journey context.
// Empty filter fields on a step mean it is globally applicable.
func filterRoadmapSteps(steps []RoadmapStep, journeyCtx JourneyContext) []RoadmapStep {
	matches := make([]RoadmapStep, 0, len(steps))

	for _, step := range steps {
		if stepMatchesJourney(step, journeyCtx) {
			matches = append(matches, step)
		}
	}

	return matches
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}

	switch s := v.(type) {
	case string:
		return s
	case *string:
		if s == nil {
			return ""
		}

		return *s
	}

	return ""
}

func stepMatchesJourney(step RoadmapStep, journeyCtx JourneyContext) bool {
	if step.TargetCountry != "" && step.TargetCountry != journeyCtx.DestinationCountry {
		return false
	}

	if step.SourceCountry != "" && step.SourceCountry != journeyCtx.OriginCountry {
		return false
	}

	if step.JourneyStage != "" && step.JourneyStage != journeyCtx.JourneyStage {
		return false
	}

	if step.VisaRoute != "" && step.VisaRoute != journeyCtx.VisaRoute {
		return false
	}

	if step.Purpose != "" && step.Purpose != journeyCtx.Purpose {
		return false
	}

	return true
}

// generateAIRoadmapSteps calls the AI gateway (§20) to generate 2–3
// personalized supplementary roadmap steps as prose, grounded via the RAG
// layer (§69, ADR-069). Falls back gracefully to no AI steps if the AI
// gateway or RAG retrieval fails — curated steps are always present.
// // The prompt includes the user's dimension scores and threshold label so
// the AI can target its recommendations to the user's weakest areas.
// RAG grounding (when available) injects relevant platform content passages
// so the AI's recommendations reference real JapaConnect resources.
func (s *Service) generateAIRoadmapSteps(ctx context.Context, userID pgtype.UUID, journeyCtx JourneyContext, dimensionScores map[string]int, thresholdLabel string) []RoadmapStep {
	// Build a summary of the user's weakest dimensions for the prompt.
	weakestDims := make([]string, 0, len(dimensionScores))

	type dimScore struct {
		dim   string
		score int
	}

	sorted := make([]dimScore, 0, len(dimensionScores))
	for dim, score := range dimensionScores {
		sorted = append(sorted, dimScore{dim, score})
	}

	sort.Slice(sorted, func(i, j int) bool { return sorted[i].score < sorted[j].score })

	for _, ds := range sorted {
		weakestDims = append(weakestDims, fmt.Sprintf("%s: %d/100", ds.dim, ds.score))
	}

	// Build a geo-aware prompt. The destination country is the strongest signal;
	// origin, stage, visa, and purpose sharpen the recommendations.
	var geoHints []string
	if journeyCtx.DestinationCountry != "" {
		geoHints = append(geoHints, "destination country: "+journeyCtx.DestinationCountry)
	}

	if journeyCtx.OriginCountry != "" {
		geoHints = append(geoHints, "origin country: "+journeyCtx.OriginCountry)
	}

	if journeyCtx.JourneyStage != "" {
		geoHints = append(geoHints, "journey stage: "+journeyCtx.JourneyStage)
	}

	if journeyCtx.VisaRoute != "" {
		geoHints = append(geoHints, "visa route: "+journeyCtx.VisaRoute)
	}

	if journeyCtx.Purpose != "" {
		geoHints = append(geoHints, "purpose: "+journeyCtx.Purpose)
	}

	prompt := fmt.Sprintf(
		"You are a relocation advisor for people planning an international move (japa). "+
			"The user completed a readiness assessment and scored '%s' overall. "+
			"Their dimension scores (weakest first): %s. "+
			"%s "+
			"Generate 2-3 personalized next steps as JSON array. Each step must have: "+
			"\"title\" (short, actionable), \"description\" (1-2 sentences), "+
			"\"content_slug\" (empty string if no specific platform content), "+
			"\"dimension\" (the dimension this step addresses). "+
			"Optional geo-aware fields: \"target_country\", \"source_country\", \"journey_stage\", \"visa_route\". "+
			"Focus on the weakest dimensions and make recommendations specific to the destination country. "+
			"Be specific and practical. "+
			"Do not repeat steps that are already in the curated roadmap.",
		thresholdLabel,
		strings.Join(weakestDims, ", "),
		strings.Join(geoHints, "; "),
	)

	// RAG grounding: retrieve relevant platform content passages (ADR-069).
	// Geo-filter grounding to the destination country when available so the AI
	// recommends content that actually exists for that destination.
	ragContext := ""

	if s.rag != nil {
		query := "relocation readiness advice for weak areas: " + strings.Join(weakestDims[:min(3, len(weakestDims))], ", ")

		chunks, err := s.rag.Retrieve(ctx, query, "en", &journeyCtx.GeoSpaceID, userID)
		if err != nil {
			slog.Warn("assessment: ai roadmap: RAG retrieve failed", "error", err)
		} else if len(chunks) > 0 {
			ragContext = s.rag.FormatContext(chunks)
		}
	}

	payload := map[string]any{
		"prompt":      prompt,
		"max_tokens":  500,
		"temperature": 0.7,
	}
	if ragContext != "" {
		payload["context"] = ragContext
	}

	resp, err := s.ai.Dispatch(ctx, "content_generation", AIRequest{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		slog.Warn("assessment: ai roadmap: dispatch failed", "error", err)
		return nil
	}

	if resp.Status != "success" {
		slog.Warn("assessment: ai roadmap: non-success status", "status", resp.Status)
		return nil
	}

	// The AI gateway returns provider-specific response data in Raw.
	// We expect a "text" or "content" field containing the generated text.
	rawText, ok := extractAIText(resp.Raw)
	if !ok || rawText == "" {
		slog.Warn("assessment: ai roadmap: no text in response")
		return nil
	}

	// The AI is instructed to return a JSON array. Parse it.
	// If parsing fails, log and fall back to curated-only.
	var steps []RoadmapStep
	if err := json.Unmarshal([]byte(rawText), &steps); err != nil {
		// Try extracting JSON array from prose response.
		jsonStart := strings.Index(rawText, "[")
		jsonEnd := strings.LastIndex(rawText, "]")

		if jsonStart >= 0 && jsonEnd > jsonStart {
			if err := json.Unmarshal([]byte(rawText[jsonStart:jsonEnd+1]), &steps); err != nil {
				slog.Warn("assessment: ai roadmap: parse steps failed", "error", err)
				return nil
			}
		} else {
			slog.Warn("assessment: ai roadmap: no JSON array in response")
			return nil
		}
	}

	// Cap at 3 AI steps.
	if len(steps) > 3 {
		steps = steps[:3]
	}

	return steps
}

// extractAIText extracts the generated text from the AI gateway's
// provider-specific response data. Different providers use different field
// names ("text", "content", "output", "generated_text").
func extractAIText(raw map[string]any) (string, bool) {
	for _, key := range []string{"text", "content", "output", "generated_text", "response"} {
		if v, ok := raw[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
	}

	return "", false
}

// CleanupAnonymousResults deletes anonymous assessment_results older than the
// given retention window. Called by the assessment:cleanup_anonymous Asynq
// task (ADR-071 §5). Returns the number of rows deleted.
func (s *Service) CleanupAnonymousResults(ctx context.Context, retentionHours int32) (int64, error) {
	return s.store.CleanupAnonymousResults(ctx, retentionHours)
}

// curatedRoadmapLibrary returns the deterministic mapping of dimension names
// to recommended roadmap steps. Each step links to a platform content slug.
// This is the default fallback library used when scoring_config.curated_roadmap
// is not set; admins can override it via scoring_config (ADR-071 §3).
// // The library covers both the default seeded form dimensions (financial,
// logistical, professional, personal — see migration 000279) and the
// ADR-071 §1 example dimensions (language, qualifications,
// visa_eligibility, destination_knowledge) so the roadmap is populated
// regardless of which dimension set the active form uses.
func curatedRoadmapLibrary() map[string][]RoadmapStep {
	return map[string][]RoadmapStep{
		"financial": {
			{
				Title:       "Build your relocation savings fund",
				Description: "Calculate your target savings based on your destination's cost of living and create a monthly savings plan.",
				ContentSlug: "guide-banking-finance-relocation",
			},
			{
				Title:       "Use the Cost of Living Calculator",
				Description: "Compare living costs between your current city and target destination to set a realistic budget.",
				ContentSlug: "tool-cost-of-living-calculator",
			},
		},
		"logistical": {
			{
				Title:       "Determine your visa pathway",
				Description: "Research visa categories you qualify for based on your skills, experience, and destination.",
				ContentSlug: "guide-visa-pathways-overview",
			},
			{
				Title:       "Prepare your visa application documents",
				Description: "Gather and organise the documents required for your target visa category.",
				ContentSlug: "guide-visa-document-checklist",
			},
			{
				Title:       "Research housing at your destination",
				Description: "Compare neighbourhoods, rental costs, and short-term accommodation options before you arrive.",
				ContentSlug: "guide-destination-research-guide",
			},
		},
		"professional": {
			{
				Title:       "Get your credentials evaluated",
				Description: "Research credential recognition requirements for your profession in your target country.",
				ContentSlug: "guide-credential-evaluation",
			},
			{
				Title:       "Upgrade your professional certifications",
				Description: "Identify and pursue certifications that are in demand in your destination's job market.",
				ContentSlug: "guide-professional-certifications",
			},
		},
		"personal": {
			{
				Title:       "Connect with the diaspora community",
				Description: "Join spaces and forums for people from your home country in your destination country for on-the-ground insights and support.",
				ContentSlug: "guide-connecting-with-diaspora",
			},
			{
				Title:       "Plan your emotional and family readiness",
				Description: "Reflect on cultural adjustment, family logistics, and support networks to ease the transition.",
				ContentSlug: "guide-emotional-readiness-relocation",
			},
		},
		"language": {
			{
				Title:       "Start a language learning track",
				Description: "Begin structured language lessons for your destination country's primary language.",
				ContentSlug: "track-language-proficiency",
			},
		},
		"qualifications": {
			{
				Title:       "Get your credentials evaluated",
				Description: "Research credential recognition requirements for your profession in your target country.",
				ContentSlug: "guide-credential-evaluation",
			},
			{
				Title:       "Upgrade your professional certifications",
				Description: "Identify and pursue certifications that are in demand in your destination's job market.",
				ContentSlug: "guide-professional-certifications",
			},
		},
		"visa_eligibility": {
			{
				Title:       "Determine your visa pathway",
				Description: "Research visa categories you qualify for based on your skills, experience, and destination.",
				ContentSlug: "guide-visa-pathways-overview",
			},
			{
				Title:       "Prepare your visa application documents",
				Description: "Gather and organise the documents required for your target visa category.",
				ContentSlug: "guide-visa-document-checklist",
			},
		},
		"destination_knowledge": {
			{
				Title:       "Research your destination city",
				Description: "Learn about housing, healthcare, transportation, and community in your target city.",
				ContentSlug: "guide-destination-research-guide",
			},
			{
				Title:       "Connect with the diaspora community",
				Description: "Join spaces and forums for people from your home country in your destination country for on-the-ground insights.",
				ContentSlug: "guide-connecting-with-diaspora",
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
		placeholder := ""
		if f.Placeholder != nil {
			placeholder = *f.Placeholder
		}

		opts := f.Options
		if opts == nil {
			opts = json.RawMessage("[]")
		}

		fieldDTOs = append(fieldDTOs, AssessmentField{
			ID:          f.ID.String(),
			Label:       f.Label,
			Key:         f.Key,
			FieldType:   f.FieldType,
			Required:    f.IsRequired,
			Position:    f.Position,
			Placeholder: placeholder,
			Options:     opts,
			Config:      parseFieldScoringConfig(f.Config),
		})
	}

	return &AssessmentFormDTO{
		ID: form.ID.String(),

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
