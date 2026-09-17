// Package assessment — handlers.go provides the HTTP handlers for the Japa
// Readiness Assessment routes (Section 71, ADR-071).
//
// Routes:
//
//	GET   /api/v1/assessment                                — active assessment form + scoring metadata
//	POST  /api/v1/assessment/submit                         — submit answers (anonymous allowed)
//	GET   /api/v1/assessment/result/:id                     — result + roadmap (anonymous results public)
//	GET   /api/v1/assessment/result/by-submission/:submission_id — poll for result after async submit
//	GET   /api/v1/assessment/history                        — user's past results (auth required)
//	GET   /api/v1/admin/assessment/form        — get form config (admin)
//	PATCH /api/v1/admin/assessment/form        — update form/scoring/thresholds (admin)
//	GET   /api/v1/admin/assessment/results     — all users' results (admin)
//	GET   /api/v1/admin/assessment/stats       — score distribution, completion rate (admin)
//
// Ref: japaconnect-engineering/sections/71-readiness-assessment.md
//
//	japaconnect-engineering/adr/ADR-071-japa-readiness-assessment.md
package assessment

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/visitor"
)

// Handlers exposes the assessment HTTP endpoints.
type Handlers struct {
	service     *Service
	feedService *feed.Service
	settings    *config.Settings
}

// NewHandlers creates assessment handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// SetFeedService injects the feed service so the assessment handlers can
// resolve roadmap content items with journey-aware ranking.
func (h *Handlers) SetFeedService(fs *feed.Service) {
	h.feedService = fs
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
// ---------------------------------------------------------------------------

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

// parsePageParams extracts page and limit from query string, applying
// sensible defaults and bounds.
func parsePageParams(r *http.Request) (int32, int32) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	return int32(page), int32(limit) // #nosec
}

// parseScoreParams extracts min_score and max_score from query string.
// A value of 0 means no filter.
func parseScoreParams(r *http.Request) (int32, int32) {
	minScore, _ := strconv.Atoi(r.URL.Query().Get("min_score"))
	maxScore, _ := strconv.Atoi(r.URL.Query().Get("max_score"))

	return int32(minScore), int32(maxScore) // #nosec
}

// formatTimestamp formats a pgtype.Timestamptz as RFC 3339 UTC.
func formatTimestamp(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}

	return ts.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
}

// publicFieldDTOs strips the per-option scores from each field's config before
// sending the form to the browser. Weights and dimensions remain so the UI can
// still render dimension grouping, but the scoring key is internal to the
// backend (ADR-071 §2).
func publicFieldDTOs(fields []AssessmentField) []map[string]any {
	out := make([]map[string]any, 0, len(fields))

	for _, f := range fields {
		opts := f.Options
		if opts == nil {
			opts = json.RawMessage("[]")
		}

		out = append(out, map[string]any{
			"id":          f.ID,
			"label":       f.Label,
			"key":         f.Key,
			"field_type":  f.FieldType,
			"is_required": f.Required,
			"position":    f.Position,
			"placeholder": f.Placeholder,
			"options":     opts,
			"config": map[string]any{
				"weight":    f.Config.Weight,
				"dimension": f.Config.Dimension,
			},
		})
	}

	return out
}

// ---------------------------------------------------------------------------
// Public handlers
// ---------------------------------------------------------------------------

// HandleGetAssessment handles GET /api/v1/assessment — returns the active
// assessment form with scoring metadata (thresholds only, not weights —
// weights are internal per ADR-071 §2). Public route.
func (h *Handlers) HandleGetAssessment(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	// Check the global toggle (assessment.enabled).
	if h.settings != nil {
		if enabled := h.settings.GetBoolOr(r.Context(), "assessment.enabled", true); !enabled {
			errpkg.WriteError(w, http.StatusNotFound, "assessment_disabled", "Assessment is not available")
			return
		}
	}

	form, err := h.service.GetActiveAssessment(r.Context())
	if err != nil {
		slog.Error("assessment: get active failed", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "no_active_assessment", "No active assessment form found")

		return
	}

	// Public response: include thresholds (for the UI to show score bands)
	// but exclude dimension_weights (weights are internal, never exposed to
	// the browser — ADR-071 §2). Also exclude per-option scores from field
	// configs so users cannot read the scoring key. Weight and dimension are
	// kept because the multi-step UI needs dimension for grouping.
	thresholds := make([]map[string]any, 0, len(form.ScoringConfig.Thresholds))
	for _, t := range form.ScoringConfig.Thresholds {
		thresholds = append(thresholds, map[string]any{
			"min":   t.Min,
			"max":   t.Max,
			"label": t.Label,
			"color": t.Color,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":            form.ID,
			"name":          form.Name,
			"slug":          form.Slug,
			"description":   form.Description,
			"is_multi_step": form.IsMultiStep,
			"submit_label":  form.SubmitLabel,
			"status":        form.Status,
			"max_score":     form.ScoringConfig.MaxScore,
			"thresholds":    thresholds,
			"fields":        publicFieldDTOs(form.Fields),
			"dimensions":    assessmentDimensions(form),
			"scoring_config": map[string]any{
				"max_score":  form.ScoringConfig.MaxScore,
				"thresholds": thresholds,
			},
		},
	})
}

