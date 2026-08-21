// Package japastage — service.go orchestrates japa stage profile reads,
// stage updates, visibility toggles, and the public stage directory. It is
// feature-flagged via the japa_stage.enabled system_setting (§87).
//
// On a successful stage change, UpdateOwn fires the japa_stage:recommend
// Asynq task (§87 Stage-Based Recommendations) which dispatches tool
// recommendations, updates the JapaBot peer card via Honcho, and prompts a
// Readiness Assessment when the new stage is `planning` and the member has
// no completed assessment.
//
// Ref: japaconnect-engineering/sections/87-japa-stage-progress-profiles.md
//
//	japaconnect-engineering/adr/ADR-095-japa-stage-profiles.md
package japastage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/honcho"
)

// ProfileDTO is the API response shape for a single japa stage profile
// (own view — includes history and visibility flag).
type ProfileDTO struct {
	UserID        string          `json:"user_id"`
	Username      string          `json:"username,omitempty"`
	DisplayName   string          `json:"display_name,omitempty"`
	AvatarURL     string          `json:"avatar_url,omitempty"`
	Stage         string          `json:"stage,omitempty"`
	DestSpaceID   string          `json:"destination_space_id,omitempty"`
	DestSpaceName string          `json:"destination_space_name,omitempty"`
	DestSpaceCode string          `json:"destination_space_code,omitempty"`
	IsPublic      bool            `json:"is_public"`
	History       json.RawMessage `json:"history,omitempty"`
	UpdatedAt     string          `json:"updated_at,omitempty"`
}

// PublicProfileDTO is the trimmed shape returned for the public view and
// the stage directory. History and the visibility flag are omitted.
type PublicProfileDTO struct {
	UserID        string `json:"user_id"`
	Username      string `json:"username,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	Stage         string `json:"stage"`
	DestSpaceID   string `json:"destination_space_id,omitempty"`
	DestSpaceName string `json:"destination_space_name,omitempty"`
	DestSpaceCode string `json:"destination_space_code,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// UpdateStageInput is the request body for PATCH /me/japa-stage. IsPublic
// is a pointer so omitted (`null`/absent) means "leave unchanged" while an
// explicit `false` clears visibility (§87 japa_stage_is_public toggle).
type UpdateStageInput struct {
	Stage       string `json:"stage"`
	DestSpaceID string `json:"destination_space_id"`
	IsPublic    *bool  `json:"is_public"`
}

// Notifier is the interface used to dispatch japa-stage recommendation
// notifications. Satisfied by *notifications.Service; declared here to
// avoid an import cycle (notifications → worker → japastage).
// Ref: japaconnect-engineering/sections/35-notification-system.md
type Notifier interface {
	CreateNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// AssessmentChecker reports whether a user has any completed Readiness
// Assessment results. Satisfied by *assessment.Store; declared here to
// avoid an import cycle. Ref: §71/§91 Readiness Assessment.
type AssessmentChecker interface {
	CountUserAssessmentResults(ctx context.Context, tenantID, userID pgtype.UUID) (int64, error)
}

// Service holds the business logic for Japa Stage Progress Profiles.
type Service struct {
	store    *Store
	settings *config.Settings
	notifier Notifier
	honcho   *honcho.Client
	assess   AssessmentChecker
}

// NewService creates a japa stage service with the core dependencies needed
// for reads and updates. Use WithNotifier/WithHoncho/WithAssessmentChecker
// to wire the optional collaborators used by the japa_stage:recommend
// fan-out (§87 Stage-Based Recommendations).
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// WithNotifier attaches the notification dispatcher used by RecommendForStage.
func (s *Service) WithNotifier(n Notifier) *Service { s.notifier = n; return s }

// WithHoncho attaches the Honcho client used to update the JapaBot peer
// card memory on a stage change (§87 / §69).
func (s *Service) WithHoncho(c *honcho.Client) *Service { s.honcho = c; return s }

// WithAssessmentChecker attaches the Readiness Assessment checker used to
// decide whether to prompt a planning-stage member for an assessment (§87).
func (s *Service) WithAssessmentChecker(a AssessmentChecker) *Service { s.assess = a; return s }

// Enabled reads the japa_stage.enabled system_setting toggle (§87 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "japa_stage.enabled", false)
}

// Option is a single selectable value exposed by the option endpoint.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

var stageLabels = map[string]string{
	"considering": "Considering",
	"planning":    "Planning",
	"preparing":   "Preparing",
	"applied":     "Applied",
	"approved":    "Approved",
	"relocated":   "Relocated",
	"settled":     "Settled",
}

// Stages returns the canonical japa stage option list (Rule 16).
func (s *Service) Stages() []Option {
	out := make([]Option, 0, len(validStages))
	for _, v := range validStages {
		out = append(out, Option{Value: v, Label: stageLabels[v]})
	}
	return out
}

// GetOwn returns the authenticated user's own japa stage profile (full
// view, including history and the visibility flag). Backs GET /me/japa-stage.
func (s *Service) GetOwn(ctx context.Context, userID, tenantID pgtype.UUID) (ProfileDTO, error) {
	row, err := s.store.GetUserProfile(ctx, userID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ProfileDTO{}, pgx.ErrNoRows
		}
		return ProfileDTO{}, fmt.Errorf("japastage: get own: %w", err)
	}
	return profileRowToDTO(row), nil
}

