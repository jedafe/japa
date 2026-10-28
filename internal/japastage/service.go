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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/visitor"
)

// ProfileDTO is the API response shape for a single japa stage profile
// (own view — includes history and visibility flag).
type ProfileDTO struct {
	UserID        string          `json:"user_id"`
	Username      string          `json:"username,omitempty"`
	DisplayName   string          `json:"display_name,omitempty"`
	AvatarURL     string          `json:"avatar_url,omitempty"`
	Stage         string          `json:"stage,omitempty"`
	Intent        string          `json:"intent,omitempty"`
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
	Intent        string `json:"intent,omitempty"`
	DestSpaceID   string `json:"destination_space_id,omitempty"`
	DestSpaceName string `json:"destination_space_name,omitempty"`
	DestSpaceCode string `json:"destination_space_code,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// UpdateStageInput is the request body for PATCH /me/japa-stage. IsPublic
// is a pointer so omitted (`null`/absent) means "leave unchanged" while an
// explicit `false` clears visibility (§87 japa_stage_is_public toggle).
// Stage may be omitted when only the intent is being set (ADR-122); Intent
// may be omitted to leave the journey class unchanged. A submitted stage
// must belong to the effective intent's applicable set (ADR-122
// class-scoped terminals).
type UpdateStageInput struct {
	Stage       string `json:"stage"`
	Intent      string `json:"intent"`
	DestSpaceID string `json:"destination_space_id"`
	IsPublic    *bool  `json:"is_public"`
}

// ErrInvalidStageForIntent is returned when a japa_stage is outside the
// effective intent's applicable set (ADR-122 intent×stage rule — e.g.
// settled on a visit intent, or returned on a work intent). The HTTP layer
// maps it to a 400 invalid_stage_for_intent response.
var ErrInvalidStageForIntent = errors.New("japastage: stage is not valid for the given intent")

// Notifier is the interface used to dispatch japa-stage recommendation
// notifications. Satisfied by *notifications.Service; declared here to
// avoid an import cycle (notifications → worker → japastage).
// Ref: japaconnect-engineering/sections/35-notification-system.md.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// AssessmentChecker reports whether a user has any completed Readiness
// Assessment results. Satisfied by *assessment.Store; declared here to
// avoid an import cycle. Ref: §71/§91 Readiness Assessment.
type AssessmentChecker interface {
	CountUserAssessmentResults(ctx context.Context, userID pgtype.UUID) (int64, error)
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the business logic for Japa Stage Progress Profiles.
type Service struct {
	store    *Store
	settings *config.Settings
	checker  FeatureChecker
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

// SetFeatureChecker injects the feature flag checker so Enabled reads the
// feature_flags table (the single source of truth for feature reachability)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// Enabled reads the japa_stage.enabled feature flag (§87); system_settings
// is consulted only as a fallback when no checker is injected.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "japa_stage.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "japa_stage.enabled", false)
}

// achievementFeedEnabled resolves the achievement.enabled feature flag —
// used by the §87 integration prompt that suggests posting to the §89
// Achievement Feed when an arrival milestone is reached.
func (s *Service) achievementFeedEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "achievement.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "achievement.enabled", false)
}

// Option is a single selectable value exposed by the option endpoints.
// Intents lists the journey classes a stage applies to (ADR-122) and is
// only populated on stage options — empty for intent options.
type Option struct {
	Value   string   `json:"value"`
	Label   string   `json:"label"`
	Intents []string `json:"intents,omitempty"`
}

var stageLabels = map[string]string{
	"considering": "Considering",
	"planning":    "Planning",
	"preparing":   "Preparing",
	"applied":     "Applied",
	"approved":    "Approved",
	"relocated":   "Relocated",
	"settled":     "Settled",
	"traveled":    "Traveled",
	"returned":    "Returned",
}

var (
	relocationIntentList = []string{"relocation", "study", "work", "family"}
	tripIntentList       = []string{"visit", "business"}
)

// stageIntents maps each stage to the journey classes it applies to —
// shared-arc stages list all six intents, terminal stages list only their
// class (ADR-122).
var stageIntents = map[string][]string{
	"considering": validIntents,
	"planning":    validIntents,
	"preparing":   validIntents,
	"applied":     validIntents,
	"approved":    validIntents,
	"relocated":   relocationIntentList,
	"settled":     relocationIntentList,
	"traveled":    tripIntentList,
	"returned":    tripIntentList,
}

// Stages returns the canonical japa stage option list (Rule 16). Each
// option carries the journey classes it applies to. A non-empty intent
// filters the list to that class's applicable stages.
func (s *Service) Stages(intent string) []Option {
	values := validStages
	if intent != "" {
		values = StagesForIntent(intent)
	}

	out := make([]Option, 0, len(values))
	for _, v := range values {
		out = append(out, Option{Value: v, Label: stageLabels[v], Intents: stageIntents[v]})
	}

	return out
}

var intentLabels = map[string]string{
	"relocation": "Relocation",
	"study":      "Study abroad",
	"work":       "Work",
	"family":     "Family route",
	"visit":      "Visit / tourism",
	"business":   "Business",
}

// Intents returns the canonical japa_intent option list (Rule 16, ADR-122).
func (s *Service) Intents() []Option {
	out := make([]Option, 0, len(validIntents))
	for _, v := range validIntents {
		out = append(out, Option{Value: v, Label: intentLabels[v]})
	}

	return out
}

// GetOwn returns the authenticated user's own japa stage profile (full
// view, including history and the visibility flag). Backs GET /me/japa-stage.
func (s *Service) GetOwn(ctx context.Context, userID pgtype.UUID) (ProfileDTO, error) {
	row, err := s.store.GetUserProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProfileDTO{}, pgx.ErrNoRows
		}

		return ProfileDTO{}, fmt.Errorf("japastage: get own: %w", err)
	}

	return profileRowToDTO(row), nil
}

// GetByUsername returns the public japa stage profile for a username. If the
// profile is not public (japa_stage_is_public = false), an empty DTO is
// returned without error so the public view simply shows nothing.
func (s *Service) GetByUsername(ctx context.Context, username string) (PublicProfileDTO, error) {
	row, err := s.store.GetProfileByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicProfileDTO{}, pgx.ErrNoRows
		}

		return PublicProfileDTO{}, fmt.Errorf("japastage: get by username: %w", err)
	}

	if !row.JapaStagePublic {
		return PublicProfileDTO{}, nil
	}

	return profileRowToPublicDTO(row), nil
}

// UpdateOwn updates the authenticated user's own japa stage, intent,
// destination space, and (when IsPublic is non-nil) visibility. Stage and
// intent are independently optional: a stage-only PATCH leaves the intent
// unchanged, an intent-only PATCH leaves the stage unchanged — unless the
// new intent class makes the stored stage incompatible, in which case it
// clears (ADR-122 class-scoped stages). On success the caller should fire
// the japa_stage:recommend Asynq task (Rule 8 — fire-and-forget) so the
// worker can dispatch recommendations.
func (s *Service) UpdateOwn(ctx context.Context, userID pgtype.UUID, input UpdateStageInput) (ProfileDTO, error) {
	if input.Stage != "" && !ValidStage(input.Stage) {
		return ProfileDTO{}, fmt.Errorf("invalid stage %q; must be one of: %s", input.Stage, JoinStages())
	}

	if input.Intent != "" && !ValidIntent(input.Intent) {
		return ProfileDTO{}, fmt.Errorf("invalid intent %q; must be one of: %s", input.Intent, JoinIntents())
	}

	if input.Stage == "" && input.Intent == "" {
		return ProfileDTO{}, errors.New("stage or intent is required")
	}

	// Resolve the effective intent: the submitted value wins, otherwise the
	// stored one applies. Needed to validate a submitted stage against the
	// intent×stage rule even when the member did not resend the intent.
	var currentIntent, currentStage string

	if cur, err := s.store.GetUserProfile(ctx, userID); err == nil {
		if cur.JapaIntent != nil {
			currentIntent = *cur.JapaIntent
		}

		if cur.JapaStage != nil {
			currentStage = *cur.JapaStage
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ProfileDTO{}, fmt.Errorf("japastage: read current profile: %w", err)
	}

	effectiveIntent := input.Intent
	if effectiveIntent == "" {
		effectiveIntent = currentIntent
	}

	var stageParam *string

	switch {
	case input.Stage != "":
		if !ValidStageForIntent(input.Stage, effectiveIntent) {
			return ProfileDTO{}, ErrInvalidStageForIntent
		}

		stageParam = &input.Stage
	case input.Intent != "" && currentStage != "" && !ValidStageForIntent(currentStage, effectiveIntent):
		// The class switch makes the stored stage incompatible (e.g.
		// relocated → visit) — clear it; the history entry records the
		// transition and shared-arc stages are preserved.
		clear := ""
		stageParam = &clear
	}

	var intentParam *string
	if input.Intent != "" {
		intentParam = &input.Intent
	}

	var destSpaceID pgtype.UUID
	if input.DestSpaceID != "" {
		if err := destSpaceID.Scan(input.DestSpaceID); err != nil {
			return ProfileDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}
	}

	if err := s.store.UpdateStage(ctx, userID, stageParam, intentParam, destSpaceID, input.IsPublic); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProfileDTO{}, pgx.ErrNoRows
		}

		return ProfileDTO{}, fmt.Errorf("japastage: update own: %w", err)
	}

	row, err := s.store.GetUserProfile(ctx, userID)
	if err != nil {
		return ProfileDTO{}, fmt.Errorf("japastage: reload profile: %w", err)
	}

	return profileRowToDTO(row), nil
}

// ApplyJourneyClass merges journey-class writes from surfaces that learn
// the intent indirectly — onboarding primary_goal / japa_status answers and
// the purpose custom field on PUT /me/journey (ADR-128). rawIntent is
// normalized through NormalizeIntent (accepts primary_goal and purpose
// vocabularies); stage must already be a canonical §87 stage or empty.
// Unlike UpdateOwn the write is a merge: empty arguments leave the column
// unchanged, and the intent×stage rule is resolved atomically inside
// UpsertJourneyClass — a submitted stage that is invalid for the effective
// intent is ignored, and a stored stage invalidated by a class switch is
// cleared in the same statement (no read-then-write race).
func (s *Service) ApplyJourneyClass(ctx context.Context, userID pgtype.UUID, rawIntent, stage, source string) error {
	intent := visitor.NormalizeIntent(rawIntent)
	if intent == "" && stage == "" {
		return nil
	}

	if stage != "" && !ValidStage(stage) {
		stage = "" // never a canonical §87 stage — drop rather than write
	}

	if err := s.store.UpsertJourneyClass(ctx, userID, intent, stage, source); err != nil {
		return fmt.Errorf("japastage: apply journey class: %w", err)
	}

	return nil
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
// intent is the member's journey class (ADR-122) and may be empty.
func (s *Service) RecommendForStage(ctx context.Context, userID pgtype.UUID, stage, intent, destSpaceID string) error {
	if s.settings == nil {
		return nil
	}

	// (1) Tool recommendation notification.
	if s.notifier != nil {
		tools := s.resolveToolSlugs(ctx, stage, intent)
		title := "Recommended tools for your stage"

		body := "Based on your japa stage, these tools may help next."
		if len(tools) > 0 {
			body = "Recommended next: " + strings.Join(tools, ", ") + "."
		}

		zero := pgtype.UUID{}
		if err := s.notifier.CreateNotification(ctx, userID, "japa_stage.recommend", "system", zero, "japa_stage", userID, title, body, "/japa-stage/me"); err != nil {
			slog.Warn("japastage: recommend notification failed", "error", err)
		}
	}

	// (2) JapaBot peer card update via Honcho (§69).
	if s.honcho != nil {
		content := fmt.Sprintf("japa_stage=%s; japa_intent=%s; destination_space_id=%s", stage, intent, destSpaceID)
		if _, err := s.honcho.AddMemory(ctx, userID.String(), honcho.AddMemoryInput{
			Content:  content,
			Metadata: map[string]any{"japa_stage": stage, "japa_intent": intent, "destination_space_id": destSpaceID},
		}); err != nil {
			slog.Warn("japastage: honcho peer card update failed", "error", err)
		}
	}

	// (3) Readiness Assessment prompt (stage=planning, no completed result).
	// Relocation-class only — the assessment measures relocation readiness,
	// so visit/business members are not prompted (ADR-122). An empty intent
	// preserves legacy behavior for payloads that predate the intent field.
	if stage == "planning" && (intent == "" || IsRelocationIntent(intent)) && s.assess != nil {
		count, err := s.assess.CountUserAssessmentResults(ctx, userID)
		if err != nil {
			slog.Warn("japastage: assessment count failed", "error", err)
		} else if count == 0 && s.notifier != nil {
			zero := pgtype.UUID{}
			if err := s.notifier.CreateNotification(ctx, userID, "japa_stage.readiness_prompt", "system", zero, "japa_stage", userID, "Ready to plan? Take the Readiness Assessment", "Complete the Readiness Assessment to get a personalised roadmap.", "/tools/readiness"); err != nil {
				slog.Warn("japastage: readiness prompt notification failed", "error", err)
			}
		}
	}

	// (4) Achievement Feed prompt (stage=relocated, settled, or traveled —
	// the arrival milestone of each class; §89 enabled).
	// Ref: §87 Integration — "stage advance to relocated, settled, or
	// traveled triggers an opt-in prompt to post to the achievement feed."
	if (stage == "relocated" || stage == "settled" || stage == "traveled") && s.notifier != nil &&
		s.achievementFeedEnabled(ctx) {
		zero := pgtype.UUID{}
		title := "Share your milestone in the Achievement Feed"
		body := "You've reached a major journey milestone. Share your win with the community to inspire others on the same journey."

		if err := s.notifier.CreateNotification(ctx, userID, "japa_stage.achievement_prompt", "system", zero, "japa_stage", userID, title, body, "/achievements/new"); err != nil {
			slog.Warn("japastage: achievement prompt notification failed", "error", err)
		}
	}

	return nil
}

// resolveToolSlugs reads the geo_personalization.stage_tool_matrix setting
// and returns the tool slugs for the given stage. Resolution order per
// ADR-064a: exact "{archetype}:{stage}" → "{archetype}:*" → "*:{stage}" →
// "*:*". The archetype is resolved by the recommendations resolver (§50);
// here the member's japa_intent fills the archetype slot (ADR-122 — e.g.
// "study:applied" beats "*:applied"), with the stage-only and wildcard
// fallbacks preserved.
func (s *Service) resolveToolSlugs(ctx context.Context, stage, intent string) []string {
	raw := s.settings.GetStringOr(ctx, "geo_personalization.stage_tool_matrix", "")
	if raw == "" {
		return nil
	}

	var matrix map[string][]string
	if err := json.Unmarshal([]byte(raw), &matrix); err != nil {
		slog.Warn("japastage: invalid stage_tool_matrix JSON", "error", err)
		return nil
	}

	if intent != "" {
		if v, ok := matrix[intent+":"+stage]; ok && len(v) > 0 {
			return v
		}

		if v, ok := matrix[intent+":*"]; ok && len(v) > 0 {
			return v
		}
	}

	if v, ok := matrix["*:"+stage]; ok && len(v) > 0 {
		return v
	}

	if v, ok := matrix["*:*"]; ok && len(v) > 0 {
		return v
	}

	return nil
}

// ListByStage returns a paginated directory of public japa stage profiles.
// stage, when non-empty, filters to that stage; intent, when non-empty,
// filters to that journey class (ADR-122). When both are empty, all public
// profiles with a set stage are returned. destSpaceID, when non-empty,
// further filters by destination (geo-aware ?dest= filter, §87).
func (s *Service) ListByStage(ctx context.Context, stage, intent, destSpaceID string, page, limit int32) ([]PublicProfileDTO, int64, error) {
	if stage != "" && !ValidStage(stage) {
		return nil, 0, fmt.Errorf("invalid stage %q; must be one of: %s", stage, JoinStages())
	}

	if intent != "" && !ValidIntent(intent) {
		return nil, 0, fmt.Errorf("invalid intent %q; must be one of: %s", intent, JoinIntents())
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

	rows, total, err := s.store.ListByStage(ctx, stage, intent, dest, limit, offset)
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

	if r.JapaIntent != nil {
		dto.Intent = *r.JapaIntent
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

	if r.JapaIntent != nil {
		dto.Intent = *r.JapaIntent
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

	if r.JapaIntent != nil {
		dto.Intent = *r.JapaIntent
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

// IsBlocked reports whether either user has blocked the other (§41).
// japa_stage uses direct pgx (no sqlc queries yet) — same EXISTS shape.
func (s *Service) IsBlocked(ctx context.Context, a, b pgtype.UUID) (bool, error) {
	var blocked bool
	err := s.store.pool.Reader.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_blocks
		 WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1))`,
		a, b).Scan(&blocked)
	return blocked, err
}