// HandleSubmit handles POST /api/v1/assessment/submit — submit assessment
// answers. Anonymous users are allowed; authenticated users get persistent
// history. Body: {"field_values": {...}}.
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	if h.settings != nil {
		if enabled := h.settings.GetBoolOr(r.Context(), "assessment.enabled", true); !enabled {
			errpkg.WriteError(w, http.StatusNotFound, "assessment_disabled", "Assessment is not available")
			return
		}
	}

	var body struct {
		FieldValues json.RawMessage `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(body.FieldValues) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field_values", "field_values is required")
		return
	}

	// Get the active assessment form to resolve the form ID.
	form, err := h.service.GetActiveAssessment(r.Context())
	if err != nil {
		slog.Error("assessment: submit: get form failed", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "no_active_assessment", "No active assessment form found")

		return
	}

	var formID pgtype.UUID
	if err := formID.Scan(form.ID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "invalid_form_id", "Failed to parse form ID")
		return
	}

	// Authentication is optional — anonymous users can submit.
	var userID *uuid.UUID

	if u := auth.FromContext(r.Context()); u != nil {
		uid := uuid.UUID(u.UserID.Bytes)
		userID = &uid
	}

	submissionID, err := h.service.SubmitAssessment(r.Context(), userID, formID, body.FieldValues)
	if err != nil {
		slog.Error("assessment: submit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submission_failed", "Failed to create submission")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"submission_id": submissionID.String(),
			"status":        "queued",
		},
	})
}

// HandleGetResult handles GET /api/v1/assessment/result/:id — returns a
// single assessment result with roadmap. Public if the result has no user_id
// (anonymous); auth required for user results.
func (h *Handlers) HandleGetResult(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	resultID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	result, err := h.service.GetResult(r.Context(), resultID)
	if err != nil {
		slog.Error("assessment: get result failed", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "result_not_found", "Assessment result not found")

		return
	}

	// If the result belongs to a user, require authentication and verify
	// ownership (or admin access). Anonymous results (user_id is null) are
	// public.
	if result.UserID.Valid {
		u, ok := requireUser(w, r)
		if !ok {
			return
		}
		// Only the owner or an admin can view a user's result. The admin
		// routes are gated by RBAC middleware at registration; here we
		// check ownership for the public route.
		if u.UserID.Bytes != result.UserID.Bytes {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have access to this result")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": shapeResult(result)})
}

// HandleGetResultBySubmission handles GET
// /api/v1/assessment/result/by-submission/:submission_id — polls for the
// assessment result linked to a submission. The submit endpoint is async
// (Asynq scoring task, §71): it returns a submission_id immediately, and the
// client polls this endpoint until the worker writes the result.
//
// Returns 200 with the result once scored, 202 with {"status":"pending"} while
// the scoring task is still in flight, and 404 if the submission has no result
// and no in-flight scoring is expected (treated as pending here — the caller
// distinguishes via retry budget on the client).
func (h *Handlers) HandleGetResultBySubmission(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	submissionID, ok := scanUUIDParam(w, r, "submission_id")
	if !ok {
		return
	}

	result, err := h.service.GetResultBySubmission(r.Context(), submissionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Not scored yet — tell the client to keep polling.
			errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
				"data": map[string]any{"status": "pending"},
			})

			return
		}

		slog.Error("assessment: get result by submission failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "result_lookup_failed", "Failed to look up result")

		return
	}

	// Ownership: same rule as HandleGetResult. Anonymous results are public;
	// user results require the owner (or an admin via the admin routes).
	if result.UserID.Valid {
		u, ok := requireUser(w, r)
		if !ok {
			return
		}

		if u.UserID.Bytes != result.UserID.Bytes {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have access to this result")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": shapeResult(result)})
}

// HandleGetResultRoadmap handles GET /api/v1/assessment/result/:id/roadmap.
// It returns the content items for the result's roadmap, ranked by the
// visitor's journey context captured at submission time. Anonymous results are
// public; user results require the owner.
func (h *Handlers) HandleGetResultRoadmap(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	if h.feedService == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "feed_unavailable", "Feed service not configured")
		return
	}

	resultID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	result, err := h.service.GetResult(r.Context(), resultID)
	if err != nil {
		slog.Error("assessment: get result roadmap: load result failed", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "result_not_found", "Assessment result not found")

		return
	}

	// Ownership: same rule as HandleGetResult.
	var requesterID pgtype.UUID

	if result.UserID.Valid {
		u, ok := requireUser(w, r)
		if !ok {
			return
		}

		if u.UserID.Bytes != result.UserID.Bytes {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have access to this result")
			return
		}

		requesterID = u.UserID
	}

	fieldValues, err := h.service.GetSubmissionFieldValues(r.Context(), result.SubmissionID)
	if err != nil {
		slog.Error("assessment: get result roadmap: load submission failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submission_lookup_failed", "Failed to load submission")

		return
	}

	var answers map[string]any
	if len(fieldValues) > 0 {
		_ = json.Unmarshal(fieldValues, &answers)
	}

	if answers == nil {
		answers = map[string]any{}
	}

	journeyCtx := parseJourneyFields(answers)
	vc := visitor.VisitorContext{
		OriginCountry:      journeyCtx.OriginCountry,
		DestinationCountry: journeyCtx.DestinationCountry,
		VisaRoute:          journeyCtx.VisaRoute,
		JourneyStage:       journeyCtx.JourneyStage,
		Purpose:            journeyCtx.Purpose,
		TravelDate:         journeyCtx.TravelDate,
	}

	var roadmap []RoadmapStep
	if len(result.RoadmapContent) > 0 {
		_ = json.Unmarshal(result.RoadmapContent, &roadmap)
	}

	if roadmap == nil {
		roadmap = []RoadmapStep{}
	}

	slugs := make([]string, 0, len(roadmap))

	for _, step := range roadmap {
		if step.ContentSlug != "" {
			slugs = append(slugs, step.ContentSlug)
		}
	}

	page, limit := parsePageParams(r)
	offset := (page - 1) * limit

	items, err := h.feedService.GetRoadmapFeedWithContext(r.Context(), requesterID, slugs, limit, offset, &vc)
	if err != nil {
		slog.Error("assessment: get result roadmap: feed failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "roadmap_feed_failed", "Failed to load roadmap content")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"context": map[string]any{
				"origin_country":      vc.OriginCountry,
				"destination_country": vc.DestinationCountry,
				"visa_route":          vc.VisaRoute,
				"journey_stage":       vc.JourneyStage,
				"purpose":             vc.Purpose,
				"travel_date":         vc.TravelDate,
			},
			"items": items,
		},
	})
}

// shapeResult builds the public JSON shape for an assessment result, parsing
// the dimension_scores and roadmap_content JSONB columns.
func shapeResult(result gen.AssessmentResult) map[string]any {
	var dimensionScores map[string]int
	if len(result.DimensionScores) > 0 {
		_ = json.Unmarshal(result.DimensionScores, &dimensionScores)
	}

	if dimensionScores == nil {
		dimensionScores = map[string]int{}
	}

	var roadmap []RoadmapStep
	if len(result.RoadmapContent) > 0 {
		_ = json.Unmarshal(result.RoadmapContent, &roadmap)
	}

	if roadmap == nil {
		roadmap = []RoadmapStep{}
	}

	userIDStr := ""
	if result.UserID.Valid {
		userIDStr = result.UserID.String()
	}

	return map[string]any{
		"id":               result.ID.String(),
		"total_score":      result.TotalScore,
		"dimension_scores": dimensionScores,
		"threshold_label":  result.ThresholdLabel,
		"roadmap":          roadmap,
		"user_id":          userIDStr,
		"submission_id":    result.SubmissionID.String(),
		"created_at":       formatTimestamp(result.CreatedAt),
	}
}

// HandleHistory handles GET /api/v1/assessment/history — returns the
// authenticated user's past assessment results. Auth required.
func (h *Handlers) HandleHistory(w http.ResponseWriter, r *http.Request) {
	u, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	page, limit := parsePageParams(r)

	results, total, err := h.service.ListHistory(r.Context(), u.UserID, page, limit)
	if err != nil {
		slog.Error("assessment: history failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "history_failed", "Failed to get history")

		return
	}

	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"id":              r.ID.String(),
			"total_score":     r.TotalScore,
			"threshold_label": r.ThresholdLabel,
			"submission_id":   r.SubmissionID.String(),
			"created_at":      formatTimestamp(r.CreatedAt),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// ---------------------------------------------------------------------------
// Admin handlers
// ---------------------------------------------------------------------------

// HandleAdminGetForm handles GET /api/v1/admin/assessment/form — returns the
// assessment form config including full scoring_config (weights, thresholds).
// Admin-only (gated by assessment.manage RBAC middleware).
func (h *Handlers) HandleAdminGetForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	form, err := h.service.GetActiveAssessment(r.Context())
	if err != nil {
		slog.Error("assessment: admin get form failed", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "no_active_assessment", "No active assessment form found")

		return
	}

	// Admin response includes the full scoring_config (weights + thresholds).
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": form,
	})
}

// HandleAdminUpdateForm handles PATCH /api/v1/admin/assessment/form — updates
// the assessment form's scoring_config. Admin-only. Body: {"scoring_config": {...}}.
func (h *Handlers) HandleAdminUpdateForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	var body struct {
		ScoringConfig json.RawMessage `json:"scoring_config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(body.ScoringConfig) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_scoring_config", "scoring_config is required")
		return
	}

	// Resolve the active form ID.
	form, err := h.service.GetActiveAssessment(r.Context())
	if err != nil {
		slog.Error("assessment: admin update: get form failed", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "no_active_assessment", "No active assessment form found")

		return
	}

	var formID pgtype.UUID
	if err := formID.Scan(form.ID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "invalid_form_id", "Failed to parse form ID")
		return
	}

	updated, err := h.service.UpdateFormScoring(r.Context(), formID, body.ScoringConfig)
	if err != nil {
		slog.Error("assessment: admin update form failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update scoring config")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":             updated.ID.String(),
			"form_type":      updated.FormType,
			"scoring_config": json.RawMessage(updated.ScoringConfig),
		},
	})
}