// GetByUsername returns the public japa stage profile for a username. If the
// profile is not public (japa_stage_is_public = false), an empty DTO is
// returned without error so the public view simply shows nothing.
func (s *Service) GetByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (PublicProfileDTO, error) {
	row, err := s.store.GetProfileByUsername(ctx, username, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PublicProfileDTO{}, pgx.ErrNoRows
		}
		return PublicProfileDTO{}, fmt.Errorf("japastage: get by username: %w", err)
	}
	if !row.JapaStagePublic {
		return PublicProfileDTO{}, nil
	}
	return profileRowToPublicDTO(row), nil
}

// UpdateOwn updates the authenticated user's own japa stage, destination
// space, and (when IsPublic is non-nil) visibility. The stage must be one
// of the allowed values; destSpaceID may be empty to clear it. On success
// the caller should fire the japa_stage:recommend Asynq task (Rule 8 —
// fire-and-forget) so the worker can dispatch recommendations.
func (s *Service) UpdateOwn(ctx context.Context, userID, tenantID pgtype.UUID, input UpdateStageInput) (ProfileDTO, error) {
	if !ValidStage(input.Stage) {
		return ProfileDTO{}, fmt.Errorf("invalid stage %q; must be one of: %s", input.Stage, JoinStages())
	}

	var destSpaceID pgtype.UUID
	if input.DestSpaceID != "" {
		if err := destSpaceID.Scan(input.DestSpaceID); err != nil {
			return ProfileDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}
	}

	if err := s.store.UpdateStage(ctx, userID, tenantID, input.Stage, destSpaceID, input.IsPublic); err != nil {
		if err == pgx.ErrNoRows {
			return ProfileDTO{}, pgx.ErrNoRows
		}
		return ProfileDTO{}, fmt.Errorf("japastage: update own: %w", err)
	}

	row, err := s.store.GetUserProfile(ctx, userID, tenantID)
	if err != nil {
		return ProfileDTO{}, fmt.Errorf("japastage: reload profile: %w", err)
	}
	return profileRowToDTO(row), nil
}

// RecommendForStage implements the japa_stage:recommend fan-out (§87 Stage-
// Based Recommendations). It is invoked by the Asynq worker after a
// successful PATCH /me/japa-stage and:
//  1. Sends a notification with the recommended tools for the new stage
//     (resolved from geo_personalization.stage_tool_matrix).
//  2. Updates the JapaBot peer card via the Honcho API so japa_stage +
//     destination become implicit context for every conversation (§69).
//  3. Prompts a Readiness Assessment when stage = `planning` and the member
//     has no completed assessment (§71/§91).
//
// Each side effect is best-effort: a failure logs a warning but does not
// abort the others. The task is idempotent on (user_id, stage, set_at) —
// notification dedup + Honcho memory content are keyed on the same stage
// payload, so a retry never double-sends a divergent recommendation.
func (s *Service) RecommendForStage(ctx context.Context, userID, tenantID pgtype.UUID, stage, destSpaceID string) error {
	if s.settings == nil {
		return nil
	}

	// (1) Tool recommendation notification.
	if s.notifier != nil {
		tools := s.resolveToolSlugs(ctx, tenantID, stage)
		title := "Recommended tools for your stage"
		body := "Based on your japa stage, these tools may help next."
		if len(tools) > 0 {
			body = "Recommended next: " + strings.Join(tools, ", ") + "."
		}
		zero := pgtype.UUID{}
		if err := s.notifier.CreateNotification(ctx, tenantID, userID, "japa_stage.recommend", "system", zero, "japa_stage", userID, title, body, "/japa-stage/me"); err != nil {
			slog.Warn("japastage: recommend notification failed", "error", err)
		}
	}

	// (2) JapaBot peer card update via Honcho (§69).
	if s.honcho != nil {
		content := fmt.Sprintf("japa_stage=%s; destination_space_id=%s", stage, destSpaceID)
		if _, err := s.honcho.AddMemory(ctx, userID.String(), honcho.AddMemoryInput{
			Content:  content,
			Metadata: map[string]any{"japa_stage": stage, "destination_space_id": destSpaceID},
		}); err != nil {
			slog.Warn("japastage: honcho peer card update failed", "error", err)
		}
	}

	// (3) Readiness Assessment prompt (stage=planning, no completed result).
	if stage == "planning" && s.assess != nil {
		count, err := s.assess.CountUserAssessmentResults(ctx, tenantID, userID)
		if err != nil {
			slog.Warn("japastage: assessment count failed", "error", err)
		} else if count == 0 && s.notifier != nil {
			zero := pgtype.UUID{}
			if err := s.notifier.CreateNotification(ctx, tenantID, userID, "japa_stage.readiness_prompt", "system", zero, "japa_stage", userID, "Ready to plan? Take the Readiness Assessment", "Complete the Readiness Assessment to get a personalised roadmap.", "/assessment"); err != nil {
				slog.Warn("japastage: readiness prompt notification failed", "error", err)
			}
		}
	}

	// (4) Achievement Feed prompt (stage=relocated or settled, §89 enabled).
	// Ref: §87 Integration — "stage advance to relocated or settled triggers
	// an opt-in prompt to post to the achievement feed."
	if (stage == "relocated" || stage == "settled") && s.notifier != nil &&
		s.settings.GetBoolOr(ctx, tenantID, "achievement.enabled", false) {
		zero := pgtype.UUID{}
		title := "Share your milestone in the Achievement Feed"
		body := "You've reached a major relocation milestone. Share your win with the community to inspire others on the same journey."
		if err := s.notifier.CreateNotification(ctx, tenantID, userID, "japa_stage.achievement_prompt", "system", zero, "japa_stage", userID, title, body, "/achievements/new"); err != nil {
			slog.Warn("japastage: achievement prompt notification failed", "error", err)
		}
	}

	return nil
}

// resolveToolSlugs reads the geo_personalization.stage_tool_matrix setting
// and returns the tool slugs for the given stage. Resolution order per
// ADR-064a: exact "{archetype}:{stage}" → "{archetype}:*" → "*:{stage}" →
// "*:*". The archetype is resolved by the recommendations resolver (§50);
// here only the stage-only fallbacks ("*:{stage}" and "*:*") are used.
func (s *Service) resolveToolSlugs(ctx context.Context, tenantID pgtype.UUID, stage string) []string {
	raw := s.settings.GetStringOr(ctx, tenantID, "geo_personalization.stage_tool_matrix", "")
	if raw == "" {
		return nil
	}
	var matrix map[string][]string
	if err := json.Unmarshal([]byte(raw), &matrix); err != nil {
		slog.Warn("japastage: invalid stage_tool_matrix JSON", "error", err)
		return nil
	}
	if v, ok := matrix["*:"+stage]; ok && len(v) > 0 {
		return v
	}
	if v, ok := matrix["*:*"]; ok && len(v) > 0 {
		return v
	}
	return nil
}