// HandleAdminListResults handles GET /api/v1/admin/assessment/results —
// returns all users' assessment results, paginated and filterable by score
// range. Admin-only (gated by assessment.view_results RBAC middleware).
func (h *Handlers) HandleAdminListResults(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	page, limit := parsePageParams(r)
	minScore, maxScore := parseScoreParams(r)
	sortKey := r.URL.Query().Get("sort")

	sortDir := r.URL.Query().Get("dir")
	if sortDir == "" {
		sortDir = r.URL.Query().Get("order")
	}

	results, total, err := h.service.AdminListResults(r.Context(), minScore, maxScore, page, limit, sortKey, sortDir)
	if err != nil {
		slog.Error("assessment: admin list results failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list results")

		return
	}

	items := make([]map[string]any, 0, len(results))

	for _, r := range results {
		username := ""
		if r.Username != nil {
			username = *r.Username
		}

		userIDStr := ""
		if r.UserID.Valid {
			userIDStr = r.UserID.String()
		}

		items = append(items, map[string]any{
			"id":              r.ID.String(),
			"user_id":         userIDStr,
			"username":        username,
			"total_score":     r.TotalScore,
			"threshold_label": r.ThresholdLabel,
			"submission_id":   r.SubmissionID.String(),
			"created_at":      formatTimestamp(r.CreatedAt),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"pagination": map[string]any{
			"page":      page,
			"limit":     limit,
			"total":     total,
			"min_score": minScore,
			"max_score": maxScore,
		},
	})
}

// HandleAdminStats handles GET /api/v1/admin/assessment/stats — returns
// aggregate score distribution and completion statistics. Admin-only.
func (h *Handlers) HandleAdminStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "assessment_unavailable", "Assessment service not configured")
		return
	}

	distribution, err := h.service.AdminScoreDistribution(r.Context())
	if err != nil {
		slog.Error("assessment: admin stats: distribution failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "Failed to get score distribution")

		return
	}

	completion, err := h.service.AdminCompletionStats(r.Context())
	if err != nil {
		slog.Error("assessment: admin stats: completion failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "Failed to get completion stats")

		return
	}

	distributionItems := make([]map[string]any, 0, len(distribution))
	for _, d := range distribution {
		distributionItems = append(distributionItems, map[string]any{
			"threshold_label": d.ThresholdLabel,
			"count":           d.Count,
			"avg_score":       d.AvgScore,
		})
	}

	completionRate := 0.0
	if completion.TotalSubmissions > 0 {
		completionRate = float64(completion.TotalScored) / float64(completion.TotalSubmissions) * 100.0
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"score_distribution": distributionItems,
			"completion": map[string]any{
				"total_submissions": completion.TotalSubmissions,
				"total_scored":      completion.TotalScored,
				"avg_score":         completion.AvgScore,
				"completion_rate":   completionRate,
			},
		},
	})
}