// ListByStage returns a paginated directory of public profiles for a given
// stage. Only profiles with japa_stage_is_public = true are included.
// destSpaceID, when non-empty, further filters by destination (geo-aware
// ?dest= filter, §87).
func (s *Service) ListByStage(ctx context.Context, tenantID pgtype.UUID, stage, destSpaceID string, page, limit int32) ([]PublicProfileDTO, int64, error) {
	if !ValidStage(stage) {
		return nil, 0, fmt.Errorf("invalid stage %q; must be one of: %s", stage, JoinStages())
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	var dest pgtype.UUID
	if destSpaceID != "" {
		if err := dest.Scan(destSpaceID); err != nil {
			return nil, 0, fmt.Errorf("invalid dest filter: %w", err)
		}
	}

	rows, total, err := s.store.ListByStage(ctx, tenantID, stage, dest, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("japastage: list by stage: %w", err)
	}

	dtos := make([]PublicProfileDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, directoryRowToDTO(r))
	}
	return dtos, total, nil
}

// profileRowToDTO converts a ProfileRow (joined query result) to a ProfileDTO
// (own view — includes history and visibility).
func profileRowToDTO(r ProfileRow) ProfileDTO {
	dto := ProfileDTO{
		UserID:    r.UserID.String(),
		IsPublic:  r.JapaStagePublic,
		UpdatedAt: formatTs(r.JapaStageUpdated),
	}
	if r.Username != nil {
		dto.Username = *r.Username
	}
	if r.DisplayName != nil {
		dto.DisplayName = *r.DisplayName
	}
	if r.AvatarURL != nil {
		dto.AvatarURL = *r.AvatarURL
	}
	if r.JapaStage != nil {
		dto.Stage = *r.JapaStage
	}
	if r.JapaStageDestID.Valid {
		dto.DestSpaceID = r.JapaStageDestID.String()
	}
	if r.JapaStageDestName != nil {
		dto.DestSpaceName = *r.JapaStageDestName
	}
	if r.JapaStageDestCode != nil {
		dto.DestSpaceCode = *r.JapaStageDestCode
	}
	if len(r.JapaStageHistory) > 0 && string(r.JapaStageHistory) != "null" {
		dto.History = json.RawMessage(r.JapaStageHistory)
	}
	return dto
}

// profileRowToPublicDTO converts a ProfileRow to a PublicProfileDTO (public
// view — omits history and visibility).
func profileRowToPublicDTO(r ProfileRow) PublicProfileDTO {
	dto := PublicProfileDTO{
		UserID:    r.UserID.String(),
		UpdatedAt: formatTs(r.JapaStageUpdated),
	}
	if r.Username != nil {
		dto.Username = *r.Username
	}
	if r.DisplayName != nil {
		dto.DisplayName = *r.DisplayName
	}
	if r.AvatarURL != nil {
		dto.AvatarURL = *r.AvatarURL
	}
	if r.JapaStage != nil {
		dto.Stage = *r.JapaStage
	}
	if r.JapaStageDestID.Valid {
		dto.DestSpaceID = r.JapaStageDestID.String()
	}
	if r.JapaStageDestName != nil {
		dto.DestSpaceName = *r.JapaStageDestName
	}
	if r.JapaStageDestCode != nil {
		dto.DestSpaceCode = *r.JapaStageDestCode
	}
	return dto
}

// directoryRowToDTO converts a DirectoryRow to a PublicProfileDTO.
func directoryRowToDTO(r DirectoryRow) PublicProfileDTO {
	dto := PublicProfileDTO{
		UserID:    r.UserID.String(),
		UpdatedAt: formatTs(r.JapaStageUpdated),
	}
	if r.Username != nil {
		dto.Username = *r.Username
	}
	if r.DisplayName != nil {
		dto.DisplayName = *r.DisplayName
	}
	if r.AvatarURL != nil {
		dto.AvatarURL = *r.AvatarURL
	}
	if r.JapaStage != nil {
		dto.Stage = *r.JapaStage
	}
	if r.JapaStageDestID.Valid {
		dto.DestSpaceID = r.JapaStageDestID.String()
	}
	if r.JapaStageDestName != nil {
		dto.DestSpaceName = *r.JapaStageDestName
	}
	if r.JapaStageDestCode != nil {
		dto.DestSpaceCode = *r.JapaStageDestCode
	}
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
